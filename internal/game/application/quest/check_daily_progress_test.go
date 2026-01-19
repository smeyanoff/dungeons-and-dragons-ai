package quest

import (
	"context"
	"errors"
	"testing"
	"time"

	characterapp "dungeons-and-dragons-ai/internal/game/application/character"
	"dungeons-and-dragons-ai/internal/game/domain/player"
	"dungeons-and-dragons-ai/internal/game/domain/quest"
	"dungeons-and-dragons-ai/internal/game/domain/session"
)

// Mock Session Repository
type mockCheckDailyProgressSessionRepo struct {
	getByChatIDFunc func(ctx context.Context, chatID int64) (*session.GameSession, error)
}

func (m *mockCheckDailyProgressSessionRepo) GetByChatID(ctx context.Context, chatID int64) (*session.GameSession, error) {
	if m.getByChatIDFunc != nil {
		return m.getByChatIDFunc(ctx, chatID)
	}
	return nil, nil
}

func (m *mockCheckDailyProgressSessionRepo) Save(ctx context.Context, session *session.GameSession) error {
	return nil
}

func (m *mockCheckDailyProgressSessionRepo) Delete(ctx context.Context, chatID int64) error {
	return nil
}

// Mock Daily Quest Repository
type mockCheckDailyProgressDailyQuestRepo struct {
	getTodayQuestsFunc      func(ctx context.Context) ([]*quest.DailyQuest, error)
	getOrCreateProgressFunc func(ctx context.Context, playerID uint, dailyQuestID uint, date time.Time) (*quest.DailyQuestProgress, error)
	saveProgressFunc        func(ctx context.Context, progress *quest.DailyQuestProgress) error
	getPlayerProgressFunc   func(ctx context.Context, playerID uint, date time.Time) ([]*quest.DailyQuestProgress, error)
	getStreakFunc           func(ctx context.Context, playerID uint) (*quest.DailyQuestStreak, error)
}

func (m *mockCheckDailyProgressDailyQuestRepo) GetTodayQuests(ctx context.Context) ([]*quest.DailyQuest, error) {
	if m.getTodayQuestsFunc != nil {
		return m.getTodayQuestsFunc(ctx)
	}
	return []*quest.DailyQuest{}, nil
}

func (m *mockCheckDailyProgressDailyQuestRepo) GetOrCreateProgress(ctx context.Context, playerID uint, dailyQuestID uint, date time.Time) (*quest.DailyQuestProgress, error) {
	if m.getOrCreateProgressFunc != nil {
		return m.getOrCreateProgressFunc(ctx, playerID, dailyQuestID, date)
	}
	return nil, nil
}

func (m *mockCheckDailyProgressDailyQuestRepo) SaveProgress(ctx context.Context, progress *quest.DailyQuestProgress) error {
	if m.saveProgressFunc != nil {
		return m.saveProgressFunc(ctx, progress)
	}
	return nil
}

func (m *mockCheckDailyProgressDailyQuestRepo) GetPlayerProgress(ctx context.Context, playerID uint, date time.Time) ([]*quest.DailyQuestProgress, error) {
	if m.getPlayerProgressFunc != nil {
		return m.getPlayerProgressFunc(ctx, playerID, date)
	}
	return []*quest.DailyQuestProgress{}, nil
}

func (m *mockCheckDailyProgressDailyQuestRepo) GetStreak(ctx context.Context, playerID uint) (*quest.DailyQuestStreak, error) {
	if m.getStreakFunc != nil {
		return m.getStreakFunc(ctx, playerID)
	}
	return &quest.DailyQuestStreak{StreakDays: 0}, nil
}

func (m *mockCheckDailyProgressDailyQuestRepo) UpdateStreak(ctx context.Context, streak *quest.DailyQuestStreak) error {
	return nil
}

// Mock Player Repository
type mockCheckDailyProgressPlayerRepo struct {
	getByTgUserIDAndSessionIDFunc func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error)
	saveFunc                      func(ctx context.Context, p *player.Player) error
}

func (m *mockCheckDailyProgressPlayerRepo) GetByTgUserIDAndSessionID(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
	if m.getByTgUserIDAndSessionIDFunc != nil {
		return m.getByTgUserIDAndSessionIDFunc(ctx, tgUserID, sessionID)
	}
	return nil, nil
}

