package integration

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
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
	ratingapp "dungeons-and-dragons-ai/internal/game/application/rating"
	spellapp "dungeons-and-dragons-ai/internal/game/application/spell"
	subscriptionapp "dungeons-and-dragons-ai/internal/game/application/subscription"
	worldeventapp "dungeons-and-dragons-ai/internal/game/application/world_event"
	mapapp "dungeons-and-dragons-ai/internal/game/application/worldmap"
	dmcache "dungeons-and-dragons-ai/internal/game/infrastructure/cache"
	contextbuilder "dungeons-and-dragons-ai/internal/game/infrastructure/context"
	"dungeons-and-dragons-ai/internal/game/infrastructure/persistence"
	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/rating"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	llminfrastructure "dungeons-and-dragons-ai/internal/llm/infrastructure"
	ragapp "dungeons-and-dragons-ai/internal/rag/application"
	ragembeddings "dungeons-and-dragons-ai/internal/rag/infrastructure/embeddings"
	ragvectorstore "dungeons-and-dragons-ai/internal/rag/infrastructure/vectorstore"
	"dungeons-and-dragons-ai/pkg/gigachat"
	"dungeons-and-dragons-ai/pkg/logger"

	"github.com/qdrant/go-client/qdrant"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// testConfig содержит конфигурацию для интеграционных тестов
type testConfig struct {
	db                    *gorm.DB
	qdrantClient          *qdrant.Client
	chatID                int64
	tgUserID              int64
	ctx                   context.Context
	initCampaignUC        *campaign.InitCampaignUseCase
	handleActionUC        *player_action.HandleActionUseCase
	createCharacterUC     *characterapp.CreateCharacterUseCase
	getHistoryUC          *history.GetHistoryUseCase
	getInventoryUC        *inventoryapp.GetInventoryUseCase
	addItemUC             *inventoryapp.AddItemUseCase
	handleCombatUC        *combatapp.HandleCombatUseCase
	rollDiceUC            *dice.RollDiceUseCase
	getQuestsUC           *questapp.GetQuestsUseCase
	getDailyQuestsUC      *questapp.GetDailyQuestsUseCase
	checkDailyProgressUC  *questapp.CheckDailyQuestProgressUseCase
	getMapUC              *mapapp.GetMapUseCase
	getAchievementsUC      *achievementapp.GetAchievementsUseCase
	getSpellsUC           *spellapp.GetSpellsUseCase
	useSpellUC            *spellapp.UseSpellUseCase
	getLeaderboardUC      *ratingapp.GetLeaderboardUseCase
	updateRatingUC        *ratingapp.UpdateRatingUseCase
	addExperienceUC       *characterapp.AddExperienceUseCase
	sessionRepo           session.Repository
}

