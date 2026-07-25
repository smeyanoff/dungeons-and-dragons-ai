package integration

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	abilitycheck "dungeons-and-dragons-ai/internal/game/application/ability_check"
	mapapp "dungeons-and-dragons-ai/internal/game/application/worldmap"
	"dungeons-and-dragons-ai/internal/game/infrastructure/persistence"
	telegrambot "dungeons-and-dragons-ai/internal/telegram"
)

// TestTelegramGameplay_FullSystemsIntegration_RealLLM
// Полный интеграционный тест всех систем игры с реальными LLM ответами
// Тестирует полную интеграцию всех подсистем:
// - Инвентарь (items, equipment)
// - Квесты (main quests, daily quests)
// - Заклинания (spells, spellcasting)
// - Достижения (achievements, progression)
// - Карта мира (navigation, locations)
// - История (game log, events)
// - Сессионные цели (session goals)
// - Cooperative режим (multiplayer)
// Этот тест проверяет, что все системы работают вместе без конфликтов
func TestTelegramGameplay_FullSystemsIntegration_RealLLM(t *testing.T) {
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

	// Создаем бота с полными зависимостями
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
		cfg.initCampaignUC,    // Real LLM
		cfg.handleActionUC,    // Real LLM
		cfg.createCharacterUC, // Real LLM
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
		nil, // deleteSessionDataUC
	)
	if err != nil {
		t.Fatalf("Не удалось создать Telegram bot: %v", err)
	}

	// ===== ПОДГОТОВКА: СОЗДАНИЕ ПОЛНОЙ ИГРОВОЙ СЕССИИ =====

	t.Run("Подготовка: Создание полной игровой сессии", func(t *testing.T) {
		// Создаем игру
		if err := cfg.waitForRateLimit(ctx); err != nil {
			problems = append(problems, fmt.Sprintf("Rate limiter: %v", err))
		}

		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/newgame комплексный мир для тестирования всех систем")); err != nil {
			problems = append(problems, fmt.Sprintf("/newgame: %v", err))
			t.Fatalf("/newgame: %v", err)
		}

		// Создаем персонажа
		if err := cfg.waitForRateLimit(ctx); err != nil {
			problems = append(problems, fmt.Sprintf("Rate limiter: %v", err))
		}

		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/createcharacter УниверсальныйВоин human fighter")); err != nil {
			problems = append(problems, fmt.Sprintf("/createcharacter: %v", err))
			t.Fatalf("/createcharacter: %v", err)
		}

		// Делаем несколько действий чтобы наполнить игру контентом
		if err := cfg.waitForRateLimit(ctx); err != nil {
			problems = append(problems, fmt.Sprintf("Rate limiter: %v", err))
		}

		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "Исследую окрестности и ищу полезные предметы")); err != nil {
			problems = append(problems, fmt.Sprintf("Исследовательское действие: %v", err))
		}

		t.Log("✅ Полная игровая сессия подготовлена")
	})

	// ===== ТЕСТИРОВАНИЕ ИНТЕГРАЦИИ ВСЕХ СИСТЕМ =====

	t.Run("Система 1: Инвентарь и предметы", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/inventory")); err != nil {
			problems = append(problems, fmt.Sprintf("Инвентарь: %v", err))
		}

		// Проверяем что ответ содержит информацию об инвентаре
		lastMsg := lastNonThinkingPlayerFacingText(fakeAPI.snapshotCalls(), chatID)
		if lastMsg == "" || !strings.Contains(lastMsg, "инвентар") {
			problems = append(problems, "Система инвентаря не отображает информацию")
		}

		// Проверяем что инвентарь интегрируется с другими системами
		gs, _ := cfg.sessionRepo.GetByChatID(ctx, chatID)
		if gs != nil {
			player := gs.GetFirstPlayer()
			if player != nil {
				inventoryRepo := persistence.NewInventoryRepository(cfg.db)
				inventory, err := inventoryRepo.GetByCharacterID(ctx, player.Character.ID)
				if err == nil && inventory != nil {
					t.Logf("✅ Инвентарь интегрирован: %d предметов", len(inventory.Items))
				}
			}
		}
	})

	t.Run("Система 2: Квесты и ежедневные задания", func(t *testing.T) {
		// Основные квесты
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/quests")); err != nil {
			problems = append(problems, fmt.Sprintf("Квесты: %v", err))
		}

		// Ежедневные задания
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/daily")); err != nil {
			problems = append(problems, fmt.Sprintf("Ежедневные задания: %v", err))
		}

		// Проверяем интеграцию квестов с прогрессом
		calls := fakeAPI.snapshotCalls()
		hasQuestInfo := false
		hasDailyInfo := false

		for _, call := range calls {
			if call.ChatID == chatID {
				if strings.Contains(strings.ToLower(call.Text), "квест") ||
					strings.Contains(strings.ToLower(call.Text), "задани") ||
					strings.Contains(strings.ToLower(call.Text), "quest") {
					hasQuestInfo = true
				}
				if strings.Contains(strings.ToLower(call.Text), "ежедневн") ||
					strings.Contains(strings.ToLower(call.Text), "daily") {
					hasDailyInfo = true
				}
			}
		}

		if !hasQuestInfo {
			problems = append(problems, "Система квестов не отображает информацию")
		}
		if !hasDailyInfo {
			problems = append(problems, "Система ежедневных заданий не отображает информацию")
		}

		if hasQuestInfo && hasDailyInfo {
			t.Log("✅ Системы квестов и ежедневных заданий интегрированы")
		}
	})

	t.Run("Система 3: Заклинания и магия", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/spells")); err != nil {
			problems = append(problems, fmt.Sprintf("Заклинания: %v", err))
		}

		// Проверяем что заклинания доступны
		lastMsg := lastNonThinkingPlayerFacingText(fakeAPI.snapshotCalls(), chatID)
		if lastMsg == "" || (!strings.Contains(lastMsg, "заклинани") && !strings.Contains(lastMsg, "маги")) {
			llmFeedback = append(llmFeedback, "Система заклинаний не отображает информацию о доступной магии")
		}

		// Проверяем интеграцию с классовой системой
		gs, _ := cfg.sessionRepo.GetByChatID(ctx, chatID)
		if gs != nil && gs.GetFirstPlayer() != nil {
			player := gs.GetFirstPlayer()
			if player.Character.Class == "fighter" {
				// Воин может не иметь заклинаний - это нормально
				t.Log("✅ Система заклинаний корректно работает с классом fighter (без магии)")
			}
		}
	})

	t.Run("Система 4: Достижения и прогресс", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/achievements")); err != nil {
			problems = append(problems, fmt.Sprintf("Достижения: %v", err))
		}

		// Проверяем что достижения отображаются
		lastMsg := lastNonThinkingPlayerFacingText(fakeAPI.snapshotCalls(), chatID)
		if lastMsg == "" || (!strings.Contains(lastMsg, "достижен") && !strings.Contains(lastMsg, "achievement")) {
			llmFeedback = append(llmFeedback, "Система достижений не отображает прогресс игрока")
		}
	})

	t.Run("Система 5: Карта мира и навигация", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/map")); err != nil {
			problems = append(problems, fmt.Sprintf("Карта: %v", err))
		}

		// Проверяем наличие навигационных кнопок
		calls := fakeAPI.snapshotCalls()
		hasNavigationButtons := false
		for _, call := range calls {
			if call.ChatID == chatID && strings.Contains(call.Text, "map_to_") {
				hasNavigationButtons = true
				break
			}
		}

		if !hasNavigationButtons {
			problems = append(problems, "Карта мира не предоставляет кнопки навигации")
		}

		// Проверяем интеграцию с location events
		gs, _ := cfg.sessionRepo.GetByChatID(ctx, chatID)
		if gs != nil && len(gs.World.Locations) > 0 {
			t.Logf("✅ Карта мира интегрирована с системой локаций: %d локаций", len(gs.World.Locations))
		}
	})

	t.Run("Система 6: История и события", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/history")); err != nil {
			problems = append(problems, fmt.Sprintf("История: %v", err))
		}

		// Проверяем что история содержит события
		lastMsg := lastNonThinkingPlayerFacingText(fakeAPI.snapshotCalls(), chatID)
		if lastMsg == "" || len(strings.TrimSpace(lastMsg)) < 50 {
			llmFeedback = append(llmFeedback, "Система истории не предоставляет достаточно информации")
		}

		// Проверяем отсутствие утечек tool-текста
		if leak := findToolLeak(fakeAPI.snapshotCalls(), chatID); leak != "" {
			problems = append(problems, fmt.Sprintf("Утечка tool-текста в истории: %s", leak))
		}
	})

	t.Run("Система 7: Сессионные цели", func(t *testing.T) {
		// Проверяем через квесты (могут быть интегрированы)
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/quests")); err != nil {
			problems = append(problems, fmt.Sprintf("Сессионные цели через квесты: %v", err))
		}

		// Ищем упоминание целей сессии
		calls := fakeAPI.snapshotCalls()
		hasSessionGoals := false
		for _, call := range calls {
			if call.ChatID == chatID && (strings.Contains(strings.ToLower(call.Text), "сесси") ||
				strings.Contains(strings.ToLower(call.Text), "цель") ||
				strings.Contains(strings.ToLower(call.Text), "таймер")) {
				hasSessionGoals = true
				break
			}
		}

		if !hasSessionGoals {
			llmFeedback = append(llmFeedback, "Сессионные цели не интегрированы в интерфейс игры")
		} else {
			t.Log("✅ Сессионные цели интегрированы в систему")
		}
	})

	t.Run("Система 8: Cooperative режим", func(t *testing.T) {
		// Добавляем второго игрока
		player2UserID := int64(123456789)

		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, player2UserID, "/createcharacter МагГном gnome wizard")); err != nil {
			llmFeedback = append(llmFeedback, "Cooperative режим не поддерживает добавление второго игрока")
		}

		// Проверяем что второй игрок может просматривать общую информацию
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, player2UserID, "/map")); err != nil {
			problems = append(problems, fmt.Sprintf("Cooperative карта для второго игрока: %v", err))
		}

		// Проверяем количество игроков
		gs, _ := cfg.sessionRepo.GetByChatID(ctx, chatID)
		if gs != nil {
			playerCount := 0
			if gs.GetFirstPlayer() != nil {
				playerCount++
			}
			if gs.FindPlayerByTgUserID(player2UserID) != nil {
				playerCount++
			}

			if playerCount >= 2 {
				t.Logf("✅ Cooperative режим работает: %d игроков", playerCount)
			} else {
				llmFeedback = append(llmFeedback, "Cooperative режим не полностью реализован")
			}
		}
	})

	t.Run("Система 9: Броски кубика и ability checks", func(t *testing.T) {
		// Создаем pending ability check вручную
		gs, err := cfg.sessionRepo.GetByChatID(ctx, chatID)
		if err == nil && gs != nil {
			gs.SetPendingAbilityCheck("test_perception", "wisdom", 13)
			if err := cfg.sessionRepo.Save(ctx, gs); err == nil {
				// Теперь тестируем бросок
				if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/roll d20")); err != nil {
					problems = append(problems, fmt.Sprintf("Бросок кубика: %v", err))
				}

				// Проверяем что pending check очищен
				gs, _ = cfg.sessionRepo.GetByChatID(ctx, chatID)
				if gs != nil && gs.HasPendingAbilityCheck() {
					problems = append(problems, "Ability check не очищен после броска кубика")
				} else {
					t.Log("✅ Система бросков кубика интегрирована с ability checks")
				}
			}
		}
	})

	t.Run("Система 10: Боевая система", func(t *testing.T) {
		if err := cfg.waitForRateLimit(ctx); err != nil {
			problems = append(problems, fmt.Sprintf("Rate limiter перед боем: %v", err))
		}

		// Пытаемся инициировать бой
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "Вызываю монстра на бой! Атакую его!")); err != nil {
			problems = append(problems, fmt.Sprintf("Инициирование боя: %v", err))
		}

		// Проверяем статус боя
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/battlefield table")); err != nil {
			problems = append(problems, fmt.Sprintf("Просмотр поля боя: %v", err))
		}

		// Проверяем что боевая система интегрирована
		gs, _ := cfg.sessionRepo.GetByChatID(ctx, chatID)
		activeCombat, _ := combatRepo.GetActiveBySessionID(ctx, gs.ID)
		if activeCombat != nil {
			t.Logf("✅ Боевая система активна: %d участников", len(activeCombat.Participants))
		} else {
			t.Log("ℹ️  Бой не активен - возможно, LLM не распознал боевую ситуацию")
		}
	})

	// ===== ПРОВЕРКА ПРОФИЛАКТИЧЕСКИХ МЕР =====

	t.Run("Профилактика: Проверка на утечки tool-текста во всех системах", func(t *testing.T) {
		if leak := findToolLeak(fakeAPI.snapshotCalls(), chatID); leak != "" {
			problems = append(problems, fmt.Sprintf("Критическая утечка tool-текста: %s", leak))
		}
	})

	t.Run("Профилактика: Проверка качества всех LLM ответов", func(t *testing.T) {
		calls := fakeAPI.snapshotCalls()
		shortResponses := 0

		for _, call := range calls {
			if call.ChatID == chatID && call.Method == "sendMessage" && call.Text != "" {
				text := strings.TrimSpace(call.Text)
				if len([]rune(text)) < 30 && !strings.Contains(text, "✅") && !strings.Contains(text, "❌") {
					shortResponses++
				}
			}
		}

		if shortResponses > 5 {
			llmFeedback = append(llmFeedback, fmt.Sprintf("Слишком много коротких LLM ответов: %d из %d", shortResponses, len(calls)))
		}
	})

	// ===== ЗАПИСЬ РЕЗУЛЬТАТОВ =====

	if len(problems) > 0 {
		writeToTestingReport(problems)
		t.Logf("❌ Найдено проблем в интеграции систем: %d", len(problems))
		for i, problem := range problems {
			t.Logf("  %d. %s", i+1, problem)
		}
	} else {
		t.Log("✅ Все системы игры корректно интегрированы")
	}

	if len(llmFeedback) > 0 {
		writeToFeedback(llmFeedback)
		t.Logf("📝 Собран feedback по интеграции систем: %d записей", len(llmFeedback))
		for i, feedback := range llmFeedback {
			t.Logf("  %d. %s", i+1, feedback)
		}
	}
}
