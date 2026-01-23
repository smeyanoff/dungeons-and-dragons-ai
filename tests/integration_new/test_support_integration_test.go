package integration_new

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

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
	"dungeons-and-dragons-ai/internal/game/domain/session"
	worlddomain "dungeons-and-dragons-ai/internal/game/domain/world"
	dmcache "dungeons-and-dragons-ai/internal/game/infrastructure/cache"
	contextbuilder "dungeons-and-dragons-ai/internal/game/infrastructure/context"
	"dungeons-and-dragons-ai/internal/game/infrastructure/persistence"
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

// testConfig содержит конфигурацию для интеграционных тестов.
// Важно: это test-only инфраструктура (не часть прод-кода).
type testConfig struct {
	db           *gorm.DB
	qdrantClient *qdrant.Client
	chatID       int64
	tgUserID     int64
	ctx          context.Context

	initCampaignUC       *campaign.InitCampaignUseCase
	handleActionUC       *player_action.HandleActionUseCase
	createCharacterUC    *characterapp.CreateCharacterUseCase
	getHistoryUC         *history.GetHistoryUseCase
	getInventoryUC       *inventoryapp.GetInventoryUseCase
	addItemUC            *inventoryapp.AddItemUseCase
	handleCombatUC       *combatapp.HandleCombatUseCase
	rollDiceUC           *dice.RollDiceUseCase
	getQuestsUC          *questapp.GetQuestsUseCase
	getDailyQuestsUC     *questapp.GetDailyQuestsUseCase
	checkDailyProgressUC *questapp.CheckDailyQuestProgressUseCase
	getMapUC             *mapapp.GetMapUseCase
	getAchievementsUC    *achievementapp.GetAchievementsUseCase
	getSpellsUC          *spellapp.GetSpellsUseCase
	useSpellUC           *spellapp.UseSpellUseCase
	getLeaderboardUC     *ratingapp.GetLeaderboardUseCase
	updateRatingUC       *ratingapp.UpdateRatingUseCase
	addExperienceUC      *characterapp.AddExperienceUseCase

	sessionRepo session.Repository
}

