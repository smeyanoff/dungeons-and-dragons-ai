package integration

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	abilitycheck "dungeons-and-dragons-ai/internal/game/application/ability_check"
	mapapp "dungeons-and-dragons-ai/internal/game/application/worldmap"
	"dungeons-and-dragons-ai/internal/game/infrastructure/persistence"
	telegrambot "dungeons-and-dragons-ai/internal/telegram"
)

// TestTelegramGameplay_RealLLM_PendingAbilityCheck_ManualAndRoll verifies the "tool-first" UX pieces
// that must be deterministic even with real LLM:
// - pending ability check exists in session
// - user can resolve it by sending a number (offline dice) OR via /roll d20
// - pending is cleared and a StoryEvent is created
// - no tool/internal markers leak to player-facing messages
//
// We intentionally avoid relying on the LLM to request the check (non-deterministic);
// instead, we create a pending check at the session level (as if tools requested it),
// then validate Telegram-side behavior end-to-end.
func TestTelegramGameplay_RealLLM_PendingAbilityCheck_ManualAndRoll(t *testing.T) {
	cfg := setupTelegramGameplayTest(t)
	defer cleanupTest(t, cfg.testConfig)

	ctx := cfg.ctx
	chatID := cfg.chatID
	tgUserID := cfg.tgUserID

	// Fake Telegram API server
	fakeAPI := newFakeTelegramAPI()
	srv := httptest.NewServer(fakeAPI.handler(t))
	defer srv.Close()
	apiEndpointFmt := strings.TrimRight(srv.URL, "/") + "/bot%s/%s"

	// Bot dependencies that are not exposed on testConfig.
	eventRepo := persistence.NewGameEventRepository(cfg.db)
	combatRepo := persistence.NewCombatRepository(cfg.db)
	feedbackRepo := persistence.NewFeedbackRepository(cfg.db)
	playerRepo := persistence.NewPlayerRepository(cfg.db)
	worldEventRepo := persistence.NewWorldEventRepository(cfg.db)
	// For tests, we need to pass nil for LLM and other dependencies
	inventoryRepo := persistence.NewInventoryRepository(cfg.db)
	moveToLocationUC := mapapp.NewMoveToLocationUseCase(nil, cfg.sessionRepo, worldEventRepo, nil, nil, inventoryRepo)

	performAbilityCheckUC := abilitycheck.NewPerformAbilityCheckUseCase(cfg.sessionRepo, eventRepo, nil)

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
		nil, // generateImageUC (avoid extra model calls in tests)
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
		nil, // indexDocUC (avoid embeddings in tests)
		nil, // deleteSessionDataUC
	)
	if err != nil {
		t.Fatalf("Не удалось создать Telegram bot (fake API): %v", err)
	}

	// 1) /newgame (real LLM)
	if err := cfg.waitForRateLimit(ctx); err != nil {
		t.Fatalf("rate limiter: %v", err)
	}
	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/newgame классическое фэнтези")); err != nil {
		t.Fatalf("/newgame: %v", err)
	}

	gs, err := cfg.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil || gs == nil || !gs.IsActive() {
		t.Fatalf("После /newgame не найдена активная сессия: err=%v session_nil=%v", err, gs == nil)
	}

	// 2) /createcharacter
	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/createcharacter ТестовыйГерой elf rogue")); err != nil {
		t.Fatalf("/createcharacter: %v", err)
	}

	// 3) Set pending ability check (as if tools requested it).
	gs, _ = cfg.sessionRepo.GetByChatID(ctx, chatID)
	if gs == nil {
		t.Fatalf("Сессия не найдена перед pending check")
	}
	gs.SetPendingAbilityCheck("pending_test_1", "dexterity", 12)
	if err := cfg.sessionRepo.Save(ctx, gs); err != nil {
		t.Fatalf("Не удалось сохранить pending ability check: %v", err)
	}

	// 4) Resolve via manual input (offline dice): "17"
	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "17")); err != nil {
		t.Fatalf("manual ability check input: %v", err)
	}
	gs, _ = cfg.sessionRepo.GetByChatID(ctx, chatID)
	if gs == nil || gs.HasPendingAbilityCheck() {
		t.Fatalf("Pending ability check не очищен после ручного ввода результата")
	}

	// 5) Resolve via /roll d20 (auto roll)
	gs.SetPendingAbilityCheck("pending_test_2", "wisdom", 10)
	if err := cfg.sessionRepo.Save(ctx, gs); err != nil {
		t.Fatalf("Не удалось сохранить pending ability check #2: %v", err)
	}
	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/roll d20")); err != nil {
		t.Fatalf("/roll d20: %v", err)
	}
	gs, _ = cfg.sessionRepo.GetByChatID(ctx, chatID)
	if gs == nil || gs.HasPendingAbilityCheck() {
		t.Fatalf("Pending ability check не очищен после /roll d20")
	}

	// 6) Verify at least 2 ability-check events were written to history.
	evs, err := eventRepo.GetBySessionID(context.Background(), gs.ID, 50)
	if err != nil {
		t.Fatalf("GetBySessionID: %v", err)
	}
	checkEvents := 0
	for _, e := range evs {
		if strings.Contains(e.Content, "🎲 Проверка") && strings.Contains(strings.ToLower(e.Content), "dc") {
			checkEvents++
		}
	}
	if checkEvents < 2 {
		t.Fatalf("Ожидали >=2 события '🎲 Проверка ... (DC ...)' в истории, получили %d", checkEvents)
	}

	// 7) Verify no tool/internal markers leak to player-facing messages.
	if leak := findToolLeak(fakeAPI.snapshotCalls(), chatID); leak != "" {
		t.Fatalf("Обнаружен tool-текст в player-facing сообщении: %s", leak)
	}

	// 8) Sanity: player-facing output contains ability check messages.
	calls := fakeAPI.snapshotCalls()
	found := 0
	for _, c := range calls {
		if c.ChatID != chatID || c.Method != "sendMessage" {
			continue
		}
		if strings.Contains(c.Text, "🎲 Проверка") {
			found++
		}
	}
	if found < 2 {
		t.Fatalf("Ожидали >=2 Telegram сообщений с '🎲 Проверка', получили %d", found)
	}
}
