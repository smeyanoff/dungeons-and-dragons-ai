package integration

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	characterapp "dungeons-and-dragons-ai/internal/game/application/character"
	questapp "dungeons-and-dragons-ai/internal/game/application/quest"
	"dungeons-and-dragons-ai/internal/game/domain/quest"
	questdomain "dungeons-and-dragons-ai/internal/game/domain/quest"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	worlddomain "dungeons-and-dragons-ai/internal/game/domain/world"
	"dungeons-and-dragons-ai/internal/game/infrastructure/persistence"
	telegrambot "dungeons-and-dragons-ai/internal/telegram"
)

// TestTelegramDailyQuestsCommand проверяет команду /daily для просмотра ежедневных заданий
func TestTelegramDailyQuestsCommand(t *testing.T) {
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
	feedbackRepo := persistence.NewFeedbackRepository(cfg.db)

	// Создаем deterministic world+session
	q := &questdomain.Quest{Title: "Test Quest (Daily)", Description: "Deterministic quest for daily quests testing"}
	w := worlddomain.New("Test World (Daily)")
	w.Description = "Deterministic test world for daily quests"
	w.SetMainQuest(q)
	w.Locations = []worlddomain.Location{{Name: "Start", Description: "Start location"}}
	if err := cfg.worldRepo.Save(ctx, w); err != nil {
		t.Fatalf("Не удалось сохранить тестовый мир: %v", err)
	}
	gs := &session.GameSession{ChatID: chatID, State: session.StateActive, World: *w, WorldID: w.ID}
	if err := cfg.sessionRepo.Save(ctx, gs); err != nil {
		t.Fatalf("Не удалось сохранить сессию: %v", err)
	}

	// Создаем персонажа
	createCharacterUC := characterapp.NewCreateCharacterUseCase(cfg.sessionRepo, cfg.playerRepo)
	player, err := createCharacterUC.Execute(ctx, newCharacterRequest(chatID))
	if err != nil {
		t.Fatalf("Не удалось создать персонажа: %v", err)
	}

	// Создаем use case для daily quests
	dailyQuestRepo := persistence.NewDailyQuestRepository(cfg.db)
	getDailyQuestsUC := questapp.NewGetDailyQuestsUseCase(cfg.sessionRepo, dailyQuestRepo, cfg.playerRepo)

	// Создаем bot с daily quests use case
	bot, err := telegrambot.NewBotWithAPIEndpoint(
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
		getDailyQuestsUC,
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
		nil, // combatRepo
		feedbackRepo,
		nil, // eventRepo
		nil, // indexDocUC
	)
	if err != nil {
		t.Fatalf("Не удалось создать Telegram bot: %v", err)
	}

	// Создаем тестовые ежедневные задания
	today := time.Now()
	dailyQuests := []*quest.DailyQuest{
		quest.NewDailyQuest(quest.DailyQuestTypeCompleteQuest, "Завершить квест", "Завершите любой квест в игре", 1, 50, 25),
		quest.NewDailyQuest(quest.DailyQuestTypeWinCombat, "Победить в бою", "Выиграйте сражение с монстрами", 2, 75, 50),
		quest.NewDailyQuest(quest.DailyQuestTypeExploreLocation, "Исследовать локацию", "Посетите новую локацию на карте", 3, 30, 15),
	}

	for _, dq := range dailyQuests {
		dq.CreatedAt = today
		if err := cfg.db.Create(dq).Error; err != nil {
			t.Fatalf("Не удалось сохранить ежедневное задание: %v", err)
		}
	}

	// Создаем прогресс для игрока (некоторые выполнены, некоторые нет)
	progresses := []*quest.DailyQuestProgress{
		{
			PlayerID:     player.ID,
			DailyQuestID: dailyQuests[0].ID,
			CurrentValue: 1,
			TargetValue:  1,
			Completed:    true,
			Date:         today,
			CompletedAt:  &today,
		},
		{
			PlayerID:     player.ID,
			DailyQuestID: dailyQuests[1].ID,
			CurrentValue: 1,
			TargetValue:  2,
			Completed:    false,
			Date:         today,
		},
		// Третье задание без прогресса (не начато)
	}

	for _, p := range progresses {
		if err := dailyQuestRepo.SaveProgress(ctx, p); err != nil {
			t.Fatalf("Не удалось сохранить прогресс: %v", err)
		}
	}

	// Создаем стрик для игрока
	streak := &quest.DailyQuestStreak{
		PlayerID:   player.ID,
		StreakDays: 5,
		LastDate:   today,
	}
	if err := dailyQuestRepo.UpdateStreak(ctx, streak); err != nil {
		t.Fatalf("Не удалось сохранить стрик: %v", err)
	}

	// Выполняем команду /daily
	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/daily")); err != nil {
		t.Fatalf("Ошибка при /daily: %v", err)
	}

	// Проверяем ответ
	lastMsg := lastMessageText(fakeAPI, chatID)
	if lastMsg == "" {
		t.Fatal("Ожидалось сообщение с ежедневными заданиями, но сообщений нет")
	}

	// Проверяем наличие заголовка (может быть в любом сообщении из-за разбиения на части)
	allMsgs := allMessagesText(fakeAPI, chatID)
	t.Logf("Все сообщения: %s", allMsgs)
	if !strings.Contains(allMsgs, "Ежедневные задания") {
		t.Fatalf("Ожидался заголовок 'Ежедневные задания' в сообщениях, получили: %s", allMsgs)
	}

	// Проверяем наличие стриков
	if !strings.Contains(allMsgs, "Стрик") || !strings.Contains(allMsgs, "5 дней") {
		t.Fatalf("Ожидалось отображение стриков, получили: %s", allMsgs)
	}

	// Проверяем наличие заданий
	for _, dq := range dailyQuests {
		if !strings.Contains(lastMsg, dq.Title) {
			t.Fatalf("Ожидалось задание '%s', получили: %s", dq.Title, lastMsg)
		}
	}

	// Проверяем статусы заданий
	if !strings.Contains(lastMsg, "✅") { // Выполненное задание
		t.Fatalf("Ожидался статус выполненного задания (✅), получили: %s", lastMsg)
	}

	if !strings.Contains(lastMsg, "🔄") { // Задание в процессе
		t.Fatalf("Ожидался статус задания в процессе (🔄), получили: %s", lastMsg)
	}

	if !strings.Contains(lastMsg, "⚪") { // Не начатое задание
		t.Fatalf("Ожидался статус не начатого задания (⚪), получили: %s", lastMsg)
	}

	// Проверяем наличие наград
	if !strings.Contains(lastMsg, "опыта") || !strings.Contains(lastMsg, "золота") {
		t.Fatalf("Ожидались награды (опыт и золото), получили: %s", lastMsg)
	}

	// Проверяем информацию о еженедельном бонусе
	if !strings.Contains(lastMsg, "Еженедельный бонус") {
		t.Fatalf("Ожидалась информация о еженедельном бонусе, получили: %s", lastMsg)
	}
}

