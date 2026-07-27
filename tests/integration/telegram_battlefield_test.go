package integration

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	characterapp "dungeons-and-dragons-ai/internal/game/application/character"
	combatapp "dungeons-and-dragons-ai/internal/game/application/combat"
	"dungeons-and-dragons-ai/internal/game/domain/character"
	combatdomain "dungeons-and-dragons-ai/internal/game/domain/combat"
	questdomain "dungeons-and-dragons-ai/internal/game/domain/quest"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	worlddomain "dungeons-and-dragons-ai/internal/game/domain/world"
	"dungeons-and-dragons-ai/internal/game/infrastructure/persistence"
	telegrambot "dungeons-and-dragons-ai/internal/telegram"
)

// TestTelegramBattlefieldCommand проверяет команду /battlefield в активном бою.
func TestTelegramBattlefieldCommand(t *testing.T) {
	cfg := setupInfraOnlyIntegrationTest(t)
	if cfg == nil {
		return
	}
	defer cleanupTest(t, &testConfig{db: cfg.db, chatID: cfg.chatID, tgUserID: cfg.tgUserID})

	ctx := cfg.ctx
	chatID := cfg.chatID
	tgUserID := cfg.tgUserID

	fakeAPI := newFakeTelegramAPI()
	srv := httptest.NewServer(fakeAPI.handler(t))
	defer srv.Close()

	apiEndpointFmt := strings.TrimRight(srv.URL, "/") + "/bot%s/%s"
	combatRepo := persistence.NewCombatRepository(cfg.db)
	feedbackRepo := persistence.NewFeedbackRepository(cfg.db)

	// Deterministic world+session (no real LLM).
	q := &questdomain.Quest{Title: "Test Quest (Battlefield)", Description: "Deterministic quest for /battlefield"}
	w := worlddomain.New("Test World (Battlefield)")
	w.Description = "Deterministic test world for /battlefield"
	w.SetMainQuest(q)
	w.Locations = []worlddomain.Location{{Name: "Start", Description: "Start location"}}
	if err := cfg.worldRepo.Save(ctx, w); err != nil {
		t.Fatalf("Не удалось сохранить тестовый мир: %v", err)
	}
	gs := &session.GameSession{ChatID: chatID, State: session.StateActive, World: *w, WorldID: w.ID}
	if err := cfg.sessionRepo.Save(ctx, gs); err != nil {
		t.Fatalf("Не удалось сохранить сессию: %v", err)
	}

	// Character (no real LLM).
	createCharacterUC := characterapp.NewCreateCharacterUseCase(cfg.sessionRepo, cfg.playerRepo, cfg.inventoryRepo, cfg.spellRepo)
	player, err := createCharacterUC.Execute(ctx, newCharacterRequest(chatID))
	if err != nil {
		t.Fatalf("Не удалось создать персонажа: %v", err)
	}

	handleCombatUC := combatapp.NewHandleCombatUseCase(combatRepo, cfg.sessionRepo)

	bot, err := telegrambot.NewBotWithAPIEndpoint(
		"TEST_TOKEN",
		apiEndpointFmt,
		nil, // initCampaignUC
		nil, // handleActionUC
		nil, // createCharacterUC
		nil, // getHistoryUC
		nil, // getInventoryUC
		nil, // addItemUC
		handleCombatUC,
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
		combatRepo,
		feedbackRepo,
		nil, // eventRepo
		nil, // indexDocUC
		nil, // deleteSessionDataUC
	)
	if err != nil {
		t.Fatalf("Не удалось создать Telegram bot (fake API): %v", err)
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

// TestTelegramBattlefieldCommandFormats проверяет разные форматы отображения поля боя
func TestTelegramBattlefieldCommandFormats(t *testing.T) {
	cfg := setupInfraOnlyIntegrationTest(t)
	if cfg == nil {
		return
	}
	defer cleanupTest(t, &testConfig{db: cfg.db, chatID: cfg.chatID, tgUserID: cfg.tgUserID})

	ctx := cfg.ctx
	chatID := cfg.chatID
	tgUserID := cfg.tgUserID

	fakeAPI := newFakeTelegramAPI()
	srv := httptest.NewServer(fakeAPI.handler(t))
	defer srv.Close()

	apiEndpointFmt := strings.TrimRight(srv.URL, "/") + "/bot%s/%s"
	combatRepo := persistence.NewCombatRepository(cfg.db)
	feedbackRepo := persistence.NewFeedbackRepository(cfg.db)

	// Deterministic world+session
	q := &questdomain.Quest{Title: "Test Quest (Battlefield Formats)", Description: "Deterministic quest for testing battlefield formats"}
	w := worlddomain.New("Test World (Battlefield Formats)")
	w.Description = "Deterministic test world for battlefield formats"
	w.SetMainQuest(q)
	w.Locations = []worlddomain.Location{{Name: "Start", Description: "Start location"}}
	if err := cfg.worldRepo.Save(ctx, w); err != nil {
		t.Fatalf("Не удалось сохранить тестовый мир: %v", err)
	}
	gs := &session.GameSession{ChatID: chatID, State: session.StateActive, World: *w, WorldID: w.ID}
	if err := cfg.sessionRepo.Save(ctx, gs); err != nil {
		t.Fatalf("Не удалось сохранить сессию: %v", err)
	}

	// Character
	createCharacterUC := characterapp.NewCreateCharacterUseCase(cfg.sessionRepo, cfg.playerRepo, cfg.inventoryRepo, cfg.spellRepo)
	player, err := createCharacterUC.Execute(ctx, newCharacterRequest(chatID))
	if err != nil {
		t.Fatalf("Не удалось создать персонажа: %v", err)
	}

	handleCombatUC := combatapp.NewHandleCombatUseCase(combatRepo, cfg.sessionRepo)

	bot, err := telegrambot.NewBotWithAPIEndpoint(
		"TEST_TOKEN",
		apiEndpointFmt,
		nil, // initCampaignUC
		nil, // handleActionUC
		nil, // createCharacterUC
		nil, // getHistoryUC
		nil, // getInventoryUC
		nil, // addItemUC
		handleCombatUC,
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
		combatRepo,
		feedbackRepo,
		nil, // eventRepo
		nil, // indexDocUC
		nil, // deleteSessionDataUC
	)
	if err != nil {
		t.Fatalf("Не удалось создать Telegram bot (fake API): %v", err)
	}

	// Создаем активный бой с несколькими врагами
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
				MonsterName:        "Goblin Warrior",
				MonsterHP:          15,
				MonsterMaxHP:       15,
				MonsterAC:          13,
				MonsterAttackBonus: 3,
			},
			{
				IsPlayer:           false,
				MonsterName:        "Orc",
				MonsterHP:          25,
				MonsterMaxHP:       30,
				MonsterAC:          15,
				MonsterAttackBonus: 4,
			},
		},
	}
	if err := combat.Start(); err != nil {
		t.Fatalf("Не удалось стартовать бой: %v", err)
	}
	if err := combatRepo.Save(ctx, combat); err != nil {
		t.Fatalf("Не удалось сохранить бой: %v", err)
	}

	// Тест разных форматов
	formats := []string{"table", "compact", "detailed"}
	for _, format := range formats {
		t.Run(fmt.Sprintf("Format_%s", format), func(t *testing.T) {
			command := "/battlefield"
			if format != "table" {
				command = fmt.Sprintf("/battlefield %s", format)
			}

			// Выполняем команду
			if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, command)); err != nil {
				t.Fatalf("Ошибка при /battlefield %s: %v", format, err)
			}

			// Проверяем ответ
			lastMsg := lastMessageText(fakeAPI, chatID)
			if lastMsg == "" {
				t.Fatal("Ожидалось сообщение с полем боя, но сообщений нет")
			}

			// Проверяем наличие поля боя в ответе
			if !strings.Contains(lastMsg, "Поле боя") && !strings.Contains(lastMsg, "ПОЛЕ БОЯ") {
				t.Fatalf("Ожидалось сообщение с полем боя, получили: %s", lastMsg)
			}

			// Проверяем наличие врагов
			if !strings.Contains(lastMsg, "Goblin Warrior") || !strings.Contains(lastMsg, "Orc") {
				t.Fatalf("Ожидалось упоминание всех врагов в поле боя, получили: %s", lastMsg)
			}

			// Проверяем формат-специфичные элементы
			switch format {
			case "table":
				if !strings.Contains(lastMsg, "HP") || !strings.Contains(lastMsg, "AC") {
					t.Fatalf("В табличном формате должны быть колонки HP и AC: %s", lastMsg)
				}
			case "compact":
				// Компактный формат должен быть кратким
				if len(lastMsg) > 1000 {
					t.Fatalf("Компактный формат должен быть кратким, но получил длинное сообщение: %d символов", len(lastMsg))
				}
			case "detailed":
				if !strings.Contains(lastMsg, "ДЕТАЛЬНАЯ ИНФОРМАЦИЯ") || !strings.Contains(lastMsg, "Бонус атаки") || !strings.Contains(lastMsg, "Инициатива") {
					t.Fatalf("В детальном формате должны быть подробные характеристики: %s", lastMsg)
				}
			}
		})
	}
}