// setupIntegrationTest настраивает окружение для интеграционных тестов
func setupIntegrationTest(t *testing.T) *testConfig {
	ctx := context.Background()

	// Инициализация логгера
	if err := logger.InitFromEnv(); err != nil {
		t.Logf("Не удалось инициализировать логгер: %v", err)
	}

	// Подключение к БД
	// Для локального запуска используем localhost, для Docker - значение из .env
	dbDSN := getEnv("DATABASE_URL", "postgres://dnd_user:dnd_password@localhost:5432/dnd?sslmode=disable")
	// Если в DATABASE_URL указан postgres (Docker hostname), заменяем на localhost для локального запуска
	if strings.Contains(dbDSN, "@postgres:") {
		dbDSN = strings.Replace(dbDSN, "@postgres:", "@localhost:", 1)
	}
	db, err := gorm.Open(postgres.Open(dbDSN), &gorm.Config{})
	if err != nil {
		t.Fatalf("Не удалось подключиться к БД: %v", err)
	}

	// Проверка подключения
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("Не удалось получить sql.DB: %v", err)
	}
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("Не удалось выполнить ping к БД: %v", err)
	}

	// Подключение к Qdrant
	// Для локального запуска используем localhost, для Docker - значение из .env
	// Qdrant клиент использует gRPC, поэтому нужен порт для gRPC (6335 для локального подключения)
	qdrantHost := getEnv("QDRANT_HOST", "localhost")
	if qdrantHost == "qdrant" {
		qdrantHost = "localhost"
	}
	// Используем QDRANT_GRPC_PORT для gRPC подключения, по умолчанию 6335 (локальный порт для gRPC)
	qdrantGrpcPort := getEnv("QDRANT_GRPC_PORT", "6335")
	if qdrantGrpcPort == "" {
		qdrantGrpcPort = "6335"
	}
	qdrantClient, err := qdrant.NewClient(&qdrant.Config{
		Host: qdrantHost,
		Port: parsePort(qdrantGrpcPort),
	})
	if err != nil {
		t.Fatalf("Не удалось подключиться к Qdrant: %v", err)
	}

	// Проверка подключения к Qdrant
	_, err = qdrantClient.ListCollections(ctx)
	if err != nil {
		t.Fatalf("Не удалось проверить подключение к Qdrant: %v", err)
	}

	// Проверяем, что контейнеры доступны (после подключения)
	if !isContainersRunning(t) {
		t.Skip("Контейнеры не запущены или недоступны. Запустите: make docker-up")
	}

	// Инициализация GigaChat (для реальных тестов нужны реальные credentials)
	gigachatClientID := getEnv("GIGACHAT_CLIENT_ID", "")
	gigachatClientSecret := getEnv("GIGACHAT_CLIENT_SECRET", "")
	if gigachatClientID == "" || gigachatClientSecret == "" {
		t.Skip("GIGACHAT_CLIENT_ID и GIGACHAT_CLIENT_SECRET не установлены. Пропускаем тесты, требующие LLM.")
	}

	skipTLSVerify := getEnv("GIGACHAT_SKIP_TLS_VERIFY", "false") == "true"
	gigachatCfg := gigachat.Config{
		AuthBaseURL:   getEnv("GIGACHAT_AUTH_URL", "https://ngw.devices.sberbank.ru:9443"),
		APIBaseURL:    getEnv("GIGACHAT_API_URL", "https://gigachat.devices.sberbank.ru/api/v1"),
		ClientID:      gigachatClientID,
		ClientSecret:  gigachatClientSecret,
		Scope:         getEnv("GIGACHAT_SCOPE", "GIGACHAT_API_PERS"),
		SkipTLSVerify: skipTLSVerify,
	}
	gigachatClient := gigachat.NewClient(gigachatCfg)
	gigachatModel := getEnv("GIGACHAT_MODEL", "GigaChat")

	// Инициализация LLM
	llm := llminfrastructure.NewGigachatLLM(gigachatClient, gigachatModel)
	imageGenerator := llminfrastructure.NewGigachatImageGenerator(gigachatClient, gigachatModel)

	// Инициализация ImageStorage
	imageStoragePath := getEnv("IMAGE_STORAGE_PATH", "./test_images")
	imageStorage, err := imageapp.NewLocalImageStorage(imageStoragePath)
	if err != nil {
		t.Fatalf("Не удалось создать image storage: %v", err)
	}
	generateImageUC := imageapp.NewImageGenerationUseCase(imageGenerator, imageStorage)
	dailyLimit := 5
	fallbackLimiter := imageapp.NewInMemoryRateLimiter(dailyLimit)
	generateImageUC.SetLimiter(fallbackLimiter)

	// Инициализация RAG
	embedder := ragembeddings.NewGigachatEmbedder(gigachatClient)
	vectorStore := ragvectorstore.NewQdrantStore(qdrantClient)
	if err := vectorStore.EnsureCollection(ctx); err != nil {
		t.Fatalf("Не удалось создать коллекцию Qdrant: %v", err)
	}
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
	achievementRepo := persistence.NewAchievementRepository(db)
	spellRepo := persistence.NewSpellRepository(db)
	subscriptionRepo := persistence.NewSubscriptionRepository(db)
	ratingRepo := persistence.NewRatingRepository(db)

	// Инициализация use cases
	checkLimitsUC := subscriptionapp.NewCheckLimitsUseCase(subscriptionRepo, sessionRepo, sessionRepo, eventRepo, fallbackLimiter)
	subscriptionImageLimiter := subscriptionapp.NewSubscriptionImageLimiter(checkLimitsUC, fallbackLimiter)
	generateImageUC.SetLimiter(subscriptionImageLimiter)

	responseCache := dmcache.NewDMResponseCache(1 * time.Hour)
	actionValidator := player_action.NewActionValidator()

	initCampaignUC := campaign.NewInitCampaignUseCase(llm, worldRepo)
	simpleContextBuilder := contextbuilder.NewSimpleContextBuilder()
	ragContextBuilder := contextbuilder.NewRAGContextBuilder(simpleContextBuilder, retrieveContextUC, eventRepo, inventoryRepo, combatRepo)
	addExperienceUC := characterapp.NewAddExperienceUseCase(playerRepo, sessionRepo)
	checkAchievementsUC := achievementapp.NewCheckAchievementsUseCase(achievementRepo, playerRepo)
	notificationService := &achievementapp.NoOpNotificationService{}
	addExperienceUC.SetCheckAchievementsUseCase(checkAchievementsUC)
	addExperienceUC.SetNotificationService(notificationService)
	checkWorldEventsUC := worldeventapp.NewCheckWorldEventsUseCase(worldEventRepo)
	useSpellUC := spellapp.NewUseSpellUseCase(spellRepo, sessionRepo, playerRepo, combatRepo)
	getDailyQuestsUC := questapp.NewGetDailyQuestsUseCase(sessionRepo, dailyQuestRepo, playerRepo)
	completeDailyQuestUC := questapp.NewCompleteDailyQuestUseCase(sessionRepo, dailyQuestRepo, playerRepo, addExperienceUC)
	checkDailyProgressUC := questapp.NewCheckDailyQuestProgressUseCase(sessionRepo, dailyQuestRepo, playerRepo, completeDailyQuestUC)

	// Инициализация рейтингов и лидербордов
	getLeaderboardUC := ratingapp.NewGetLeaderboardUseCase(ratingRepo)
	// Создаем адаптер для AchievementRepository для использования в rating пакете
	achievementRepoAdapter := &ratingAchievementRepoAdapter{repo: achievementRepo}
	updateRatingUC := ratingapp.NewUpdateRatingUseCase(ratingRepo, sessionRepo, playerRepo, achievementRepoAdapter)
	
	// Создаем getSubscriptionUC для handleActionUC
	getSubscriptionUC := subscriptionapp.NewGetSubscriptionUseCase(subscriptionRepo)

	// Адаптер для daily quest progress (как в main.go)
	dailyQuestProgressAdapter := &dailyQuestProgressAdapterForPlayerAction{uc: checkDailyProgressUC}
	
	// Адаптер для RatingUpdater из updateRatingUC для player_action
	ratingUpdaterAdapterForPlayerAction := &ratingUpdaterAdapterForPlayerAction{uc: updateRatingUC}
	
	// Настраиваем обновление рейтингов при получении опыта
	ratingUpdaterAdapterForCharacter := &ratingUpdaterAdapter{uc: updateRatingUC}
	addExperienceUC.SetRatingUpdater(ratingUpdaterAdapterForCharacter)
	
	handleActionUC := player_action.NewHandleActionUseCase(
		llm, sessionRepo, ragContextBuilder, eventRepo, indexDocUC, combatRepo, questRepo,
		inventoryRepo, addExperienceUC, checkWorldEventsUC, checkAchievementsUC,
		notificationService, generateImageUC, useSpellUC, responseCache, actionValidator,
		dailyQuestProgressAdapter, getSubscriptionUC, ratingUpdaterAdapterForPlayerAction,
	)
	createCharacterUC := characterapp.NewCreateCharacterUseCase(sessionRepo, playerRepo)
	getHistoryUC := history.NewGetHistoryUseCase(sessionRepo, eventRepo)
	getInventoryUC := inventoryapp.NewGetInventoryUseCase(sessionRepo, inventoryRepo)
	addItemUC := inventoryapp.NewAddItemUseCase(sessionRepo, inventoryRepo)
	handleCombatUC := combatapp.NewHandleCombatUseCase(combatRepo, sessionRepo)
	handleCombatUC.SetCheckAchievementsUseCase(checkAchievementsUC)
	handleCombatUC.SetCheckDailyProgressUseCase(checkDailyProgressUC)
	rollDiceUC := dice.NewRollDiceUseCase()
	getQuestsUC := questapp.NewGetQuestsUseCase(sessionRepo, questRepo)
	getMapUC := mapapp.NewGetMapUseCase(sessionRepo)
	getAchievementsUC := achievementapp.NewGetAchievementsUseCase(achievementRepo, sessionRepo)
	getSpellsUC := spellapp.NewGetSpellsUseCase(spellRepo, sessionRepo)

	// Инициализация базовых данных
	initCtx, initCancel := context.WithTimeout(ctx, 30*time.Second)
	defer initCancel()
	if err := achievementRepo.InitDefaultAchievements(initCtx); err != nil {
		t.Logf("Не удалось инициализировать достижения: %v", err)
	}
	if err := spellRepo.InitDefaultSpells(initCtx); err != nil {
		t.Logf("Не удалось инициализировать заклинания: %v", err)
	}

	// Тестовые ID
	chatID := int64(123456789)
	tgUserID := int64(123456789)

	return &testConfig{
		db:                   db,
		qdrantClient:         qdrantClient,
		chatID:               chatID,
		tgUserID:             tgUserID,
		ctx:                  ctx,
		initCampaignUC:       initCampaignUC,
		handleActionUC:       handleActionUC,
		createCharacterUC:    createCharacterUC,
		getHistoryUC:         getHistoryUC,
		getInventoryUC:       getInventoryUC,
		addItemUC:            addItemUC,
		handleCombatUC:       handleCombatUC,
		rollDiceUC:           rollDiceUC,
		getQuestsUC:          getQuestsUC,
		getDailyQuestsUC:     getDailyQuestsUC,
		checkDailyProgressUC: checkDailyProgressUC,
		getMapUC:             getMapUC,
		getAchievementsUC:    getAchievementsUC,
		getSpellsUC:          getSpellsUC,
		useSpellUC:           useSpellUC,
		getLeaderboardUC:     getLeaderboardUC,
		updateRatingUC:       updateRatingUC,
		addExperienceUC:    addExperienceUC,
		sessionRepo:          sessionRepo,
	}
}

