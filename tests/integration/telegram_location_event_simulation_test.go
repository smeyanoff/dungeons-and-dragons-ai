package integration

import (
	"context"
	"strings"
	"testing"

	achievementapp "dungeons-and-dragons-ai/internal/game/application/achievement"
	characterapp "dungeons-and-dragons-ai/internal/game/application/character"
	dm_tools "dungeons-and-dragons-ai/internal/game/application/dm_tools"
	locationeventapp "dungeons-and-dragons-ai/internal/game/application/location_event"
	"dungeons-and-dragons-ai/internal/game/application/player_action"
	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/event"
	questdomain "dungeons-and-dragons-ai/internal/game/domain/quest"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	worlddomain "dungeons-and-dragons-ai/internal/game/domain/world"
	contextbuilder "dungeons-and-dragons-ai/internal/game/infrastructure/context"
	"dungeons-and-dragons-ai/internal/game/infrastructure/persistence"
	"dungeons-and-dragons-ai/internal/llm/domain"
	ragapp "dungeons-and-dragons-ai/internal/rag/application"
	telegrambot "dungeons-and-dragons-ai/internal/telegram"

	"net/http/httptest"
)

// recordingScriptedLLM is a deterministic LLM stub that:
// - returns scripted DM responses via GenerateWithTools (records prompts)
// - returns scripted analyzer JSON via Generate (for dm_analyzer)
type recordingScriptedLLM struct {
	dmResponses      []domain.LLMResponseWithTools
	analyzerJSON     []string
	generateCalls    int
	generateWithTool int

	prompts []string
}

func (l *recordingScriptedLLM) Generate(ctx context.Context, prompt string) (string, error) {
	_ = ctx
	_ = prompt
	l.generateCalls++
	if len(l.analyzerJSON) == 0 {
		// Safe default: valid empty analysis.
		return `{"combat_detected":false,"enemies":[],"quest_completed":false,"quest_failed":false,"quest_title":"","experience_gained":0,"experience_reason":"","items_received":[],"location_visited":null,"npc_met":null,"generated_images":[]}`, nil
	}
	idx := l.generateCalls - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(l.analyzerJSON) {
		idx = len(l.analyzerJSON) - 1
	}
	return l.analyzerJSON[idx], nil
}

func (l *recordingScriptedLLM) GenerateWithMaxTokens(ctx context.Context, prompt string, maxTokens int) (string, error) {
	_ = maxTokens
	return l.Generate(ctx, prompt)
}

func (l *recordingScriptedLLM) GenerateWithTools(ctx context.Context, prompt string, tools []dm_tools.Tool) (*domain.LLMResponseWithTools, error) {
	_ = ctx
	_ = tools
	l.prompts = append(l.prompts, prompt)
	l.generateWithTool++

	if len(l.dmResponses) == 0 {
		resp := domain.LLMResponseWithTools{Content: "OK", Finished: true}
		return &resp, nil
	}

	idx := l.generateWithTool - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(l.dmResponses) {
		idx = len(l.dmResponses) - 1
	}
	resp := l.dmResponses[idx]
	return &resp, nil
}

