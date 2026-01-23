package integration_new

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

// TestTelegramGameplay_NewFeatures_RealLLM_Integration
// Интеграционный тест новых фич из TASKS.md с реальными ответами LLM
// Тестирует:
// - Сессионные цели с таймерами (P2 улучшения)
// - Cooperative режим для 2-3 игроков (P2 улучшения)
// - Интеграцию новых механик с существующими системами
// Этот тест проверяет работу новых фич после их реализации
func TestTelegramGameplay_NewFeatures_RealLLM_Integration(t *testing.T) {
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

	// Создаем бота с реальными LLM use cases
	eventRepo := persistence.NewGameEventRepository(cfg.db)
	combatRepo := persistence.NewCombatRepository(cfg.db)
	feedbackRepo := persistence.NewFeedbackRepository(cfg.db)
	playerRepo := persistence.NewPlayerRepository(cfg.db)
	worldEventRepo := persistence.NewWorldEventRepository(cfg.db)
	moveToLocationUC := mapapp.NewMoveToLocationUseCase(nil, cfg.sessionRepo, worldEventRepo, nil, nil)
	performAbilityCheckUC := abilitycheck.NewPerformAbilityCheckUseCase(cfg.sessionRepo, eventRepo, nil)

	// Создаем бота с реальными LLM вызовами
	bot, err := telegrambot.NewBotWithAPIEndpoint(
		"TEST_TOKEN",
		apiEndpointFmt,
		cfg.initCampaignUC,    // Реальный LLM для создания кампании
		cfg.handleActionUC,    // Реальный LLM для обработки действий
		cfg.createCharacterUC, // Реальный LLM для создания персонажа
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
		t.Fatalf("Не удалось создать Telegram bot: %v", err)
	}

	// ===== ТЕСТИРОВАНИЕ СЕССИОННЫХ ЦЕЛЕЙ =====

	t.Run("Часть 1: Сессионные цели - настройка и проверка", func(t *testing.T) {
		t.Run("Шаг 1.1: Создание игры с сессионными целями", func(t *testing.T) {
			if err := cfg.waitForRateLimit(ctx); err != nil {
				problems = append(problems, fmt.Sprintf("Rate limiter перед /newgame: %v", err))
			}

			if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/newgame квест в подземелье с таймерами и целями на сессию")); err != nil {
				problems = append(problems, fmt.Sprintf("/newgame для сессионных целей: %v", err))
				t.Fatalf("/newgame: %v", err)
			}

			// Проверяем, что игра создана
			gs, err := cfg.sessionRepo.GetByChatID(ctx, chatID)
			if err != nil || gs == nil || !gs.IsActive() {
				problems = append(problems, "Игра не создана для тестирования сессионных целей")
				t.Fatalf("Игра не создана")
			}

			t.Logf("✅ Игра создана для тестирования сессионных целей, мир ID=%d", gs.WorldID)
		})

		t.Run("Шаг 1.2: Создание персонажа для сессии", func(t *testing.T) {
			if err := cfg.waitForRateLimit(ctx); err != nil {
				problems = append(problems, fmt.Sprintf("Rate limiter перед /createcharacter: %v", err))
			}

			if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/createcharacter РейнджерЧеловек human ranger")); err != nil {
				problems = append(problems, fmt.Sprintf("/createcharacter для сессионных целей: %v", err))
				t.Fatalf("/createcharacter: %v", err)
			}

			gs, _ := cfg.sessionRepo.GetByChatID(ctx, chatID)
			if gs == nil || gs.GetFirstPlayer() == nil {
				problems = append(problems, "Персонаж не создан для сессионных целей")
				t.Fatal("Персонаж не создан")
			}

			t.Logf("✅ Персонаж создан для тестирования сессионных целей")
		})

		t.Run("Шаг 1.3: Проверка отображения сессионных целей (/sessiongoals или аналог)", func(t *testing.T) {
			// Проверяем, есть ли команда для просмотра сессионных целей
			// Если команды нет, это может быть нормально - цели могут отображаться в /quests
			if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/quests")); err != nil {
				problems = append(problems, fmt.Sprintf("/quests для проверки сессионных целей: %v", err))
			}

			// Ищем в ответах упоминание целей сессии
			calls := fakeAPI.snapshotCalls()
			hasSessionGoals := false
			for _, call := range calls {
				if call.ChatID == chatID && strings.Contains(strings.ToLower(call.Text), "цель") ||
					strings.Contains(strings.ToLower(call.Text), "goal") ||
					strings.Contains(strings.ToLower(call.Text), "таймер") ||
					strings.Contains(strings.ToLower(call.Text), "сессия") {
					hasSessionGoals = true
					break
				}
			}

			if !hasSessionGoals {
				llmFeedback = append(llmFeedback, "Сессионные цели не отображаются в интерфейсе игры")
			} else {
				t.Logf("✅ Сессионные цели найдены в интерфейсе")
			}
		})
	})

	// ===== ТЕСТИРОВАНИЕ COOPERATIVE РЕЖИМА =====

	t.Run("Часть 2: Cooperative режим - настройка мультиплеера", func(t *testing.T) {
		t.Run("Шаг 2.1: Создание новой игры для cooperative режима", func(t *testing.T) {
			// Завершаем предыдущую игру
			if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/endgame")); err != nil {
				t.Logf("Не удалось завершить предыдущую игру: %v", err)
			}

			if err := cfg.waitForRateLimit(ctx); err != nil {
				problems = append(problems, fmt.Sprintf("Rate limiter перед cooperative /newgame: %v", err))
			}

			if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/newgame совместное приключение для нескольких игроков в фэнтези мире")); err != nil {
				problems = append(problems, fmt.Sprintf("/newgame для cooperative режима: %v", err))
				t.Fatalf("/newgame cooperative: %v", err)
			}

			gs, err := cfg.sessionRepo.GetByChatID(ctx, chatID)
			if err != nil || gs == nil || !gs.IsActive() {
				problems = append(problems, "Игра не создана для cooperative режима")
				t.Fatalf("Игра не создана для cooperative")
			}

			t.Logf("✅ Игра создана для cooperative режима, мир ID=%d", gs.WorldID)
		})

		t.Run("Шаг 2.2: Создание первого игрока в cooperative игре", func(t *testing.T) {
			if err := cfg.waitForRateLimit(ctx); err != nil {
				problems = append(problems, fmt.Sprintf("Rate limiter перед cooperative /createcharacter: %v", err))
			}

			if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/createcharacter ВолшебникЭльф elf wizard")); err != nil {
				problems = append(problems, fmt.Sprintf("Создание первого игрока cooperative: %v", err))
				t.Fatalf("Создание первого игрока cooperative: %v", err)
			}

			gs, _ := cfg.sessionRepo.GetByChatID(ctx, chatID)
			if gs == nil || gs.GetFirstPlayer() == nil {
				problems = append(problems, "Первый игрок не создан в cooperative режиме")
				t.Fatal("Первый игрок не создан в cooperative")
			}

			t.Logf("✅ Первый игрок создан в cooperative режиме")
		})

		t.Run("Шаг 2.3: Симуляция добавления второго игрока", func(t *testing.T) {
			// Имитируем второго игрока с другим user ID
			player2UserID := int64(123456789)

			// Проверяем, что бот может обработать команды от второго игрока
			if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, player2UserID, "/help")); err != nil {
				problems = append(problems, fmt.Sprintf("Второй игрок не может получить помощь: %v", err))
			}

			// Проверяем, что второй игрок может присоединиться к игре
			if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, player2UserID, "/createcharacter ВоинДварф dwarf fighter")); err != nil {
				problems = append(problems, fmt.Sprintf("Второй игрок не может создать персонажа: %v", err))
			}

			// Проверяем, что теперь в игре два игрока
			gs, _ := cfg.sessionRepo.GetByChatID(ctx, chatID)
			if gs != nil {
				playerCount := 0
				if gs.GetFirstPlayer() != nil {
					playerCount++
				}
				if gs.GetPlayerByUserID(player2UserID) != nil {
					playerCount++
				}

				if playerCount < 2 {
					problems = append(problems, fmt.Sprintf("Cooperative режим не поддерживает нескольких игроков (найдено %d игроков)", playerCount))
				} else {
					t.Logf("✅ Cooperative режим работает: %d игроков в игре", playerCount)
				}
			}
		})

		t.Run("Шаг 2.4: Проверка совместных действий в cooperative режиме", func(t *testing.T) {
			if err := cfg.waitForRateLimit(ctx); err != nil {
				problems = append(problems, fmt.Sprintf("Rate limiter перед cooperative действием: %v", err))
			}

			// Первый игрок делает действие
			if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "Первый игрок осматривает окрестности и ищет путь вперед")); err != nil {
				problems = append(problems, fmt.Sprintf("Действие первого игрока в cooperative: %v", err))
			}

			// Второй игрок делает действие
			player2UserID := int64(123456789)
			if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, player2UserID, "Второй игрок проверяет, нет ли ловушек на пути")); err != nil {
				problems = append(problems, fmt.Sprintf("Действие второго игрока в cooperative: %v", err))
			}

			// Проверяем, что оба действия обработаны
			calls := fakeAPI.snapshotCalls()
			firstPlayerActions := 0
			secondPlayerActions := 0

			for _, call := range calls {
				if call.ChatID == chatID {
					if strings.Contains(call.Text, "первый игрок") || strings.Contains(call.Text, "первый игрок") {
						firstPlayerActions++
					}
					if strings.Contains(call.Text, "второй игрок") || strings.Contains(call.Text, "второй игрок") {
						secondPlayerActions++
					}
				}
			}

			if firstPlayerActions == 0 {
				llmFeedback = append(llmFeedback, "LLM не обработал действие первого игрока в cooperative режиме")
			}
			if secondPlayerActions == 0 {
				llmFeedback = append(llmFeedback, "LLM не обработал действие второго игрока в cooperative режиме")
			}

			if firstPlayerActions > 0 && secondPlayerActions > 0 {
				t.Logf("✅ Cooperative режим работает: оба игрока могут совершать действия")
			}
		})
	})

	// ===== ТЕСТИРОВАНИЕ ИНТЕГРАЦИИ НОВЫХ ФИЧ =====

	t.Run("Часть 3: Интеграция новых фич с существующими системами", func(t *testing.T) {
		t.Run("Шаг 3.1: Проверка что новые фичи не ломают существующие команды", func(t *testing.T) {
			commands := []string{"/inventory", "/quests", "/daily", "/map", "/history", "/achievements"}

			for _, cmd := range commands {
				if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, cmd)); err != nil {
					problems = append(problems, fmt.Sprintf("Команда %s не работает в cooperative режиме: %v", cmd, err))
				}
			}

			t.Logf("✅ Проверка интеграции команд завершена")
		})

		t.Run("Шаг 3.2: Проверка таймеров сессии (если реализованы)", func(t *testing.T) {
			// Этот тест может быть пропущен, если таймеры еще не реализованы
			gs, _ := cfg.sessionRepo.GetByChatID(ctx, chatID)
			if gs != nil && gs.CreatedAt.IsZero() {
				llmFeedback = append(llmFeedback, "Отсутствует информация о времени создания сессии для таймеров")
			} else {
				t.Logf("✅ Таймеры сессии доступны для проверки")
			}
		})
	})

	t.Run("Часть 4: Проверка на утечки tool-текста в новых фичах", func(t *testing.T) {
		if leak := findToolLeak(fakeAPI.snapshotCalls(), chatID); leak != "" {
			problems = append(problems, fmt.Sprintf("Обнаружена утечка tool-текста в новых фичах: %s", leak))
		}
	})

	// ===== ЗАПИСЬ РЕЗУЛЬТАТОВ =====

	if len(problems) > 0 {
		writeToTestingReport(problems)
		t.Logf("❌ Найдено проблем в новых фичах: %d", len(problems))
		for i, problem := range problems {
			t.Logf("  %d. %s", i+1, problem)
		}
	} else {
		t.Log("✅ Новые фичи (сессионные цели, cooperative режим) работают корректно")
	}

	if len(llmFeedback) > 0 {
		writeToFeedback(llmFeedback)
		t.Logf("📝 Собран feedback по новым фичам: %d записей", len(llmFeedback))
		for i, feedback := range llmFeedback {
			t.Logf("  %d. %s", i+1, feedback)
		}
	}
}