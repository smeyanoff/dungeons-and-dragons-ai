package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"syscall"
	"time"

	abilitycheck "dungeons-and-dragons-ai/internal/game/application/ability_check"
	achievementapp "dungeons-and-dragons-ai/internal/game/application/achievement"
	"dungeons-and-dragons-ai/internal/game/application/campaign"
	characterapp "dungeons-and-dragons-ai/internal/game/application/character"
	combatapp "dungeons-and-dragons-ai/internal/game/application/combat"
	"dungeons-and-dragons-ai/internal/game/application/dice"
	dm_analyzer "dungeons-and-dragons-ai/internal/game/application/dm_analyzer"
	"dungeons-and-dragons-ai/internal/game/application/history"
	imageapp "dungeons-and-dragons-ai/internal/game/application/image"
	inventoryapp "dungeons-and-dragons-ai/internal/game/application/inventory"
	locationeventapp "dungeons-and-dragons-ai/internal/game/application/location_event"
	"dungeons-and-dragons-ai/internal/game/application/player_action"
	questapp "dungeons-and-dragons-ai/internal/game/application/quest"
	ratingapp "dungeons-and-dragons-ai/internal/game/application/rating"
	spellapp "dungeons-and-dragons-ai/internal/game/application/spell"
	subscriptionapp "dungeons-and-dragons-ai/internal/game/application/subscription"
	worldeventapp "dungeons-and-dragons-ai/internal/game/application/world_event"
	mapapp "dungeons-and-dragons-ai/internal/game/application/worldmap"
	"dungeons-and-dragons-ai/internal/game/domain/achievement"
	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/combat"
	"dungeons-and-dragons-ai/internal/game/domain/event"
	"dungeons-and-dragons-ai/internal/game/domain/feedback"
	"dungeons-and-dragons-ai/internal/game/domain/inventory"
	"dungeons-and-dragons-ai/internal/game/domain/item"
	llmlogdomain "dungeons-and-dragons-ai/internal/game/domain/llm_log"
	"dungeons-and-dragons-ai/internal/game/domain/player"
	"dungeons-and-dragons-ai/internal/game/domain/quest"
	"dungeons-and-dragons-ai/internal/game/domain/rating"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/game/domain/spell"
	"dungeons-and-dragons-ai/internal/game/domain/subscription"
	"dungeons-and-dragons-ai/internal/game/domain/world"
	dmcache "dungeons-and-dragons-ai/internal/game/infrastructure/cache"
	contextbuilder "dungeons-and-dragons-ai/internal/game/infrastructure/context"
	"dungeons-and-dragons-ai/internal/game/infrastructure/persistence"
	llminfrastructure "dungeons-and-dragons-ai/internal/llm/infrastructure"
	"dungeons-and-dragons-ai/internal/monitoring"
	ragapp "dungeons-and-dragons-ai/internal/rag/application"
	ragembeddings "dungeons-and-dragons-ai/internal/rag/infrastructure/embeddings"
	ragvectorstore "dungeons-and-dragons-ai/internal/rag/infrastructure/vectorstore"
	"dungeons-and-dragons-ai/internal/telegram"
	"dungeons-and-dragons-ai/pkg/gigachat"
	"dungeons-and-dragons-ai/pkg/logger"
	"dungeons-and-dragons-ai/pkg/version"

	"github.com/qdrant/go-client/qdrant"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormLogger "gorm.io/gorm/logger"
)

// dailyQuestProgressAdapterForPlayerAction адаптирует questapp.CheckDailyQuestProgressUseCase к интерфейсу player_action.DailyQuestProgressChecker
type dailyQuestProgressAdapterForPlayerAction struct {
	uc *questapp.CheckDailyQuestProgressUseCase
}

func (a *dailyQuestProgressAdapterForPlayerAction) Execute(ctx context.Context, req player_action.CheckDailyQuestProgressRequest) error {
	// Преобразуем запрос из player_action в формат quest
	questReq := questapp.CheckProgressRequest{
		ChatID:    req.ChatID,
		TgUserID:  req.TgUserID,
		QuestType: req.QuestType,
		Increment: req.Increment,
	}
	return a.uc.Execute(ctx, questReq)
}