// ratingAchievementRepoAdapter адаптирует achievementRepo к интерфейсу rating.AchievementRepository
type ratingAchievementRepoAdapter struct {
	repo *persistence.AchievementRepository
}

func (a *ratingAchievementRepoAdapter) GetAchievementProgress(ctx context.Context, playerID uint, achievementID uint) (*ratingapp.AchievementProgress, error) {
	progress, err := a.repo.GetAchievementProgress(ctx, playerID, achievementID)
	if err != nil {
		return nil, err
	}
	return &ratingapp.AchievementProgress{
		PlayerID:     progress.PlayerID,
		AchievementID: progress.AchievementID,
		CurrentValue: progress.CurrentValue,
	}, nil
}

func (a *ratingAchievementRepoAdapter) GetAchievementProgressByRequirementKey(ctx context.Context, playerID uint, requirementKey string) (int, error) {
	return a.repo.GetAchievementProgressByRequirementKey(ctx, playerID, requirementKey)
}

// ratingUpdaterAdapter адаптирует updateRatingUC к интерфейсу characterapp.RatingUpdater
type ratingUpdaterAdapter struct {
	uc *ratingapp.UpdateRatingUseCase
}

func (a *ratingUpdaterAdapter) Execute(ctx context.Context, req characterapp.RatingUpdateRequest) error {
	ratingReq := ratingapp.UpdateRatingRequest{
		TgUserID: req.TgUserID,
		ChatID:   req.ChatID,
	}
	return a.uc.Execute(ctx, ratingReq)
}

// ratingUpdaterAdapterForPlayerAction адаптирует updateRatingUC к интерфейсу player_action.RatingUpdater
type ratingUpdaterAdapterForPlayerAction struct {
	uc *ratingapp.UpdateRatingUseCase
}

func (a *ratingUpdaterAdapterForPlayerAction) Execute(ctx context.Context, req player_action.RatingUpdateRequest) error {
	ratingReq := ratingapp.UpdateRatingRequest{
		TgUserID: req.TgUserID,
		ChatID:   req.ChatID,
	}
	return a.uc.Execute(ctx, ratingReq)
}

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

// cleanupTest очищает данные после теста
func cleanupTest(t *testing.T, cfg *testConfig) {
	// Удаляем тестовую сессию в правильном порядке (сначала дочерние записи, потом родительские)
	if cfg.db != nil {
		// Получаем ID сессии для удаления связанных записей
		var sessionID uint
		cfg.db.Raw("SELECT id FROM game_sessions WHERE chat_id = ?", cfg.chatID).Scan(&sessionID)
		
		if sessionID > 0 {
			// Удаляем дочерние записи в правильном порядке (сначала самые глубокие)
			// 1. Удаляем участников боя перед боями
			cfg.db.Exec("DELETE FROM combat_participants WHERE combat_id IN (SELECT id FROM combats WHERE game_session_id = ?)", sessionID)
			// 2. Удаляем бои
			cfg.db.Exec("DELETE FROM combats WHERE game_session_id = ?", sessionID)
			// 3. Удаляем предметы из инвентаря перед инвентарями
			cfg.db.Exec("DELETE FROM inventory_items WHERE inventory_id IN (SELECT id FROM inventories WHERE character_id IN (SELECT character_id FROM players WHERE game_session_id = ?))", sessionID)
			// 4. Удаляем инвентари
			cfg.db.Exec("DELETE FROM inventories WHERE character_id IN (SELECT character_id FROM players WHERE game_session_id = ?)", sessionID)
			// 5. Удаляем события
			cfg.db.Exec("DELETE FROM story_events WHERE game_session_id = ?", sessionID)
			// 6. Удаляем игроков
			cfg.db.Exec("DELETE FROM players WHERE game_session_id = ?", sessionID)
			// 7. Удаляем персонажей
			cfg.db.Exec("DELETE FROM characters WHERE id IN (SELECT character_id FROM players WHERE tg_user_id = ?)", cfg.tgUserID)
			// 8. Удаляем сессию в последнюю очередь
			cfg.db.Exec("DELETE FROM game_sessions WHERE id = ?", sessionID)
		}
		
		// Также удаляем по tg_user_id на случай, если что-то осталось
		cfg.db.Exec("DELETE FROM players WHERE tg_user_id = ?", cfg.tgUserID)
		cfg.db.Exec("DELETE FROM characters WHERE id IN (SELECT character_id FROM players WHERE tg_user_id = ?)", cfg.tgUserID)
	}
}