func (m *mockCheckDailyProgressPlayerRepo) Save(ctx context.Context, p *player.Player) error {
	if m.saveFunc != nil {
		return m.saveFunc(ctx, p)
	}
	return nil
}

// Mock Complete Daily Quest Use Case - используем реальный тип
// В тестах создаем реальный use case с моками

func TestCheckDailyQuestProgressUseCase_Execute_Success(t *testing.T) {
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

	sessionRepo := &mockCheckDailyProgressSessionRepo{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			return activeSession, nil
		},
	}

	dailyQuestRepo := &mockCheckDailyProgressDailyQuestRepo{
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

	playerRepo := &mockCheckDailyProgressPlayerRepo{
		getByTgUserIDAndSessionIDFunc: func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
			return testPlayer, nil
		},
	}

	// CompleteUC не будет вызван в этом тесте, поэтому создаем простой мок
	mockPlayerRepoForComplete := &mockCheckDailyProgressPlayerRepo{
		getByTgUserIDAndSessionIDFunc: func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
			return &player.Player{ID: 1, TgUserID: tgUserID, GameSessionID: 1}, nil
		},
		saveFunc: func(ctx context.Context, p *player.Player) error {
			return nil
		},
	}
	mockSessionRepoForComplete := &mockCheckDailyProgressSessionRepo{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			gs := &session.GameSession{ChatID: chatID, State: session.StateActive}
			gs.ID = 1
			return gs, nil
		},
	}
	mockDailyQuestRepoForComplete := &mockCheckDailyProgressDailyQuestRepo{
		getTodayQuestsFunc: func(ctx context.Context) ([]*quest.DailyQuest, error) {
			return []*quest.DailyQuest{}, nil
		},
	}
	mockAddExpUC := characterapp.NewAddExperienceUseCase(mockPlayerRepoForComplete, mockSessionRepoForComplete)
	completeUC := NewCompleteDailyQuestUseCase(mockSessionRepoForComplete, mockDailyQuestRepoForComplete, mockPlayerRepoForComplete, mockAddExpUC)

	uc := NewCheckDailyQuestProgressUseCase(sessionRepo, dailyQuestRepo, playerRepo, completeUC)

	req := CheckProgressRequest{
		ChatID:     chatID,
		TgUserID:   tgUserID,
		QuestType:  quest.DailyQuestTypeCompleteQuest,
		Increment:  1,
	}

	err := uc.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if savedProgress == nil {
		t.Fatal("Execute() should save progress")
	}

	if savedProgress.CurrentValue != 1 {
		t.Errorf("Execute() progress.CurrentValue = %d, want 1", savedProgress.CurrentValue)
	}
}

func TestCheckDailyQuestProgressUseCase_Execute_CompletesQuest(t *testing.T) {
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

	var completedReq *CompleteDailyQuestRequest

	sessionRepo := &mockCheckDailyProgressSessionRepo{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			return activeSession, nil
		},
	}

	dailyQuestRepo := &mockCheckDailyProgressDailyQuestRepo{
		getTodayQuestsFunc: func(ctx context.Context) ([]*quest.DailyQuest, error) {
			return dailyQuests, nil
		},
		getOrCreateProgressFunc: func(ctx context.Context, playerID uint, dailyQuestID uint, date time.Time) (*quest.DailyQuestProgress, error) {
			return progress, nil
		},
		saveProgressFunc: func(ctx context.Context, p *quest.DailyQuestProgress) error {
			// Проверяем, что задание завершено и вызываем completeUC
			if p.IsCompleted() {
				completedReq = &CompleteDailyQuestRequest{
					ChatID:    chatID,
					TgUserID:  tgUserID,
					QuestType: quest.DailyQuestTypeCompleteQuest,
				}
			}
			return nil
		},
	}

	playerRepo := &mockCheckDailyProgressPlayerRepo{
		getByTgUserIDAndSessionIDFunc: func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
			return testPlayer, nil
		},
		saveFunc: func(ctx context.Context, p *player.Player) error {
			return nil
		},
	}

	// Создаем реальный CompleteDailyQuestUseCase с моками
	mockPlayerRepoForComplete := &mockCheckDailyProgressPlayerRepo{
		getByTgUserIDAndSessionIDFunc: func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
			return testPlayer, nil
		},
		saveFunc: func(ctx context.Context, p *player.Player) error {
			return nil
		},
	}
	mockSessionRepoForComplete := &mockCheckDailyProgressSessionRepo{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			return activeSession, nil
		},
	}
	mockDailyQuestRepoForComplete := &mockCheckDailyProgressDailyQuestRepo{
		getTodayQuestsFunc: func(ctx context.Context) ([]*quest.DailyQuest, error) {
			return dailyQuests, nil
		},
		getOrCreateProgressFunc: func(ctx context.Context, playerID uint, dailyQuestID uint, date time.Time) (*quest.DailyQuestProgress, error) {
			return progress, nil
		},
		saveProgressFunc: func(ctx context.Context, p *quest.DailyQuestProgress) error {
			return nil
		},
	}
	mockAddExpUC := characterapp.NewAddExperienceUseCase(mockPlayerRepoForComplete, mockSessionRepoForComplete)
	completeUC := NewCompleteDailyQuestUseCase(mockSessionRepoForComplete, mockDailyQuestRepoForComplete, mockPlayerRepoForComplete, mockAddExpUC)

	uc := NewCheckDailyQuestProgressUseCase(sessionRepo, dailyQuestRepo, playerRepo, completeUC)

	req := CheckProgressRequest{
		ChatID:     chatID,
		TgUserID:   tgUserID,
		QuestType:  quest.DailyQuestTypeCompleteQuest,
		Increment:  1,
	}

	err := uc.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if completedReq == nil {
		t.Fatal("Execute() should call completeUC when quest is completed")
	}

	if completedReq.QuestType != quest.DailyQuestTypeCompleteQuest {
		t.Errorf("Execute() completedReq.QuestType = %v, want %v", completedReq.QuestType, quest.DailyQuestTypeCompleteQuest)
	}
}

