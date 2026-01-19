package quest

import (
	"context"
	"testing"
	"time"

	characterapp "dungeons-and-dragons-ai/internal/game/application/character"
	"dungeons-and-dragons-ai/internal/game/domain/player"
	"dungeons-and-dragons-ai/internal/game/domain/quest"
	"dungeons-and-dragons-ai/internal/game/domain/session"
)

// Mock Session Repository
type mockCompleteDailyQuestSessionRepo struct {
	getByChatIDFunc func(ctx context.Context, chatID int64) (*session.GameSession, error)
}

func (m *mockCompleteDailyQuestSessionRepo) GetByChatID(ctx context.Context, chatID int64) (*session.GameSession, error) {
	if m.getByChatIDFunc != nil {
		return m.getByChatIDFunc(ctx, chatID)
	}
	return nil, nil
}

func (m *mockCompleteDailyQuestSessionRepo) Save(ctx context.Context, session *session.GameSession) error {
	return nil
}

func (m *mockCompleteDailyQuestSessionRepo) Delete(ctx context.Context, chatID int64) error {
	return nil
}

// Mock Daily Quest Repository
type mockCompleteDailyQuestDailyQuestRepo struct {
	getTodayQuestsFunc      func(ctx context.Context) ([]*quest.DailyQuest, error)
	getOrCreateProgressFunc func(ctx context.Context, playerID uint, dailyQuestID uint, date time.Time) (*quest.DailyQuestProgress, error)
	saveProgressFunc        func(ctx context.Context, progress *quest.DailyQuestProgress) error
	getPlayerProgressFunc   func(ctx context.Context, playerID uint, date time.Time) ([]*quest.DailyQuestProgress, error)
	getStreakFunc           func(ctx context.Context, playerID uint) (*quest.DailyQuestStreak, error)
	updateStreakFunc        func(ctx context.Context, streak *quest.DailyQuestStreak) error
}

func (m *mockCompleteDailyQuestDailyQuestRepo) GetTodayQuests(ctx context.Context) ([]*quest.DailyQuest, error) {
	if m.getTodayQuestsFunc != nil {
		return m.getTodayQuestsFunc(ctx)
	}
	return []*quest.DailyQuest{}, nil
}

func (m *mockCompleteDailyQuestDailyQuestRepo) GetOrCreateProgress(ctx context.Context, playerID uint, dailyQuestID uint, date time.Time) (*quest.DailyQuestProgress, error) {
	if m.getOrCreateProgressFunc != nil {
		return m.getOrCreateProgressFunc(ctx, playerID, dailyQuestID, date)
	}
	return nil, nil
}

func (m *mockCompleteDailyQuestDailyQuestRepo) SaveProgress(ctx context.Context, progress *quest.DailyQuestProgress) error {
	if m.saveProgressFunc != nil {
		return m.saveProgressFunc(ctx, progress)
	}
	return nil
}

func (m *mockCompleteDailyQuestDailyQuestRepo) GetPlayerProgress(ctx context.Context, playerID uint, date time.Time) ([]*quest.DailyQuestProgress, error) {
	if m.getPlayerProgressFunc != nil {
		return m.getPlayerProgressFunc(ctx, playerID, date)
	}
	return []*quest.DailyQuestProgress{}, nil
}

func (m *mockCompleteDailyQuestDailyQuestRepo) GetStreak(ctx context.Context, playerID uint) (*quest.DailyQuestStreak, error) {
	if m.getStreakFunc != nil {
		return m.getStreakFunc(ctx, playerID)
	}
	return &quest.DailyQuestStreak{StreakDays: 0}, nil
}

func (m *mockCompleteDailyQuestDailyQuestRepo) UpdateStreak(ctx context.Context, streak *quest.DailyQuestStreak) error {
	if m.updateStreakFunc != nil {
		return m.updateStreakFunc(ctx, streak)
	}
	return nil
}

// Mock Player Repository
type mockCompleteDailyQuestPlayerRepo struct {
	getByTgUserIDAndSessionIDFunc func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error)
	saveFunc                      func(ctx context.Context, p *player.Player) error
}

func (m *mockCompleteDailyQuestPlayerRepo) GetByTgUserIDAndSessionID(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
	if m.getByTgUserIDAndSessionIDFunc != nil {
		return m.getByTgUserIDAndSessionIDFunc(ctx, tgUserID, sessionID)
	}
	return nil, nil
}

