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
	spellapp "dungeons-and-dragons-ai/internal/game/application/spell"
	subscriptionapp "dungeons-and-dragons-ai/internal/game/application/subscription"
	worldeventapp "dungeons-and-dragons-ai/internal/game/application/world_event"
	mapapp "dungeons-and-dragons-ai/internal/game/application/worldmap"
	dmcache "dungeons-and-dragons-ai/internal/game/infrastructure/cache"
	contextbuilder "dungeons-and-dragons-ai/internal/game/infrastructure/context"
	"dungeons-and-dragons-ai/internal/game/infrastructure/persistence"
	"dungeons-and-dragons-ai/internal/game/domain/character"
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

	// Адаптер для daily quest progress (как в main.go)
	dailyQuestProgressAdapter := &dailyQuestProgressAdapterForPlayerAction{uc: checkDailyProgressUC}
	handleActionUC := player_action.NewHandleActionUseCase(
		llm, sessionRepo, ragContextBuilder, eventRepo, indexDocUC, combatRepo, questRepo,
		inventoryRepo, addExperienceUC, checkWorldEventsUC, checkAchievementsUC,
		notificationService, generateImageUC, useSpellUC, responseCache, actionValidator,
		dailyQuestProgressAdapter,
	)
	createCharacterUC := characterapp.NewCreateCharacterUseCase(sessionRepo, playerRepo)
	getHistoryUC := history.NewGetHistoryUseCase(sessionRepo, eventRepo)
	getInventoryUC := inventoryapp.NewGetInventoryUseCase(sessionRepo, inventoryRepo)
	addItemUC := inventoryapp.NewAddItemUseCase(sessionRepo, inventoryRepo)
	handleCombatUC := combatapp.NewHandleCombatUseCase(combatRepo, sessionRepo)
	handleCombatUC.SetCheckAchievementsUseCase(checkAchievementsUC)
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
		sessionRepo:          sessionRepo,
	}
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

// Вспомогательные функции

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