// TestTelegramBattlefieldCommandNoCombat проверяет команду /battlefield когда боя нет
func TestTelegramBattlefieldCommandNoCombat(t *testing.T) {
	cfg := setupInfraOnlyIntegrationTest(t)
	if cfg == nil {
		return
	}
	defer cleanupTest(t, &testConfig{db: cfg.db, chatID: cfg.chatID, tgUserID: cfg.tgUserID})

	ctx := cfg.ctx
	chatID := cfg.chatID
	tgUserID := cfg.tgUserID

	fakeAPI := newFakeTelegramAPI()
	srv := httptest.NewServer(fakeAPI.handler(t))
	defer srv.Close()

	apiEndpointFmt := strings.TrimRight(srv.URL, "/") + "/bot%s/%s"
	combatRepo := persistence.NewCombatRepository(cfg.db)
	feedbackRepo := persistence.NewFeedbackRepository(cfg.db)

	// Deterministic world+session (без боя)
	q := &questdomain.Quest{Title: "Test Quest (No Combat)", Description: "Deterministic quest for testing battlefield without combat"}
	w := worlddomain.New("Test World (No Combat)")
	w.Description = "Deterministic test world for battlefield without combat"
	w.SetMainQuest(q)
	w.Locations = []worlddomain.Location{{Name: "Start", Description: "Start location"}}
	if err := cfg.worldRepo.Save(ctx, w); err != nil {
		t.Fatalf("Не удалось сохранить тестовый мир: %v", err)
	}
	gs := &session.GameSession{ChatID: chatID, State: session.StateActive, World: *w, WorldID: w.ID}
	if err := cfg.sessionRepo.Save(ctx, gs); err != nil {
		t.Fatalf("Не удалось сохранить сессию: %v", err)
	}

	// Character
	createCharacterUC := characterapp.NewCreateCharacterUseCase(cfg.sessionRepo, cfg.playerRepo, cfg.inventoryRepo, cfg.spellRepo)
	_, err := createCharacterUC.Execute(ctx, newCharacterRequest(chatID))
	if err != nil {
		t.Fatalf("Не удалось создать персонажа: %v", err)
	}

	handleCombatUC := combatapp.NewHandleCombatUseCase(combatRepo, cfg.sessionRepo)

	bot, err := telegrambot.NewBotWithAPIEndpoint(
		"TEST_TOKEN",
		apiEndpointFmt,
		nil, // initCampaignUC
		nil, // handleActionUC
		nil, // createCharacterUC
		nil, // getHistoryUC
		nil, // getInventoryUC
		nil, // addItemUC
		handleCombatUC,
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
		combatRepo,
		feedbackRepo,
		nil, // eventRepo
		nil, // indexDocUC
		nil, // deleteSessionDataUC
	)
	if err != nil {
		t.Fatalf("Не удалось создать Telegram bot (fake API): %v", err)
	}

	// Выполняем команду /battlefield когда боя нет
	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/battlefield")); err != nil {
		t.Fatalf("Ошибка при /battlefield без боя: %v", err)
	}

	// Проверяем, что получено сообщение об отсутствии боя
	lastMsg := lastMessageText(fakeAPI, chatID)
	if lastMsg == "" {
		t.Fatal("Ожидалось сообщение об отсутствии боя, но сообщений нет")
	}

	if !strings.Contains(lastMsg, "нет активного боя") && !strings.Contains(lastMsg, "Сейчас нет активного боя") {
		t.Fatalf("Ожидалось сообщение об отсутствии боя, получили: %s", lastMsg)
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
