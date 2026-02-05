package integration

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	abilitycheck "dungeons-and-dragons-ai/internal/game/application/ability_check"
	mapapp "dungeons-and-dragons-ai/internal/game/application/worldmap"
	"dungeons-and-dragons-ai/internal/game/infrastructure/persistence"
	telegrambot "dungeons-and-dragons-ai/internal/telegram"
)

// TestTelegramGameplay_ComprehensiveUserJourney_StubbedLLM
// Тестирует полный игровой процесс пользователя через Telegram с stubbed LLM ответами
// Проверяет все основные механики: создание игры, персонажа, исследование, бой, инвентарь, квесты, заклинания и т.д.
// Этот тест написан для проверки интеграции всех компонентов системы как если бы пользователь играл в игру
func TestTelegramGameplay_ComprehensiveUserJourney_StubbedLLM(t *testing.T) {
	cfg := setupTelegramGameplayTest(t)
	defer cleanupTest(t, cfg.testConfig)

	ctx := cfg.ctx
	chatID := cfg.chatID
	tgUserID := cfg.tgUserID

	var problems []string
	var llmFeedback []string

	// Fake Telegram API server для симуляции пользовательских команд
	fakeAPI := newFakeTelegramAPI()
	srv := httptest.NewServer(fakeAPI.handler(t))
	defer srv.Close()
	apiEndpointFmt := strings.TrimRight(srv.URL, "/") + "/bot%s/%s"

	// Создаем бота с mock зависимостями для большинства операций, но с реальной логикой
	eventRepo := persistence.NewGameEventRepository(cfg.db)
	combatRepo := persistence.NewCombatRepository(cfg.db)
	feedbackRepo := persistence.NewFeedbackRepository(cfg.db)
	playerRepo := persistence.NewPlayerRepository(cfg.db)
	worldEventRepo := persistence.NewWorldEventRepository(cfg.db)
	// For tests, we need to pass nil for LLM and other dependencies
	moveToLocationUC := mapapp.NewMoveToLocationUseCase(nil, cfg.sessionRepo, worldEventRepo, nil, nil)
	performAbilityCheckUC := abilitycheck.NewPerformAbilityCheckUseCase(cfg.sessionRepo, eventRepo, nil)

	// Создаем бота без реального LLM для большинства операций
	bot, err := telegrambot.NewBotWithAPIEndpoint(
		"TEST_TOKEN",
		apiEndpointFmt,
		cfg.initCampaignUC,    // Реальный use case для создания кампании
		cfg.handleActionUC,    // Реальный use case для обработки действий
		cfg.createCharacterUC, // Реальный use case для создания персонажа
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
		nil, // generateImageUC - mock
		nil, // getSubscriptionUC - mock
		nil, // checkLimitsUC - mock
		nil, // getLeaderboardUC - mock
		nil, // updateRatingUC - mock
		performAbilityCheckUC,
		cfg.sessionRepo,
		playerRepo,
		combatRepo,
		feedbackRepo,
		eventRepo,
		nil, // indexDocUC - mock (избегаем RAG вызовов)
	)
	if err != nil {
		t.Fatalf("Не удалось создать Telegram bot: %v", err)
	}

	// ===== ПОЛЬЗОВАТЕЛЬСКИЙ JOURNEY: ПОЛНЫЙ ИГРОВОЙ ПРОЦЕСС =====

	t.Run("Шаг 1: Пользователь начинает игру (/help)", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/help")); err != nil {
			problems = append(problems, fmt.Sprintf("/help: %v", err))
		}
		// Проверяем, что бот ответил на /help
		calls := fakeAPI.snapshotCalls()
		hasHelpResponse := false
		for _, call := range calls {
			if call.ChatID == chatID && strings.Contains(call.Text, "Доступные команды") {
				hasHelpResponse = true
				break
			}
		}
		if !hasHelpResponse {
			problems = append(problems, "Бот не ответил на команду /help")
		}
	})

	t.Run("Шаг 2: Создание новой игры (/newgame)", func(t *testing.T) {
		if err := cfg.waitForRateLimit(ctx); err != nil {
			problems = append(problems, fmt.Sprintf("Rate limiter перед /newgame: %v", err))
		}

		start := time.Now()
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/newgame фэнтези мир с драконами и магией")); err != nil {
			problems = append(problems, fmt.Sprintf("/newgame: %v", err))
			t.Fatalf("/newgame: %v", err)
		}
		duration := time.Since(start)

		// Проверяем, что игра создана
		gs, err := cfg.sessionRepo.GetByChatID(ctx, chatID)
		if err != nil || gs == nil || !gs.IsActive() {
			problems = append(problems, "Игра не создана после /newgame")
			t.Fatalf("Игра не создана после /newgame")
		}

		t.Logf("✅ Игра создана за %.2fs, мир ID=%d, локаций=%d", duration.Seconds(), gs.WorldID, len(gs.World.Locations))

		// Проверяем качество LLM ответа
		dmResponse := lastNonThinkingPlayerFacingText(fakeAPI.snapshotCalls(), chatID)
		if dmResponse != "" && len([]rune(dmResponse)) < 100 {
			llmFeedback = append(llmFeedback, fmt.Sprintf("Слишком короткий ответ DM при создании игры (%d символов): %s", len([]rune(dmResponse)), dmResponse))
		}
	})

	t.Run("Шаг 3: Создание персонажа (/createcharacter)", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/createcharacter ЭльфийскийРазведчик elf ranger")); err != nil {
			problems = append(problems, fmt.Sprintf("/createcharacter: %v", err))
			t.Fatalf("/createcharacter: %v", err)
		}

		// Проверяем, что персонаж создан
		gs, _ := cfg.sessionRepo.GetByChatID(ctx, chatID)
		if gs == nil || gs.GetFirstPlayer() == nil {
			problems = append(problems, "Персонаж не создан после /createcharacter")
			t.Fatal("Персонаж не создан после /createcharacter")
		}

		player := gs.GetFirstPlayer()
		t.Logf("✅ Персонаж создан: %s (%s %s), STR=%d, DEX=%d, CON=%d, INT=%d, WIS=%d, CHA=%d",
			player.Character.Name, player.Character.Race, player.Character.Class,
			player.Character.Stats.Strength, player.Character.Stats.Dexterity,
			player.Character.Stats.Constitution, player.Character.Stats.Intelligence,
			player.Character.Stats.Wisdom, player.Character.Stats.Charisma)
	})

	t.Run("Шаг 4: Исследование мира (игровое действие)", func(t *testing.T) {
		if err := cfg.waitForRateLimit(ctx); err != nil {
			problems = append(problems, fmt.Sprintf("Rate limiter перед действием: %v", err))
		}

		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "Осматриваюсь вокруг, ищу следы жизни или опасности")); err != nil {
			problems = append(problems, fmt.Sprintf("Игровое действие 'осмотр': %v", err))
			t.Fatalf("Игровое действие: %v", err)
		}

		dmResponse := lastNonThinkingPlayerFacingText(fakeAPI.snapshotCalls(), chatID)
		if dmResponse != "" && len([]rune(dmResponse)) < 50 {
			llmFeedback = append(llmFeedback, fmt.Sprintf("Слишком короткий ответ DM на исследование (%d символов): %s", len([]rune(dmResponse)), dmResponse))
		}

		// Проверяем, что могло создаться pending ability check
		gs, _ := cfg.sessionRepo.GetByChatID(ctx, chatID)
		if gs != nil && gs.HasPendingAbilityCheck() {
			t.Logf("ℹ️  Создана pending ability check: %s (DC=%d)", gs.PendingAbilityCheckAbility, gs.PendingAbilityCheckDC)
		}
	})

	t.Run("Шаг 5: Проверка способностей (/roll d20)", func(t *testing.T) {
		// Создаем pending ability check вручную для тестирования
		gs, err := cfg.sessionRepo.GetByChatID(ctx, chatID)
		if err != nil || gs == nil {
			t.Fatalf("Сессия не найдена для создания pending check")
		}

		gs.SetPendingAbilityCheck("test_perception", "wisdom", 14)
		if err := cfg.sessionRepo.Save(ctx, gs); err != nil {
			t.Fatalf("Не удалось сохранить pending ability check: %v", err)
		}

		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/roll d20")); err != nil {
			problems = append(problems, fmt.Sprintf("/roll d20: %v", err))
		}

		// Проверяем, что pending check очищен
		gs, _ = cfg.sessionRepo.GetByChatID(ctx, chatID)
		if gs != nil && gs.HasPendingAbilityCheck() {
			problems = append(problems, "Pending ability check не очищен после /roll d20")
		}
	})

	t.Run("Шаг 6: Инициирование боя через действие", func(t *testing.T) {
		if err := cfg.waitForRateLimit(ctx); err != nil {
			problems = append(problems, fmt.Sprintf("Rate limiter перед боем: %v", err))
		}

		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "Вызываю монстра на бой!")); err != nil {
			problems = append(problems, fmt.Sprintf("Действие инициирующее бой: %v", err))
		}

		// Получаем актуальную сессию
		gs, _ := cfg.sessionRepo.GetByChatID(ctx, chatID)

		// Проверяем, что бой мог создаться
		activeCombat, _ := combatRepo.GetActiveBySessionID(ctx, gs.ID)
		if activeCombat != nil {
			t.Logf("✅ Бой инициирован: %d участников", len(activeCombat.Participants))
		}
	})

	t.Run("Шаг 7: Просмотр поля боя (/battlefield)", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/battlefield table")); err != nil {
			problems = append(problems, fmt.Sprintf("/battlefield: %v", err))
		}

		lastMsg := lastNonThinkingPlayerFacingText(fakeAPI.snapshotCalls(), chatID)
		if lastMsg == "" || !strings.Contains(lastMsg, "Поле боя") {
			problems = append(problems, "После /battlefield не найдено сообщение с полем боя")
		}
	})

	t.Run("Шаг 8: Атака в бою (/attack)", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/attack луком")); err != nil {
			problems = append(problems, fmt.Sprintf("/attack: %v", err))
		}

		// Получаем актуальную сессию
		gs, _ := cfg.sessionRepo.GetByChatID(ctx, chatID)

		_ = waitForCondition(t, 750*time.Millisecond, 25*time.Millisecond, func() bool {
			activeCombat, _ := combatRepo.GetActiveBySessionID(ctx, gs.ID)
			return activeCombat == nil || !activeCombat.IsActive()
		})

		activeCombat, _ := combatRepo.GetActiveBySessionID(ctx, gs.ID)
		if activeCombat != nil {
			t.Logf("ℹ️  После атаки: бой активен, %d участников", len(activeCombat.Participants))
		}
	})

	t.Run("Шаг 9: Проверка инвентаря (/inventory)", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/inventory")); err != nil {
			problems = append(problems, fmt.Sprintf("/inventory: %v", err))
		}
	})

	t.Run("Шаг 10: Просмотр квестов (/quests)", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/quests")); err != nil {
			problems = append(problems, fmt.Sprintf("/quests: %v", err))
		}
	})

	t.Run("Шаг 11: Просмотр ежедневных заданий (/daily)", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/daily")); err != nil {
			problems = append(problems, fmt.Sprintf("/daily: %v", err))
		}
	})

	t.Run("Шаг 12: Просмотр заклинаний (/spells)", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/spells")); err != nil {
			problems = append(problems, fmt.Sprintf("/spells: %v", err))
		}
	})

	t.Run("Шаг 13: Просмотр достижений (/achievements)", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/achievements")); err != nil {
			problems = append(problems, fmt.Sprintf("/achievements: %v", err))
		}
	})

	t.Run("Шаг 14: Просмотр карты (/map)", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/map")); err != nil {
			problems = append(problems, fmt.Sprintf("/map: %v", err))
		}

		// Проверяем наличие inline-кнопок навигации
		calls := fakeAPI.snapshotCalls()
		hasNavigationButtons := false
		for _, call := range calls {
			if call.ChatID == chatID && call.Method == "sendMessage" {
				if strings.Contains(call.Text, "map_to_") {
					hasNavigationButtons = true
					break
				}
			}
		}
		if !hasNavigationButtons {
			problems = append(problems, "После /map не найдены inline-кнопки навигации (map_to_*)")
		}
	})

	t.Run("Шаг 15: Просмотр истории (/history)", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/history")); err != nil {
			problems = append(problems, fmt.Sprintf("/history: %v", err))
		}
	})

	t.Run("Шаг 16: Использование предмета из инвентаря (если есть)", func(t *testing.T) {
		// Этот шаг пропускаем, если инвентарь пустой - это нормально для начала игры
		gs, _ := cfg.sessionRepo.GetByChatID(ctx, chatID)
		if gs != nil {
			player := gs.GetFirstPlayer()
			if player != nil {
				inventoryRepo := persistence.NewInventoryRepository(cfg.db)
				inventory, err := inventoryRepo.GetByCharacterID(ctx, player.Character.ID)
				if err == nil && inventory != nil && len(inventory.Items) > 0 {
					t.Logf("ℹ️  Персонаж имеет предметы в инвентаре: %d шт", len(inventory.Items))
				}
			}
		}
	})

	t.Run("Шаг 17: Проверка на утечки tool-текста", func(t *testing.T) {
		if leak := findToolLeak(fakeAPI.snapshotCalls(), chatID); leak != "" {
			problems = append(problems, fmt.Sprintf("Обнаружена утечка tool-текста в player-facing сообщении: %s", leak))
		}
	})

	t.Run("Шаг 18: Завершение игры (/endgame)", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/endgame")); err != nil {
			problems = append(problems, fmt.Sprintf("/endgame: %v", err))
		}

		// Проверяем, что игра завершена
		gs, _ := cfg.sessionRepo.GetByChatID(ctx, chatID)
		if gs != nil && gs.IsActive() {
			problems = append(problems, "Игра не завершена после /endgame")
		} else {
			t.Log("✅ Игра успешно завершена")
		}
	})

	// ===== ЗАПИСЬ РЕЗУЛЬТАТОВ =====

	if len(problems) > 0 {
		writeToTestingReport(problems)
		t.Logf("❌ Найдено проблем: %d", len(problems))
		for i, problem := range problems {
			t.Logf("  %d. %s", i+1, problem)
		}
	} else {
		t.Log("✅ Все основные механики работают корректно")
	}

	if len(llmFeedback) > 0 {
		writeToFeedback(llmFeedback)
		t.Logf("📝 Собран feedback по LLM: %d записей", len(llmFeedback))
		for i, feedback := range llmFeedback {
			t.Logf("  %d. %s", i+1, feedback)
		}
	}
}
