package integration

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	abilitycheck "dungeons-and-dragons-ai/internal/game/application/ability_check"
	achievementapp "dungeons-and-dragons-ai/internal/game/application/achievement"
	"dungeons-and-dragons-ai/internal/game/application/campaign"
	characterapp "dungeons-and-dragons-ai/internal/game/application/character"
	"dungeons-and-dragons-ai/internal/game/application/dm_analyzer"
	"dungeons-and-dragons-ai/internal/game/application/history"
	inventoryapp "dungeons-and-dragons-ai/internal/game/application/inventory"
	"dungeons-and-dragons-ai/internal/game/application/player_action"
	mapapp "dungeons-and-dragons-ai/internal/game/application/worldmap"
	"dungeons-and-dragons-ai/internal/game/infrastructure/persistence"
	"dungeons-and-dragons-ai/internal/llm/domain"
	llmtools "dungeons-and-dragons-ai/internal/llm/domain/tools"
	ragapp "dungeons-and-dragons-ai/internal/rag/application"
	telegrambot "dungeons-and-dragons-ai/internal/telegram"
)

// initCampaignStubLLM is a deterministic LLM stub for InitCampaign use-case.
// It answers campaign prompts with valid JSON and never touches the network.
type initCampaignStubLLM struct{}

func (l *initCampaignStubLLM) Generate(ctx context.Context, prompt string) (string, error) {
	_ = ctx

	switch {
	case strings.Contains(prompt, `"title"`) && strings.Contains(prompt, `"items"`):
		return `{
  "title": "Забытый артефакт",
  "description": "Герои ищут древний артефакт, чтобы остановить пробуждающееся зло. След ведёт через руины и подземелья.",
  "items": [
    { "name": "Ключ-руна", "purpose": "открывает запечатанные двери" },
    { "name": "Осколок обелиска", "purpose": "указывает путь к артефакту" }
  ]
}`, nil
	case strings.Contains(prompt, `"locations"`) && strings.Contains(prompt, "Создай список ключевых локаций"):
		// Keep descriptions short to satisfy strict prompt constraints.
		return `{
  "locations": [
    { "name": "Таверна \"У Дуба\"", "description": "место слухов и встреч" },
    { "name": "Руины Храма", "description": "разрушенное святилище в лесу" },
    { "name": "Катакомбы", "description": "подземные ходы под руинами" }
  ]
}`, nil
	case strings.Contains(prompt, `"connections"`) && strings.Contains(prompt, "Создай связи между локациями"):
		return `{
  "connections": {
    "Таверна \"У Дуба\"": [
      { "to_location": "Руины Храма", "direction": "north", "description": "лесная дорога" }
    ],
    "Руины Храма": [
      { "to_location": "Катакомбы", "direction": "down", "description": "лестница в подвал" },
      { "to_location": "Таверна \"У Дуба\"", "direction": "south", "description": "обратно к деревне" }
    ],
    "Катакомбы": [
      { "to_location": "Руины Храма", "direction": "up", "description": "к поверхности" }
    ]
  }
}`, nil
	case strings.Contains(prompt, `"predefined_checks"`) && strings.Contains(prompt, "предопределенные проверки"):
		return `{
  "predefined_checks": [
    {
      "ability": "wisdom",
      "dc": 12,
      "description": "Проверка Восприятия (DC 12) — заметить свежие следы",
      "location_hint": "у обломков колонн"
    }
  ]
}`, nil
	case strings.Contains(prompt, `"npcs"`) && strings.Contains(prompt, "Создай NPC"):
		return `{
  "npcs": [
    { "name": "Старик Норен", "role": "сказитель" }
  ]
}`, nil
	default:
		// Safe fallback: valid JSON to reduce repair/retry noise.
		return `{}`, nil
	}
}

func (l *initCampaignStubLLM) GenerateWithMaxTokens(ctx context.Context, prompt string, maxTokens int) (string, error) {
	_ = maxTokens
	return l.Generate(ctx, prompt)
}

func (l *initCampaignStubLLM) GenerateWithTools(ctx context.Context, prompt string, tools []llmtools.Tool) (*domain.LLMResponseWithTools, error) {
	_ = ctx
	_ = prompt
	_ = tools
	return &domain.LLMResponseWithTools{Content: "", Finished: true}, nil
}