// TestTelegramDailyQuestsNoCharacter проверяет команду /daily когда персонаж не создан
func TestTelegramDailyQuestsNoCharacter(t *testing.T) {
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
	feedbackRepo := persistence.NewFeedbackRepository(cfg.db)

	// Создаем deterministic world+session без персонажа
	q := &questdomain.Quest{Title: "Test Quest (No Character)", Description: "Deterministic quest for testing without character"}
	w := worlddomain.New("Test World (No Character)")
	w.Description = "Deterministic test world without character"
	w.SetMainQuest(q)
	w.Locations = []worlddomain.Location{{Name: "Start", Description: "Start location"}}
	if err := cfg.worldRepo.Save(ctx, w); err != nil {
		t.Fatalf("Не удалось сохранить тестовый мир: %v", err)
	}
	gs := &session.GameSession{ChatID: chatID, State: session.StateActive, World: *w, WorldID: w.ID}
	if err := cfg.sessionRepo.Save(ctx, gs); err != nil {
		t.Fatalf("Не удалось сохранить сессию: %v", err)
	}

	// Создаем use case для daily quests
	dailyQuestRepo := persistence.NewDailyQuestRepository(cfg.db)
	getDailyQuestsUC := questapp.NewGetDailyQuestsUseCase(cfg.sessionRepo, dailyQuestRepo, cfg.playerRepo)

	// Создаем bot
	bot, err := telegrambot.NewBotWithAPIEndpoint(
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
		getDailyQuestsUC,
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
		nil, // combatRepo
		feedbackRepo,
		nil, // eventRepo
		nil, // indexDocUC
	)
	if err != nil {
		t.Fatalf("Не удалось создать Telegram bot: %v", err)
	}

	// Выполняем команду /daily без персонажа
	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/daily")); err != nil {
		t.Fatalf("Ошибка при /daily без персонажа: %v", err)
	}

	// Проверяем ответ - должно быть сообщение об ошибке
	lastMsg := lastMessageText(fakeAPI, chatID)
	if lastMsg == "" {
		t.Fatal("Ожидалось сообщение об ошибке, но сообщений нет")
	}

	if !strings.Contains(lastMsg, "character not created") || !strings.Contains(lastMsg, "createcharacter") {
		t.Fatalf("Ожидалось сообщение о необходимости создать персонажа, получили: %s", lastMsg)
	}
}

