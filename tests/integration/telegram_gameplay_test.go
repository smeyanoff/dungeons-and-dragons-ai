package integration

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	achievementapp "dungeons-and-dragons-ai/internal/game/application/achievement"
	characterapp "dungeons-and-dragons-ai/internal/game/application/character"
	inventoryapp "dungeons-and-dragons-ai/internal/game/application/inventory"
	questapp "dungeons-and-dragons-ai/internal/game/application/quest"
	spellapp "dungeons-and-dragons-ai/internal/game/application/spell"
	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/game/infrastructure/persistence"
)

// TestTelegramGameplay_CompleteFlow тестирует полный игровой процесс как реальный пользователь через Telegram
func TestTelegramGameplay_CompleteFlow(t *testing.T) {
	cfg := setupIntegrationTest(t)
	defer cleanupTest(t, cfg)

	ctx := cfg.ctx
	chatID := cfg.chatID
	tgUserID := cfg.tgUserID

	var problems []string
	var llmFeedback []string

	// Шаг 1: Создание новой игры (/newgame)
	t.Run("Шаг 1: Создание новой игры", func(t *testing.T) {
		world, err := cfg.initCampaignUC.Execute(ctx, "классическое фэнтези с магией")
		if err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось создать игру: %v", err))
			t.Fatalf("Не удалось создать игру: %v", err)
		}
		if world == nil {
			problems = append(problems, "Мир не создан после выполнения InitCampaign")
			t.Fatal("Мир не создан")
		}
		if len(world.Locations) == 0 {
			problems = append(problems, "Мир создан без локаций")
			t.Error("Мир создан без локаций")
		}

		gs := &session.GameSession{
			ChatID:  chatID,
			State:   session.StateActive,
			World:   *world,
			WorldID: world.ID,
		}
		if err := cfg.sessionRepo.Save(ctx, gs); err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось сохранить сессию: %v", err))
			t.Fatalf("Не удалось сохранить сессию: %v", err)
		}

		t.Logf("✅ Игра создана: мир ID=%d, локаций=%d", world.ID, len(world.Locations))
	})

	// Шаг 2: Создание персонажа (/createcharacter)
	t.Run("Шаг 2: Создание персонажа", func(t *testing.T) {
		req := characterapp.CreateCharacterRequest{
			ChatID: chatID,
			Name:   "ТестовыйГерой",
			Race:   character.RaceElf,
			Class:  character.ClassWizard,
		}

		player, err := cfg.createCharacterUC.Execute(ctx, req)
		if err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось создать персонажа: %v", err))
			t.Fatalf("Не удалось создать персонажа: %v", err)
		}
		if player == nil {
			problems = append(problems, "Персонаж не создан")
			t.Fatal("Персонаж не создан")
		}
		// Проверяем, что характеристики установлены (Stats - это структура, не указатель)
		if player.Character.Stats.Strength == 0 && player.Character.Stats.Dexterity == 0 {
			problems = append(problems, "Персонаж создан без характеристик (Stats)")
			t.Error("Персонаж создан без характеристик")
		}

		t.Logf("✅ Персонаж создан: %s (%s %s), HP=%d, MaxHP=%d",
			player.Character.Name, player.Character.Race, player.Character.Class,
			player.Character.HP, player.Character.MaxHP)
	})

	// Шаг 3: Первое игровое действие (исследование)
	t.Run("Шаг 3: Первое игровое действие", func(t *testing.T) {
		response, err := cfg.handleActionUC.Execute(ctx, chatID, "Осматриваю комнату, в которой нахожусь")
		if err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось обработать действие: %v", err))
			t.Fatalf("Не удалось обработать действие: %v", err)
		}
		if response == "" {
			problems = append(problems, "Ответ DM пуст после первого действия")
			t.Fatal("Ответ DM пуст")
		}
		if len(response) < 50 {
			llmFeedback = append(llmFeedback, fmt.Sprintf("Ответ DM слишком короткий (%d символов): %s", len(response), response))
		}

		// Проверяем, что ответ содержит описание
		if !strings.Contains(strings.ToLower(response), "комнат") && !strings.Contains(strings.ToLower(response), "локация") {
			llmFeedback = append(llmFeedback, fmt.Sprintf("Ответ DM не содержит описание локации: %s", response[:100]))
		}

		t.Logf("✅ DM ответил: %s...", response[:min(200, len(response))])
	})

	// Шаг 4: Просмотр инвентаря (/inventory)
	t.Run("Шаг 4: Просмотр инвентаря", func(t *testing.T) {
		inventoryText, err := cfg.getInventoryUC.Execute(ctx, chatID, tgUserID)
		if err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось получить инвентарь: %v", err))
			t.Fatalf("Не удалось получить инвентарь: %v", err)
		}
		if inventoryText == "" {
			problems = append(problems, "Текст инвентаря пуст")
			t.Error("Текст инвентаря пуст")
		}

		t.Logf("✅ Инвентарь: %s", inventoryText)
	})

	// Шаг 5: Исследование и подбор предмета
	t.Run("Шаг 5: Исследование и подбор предмета", func(t *testing.T) {
		// Ищем предмет
		response, err := cfg.handleActionUC.Execute(ctx, chatID, "Ищу предметы в комнате")
		if err != nil {
			t.Logf("⚠️ Действие обработано с ошибкой: %v", err)
		} else {
			t.Logf("✅ DM ответил на поиск предметов: %s...", response[:min(150, len(response))])
		}

		// Пытаемся подобрать предмет (если DM его описал)
		req := inventoryapp.AddItemRequest{
			ChatID:   chatID,
			TgUserID: tgUserID,
			ItemName: "меч",
			Quantity: 1,
		}
		result, err := cfg.addItemUC.Execute(ctx, req)
		if err != nil {
			t.Logf("⚠️ Не удалось подобрать предмет (может быть не в контексте): %v", err)
		} else {
			t.Logf("✅ Результат подбора: %s", result)
		}
	})

	// Шаг 6: Просмотр ежедневных заданий (/daily)
	t.Run("Шаг 6: Просмотр ежедневных заданий", func(t *testing.T) {
		dailyText, err := cfg.getDailyQuestsUC.Execute(ctx, chatID, tgUserID)
		if err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось получить ежедневные задания: %v", err))
			t.Fatalf("Не удалось получить ежедневные задания: %v", err)
		}
		if dailyText == "" {
			problems = append(problems, "Текст ежедневных заданий пуст")
			t.Error("Текст ежедневных заданий пуст")
		}

		// Проверяем, что есть задания
		if !strings.Contains(dailyText, "задание") && !strings.Contains(dailyText, "квест") {
			problems = append(problems, "Ежедневные задания не отображаются корректно")
		}

		t.Logf("✅ Ежедневные задания: %s", dailyText)
	})

	// Шаг 7: Просмотр квестов (/quests)
	t.Run("Шаг 7: Просмотр квестов", func(t *testing.T) {
		questsText, err := cfg.getQuestsUC.Execute(ctx, chatID)
		if err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось получить квесты: %v", err))
			t.Fatalf("Не удалось получить квесты: %v", err)
		}
		if questsText == "" {
			t.Logf("⚠️ Квесты пусты (может быть нормально для начала игры)")
		} else {
			t.Logf("✅ Квесты: %s", questsText)
		}
	})

	// Шаг 8: Бросок кубика (/roll)
	t.Run("Шаг 8: Бросок кубика", func(t *testing.T) {
		result, err := cfg.rollDiceUC.Execute(ctx, "d20")
		if err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось бросить кубик: %v", err))
			t.Fatalf("Не удалось бросить кубик: %v", err)
		}
		if result == "" {
			problems = append(problems, "Результат броска пуст")
			t.Fatal("Результат броска пуст")
		}

		t.Logf("✅ Результат броска: %s", result)
	})

	// Шаг 9: Просмотр заклинаний (/spells)
	t.Run("Шаг 9: Просмотр заклинаний", func(t *testing.T) {
		req := spellapp.GetSpellsRequest{
			ChatID:   chatID,
			TgUserID: tgUserID,
		}
		spellsText, err := cfg.getSpellsUC.Execute(ctx, req)
		if err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось получить заклинания: %v", err))
			t.Fatalf("Не удалось получить заклинания: %v", err)
		}
		if spellsText == "" {
			problems = append(problems, "Текст заклинаний пуст")
			t.Error("Текст заклинаний пуст")
		}

		t.Logf("✅ Заклинания: %s", spellsText)
	})

	// Шаг 10: Просмотр достижений (/achievements)
	t.Run("Шаг 10: Просмотр достижений", func(t *testing.T) {
		req := achievementapp.GetAchievementsRequest{
			ChatID:   chatID,
			TgUserID: tgUserID,
		}
		achievementsText, err := cfg.getAchievementsUC.Execute(ctx, req)
		if err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось получить достижения: %v", err))
			t.Fatalf("Не удалось получить достижения: %v", err)
		}
		if achievementsText == "" {
			problems = append(problems, "Текст достижений пуст")
			t.Error("Текст достижений пуст")
		}

		t.Logf("✅ Достижения: %s", achievementsText)
	})

	// Шаг 11: Просмотр карты (/map)
	t.Run("Шаг 11: Просмотр карты", func(t *testing.T) {
		mapText, err := cfg.getMapUC.Execute(ctx, chatID)
		if err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось получить карту: %v", err))
			t.Fatalf("Не удалось получить карту: %v", err)
		}
		if mapText == "" {
			problems = append(problems, "Текст карты пуст")
			t.Error("Текст карты пуст")
		}

		t.Logf("✅ Карта: %s", mapText)
	})

	// Шаг 12: Просмотр истории (/history)
	t.Run("Шаг 12: Просмотр истории", func(t *testing.T) {
		historyText, err := cfg.getHistoryUC.Execute(ctx, chatID, 10)
		if err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось получить историю: %v", err))
			t.Fatalf("Не удалось получить историю: %v", err)
		}
		if historyText == "" {
			problems = append(problems, "Текст истории пуст")
			t.Error("Текст истории пуст")
		}

		t.Logf("✅ История: %s", historyText)
	})

	// Записываем найденные проблемы
	if len(problems) > 0 {
		writeToTestingReport(problems)
	}
	if len(llmFeedback) > 0 {
		writeToFeedback(llmFeedback)
	}
}