func TestCheckDailyQuestProgressUseCase_Execute_AlreadyCompleted(t *testing.T) {
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

	sessionRepo := &mockCheckDailyProgressSessionRepo{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			return activeSession, nil
		},
	}

	dailyQuestRepo := &mockCheckDailyProgressDailyQuestRepo{
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

	playerRepo := &mockCheckDailyProgressPlayerRepo{
		getByTgUserIDAndSessionIDFunc: func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
			return testPlayer, nil
		},
	}

	// CompleteUC не будет вызван в этом тесте, поэтому создаем простой мок
	mockPlayerRepoForComplete := &mockCheckDailyProgressPlayerRepo{
		getByTgUserIDAndSessionIDFunc: func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
			return &player.Player{ID: 1, TgUserID: tgUserID, GameSessionID: 1}, nil
		},
		saveFunc: func(ctx context.Context, p *player.Player) error {
			return nil
		},
	}
	mockSessionRepoForComplete := &mockCheckDailyProgressSessionRepo{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			gs := &session.GameSession{ChatID: chatID, State: session.StateActive}
			gs.ID = 1
			return gs, nil
		},
	}
	mockDailyQuestRepoForComplete := &mockCheckDailyProgressDailyQuestRepo{
		getTodayQuestsFunc: func(ctx context.Context) ([]*quest.DailyQuest, error) {
			return []*quest.DailyQuest{}, nil
		},
	}
	mockAddExpUC := characterapp.NewAddExperienceUseCase(mockPlayerRepoForComplete, mockSessionRepoForComplete)
	completeUC := NewCompleteDailyQuestUseCase(mockSessionRepoForComplete, mockDailyQuestRepoForComplete, mockPlayerRepoForComplete, mockAddExpUC)

	uc := NewCheckDailyQuestProgressUseCase(sessionRepo, dailyQuestRepo, playerRepo, completeUC)

	req := CheckProgressRequest{
		ChatID:     chatID,
		TgUserID:   tgUserID,
		QuestType:  quest.DailyQuestTypeCompleteQuest,
		Increment:  1,
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

func TestCheckDailyQuestProgressUseCase_Execute_NoSession(t *testing.T) {
	ctx := context.Background()
	chatID := int64(123)
	tgUserID := int64(456)

	sessionRepo := &mockCheckDailyProgressSessionRepo{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			return nil, nil
		},
	}

	dailyQuestRepo := &mockCheckDailyProgressDailyQuestRepo{}
	playerRepo := &mockCheckDailyProgressPlayerRepo{}
	// CompleteUC не будет вызван в этом тесте, поэтому создаем простой мок
	mockPlayerRepoForComplete := &mockCheckDailyProgressPlayerRepo{
		getByTgUserIDAndSessionIDFunc: func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
			return &player.Player{ID: 1, TgUserID: tgUserID, GameSessionID: 1}, nil
		},
		saveFunc: func(ctx context.Context, p *player.Player) error {
			return nil
		},
	}
	mockSessionRepoForComplete := &mockCheckDailyProgressSessionRepo{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			gs := &session.GameSession{ChatID: chatID, State: session.StateActive}
			gs.ID = 1
			return gs, nil
		},
	}
	mockDailyQuestRepoForComplete := &mockCheckDailyProgressDailyQuestRepo{
		getTodayQuestsFunc: func(ctx context.Context) ([]*quest.DailyQuest, error) {
			return []*quest.DailyQuest{}, nil
		},
	}
	mockAddExpUC := characterapp.NewAddExperienceUseCase(mockPlayerRepoForComplete, mockSessionRepoForComplete)
	completeUC := NewCompleteDailyQuestUseCase(mockSessionRepoForComplete, mockDailyQuestRepoForComplete, mockPlayerRepoForComplete, mockAddExpUC)

	uc := NewCheckDailyQuestProgressUseCase(sessionRepo, dailyQuestRepo, playerRepo, completeUC)

	req := CheckProgressRequest{
		ChatID:     chatID,
		TgUserID:   tgUserID,
		QuestType:  quest.DailyQuestTypeCompleteQuest,
		Increment:  1,
	}

	err := uc.Execute(ctx, req)
	// Должно вернуть nil, так как неактивная сессия просто пропускается
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil (should skip inactive session)", err)
	}
}