// TestTelegramDailyQuestsProgressUpdate проверяет обновление прогресса ежедневных заданий
func TestTelegramDailyQuestsProgressUpdate(t *testing.T) {
	cfg := setupInfraOnlyIntegrationTest(t)
	if cfg == nil {
		return
	}
	defer cleanupTest(t, &testConfig{db: cfg.db, chatID: cfg.chatID, tgUserID: cfg.tgUserID})

	ctx := cfg.ctx
	chatID := cfg.chatID

	dailyQuestRepo := persistence.NewDailyQuestRepository(cfg.db)

	// Создаем deterministic world+session
	q := &questdomain.Quest{Title: "Test Quest (Progress)", Description: "Deterministic quest for progress testing"}
	w := worlddomain.New("Test World (Progress)")
	w.Description = "Deterministic test world for progress testing"
	w.SetMainQuest(q)
	w.Locations = []worlddomain.Location{{Name: "Start", Description: "Start location"}}
	if err := cfg.worldRepo.Save(ctx, w); err != nil {
		t.Fatalf("Не удалось сохранить тестовый мир: %v", err)
	}
	gs := &session.GameSession{ChatID: chatID, State: session.StateActive, World: *w, WorldID: w.ID}
	if err := cfg.sessionRepo.Save(ctx, gs); err != nil {
		t.Fatalf("Не удалось сохранить сессию: %v", err)
	}

	// Создаем персонажа
	createCharacterUC := characterapp.NewCreateCharacterUseCase(cfg.sessionRepo, cfg.playerRepo)
	player, err := createCharacterUC.Execute(ctx, newCharacterRequest(chatID))
	if err != nil {
		t.Fatalf("Не удалось создать персонажа: %v", err)
	}

	// Создаем ежедневное задание "завершить квест"
	today := time.Now()
	dailyQuest := quest.NewDailyQuest(quest.DailyQuestTypeCompleteQuest, "Завершить квест", "Завершите любой квест", 1, 50, 25)
	dailyQuest.CreatedAt = today
	if err := cfg.db.Create(dailyQuest).Error; err != nil {
		t.Fatalf("Не удалось сохранить ежедневное задание: %v", err)
	}

	// Создаем начальный прогресс (0/1)
	initialProgress := &quest.DailyQuestProgress{
		PlayerID:     player.ID,
		DailyQuestID: dailyQuest.ID,
		CurrentValue: 0,
		TargetValue:  1,
		Completed:    false,
		Date:         today,
	}
	if err := dailyQuestRepo.SaveProgress(ctx, initialProgress); err != nil {
		t.Fatalf("Не удалось сохранить начальный прогресс: %v", err)
	}

	// Проверяем, что прогресс не завершен
	if initialProgress.IsCompleted() {
		t.Fatal("Начальный прогресс не должен быть завершенным")
	}

	// Обновляем прогресс (увеличиваем на 1)
	initialProgress.IncrementProgress(1)

	// Проверяем, что прогресс теперь завершен
	if !initialProgress.IsCompleted() {
		t.Fatal("После увеличения прогресса задание должно быть завершено")
	}

	if initialProgress.CurrentValue != 1 {
		t.Fatalf("Ожидалось CurrentValue=1, получили %d", initialProgress.CurrentValue)
	}

	// Сохраняем обновленный прогресс
	if err := dailyQuestRepo.SaveProgress(ctx, initialProgress); err != nil {
		t.Fatalf("Не удалось сохранить обновленный прогресс: %v", err)
	}

	// Проверяем, что в БД прогресс сохранился корректно
	savedProgress, err := dailyQuestRepo.GetPlayerProgress(ctx, player.ID, today)
	if err != nil {
		t.Fatalf("Не удалось получить прогресс из БД: %v", err)
	}

	if len(savedProgress) == 0 {
		t.Fatal("Прогресс не найден в БД")
	}

	foundProgress := false
	for _, p := range savedProgress {
		if p.DailyQuestID == dailyQuest.ID {
			foundProgress = true
			if !p.IsCompleted() {
				t.Fatal("Прогресс в БД должен быть завершенным")
			}
			if p.CurrentValue != 1 {
				t.Fatalf("CurrentValue в БД должен быть 1, получили %d", p.CurrentValue)
			}
			if p.CompletedAt == nil {
				t.Fatal("CompletedAt должно быть установлено")
			}
			break
		}
	}

	if !foundProgress {
		t.Fatal("Прогресс для задания не найден в БД")
	}
}

