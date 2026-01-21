package integration

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	characterapp "dungeons-and-dragons-ai/internal/game/application/character"
	"dungeons-and-dragons-ai/internal/game/domain/character"
	combatdomain "dungeons-and-dragons-ai/internal/game/domain/combat"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/game/infrastructure/persistence"
	telegrambot "dungeons-and-dragons-ai/internal/telegram"
)

// TestTelegramBattlefieldCommand проверяет команду /battlefield в активном бою.
func TestTelegramBattlefieldCommand(t *testing.T) {
	cfg := setupTelegramGameplayTest(t)
	defer cleanupTest(t, cfg.testConfig)

	ctx := cfg.ctx
	chatID := cfg.chatID
	tgUserID := cfg.tgUserID

	fakeAPI := newFakeTelegramAPI()
	srv := httptest.NewServer(fakeAPI.handler(t))
	defer srv.Close()

	apiEndpointFmt := strings.TrimRight(srv.URL, "/") + "/bot%s/%s"
	combatRepo := persistence.NewCombatRepository(cfg.db)
	feedbackRepo := persistence.NewFeedbackRepository(cfg.db)

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
		nil, // moveToLocationUC
		cfg.getAchievementsUC,
		cfg.getSpellsUC,
		cfg.useSpellUC,
		nil, // generateImageUC
		nil, // getSubscriptionUC
		nil, // checkLimitsUC
		nil, // getLeaderboardUC
		nil, // updateRatingUC
		nil, // performAbilityCheckUC
		cfg.sessionRepo,
		combatRepo,
		feedbackRepo,
		nil, // eventRepo
		nil, // indexDocUC
	)
	if err != nil {
		t.Fatalf("Не удалось создать Telegram bot (fake API): %v", err)
	}

	world, err := cfg.initCampaignUC.Execute(ctx, "классическое фэнтези")
	if err != nil {
		t.Fatalf("Не удалось создать мир: %v", err)
	}

	gs := &session.GameSession{ChatID: chatID, State: session.StateActive, World: *world, WorldID: world.ID}
	if err := cfg.sessionRepo.Save(ctx, gs); err != nil {
		t.Fatalf("Не удалось сохранить сессию: %v", err)
	}

	player, err := cfg.createCharacterUC.Execute(ctx, newCharacterRequest(chatID))
	if err != nil {
		t.Fatalf("Не удалось создать персонажа: %v", err)
	}

	// Создаем активный бой
	playerCharID := player.Character.ID
	combat := &combatdomain.Combat{
		GameSessionID: gs.ID,
		State:         combatdomain.CombatStateNotStarted,
		Participants: []combatdomain.CombatParticipant{
			{
				IsPlayer:    true,
				CharacterID: &playerCharID,
				Character:   &player.Character,
			},
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

	// Выполняем команду /battlefield
	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/battlefield table")); err != nil {
		t.Fatalf("Ошибка при /battlefield: %v", err)
	}

	// Проверяем, что отправлено сообщение с полем боя
	lastMsg := lastMessageText(fakeAPI, chatID)
	if lastMsg == "" {
		t.Fatal("Ожидалось сообщение с полем боя, но сообщений нет")
	}
	if !strings.Contains(lastMsg, "Поле боя") && !strings.Contains(lastMsg, "ПОЛЕ БОЯ") {
		t.Fatalf("Ожидалось сообщение с полем боя, получили: %s", lastMsg)
	}
	if !strings.Contains(lastMsg, "Goblin") {
		t.Fatalf("Ожидалось упоминание врага в поле боя, получили: %s", lastMsg)
	}
}

func newCharacterRequest(chatID int64) characterapp.CreateCharacterRequest {
	return characterapp.CreateCharacterRequest{
		ChatID: chatID,
		Name:   fmt.Sprintf("Герой_%d", chatID),
		Race:   character.RaceHuman,
		Class:  character.ClassFighter,
	}
}

func lastMessageText(fakeAPI *fakeTelegramAPI, chatID int64) string {
	fakeAPI.mu.Lock()
	defer fakeAPI.mu.Unlock()
	for i := len(fakeAPI.calls) - 1; i >= 0; i-- {
		call := fakeAPI.calls[i]
		if call.ChatID == chatID && call.Method == "sendMessage" {
			return call.Text
		}
	}
	return ""
}
