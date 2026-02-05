package integration

import (
	"net/http/httptest"
	"strings"
	"testing"

	abilitycheck "dungeons-and-dragons-ai/internal/game/application/ability_check"
	mapapp "dungeons-and-dragons-ai/internal/game/application/worldmap"
	"dungeons-and-dragons-ai/internal/game/infrastructure/persistence"
	telegrambot "dungeons-and-dragons-ai/internal/telegram"
)

func TestTelegramBasicCommands(t *testing.T) {
	cfg := setupTelegramGameplayTest(t)
	if cfg == nil {
		return
	}
	defer cleanupTest(t, cfg.testConfig)

	ctx := cfg.ctx
	chatID := cfg.chatID
	tgUserID := cfg.tgUserID

	fakeAPI := newFakeTelegramAPI()
	srv := httptest.NewServer(fakeAPI.handler(t))
	defer srv.Close()

	apiEndpointFmt := strings.TrimRight(srv.URL, "/") + "/bot%s/%s"

	eventRepo := persistence.NewGameEventRepository(cfg.db)
	combatRepo := persistence.NewCombatRepository(cfg.db)
	feedbackRepo := persistence.NewFeedbackRepository(cfg.db)
	playerRepo := persistence.NewPlayerRepository(cfg.db)
	worldEventRepo := persistence.NewWorldEventRepository(cfg.db)
	moveToLocationUC := mapapp.NewMoveToLocationUseCase(nil, cfg.sessionRepo, worldEventRepo, nil, nil)
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
		nil, // indexDocUC
	)
	if err != nil {
		t.Fatalf("Не удалось создать Telegram bot (fake API): %v", err)
	}

	assertLastResponse := func(step string) {
		t.Helper()
		fakeAPI.mu.Lock()
		defer fakeAPI.mu.Unlock()
		if len(fakeAPI.calls) == 0 {
			t.Fatalf("%s: bot did not send any messages", step)
		}
		last := fakeAPI.calls[len(fakeAPI.calls)-1]
		if strings.TrimSpace(last.Text) == "" {
			t.Fatalf("%s: bot responded with empty text", step)
		}
	}

	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/help")); err != nil {
		t.Fatalf("/help: %v", err)
	}
	assertLastResponse("/help")

	if err := cfg.waitForRateLimit(ctx); err != nil {
		t.Logf("rate limiter before /newgame: %v", err)
	}
	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/newgame базовый тест")); err != nil {
		t.Fatalf("/newgame: %v", err)
	}
	assertLastResponse("/newgame")

	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/createcharacter ТестовыйГерой elf wizard")); err != nil {
		t.Fatalf("/createcharacter: %v", err)
	}
	assertLastResponse("/createcharacter")

	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/map")); err != nil {
		t.Fatalf("/map: %v", err)
	}
	assertLastResponse("/map")

	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/battlefield")); err != nil {
		t.Fatalf("/battlefield: %v", err)
	}
	assertLastResponse("/battlefield")

	t.Logf("✅ Basic commands handled for chat_id=%d", chatID)
}