// TestTelegramDailyQuestsStreakUpdate проверяет обновление стриков
func TestTelegramDailyQuestsStreakUpdate(t *testing.T) {
	cfg := setupInfraOnlyIntegrationTest(t)
	if cfg == nil {
		return
	}
	defer cleanupTest(t, &testConfig{db: cfg.db, chatID: cfg.chatID, tgUserID: cfg.tgUserID})

	ctx := cfg.ctx
	dailyQuestRepo := persistence.NewDailyQuestRepository(cfg.db)

	// Создаем deterministic world+session
	q := &questdomain.Quest{Title: "Test Quest (Streak)", Description: "Deterministic quest for streak testing"}
	w := worlddomain.New("Test World (Streak)")
	w.Description = "Deterministic test world for streak testing"
	w.SetMainQuest(q)
	w.Locations = []worlddomain.Location{{Name: "Start", Description: "Start location"}}
	if err := cfg.worldRepo.Save(ctx, w); err != nil {
		t.Fatalf("Не удалось сохранить тестовый мир: %v", err)
	}
	gs := &session.GameSession{ChatID: cfg.chatID, State: session.StateActive, World: *w, WorldID: w.ID}
	if err := cfg.sessionRepo.Save(ctx, gs); err != nil {
		t.Fatalf("Не удалось сохранить сессию: %v", err)
	}

	// Создаем персонажа
	createCharacterUC := characterapp.NewCreateCharacterUseCase(cfg.sessionRepo, cfg.playerRepo)
	player, err := createCharacterUC.Execute(ctx, newCharacterRequest(cfg.chatID))
	if err != nil {
		t.Fatalf("Не удалось создать персонажа: %v", err)
	}

	// Очищаем существующие streaks для этого player ID
	cfg.db.Where("player_id = ?", player.ID).Delete(&quest.DailyQuestStreak{})

	// Создаем начальный стрик через прямое создание в БД
	initialStreak := &quest.DailyQuestStreak{
		PlayerID:   player.ID,
		StreakDays: 3,
		LastDate:   time.Now().AddDate(0, 0, -1), // Вчера
	}
	if err := cfg.db.Create(initialStreak).Error; err != nil {
		t.Fatalf("Не удалось сохранить начальный стрик: %v", err)
	}

	// Получаем стрик из БД
	savedStreak, err := dailyQuestRepo.GetStreak(ctx, player.ID)
	if err != nil {
		t.Fatalf("Не удалось получить стрик из БД: %v", err)
	}

	if savedStreak.StreakDays != 3 {
		t.Fatalf("Ожидалось StreakDays=3, получили %d", savedStreak.StreakDays)
	}

	// Обновляем стрик (сегодня выполнены задания)
	initialStreak.StreakDays = 4
	initialStreak.LastDate = time.Now() // Сегодня
	if err := dailyQuestRepo.UpdateStreak(ctx, initialStreak); err != nil {
		t.Fatalf("Не удалось обновить стрик: %v", err)
	}

	// Проверяем обновленный стрик
	finalStreak, err := dailyQuestRepo.GetStreak(ctx, player.ID)
	if err != nil {
		t.Fatalf("Не удалось получить обновленный стрик: %v", err)
	}

	if finalStreak.StreakDays != 4 {
		t.Fatalf("Ожидалось StreakDays=4, получили %d", finalStreak.StreakDays)
	}
}

func allMessagesText(fakeAPI *fakeTelegramAPI, chatID int64) string {
	fakeAPI.mu.Lock()
	defer fakeAPI.mu.Unlock()
	var messages []string
	for i := len(fakeAPI.calls) - 1; i >= 0; i-- {
		call := fakeAPI.calls[i]
		if call.ChatID == chatID && call.Method == "sendMessage" {
			messages = append([]string{call.Text}, messages...) // Добавляем в начало для правильного порядка
		}
	}
	return strings.Join(messages, "\n\n")
}