// TestTelegramGameplay_CombatFlow тестирует боевую систему как реальный пользователь
func TestTelegramGameplay_CombatFlow(t *testing.T) {
	cfg := setupIntegrationTest(t)
	defer cleanupTest(t, cfg)

	ctx := cfg.ctx
	chatID := cfg.chatID

	var problems []string
	var llmFeedback []string

	// Создаем игру и персонажа
	world, err := cfg.initCampaignUC.Execute(ctx, "боевое фэнтези с гоблинами")
	if err != nil {
		t.Fatalf("Не удалось создать игру: %v", err)
	}

	gs := &session.GameSession{
		ChatID:  chatID,
		State:   session.StateActive,
		World:   *world,
		WorldID: world.ID,
	}
	if err := cfg.sessionRepo.Save(ctx, gs); err != nil {
		t.Fatalf("Не удалось сохранить сессию: %v", err)
	}

	req := characterapp.CreateCharacterRequest{
		ChatID: chatID,
		Name:   "Воин",
		Race:   character.RaceHuman,
		Class:  character.ClassFighter,
	}
	player, err := cfg.createCharacterUC.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Не удалось создать персонажа: %v", err)
	}

	initialHP := player.Character.HP
	t.Logf("✅ Персонаж создан: %s, HP=%d, MaxHP=%d", player.Character.Name, initialHP, player.Character.MaxHP)

	// Шаг 1: Инициация боя через действие
	t.Run("Инициация боя", func(t *testing.T) {
		response, err := cfg.handleActionUC.Execute(ctx, chatID, "Атакую гоблина мечом")
		if err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось обработать боевое действие: %v", err))
			t.Fatalf("Не удалось обработать боевое действие: %v", err)
		}
		if response == "" {
			problems = append(problems, "Ответ DM пуст при инициации боя")
			t.Fatal("Ответ DM пуст")
		}

		// Проверяем, что бой начался
		gs, err := cfg.sessionRepo.GetByChatID(ctx, chatID)
		if err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось получить сессию: %v", err))
		} else if gs != nil {
			combatRepo := persistence.NewCombatRepository(cfg.db)
			combat, err := combatRepo.GetActiveBySessionID(ctx, gs.ID)
			if err != nil {
				problems = append(problems, fmt.Sprintf("Ошибка при проверке активного боя: %v", err))
			} else if combat == nil {
				t.Logf("⚠️ Бой еще не начат (может быть нормально)")
			}
		}

		// Проверяем ответ DM на наличие боевых элементов
		hasCombatKeywords := strings.Contains(strings.ToLower(response), "бой") ||
			strings.Contains(strings.ToLower(response), "атака") ||
			strings.Contains(strings.ToLower(response), "урон") ||
			strings.Contains(strings.ToLower(response), "гоблин")
		if !hasCombatKeywords {
			llmFeedback = append(llmFeedback, fmt.Sprintf("Ответ DM при инициации боя не содержит боевых элементов: %s", response[:200]))
		}

		t.Logf("✅ Бой инициирован: %s...", response[:min(200, len(response))])
	})

	// Шаг 2: Проверка статуса боя
	t.Run("Проверка статуса боя", func(t *testing.T) {
		// Получаем сессию для проверки боя
		gs, err := cfg.sessionRepo.GetByChatID(ctx, chatID)
		if err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось получить сессию: %v", err))
			return
		}

		// Пытаемся получить активный бой
		combatRepo := persistence.NewCombatRepository(cfg.db)
		combat, err := combatRepo.GetActiveBySessionID(ctx, gs.ID)
		if err != nil {
			problems = append(problems, fmt.Sprintf("Ошибка при получении боя: %v", err))
		}

		if combat != nil {
			t.Logf("✅ Активный бой найден: участников=%d", len(combat.Participants))

			// Проверяем, что есть участники
			if len(combat.Participants) == 0 {
				problems = append(problems, "Бой создан без участников")
			}

			// Проверяем порядок ходов
			if combat.CurrentTurn < 0 || combat.CurrentTurn >= len(combat.Participants) {
				problems = append(problems, fmt.Sprintf("Некорректный индекс текущего хода: %d (участников: %d)", combat.CurrentTurn, len(combat.Participants)))
			}
		} else {
			t.Logf("⚠️ Активный бой не найден (может быть бой еще не начался)")
		}
	})

	// Шаг 3: Атака через команду /attack
	t.Run("Атака через команду", func(t *testing.T) {
		result, err := cfg.handleCombatUC.Execute(ctx, chatID, "атакую гоблина")
		if err != nil {
			// Это может быть нормально, если бой еще не начался
			t.Logf("⚠️ Не удалось атаковать: %v (может быть нет активного боя)", err)
		} else {
			if result == "" {
				problems = append(problems, "Результат атаки пуст")
			} else {
				t.Logf("✅ Результат атаки: %s", result)
			}
		}
	})

	// Шаг 4: Проверка HP после боя
	t.Run("Проверка HP после боя", func(t *testing.T) {
		// Получаем обновленного игрока через сессию
		gs, err := cfg.sessionRepo.GetByChatID(ctx, chatID)
		if err != nil {
			t.Logf("⚠️ Не удалось получить сессию: %v", err)
			return
		}
		if gs == nil || len(gs.Players) == 0 {
			t.Logf("⚠️ Игроки не найдены в сессии")
			return
		}

		updatedPlayer := gs.Players[0]
		if updatedPlayer.Character.HP < initialHP {
			t.Logf("✅ HP изменился после боя: было %d, стало %d", initialHP, updatedPlayer.Character.HP)
		} else if updatedPlayer.Character.HP > initialHP {
			llmFeedback = append(llmFeedback, fmt.Sprintf("HP увеличился после боя (неожиданно): было %d, стало %d", initialHP, updatedPlayer.Character.HP))
		}
	})

	// Записываем найденные проблемы
	if len(problems) > 0 {
		writeToTestingReport(problems)
	}
	if len(llmFeedback) > 0 {
		writeToFeedback(llmFeedback)
	}
}

