package integration

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	abilitycheck "dungeons-and-dragons-ai/internal/game/application/ability_check"
	mapapp "dungeons-and-dragons-ai/internal/game/application/worldmap"
	combatdomain "dungeons-and-dragons-ai/internal/game/domain/combat"
	"dungeons-and-dragons-ai/internal/game/infrastructure/persistence"
	telegrambot "dungeons-and-dragons-ai/internal/telegram"
)

// TestTelegramGameplay_RealLLM_UserJourney_MainMechanics runs an end-to-end "as if user plays via Telegram"
// scenario with real LLM for /newgame + player action, plus deterministic checks for:
// - pending ability check resolution via /roll d20
// - combat UX via /battlefield + /attack
// - core commands (/inventory, /quests, /daily, /achievements, /spells, /map, /history, /endgame)
// - no tool/internal marker leaks to player-facing messages
func TestTelegramGameplay_RealLLM_UserJourney_MainMechanics(t *testing.T) {
	cfg := setupTelegramGameplayTest(t)
	defer cleanupTest(t, cfg.testConfig)

	ctx := cfg.ctx
	chatID := cfg.chatID
	tgUserID := cfg.tgUserID

	var problems []string
	var llmFeedback []string

	// Fake Telegram API server
	fakeAPI := newFakeTelegramAPI()
	srv := httptest.NewServer(fakeAPI.handler(t))
	defer srv.Close()
	apiEndpointFmt := strings.TrimRight(srv.URL, "/") + "/bot%s/%s"

	// Bot repos/use-cases not exposed on testConfig.
	eventRepo := persistence.NewGameEventRepository(cfg.db)
	combatRepo := persistence.NewCombatRepository(cfg.db)
	feedbackRepo := persistence.NewFeedbackRepository(cfg.db)
	playerRepo := persistence.NewPlayerRepository(cfg.db)
	worldEventRepo := persistence.NewWorldEventRepository(cfg.db)
	// For tests, we need to pass nil for LLM and other dependencies
	moveToLocationUC := mapapp.NewMoveToLocationUseCase(nil, cfg.sessionRepo, worldEventRepo, nil, nil)
	performAbilityCheckUC := abilitycheck.NewPerformAbilityCheckUseCase(cfg.sessionRepo, eventRepo, nil)

	// IMPORTANT: to avoid extra (costly) model calls, we pass generateImageUC=nil and indexDocUC=nil in bot.
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
		nil, // generateImageUC
		nil, // getSubscriptionUC
		nil, // checkLimitsUC
		nil, // getLeaderboardUC
		nil, // updateRatingUC
		performAbilityCheckUC,
		cfg.sessionRepo,
		playerRepo,
		combatRepo,
		feedbackRepo,
		eventRepo,
		nil, // indexDocUC (avoid embeddings calls from /roll)
	)
	if err != nil {
		t.Fatalf("Не удалось создать Telegram bot (fake API): %v", err)
	}

	// 0) /help
	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/help")); err != nil {
		problems = append(problems, fmt.Sprintf("/help: %v", err))
	}

	// 1) /newgame (real LLM)
	if err := cfg.waitForRateLimit(ctx); err != nil {
		problems = append(problems, fmt.Sprintf("rate limiter (before /newgame): %v", err))
	}
	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/newgame классическое фэнтези")); err != nil {
		problems = append(problems, fmt.Sprintf("/newgame: %v", err))
		t.Fatalf("/newgame: %v", err)
	}

	gs, err := cfg.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil || gs == nil || !gs.IsActive() {
		t.Fatalf("После /newgame не найдена активная сессия: err=%v session_nil=%v", err, gs == nil)
	}
	if gs.CurrentLocationID == nil {
		problems = append(problems, "После /newgame не установлена CurrentLocationID (навигация по /map может не работать)")
	}

	// 2) /createcharacter
	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/createcharacter ТестовыйГерой elf rogue")); err != nil {
		problems = append(problems, fmt.Sprintf("/createcharacter: %v", err))
		t.Fatalf("/createcharacter: %v", err)
	}

	// 3) Player action (real LLM) + basic quality heuristics
	if err := cfg.waitForRateLimit(ctx); err != nil {
		problems = append(problems, fmt.Sprintf("rate limiter (before player action): %v", err))
	}
	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "Осматриваю место вокруг и ищу следы опасности")); err != nil {
		problems = append(problems, fmt.Sprintf("player action: %v", err))
		t.Fatalf("player action: %v", err)
	}
	if dmText := lastNonThinkingPlayerFacingText(fakeAPI.snapshotCalls(), chatID); dmText != "" && len([]rune(dmText)) < 80 {
		llmFeedback = append(llmFeedback, fmt.Sprintf("Ответ DM слишком короткий (%d символов): %s", len([]rune(dmText)), dmText))
	}

	// 4) Pending ability check -> /roll d20 should resolve it (tool-first UX piece must be deterministic).
	gs, err = cfg.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil || gs == nil {
		t.Fatalf("Сессия не найдена перед pending check: %v", err)
	}
	gs.SetPendingAbilityCheck("pending_user_journey_1", "wisdom", 12)
	if err := cfg.sessionRepo.Save(ctx, gs); err != nil {
		t.Fatalf("Не удалось сохранить pending ability check: %v", err)
	}
	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/roll d20")); err != nil {
		problems = append(problems, fmt.Sprintf("/roll d20 (resolve pending): %v", err))
		t.Fatalf("/roll d20: %v", err)
	}
	gs, _ = cfg.sessionRepo.GetByChatID(ctx, chatID)
	if gs == nil || gs.HasPendingAbilityCheck() {
		problems = append(problems, "Pending ability check не очищен после /roll d20")
	}

	// 5) Deterministic combat UX: create active combat and use /battlefield + /attack.
	gs, err = cfg.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil || gs == nil {
		t.Fatalf("Сессия не найдена перед боем: %v", err)
	}
	player := gs.GetFirstPlayer()
	if player == nil || player.Character.ID == 0 {
		t.Fatalf("Персонаж не найден перед боем")
	}
	playerCharID := player.Character.ID
	combat := &combatdomain.Combat{
		GameSessionID: gs.ID,
		State:         combatdomain.CombatStateNotStarted,
		Participants: []combatdomain.CombatParticipant{
			{IsPlayer: true, CharacterID: &playerCharID, Character: &player.Character},
			{
				IsPlayer:           false,
				MonsterName:        "Goblin",
				MonsterHP:          10,
				MonsterMaxHP:       10,
				MonsterAC:          12,
				MonsterAttackBonus: 2,
			},
		},
	}
	if err := combat.Start(); err != nil {
		t.Fatalf("Не удалось стартовать бой: %v", err)
	}
	if err := combatRepo.Save(ctx, combat); err != nil {
		t.Fatalf("Не удалось сохранить бой: %v", err)
	}

	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/battlefield table")); err != nil {
		problems = append(problems, fmt.Sprintf("/battlefield: %v", err))
	}
	last := lastNonThinkingPlayerFacingText(fakeAPI.snapshotCalls(), chatID)
	if last == "" || !strings.Contains(last, "Поле боя") {
		problems = append(problems, "После /battlefield не удалось получить player-facing сообщение с полем боя")
	}
	if last != "" && !strings.Contains(last, "Goblin") {
		problems = append(problems, fmt.Sprintf("Поле боя не содержит врага (ожидали Goblin): %s", last))
	}

	// Snapshot before attack to see progress.
	activeBefore, _ := combatRepo.GetActiveBySessionID(ctx, gs.ID)
	hpBefore := 0
	if activeBefore != nil {
		for _, p := range activeBefore.Participants {
			if !p.IsPlayer && strings.EqualFold(p.MonsterName, "Goblin") {
				hpBefore = p.MonsterHP
			}
		}
	}

	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/attack мечом")); err != nil {
		problems = append(problems, fmt.Sprintf("/attack: %v", err))
	}

	_ = waitForCondition(t, 500*time.Millisecond, 25*time.Millisecond, func() bool {
		active, _ := combatRepo.GetActiveBySessionID(ctx, gs.ID)
		if active == nil {
			return true
		}
		hpAfter := hpBefore
		for _, p := range active.Participants {
			if !p.IsPlayer && strings.EqualFold(p.MonsterName, "Goblin") {
				hpAfter = p.MonsterHP
			}
		}
		return hpBefore > 0 && hpAfter != hpBefore
	})

	activeAfter, _ := combatRepo.GetActiveBySessionID(ctx, gs.ID)
	if activeAfter == nil {
		// Combat may have ended in one hit; that's OK.
	} else {
		hpAfter := hpBefore
		for _, p := range activeAfter.Participants {
			if !p.IsPlayer && strings.EqualFold(p.MonsterName, "Goblin") {
				hpAfter = p.MonsterHP
			}
		}
		// Heuristic: after an attack, something should change (HP or combat log/turn).
		if hpBefore > 0 && hpAfter == hpBefore {
			problems = append(problems, fmt.Sprintf("После /attack HP врага не изменился (Goblin HP=%d)", hpAfter))
		}
	}

	// 6) Core commands (should not error)
	for _, cmd := range []string{"/inventory", "/quests", "/daily", "/achievements", "/spells", "/abilities", "/map", "/history", "/endgame"} {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, cmd)); err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", cmd, err))
		}
	}

	// 7) No tool/internal markers leak to player-facing messages.
	if leak := findToolLeak(fakeAPI.snapshotCalls(), chatID); leak != "" {
		t.Fatalf("Обнаружен tool-текст в player-facing сообщении: %s", leak)
	}

	if len(problems) > 0 {
		writeToTestingReport(problems)
	}
	if len(llmFeedback) > 0 {
		writeToFeedback(llmFeedback)
	}
}

func lastNonThinkingPlayerFacingText(calls []tgCapturedCall, chatID int64) string {
	for i := len(calls) - 1; i >= 0; i-- {
		c := calls[i]
		if c.ChatID != chatID {
			continue
		}
		if c.Method != "sendMessage" && c.Method != "editMessageText" {
			continue
		}
		text := strings.TrimSpace(c.Text)
		if text == "" || text == "🤔 Думаю..." {
			continue
		}
		return text
	}
	return ""
}