// TestGameFlow_CompleteScenario тестирует полный сценарий игры
func TestGameFlow_CompleteScenario(t *testing.T) {
	cfg := setupIntegrationTest(t)
	defer cleanupTest(t, cfg)

	ctx := cfg.ctx
	chatID := cfg.chatID
	tgUserID := cfg.tgUserID

	t.Run("1. Создание новой игры", func(t *testing.T) {
		world, err := cfg.initCampaignUC.Execute(ctx, "классическое фэнтези")
		if err != nil {
			t.Fatalf("Не удалось создать игру: %v", err)
		}
		if world == nil {
			t.Fatal("Мир не создан")
		}
		if world.ID == 0 {
			t.Fatal("ID мира не установлен")
		}

		// Создаем сессию
		gs := &session.GameSession{
			ChatID:  chatID,
			State:   session.StateActive,
			World:   *world,
			WorldID: world.ID,
		}
		if err := cfg.sessionRepo.Save(ctx, gs); err != nil {
			t.Fatalf("Не удалось сохранить сессию: %v", err)
		}

		// Проверяем, что сессия создана
		gs, err = cfg.sessionRepo.GetByChatID(ctx, chatID)
		if err != nil {
			t.Fatalf("Не удалось получить сессию: %v", err)
		}
		if gs == nil {
			t.Fatal("Сессия не создана")
		}
		if !gs.IsActive() {
			t.Fatal("Сессия не активна")
		}
		if gs.WorldID == 0 {
			t.Fatal("Мир не создан")
		}
	})

	t.Run("2. Создание персонажа", func(t *testing.T) {
		req := characterapp.CreateCharacterRequest{
			ChatID: chatID,
			Name:   "ТестовыйГерой",
			Race:   character.RaceElf,
			Class:  character.ClassWizard,
		}

		player, err := cfg.createCharacterUC.Execute(ctx, req)
		if err != nil {
			t.Fatalf("Не удалось создать персонажа: %v", err)
		}
		if player == nil {
			t.Fatal("Игрок не создан")
		}
		if player.Character.Name != "ТестовыйГерой" {
			t.Fatalf("Неверное имя персонажа: ожидалось 'ТестовыйГерой', получено '%s'", player.Character.Name)
		}
		if player.Character.Race != character.RaceElf {
			t.Fatalf("Неверная раса: ожидалось 'elf', получено '%s'", player.Character.Race)
		}
		if player.Character.Class != character.ClassWizard {
			t.Fatalf("Неверный класс: ожидалось 'wizard', получено '%s'", player.Character.Class)
		}
	})

	t.Run("3. Игровое действие - исследование", func(t *testing.T) {
		// Отправляем игровое действие
		response, err := cfg.handleActionUC.Execute(ctx, chatID, "Осматриваю комнату")
		if err != nil {
			t.Fatalf("Не удалось обработать действие игрока: %v", err)
		}
		if response == "" {
			t.Fatal("Ответ DM пуст")
		}
		t.Logf("Ответ DM: %s", response)
	})

	t.Run("4. Просмотр инвентаря", func(t *testing.T) {
		inventoryText, err := cfg.getInventoryUC.Execute(ctx, chatID, tgUserID)
		if err != nil {
			t.Fatalf("Не удалось получить инвентарь: %v", err)
		}
		if inventoryText == "" {
			t.Fatal("Текст инвентаря пуст")
		}
		t.Logf("Инвентарь: %s", inventoryText)
	})

	t.Run("5. Подбор предмета", func(t *testing.T) {
		req := inventoryapp.AddItemRequest{
			ChatID:   chatID,
			TgUserID: tgUserID,
			ItemName: "меч",
			Quantity: 1,
		}

		result, err := cfg.addItemUC.Execute(ctx, req)
		if err != nil {
			t.Logf("Не удалось подобрать предмет (может быть не в контексте): %v", err)
		} else {
			t.Logf("Результат подбора предмета: %s", result)
		}
	})

	t.Run("6. Бросок кубика", func(t *testing.T) {
		result, err := cfg.rollDiceUC.Execute(ctx, "d20")
		if err != nil {
			t.Fatalf("Не удалось бросить кубик: %v", err)
		}
		if result == "" {
			t.Fatal("Результат броска пуст")
		}
		t.Logf("Результат броска: %s", result)
	})

	t.Run("7. Просмотр квестов", func(t *testing.T) {
		questsText, err := cfg.getQuestsUC.Execute(ctx, chatID)
		if err != nil {
			t.Fatalf("Не удалось получить квесты: %v", err)
		}
		if questsText == "" {
			t.Fatal("Текст квестов пуст")
		}
		t.Logf("Квесты: %s", questsText)
	})

	t.Run("8. Просмотр ежедневных заданий", func(t *testing.T) {
		dailyText, err := cfg.getDailyQuestsUC.Execute(ctx, chatID, tgUserID)
		if err != nil {
			t.Fatalf("Не удалось получить ежедневные задания: %v", err)
		}
		if dailyText == "" {
			t.Fatal("Текст ежедневных заданий пуст")
		}
		t.Logf("Ежедневные задания: %s", dailyText)
	})

	t.Run("9. Просмотр карты", func(t *testing.T) {
		mapText, err := cfg.getMapUC.Execute(ctx, chatID)
		if err != nil {
			t.Fatalf("Не удалось получить карту: %v", err)
		}
		if mapText == "" {
			t.Fatal("Текст карты пуст")
		}
		t.Logf("Карта: %s", mapText)
	})

	t.Run("10. Просмотр истории", func(t *testing.T) {
		historyText, err := cfg.getHistoryUC.Execute(ctx, chatID, 10)
		if err != nil {
			t.Fatalf("Не удалось получить историю: %v", err)
		}
		if historyText == "" {
			t.Fatal("Текст истории пуст")
		}
		t.Logf("История: %s", historyText)
	})

	t.Run("11. Просмотр достижений", func(t *testing.T) {
		req := achievementapp.GetAchievementsRequest{
			ChatID:  chatID,
			TgUserID: tgUserID,
		}
		achievementsText, err := cfg.getAchievementsUC.Execute(ctx, req)
		if err != nil {
			t.Fatalf("Не удалось получить достижения: %v", err)
		}
		if achievementsText == "" {
			t.Fatal("Текст достижений пуст")
		}
		t.Logf("Достижения: %s", achievementsText)
	})

	t.Run("12. Просмотр заклинаний", func(t *testing.T) {
		req := spellapp.GetSpellsRequest{
			ChatID:  chatID,
			TgUserID: tgUserID,
		}
		spellsText, err := cfg.getSpellsUC.Execute(ctx, req)
		if err != nil {
			t.Fatalf("Не удалось получить заклинания: %v", err)
		}
		if spellsText == "" {
			t.Fatal("Текст заклинаний пуст")
		}
		t.Logf("Заклинания: %s", spellsText)
	})

	t.Run("13. Завершение игры", func(t *testing.T) {
		gs, err := cfg.sessionRepo.GetByChatID(ctx, chatID)
		if err != nil {
			t.Fatalf("Не удалось получить сессию: %v", err)
		}
		if gs == nil {
			t.Fatal("Сессия не найдена")
		}

		gs.End()
		if err := cfg.sessionRepo.Save(ctx, gs); err != nil {
			t.Fatalf("Не удалось сохранить сессию: %v", err)
		}

		// Проверяем, что сессия завершена
		gs, err = cfg.sessionRepo.GetByChatID(ctx, chatID)
		if err != nil {
			t.Fatalf("Не удалось получить сессию: %v", err)
		}
		if gs.IsActive() {
			t.Fatal("Сессия должна быть завершена")
		}
	})
}

// TestCombatMechanics тестирует боевые механики
func TestCombatMechanics(t *testing.T) {
	cfg := setupIntegrationTest(t)
	defer cleanupTest(t, cfg)

	ctx := cfg.ctx
	chatID := cfg.chatID

	// Создаем игру и персонажа
	world, err := cfg.initCampaignUC.Execute(ctx, "боевое фэнтези")
	if err != nil {
		t.Fatalf("Не удалось создать игру: %v", err)
	}

	gs := &session.GameSession{
		ChatID:  chatID,
		State:   session.StateActive,
		World:   *world,
		WorldID: world.ID,
	}
	if err := cfg.sessionRepo.Save(ctx, gs); err != nil {
		t.Fatalf("Не удалось сохранить сессию: %v", err)
	}

	req := characterapp.CreateCharacterRequest{
		ChatID: chatID,
		Name:   "Воин",
		Race:   character.RaceHuman,
		Class:  character.ClassFighter,
	}
	_, err = cfg.createCharacterUC.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Не удалось создать персонажа: %v", err)
	}

	t.Run("Инициация боя через действие", func(t *testing.T) {
		// Пытаемся начать бой через действие
		response, err := cfg.handleActionUC.Execute(ctx, chatID, "Атакую гоблина")
		if err != nil {
			t.Logf("Действие обработано (бой может не начаться): %v", err)
		} else {
			t.Logf("Ответ DM: %s", response)
		}
	})

	t.Run("Атака через команду", func(t *testing.T) {
		result, err := cfg.handleCombatUC.Execute(ctx, chatID, "атакую гоблина")
		if err != nil {
			t.Logf("Не удалось атаковать (может быть нет активного боя): %v", err)
		} else {
			t.Logf("Результат атаки: %s", result)
		}
	})
}