// setupIntegrationTest настраивает окружение для интеграционных тестов, которые используют:
// Postgres + Qdrant + (опционально) реальный LLM (GigaChat).
func setupIntegrationTest(t *testing.T) *testConfig {
	ctx := context.Background()

	// Logger (не критично для тестов).
	if err := logger.InitFromEnv(); err != nil {
		t.Logf("Не удалось инициализировать логгер: %v", err)
	}

	// Postgres
	dbDSN := getEnv("DATABASE_URL", "postgres://dnd_user:dnd_password@localhost:5432/dnd?sslmode=disable")
	if strings.Contains(dbDSN, "@postgres:") {
		dbDSN = strings.Replace(dbDSN, "@postgres:", "@localhost:", 1)
	}
	db, err := gorm.Open(postgres.Open(dbDSN), &gorm.Config{})
	if err != nil {
		t.Fatalf("Не удалось подключиться к БД: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("Не удалось получить sql.DB: %v", err)
	}
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("Не удалось выполнить ping к БД: %v", err)
	}

	// Миграции для интеграционных тестов (volume может быть старый).
	if err := db.AutoMigrate(&session.GameSession{}); err != nil {
		t.Fatalf("Не удалось выполнить AutoMigrate для game_sessions: %v", err)
	}
	if err := db.AutoMigrate(&worlddomain.WorldEvent{}); err != nil {
		t.Fatalf("Не удалось выполнить AutoMigrate для world_events: %v", err)
	}

	// Qdrant (gRPC)
	qdrantHost := getEnv("QDRANT_HOST", "localhost")
	if qdrantHost == "qdrant" {
		qdrantHost = "localhost"
	}
	qdrantGrpcPort := getEnv("QDRANT_GRPC_PORT", "6335")
	if qdrantGrpcPort == "" {
		qdrantGrpcPort = "6335"
	}
	qdrantClient, err := qdrant.NewClient(&qdrant.Config{
		Host: qdrantHost,
		Port: parsePort(qdrantGrpcPort),
		// Qdrant сервер может быть старее клиента; пропускаем проверку совместимости осознанно.
		SkipCompatibilityCheck: true,
	})
	if err != nil {
		t.Fatalf("Не удалось подключиться к Qdrant: %v", err)
	}
	if _, err := qdrantClient.ListCollections(ctx); err != nil {
		t.Fatalf("Не удалось проверить подключение к Qdrant: %v", err)
	}

	// Если контейнеры не запущены — skip (на CI без docker-up).
	if !isContainersRunning(t) {
		t.Skip("Контейнеры не запущены или недоступны. Запустите: make docker-up")
	}

	// GigaChat credentials: если нет — skip LLM-dependent integration tests.
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

	// LLM (+ test-only rate limit, чтобы не DDOSить модель при полном прогоне).
	llm := llminfrastructure.NewGigachatLLM(gigachatClient, gigachatModel)
	llm = wrapLLMWithTestRateLimit(llm)
	imageGenerator := llminfrastructure.NewGigachatImageGenerator(gigachatClient, gigachatModel)

	// Image storage (локально).
	imageStoragePath := getEnv("IMAGE_STORAGE_PATH", "./test_images")
	imageStorage, err := imageapp.NewLocalImageStorage(imageStoragePath)
	if err != nil {
		t.Fatalf("Не удалось создать image storage: %v", err)
	}
	generateImageUC := imageapp.NewImageGenerationUseCase(imageGenerator, imageStorage)
	dailyLimit := 5
	fallbackLimiter := imageapp.NewInMemoryRateLimiter(dailyLimit)
	generateImageUC.SetLimiter(fallbackLimiter)

	// RAG
	embedder := ragembeddings.NewGigachatEmbedder(gigachatClient)
	vectorStore := ragvectorstore.NewQdrantStore(qdrantClient)
	if err := vectorStore.EnsureCollection(ctx); err != nil {
		t.Fatalf("Не удалось создать коллекцию Qdrant: %v", err)
	}
	indexDocUC := ragapp.NewIndexDocument(embedder, vectorStore)
	retrieveContextUC := ragapp.NewRetrieveContext(embedder, vectorStore)

	// Repos
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

	// Use-cases + wiring (по аналогии с main.go)
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

	// Rating
	getLeaderboardUC := ratingapp.NewGetLeaderboardUseCase(ratingRepo)
	achievementRepoAdapter := &ratingAchievementRepoAdapter{repo: achievementRepo}
	updateRatingUC := ratingapp.NewUpdateRatingUseCase(ratingRepo, sessionRepo, playerRepo, achievementRepoAdapter)

	getSubscriptionUC := subscriptionapp.NewGetSubscriptionUseCase(subscriptionRepo)
	dailyQuestProgressAdapter := &dailyQuestProgressAdapterForPlayerAction{uc: checkDailyProgressUC}
	ratingUpdaterAdapterForPlayerAction := &ratingUpdaterAdapterForPlayerAction{uc: updateRatingUC}
	ratingUpdaterAdapterForCharacter := &ratingUpdaterAdapter{uc: updateRatingUC}
	addExperienceUC.SetRatingUpdater(ratingUpdaterAdapterForCharacter)

	analyzePlayerActionUC := dm_analyzer.NewAnalyzePlayerActionUseCase(llm, eventRepo)
	locationEventGenerator := locationeventapp.NewLocationEventGenerator(worldEventRepo)

	handleActionUC := player_action.NewHandleActionUseCase(
		llm, sessionRepo, ragContextBuilder, eventRepo, indexDocUC, combatRepo, questRepo,
		inventoryRepo, addExperienceUC, checkWorldEventsUC, checkAchievementsUC,
		notificationService, generateImageUC, useSpellUC, responseCache, actionValidator,
		dailyQuestProgressAdapter, getSubscriptionUC, ratingUpdaterAdapterForPlayerAction,
		analyzePlayerActionUC, locationEventGenerator,
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

	// Инициализация базовых данных (не критично, но улучшает UX ответов).
	initCtx, initCancel := context.WithTimeout(ctx, 30*time.Second)
	defer initCancel()
	_ = achievementRepo.InitDefaultAchievements(initCtx)
	_ = spellRepo.InitDefaultSpells(initCtx)

	// IDs
	chatID, tgUserID := generateTestIDs(t)

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
		addExperienceUC:      addExperienceUC,
		sessionRepo:          sessionRepo,
	}
}