// locationEventRepoAdapter адаптирует persistence.WorldEventRepository к locationeventapp.LocationEventRepository
type locationEventRepoAdapter struct {
	repo *persistence.WorldEventRepository
}

func (a *locationEventRepoAdapter) GetByLocationID(ctx context.Context, locationID uint) ([]world.WorldEvent, error) {
	return a.repo.GetByLocationID(ctx, locationID)
}

func (a *locationEventRepoAdapter) Save(ctx context.Context, e *world.WorldEvent) error {
	return a.repo.Save(ctx, e)
}

var bot *telegram.Bot // Глобальная переменная для health check

func main() {
	// Инициализация логгера (должна быть первой)
	if err := logger.InitFromEnv(); err != nil {
		// Fallback на стандартный логгер если не удалось инициализировать
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	// Важно: некоторые зависимости (включая go-telegram-bot-api) используют stdlib `log`
	// и могут выводить URL вида https://api.telegram.org/bot<TOKEN>/...
	// Редактируем такие строки глобально, чтобы секреты не попадали в stdout/stderr и агрегаторы логов.
	log.SetOutput(newRedactingWriter(os.Stderr))

	logger.Info("Starting application",
		logger.String("version", version.Version),
		logger.String("commit", version.Commit),
		logger.String("buildTime", version.BuildTime),
	)

	// Загружаем переменные окружения
	telegramToken := getEnv("TELEGRAM_BOT_TOKEN", "")
	if telegramToken == "" {
		logger.Fatal("TELEGRAM_BOT_TOKEN is required")
	}

	dbDSN := getEnv("DATABASE_URL", "postgres://user:password@localhost:5432/dnd?sslmode=disable")
	gigachatClientID := getEnv("GIGACHAT_CLIENT_ID", "")
	gigachatClientSecret := getEnv("GIGACHAT_CLIENT_SECRET", "")
	gigachatModel := getEnv("GIGACHAT_MODEL", "GigaChat")
	qdrantHost := getEnv("QDRANT_HOST", "localhost")
	qdrantPortStr := getEnv("QDRANT_PORT", "6334")
	qdrantPort := 6334
	if port, err := strconv.Atoi(qdrantPortStr); err == nil {
		qdrantPort = port
	}

	// Инициализация БД
	logger.Info("Initializing database connection")
	db, err := initDB(dbDSN)
	if err != nil {
		logger.Fatal("Failed to initialize database",
			logger.ErrorField(err),
			logger.String("dsn", maskDSN(dbDSN)),
		)
	}
	logger.Info("Database connection established")

	// Автомиграции
	logger.Info("Running database migrations")
	if err := runMigrations(db); err != nil {
		logger.Fatal("Failed to run migrations",
			logger.ErrorField(err),
		)
	}
	logger.Info("Database migrations completed")

	// Инициализация GigaChat
	gigachatCfg := gigachat.Config{
		AuthBaseURL:      getEnv("GIGACHAT_AUTH_URL", "https://ngw.devices.sberbank.ru:9443"),
		APIBaseURL:       getEnv("GIGACHAT_API_URL", "https://gigachat.devices.sberbank.ru/api/v1"),
		ClientID:         gigachatClientID,
		ClientSecret:     gigachatClientSecret,
		Scope:            getEnv("GIGACHAT_SCOPE", "GIGACHAT_API_CORP"),
		ConcurrencyLimit: 1, // Ограничиваем до 1 одновременного запроса для стабильности
		RPSLimit:         5.0, // Увеличиваем до 5 RPS для генерации контента
		RateBurst:        3,   // Burst до 3 запросов для сложных операций
	}

	// Валидация GigaChat credentials
	if gigachatClientID == "" || gigachatClientSecret == "" {
		logger.Fatal("GIGACHAT_CLIENT_ID and GIGACHAT_CLIENT_SECRET are required")
	}

	// Проверка формата credentials (базовая валидация)
	if len(gigachatClientID) < 10 {
		logger.Warn("GIGACHAT_CLIENT_ID seems too short",
			logger.Int("length", len(gigachatClientID)),
		)
	}
	if len(gigachatClientSecret) < 10 {
		logger.Warn("GIGACHAT_CLIENT_SECRET seems too short",
			logger.Int("length", len(gigachatClientSecret)),
		)
	}

	// Проверка scope
	gigachatScope := getEnv("GIGACHAT_SCOPE", "GIGACHAT_API_PERS")
	if gigachatScope == "" {
		logger.Fatal("GIGACHAT_SCOPE is required")
	}
	logger.Info("GigaChat configuration loaded",
		logger.String("authURL", gigachatCfg.AuthBaseURL),
		logger.String("apiURL", gigachatCfg.APIBaseURL),
		logger.String("scope", gigachatScope),
		logger.String("model", gigachatModel),
		logger.String("clientID", maskClientID(gigachatCfg.ClientID)),
		logger.Float64("rps_limit", gigachatCfg.RPSLimit),
		logger.Int("rate_burst", gigachatCfg.RateBurst),
		logger.Int("concurrency_limit", gigachatCfg.ConcurrencyLimit),
	)

	gigachatClient := gigachat.NewClient(gigachatCfg)

	// Пробуем получить токен при старте для проверки credentials (опционально, можно отключить)
	if shouldValidateCredentials := getEnv("GIGACHAT_VALIDATE_ON_START", "false"); shouldValidateCredentials == "true" {
		logger.Info("Validating GigaChat credentials on startup")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		token, err := gigachatClient.GetToken(ctx)
		if err != nil {
			logger.Fatal("Failed to validate GigaChat credentials on startup",
				logger.ErrorField(err),
				logger.String("hint", "Please check GIGACHAT_CLIENT_ID, GIGACHAT_CLIENT_SECRET, and GIGACHAT_SCOPE. You can disable this check by setting GIGACHAT_VALIDATE_ON_START=false"),
			)
		}
		logger.Info("GigaChat credentials validated successfully",
			logger.Bool("token_obtained", token != ""),
		)
		_ = token // Используем токен для проверки, но не сохраняем
	}

	// Инициализация LLM
	baseLLM := llminfrastructure.NewGigachatLLM(gigachatClient, gigachatModel)

	// Инициализация мониторинга LLM
	llmLogRepo := persistence.NewLLMLogRepository(db)
	llm := llminfrastructure.NewMonitoredLLM(baseLLM, llmLogRepo)
	logger.Info("LLM monitoring initialized")

	// Инициализация ImageGenerator для генерации изображений
	imageGenerator := llminfrastructure.NewGigachatImageGenerator(gigachatClient, gigachatModel)

	// Инициализация ImageStorage для локального хранения изображений
	imageStoragePath := getEnv("IMAGE_STORAGE_PATH", "./images")
	imageStorage, err := imageapp.NewLocalImageStorage(imageStoragePath)
	if err != nil {
		logger.Fatal("Failed to create image storage",
			logger.ErrorField(err),
			logger.String("path", imageStoragePath),
		)
	}
	logger.Info("Image storage initialized",
		logger.String("path", imageStoragePath),
	)

	// Создаем ImageGenerationUseCase
	generateImageUC := imageapp.NewImageGenerationUseCase(imageGenerator, imageStorage)

	// Настраиваем лимитер для генерации изображений (5/день для Free по умолчанию)
	// Будет обновлен после создания subscriptionRepo для интеграции с подписками
	dailyLimit := 5
	if envLimit := getEnv("IMAGE_DAILY_LIMIT", ""); envLimit != "" {
		if limit, err := strconv.Atoi(envLimit); err == nil && limit > 0 {
			dailyLimit = limit
		}
	}
	fallbackLimiter := imageapp.NewInMemoryRateLimiter(dailyLimit)
	generateImageUC.SetLimiter(fallbackLimiter)
	logger.Info("Image generation rate limiter configured (will be updated with subscription integration)",
		logger.Int("daily_limit", dailyLimit),
	)

	// Инициализация Qdrant
	logger.Info("Initializing Qdrant client",
		logger.String("host", qdrantHost),
		logger.Int("port", qdrantPort),
	)
	qdrantClient, err := qdrant.NewClient(&qdrant.Config{
		Host: qdrantHost,
		Port: qdrantPort,
		// Qdrant сервер может быть старее клиента; пропускаем проверку совместимости осознанно.
		SkipCompatibilityCheck: true,
	})
	if err != nil {
		logger.Fatal("Failed to initialize Qdrant client",
			logger.ErrorField(err),
			logger.String("host", qdrantHost),
			logger.Int("port", qdrantPort),
		)
	}
	logger.Info("Qdrant client initialized")

	// Инициализация RAG компонентов
	logger.Info("Initializing RAG components")
	embedder := ragembeddings.NewGigachatEmbedder(gigachatClient)
	vectorStore := ragvectorstore.NewQdrantStore(qdrantClient)

	// Инициализация коллекции Qdrant
	logger.Info("Ensuring Qdrant collection exists")
	if err := vectorStore.EnsureCollection(context.Background()); err != nil {
		logger.Fatal("Failed to ensure Qdrant collection",
			logger.ErrorField(err),
		)
	}
	logger.Info("Qdrant collection initialized")

	indexDocUC := ragapp.NewIndexDocument(embedder, vectorStore)
	retrieveContextUC := ragapp.NewRetrieveContext(embedder, vectorStore)

	// Инициализация репозиториев
	worldRepo := persistence.NewWorldRepository(db)
	sessionRepo := persistence.NewGameSessionRepository(db)
	playerRepo := persistence.NewPlayerRepository(db)
	eventRepo := persistence.NewGameEventRepository(db)
	inventoryRepo := persistence.NewInventoryRepository(db)
	combatRepo := persistence.NewCombatRepository(db)
	questRepo := persistence.NewQuestRepository(db)
	dailyQuestRepo := persistence.NewDailyQuestRepository(db)
	worldEventRepo := persistence.NewWorldEventRepository(db)
	feedbackRepo := persistence.NewFeedbackRepository(db)
	achievementRepo := persistence.NewAchievementRepository(db)
	spellRepo := persistence.NewSpellRepository(db)
	subscriptionRepo := persistence.NewSubscriptionRepository(db)

	// Создаем use cases для подписок (нужно для лимитера изображений)
	getSubscriptionUC := subscriptionapp.NewGetSubscriptionUseCase(subscriptionRepo)
	// Передаем sessionRepo как SessionCountRepository (GameSessionRepository реализует нужные методы)
	// и eventRepo как EventCountRepository (GameEventRepository реализует нужные методы)
	checkLimitsUC := subscriptionapp.NewCheckLimitsUseCase(subscriptionRepo, sessionRepo, sessionRepo, eventRepo, fallbackLimiter)

	// Обновляем лимитер изображений для использования системы подписок
	subscriptionImageLimiter := subscriptionapp.NewSubscriptionImageLimiter(checkLimitsUC, fallbackLimiter)
	generateImageUC.SetLimiter(subscriptionImageLimiter)
	logger.Info("Image generation rate limiter updated with subscription integration")

	// Инициализация кэша ответов DM (TTL 1 час)
	responseCache := dmcache.NewDMResponseCache(1 * time.Hour)

	// Инициализация валидатора действий
	actionValidator := player_action.NewActionValidator()

	// Инициализация use cases
	initCampaignUC := campaign.NewInitCampaignUseCase(llm, worldRepo)
	simpleContextBuilder := contextbuilder.NewSimpleContextBuilder()
	ragContextBuilder := contextbuilder.NewRAGContextBuilder(simpleContextBuilder, retrieveContextUC, eventRepo, inventoryRepo, combatRepo)
	ragContextBuilder.SetWorldEventRepository(worldEventRepo)
	addExperienceUC := characterapp.NewAddExperienceUseCase(playerRepo, sessionRepo)
	checkAchievementsUC := achievementapp.NewCheckAchievementsUseCase(achievementRepo, playerRepo)

	// Создаем notification service для отправки уведомлений о достижениях через Telegram
	// Нужно создать bot API для notification service, но bot еще не создан
	// Поэтому создадим его позже или передадим через callback
	// Временно создадим NoOpNotificationService и заменим позже
	notificationService := &achievementapp.NoOpNotificationService{}

	// Настраиваем проверку достижений в AddExperienceUseCase
	addExperienceUC.SetCheckAchievementsUseCase(checkAchievementsUC)
	addExperienceUC.SetNotificationService(notificationService)
	checkWorldEventsUC := worldeventapp.NewCheckWorldEventsUseCase(worldEventRepo)
	useSpellUC := spellapp.NewUseSpellUseCase(spellRepo, sessionRepo, playerRepo, combatRepo)
	// Создаем use cases для ежедневных заданий (нужны для handleActionUC)
	getDailyQuestsUC := questapp.NewGetDailyQuestsUseCase(sessionRepo, dailyQuestRepo, playerRepo)
	completeDailyQuestUC := questapp.NewCompleteDailyQuestUseCase(sessionRepo, dailyQuestRepo, playerRepo, addExperienceUC)
	checkDailyProgressUC := questapp.NewCheckDailyQuestProgressUseCase(sessionRepo, dailyQuestRepo, playerRepo, completeDailyQuestUC)

	// Создаем use cases для рейтингов и лидербордов
	ratingRepo := persistence.NewRatingRepository(db)
	getLeaderboardUC := ratingapp.NewGetLeaderboardUseCase(ratingRepo)
	// Создаем адаптер для AchievementRepository для использования в rating пакете
	achievementRepoAdapter := &ratingAchievementRepoAdapter{repo: achievementRepo}
	updateRatingUC := ratingapp.NewUpdateRatingUseCase(ratingRepo, sessionRepo, playerRepo, achievementRepoAdapter)

	// Создаем адаптер для преобразования типов запросов между player_action и quest
	dailyQuestProgressAdapter := &dailyQuestProgressAdapterForPlayerAction{
		uc: checkDailyProgressUC,
	}
	// Создаем адаптер для RatingUpdater из updateRatingUC
	ratingUpdaterAdapterAction := &ratingUpdaterAdapterAction{uc: updateRatingUC}
	// Создаем анализатор действий игрока для определения необходимости проверок
	analyzePlayerActionUC := dm_analyzer.NewAnalyzePlayerActionUseCase(llm, eventRepo)

	// Создаем генератор событий локаций
	locationEventRepo := &locationEventRepoAdapter{repo: worldEventRepo}
	generateLocationEventUC := locationeventapp.NewLocationEventGenerator(locationEventRepo)

	handleActionUC := player_action.NewHandleActionUseCase(llm, sessionRepo, ragContextBuilder, eventRepo, indexDocUC, combatRepo, questRepo, inventoryRepo, addExperienceUC, checkWorldEventsUC, checkAchievementsUC, notificationService, generateImageUC, useSpellUC, responseCache, actionValidator, dailyQuestProgressAdapter, getSubscriptionUC, ratingUpdaterAdapterAction, analyzePlayerActionUC, generateLocationEventUC)
	createCharacterUC := characterapp.NewCreateCharacterUseCase(sessionRepo, playerRepo)
	getHistoryUC := history.NewGetHistoryUseCase(sessionRepo, eventRepo)
	getInventoryUC := inventoryapp.NewGetInventoryUseCase(sessionRepo, inventoryRepo)
	addItemUC := inventoryapp.NewAddItemUseCase(sessionRepo, inventoryRepo)
	handleCombatUC := combatapp.NewHandleCombatUseCase(combatRepo, sessionRepo)
	// Настраиваем проверку достижений в HandleCombatUseCase
	handleCombatUC.SetCheckAchievementsUseCase(checkAchievementsUC)
	// Настраиваем проверку ежедневных заданий в HandleCombatUseCase
	handleCombatUC.SetCheckDailyProgressUseCase(checkDailyProgressUC)
	rollDiceUC := dice.NewRollDiceUseCase()
	getQuestsUC := questapp.NewGetQuestsUseCase(sessionRepo, questRepo)
	getMapUC := mapapp.NewGetMapUseCase(sessionRepo)
	moveToLocationUC := mapapp.NewMoveToLocationUseCase(llm, sessionRepo, worldEventRepo, eventRepo, indexDocUC)
	getAchievementsUC := achievementapp.NewGetAchievementsUseCase(achievementRepo, sessionRepo)
	getSpellsUC := spellapp.NewGetSpellsUseCase(spellRepo, sessionRepo)
	performAbilityCheckUC := abilitycheck.NewPerformAbilityCheckUseCase(sessionRepo, eventRepo, indexDocUC)
	// getSubscriptionUC и checkLimitsUC уже созданы выше для использования в лимитере изображений
	// ratingRepo, getLeaderboardUC и updateRatingUC уже созданы выше для использования в handleActionUC и боте

	// Инициализация базовых достижений при старте
	logger.Info("Initializing default achievements")
	initCtx, initCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer initCancel()
	if err := achievementRepo.InitDefaultAchievements(initCtx); err != nil {
		logger.Warn("Failed to initialize default achievements",
			logger.ErrorField(err),
		)
	} else {
		logger.Info("Default achievements initialized successfully")
	}

	// Инициализация базовых заклинаний при старте
	logger.Info("Initializing default spells")
	if err := spellRepo.InitDefaultSpells(initCtx); err != nil {
		logger.Warn("Failed to initialize default spells",
			logger.ErrorField(err),
		)
	} else {
		logger.Info("Default spells initialized successfully")
	}

	// Запуск HTTP серверов ДО создания бота для доступности monitoring
	// Запуск HTTP сервера для health check
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		// Проверка подключения к БД
		sqlDB, err := db.DB()
		if err != nil {
			http.Error(w, "Database connection error", http.StatusServiceUnavailable)
			return
		}
		if err := sqlDB.Ping(); err != nil {
			http.Error(w, "Database ping error", http.StatusServiceUnavailable)
			return
		}

		// Проверка подключения к Qdrant
		if qdrantClient == nil {
			http.Error(w, "Qdrant client not initialized", http.StatusServiceUnavailable)
			return
		}
		// Реальная проверка подключения
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		_, qdrantErr := qdrantClient.ListCollections(ctx)
		if qdrantErr != nil {
			http.Error(w, "Qdrant unavailable: "+qdrantErr.Error(), http.StatusServiceUnavailable)
			return
		}

		// Проверка подключения к Telegram (если бот инициализирован)
		if bot != nil {
			healthCtx, healthCancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer healthCancel()
			if err := bot.HealthCheck(healthCtx); err != nil {
				http.Error(w, "Telegram API unavailable: "+err.Error(), http.StatusServiceUnavailable)
				return
			}
		}

		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK")
	})

	http.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "Ready")
	})

	// Запуск HTTP сервера для мониторинга LLM
	monitoringPort := getEnv("MONITORING_PORT", "8081")
	monitoringAddr := fmt.Sprintf(":%s", monitoringPort)
	monitoringServer := monitoring.NewServer(monitoringAddr, llmLogRepo)

	go func() {
		if err := monitoringServer.Start(); err != nil && err != http.ErrServerClosed {
			logger.Error("Monitoring server failed",
				logger.ErrorField(err),
				logger.String("addr", monitoringAddr),
			)
		}
	}()
	logger.Info("LLM monitoring server started",
		logger.String("addr", monitoringAddr),
		logger.String("url", fmt.Sprintf("http://localhost:%s", monitoringPort)),
	)

	http.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		ver := version.Get()
		fmt.Fprintf(w, `{"version":"%s","commit":"%s","buildTime":"%s","goVersion":"%s"}`,
			ver.Version, ver.Commit, ver.BuildTime, ver.GoVersion)
	})

	server := &http.Server{
		Addr:         ":8080",
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		logger.Info("Starting health check server",
			logger.String("addr", ":8080"),
		)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("HTTP server error",
				logger.ErrorField(err),
			)
		}
	}()

	// Инициализация бота
	logger.Info("Initializing Telegram bot")
	bot, err = telegram.NewBot(telegramToken, initCampaignUC, handleActionUC, createCharacterUC, getHistoryUC, getInventoryUC, addItemUC, handleCombatUC, rollDiceUC, getQuestsUC, getDailyQuestsUC, checkDailyProgressUC, getMapUC, moveToLocationUC, getAchievementsUC, getSpellsUC, useSpellUC, generateImageUC, getSubscriptionUC, checkLimitsUC, getLeaderboardUC, updateRatingUC, performAbilityCheckUC, sessionRepo, playerRepo, combatRepo, feedbackRepo, eventRepo, indexDocUC)
	if err != nil {
		logger.Error("Failed to create bot - continuing without Telegram bot",
			logger.ErrorField(err),
		)
		logger.Warn("Application will run in monitoring-only mode without Telegram bot functionality")
	} else {
		// После создания бота, настраиваем TelegramNotificationService в use cases
		// Используем API из bot для отправки уведомлений о достижениях
		telegramNotificationService := achievementapp.NewTelegramNotificationServiceFromBot(bot)
		addExperienceUC.SetNotificationService(telegramNotificationService)
		handleCombatUC.SetNotificationService(telegramNotificationService)

		// Настраиваем обновление рейтингов в use cases
		// Создаем адаптеры для RatingUpdater из updateRatingUC
		ratingUpdaterAdapterExp := &ratingUpdaterAdapter{uc: updateRatingUC}
		ratingUpdaterAdapterCombat := &ratingUpdaterAdapterCombat{uc: updateRatingUC}
		addExperienceUC.SetRatingUpdater(ratingUpdaterAdapterExp)
		handleCombatUC.SetRatingUpdater(ratingUpdaterAdapterCombat)
		logger.Info("Rating updater configured for use cases")

		logger.Info("Telegram notification service configured for achievements")
		logger.Info("Telegram bot initialized")
	}

	// Graceful shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Запускаем бота только если он был успешно создан
	if bot != nil {
		logger.Info("Starting bot",
			logger.String("version", version.Version),
			logger.String("commit", version.Commit),
			logger.String("buildTime", version.BuildTime),
		)

		// Запускаем бота в горутине
		botErrChan := make(chan error, 1)
		go func() {
			if err := bot.Start(ctx); err != nil {
				botErrChan <- err
			}
		}()

		// Ожидаем сигнала завершения или ошибки бота
		select {
		case <-ctx.Done():
			logger.Info("Shutting down...")
		case err := <-botErrChan:
			logger.Error("Bot error - continuing with monitoring only",
				logger.ErrorField(err),
			)
		}
	} else {
		logger.Info("Running in monitoring-only mode - waiting for shutdown signal")
		// Ожидаем сигнала завершения
		<-ctx.Done()
		logger.Info("Shutting down...")
	}

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Закрываем соединение с БД
	logger.Info("Closing database connection")
	sqlDB, err := db.DB()
	if err != nil {
		logger.Error("Failed to get underlying SQL DB",
			logger.ErrorField(err),
		)
	} else {
		if err := sqlDB.Close(); err != nil {
			logger.Error("Database connection close error",
				logger.ErrorField(err),
			)
		} else {
			logger.Info("Database connection closed successfully")
		}
	}

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server shutdown error",
			logger.ErrorField(err),
		)
	}

	logger.Info("Shutdown complete")
}