// TestCharacterCreation тестирует создание персонажей разных рас и классов
func TestCharacterCreation(t *testing.T) {
	cfg := setupIntegrationTest(t)
	defer cleanupTest(t, cfg)

	ctx := cfg.ctx

	testCases := []struct {
		name      string
		chatID    int64
		nameChar  string
		race      character.Race
		class     character.Class
		expectErr bool
	}{
		{"Elf Wizard", 200000001, "ЭльфМаг", character.RaceElf, character.ClassWizard, false},
		{"Human Fighter", 200000002, "ЧеловекВоин", character.RaceHuman, character.ClassFighter, false},
		{"Dwarf Cleric", 200000003, "ДварфЖрец", character.RaceDwarf, character.ClassCleric, false},
		{"Orc Rogue", 200000004, "ОркВор", character.RaceOrc, character.ClassRogue, false},
		{"Halfling Ranger", 200000005, "ХоббитСледопыт", character.RaceHalfling, character.ClassRanger, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Создаем новую игру для каждого персонажа
			world, err := cfg.initCampaignUC.Execute(ctx, "тест")
			if err != nil {
				t.Fatalf("Не удалось создать игру: %v", err)
			}

			gs := &session.GameSession{
				ChatID:  tc.chatID,
				State:   session.StateActive,
				World:   *world,
				WorldID: world.ID,
			}
			if err := cfg.sessionRepo.Save(ctx, gs); err != nil {
				t.Fatalf("Не удалось сохранить сессию: %v", err)
			}

			req := characterapp.CreateCharacterRequest{
				ChatID: tc.chatID,
				Name:   tc.nameChar,
				Race:   tc.race,
				Class:  tc.class,
			}

			player, err := cfg.createCharacterUC.Execute(ctx, req)
			if tc.expectErr && err == nil {
				t.Fatal("Ожидалась ошибка, но её не было")
			}
			if !tc.expectErr && err != nil {
				t.Fatalf("Неожиданная ошибка: %v", err)
			}

			if !tc.expectErr {
				if player == nil {
					t.Fatal("Игрок не создан")
				}
				if player.Character.Race != tc.race {
					t.Fatalf("Неверная раса: ожидалось %s, получено %s", tc.race, player.Character.Race)
				}
				if player.Character.Class != tc.class {
					t.Fatalf("Неверный класс: ожидалось %s, получено %s", tc.class, player.Character.Class)
				}
			}

			// Очищаем после теста
			cfg.db.Exec("DELETE FROM game_sessions WHERE chat_id = ?", tc.chatID)
		})
	}
}

// TestInventoryOperations тестирует операции с инвентарем
func TestInventoryOperations(t *testing.T) {
	cfg := setupIntegrationTest(t)
	defer cleanupTest(t, cfg)

	ctx := cfg.ctx
	chatID := cfg.chatID
	tgUserID := cfg.tgUserID

	// Создаем игру и персонажа
	world, err := cfg.initCampaignUC.Execute(ctx, "тест")
	if err != nil {
		t.Fatalf("Не удалось создать игру: %v", err)
	}

	gs := &session.GameSession{
		ChatID:  chatID,
		State:   session.StateActive,
		World:   *world,
		WorldID: world.ID,
	}
	if err := cfg.sessionRepo.Save(ctx, gs); err != nil {
		t.Fatalf("Не удалось сохранить сессию: %v", err)
	}

	req := characterapp.CreateCharacterRequest{
		ChatID: chatID,
		Name:   "Тест",
		Race:   character.RaceHuman,
		Class:  character.ClassFighter,
	}
	_, err = cfg.createCharacterUC.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Не удалось создать персонажа: %v", err)
	}

	t.Run("Просмотр пустого инвентаря", func(t *testing.T) {
		inventoryText, err := cfg.getInventoryUC.Execute(ctx, chatID, tgUserID)
		if err != nil {
			t.Fatalf("Не удалось получить инвентарь: %v", err)
		}
		if inventoryText == "" {
			t.Fatal("Текст инвентаря пуст")
		}
		t.Logf("Инвентарь: %s", inventoryText)
	})

	t.Run("Подбор предмета", func(t *testing.T) {
		req := inventoryapp.AddItemRequest{
			ChatID:   chatID,
			TgUserID: tgUserID,
			ItemName: "меч",
			Quantity: 1,
		}

		result, err := cfg.addItemUC.Execute(ctx, req)
		if err != nil {
			t.Logf("Не удалось подобрать предмет (может быть не в контексте): %v", err)
		} else {
			t.Logf("Результат подбора предмета: %s", result)
		}
	})
}

// TestDiceRolling тестирует броски кубиков
func TestDiceRolling(t *testing.T) {
	cfg := setupIntegrationTest(t)
	defer cleanupTest(t, cfg)

	ctx := cfg.ctx

	testCases := []struct {
		name string
		args string
	}{
		{"d20", "d20"},
		{"2d6", "2d6"},
		{"d20+5", "d20+5"},
		{"2d6+3", "2d6+3"},
		{"d100", "d100"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := cfg.rollDiceUC.Execute(ctx, tc.args)
			if err != nil {
				t.Fatalf("Не удалось бросить кубик %s: %v", tc.args, err)
			}
			if result == "" {
				t.Fatalf("Результат броска пуст для %s", tc.args)
			}
			t.Logf("Результат броска %s: %s", tc.args, result)
		})
	}
}