func (m *mockCompleteDailyQuestPlayerRepo) Save(ctx context.Context, p *player.Player) error {
	if m.saveFunc != nil {
		return m.saveFunc(ctx, p)
	}
	return nil
}

// Mock Add Experience Use Case - используем реальный тип, но создаем обертку для тестирования
// В реальных тестах можно использовать реальный use case с моками репозиториев

func TestCompleteDailyQuestUseCase_Execute_Success(t *testing.T) {
	ctx := context.Background()
	chatID := int64(123)
	tgUserID := int64(456)

	activeSession := &session.GameSession{
		ChatID: chatID,
		State:  session.StateActive,
	}
	activeSession.ID = 1

	testPlayer := &player.Player{
		ID:            1,
		TgUserID:      tgUserID,
		GameSessionID: 1,
	}

	targetQuest := quest.NewDailyQuest(
		quest.DailyQuestTypeCompleteQuest,
		"Завершить квест",
		"Завершите любой активный квест",
		1,
		50,
		10,
	)
	targetQuest.ID = 1

	dailyQuests := []*quest.DailyQuest{targetQuest}

	progress := &quest.DailyQuestProgress{
		ID:           1,
		PlayerID:     1,
		DailyQuestID: 1,
		CurrentValue: 1,
		TargetValue:  1,
		Completed:    false,
		Date:         time.Now(),
	}

	// Создаем копию для allProgress, где задание завершено
	completedProgress := &quest.DailyQuestProgress{
		ID:           1,
		PlayerID:     1,
		DailyQuestID: 1,
		CurrentValue: 1,
		TargetValue:  1,
		Completed:    true,
		Date:         time.Now(),
	}
	allProgress := []*quest.DailyQuestProgress{completedProgress}

	streak := &quest.DailyQuestStreak{
		ID:         1,
		PlayerID:   1,
		StreakDays: 0,
		LastDate:   time.Time{},
	}

	var savedProgress *quest.DailyQuestProgress

	sessionRepo := &mockCompleteDailyQuestSessionRepo{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			return activeSession, nil
		},
	}

	dailyQuestRepo := &mockCompleteDailyQuestDailyQuestRepo{
		getTodayQuestsFunc: func(ctx context.Context) ([]*quest.DailyQuest, error) {
			return dailyQuests, nil
		},
		getOrCreateProgressFunc: func(ctx context.Context, playerID uint, dailyQuestID uint, date time.Time) (*quest.DailyQuestProgress, error) {
			return progress, nil
		},
		saveProgressFunc: func(ctx context.Context, p *quest.DailyQuestProgress) error {
			savedProgress = p
			return nil
		},
		getPlayerProgressFunc: func(ctx context.Context, playerID uint, date time.Time) ([]*quest.DailyQuestProgress, error) {
			return allProgress, nil
		},
		getStreakFunc: func(ctx context.Context, playerID uint) (*quest.DailyQuestStreak, error) {
			return streak, nil
		},
		updateStreakFunc: func(ctx context.Context, s *quest.DailyQuestStreak) error {
			return nil
		},
	}

	playerRepo := &mockCompleteDailyQuestPlayerRepo{
		getByTgUserIDAndSessionIDFunc: func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
			return testPlayer, nil
		},
	}

	// Создаем реальный use case с моками для AddExperience
	mockPlayerRepoForExp := &mockCompleteDailyQuestPlayerRepo{
		getByTgUserIDAndSessionIDFunc: func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
			return testPlayer, nil
		},
		saveFunc: func(ctx context.Context, p *player.Player) error {
			return nil
		},
	}
	mockSessionRepoForExp := &mockCompleteDailyQuestSessionRepo{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			return activeSession, nil
		},
	}
	addExpUC := characterapp.NewAddExperienceUseCase(mockPlayerRepoForExp, mockSessionRepoForExp)

	uc := NewCompleteDailyQuestUseCase(sessionRepo, dailyQuestRepo, playerRepo, addExpUC)

	req := CompleteDailyQuestRequest{
		ChatID:    chatID,
		TgUserID:  tgUserID,
		QuestType: quest.DailyQuestTypeCompleteQuest,
	}

	err := uc.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if savedProgress == nil {
		t.Fatal("Execute() should save progress")
	}

	if !savedProgress.Completed {
		t.Error("Execute() should mark progress as completed")
	}

	if savedProgress.CompletedAt == nil {
		t.Error("Execute() should set CompletedAt")
	}
}