// TestTelegramGameplay_DailyQuests тестирует систему ежедневных заданий
func TestTelegramGameplay_DailyQuests(t *testing.T) {
	cfg := setupIntegrationTest(t)
	defer cleanupTest(t, cfg)

	ctx := cfg.ctx
	chatID := cfg.chatID
	tgUserID := cfg.tgUserID

	var problems []string

	// Создаем игру и персонажа
	world, err := cfg.initCampaignUC.Execute(ctx, "тест")
	if err != nil {
		t.Fatalf("Не удалось создать игру: %v", err)
	}

	gs := &session.GameSession{
		ChatID:  chatID,
		State:   session.StateActive,
		World:   *world,
		WorldID: world.ID,
	}
	if err := cfg.sessionRepo.Save(ctx, gs); err != nil {
		t.Fatalf("Не удалось сохранить сессию: %v", err)
	}

	req := characterapp.CreateCharacterRequest{
		ChatID: chatID,
		Name:   "Тест",
		Race:   character.RaceHuman,
		Class:  character.ClassFighter,
	}
	_, err = cfg.createCharacterUC.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Не удалось создать персонажа: %v", err)
	}

	// Шаг 1: Получение ежедневных заданий
	t.Run("Получение ежедневных заданий", func(t *testing.T) {
		dailyText, err := cfg.getDailyQuestsUC.Execute(ctx, chatID, tgUserID)
		if err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось получить ежедневные задания: %v", err))
			t.Fatalf("Не удалось получить ежедневные задания: %v", err)
		}
		if dailyText == "" {
			problems = append(problems, "Текст ежедневных заданий пуст")
			t.Fatal("Текст ежедневных заданий пуст")
		}

		// Проверяем наличие заданий
		hasQuestTypes := strings.Contains(dailyText, "завершить квест") ||
			strings.Contains(dailyText, "победить в бою") ||
			strings.Contains(dailyText, "исследовать локацию")
		if !hasQuestTypes {
			problems = append(problems, "Ежедневные задания не содержат ожидаемых типов")
		}

		t.Logf("✅ Ежедневные задания получены: %s", dailyText)
	})

	// Шаг 2: Проверка прогресса заданий
	t.Run("Проверка прогресса заданий", func(t *testing.T) {
		// Выполняем действие, которое должно обновить прогресс
		_, err := cfg.handleActionUC.Execute(ctx, chatID, "Исследую локацию")
		if err != nil {
			t.Logf("⚠️ Действие обработано с ошибкой: %v", err)
		}

		// Проверяем прогресс через checkDailyProgressUC
		progressReq := questapp.CheckProgressRequest{
			ChatID:    chatID,
			TgUserID:  tgUserID,
			QuestType: "explore_location",
			Increment: 1,
		}
		err = cfg.checkDailyProgressUC.Execute(ctx, progressReq)
		if err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось обновить прогресс задания: %v", err))
		} else {
			t.Logf("✅ Прогресс задания обновлен")
		}
	})

	// Записываем найденные проблемы
	if len(problems) > 0 {
		writeToTestingReport(problems)
	}
}