// TestRatingSystem тестирует систему рейтингов и лидербордов
func TestRatingSystem(t *testing.T) {
	cfg := setupIntegrationTest(t)
	defer cleanupTest(t, cfg)

	ctx := cfg.ctx
	chatID := cfg.chatID
	tgUserID := cfg.tgUserID

	var problems []string
	var llmFeedback []string

	// Создаем игру и персонажа
	world, err := cfg.initCampaignUC.Execute(ctx, "тест рейтингов")
	if err != nil {
		problems = append(problems, fmt.Sprintf("Не удалось создать игру: %v", err))
		t.Fatalf("Не удалось создать игру: %v", err)
	}

	gs := &session.GameSession{
		ChatID:  chatID,
		State:   session.StateActive,
		World:   *world,
		WorldID: world.ID,
	}
	if err := cfg.sessionRepo.Save(ctx, gs); err != nil {
		problems = append(problems, fmt.Sprintf("Не удалось сохранить сессию: %v", err))
		t.Fatalf("Не удалось сохранить сессию: %v", err)
	}

	req := characterapp.CreateCharacterRequest{
		ChatID: chatID,
		Name:   "РейтинговыйГерой",
		Race:   character.RaceHuman,
		Class:  character.ClassFighter,
	}
	player, err := cfg.createCharacterUC.Execute(ctx, req)
	if err != nil {
		problems = append(problems, fmt.Sprintf("Не удалось создать персонажа: %v", err))
		t.Fatalf("Не удалось создать персонажа: %v", err)
	}

	initialLevel := player.Character.Level
	initialXP := player.Character.Experience

	t.Logf("✅ Персонаж создан: уровень=%d, опыт=%d", initialLevel, initialXP)

	// Шаг 1: Получение опыта и проверка обновления рейтинга
	t.Run("Получение опыта и обновление рейтинга", func(t *testing.T) {
		// Добавляем опыт
		addXPReq := characterapp.AddExperienceRequest{
			ChatID: chatID,
			Amount: 500,
			Reason: "test",
		}
		playerAfterXP, leveledUp, err := cfg.addExperienceUC.Execute(ctx, addXPReq)
		if err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось добавить опыт: %v", err))
			t.Fatalf("Не удалось добавить опыт: %v", err)
		}
		if playerAfterXP == nil {
			problems = append(problems, "Игрок не возвращен после добавления опыта")
			t.Fatal("Игрок не возвращен")
		}

		t.Logf("✅ Опыт добавлен: leveledUp=%v, новый опыт=%d", leveledUp, playerAfterXP.Character.Experience)

		// Обновляем рейтинг
		updateReq := ratingapp.UpdateRatingRequest{
			TgUserID: tgUserID,
			ChatID:   chatID,
		}
		if err := cfg.updateRatingUC.Execute(ctx, updateReq); err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось обновить рейтинг: %v", err))
			t.Fatalf("Не удалось обновить рейтинг: %v", err)
		}

		// Проверяем лидерборд по опыту
		leaderboardReq := ratingapp.GetLeaderboardRequest{
			MetricType: rating.MetricTypeExperience,
			Limit:      10,
			TgUserID:   tgUserID,
		}
		leaderboardResp, err := cfg.getLeaderboardUC.Execute(ctx, leaderboardReq)
		if err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось получить лидерборд: %v", err))
			t.Fatalf("Не удалось получить лидерборд: %v", err)
		}

		if leaderboardResp == nil {
			problems = append(problems, "Лидерборд пуст")
			t.Fatal("Лидерборд пуст")
		}

		t.Logf("✅ Лидерборд получен: метрика=%s, записей=%d, ранг пользователя=%d",
			leaderboardResp.MetricType, len(leaderboardResp.Entries), leaderboardResp.UserRank)

		// Проверяем, что пользователь в лидерборде или имеет рейтинг
		if len(leaderboardResp.Entries) == 0 && leaderboardResp.UserRating == 0 {
			llmFeedback = append(llmFeedback, "Пользователь не появился в лидерборде после получения опыта")
		}
	})

	// Шаг 2: Проверка лидербордов по разным метрикам
	t.Run("Проверка лидербордов по разным метрикам", func(t *testing.T) {
		metrics := []rating.RatingMetricType{
			rating.MetricTypeLevel,
			rating.MetricTypeExperience,
			rating.MetricTypeCombatWins,
			rating.MetricTypeQuestsCompleted,
			rating.MetricTypeTotalRating,
		}

		for _, metricType := range metrics {
			req := ratingapp.GetLeaderboardRequest{
				MetricType: metricType,
				Limit:      10,
				TgUserID:   tgUserID,
			}
			resp, err := cfg.getLeaderboardUC.Execute(ctx, req)
			if err != nil {
				problems = append(problems, fmt.Sprintf("Не удалось получить лидерборд по метрике %s: %v", metricType, err))
				t.Errorf("Не удалось получить лидерборд по метрике %s: %v", metricType, err)
				continue
			}

			if resp == nil {
				problems = append(problems, fmt.Sprintf("Лидерборд по метрике %s пуст", metricType))
				t.Errorf("Лидерборд по метрике %s пуст", metricType)
				continue
			}

			t.Logf("✅ Лидерборд по метрике %s: записей=%d", metricType, len(resp.Entries))
		}
	})

	// Записываем найденные проблемы
	if len(problems) > 0 {
		writeToTestingReport(problems)
	}
	if len(llmFeedback) > 0 {
		writeToFeedback(llmFeedback)
	}
}

// TestCombatFlow_WithLLM тестирует полный поток боя с реальными ответами LLM
func TestCombatFlow_WithLLM(t *testing.T) {
	cfg := setupIntegrationTest(t)
	defer cleanupTest(t, cfg)

	ctx := cfg.ctx
	chatID := cfg.chatID

	var problems []string
	var llmFeedback []string

	// Создаем игру и персонажа
	world, err := cfg.initCampaignUC.Execute(ctx, "боевое фэнтези с гоблинами")
	if err != nil {
		problems = append(problems, fmt.Sprintf("Не удалось создать игру: %v", err))
		t.Fatalf("Не удалось создать игру: %v", err)
	}

	gs := &session.GameSession{
		ChatID:  chatID,
		State:   session.StateActive,
		World:   *world,
		WorldID: world.ID,
	}
	if err := cfg.sessionRepo.Save(ctx, gs); err != nil {
		problems = append(problems, fmt.Sprintf("Не удалось сохранить сессию: %v", err))
		t.Fatalf("Не удалось сохранить сессию: %v", err)
	}

	req := characterapp.CreateCharacterRequest{
		ChatID: chatID,
		Name:   "ВоинБой",
		Race:   character.RaceHuman,
		Class:  character.ClassFighter,
	}
	player, err := cfg.createCharacterUC.Execute(ctx, req)
	if err != nil {
		problems = append(problems, fmt.Sprintf("Не удалось создать персонажа: %v", err))
		t.Fatalf("Не удалось создать персонажа: %v", err)
	}

	initialHP := player.Character.HP
	t.Logf("✅ Персонаж создан: %s, HP=%d, MaxHP=%d", player.Character.Name, initialHP, player.Character.MaxHP)

	// Шаг 1: Инициация боя через действие с реальным LLM
	t.Run("Инициация боя через действие", func(t *testing.T) {
		response, err := cfg.handleActionUC.Execute(ctx, chatID, "Вижу гоблина и атакую его мечом")
		if err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось обработать боевое действие: %v", err))
			t.Fatalf("Не удалось обработать боевое действие: %v", err)
		}
		if response == "" {
			problems = append(problems, "Ответ DM пуст при инициации боя")
			t.Fatal("Ответ DM пуст")
		}

		// Проверяем, что ответ содержит боевые элементы
		responseLower := strings.ToLower(response)
		hasCombatKeywords := strings.Contains(responseLower, "бой") ||
			strings.Contains(responseLower, "атака") ||
			strings.Contains(responseLower, "урон") ||
			strings.Contains(responseLower, "гоблин") ||
			strings.Contains(responseLower, "hit") ||
			strings.Contains(responseLower, "damage")

		if !hasCombatKeywords {
			llmFeedback = append(llmFeedback, fmt.Sprintf("Ответ DM при инициации боя не содержит боевых элементов: %s", response[:200]))
		}

		t.Logf("✅ DM ответил на боевое действие: %s...", response[:min(300, len(response))])
	})

	// Шаг 2: Проверка активного боя
	t.Run("Проверка активного боя", func(t *testing.T) {
		gs, err := cfg.sessionRepo.GetByChatID(ctx, chatID)
		if err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось получить сессию: %v", err))
			return
		}

		combatRepo := persistence.NewCombatRepository(cfg.db)
		combat, err := combatRepo.GetActiveBySessionID(ctx, gs.ID)
		if err != nil {
			problems = append(problems, fmt.Sprintf("Ошибка при получении боя: %v", err))
			return
		}

		if combat != nil {
			t.Logf("✅ Активный бой найден: участников=%d, текущий ход=%d", len(combat.Participants), combat.CurrentTurn)

			// Проверяем, что есть участники
			if len(combat.Participants) == 0 {
				problems = append(problems, "Бой создан без участников")
			}

			// Проверяем порядок ходов
			if combat.CurrentTurn < 0 || combat.CurrentTurn >= len(combat.Participants) {
				problems = append(problems, fmt.Sprintf("Некорректный индекс текущего хода: %d (участников: %d)", combat.CurrentTurn, len(combat.Participants)))
			}
		} else {
			t.Logf("⚠️ Активный бой не найден (может быть бой еще не начался или уже завершен)")
		}
	})

	// Шаг 3: Выполнение атак в бою
	t.Run("Выполнение атак в бою", func(t *testing.T) {
		gs, err := cfg.sessionRepo.GetByChatID(ctx, chatID)
		if err != nil {
			t.Logf("⚠️ Не удалось получить сессию: %v", err)
			return
		}

		combatRepo := persistence.NewCombatRepository(cfg.db)
		combat, err := combatRepo.GetActiveBySessionID(ctx, gs.ID)
		if err != nil {
			t.Logf("⚠️ Ошибка при получении боя: %v", err)
			return
		}

		if combat != nil {
			// Пытаемся атаковать
			result, err := cfg.handleCombatUC.Execute(ctx, chatID, "атакую гоблина")
			if err != nil {
				t.Logf("⚠️ Не удалось атаковать: %v (может быть нет активного боя или бой завершен)", err)
			} else {
				if result == "" {
					problems = append(problems, "Результат атаки пуст")
				} else {
					t.Logf("✅ Результат атаки: %s", result)
				}
			}
		}
	})

	// Записываем найденные проблемы
	if len(problems) > 0 {
		writeToTestingReport(problems)
	}
	if len(llmFeedback) > 0 {
		writeToFeedback(llmFeedback)
	}
}