// TestTelegramGameplay_BotSimulation_UserJourney_StubbedLLM runs a stable gameplay scenario
// "as if the user plays via Telegram", but without real LLM/RAG network calls.
func TestTelegramGameplay_BotSimulation_UserJourney_StubbedLLM(t *testing.T) {
	cfg := setupInfraOnlyIntegrationTest(t)
	if cfg == nil {
		return
	}
	defer cleanupTest(t, &testConfig{db: cfg.db, chatID: cfg.chatID, tgUserID: cfg.tgUserID})

	ctx := cfg.ctx
	chatID := cfg.chatID
	tgUserID := cfg.tgUserID

	var problems []string

	// Fake Telegram API server
	fakeAPI := newFakeTelegramAPI()
	srv := httptest.NewServer(fakeAPI.handler(t))
	defer srv.Close()
	apiEndpointFmt := strings.TrimRight(srv.URL, "/") + "/bot%s/%s"

	// Repos
	worldRepo := cfg.worldRepo
	sessionRepo := cfg.sessionRepo
	playerRepo := cfg.playerRepo
	eventRepo := cfg.eventRepo
	inventoryRepo := persistence.NewInventoryRepository(cfg.db)
	worldEventRepo := persistence.NewWorldEventRepository(cfg.db)

	// Use-cases (no real LLM / no real embeddings)
	initCampaignUC := campaign.NewInitCampaignUseCase(&initCampaignStubLLM{}, worldRepo)
	createCharacterUC := characterapp.NewCreateCharacterUseCase(sessionRepo, playerRepo, inventoryRepo)
	getHistoryUC := history.NewGetHistoryUseCase(sessionRepo, eventRepo)
	getInventoryUC := inventoryapp.NewGetInventoryUseCase(sessionRepo, inventoryRepo)

	// No-op indexer: HandleActionUseCase indexes unconditionally.
	indexDocUC := ragapp.NewIndexDocument(noopEmbedder{}, noopVectorStore{})

	// DM LLM не должен запрашивать проверки — их создаёт анализатор действий.
	actionLLM := noopDMLLM{}
	analyzeUC := dm_analyzer.NewAnalyzePlayerActionUseCase(
		analysisLLM{json: `{"needs_ability_check":true,"ability_check":{"ability":"dexterity","dc":13,"reason":"вскрытие замка","stakes":"если провал — сработает ловушка"},"needs_predefined_check":false,"needs_random_roll":false,"simple_action":false,"recommendation":""}`},
		eventRepo,
	)

	handleActionUC := player_action.NewHandleActionUseCase(
		actionLLM,
		sessionRepo,
		staticContextBuilder{},
		eventRepo,
		indexDocUC,
		nil, // combatRepo
		nil, // questRepo
		inventoryRepo,
		nil, // addExperienceUC
		nil, // checkWorldEventsUC
		nil, // checkAchievementsUC
		&achievementapp.NoOpNotificationService{},
		nil, // generateImageUC
		nil, // useSpellUC
		nil, // responseCache
		player_action.NewActionValidator(),
		nil,       // checkDailyProgressUC
		nil,       // getSubscriptionUC
		nil,       // updateRatingUC
		analyzeUC, // analyzePlayerActionUC
		nil,       // generateLocationEventUC
	)

	getMapUC := mapapp.NewGetMapUseCase(sessionRepo)
	// For tests, we need to pass nil for LLM and other dependencies
	moveToLocationUC := mapapp.NewMoveToLocationUseCase(nil, sessionRepo, worldEventRepo, nil, nil)

	performAbilityCheckUC := abilitycheck.NewPerformAbilityCheckUseCase(sessionRepo, eventRepo, nil)

	bot, err := telegrambot.NewBotWithAPIEndpoint(
		"TEST_TOKEN",
		apiEndpointFmt,
		initCampaignUC,
		handleActionUC,
		createCharacterUC,
		getHistoryUC,
		getInventoryUC,
		nil, // addItemUC
		nil, // handleCombatUC
		nil, // rollDiceUC
		nil, // getQuestsUC
		nil, // getDailyQuestsUC
		nil, // checkDailyProgressUC
		getMapUC,
		moveToLocationUC,
		nil, // getAchievementsUC
		nil, // getSpellsUC
		nil, // useSpellUC
		nil, // generateImageUC
		nil, // getSubscriptionUC
		nil, // checkLimitsUC
		nil, // getLeaderboardUC
		nil, // updateRatingUC
		performAbilityCheckUC,
		sessionRepo,
		playerRepo,
		nil, // combatRepo
		nil, // feedbackRepo
		eventRepo,
		nil, // indexDocUC (/roll)
	)
	if err != nil {
		t.Fatalf("Не удалось создать Telegram bot (fake API): %v", err)
	}

	// 0) /help
	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/help")); err != nil {
		problems = append(problems, fmt.Sprintf("/help: %v", err))
	}

	// 1) /newgame (stubbed LLM -> valid JSON)
	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/newgame классическое фэнтези")); err != nil {
		problems = append(problems, fmt.Sprintf("/newgame: %v", err))
	}
	gs, err := sessionRepo.GetByChatID(ctx, chatID)
	if err != nil || gs == nil || !gs.IsActive() {
		t.Fatalf("После /newgame не найдена активная сессия: err=%v session_nil=%v", err, gs == nil)
	}
	if gs.CurrentLocationID == nil {
		// Not fatal, but breaks /map navigation UX.
		problems = append(problems, "После /newgame не установлена CurrentLocationID (навигация по карте может не работать)")
	}

	// 2) /createcharacter
	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/createcharacter ТестовыйГерой elf rogue")); err != nil {
		problems = append(problems, fmt.Sprintf("/createcharacter: %v", err))
	}

	// 3) player action -> tool-first ability check prompt + /roll d20
	beforeCalls := fakeAPI.snapshotCalls()
	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "Пытаюсь вскрыть замок на сундуке")); err != nil {
		problems = append(problems, fmt.Sprintf("player action: %v", err))
	}

	calls := fakeAPI.snapshotCalls()
	startIdx := len(beforeCalls)
	if startIdx > len(calls) {
		startIdx = 0
	}
	promptFound := false
	for _, c := range calls[startIdx:] {
		if (c.Method == "sendMessage" || c.Method == "editMessageText") &&
			c.ChatID == chatID &&
			strings.Contains(c.Text, "🎲 Нужна проверка") &&
			strings.Contains(c.Text, "DC") {
			promptFound = true
			break
		}
	}
	if !promptFound {
		t.Fatalf("Не удалось найти player-facing подсказку про ability check (ожидали новый текст с '🎲 Нужна проверка' и 'DC' после действия игрока)")
	}
	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/roll d20")); err != nil {
		t.Fatalf("/roll d20: %v", err)
	}

	// 4) /map + navigate first button (if present)
	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/map")); err != nil {
		problems = append(problems, fmt.Sprintf("/map: %v", err))
	}

	calls = fakeAPI.snapshotCalls()
	navMsgID := 0
	navReplyMarkup := ""
	for _, c := range calls {
		if c.Method == "sendMessage" && strings.Contains(c.Text, "Куда идём дальше") && strings.TrimSpace(c.ReplyMarkup) != "" {
			navMsgID = c.MessageID
			navReplyMarkup = c.ReplyMarkup
		}
	}
	navCb, navOk := extractFirstCallbackData(navReplyMarkup)
	if navMsgID != 0 && navOk {
		if err := bot.HandleUpdate(ctx, makeCallbackUpdate(chatID, tgUserID, navMsgID, navCb)); err != nil {
			problems = append(problems, fmt.Sprintf("map navigation callback (%s): %v", navCb, err))
		}
	}

	// 5) /history should not leak tool markup to player
	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/history")); err != nil {
		problems = append(problems, fmt.Sprintf("/history: %v", err))
	}

	if leak := findToolLeak(fakeAPI.snapshotCalls(), chatID); leak != "" {
		t.Fatalf("Обнаружен tool-текст в player-facing сообщении: %s", leak)
	}

	// 6) /endgame
	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/endgame")); err != nil {
		problems = append(problems, fmt.Sprintf("/endgame: %v", err))
	}

	// Persist problems for human review
	if len(problems) > 0 {
		writeToTestingReport(problems)
	}
}