func TestCompleteDailyQuestUseCase_Execute_AlreadyCompleted(t *testing.T) {
	ctx := context.Background()
	chatID := int64(123)
	tgUserID := int64(456)

	activeSession := &session.GameSession{
		ChatID: chatID,
		State:  session.StateActive,
	}
	activeSession.ID = 1

	testPlayer := &player.Player{
		ID:            1,
		TgUserID:      tgUserID,
		GameSessionID: 1,
	}

	targetQuest := quest.NewDailyQuest(
		quest.DailyQuestTypeCompleteQuest,
		"Завершить квест",
		"Завершите любой активный квест",
		1,
		50,
		10,
	)
	targetQuest.ID = 1

	dailyQuests := []*quest.DailyQuest{targetQuest}

	now := time.Now()
	progress := &quest.DailyQuestProgress{
		ID:           1,
		PlayerID:     1,
		DailyQuestID: 1,
		CurrentValue: 1,
		TargetValue:  1,
		Completed:    true,
		CompletedAt:  &now,
		Date:         time.Now(),
	}

	var savedProgress *quest.DailyQuestProgress

	sessionRepo := &mockCompleteDailyQuestSessionRepo{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			return activeSession, nil
		},
	}

	dailyQuestRepo := &mockCompleteDailyQuestDailyQuestRepo{
		getTodayQuestsFunc: func(ctx context.Context) ([]*quest.DailyQuest, error) {
			return dailyQuests, nil
		},
		getOrCreateProgressFunc: func(ctx context.Context, playerID uint, dailyQuestID uint, date time.Time) (*quest.DailyQuestProgress, error) {
			return progress, nil
		},
		saveProgressFunc: func(ctx context.Context, p *quest.DailyQuestProgress) error {
			savedProgress = p
			return nil
		},
	}

	playerRepo := &mockCompleteDailyQuestPlayerRepo{
		getByTgUserIDAndSessionIDFunc: func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
			return testPlayer, nil
		},
	}

	mockPlayerRepoForExp := &mockCompleteDailyQuestPlayerRepo{
		getByTgUserIDAndSessionIDFunc: func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
			return testPlayer, nil
		},
		saveFunc: func(ctx context.Context, p *player.Player) error {
			return nil
		},
	}
	mockSessionRepoForExp := &mockCompleteDailyQuestSessionRepo{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			return activeSession, nil
		},
	}
	addExpUC := characterapp.NewAddExperienceUseCase(mockPlayerRepoForExp, mockSessionRepoForExp)

	uc := NewCompleteDailyQuestUseCase(sessionRepo, dailyQuestRepo, playerRepo, addExpUC)

	req := CompleteDailyQuestRequest{
		ChatID:    chatID,
		TgUserID:  tgUserID,
		QuestType: quest.DailyQuestTypeCompleteQuest,
	}

	err := uc.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	// Прогресс не должен быть сохранен, так как задание уже завершено
	if savedProgress != nil {
		t.Error("Execute() should not save progress if already completed")
	}
}

func TestCompleteDailyQuestUseCase_Execute_NotCompletedYet(t *testing.T) {
	ctx := context.Background()
	chatID := int64(123)
	tgUserID := int64(456)

	activeSession := &session.GameSession{
		ChatID: chatID,
		State:  session.StateActive,
	}
	activeSession.ID = 1

	testPlayer := &player.Player{
		ID:            1,
		TgUserID:      tgUserID,
		GameSessionID: 1,
	}

	targetQuest := quest.NewDailyQuest(
		quest.DailyQuestTypeCompleteQuest,
		"Завершить квест",
		"Завершите любой активный квест",
		1,
		50,
		10,
	)
	targetQuest.ID = 1

	dailyQuests := []*quest.DailyQuest{targetQuest}

	progress := &quest.DailyQuestProgress{
		ID:           1,
		PlayerID:     1,
		DailyQuestID: 1,
		CurrentValue: 0,
		TargetValue:  1,
		Completed:    false,
		Date:         time.Now(),
	}

	var savedProgress *quest.DailyQuestProgress

	sessionRepo := &mockCompleteDailyQuestSessionRepo{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			return activeSession, nil
		},
	}

	dailyQuestRepo := &mockCompleteDailyQuestDailyQuestRepo{
		getTodayQuestsFunc: func(ctx context.Context) ([]*quest.DailyQuest, error) {
			return dailyQuests, nil
		},
		getOrCreateProgressFunc: func(ctx context.Context, playerID uint, dailyQuestID uint, date time.Time) (*quest.DailyQuestProgress, error) {
			return progress, nil
		},
		saveProgressFunc: func(ctx context.Context, p *quest.DailyQuestProgress) error {
			savedProgress = p
			return nil
		},
	}

	playerRepo := &mockCompleteDailyQuestPlayerRepo{
		getByTgUserIDAndSessionIDFunc: func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
			return testPlayer, nil
		},
	}

	mockPlayerRepoForExp := &mockCompleteDailyQuestPlayerRepo{
		getByTgUserIDAndSessionIDFunc: func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
			return testPlayer, nil
		},
		saveFunc: func(ctx context.Context, p *player.Player) error {
			return nil
		},
	}
	mockSessionRepoForExp := &mockCompleteDailyQuestSessionRepo{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			return activeSession, nil
		},
	}
	addExpUC := characterapp.NewAddExperienceUseCase(mockPlayerRepoForExp, mockSessionRepoForExp)

	uc := NewCompleteDailyQuestUseCase(sessionRepo, dailyQuestRepo, playerRepo, addExpUC)

	req := CompleteDailyQuestRequest{
		ChatID:    chatID,
		TgUserID:  tgUserID,
		QuestType: quest.DailyQuestTypeCompleteQuest,
	}

	err := uc.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	// Прогресс не должен быть сохранен, так как задание еще не выполнено
	if savedProgress != nil {
		t.Error("Execute() should not save progress if quest is not completed yet")
	}
}