// TestDailyQuests_Complete тестирует выполнение всех трех типов ежедневных заданий
func TestDailyQuests_Complete(t *testing.T) {
	cfg := setupIntegrationTest(t)
	defer cleanupTest(t, cfg)

	ctx := cfg.ctx
	chatID := cfg.chatID
	tgUserID := cfg.tgUserID

	var problems []string

	// Создаем игру и персонажа
	world, err := cfg.initCampaignUC.Execute(ctx, "тест ежедневных заданий")
	if err != nil {
		problems = append(problems, fmt.Sprintf("Не удалось создать игру: %v", err))
		t.Fatalf("Не удалось создать игру: %v", err)
	}

	gs := &session.GameSession{
		ChatID:  chatID,
		State:   session.StateActive,
		World:   *world,
		WorldID: world.ID,
	}
	if err := cfg.sessionRepo.Save(ctx, gs); err != nil {
		problems = append(problems, fmt.Sprintf("Не удалось сохранить сессию: %v", err))
		t.Fatalf("Не удалось сохранить сессию: %v", err)
	}

	req := characterapp.CreateCharacterRequest{
		ChatID: chatID,
		Name:   "КвестГерой",
		Race:   character.RaceHuman,
		Class:  character.ClassFighter,
	}
	_, err = cfg.createCharacterUC.Execute(ctx, req)
	if err != nil {
		problems = append(problems, fmt.Sprintf("Не удалось создать персонажа: %v", err))
		t.Fatalf("Не удалось создать персонажа: %v", err)
	}

	// Шаг 1: Получение ежедневных заданий
	t.Run("Получение ежедневных заданий", func(t *testing.T) {
		dailyText, err := cfg.getDailyQuestsUC.Execute(ctx, chatID, tgUserID)
		if err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось получить ежедневные задания: %v", err))
			t.Fatalf("Не удалось получить ежедневные задания: %v", err)
		}
		if dailyText == "" {
			problems = append(problems, "Текст ежедневных заданий пуст")
			t.Fatal("Текст ежедневных заданий пуст")
		}

		// Проверяем наличие всех трех типов заданий
		textLower := strings.ToLower(dailyText)
		hasExploreQuest := strings.Contains(textLower, "исследовать локацию") || strings.Contains(textLower, "локация")
		hasQuestQuest := strings.Contains(textLower, "завершить квест") || strings.Contains(textLower, "квест")
		hasCombatQuest := strings.Contains(textLower, "победить в бою") || strings.Contains(textLower, "бой")

		if !hasExploreQuest {
			problems = append(problems, "Ежедневные задания не содержат задание на исследование локации")
		}
		if !hasQuestQuest {
			problems = append(problems, "Ежедневные задания не содержат задание на завершение квеста")
		}
		if !hasCombatQuest {
			problems = append(problems, "Ежедневные задания не содержат задание на победу в бою")
		}

		t.Logf("✅ Ежедневные задания получены: %s", dailyText)
	})

	// Шаг 2: Выполнение задания на исследование локации
	t.Run("Выполнение задания на исследование локации", func(t *testing.T) {
		// Выполняем действие, которое должно обновить прогресс
		_, err := cfg.handleActionUC.Execute(ctx, chatID, "Исследую комнату, осматриваю все детали")
		if err != nil {
			t.Logf("⚠️ Действие обработано с ошибкой: %v", err)
		}

		// Проверяем прогресс через getDailyQuestsUC
		dailyText, err := cfg.getDailyQuestsUC.Execute(ctx, chatID, tgUserID)
		if err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось получить ежедневные задания после действия: %v", err))
		} else {
			t.Logf("✅ Ежедневные задания после исследования: %s", dailyText)
		}
	})

	// Шаг 3: Выполнение задания на победу в бою
	t.Run("Выполнение задания на победу в бою", func(t *testing.T) {
		// Пытаемся начать бой
		_, err := cfg.handleActionUC.Execute(ctx, chatID, "Атакую врага")
		if err != nil {
			t.Logf("⚠️ Не удалось начать бой: %v", err)
		}

		// Проверяем прогресс после боя (если бой начался и завершился)
		dailyText, err := cfg.getDailyQuestsUC.Execute(ctx, chatID, tgUserID)
		if err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось получить ежедневные задания после боя: %v", err))
		} else {
			t.Logf("✅ Ежедневные задания после боя: %s", dailyText)
		}
	})

	// Записываем найденные проблемы
	if len(problems) > 0 {
		writeToTestingReport(problems)
	}
}

