package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	achievementapp "dungeons-and-dragons-ai/internal/game/application/achievement"
	"dungeons-and-dragons-ai/internal/game/application/campaign"
	characterapp "dungeons-and-dragons-ai/internal/game/application/character"
	combatapp "dungeons-and-dragons-ai/internal/game/application/combat"
	"dungeons-and-dragons-ai/internal/game/application/dice"
	imageapp "dungeons-and-dragons-ai/internal/game/application/image"
	"dungeons-and-dragons-ai/internal/game/application/history"
	inventoryapp "dungeons-and-dragons-ai/internal/game/application/inventory"
	"dungeons-and-dragons-ai/internal/game/application/player_action"
	questapp "dungeons-and-dragons-ai/internal/game/application/quest"
	spellapp "dungeons-and-dragons-ai/internal/game/application/spell"
	subscriptionapp "dungeons-and-dragons-ai/internal/game/application/subscription"
	worldeventapp "dungeons-and-dragons-ai/internal/game/application/world_event"
	mapapp 	"dungeons-and-dragons-ai/internal/game/application/worldmap"
	dmcache "dungeons-and-dragons-ai/internal/game/infrastructure/cache"
	"dungeons-and-dragons-ai/internal/game/domain/achievement"
	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/combat"
	"dungeons-and-dragons-ai/internal/game/domain/spell"
	"dungeons-and-dragons-ai/internal/game/domain/event"
	"dungeons-and-dragons-ai/internal/game/domain/feedback"
	"dungeons-and-dragons-ai/internal/game/domain/inventory"
	"dungeons-and-dragons-ai/internal/game/domain/item"
	"dungeons-and-dragons-ai/internal/game/domain/player"
	"dungeons-and-dragons-ai/internal/game/domain/quest"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/game/domain/world"
	contextbuilder "dungeons-and-dragons-ai/internal/game/infrastructure/context"
	"dungeons-and-dragons-ai/internal/game/infrastructure/persistence"
	llminfrastructure "dungeons-and-dragons-ai/internal/llm/infrastructure"
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
)

func main() {
	// Инициализация логгера (должна быть первой)
	if err := logger.InitFromEnv(); err != nil {
		// Fallback на стандартный логгер если не удалось инициализировать
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

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
		AuthBaseURL:  getEnv("GIGACHAT_AUTH_URL", "https://ngw.devices.sberbank.ru:9443"),
		APIBaseURL:   getEnv("GIGACHAT_API_URL", "https://gigachat.devices.sberbank.ru/api/v1"),
		ClientID:     gigachatClientID,
		ClientSecret: gigachatClientSecret,
		Scope:        getEnv("GIGACHAT_SCOPE", "GIGACHAT_API_PERS"),
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
	llm := llminfrastructure.NewGigachatLLM(gigachatClient, gigachatModel)
	
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
	worldEventRepo := persistence.NewWorldEventRepository(db)
	feedbackRepo := persistence.NewFeedbackRepository(db)
	achievementRepo := persistence.NewAchievementRepository(db)
	spellRepo := persistence.NewSpellRepository(db)
	subscriptionRepo := persistence.NewSubscriptionRepository(db)

	// Создаем use cases для подписок (нужно для лимитера изображений)
	getSubscriptionUC := subscriptionapp.NewGetSubscriptionUseCase(subscriptionRepo)
	checkLimitsUC := subscriptionapp.NewCheckLimitsUseCase(subscriptionRepo, sessionRepo, fallbackLimiter)

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
	useSpellUC := spellapp.NewUseSpellUseCase(spellRepo, sessionRepo, playerRepo)
	handleActionUC := player_action.NewHandleActionUseCase(llm, sessionRepo, ragContextBuilder, eventRepo, indexDocUC, combatRepo, questRepo, inventoryRepo, addExperienceUC, checkWorldEventsUC, checkAchievementsUC, notificationService, generateImageUC, useSpellUC, responseCache, actionValidator)
	createCharacterUC := characterapp.NewCreateCharacterUseCase(sessionRepo, playerRepo)
	getHistoryUC := history.NewGetHistoryUseCase(sessionRepo, eventRepo)
	getInventoryUC := inventoryapp.NewGetInventoryUseCase(sessionRepo, inventoryRepo)
	addItemUC := inventoryapp.NewAddItemUseCase(sessionRepo, inventoryRepo)
	handleCombatUC := combatapp.NewHandleCombatUseCase(combatRepo, sessionRepo)
	// Настраиваем проверку достижений в HandleCombatUseCase
	handleCombatUC.SetCheckAchievementsUseCase(checkAchievementsUC)
	rollDiceUC := dice.NewRollDiceUseCase()
	getQuestsUC := questapp.NewGetQuestsUseCase(sessionRepo, questRepo)
	getMapUC := mapapp.NewGetMapUseCase(sessionRepo)
	getAchievementsUC := achievementapp.NewGetAchievementsUseCase(achievementRepo, sessionRepo)
	getSpellsUC := spellapp.NewGetSpellsUseCase(spellRepo, sessionRepo)
	// getSubscriptionUC и checkLimitsUC уже созданы выше для использования в лимитере изображений

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

	// Инициализация бота
	logger.Info("Initializing Telegram bot")
	bot, err := telegram.NewBot(telegramToken, initCampaignUC, handleActionUC, createCharacterUC, getHistoryUC, getInventoryUC, addItemUC, handleCombatUC, rollDiceUC, getQuestsUC, getMapUC, getAchievementsUC, getSpellsUC, useSpellUC, generateImageUC, getSubscriptionUC, checkLimitsUC, sessionRepo, combatRepo, feedbackRepo)
	if err != nil {
		logger.Fatal("Failed to create bot",
			logger.ErrorField(err),
		)
	}
	
	// После создания бота, настраиваем TelegramNotificationService в use cases
	// Используем API из bot для отправки уведомлений о достижениях
	telegramNotificationService := achievementapp.NewTelegramNotificationServiceFromBot(bot)
	addExperienceUC.SetNotificationService(telegramNotificationService)
	handleCombatUC.SetNotificationService(telegramNotificationService)
	logger.Info("Telegram notification service configured for achievements")
	logger.Info("Telegram bot initialized")

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

		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK")
	})

	http.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "Ready")
	})

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

	// Graceful shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

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

	// Ожидаем сигнала завершения или ошибки
	select {
	case <-ctx.Done():
		logger.Info("Shutting down...")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("HTTP server shutdown error",
				logger.ErrorField(err),
			)
		}

		logger.Info("Shutdown complete")
	case err := <-botErrChan:
		logger.Fatal("Bot error",
			logger.ErrorField(err),
		)
	}
}

func initDB(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
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
		&combat.Combat{},
		&combat.CombatParticipant{},
		&item.Item{},
		&quest.Quest{},
		&feedback.Feedback{},
		&achievement.Achievement{},
		&achievement.PlayerAchievement{},
		&achievement.AchievementProgress{},
		&spell.Spell{},
		&spell.CharacterSpell{},
	)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
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