// ratingAchievementRepoAdapter адаптирует persistence.AchievementRepository к rating.AchievementRepository
type ratingAchievementRepoAdapter struct {
	repo *persistence.AchievementRepository
}

func (a *ratingAchievementRepoAdapter) GetAchievementProgress(ctx context.Context, playerID uint, achievementID uint) (*ratingapp.AchievementProgress, error) {
	progress, err := a.repo.GetAchievementProgress(ctx, playerID, achievementID)
	if err != nil {
		return nil, err
	}
	if progress == nil {
		return nil, nil
	}
	return &ratingapp.AchievementProgress{
		PlayerID:      progress.PlayerID,
		AchievementID: progress.AchievementID,
		CurrentValue:  progress.CurrentValue,
	}, nil
}

func (a *ratingAchievementRepoAdapter) GetAchievementProgressByRequirementKey(ctx context.Context, playerID uint, requirementKey string) (int, error) {
	return a.repo.GetAchievementProgressByRequirementKey(ctx, playerID, requirementKey)
}

// ratingUpdaterAdapter адаптирует ratingapp.UpdateRatingUseCase к интерфейсу RatingUpdater
type ratingUpdaterAdapter struct {
	uc *ratingapp.UpdateRatingUseCase
}

func (a *ratingUpdaterAdapter) Execute(ctx context.Context, req characterapp.RatingUpdateRequest) error {
	updateReq := ratingapp.UpdateRatingRequest{
		TgUserID: req.TgUserID,
		ChatID:   req.ChatID,
	}
	return a.uc.Execute(ctx, updateReq)
}