// TestFullGameplayScenario тестирует полный игровой сценарий с реальными ответами LLM
func TestFullGameplayScenario(t *testing.T) {
	cfg := setupIntegrationTest(t)
	defer cleanupTest(t, cfg)

	ctx := cfg.ctx
	chatID := cfg.chatID
	tgUserID := cfg.tgUserID

	var problems []string
	var llmFeedback []string

	// Шаг 1: Создание новой игры
	t.Run("1. Создание новой игры", func(t *testing.T) {
		world, err := cfg.initCampaignUC.Execute(ctx, "классическое фэнтези с магией и драконами")
		if err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось создать игру: %v", err))
			t.Fatalf("Не удалось создать игру: %v", err)
		}
		if world == nil || len(world.Locations) == 0 {
			problems = append(problems, "Мир создан без локаций")
			t.Fatal("Мир создан без локаций")
		}

		gs := &session.GameSession{
			ChatID:  chatID,
			State:   session.StateActive,
			World:   *world,
			WorldID: world.ID,
		}
		if err := cfg.sessionRepo.Save(ctx, gs); err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось сохранить сессию: %v", err))
			t.Fatalf("Не удалось сохранить сессию: %v", err)
		}

		t.Logf("✅ Игра создана: мир ID=%d, локаций=%d", world.ID, len(world.Locations))
	})

	// Шаг 2: Создание персонажа
	t.Run("2. Создание персонажа", func(t *testing.T) {
		req := characterapp.CreateCharacterRequest{
			ChatID: chatID,
			Name:   "ТестовыйГерой",
			Race:   character.RaceElf,
			Class:  character.ClassWizard,
		}

		player, err := cfg.createCharacterUC.Execute(ctx, req)
		if err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось создать персонажа: %v", err))
			t.Fatalf("Не удалось создать персонажа: %v", err)
		}
		if player == nil || player.Character.Stats.Strength == 0 {
			problems = append(problems, "Персонаж создан без характеристик")
			t.Error("Персонаж создан без характеристик")
		}

		t.Logf("✅ Персонаж создан: %s (%s %s), HP=%d", player.Character.Name, player.Character.Race, player.Character.Class, player.Character.HP)
	})

	// Шаг 3: Игровые действия с реальным LLM
	t.Run("3. Игровые действия", func(t *testing.T) {
		actions := []string{
			"Осматриваю комнату, в которой нахожусь",
			"Ищу предметы в комнате",
			"Изучаю картины на стенах",
		}

		for i, action := range actions {
			response, err := cfg.handleActionUC.Execute(ctx, chatID, action)
			if err != nil {
				problems = append(problems, fmt.Sprintf("Не удалось обработать действие '%s': %v", action, err))
				t.Errorf("Не удалось обработать действие: %v", err)
				continue
			}

			if response == "" {
				problems = append(problems, fmt.Sprintf("Ответ DM пуст для действия '%s'", action))
				t.Errorf("Ответ DM пуст")
				continue
			}

			if len(response) < 50 {
				llmFeedback = append(llmFeedback, fmt.Sprintf("Ответ DM слишком короткий для действия '%s' (%d символов): %s", action, len(response), response))
			}

			t.Logf("✅ Действие %d: %s...", i+1, response[:min(200, len(response))])
		}
	})

	// Шаг 4: Просмотр всех систем
	t.Run("4. Просмотр систем игры", func(t *testing.T) {
		// Инвентарь
		inventoryText, err := cfg.getInventoryUC.Execute(ctx, chatID, tgUserID)
		if err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось получить инвентарь: %v", err))
		} else {
			t.Logf("✅ Инвентарь: %s", inventoryText[:min(100, len(inventoryText))])
		}

		// Ежедневные задания
		dailyText, err := cfg.getDailyQuestsUC.Execute(ctx, chatID, tgUserID)
		if err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось получить ежедневные задания: %v", err))
		} else {
			t.Logf("✅ Ежедневные задания: %s", dailyText[:min(100, len(dailyText))])
		}

		// Квесты
		questsText, err := cfg.getQuestsUC.Execute(ctx, chatID)
		if err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось получить квесты: %v", err))
		} else if questsText != "" {
			t.Logf("✅ Квесты: %s", questsText[:min(100, len(questsText))])
		}

		// Достижения
		achievementsReq := achievementapp.GetAchievementsRequest{
			ChatID:   chatID,
			TgUserID: tgUserID,
		}
		achievementsText, err := cfg.getAchievementsUC.Execute(ctx, achievementsReq)
		if err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось получить достижения: %v", err))
		} else {
			t.Logf("✅ Достижения: %s", achievementsText[:min(100, len(achievementsText))])
		}

		// Заклинания
		spellsReq := spellapp.GetSpellsRequest{
			ChatID:   chatID,
			TgUserID: tgUserID,
		}
		spellsText, err := cfg.getSpellsUC.Execute(ctx, spellsReq)
		if err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось получить заклинания: %v", err))
		} else {
			t.Logf("✅ Заклинания: %s", spellsText[:min(100, len(spellsText))])
		}

		// Карта
		mapText, err := cfg.getMapUC.Execute(ctx, chatID)
		if err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось получить карту: %v", err))
		} else {
			t.Logf("✅ Карта: %s", mapText[:min(100, len(mapText))])
		}

		// История
		historyText, err := cfg.getHistoryUC.Execute(ctx, chatID, 10)
		if err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось получить историю: %v", err))
		} else {
			t.Logf("✅ История: %s", historyText[:min(100, len(historyText))])
		}
	})

	// Записываем найденные проблемы
	if len(problems) > 0 {
		writeToTestingReport(problems)
	}
	if len(llmFeedback) > 0 {
		writeToFeedback(llmFeedback)
	}
}

// Функции записи в отчеты

func writeToTestingReport(problems []string) {
	reportPath := "TESTING_REPORT.md"

	// Читаем существующий отчет
	existingContent := ""
	if data, err := os.ReadFile(reportPath); err == nil {
		existingContent = string(data)
	}

	// Добавляем новые проблемы
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	newSection := fmt.Sprintf("\n## Проблемы, найденные при интеграционном тестировании (%s)\n\n", timestamp)

	for i, problem := range problems {
		newSection += fmt.Sprintf("%d. %s\n", i+1, problem)
	}
	newSection += "\n---\n"

	// Записываем обновленный отчет
	updatedContent := existingContent + newSection
	if err := os.WriteFile(reportPath, []byte(updatedContent), 0644); err != nil {
		fmt.Printf("⚠️ Не удалось записать в TESTING_REPORT.md: %v\n", err)
	}
}

func writeToFeedback(feedback []string) {
	feedbackPath := "FEEDBACK.md"

	// Читаем существующий файл
	existingContent := ""
	if data, err := os.ReadFile(feedbackPath); err == nil {
		existingContent = string(data)
	}

	// Добавляем новую обратную связь
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	newSection := fmt.Sprintf("\n## Обратная связь от интеграционных тестов (%s)\n\n", timestamp)

	for i, item := range feedback {
		newSection += fmt.Sprintf("%d. %s\n", i+1, item)
	}
	newSection += "\n---\n"

	// Записываем обновленный файл
	updatedContent := existingContent + newSection
	if err := os.WriteFile(feedbackPath, []byte(updatedContent), 0644); err != nil {
		fmt.Printf("⚠️ Не удалось записать в FEEDBACK.md: %v\n", err)
	}
}

// Вспомогательные функции

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func isContainersRunning(t *testing.T) bool {
	// Проверяем подключение к БД через gorm
	dbDSN := getEnv("DATABASE_URL", "postgres://dnd_user:dnd_password@localhost:5432/dnd?sslmode=disable")
	// Если в DATABASE_URL указан postgres (Docker hostname), заменяем на localhost для локального запуска
	if strings.Contains(dbDSN, "@postgres:") {
		dbDSN = strings.Replace(dbDSN, "@postgres:", "@localhost:", 1)
	}
	db, err := gorm.Open(postgres.Open(dbDSN), &gorm.Config{})
	if err != nil {
		return false
	}

	sqlDB, err := db.DB()
	if err != nil {
		return false
	}
	defer sqlDB.Close()

	if err := sqlDB.Ping(); err != nil {
		return false
	}

	// Проверяем подключение к Qdrant
	qdrantHost := getEnv("QDRANT_HOST", "localhost")
	if qdrantHost == "qdrant" {
		qdrantHost = "localhost"
	}
	// Используем QDRANT_GRPC_PORT для gRPC подключения
	qdrantGrpcPort := getEnv("QDRANT_GRPC_PORT", "6335")
	if qdrantGrpcPort == "" {
		qdrantGrpcPort = "6335"
	}
	client, err := qdrant.NewClient(&qdrant.Config{
		Host: qdrantHost,
		Port: parsePort(qdrantGrpcPort),
	})
	if err != nil {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = client.ListCollections(ctx)
	return err == nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func parsePort(portStr string) int {
	var port int
	fmt.Sscanf(portStr, "%d", &port)
	if port == 0 {
		return 6334
	}
	return port
}