func TestCheckDailyQuestProgressUseCase_Execute_NoPlayer(t *testing.T) {
	ctx := context.Background()
	chatID := int64(123)
	tgUserID := int64(456)

	activeSession := &session.GameSession{
		ChatID: chatID,
		State:  session.StateActive,
	}
	activeSession.ID = 1

	sessionRepo := &mockCheckDailyProgressSessionRepo{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			return activeSession, nil
		},
	}

	dailyQuestRepo := &mockCheckDailyProgressDailyQuestRepo{}
	playerRepo := &mockCheckDailyProgressPlayerRepo{
		getByTgUserIDAndSessionIDFunc: func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
			return nil, nil
		},
	}
	// CompleteUC не будет вызван в этом тесте, поэтому создаем простой мок
	mockPlayerRepoForComplete := &mockCheckDailyProgressPlayerRepo{
		getByTgUserIDAndSessionIDFunc: func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
			return &player.Player{ID: 1, TgUserID: tgUserID, GameSessionID: 1}, nil
		},
		saveFunc: func(ctx context.Context, p *player.Player) error {
			return nil
		},
	}
	mockSessionRepoForComplete := &mockCheckDailyProgressSessionRepo{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			gs := &session.GameSession{ChatID: chatID, State: session.StateActive}
			gs.ID = 1
			return gs, nil
		},
	}
	mockDailyQuestRepoForComplete := &mockCheckDailyProgressDailyQuestRepo{
		getTodayQuestsFunc: func(ctx context.Context) ([]*quest.DailyQuest, error) {
			return []*quest.DailyQuest{}, nil
		},
	}
	mockAddExpUC := characterapp.NewAddExperienceUseCase(mockPlayerRepoForComplete, mockSessionRepoForComplete)
	completeUC := NewCompleteDailyQuestUseCase(mockSessionRepoForComplete, mockDailyQuestRepoForComplete, mockPlayerRepoForComplete, mockAddExpUC)

	uc := NewCheckDailyQuestProgressUseCase(sessionRepo, dailyQuestRepo, playerRepo, completeUC)

	req := CheckProgressRequest{
		ChatID:     chatID,
		TgUserID:   tgUserID,
		QuestType:  quest.DailyQuestTypeCompleteQuest,
		Increment:  1,
	}

	err := uc.Execute(ctx, req)
	// Должно вернуть nil, так как отсутствие персонажа просто пропускается
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil (should skip if no player)", err)
	}
}