// ratingUpdaterAdapterCombat адаптирует ratingapp.UpdateRatingUseCase к интерфейсу combatapp.RatingUpdater
type ratingUpdaterAdapterCombat struct {
	uc *ratingapp.UpdateRatingUseCase
}

func (a *ratingUpdaterAdapterCombat) Execute(ctx context.Context, req combatapp.RatingUpdateRequest) error {
	updateReq := ratingapp.UpdateRatingRequest{
		TgUserID: req.TgUserID,
		ChatID:   req.ChatID,
	}
	return a.uc.Execute(ctx, updateReq)
}

// ratingUpdaterAdapterAction адаптирует ratingapp.UpdateRatingUseCase к интерфейсу player_action.RatingUpdater
type ratingUpdaterAdapterAction struct {
	uc *ratingapp.UpdateRatingUseCase
}

func (a *ratingUpdaterAdapterAction) Execute(ctx context.Context, req player_action.RatingUpdateRequest) error {
	updateReq := ratingapp.UpdateRatingRequest{
		TgUserID: req.TgUserID,
		ChatID:   req.ChatID,
	}
	return a.uc.Execute(ctx, updateReq)
}

func initDB(dsn string) (*gorm.DB, error) {
	dbLogger := gormLogger.New(
		log.New(os.Stdout, "", log.LstdFlags),
		gormLogger.Config{
			SlowThreshold:             2 * time.Second,
			LogLevel:                  gormLogger.Warn,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		},
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: dbLogger,
	})
	if err != nil {
		return nil, err
	}
	return db, nil
}