// TestTelegramGameplay_SpellSystem тестирует систему заклинаний
func TestTelegramGameplay_SpellSystem(t *testing.T) {
	cfg := setupIntegrationTest(t)
	defer cleanupTest(t, cfg)

	ctx := cfg.ctx
	chatID := cfg.chatID
	tgUserID := cfg.tgUserID

	var problems []string

	// Создаем игру и персонажа-мага
	world, err := cfg.initCampaignUC.Execute(ctx, "магическое фэнтези")
	if err != nil {
		t.Fatalf("Не удалось создать игру: %v", err)
	}

	gs := &session.GameSession{
		ChatID:  chatID,
		State:   session.StateActive,
		World:   *world,
		WorldID: world.ID,
	}
	if err := cfg.sessionRepo.Save(ctx, gs); err != nil {
		t.Fatalf("Не удалось сохранить сессию: %v", err)
	}

	req := characterapp.CreateCharacterRequest{
		ChatID: chatID,
		Name:   "Маг",
		Race:   character.RaceElf,
		Class:  character.ClassWizard,
	}
	player, err := cfg.createCharacterUC.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Не удалось создать персонажа: %v", err)
	}

	t.Logf("✅ Персонаж-маг создан: %s", player.Character.Name)

	// Шаг 1: Просмотр заклинаний
	t.Run("Просмотр заклинаний", func(t *testing.T) {
		req := spellapp.GetSpellsRequest{
			ChatID:   chatID,
			TgUserID: tgUserID,
		}
		spellsText, err := cfg.getSpellsUC.Execute(ctx, req)
		if err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось получить заклинания: %v", err))
			t.Fatalf("Не удалось получить заклинания: %v", err)
		}
		if spellsText == "" {
			problems = append(problems, "Текст заклинаний пуст")
			t.Fatal("Текст заклинаний пуст")
		}

		// Проверяем, что есть заклинания для мага
		if !strings.Contains(strings.ToLower(spellsText), "заклинание") && !strings.Contains(strings.ToLower(spellsText), "spell") {
			problems = append(problems, "Список заклинаний не содержит ожидаемых элементов")
		}

		t.Logf("✅ Заклинания получены: %s", spellsText)
	})

	// Шаг 2: Использование заклинания
	t.Run("Использование заклинания", func(t *testing.T) {
		// Пытаемся использовать заклинание через действие
		response, err := cfg.handleActionUC.Execute(ctx, chatID, "Использую заклинание Магическая стрела")
		if err != nil {
			t.Logf("⚠️ Не удалось использовать заклинание: %v", err)
		} else {
			if response == "" {
				problems = append(problems, "Ответ DM пуст при использовании заклинания")
			} else {
				t.Logf("✅ Заклинание использовано: %s...", response[:min(200, len(response))])
			}
		}
	})

	// Записываем найденные проблемы
	if len(problems) > 0 {
		writeToTestingReport(problems)
	}
}