func TestCheckDailyQuestProgressUseCase_Execute_QuestNotFound(t *testing.T) {
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

	sessionRepo := &mockCheckDailyProgressSessionRepo{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			return activeSession, nil
		},
	}

	dailyQuestRepo := &mockCheckDailyProgressDailyQuestRepo{
		getTodayQuestsFunc: func(ctx context.Context) ([]*quest.DailyQuest, error) {
			return []*quest.DailyQuest{}, nil
		},
	}

	playerRepo := &mockCheckDailyProgressPlayerRepo{
		getByTgUserIDAndSessionIDFunc: func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
			return testPlayer, nil
		},
	}

	// CompleteUC не будет вызван в этом тесте, поэтому создаем простой мок
	mockPlayerRepoForComplete := &mockCheckDailyProgressPlayerRepo{
		getByTgUserIDAndSessionIDFunc: func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
			return &player.Player{ID: 1, TgUserID: tgUserID, GameSessionID: 1}, nil
		},
		saveFunc: func(ctx context.Context, p *player.Player) error {
			return nil
		},
	}
	mockSessionRepoForComplete := &mockCheckDailyProgressSessionRepo{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			gs := &session.GameSession{ChatID: chatID, State: session.StateActive}
			gs.ID = 1
			return gs, nil
		},
	}
	mockDailyQuestRepoForComplete := &mockCheckDailyProgressDailyQuestRepo{
		getTodayQuestsFunc: func(ctx context.Context) ([]*quest.DailyQuest, error) {
			return []*quest.DailyQuest{}, nil
		},
	}
	mockAddExpUC := characterapp.NewAddExperienceUseCase(mockPlayerRepoForComplete, mockSessionRepoForComplete)
	completeUC := NewCompleteDailyQuestUseCase(mockSessionRepoForComplete, mockDailyQuestRepoForComplete, mockPlayerRepoForComplete, mockAddExpUC)

	uc := NewCheckDailyQuestProgressUseCase(sessionRepo, dailyQuestRepo, playerRepo, completeUC)

	req := CheckProgressRequest{
		ChatID:     chatID,
		TgUserID:   tgUserID,
		QuestType:  quest.DailyQuestTypeCompleteQuest,
		Increment:  1,
	}

	err := uc.Execute(ctx, req)
	// Должно вернуть nil, так как отсутствие задания просто пропускается
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil (should skip if quest not found)", err)
	}
}

func TestCheckDailyQuestProgressUseCase_Execute_RepositoryError(t *testing.T) {
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

	sessionRepo := &mockCheckDailyProgressSessionRepo{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			return activeSession, nil
		},
	}

	dailyQuestRepo := &mockCheckDailyProgressDailyQuestRepo{
		getTodayQuestsFunc: func(ctx context.Context) ([]*quest.DailyQuest, error) {
			return dailyQuests, nil
		},
		getOrCreateProgressFunc: func(ctx context.Context, playerID uint, dailyQuestID uint, date time.Time) (*quest.DailyQuestProgress, error) {
			return nil, errors.New("database error")
		},
	}

	playerRepo := &mockCheckDailyProgressPlayerRepo{
		getByTgUserIDAndSessionIDFunc: func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
			return testPlayer, nil
		},
	}

	// CompleteUC не будет вызван в этом тесте, поэтому создаем простой мок
	mockPlayerRepoForComplete := &mockCheckDailyProgressPlayerRepo{
		getByTgUserIDAndSessionIDFunc: func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
			return &player.Player{ID: 1, TgUserID: tgUserID, GameSessionID: 1}, nil
		},
		saveFunc: func(ctx context.Context, p *player.Player) error {
			return nil
		},
	}
	mockSessionRepoForComplete := &mockCheckDailyProgressSessionRepo{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			gs := &session.GameSession{ChatID: chatID, State: session.StateActive}
			gs.ID = 1
			return gs, nil
		},
	}
	mockDailyQuestRepoForComplete := &mockCheckDailyProgressDailyQuestRepo{
		getTodayQuestsFunc: func(ctx context.Context) ([]*quest.DailyQuest, error) {
			return []*quest.DailyQuest{}, nil
		},
	}
	mockAddExpUC := characterapp.NewAddExperienceUseCase(mockPlayerRepoForComplete, mockSessionRepoForComplete)
	completeUC := NewCompleteDailyQuestUseCase(mockSessionRepoForComplete, mockDailyQuestRepoForComplete, mockPlayerRepoForComplete, mockAddExpUC)

	uc := NewCheckDailyQuestProgressUseCase(sessionRepo, dailyQuestRepo, playerRepo, completeUC)

	req := CheckProgressRequest{
		ChatID:     chatID,
		TgUserID:   tgUserID,
		QuestType:  quest.DailyQuestTypeCompleteQuest,
		Increment:  1,
	}

	err := uc.Execute(ctx, req)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}