func TestCompleteDailyQuestUseCase_Execute_NoSession(t *testing.T) {
	ctx := context.Background()
	chatID := int64(123)
	tgUserID := int64(456)

	sessionRepo := &mockCompleteDailyQuestSessionRepo{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			return nil, nil
		},
	}

	dailyQuestRepo := &mockCompleteDailyQuestDailyQuestRepo{}
	playerRepo := &mockCompleteDailyQuestPlayerRepo{}
	testPlayer := &player.Player{
		ID:            1,
		TgUserID:      tgUserID,
		GameSessionID: 1,
	}
	activeSession := &session.GameSession{
		ChatID: chatID,
		State:  session.StateActive,
	}
	activeSession.ID = 1
	mockPlayerRepoForExp := &mockCompleteDailyQuestPlayerRepo{
		getByTgUserIDAndSessionIDFunc: func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
			return testPlayer, nil
		},
		saveFunc: func(ctx context.Context, p *player.Player) error {
			return nil
		},
	}
	mockSessionRepoForExp := &mockCompleteDailyQuestSessionRepo{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			return activeSession, nil
		},
	}
	addExpUC := characterapp.NewAddExperienceUseCase(mockPlayerRepoForExp, mockSessionRepoForExp)

	uc := NewCompleteDailyQuestUseCase(sessionRepo, dailyQuestRepo, playerRepo, addExpUC)

	req := CompleteDailyQuestRequest{
		ChatID:    chatID,
		TgUserID:  tgUserID,
		QuestType: quest.DailyQuestTypeCompleteQuest,
	}

	err := uc.Execute(ctx, req)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}

func TestCompleteDailyQuestUseCase_Execute_QuestNotFound(t *testing.T) {
	ctx := context.Background()
	chatID := int64(123)
	tgUserID := int64(456)

	activeSession := &session.GameSession{
		ChatID: chatID,
		State:  session.StateActive,
	}
	activeSession.ID = 1

	testPlayer := &player.Player{
		ID:            1,
		TgUserID:      tgUserID,
		GameSessionID: 1,
	}

	sessionRepo := &mockCompleteDailyQuestSessionRepo{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			return activeSession, nil
		},
	}

	dailyQuestRepo := &mockCompleteDailyQuestDailyQuestRepo{
		getTodayQuestsFunc: func(ctx context.Context) ([]*quest.DailyQuest, error) {
			return []*quest.DailyQuest{}, nil
		},
	}

	playerRepo := &mockCompleteDailyQuestPlayerRepo{
		getByTgUserIDAndSessionIDFunc: func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
			return testPlayer, nil
		},
	}

	mockPlayerRepoForExp := &mockCompleteDailyQuestPlayerRepo{
		getByTgUserIDAndSessionIDFunc: func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
			return testPlayer, nil
		},
		saveFunc: func(ctx context.Context, p *player.Player) error {
			return nil
		},
	}
	mockSessionRepoForExp := &mockCompleteDailyQuestSessionRepo{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			return activeSession, nil
		},
	}
	addExpUC := characterapp.NewAddExperienceUseCase(mockPlayerRepoForExp, mockSessionRepoForExp)

	uc := NewCompleteDailyQuestUseCase(sessionRepo, dailyQuestRepo, playerRepo, addExpUC)

	req := CompleteDailyQuestRequest{
		ChatID:    chatID,
		TgUserID:  tgUserID,
		QuestType: quest.DailyQuestTypeCompleteQuest,
	}

	err := uc.Execute(ctx, req)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}