// Вспомогательные функции

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func writeToTestingReport(problems []string) {
	reportPath := "TESTING_REPORT.md"

	// Читаем существующий отчет
	existingContent := ""
	if data, err := os.ReadFile(reportPath); err == nil {
		existingContent = string(data)
	}

	// Добавляем новые проблемы
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	newSection := fmt.Sprintf("\n## Проблемы, найденные при интеграционном тестировании (%s)\n\n", timestamp)

	for i, problem := range problems {
		newSection += fmt.Sprintf("%d. %s\n", i+1, problem)
	}
	newSection += "\n---\n"

	// Записываем обновленный отчет
	updatedContent := existingContent + newSection
	if err := os.WriteFile(reportPath, []byte(updatedContent), 0644); err != nil {
		fmt.Printf("⚠️ Не удалось записать в TESTING_REPORT.md: %v\n", err)
	}
}

func writeToFeedback(feedback []string) {
	feedbackPath := "FEEDBACK.md"

	// Читаем существующий файл
	existingContent := ""
	if data, err := os.ReadFile(feedbackPath); err == nil {
		existingContent = string(data)
	}

	// Добавляем новую обратную связь
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	newSection := fmt.Sprintf("\n## Обратная связь от интеграционных тестов (%s)\n\n", timestamp)

	for i, item := range feedback {
		newSection += fmt.Sprintf("%d. %s\n", i+1, item)
	}
	newSection += "\n---\n"

	// Записываем обновленный файл
	updatedContent := existingContent + newSection
	if err := os.WriteFile(feedbackPath, []byte(updatedContent), 0644); err != nil {
		fmt.Printf("⚠️ Не удалось записать в FEEDBACK.md: %v\n", err)
	}
}
