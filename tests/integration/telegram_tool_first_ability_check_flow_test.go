package integration

import (
	"context"
	"fmt"
	"strings"
	"testing"

	abilitycheck "dungeons-and-dragons-ai/internal/game/application/ability_check"
	achievementapp "dungeons-and-dragons-ai/internal/game/application/achievement"
	characterapp "dungeons-and-dragons-ai/internal/game/application/character"
	dm_tools "dungeons-and-dragons-ai/internal/game/application/dm_tools"
	"dungeons-and-dragons-ai/internal/game/application/player_action"
	"dungeons-and-dragons-ai/internal/game/domain/character"
	questdomain "dungeons-and-dragons-ai/internal/game/domain/quest"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	worlddomain "dungeons-and-dragons-ai/internal/game/domain/world"
	"dungeons-and-dragons-ai/internal/llm/domain"
	ragapp "dungeons-and-dragons-ai/internal/rag/application"
	ragdomain "dungeons-and-dragons-ai/internal/rag/domain"
	telegrambot "dungeons-and-dragons-ai/internal/telegram"

	"net/http/httptest"
)

// scriptedLLM — детерминированная заглушка LLM для стабильных E2E тестов tool-first механик.
// 1) На первом GenerateWithTools вызывает request_ability_check
// 2) На втором GenerateWithTools возвращает финальный текст без tools
type scriptedLLM struct {
	toolCall domain.LLMResponseWithTools
	final    domain.LLMResponseWithTools
	calls    int
}

func (l *scriptedLLM) Generate(ctx context.Context, prompt string) (string, error) {
	_ = ctx
	_ = prompt
	// DM Analyzer ожидает JSON; возвращаем валидный "пустой" анализ,
	// чтобы тест не создавал ложные сигналы "invalid analyzer json".
	return `{"combat_detected":false,"enemies":[],"quest_completed":false,"quest_failed":false,"quest_title":"","experience_gained":0,"experience_reason":"","items_received":[],"location_visited":null,"npc_met":null,"generated_images":[]}`, nil
}

func (l *scriptedLLM) GenerateWithMaxTokens(ctx context.Context, prompt string, maxTokens int) (string, error) {
	_ = maxTokens
	return l.Generate(ctx, prompt)
}

func (l *scriptedLLM) GenerateWithTools(ctx context.Context, prompt string, tools []dm_tools.Tool) (*domain.LLMResponseWithTools, error) {
	_ = tools

	l.calls++
	if l.calls == 1 {
		resp := l.toolCall
		return &resp, nil
	}
	resp := l.final
	return &resp, nil
}

// --- No-op RAG dependencies to avoid network calls in tests ---

type noopEmbedder struct{}

func (noopEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	_ = ctx
	_ = text
	// Minimal non-empty embedding to exercise the pipeline.
	return []float32{0.0, 0.0, 0.0}, nil
}

type noopVectorStore struct{}

func (noopVectorStore) EnsureCollection(ctx context.Context) error {
	_ = ctx
	return nil
}

func (noopVectorStore) Upsert(ctx context.Context, doc ragdomain.Document, embedding []float32) error {
	_ = ctx
	_ = doc
	_ = embedding
	return nil
}

func (noopVectorStore) Search(ctx context.Context, sessionID uint, embedding []float32, limit int) ([]ragdomain.Document, error) {
	_ = ctx
	_ = sessionID
	_ = embedding
	_ = limit
	return nil, nil
}

type staticContextBuilder struct{}

func (staticContextBuilder) BuildContext(ctx context.Context, gs *session.GameSession, playerMessage string) (string, error) {
	_ = ctx
	_ = gs
	return fmt.Sprintf("WORLD: %s\nPLAYER: %s", gs.World.Name, playerMessage), nil
}

