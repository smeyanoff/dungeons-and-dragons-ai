package integration

import (
	"context"
	"strings"
	"testing"

	abilitycheck "dungeons-and-dragons-ai/internal/game/application/ability_check"
	characterapp "dungeons-and-dragons-ai/internal/game/application/character"
	"dungeons-and-dragons-ai/internal/game/domain/character"
	questdomain "dungeons-and-dragons-ai/internal/game/domain/quest"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	worlddomain "dungeons-and-dragons-ai/internal/game/domain/world"
	"dungeons-and-dragons-ai/internal/telegram"

	"net/http/httptest"
)

// TestTelegramGameplay_BotSimulation_AbilityCheckOneTap проверяет основной UX:
// pending ability check -> /roll d20 -> результат -> pending cleared -> событие в истории.
func TestTelegramGameplay_BotSimulation_AbilityCheckOneTap(t *testing.T) {
	cfg := setupInfraOnlyIntegrationTest(t)
	if cfg == nil {
		return
	}
	defer cleanupTest(t, &testConfig{db: cfg.db})

	ctx := cfg.ctx
	chatID := cfg.chatID
	tgUserID := cfg.tgUserID

	// Prepare deterministic world + session
	q := &questdomain.Quest{Title: "Test Quest (AbilityCheck)", Description: "Test quest for ability check flow"}
	w := worlddomain.New("Test World (AbilityCheck)")
	w.Description = "Deterministic test world for ability check one-tap flow"
	w.SetMainQuest(q)
	w.Locations = []worlddomain.Location{{Name: "Start", Description: "Start location"}}
	if err := cfg.worldRepo.Save(ctx, w); err != nil {
		t.Fatalf("Не удалось сохранить тестовый мир: %v", err)
	}

	gs := &session.GameSession{ChatID: chatID, State: session.StateActive, World: *w, WorldID: w.ID}
	if err := cfg.sessionRepo.Save(ctx, gs); err != nil {
		t.Fatalf("Не удалось сохранить сессию: %v", err)
	}

	// Create character (no LLM)
	createCharacterUC := characterapp.NewCreateCharacterUseCase(cfg.sessionRepo, cfg.playerRepo, cfg.inventoryRepo)
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

	// Ability check UC (no embeddings indexing)
	performUC := abilitycheck.NewPerformAbilityCheckUseCase(cfg.sessionRepo, cfg.eventRepo, nil)

	bot, err := telegram.NewBotWithAPIEndpoint(
		"TEST_TOKEN",
		apiEndpointFmt,
		nil, // initCampaignUC
		nil, // handleActionUC
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
		nil, // playerRepo
		nil, // combatRepo
		nil, // feedbackRepo
		nil, // eventRepo (/roll history)
		nil, // indexDocUC (/roll RAG)
	)
	if err != nil {
		t.Fatalf("Не удалось создать Telegram bot (fake API): %v", err)
	}

	// Create pending check in session
	gs, err = cfg.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil || gs == nil {
		t.Fatalf("Не удалось получить сессию: %v", err)
	}
	checkID := "test_check_1"
	gs.SetPendingAbilityCheck(checkID, "dexterity", 13)
	if err := cfg.sessionRepo.Save(ctx, gs); err != nil {
		t.Fatalf("Не удалось сохранить pending check: %v", err)
	}

	// Simulate /roll d20
	if err := bot.HandleUpdate(context.Background(), makeMessageUpdate(chatID, tgUserID, "/roll d20")); err != nil {
		t.Fatalf("/roll d20: %v", err)
	}

	// Verify pending cleared
	gs, err = cfg.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil || gs == nil {
		t.Fatalf("Не удалось получить сессию после callback: %v", err)
	}
	if gs.HasPendingAbilityCheck() {
		t.Fatalf("Pending ability check не очищен после /roll (id=%s, ability=%s, dc=%d)", gs.PendingAbilityCheckID, gs.PendingAbilityCheckAbility, gs.PendingAbilityCheckDC)
	}

	// Verify event persisted to history
	events, err := cfg.eventRepo.GetBySessionID(ctx, gs.ID, 10)
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
	calls := fakeAPI.snapshotCalls()
	foundMsg := false
	for _, c := range calls {
		if c.Method == "sendMessage" && c.ChatID == chatID && strings.Contains(c.Text, "🎲 Проверка") {
			foundMsg = true
			break
		}
	}
	if !foundMsg {
		t.Fatalf("Бот не отправил сообщение результатом ability check (sendMessage с '🎲 Проверка')")
	}
}