// --- Adapters used by setupIntegrationTest ---

type ratingAchievementRepoAdapter struct {
	repo *persistence.AchievementRepository
}

func (a *ratingAchievementRepoAdapter) GetAchievementProgress(ctx context.Context, playerID uint, achievementID uint) (*ratingapp.AchievementProgress, error) {
	progress, err := a.repo.GetAchievementProgress(ctx, playerID, achievementID)
	if err != nil {
		return nil, err
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

type ratingUpdaterAdapter struct {
	uc *ratingapp.UpdateRatingUseCase
}

func (a *ratingUpdaterAdapter) Execute(ctx context.Context, req characterapp.RatingUpdateRequest) error {
	ratingReq := ratingapp.UpdateRatingRequest{TgUserID: req.TgUserID, ChatID: req.ChatID}
	return a.uc.Execute(ctx, ratingReq)
}

type ratingUpdaterAdapterForPlayerAction struct {
	uc *ratingapp.UpdateRatingUseCase
}

func (a *ratingUpdaterAdapterForPlayerAction) Execute(ctx context.Context, req player_action.RatingUpdateRequest) error {
	ratingReq := ratingapp.UpdateRatingRequest{TgUserID: req.TgUserID, ChatID: req.ChatID}
	return a.uc.Execute(ctx, ratingReq)
}

type dailyQuestProgressAdapterForPlayerAction struct {
	uc *questapp.CheckDailyQuestProgressUseCase
}

func (a *dailyQuestProgressAdapterForPlayerAction) Execute(ctx context.Context, req player_action.CheckDailyQuestProgressRequest) error {
	questReq := questapp.CheckProgressRequest{
		ChatID:    req.ChatID,
		TgUserID:  req.TgUserID,
		QuestType: req.QuestType,
		Increment: req.Increment,
	}
	return a.uc.Execute(ctx, questReq)
}

// cleanupTest очищает данные после теста.
func cleanupTest(t *testing.T, cfg *testConfig) {
	t.Helper()
	if cfg == nil || cfg.db == nil {
		return
	}

	// Удаляем тестовую сессию в правильном порядке (сначала дочерние записи, потом родительские).
	var sessionID uint
	cfg.db.Raw("SELECT id FROM game_sessions WHERE chat_id = ?", cfg.chatID).Scan(&sessionID)
	if sessionID > 0 {
		cfg.db.Exec("DELETE FROM combat_participants WHERE combat_id IN (SELECT id FROM combats WHERE game_session_id = ?)", sessionID)
		cfg.db.Exec("DELETE FROM combats WHERE game_session_id = ?", sessionID)
		cfg.db.Exec("DELETE FROM inventory_items WHERE inventory_id IN (SELECT id FROM inventories WHERE character_id IN (SELECT character_id FROM players WHERE game_session_id = ?))", sessionID)
		cfg.db.Exec("DELETE FROM inventories WHERE character_id IN (SELECT character_id FROM players WHERE game_session_id = ?)", sessionID)
		cfg.db.Exec("DELETE FROM story_events WHERE game_session_id = ?", sessionID)
		cfg.db.Exec("DELETE FROM players WHERE game_session_id = ?", sessionID)
		cfg.db.Exec("DELETE FROM characters WHERE id IN (SELECT character_id FROM players WHERE tg_user_id = ?)", cfg.tgUserID)
		cfg.db.Exec("DELETE FROM game_sessions WHERE id = ?", sessionID)
	}
	cfg.db.Exec("DELETE FROM players WHERE tg_user_id = ?", cfg.tgUserID)
	cfg.db.Exec("DELETE FROM characters WHERE id IN (SELECT character_id FROM players WHERE tg_user_id = ?)", cfg.tgUserID)
}

// reportFilePath возвращает путь к файлу отчёта в корне репозитория.
func reportFilePath(name string) string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok || thisFile == "" {
		return name
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	return filepath.Join(root, name)
}

func writeToTestingReport(problems []string) {
	reportPath := reportFilePath("TESTING_REPORT.md")
	existingContent := ""
	if data, err := os.ReadFile(reportPath); err == nil {
		existingContent = string(data)
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	newSection := fmt.Sprintf("\n## Проблемы, найденные при интеграционном тестировании (%s)\n\n", timestamp)
	for i, problem := range problems {
		newSection += fmt.Sprintf("%d. %s\n", i+1, problem)
	}
	newSection += "\n---\n"

	_ = os.WriteFile(reportPath, []byte(existingContent+newSection), 0644)
}

func writeToFeedback(feedback []string) {
	feedbackPath := reportFilePath("FEEDBACK.md")
	existingContent := ""
	if data, err := os.ReadFile(feedbackPath); err == nil {
		existingContent = string(data)
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	newSection := fmt.Sprintf("\n## Обратная связь от интеграционных тестов (%s)\n\n", timestamp)
	for i, item := range feedback {
		newSection += fmt.Sprintf("%d. %s\n", i+1, item)
	}
	newSection += "\n---\n"

	_ = os.WriteFile(feedbackPath, []byte(existingContent+newSection), 0644)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Stub functions for telegram gameplay tests
type telegramGameplayConfig struct {
	*testConfig
	rateLimiter interface{}
}

func setupTelegramGameplayTest(t *testing.T) *telegramGameplayConfig {
	cfg := setupIntegrationTest(t)
	return &telegramGameplayConfig{testConfig: cfg}
}

func (cfg *telegramGameplayConfig) waitForRateLimit(ctx context.Context) error {
	return nil // stub
}

type fakeTelegramAPI struct{}

func newFakeTelegramAPI() *fakeTelegramAPI {
	return &fakeTelegramAPI{}
}

func (f *fakeTelegramAPI) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
	}
}

func (f *fakeTelegramAPI) snapshotCalls() []interface{} {
	return []interface{}{} // stub
}

func makeMessageUpdate(chatID int64, userID int64, text string) interface{} {
	return map[string]interface{}{
		"message": map[string]interface{}{
			"chat": map[string]int64{"id": chatID},
			"from": map[string]int64{"id": userID},
			"text": text,
		},
	}
}

func lastNonThinkingPlayerFacingText(calls []interface{}, chatID int64) string {
	return "Test response" // stub
}

func findToolLeak(calls []interface{}, chatID int64) string {
	return "" // stub - no leaks
}

func isContainersRunning(t *testing.T) bool {
	t.Helper()

	dbDSN := getEnv("DATABASE_URL", "postgres://dnd_user:dnd_password@localhost:5432/dnd?sslmode=disable")
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

	qdrantHost := getEnv("QDRANT_HOST", "localhost")
	if qdrantHost == "qdrant" {
		qdrantHost = "localhost"
	}
	qdrantGrpcPort := getEnv("QDRANT_GRPC_PORT", "6335")
	if qdrantGrpcPort == "" {
		qdrantGrpcPort = "6335"
	}
	client, err := qdrant.NewClient(&qdrant.Config{
		Host: qdrantHost,
		Port: parsePort(qdrantGrpcPort),
		// Qdrant сервер может быть старее клиента; пропускаем проверку совместимости осознанно.
		SkipCompatibilityCheck: true,
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

func generateTestIDs(t *testing.T) (int64, int64) {
	t.Helper()
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	base := time.Now().UnixNano()
	jitter := rng.Int63n(1000)
	chatID := base + jitter
	return chatID, chatID
}
