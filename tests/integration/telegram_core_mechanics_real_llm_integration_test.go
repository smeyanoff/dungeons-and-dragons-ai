package integration

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	abilitycheck "dungeons-and-dragons-ai/internal/game/application/ability_check"
	"dungeons-and-dragons-ai/internal/game/infrastructure/persistence"
	mapapp "dungeons-and-dragons-ai/internal/game/application/worldmap"
	telegrambot "dungeons-and-dragons-ai/internal/telegram"
)

// TestTelegramGameplay_CoreMechanics_RealLLM_Integration
// Интеграционный тест основных механик игры с реальными ответами LLM
// Тестирует полный цикл игры как реальный пользователь через Telegram:
// - Создание мира и кампании с LLM
// - Создание персонажа с LLM
// - Исследование мира и взаимодействие с LLM
// - Система боя и combat detection
// - Интеграция всех основных систем (инвентарь, квесты, достижения, карта, история)
// - Проверка качества LLM ответов и отсутствие утечек tool-текста
// Этот тест заменяет ручное тестирование и обеспечивает качество после каждого развертывания
func TestTelegramGameplay_CoreMechanics_RealLLM_Integration(t *testing.T) {
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

	// Создаем бота с реальными LLM use cases для интеграционного тестирования
	eventRepo := persistence.NewGameEventRepository(cfg.db)
	combatRepo := persistence.NewCombatRepository(cfg.db)
	feedbackRepo := persistence.NewFeedbackRepository(cfg.db)
	playerRepo := persistence.NewPlayerRepository(cfg.db)
	worldEventRepo := persistence.NewWorldEventRepository(cfg.db)
	moveToLocationUC := mapapp.NewMoveToLocationUseCase(nil, cfg.sessionRepo, worldEventRepo, nil, nil)
	performAbilityCheckUC := abilitycheck.NewPerformAbilityCheckUseCase(cfg.sessionRepo, eventRepo, nil)

	// Создаем бота с реальными LLM вызовами для всех основных механик
	bot, err := telegrambot.NewBotWithAPIEndpoint(
		"TEST_TOKEN",
		apiEndpointFmt,
		cfg.initCampaignUC,    // Реальный LLM для создания кампании
		cfg.handleActionUC,    // Реальный LLM для обработки действий
		cfg.createCharacterUC, // Реальный LLM для создания персонажа
		cfg.getHistoryUC,
		cfg.getInventoryUC,
		cfg.addItemUC,
		cfg.handleCombatUC,    // Реальный LLM для обработки боя
		cfg.rollDiceUC,
		cfg.getQuestsUC,
		cfg.getDailyQuestsUC,
		cfg.checkDailyProgressUC,
		cfg.getMapUC,
		moveToLocationUC,
		cfg.getAchievementsUC,
		cfg.getSpellsUC,
		cfg.useSpellUC,
		nil, // generateImageUC - отключаем для экономии
		nil, // getSubscriptionUC - не требуется для теста
		nil, // checkLimitsUC - не требуется для теста
		nil, // getLeaderboardUC - не требуется для теста
		nil, // updateRatingUC - не требуется для теста
		performAbilityCheckUC,
		cfg.sessionRepo,
		playerRepo,
		combatRepo,
		feedbackRepo,
		eventRepo,
		nil, // indexDocUC - отключаем RAG для теста
	)
	if err != nil {
		t.Fatalf("Не удалось создать Telegram bot: %v", err)
	}

	// ===== ПОЛЬЗОВАТЕЛЬСКИЙ JOURNEY: ПОЛНЫЙ ЦИКЛ ИГРЫ С РЕАЛЬНЫМ LLM =====

	t.Run("Шаг 1: Начало игры и получение справки (/help)", func(t *testing.T) {
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

	t.Run("Шаг 2: Создание новой игры с LLM (/newgame)", func(t *testing.T) {
		if err := cfg.waitForRateLimit(ctx); err != nil {
			problems = append(problems, fmt.Sprintf("Rate limiter перед /newgame: %v", err))
		}

		start := time.Now()
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/newgame фэнтези мир с древними руинами, магами и драконами")); err != nil {
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

		// Проверяем качество LLM ответа при создании мира
		dmResponse := lastNonThinkingPlayerFacingText(fakeAPI.snapshotCalls(), chatID)
		if dmResponse != "" && len([]rune(dmResponse)) < 200 {
			llmFeedback = append(llmFeedback, fmt.Sprintf("Слишком короткий LLM ответ при создании мира (%d символов): %s", len([]rune(dmResponse)), dmResponse))
		}

		// Проверяем наличие описания мира
		if dmResponse != "" && !strings.Contains(strings.ToLower(dmResponse), "мир") && !strings.Contains(strings.ToLower(dmResponse), "кампания") {
			llmFeedback = append(llmFeedback, fmt.Sprintf("LLM не описал мир при создании кампании: %s", dmResponse))
		}
	})

	t.Run("Шаг 3: Создание персонажа с LLM (/createcharacter)", func(t *testing.T) {
		if err := cfg.waitForRateLimit(ctx); err != nil {
			problems = append(problems, fmt.Sprintf("Rate limiter перед /createcharacter: %v", err))
		}

		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/createcharacter ВолшебникЭльф elf wizard")); err != nil {
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

		// Проверяем качество LLM ответа при создании персонажа
		dmResponse := lastNonThinkingPlayerFacingText(fakeAPI.snapshotCalls(), chatID)
		if dmResponse != "" && len([]rune(dmResponse)) < 100 {
			llmFeedback = append(llmFeedback, fmt.Sprintf("Слишком короткий LLM ответ при создании персонажа (%d символов): %s", len([]rune(dmResponse)), dmResponse))
		}
	})

	t.Run("Шаг 4: Исследование мира (игровое действие с LLM)", func(t *testing.T) {
		if err := cfg.waitForRateLimit(ctx); err != nil {
			problems = append(problems, fmt.Sprintf("Rate limiter перед исследованием: %v", err))
		}

		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "Осматриваюсь вокруг, изучаю окрестности и ищу следы магии или опасности")); err != nil {
			problems = append(problems, fmt.Sprintf("Игровое действие 'осмотр': %v", err))
			t.Fatalf("Игровое действие: %v", err)
		}

		dmResponse := lastNonThinkingPlayerFacingText(fakeAPI.snapshotCalls(), chatID)
		if dmResponse != "" && len([]rune(dmResponse)) < 100 {
			llmFeedback = append(llmFeedback, fmt.Sprintf("Слишком короткий LLM ответ на исследование (%d символов): %s", len([]rune(dmResponse)), dmResponse))
		}

		// Проверяем наличие описания локации
		if dmResponse != "" && !strings.Contains(strings.ToLower(dmResponse), "вид") && !strings.Contains(strings.ToLower(dmResponse), "окрест") && !strings.Contains(strings.ToLower(dmResponse), "вокруг") {
			llmFeedback = append(llmFeedback, fmt.Sprintf("LLM не описал локацию при исследовании: %s", dmResponse))
		}
	})

	t.Run("Шаг 5: Инициирование боя через действие с LLM", func(t *testing.T) {
		if err := cfg.waitForRateLimit(ctx); err != nil {
			problems = append(problems, fmt.Sprintf("Rate limiter перед боем: %v", err))
		}

		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "Выхожу из укрытия и вызываю на бой любого, кто здесь прячется!")); err != nil {
			problems = append(problems, fmt.Sprintf("Действие инициирующее бой: %v", err))
		}

		// Получаем актуальную сессию
		gs, _ := cfg.sessionRepo.GetByChatID(ctx, chatID)

		// Проверяем, что бой мог создаться
		activeCombat, _ := combatRepo.GetActiveBySessionID(ctx, gs.ID)
		if activeCombat != nil {
			t.Logf("✅ Бой инициирован: %d участников", len(activeCombat.Participants))
		} else {
			t.Logf("ℹ️  Бой не инициирован - возможно, LLM не распознал боевую ситуацию")
		}

		dmResponse := lastNonThinkingPlayerFacingText(fakeAPI.snapshotCalls(), chatID)
		if dmResponse != "" && len([]rune(dmResponse)) < 80 {
			llmFeedback = append(llmFeedback, fmt.Sprintf("Слишком короткий LLM ответ при попытке боя (%d символов): %s", len([]rune(dmResponse)), dmResponse))
		}
	})

	t.Run("Шаг 6: Просмотр статуса боя (/battlefield)", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/battlefield table")); err != nil {
			problems = append(problems, fmt.Sprintf("/battlefield: %v", err))
		}

		lastMsg := lastNonThinkingPlayerFacingText(fakeAPI.snapshotCalls(), chatID)
		if lastMsg == "" || (!strings.Contains(lastMsg, "Поле боя") && !strings.Contains(lastMsg, "бой не активен")) {
			problems = append(problems, "После /battlefield не найдено сообщение с полем боя")
		}
	})

	t.Run("Шаг 7: Проверка инвентаря (/inventory)", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/inventory")); err != nil {
			problems = append(problems, fmt.Sprintf("/inventory: %v", err))
		}
	})

	t.Run("Шаг 8: Просмотр квестов (/quests)", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/quests")); err != nil {
			problems = append(problems, fmt.Sprintf("/quests: %v", err))
		}
	})

	t.Run("Шаг 9: Просмотр ежедневных заданий (/daily)", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/daily")); err != nil {
			problems = append(problems, fmt.Sprintf("/daily: %v", err))
		}
	})

	t.Run("Шаг 10: Просмотр заклинаний (/spells)", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/spells")); err != nil {
			problems = append(problems, fmt.Sprintf("/spells: %v", err))
		}
	})

	t.Run("Шаг 11: Просмотр достижений (/achievements)", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/achievements")); err != nil {
			problems = append(problems, fmt.Sprintf("/achievements: %v", err))
		}
	})

	t.Run("Шаг 12: Просмотр карты мира (/map)", func(t *testing.T) {
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

	t.Run("Шаг 13: Просмотр истории (/history)", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/history")); err != nil {
			problems = append(problems, fmt.Sprintf("/history: %v", err))
		}
	})

	t.Run("Шаг 14: Проверка на утечки tool-текста", func(t *testing.T) {
		if leak := findToolLeak(fakeAPI.snapshotCalls(), chatID); leak != "" {
			problems = append(problems, fmt.Sprintf("Обнаружена утечка tool-текста в player-facing сообщении: %s", leak))
		}
	})

	t.Run("Шаг 15: Завершение игры (/endgame)", func(t *testing.T) {
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

	// ===== ДОПОЛНИТЕЛЬНЫЕ ПРОВЕРКИ КАЧЕСТВА LLM =====

	t.Run("Шаг 16: Анализ качества LLM ответов", func(t *testing.T) {
		// Проверяем все сообщения от LLM на качество
		calls := fakeAPI.snapshotCalls()
		for _, call := range calls {
			if call.ChatID == chatID && call.Method == "sendMessage" && call.Text != "" {
				text := strings.TrimSpace(call.Text)
				if len([]rune(text)) < 20 && !strings.Contains(text, "✅") && !strings.Contains(text, "❌") {
					llmFeedback = append(llmFeedback, fmt.Sprintf("Слишком короткий ответ LLM (%d символов): %s", len([]rune(text)), text))
				}
			}
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