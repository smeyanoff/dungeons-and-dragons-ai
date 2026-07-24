package integration

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	abilitycheck "dungeons-and-dragons-ai/internal/game/application/ability_check"
	mapapp "dungeons-and-dragons-ai/internal/game/application/worldmap"
	"dungeons-and-dragons-ai/internal/game/domain/combat"
	"dungeons-and-dragons-ai/internal/game/infrastructure/persistence"
	telegrambot "dungeons-and-dragons-ai/internal/telegram"
)

// TestTelegramGameplay_RealLLM_SingleCampaign_ToFirstCombat
// One stable "as if via Telegram" journey using:
// - mock Telegram API (capturing outbound messages)
// - real LLM + real RAG (Postgres + Qdrant required)
//
// Goal: from world creation -> character -> first ability check prompt -> roll -> move to another location -> first combat.
func TestTelegramGameplay_RealLLM_SingleCampaign_ToFirstCombat(t *testing.T) {
	cfg := setupTelegramGameplayTest(t)
	defer cleanupTest(t, cfg.testConfig)

	ctx := cfg.ctx
	chatID := cfg.chatID
	tgUserID := cfg.tgUserID

	var problems []string

	// Fake Telegram API server to capture messages.
	fakeAPI := newFakeTelegramAPI()
	srv := httptest.NewServer(fakeAPI.handler(t))
	defer srv.Close()
	apiEndpointFmt := strings.TrimRight(srv.URL, "/") + "/bot%s/%s"

	// Repos we need for assertions / deterministic combat setup.
	combatRepo := persistence.NewCombatRepository(cfg.db)
	eventRepo := persistence.NewGameEventRepository(cfg.db)
	feedbackRepo := persistence.NewFeedbackRepository(cfg.db)
	playerRepo := persistence.NewPlayerRepository(cfg.db)
	worldEventRepo := persistence.NewWorldEventRepository(cfg.db)
	llmLogRepo := persistence.NewLLMLogRepository(cfg.db)
	moveToLocationUC := mapapp.NewMoveToLocationUseCase(nil, cfg.sessionRepo, worldEventRepo, nil, nil)
	performAbilityCheckUC := abilitycheck.NewPerformAbilityCheckUseCase(cfg.sessionRepo, eventRepo, cfg.indexDocUC)

	// Bot with real dependencies.
	bot, err := telegrambot.NewBotWithAPIEndpoint(
		"TEST_TOKEN",
		apiEndpointFmt,
		cfg.initCampaignUC,
		cfg.handleActionUC,
		cfg.createCharacterUC,
		cfg.getHistoryUC,
		cfg.getInventoryUC,
		cfg.addItemUC,
		cfg.handleCombatUC,
		cfg.rollDiceUC,
		cfg.getQuestsUC,
		cfg.getDailyQuestsUC,
		cfg.checkDailyProgressUC,
		cfg.getMapUC,
		moveToLocationUC,
		cfg.getAchievementsUC,
		cfg.getSpellsUC,
		cfg.useSpellUC,
		nil, // generateImageUC - disabled for stability/cost
		nil, // getSubscriptionUC
		nil, // checkLimitsUC
		cfg.getLeaderboardUC,
		cfg.updateRatingUC,
		performAbilityCheckUC,
		cfg.sessionRepo,
		playerRepo,
		combatRepo,
		feedbackRepo,
		eventRepo,
		cfg.indexDocUC,
	)
	if err != nil {
		t.Fatalf("Не удалось создать Telegram бота: %v", err)
	}

	// 1) /newgame
	if err := cfg.waitForRateLimit(ctx); err != nil {
		problems = append(problems, fmt.Sprintf("rate limit перед /newgame: %v", err))
	}
	theme := "темное фэнтези: руины, проклятия, подземелья, вампиры; фокус на исследовании и первом бою"
	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/newgame "+theme)); err != nil {
		t.Fatalf("/newgame: %v", err)
	}
	if gs, err := cfg.sessionRepo.GetByChatID(ctx, chatID); err != nil || gs == nil || !gs.IsActive() {
		t.Fatalf("после /newgame не найдена активная сессия: err=%v session_nil=%v", err, gs == nil)
	}
	if dm := lastNonThinkingPlayerFacingText(fakeAPI.snapshotCalls(), chatID); len([]rune(dm)) < 120 {
		problems = append(problems, fmt.Sprintf("слишком короткий ответ DM на /newgame (%d символов)", len([]rune(dm))))
	}

	// 2) /createcharacter
	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/createcharacter Эребос human rogue")); err != nil {
		t.Fatalf("/createcharacter: %v", err)
	}

	// 3) Player action -> expect ability check prompt (analyzer-first)
	if err := cfg.waitForRateLimit(ctx); err != nil {
		problems = append(problems, fmt.Sprintf("rate limit перед действием: %v", err))
	}
	before := fakeAPI.snapshotCalls()
	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "Пытаюсь вскрыть запертую дверь в соседнюю комнату, работаю отмычками тихо и осторожно")); err != nil {
		t.Fatalf("player action: %v", err)
	}
	after := fakeAPI.snapshotCalls()
	startIdx := len(before)
	if startIdx > len(after) {
		startIdx = 0
	}
	foundPrompt := false
	for _, c := range after[startIdx:] {
		if c.ChatID == chatID && (c.Method == "sendMessage" || c.Method == "editMessageText") &&
			strings.Contains(c.Text, "🎲 Нужна проверка") && strings.Contains(c.Text, "DC") && strings.Contains(c.Text, "/roll") {
			foundPrompt = true
			break
		}
	}
	if !foundPrompt {
		t.Fatalf("не нашли player-facing prompt ability check после действия игрока")
	}

	// 4) /roll d20 -> pending should be cleared
	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/roll d20")); err != nil {
		t.Fatalf("/roll d20: %v", err)
	}
	gs, err := cfg.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil || gs == nil {
		t.Fatalf("не удалось получить сессию после /roll: %v", err)
	}
	if gs.HasPendingAbilityCheck() {
		problems = append(problems, fmt.Sprintf("pending ability check не очищен после /roll (id=%s ability=%s dc=%d)", gs.PendingAbilityCheckID, gs.PendingAbilityCheckAbility, gs.PendingAbilityCheckDC))
	}

	// 5) /map + navigate to another location via callback (if possible)
	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/map")); err != nil {
		t.Fatalf("/map: %v", err)
	}
	gs, _ = cfg.sessionRepo.GetByChatID(ctx, chatID)
	var fromLoc uint
	if gs != nil && gs.CurrentLocationID != nil {
		fromLoc = *gs.CurrentLocationID
	}
	calls := fakeAPI.snapshotCalls()
	navMsgID := 0
	navReplyMarkup := ""
	for _, c := range calls {
		if c.Method == "sendMessage" && c.ChatID == chatID &&
			strings.Contains(c.Text, "Куда идём дальше") && strings.TrimSpace(c.ReplyMarkup) != "" {
			navMsgID = c.MessageID
			navReplyMarkup = c.ReplyMarkup
		}
	}
	cbData, ok := extractFirstCallbackData(navReplyMarkup)
	if navMsgID == 0 || !ok {
		t.Fatalf("не удалось найти inline navigation для /map (msg_id=%d ok=%v)", navMsgID, ok)
	}
	if err := bot.HandleUpdate(ctx, makeCallbackUpdate(chatID, tgUserID, navMsgID, cbData)); err != nil {
		t.Fatalf("map navigation callback (%s): %v", cbData, err)
	}
	gs, _ = cfg.sessionRepo.GetByChatID(ctx, chatID)
	if gs == nil || gs.CurrentLocationID == nil {
		t.Fatalf("после навигации по /map не установлена CurrentLocationID")
	}
	if fromLoc != 0 && *gs.CurrentLocationID == fromLoc {
		problems = append(problems, "переход по /map callback не изменил CurrentLocationID (возможно, только одна достижимая локация)")
	}

	// 6) Ensure we have an active combat in the *new* location. To keep this test stable and not depend
	// on LLM deciding to start combat, we create a deterministic combat in DB.
	activeCombat, _ := combatRepo.GetActiveBySessionID(ctx, gs.ID)
	if activeCombat == nil {
		p := gs.FindPlayerByTgUserID(tgUserID)
		if p == nil {
			p = gs.GetFirstPlayer()
		}
		if p == nil || p.CharacterID == 0 {
			t.Fatalf("не нашли персонажа для боя")
		}
		charID := p.CharacterID
		c := &combat.Combat{
			GameSessionID: gs.ID,
			State:         combat.CombatStateNotStarted,
			Participants: []combat.CombatParticipant{
				{
					IsPlayer:    true,
					CharacterID: &charID,
					Character:   &p.Character,
				},
				{
					IsPlayer:           false,
					MonsterName:        "Гоблин-разведчик",
					MonsterHP:          10,
					MonsterMaxHP:       10,
					MonsterAC:          12,
					MonsterAttackBonus: 3,
				},
			},
		}
		if err := c.Start(); err != nil {
			t.Fatalf("не удалось стартовать бой: %v", err)
		}
		if err := combatRepo.Save(ctx, c); err != nil {
			t.Fatalf("не удалось сохранить бой: %v", err)
		}
	}

	// 7) /battlefield + /attack (as-if Telegram)
	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/battlefield table")); err != nil {
		t.Fatalf("/battlefield: %v", err)
	}
	if msg := lastNonThinkingPlayerFacingText(fakeAPI.snapshotCalls(), chatID); !strings.Contains(msg, "Поле боя") {
		problems = append(problems, "после /battlefield не найдено сообщение с 'Поле боя'")
	}
	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/attack")); err != nil {
		t.Fatalf("/attack: %v", err)
	}

	// 8) LLM prompt/context/tools analysis via llm_logs (monitored LLM in setupIntegrationTest).
	// Save happens async, give it a small window.
	logs, err := waitForLLMLogs(t, llmLogRepo, chatID, 50, 2*time.Second)
	if err != nil {
		problems = append(problems, fmt.Sprintf("не удалось получить llm_logs по chat_id: %v", err))
	} else if len(logs) == 0 {
		problems = append(problems, "не найдено ни одного llm_logs по chat_id (ожидали мониторинг промптов)")
	} else {
		// Целевой флоу — analyzer-first: request_ability_check не регистрируется как tool для DM
		// (см. createToolRegistry в handle_action.go), проверка навыка создаётся системой напрямую
		// по флагу needs_ability_check из dm_analyzer.AnalyzePlayerActionUseCase. Player-facing
		// prompt проверки уже проверен выше (шаг 3, foundPrompt) — это и есть подтверждение флоу,
		// а не наличие tool-вызова request_ability_check в llm_logs. Здесь проверяем только то,
		// что tools вообще используются DM (combat/inventory/etc — см. /attack на шаге 7).
		withTools := 0
		for _, l := range logs {
			if l != nil && l.HasTools {
				withTools++
			}
		}
		if withTools == 0 {
			problems = append(problems, fmt.Sprintf("llm_logs есть (%d), но нет ни одного запроса с tools", len(logs)))
		}
	}

	if len(problems) > 0 {
		writeToTestingReport(problems)
	}
}
