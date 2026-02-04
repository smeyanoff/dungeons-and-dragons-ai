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

// TestTelegramGameplay_ComprehensiveCampaignTest
// Комплексный тест одной тестовой кампании от создания мира до первого боя
// с полным анализом промптов, контекста, запросов и ответов DM
func TestTelegramGameplay_ComprehensiveCampaignTest(t *testing.T) {
	cfg := setupTelegramGameplayTest(t)
	defer cleanupTest(t, cfg.testConfig)

	ctx := cfg.ctx
	chatID := cfg.chatID
	tgUserID := cfg.tgUserID

	var problems []string
	var promptAnalysis []string
	var contextAnalysis []string
	var responseAnalysis []string

	// Fake Telegram API server для захвата сообщений
	fakeAPI := newFakeTelegramAPI()
	srv := httptest.NewServer(fakeAPI.handler(t))
	defer srv.Close()
	apiEndpointFmt := strings.TrimRight(srv.URL, "/") + "/bot%s/%s"

	// Репозитории для теста
	eventRepo := persistence.NewGameEventRepository(cfg.db)
	combatRepo := persistence.NewCombatRepository(cfg.db)
	feedbackRepo := persistence.NewFeedbackRepository(cfg.db)
	worldEventRepo := persistence.NewWorldEventRepository(cfg.db)
	playerRepo := persistence.NewPlayerRepository(cfg.db)

	// Use cases для перемещений и проверок
	// Для теста используем nil для LLM зависимостей, так как фокусируемся на основном пути
	moveToLocationUC := mapapp.NewMoveToLocationUseCase(nil, cfg.sessionRepo, worldEventRepo, nil, nil)
	performAbilityCheckUC := abilitycheck.NewPerformAbilityCheckUseCase(cfg.sessionRepo, eventRepo, cfg.indexDocUC)

	// Создаем бота с реальным LLM
	bot, err := telegrambot.NewBotWithAPIEndpoint(
		"TEST_TOKEN",
		apiEndpointFmt,
		cfg.initCampaignUC,    // Реальный LLM для создания кампании
		cfg.handleActionUC,    // Реальный LLM для действий игрока
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
		nil, // generateImageUC - отключено для экономии
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
		cfg.indexDocUC, // Включаем индексацию для анализа RAG
	)
	if err != nil {
		t.Fatalf("Не удалось создать Telegram бота: %v", err)
	}

	// ===== ПОЛНЫЙ ТЕСТ КАМПАНИИ =====
	t.Run("Полный тест кампании: от создания мира до первого боя", func(t *testing.T) {

		// ===== ФАЗА 1: СОЗДАНИЕ ТЕСТОВОЙ КАМПАНИИ =====
		t.Log("=== ФАЗА 1: Создание тестовой кампании ===")
		if err := cfg.waitForRateLimit(ctx); err != nil {
			problems = append(problems, fmt.Sprintf("Rate limiter перед созданием кампании: %v", err))
		}

		// Фиксированная тематика для тестирования
		campaignTheme := "темное фэнтези в заброшенном королевстве с древними проклятиями, вампирами и скрытыми сокровищами"

		start := time.Now()
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/newgame "+campaignTheme)); err != nil {
			problems = append(problems, fmt.Sprintf("Создание кампании не удалось: %v", err))
			t.Fatalf("Создание кампании не удалось: %v", err)
		}
		duration := time.Since(start)

		// Проверяем создание сессии
		gs, err := cfg.sessionRepo.GetByChatID(ctx, chatID)
		if err != nil || gs == nil || !gs.IsActive() {
			problems = append(problems, "Сессия игры не создана")
			t.Fatal("Сессия игры не создана")
		}

		t.Logf("✅ Кампания создана за %.2fs", duration.Seconds())
		t.Logf("   Мир: '%s'", gs.World.Name)
		t.Logf("   Описание: '%s'", gs.World.Description)
		t.Logf("   Локаций: %d", len(gs.World.Locations))
		t.Logf("   Квест: '%s'", gs.World.MainQuest.Title)

		// Анализ промпта создания кампании
		promptAnalysis = append(promptAnalysis,
			fmt.Sprintf("Промпт создания кампании: '%s'", campaignTheme),
			fmt.Sprintf("Длительность генерации мира: %.2fs", duration.Seconds()),
			fmt.Sprintf("Количество сгенерированных локаций: %d", len(gs.World.Locations)),
		)

		// Проверяем качество генерации мира
		if len(gs.World.Locations) < 3 {
			problems = append(problems, fmt.Sprintf("Мало локаций сгенерировано: %d", len(gs.World.Locations)))
		}

		// Анализируем ответ DM
		dmResponse := lastNonThinkingPlayerFacingText(fakeAPI.snapshotCalls(), chatID)
		if dmResponse != "" {
			responseAnalysis = append(responseAnalysis,
				fmt.Sprintf("Ответ DM при создании кампании (%d символов): %s", len(dmResponse), dmResponse),
			)

			if len(dmResponse) < 200 {
				llmFeedback := []string{fmt.Sprintf("Ответ DM слишком короткий при создании кампании: %d символов", len(dmResponse))}
				writeToFeedback(llmFeedback)
			}
		}

		// ===== ФАЗА 2: СОЗДАНИЕ ПЕРСОНАЖА =====
		t.Log("=== ФАЗА 2: Создание персонажа ===")
		characterCmd := "/createcharacter Эребос Теневой human rogue"

		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, characterCmd)); err != nil {
			problems = append(problems, fmt.Sprintf("Создание персонажа не удалось: %v", err))
			t.Fatalf("Создание персонажа не удалось: %v", err)
		}

		// Проверяем создание персонажа
		gs, _ = cfg.sessionRepo.GetByChatID(ctx, chatID)
		if gs == nil || gs.GetFirstPlayer() == nil {
			problems = append(problems, "Персонаж не создан")
			t.Fatal("Персонаж не создан")
		}

		player := gs.GetFirstPlayer()
		t.Logf("✅ Персонаж создан: %s", player.Character.Name)
		t.Logf("   Раса: %s, Класс: %s", player.Character.Race, player.Character.Class)
		t.Logf("   Уровень: %d, HP: %d/%d", player.Character.Level, player.Character.HP, player.Character.MaxHP)
		t.Logf("   Характеристики: STR=%d DEX=%d CON=%d INT=%d WIS=%d CHA=%d",
			player.Character.Stats.Strength, player.Character.Stats.Dexterity,
			player.Character.Stats.Constitution, player.Character.Stats.Intelligence,
			player.Character.Stats.Wisdom, player.Character.Stats.Charisma)

		promptAnalysis = append(promptAnalysis,
			fmt.Sprintf("Команда создания персонажа: '%s'", characterCmd),
		)

		// ===== ФАЗА 3: ПЕРВОЕ ДЕЙСТВИЕ - ПОЯВЛЕНИЕ В МИРЕ =====
		t.Log("=== ФАЗА 3: Первое действие - появление в мире ===")
		if err := cfg.waitForRateLimit(ctx); err != nil {
			problems = append(problems, fmt.Sprintf("Rate limiter перед первым действием: %v", err))
		}

		action := "Осматриваюсь вокруг, оцениваю обстановку и понимаю, где я оказался"
		start = time.Now()

		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, action)); err != nil {
			problems = append(problems, fmt.Sprintf("Первое действие не удалось: %v", err))
			t.Fatalf("Первое действие не удалось: %v", err)
		}

		duration = time.Since(start)

		t.Logf("✅ Первое действие выполнено за %.2fs", duration.Seconds())

		// Анализируем промпт и контекст
		promptAnalysis = append(promptAnalysis,
			fmt.Sprintf("Действие игрока: '%s'", action),
			fmt.Sprintf("Длительность обработки: %.2fs", duration.Seconds()),
		)

		// Получаем ответ DM
		dmResponse = lastNonThinkingPlayerFacingText(fakeAPI.snapshotCalls(), chatID)
		if dmResponse != "" {
			responseAnalysis = append(responseAnalysis,
				fmt.Sprintf("Ответ DM на первое действие (%d символов): %s", len(dmResponse), dmResponse),
			)

			// Анализируем качество ответа
			if strings.Contains(dmResponse, "error") || strings.Contains(dmResponse, "Error") {
				problems = append(problems, "Ответ DM содержит ошибки")
			}

			// Проверяем наличие описания локации
			if !strings.Contains(strings.ToLower(dmResponse), "локация") &&
			   !strings.Contains(strings.ToLower(dmResponse), "место") &&
			   !strings.Contains(strings.ToLower(dmResponse), "вокруг") {
				llmFeedback := []string{"Ответ DM не содержит описания локации"}
				writeToFeedback(llmFeedback)
			}
		}

		// Получаем сессию для анализа
		gs, _ = cfg.sessionRepo.GetByChatID(ctx, chatID)

		// Анализируем события в БД
		events, err := eventRepo.GetBySessionID(ctx, gs.ID, 10)
		if err == nil && len(events) > 0 {
			contextAnalysis = append(contextAnalysis,
				fmt.Sprintf("Создано событий в БД: %d", len(events)),
			)
			for i, event := range events {
				if i < 3 { // Показываем первые 3 события
					contextAnalysis = append(contextAnalysis,
						fmt.Sprintf("Событие %d: %s", i+1, event.Content),
					)
				}
			}
		}

		// ===== ФАЗА 4: ПРОСМОТР КАРТЫ И ПЕРЕХОД В ДРУГУЮ ЛОКАЦИЮ =====
		t.Log("=== ФАЗА 4: Просмотр карты и переход в другую локацию ===")
		// Сначала смотрим карту
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/map")); err != nil {
			problems = append(problems, fmt.Sprintf("Команда /map не удалась: %v", err))
		}

		mapResponse := lastNonThinkingPlayerFacingText(fakeAPI.snapshotCalls(), chatID)
		if mapResponse != "" {
			t.Logf("Карта мира: %s", mapResponse)
			contextAnalysis = append(contextAnalysis,
				fmt.Sprintf("Ответ на /map: %s", mapResponse),
			)
		}

		// Получаем доступные локации из сессии
		gs, _ = cfg.sessionRepo.GetByChatID(ctx, chatID)
		if gs == nil || len(gs.World.Locations) < 2 {
			problems = append(problems, "Недостаточно локаций для перехода")
			t.Skip("Недостаточно локаций для перехода")
		}

		// Выбираем вторую локацию для перехода
		targetLocation := gs.World.Locations[1]
		t.Logf("Переходим в локацию: %s (%s)", targetLocation.Name, targetLocation.Description)

		if err := cfg.waitForRateLimit(ctx); err != nil {
			problems = append(problems, fmt.Sprintf("Rate limiter перед переходом: %v", err))
		}

		// Переходим в другую локацию
		moveCommand := fmt.Sprintf("Перемещаюсь в %s", targetLocation.Name)
		start = time.Now()

		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, moveCommand)); err != nil {
			problems = append(problems, fmt.Sprintf("Переход в локацию не удался: %v", err))
			t.Fatalf("Переход в локацию не удался: %v", err)
		}

		duration = time.Since(start)

		t.Logf("✅ Переход выполнен за %.2fs", duration.Seconds())

		promptAnalysis = append(promptAnalysis,
			fmt.Sprintf("Команда перехода: '%s'", moveCommand),
			fmt.Sprintf("Целевая локация: '%s' - %s", targetLocation.Name, targetLocation.Description),
			fmt.Sprintf("Длительность перехода: %.2fs", duration.Seconds()),
		)

		// Анализируем ответ DM на переход
		dmResponse = lastNonThinkingPlayerFacingText(fakeAPI.snapshotCalls(), chatID)
		if dmResponse != "" {
			responseAnalysis = append(responseAnalysis,
				fmt.Sprintf("Ответ DM на переход (%d символов): %s", len(dmResponse), dmResponse),
			)
		}

		// ===== ФАЗА 5: ИНИЦИАЦИЯ БОЯ И ПЕРВЫЙ БОЙ =====
		t.Log("=== ФАЗА 5: Инициация боя и первый бой ===")
		// Получаем текущую сессию
		gs, _ = cfg.sessionRepo.GetByChatID(ctx, chatID)
		if gs == nil {
			t.Fatal("Сессия не найдена")
		}

		if err := cfg.waitForRateLimit(ctx); err != nil {
			problems = append(problems, fmt.Sprintf("Rate limiter перед боем: %v", err))
		}

		// Действие, которое должно привести к бою
		combatAction := "Исследую темные углы и замечаю подозрительное движение - бросаюсь investigate"
		start = time.Now()

		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, combatAction)); err != nil {
			problems = append(problems, fmt.Sprintf("Действие для боя не удалось: %v", err))
			t.Fatalf("Действие для боя не удалось: %v", err)
		}

		duration = time.Since(start)

		t.Logf("✅ Действие для боя выполнено за %.2fs", duration.Seconds())

		promptAnalysis = append(promptAnalysis,
			fmt.Sprintf("Действие для инициации боя: '%s'", combatAction),
			fmt.Sprintf("Длительность обработки: %.2fs", duration.Seconds()),
		)

		// Проверяем, начался ли бой
		combat, err := combatRepo.GetActiveBySessionID(ctx, gs.ID)
		if err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось проверить активные бои: %v", err))
		}

		if combat == nil {
			t.Logf("⚠️ Бой не начался автоматически, пробуем другое действие")
			// Если бой не начался, пробуем более агрессивное действие
			combatAction2 := "Замечаю гоблина в тени и атакую его!"
			if err := cfg.waitForRateLimit(ctx); err != nil {
				problems = append(problems, fmt.Sprintf("Rate limiter перед вторым действием: %v", err))
			}

			if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, combatAction2)); err != nil {
				problems = append(problems, fmt.Sprintf("Второе действие не удалось: %v", err))
			}

			combat, _ = combatRepo.GetActiveBySessionID(ctx, gs.ID)
			promptAnalysis = append(promptAnalysis,
				fmt.Sprintf("Второе действие для боя: '%s'", combatAction2),
			)
		}

		if combat != nil {
			t.Logf("✅ Бой начат! ID=%d, участников=%d", combat.ID, len(combat.Participants))

			// Проверяем участников боя
			for i, participant := range combat.Participants {
				if participant.IsPlayer {
					t.Logf("   Игрок: %s (HP: %d/%d)", participant.Character.Name, participant.Character.HP, participant.Character.MaxHP)
				} else {
					t.Logf("   Враг %d: %s (HP: %d/%d, AC: %d)", i, participant.MonsterName, participant.MonsterHP, participant.MonsterMaxHP, participant.MonsterAC)
				}
			}

			contextAnalysis = append(contextAnalysis,
				fmt.Sprintf("Бой начат с %d участниками", len(combat.Participants)),
			)

			// Выполняем атаку
			if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/attack")); err != nil {
				problems = append(problems, fmt.Sprintf("Команда /attack не удалась: %v", err))
			}

			// Проверяем результат атаки
			attackResponse := lastNonThinkingPlayerFacingText(fakeAPI.snapshotCalls(), chatID)
			if attackResponse != "" {
				responseAnalysis = append(responseAnalysis,
					fmt.Sprintf("Результат атаки (%d символов): %s", len(attackResponse), attackResponse),
				)
				t.Logf("Результат атаки: %s", attackResponse)
			}
		} else {
			t.Logf("⚠️ Бой так и не начался - возможно, DM решил не создавать бой в этой ситуации")
			llmFeedback := []string{"DM не инициировал бой при агрессивном действии игрока"}
			writeToFeedback(llmFeedback)
		}

		// ===== АНАЛИЗ РЕЗУЛЬТАТОВ =====
		t.Log("=== АНАЛИЗ РЕЗУЛЬТАТОВ КАМПАНИИ ===")
		t.Logf("=== АНАЛИЗ ПРОМПТОВ ===")
		for _, analysis := range promptAnalysis {
			t.Logf("• %s", analysis)
		}

		t.Logf("=== АНАЛИЗ КОНТЕКСТА ===")
		for _, analysis := range contextAnalysis {
			t.Logf("• %s", analysis)
		}

		t.Logf("=== АНАЛИЗ ОТВЕТОВ DM ===")
		for _, analysis := range responseAnalysis {
			t.Logf("• %s", analysis)
		}

		t.Logf("=== ПРОБЛЕМЫ ===")
		if len(problems) == 0 {
			t.Logf("✅ Проблем не обнаружено")
		} else {
			for _, problem := range problems {
				t.Logf("❌ %s", problem)
			}
		}

		// Записываем результаты в TESTING_REPORT.md
		reportProblems := problems
		reportFeedback := []string{}

		// Добавляем аналитику в feedback
		if len(promptAnalysis) > 0 {
			reportFeedback = append(reportFeedback, "=== АНАЛИЗ ПРОМПТОВ ===")
			reportFeedback = append(reportFeedback, promptAnalysis...)
		}

		if len(contextAnalysis) > 0 {
			reportFeedback = append(reportFeedback, "=== АНАЛИЗ КОНТЕКСТА ===")
			reportFeedback = append(reportFeedback, contextAnalysis...)
		}

		if len(responseAnalysis) > 0 {
			reportFeedback = append(reportFeedback, "=== АНАЛИЗ ОТВЕТОВ DM ===")
			reportFeedback = append(reportFeedback, responseAnalysis...)
		}

		if len(reportProblems) > 0 {
			writeToTestingReport(reportProblems)
		}

		if len(reportFeedback) > 0 {
			writeToFeedback(reportFeedback)
		}

		// Финальная проверка состояния игры
		finalGS, _ := cfg.sessionRepo.GetByChatID(ctx, chatID)
		if finalGS != nil {
			t.Logf("=== ФИНАЛЬНОЕ СОСТОЯНИЕ ИГРЫ ===")
			t.Logf("Сессия активна: %v", finalGS.IsActive())
			if finalGS.GetFirstPlayer() != nil {
				player := finalGS.GetFirstPlayer()
				t.Logf("Персонаж: %s (HP: %d/%d)", player.Character.Name, player.Character.HP, player.Character.MaxHP)
			}
		}
	})
}