func runMigrations(db *gorm.DB) error {
	return db.AutoMigrate(
		&session.GameSession{},
		&world.World{},
		&world.Location{},
		&world.LocationConnection{},
		&world.NPC{},
		&world.Monster{},
		&world.WorldEvent{},
		&player.Player{},
		&character.Character{},
		&character.Stats{},
		&event.StoryEvent{},
		&inventory.Inventory{},
		&inventory.InventoryItem{},
		&llmlogdomain.LLMLog{},
		&combat.Combat{},
		&combat.CombatParticipant{},
		&item.Item{},
		&quest.Quest{},
		&quest.DailyQuest{},
		&quest.DailyQuestProgress{},
		&quest.DailyQuestStreak{},
		&feedback.Feedback{},
		&achievement.Achievement{},
		&rating.PlayerRating{},
		&achievement.PlayerAchievement{},
		&achievement.AchievementProgress{},
		&spell.Spell{},
		&spell.CharacterSpell{},
		&subscription.Subscription{},
	)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

type redactingWriter struct {
	w       io.Writer
	reToken *regexp.Regexp
}

func newRedactingWriter(w io.Writer) *redactingWriter {
	// Telegram bot token формат обычно: <digits>:<base64url-ish>
	// Пример из ошибок http: api.telegram.org/bot8553...:AA.../getUpdates
	reToken := regexp.MustCompile(`bot[0-9]{6,}:[A-Za-z0-9_-]{10,}`)
	return &redactingWriter{
		w:       w,
		reToken: reToken,
	}
}

func (rw *redactingWriter) Write(p []byte) (n int, err error) {
	if rw == nil || rw.w == nil {
		return 0, nil
	}
	s := string(p)
	s = rw.reToken.ReplaceAllString(s, "bot***")
	_, err = rw.w.Write([]byte(s))
	if err != nil {
		return 0, err
	}
	// Возвращаем len(p) (а не len(s)), чтобы вызывающие не считали это ошибкой записи.
	return len(p), nil
}

// maskDSN маскирует чувствительные данные в DSN для логирования
func maskDSN(dsn string) string {
	// Простая маскировка - скрываем пароль
	// В production лучше использовать более сложную логику
	if len(dsn) > 20 {
		return dsn[:20] + "***"
	}
	return "***"
}

// maskClientID маскирует ClientID для логирования
func maskClientID(clientID string) string {
	if len(clientID) == 0 {
		return ""
	}
	if len(clientID) <= 8 {
		return "***"
	}
	// Показываем первые 4 и последние 4 символа
	return clientID[:4] + "***" + clientID[len(clientID)-4:]
}