// TestTelegramGameplay_BotSimulation_LocationEvent_FirstVisit detects whether the "location event" mechanic
// is actually wired into the player's Telegram gameplay loop.
//
// Expectations:
// - When DM Analyzer marks a location as first-visited, a location event is generated and saved in world_events.
// - Ideally, this location event should become visible to DM on subsequent turns (via context/history/RAG).
//
// This test will not fail the build if the event is not surfaced to DM yet; it will record the issue
// in TESTING_REPORT.md to keep the integration gap visible.
func TestTelegramGameplay_BotSimulation_LocationEvent_FirstVisit(t *testing.T) {
	cfg := setupInfraOnlyIntegrationTest(t)
	if cfg == nil {
		return
	}
	defer cleanupTest(t, &testConfig{db: cfg.db, chatID: cfg.chatID, tgUserID: cfg.tgUserID})

	ctx := cfg.ctx
	chatID := cfg.chatID
	tgUserID := cfg.tgUserID

	// Prepare deterministic world + session (2 locations so we can "visit" Cave).
	q := &questdomain.Quest{Title: "Test Quest (LocationEvent)", Description: "Test quest for location event flow"}
	w := worlddomain.New("Test World (LocationEvent)")
	w.Description = "Deterministic test world for location event bot simulation"
	w.SetMainQuest(q)
	w.Locations = []worlddomain.Location{
		{Name: "Start", Description: "Start location"},
		{Name: "Cave", Description: "A dark cave"},
	}
	if err := cfg.worldRepo.Save(ctx, w); err != nil {
		t.Fatalf("Не удалось сохранить тестовый мир: %v", err)
	}
	if len(w.Locations) < 2 || w.Locations[0].ID == 0 || w.Locations[1].ID == 0 {
		t.Fatalf("Ожидали сохраненные локации с ID, получили: %+v", w.Locations)
	}
	caveID := w.Locations[1].ID

	gs := &session.GameSession{
		ChatID:            chatID,
		State:             session.StateActive,
		World:             *w,
		WorldID:           w.ID,
		CurrentLocationID: &caveID,
	}
	if err := cfg.sessionRepo.Save(ctx, gs); err != nil {
		t.Fatalf("Не удалось сохранить сессию: %v", err)
	}

	// Create character (no real LLM).
	createCharacterUC := characterapp.NewCreateCharacterUseCase(cfg.sessionRepo, cfg.playerRepo)
	if _, err := createCharacterUC.Execute(ctx, characterapp.CreateCharacterRequest{
		ChatID: chatID,
		Name:   "ТестовыйГерой",
		Race:   character.RaceElf,
		Class:  character.ClassRogue,
	}); err != nil {
		t.Fatalf("Не удалось создать персонажа: %v", err)
	}

	// World events repo + location event generator.
	worldEventRepo := persistence.NewWorldEventRepository(cfg.db)
	locationEventGenerator := locationeventapp.NewLocationEventGenerator(worldEventRepo)

	// No-op RAG indexer to satisfy HandleActionUseCase contract (it indexes StoryEvents unconditionally).
	indexDocUC := ragapp.NewIndexDocument(noopEmbedder{}, noopVectorStore{})
	retrieveUC := ragapp.NewRetrieveContext(noopEmbedder{}, noopVectorStore{})
	contextBuilder := contextbuilder.NewRAGContextBuilder(
		contextbuilder.NewSimpleContextBuilder(),
		retrieveUC,
		cfg.eventRepo,
		nil, // inventoryRepo
		nil, // combatRepo
	)
	contextBuilder.SetWorldEventRepository(worldEventRepo)

	// LLM stub: first DM response triggers analyzer "first visit to Cave", second is a normal response.
	llm := &recordingScriptedLLM{
		dmResponses: []domain.LLMResponseWithTools{
			{Content: "Ты входишь в Cave. Здесь сыро и темно.", Finished: true},
			{Content: "Ты оглядываешься вокруг и слышишь капли воды.", Finished: true},
		},
		analyzerJSON: []string{
			`{"combat_detected":false,"enemies":[],"quest_completed":false,"quest_failed":false,"quest_title":"","experience_gained":0,"experience_reason":"","items_received":[],"location_visited":{"name":"Cave","description":"A dark cave","is_first_visit":true},"npc_met":null,"generated_images":[]}`,
			`{"combat_detected":false,"enemies":[],"quest_completed":false,"quest_failed":false,"quest_title":"","experience_gained":0,"experience_reason":"","items_received":[],"location_visited":null,"npc_met":null,"generated_images":[]}`,
		},
	}

	handleActionUC := player_action.NewHandleActionUseCase(
		llm,
		cfg.sessionRepo,
		contextBuilder,
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
		locationEventGenerator,
	)

	// Fake Telegram API server + bot.
	fakeAPI := newFakeTelegramAPI()
	srv := httptest.NewServer(fakeAPI.handler(t))
	defer srv.Close()
	apiEndpointFmt := strings.TrimRight(srv.URL, "/") + "/bot%s/%s"

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
		nil, // performAbilityCheckUC
		cfg.sessionRepo,
		nil, // playerRepo
		nil, // combatRepo
		nil, // feedbackRepo
		nil, // eventRepo (/roll history)
		nil, // indexDocUC (/roll RAG)
	)
	if err != nil {
		t.Fatalf("Не удалось создать Telegram bot (fake API): %v", err)
	}

	// 1) Player action => DM response => analyzer marks first visit => location event should be created in world_events.
	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "Я захожу в пещеру.")); err != nil {
		t.Fatalf("player action 1: %v", err)
	}

	locEvents, err := worldEventRepo.GetByLocationID(ctx, caveID)
	if err != nil {
		t.Fatalf("GetByLocationID: %v", err)
	}
	if len(locEvents) == 0 {
		t.Fatalf("Ожидали сгенерированное событие локации в world_events (location_id=%d), но событий нет", caveID)
	}
	created := locEvents[0]

	// 2) Next player action: location event should be visible to DM via history/context.
	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "Что я вижу вокруг?")); err != nil {
		t.Fatalf("player action 2: %v", err)
	}

	var secondPrompt string
	if len(llm.prompts) >= 2 {
		secondPrompt = llm.prompts[1]
	}
	if secondPrompt == "" {
		t.Fatalf("не удалось захватить второй prompt для DM (ожидали 2 вызова GenerateWithTools)")
	}
	if !strings.Contains(secondPrompt, created.Name) && !strings.Contains(secondPrompt, created.Description) {
		t.Fatalf("LocationEvent: событие локации создано (world_event_id=%d, location_id=%d, name=%q), но не найдено в следующем DM prompt", created.ID, caveID, created.Name)
	}

	// Also check whether a StoryEvent was created that mentions the location event (common integration path).
	gs2, _ := cfg.sessionRepo.GetByChatID(ctx, chatID)
	if gs2 != nil {
		evs, err := cfg.eventRepo.GetBySessionID(ctx, gs2.ID, 50)
		if err == nil {
			found := false
			for _, e := range evs {
				if e.AuthorType == event.AuthorTypeDM || e.AuthorType == event.AuthorTypeNPC || e.AuthorType == event.AuthorTypePlayer {
					if strings.Contains(e.Content, created.Name) || strings.Contains(e.Content, created.Description) {
						found = true
						break
					}
				}
			}
			if !found {
				t.Fatalf("LocationEvent: событие локации есть в world_events (id=%d, location_id=%d), но не найдено ни в одном StoryEvent (history)", created.ID, caveID)
			}
		}
	}
}