// TestTelegramGameplay_BotSimulation_ToolFirstAbilityCheckFlow проверяет полный UX “tool-first ability check”:
// сообщение игрока -> LLM tool call request_ability_check -> pending check в сессии -> бот шлёт текстовую подсказку ->
// /roll d20 -> perform ability check -> pending cleared + событие в истории.
//
// Важно: тест НЕ дергает реальный LLM/embeddings, чтобы оставаться стабильным и не DDOSить модель.
func TestTelegramGameplay_BotSimulation_ToolFirstAbilityCheckFlow(t *testing.T) {
	cfg := setupInfraOnlyIntegrationTest(t)
	if cfg == nil {
		return
	}
	defer cleanupTest(t, &testConfig{db: cfg.db, chatID: cfg.chatID, tgUserID: cfg.tgUserID})

	ctx := cfg.ctx
	chatID := cfg.chatID
	tgUserID := cfg.tgUserID

	// Prepare deterministic world + session
	q := &questdomain.Quest{Title: "Test Quest (ToolFirstAbilityCheck)", Description: "Test quest for tool-first ability check flow"}
	w := worlddomain.New("Test World (ToolFirstAbilityCheck)")
	w.Description = "Deterministic test world for tool-first ability check flow"
	w.SetMainQuest(q)
	w.Locations = []worlddomain.Location{{Name: "Start", Description: "Start location"}}
	if err := cfg.worldRepo.Save(ctx, w); err != nil {
		t.Fatalf("Не удалось сохранить тестовый мир: %v", err)
	}

	gs := &session.GameSession{ChatID: chatID, State: session.StateActive, World: *w, WorldID: w.ID}
	if err := cfg.sessionRepo.Save(ctx, gs); err != nil {
		t.Fatalf("Не удалось сохранить сессию: %v", err)
	}

	// Create character (no real LLM)
	createCharacterUC := characterapp.NewCreateCharacterUseCase(cfg.sessionRepo, cfg.playerRepo)
	if _, err := createCharacterUC.Execute(ctx, characterapp.CreateCharacterRequest{
		ChatID: chatID,
		Name:   "ТестовыйГерой",
		Race:   character.RaceElf,
		Class:  character.ClassRogue,
	}); err != nil {
		t.Fatalf("Не удалось создать персонажа: %v", err)
	}

	// Fake Telegram API server
	fakeAPI := newFakeTelegramAPI()
	srv := httptest.NewServer(fakeAPI.handler(t))
	defer srv.Close()
	apiEndpointFmt := strings.TrimRight(srv.URL, "/") + "/bot%s/%s"

	// Deterministic LLM: request ability check, then narrate.
	llm := &scriptedLLM{
		toolCall: domain.LLMResponseWithTools{
			Content:  "Мне нужна проверка.",
			Finished: false,
			ToolCalls: []dm_tools.ToolCall{
				{
					Name: "request_ability_check",
					Arguments: map[string]interface{}{
						"ability": "dexterity",
						"dc":      13,
						"reason":  "вскрытие замка",
						"stakes":  "если провал — сработает ловушка",
					},
				},
			},
		},
		final: domain.LLMResponseWithTools{
			Content:  "Замок поддается не сразу — нужно проверить ловкость.",
			Finished: true,
		},
	}

	// No-op RAG indexer to satisfy HandleActionUseCase contract (it is called unconditionally).
	indexDocUC := ragapp.NewIndexDocument(noopEmbedder{}, noopVectorStore{})

	handleActionUC := player_action.NewHandleActionUseCase(
		llm,
		cfg.sessionRepo,
		staticContextBuilder{},
		cfg.eventRepo,
		indexDocUC,
		nil, // combatRepo
		nil, // questRepo
		nil, // inventoryRepo
		nil, // addExperienceUC
		nil, // checkWorldEventsUC
		nil, // checkAchievementsUC
		&achievementapp.NoOpNotificationService{},
		nil, // generateImageUC
		nil, // useSpellUC
		nil, // responseCache
		nil, // actionValidator
		nil, // checkDailyProgressUC
		nil, // getSubscriptionUC
		nil, // updateRatingUC
		nil, // analyzePlayerActionUC
		nil, // generateLocationEventUC
	)

	performUC := abilitycheck.NewPerformAbilityCheckUseCase(cfg.sessionRepo, cfg.eventRepo, nil)

	bot, err := telegrambot.NewBotWithAPIEndpoint(
		"TEST_TOKEN",
		apiEndpointFmt,
		nil, // initCampaignUC
		handleActionUC,
		nil, // createCharacterUC
		nil, // getHistoryUC
		nil, // getInventoryUC
		nil, // addItemUC
		nil, // handleCombatUC
		nil, // rollDiceUC
		nil, // getQuestsUC
		nil, // getDailyQuestsUC
		nil, // checkDailyProgressUC
		nil, // getMapUC
		nil, // moveToLocationUC
		nil, // getAchievementsUC
		nil, // getSpellsUC
		nil, // useSpellUC
		nil, // generateImageUC
		nil, // getSubscriptionUC
		nil, // checkLimitsUC
		nil, // getLeaderboardUC
		nil, // updateRatingUC
		performUC,
		cfg.sessionRepo,
		nil,           // combatRepo
		nil,           // feedbackRepo
		cfg.eventRepo, // eventRepo (ability check writes)
		nil,           // indexDocUC (skip /roll embeddings)
	)
	if err != nil {
		t.Fatalf("Не удалось создать Telegram bot (fake API): %v", err)
	}

	// Trigger player action (this should create pending check via tool)
	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "Пытаюсь вскрыть замок на сундуке")); err != nil {
		t.Fatalf("player action: %v", err)
	}

	// Verify pending check exists and notified (prompt sent)
	gs, err = cfg.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil || gs == nil {
		t.Fatalf("Не удалось получить сессию: %v", err)
	}
	if !gs.HasPendingAbilityCheck() {
		t.Fatalf("Ожидали pending ability check после действия игрока, но его нет")
	}
	if gs.PendingAbilityCheckNotified != true {
		t.Fatalf("Ожидали PendingAbilityCheckNotified=true после prompt, но false")
	}

	// Find the pending check prompt message (text-only)
	calls := fakeAPI.snapshotCalls()
	promptMsgID := 0
	for _, c := range calls {
		if c.Method == "sendMessage" && c.ChatID == chatID && strings.Contains(c.Text, "🎲 Проверка") {
			promptMsgID = c.MessageID
		}
	}
	if promptMsgID == 0 {
		t.Fatalf("Не удалось найти prompt с текстовой подсказкой ability check (msg_id=%d)", promptMsgID)
	}

	// Simulate /roll d20
	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/roll d20")); err != nil {
		t.Fatalf("/roll d20: %v", err)
	}

	// Verify pending cleared
	gs, err = cfg.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil || gs == nil {
		t.Fatalf("Не удалось получить сессию после callback: %v", err)
	}
	if gs.HasPendingAbilityCheck() {
		t.Fatalf("Pending ability check не очищен после /roll (id=%s ability=%s dc=%d)", gs.PendingAbilityCheckID, gs.PendingAbilityCheckAbility, gs.PendingAbilityCheckDC)
	}

	// Verify event persisted to history
	events, err := cfg.eventRepo.GetBySessionID(ctx, gs.ID, 20)
	if err != nil {
		t.Fatalf("Не удалось получить события: %v", err)
	}
	found := false
	for _, e := range events {
		if strings.Contains(e.Content, "🎲 Проверка") && strings.Contains(e.Content, "DC") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Не найдено событие проверки в истории (ожидали текст вида '🎲 Проверка ... (DC ...)')")
	}

	// Verify Telegram message sent with result
	calls = fakeAPI.snapshotCalls()
	foundResult := false
	for _, c := range calls {
		if c.Method == "sendMessage" && c.ChatID == chatID && c.MessageID != promptMsgID && strings.Contains(c.Text, "🎲 Проверка") {
			foundResult = true
			break
		}
	}
	if !foundResult {
		t.Fatalf("Бот не отправил сообщение с результатом ability check")
	}
}