func TestCompleteDailyQuestUseCase_Execute_StreakUpdate(t *testing.T) {
	ctx := context.Background()
	chatID := int64(123)
	tgUserID := int64(456)

	activeSession := &session.GameSession{
		ChatID: chatID,
		State:  session.StateActive,
	}
	activeSession.ID = 1

	testPlayer := &player.Player{
		ID:            1,
		TgUserID:      tgUserID,
		GameSessionID: 1,
	}

	targetQuest := quest.NewDailyQuest(
		quest.DailyQuestTypeCompleteQuest,
		"Завершить квест",
		"Завершите любой активный квест",
		1,
		50,
		10,
	)
	targetQuest.ID = 1

	dailyQuests := []*quest.DailyQuest{targetQuest}

	now := time.Now()
	progress := &quest.DailyQuestProgress{
		ID:           1,
		PlayerID:     1,
		DailyQuestID: 1,
		CurrentValue: 1,
		TargetValue:  1,
		Completed:    false, // Не завершен, но выполнен (CurrentValue >= TargetValue)
		Date:         now,
	}

	// Создаем копию для allProgress, где задание завершено (для проверки стрика)
	completedProgress := &quest.DailyQuestProgress{
		ID:           1,
		PlayerID:     1,
		DailyQuestID: 1,
		CurrentValue: 1,
		TargetValue:  1,
		Completed:    true,
		CompletedAt:  &now,
		Date:         now,
	}
	allProgress := []*quest.DailyQuestProgress{completedProgress}

	streak := &quest.DailyQuestStreak{
		ID:         1,
		PlayerID:   1,
		StreakDays: 0,
		LastDate:   time.Time{},
	}

	var updatedStreak *quest.DailyQuestStreak

	sessionRepo := &mockCompleteDailyQuestSessionRepo{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			return activeSession, nil
		},
	}

	dailyQuestRepo := &mockCompleteDailyQuestDailyQuestRepo{
		getTodayQuestsFunc: func(ctx context.Context) ([]*quest.DailyQuest, error) {
			return dailyQuests, nil
		},
		getOrCreateProgressFunc: func(ctx context.Context, playerID uint, dailyQuestID uint, date time.Time) (*quest.DailyQuestProgress, error) {
			return progress, nil
		},
		saveProgressFunc: func(ctx context.Context, p *quest.DailyQuestProgress) error {
			return nil
		},
		getPlayerProgressFunc: func(ctx context.Context, playerID uint, date time.Time) ([]*quest.DailyQuestProgress, error) {
			return allProgress, nil
		},
		getStreakFunc: func(ctx context.Context, playerID uint) (*quest.DailyQuestStreak, error) {
			return streak, nil
		},
		updateStreakFunc: func(ctx context.Context, s *quest.DailyQuestStreak) error {
			updatedStreak = s
			return nil
		},
	}

	playerRepo := &mockCompleteDailyQuestPlayerRepo{
		getByTgUserIDAndSessionIDFunc: func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
			return testPlayer, nil
		},
	}

	mockPlayerRepoForExp := &mockCompleteDailyQuestPlayerRepo{
		getByTgUserIDAndSessionIDFunc: func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
			return testPlayer, nil
		},
		saveFunc: func(ctx context.Context, p *player.Player) error {
			return nil
		},
	}
	mockSessionRepoForExp := &mockCompleteDailyQuestSessionRepo{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			return activeSession, nil
		},
	}
	addExpUC := characterapp.NewAddExperienceUseCase(mockPlayerRepoForExp, mockSessionRepoForExp)

	uc := NewCompleteDailyQuestUseCase(sessionRepo, dailyQuestRepo, playerRepo, addExpUC)

	req := CompleteDailyQuestRequest{
		ChatID:    chatID,
		TgUserID:  tgUserID,
		QuestType: quest.DailyQuestTypeCompleteQuest,
	}

	err := uc.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if updatedStreak == nil {
		t.Fatal("Execute() should update streak when all quests are completed")
	}

	if updatedStreak.StreakDays != 1 {
		t.Errorf("Execute() streak.StreakDays = %d, want 1", updatedStreak.StreakDays)
	}
}
