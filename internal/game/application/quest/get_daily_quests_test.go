package quest

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"dungeons-and-dragons-ai/internal/game/domain/player"
	"dungeons-and-dragons-ai/internal/game/domain/quest"
	"dungeons-and-dragons-ai/internal/game/domain/session"
)

// Mock Session Repository
type mockGetDailyQuestsSessionRepo struct {
	getByChatIDFunc func(ctx context.Context, chatID int64) (*session.GameSession, error)
}

func (m *mockGetDailyQuestsSessionRepo) GetByChatID(ctx context.Context, chatID int64) (*session.GameSession, error) {
	if m.getByChatIDFunc != nil {
		return m.getByChatIDFunc(ctx, chatID)
	}
	return nil, nil
}

func (m *mockGetDailyQuestsSessionRepo) Save(ctx context.Context, session *session.GameSession) error {
	return nil
}

func (m *mockGetDailyQuestsSessionRepo) Delete(ctx context.Context, chatID int64) error {
	return nil
}

// Mock Daily Quest Repository
type mockGetDailyQuestsDailyQuestRepo struct {
	getTodayQuestsFunc   func(ctx context.Context) ([]*quest.DailyQuest, error)
	getPlayerProgressFunc func(ctx context.Context, playerID uint, date time.Time) ([]*quest.DailyQuestProgress, error)
	getStreakFunc        func(ctx context.Context, playerID uint) (*quest.DailyQuestStreak, error)
}

func (m *mockGetDailyQuestsDailyQuestRepo) GetTodayQuests(ctx context.Context) ([]*quest.DailyQuest, error) {
	if m.getTodayQuestsFunc != nil {
		return m.getTodayQuestsFunc(ctx)
	}
	return []*quest.DailyQuest{}, nil
}

func (m *mockGetDailyQuestsDailyQuestRepo) GetPlayerProgress(ctx context.Context, playerID uint, date time.Time) ([]*quest.DailyQuestProgress, error) {
	if m.getPlayerProgressFunc != nil {
		return m.getPlayerProgressFunc(ctx, playerID, date)
	}
	return []*quest.DailyQuestProgress{}, nil
}

func (m *mockGetDailyQuestsDailyQuestRepo) GetStreak(ctx context.Context, playerID uint) (*quest.DailyQuestStreak, error) {
	if m.getStreakFunc != nil {
		return m.getStreakFunc(ctx, playerID)
	}
	return &quest.DailyQuestStreak{StreakDays: 0}, nil
}

func (m *mockGetDailyQuestsDailyQuestRepo) GetOrCreateProgress(ctx context.Context, playerID uint, dailyQuestID uint, date time.Time) (*quest.DailyQuestProgress, error) {
	return nil, nil
}

func (m *mockGetDailyQuestsDailyQuestRepo) SaveProgress(ctx context.Context, progress *quest.DailyQuestProgress) error {
	return nil
}

func (m *mockGetDailyQuestsDailyQuestRepo) UpdateStreak(ctx context.Context, streak *quest.DailyQuestStreak) error {
	return nil
}

// Mock Player Repository
type mockGetDailyQuestsPlayerRepo struct {
	getByTgUserIDAndSessionIDFunc func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error)
}

func (m *mockGetDailyQuestsPlayerRepo) GetByTgUserIDAndSessionID(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
	if m.getByTgUserIDAndSessionIDFunc != nil {
		return m.getByTgUserIDAndSessionIDFunc(ctx, tgUserID, sessionID)
	}
	return nil, nil
}

func TestGetDailyQuestsUseCase_Execute_Success(t *testing.T) {
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

	dailyQuests := []*quest.DailyQuest{
		quest.NewDailyQuest(
			quest.DailyQuestTypeCompleteQuest,
			"Завершить квест",
			"Завершите любой активный квест",
			1,
			50,
			10,
		),
		quest.NewDailyQuest(
			quest.DailyQuestTypeWinCombat,
			"Победить в бою",
			"Одолейте врагов в бою",
			1,
			75,
			15,
		),
		quest.NewDailyQuest(
			quest.DailyQuestTypeExploreLocation,
			"Исследовать локацию",
			"Посетите новую локацию в мире",
			1,
			25,
			5,
		),
	}
	dailyQuests[0].ID = 1
	dailyQuests[1].ID = 2
	dailyQuests[2].ID = 3

	progress := []*quest.DailyQuestProgress{
		{
			ID:           1,
			PlayerID:     1,
			DailyQuestID: 1,
			DailyQuest:   *dailyQuests[0],
			CurrentValue: 0,
			TargetValue:  1,
			Completed:    false,
			Date:         time.Now(),
		},
		{
			ID:           2,
			PlayerID:     1,
			DailyQuestID: 2,
			DailyQuest:   *dailyQuests[1],
			CurrentValue: 1,
			TargetValue:  1,
			Completed:    true,
			Date:         time.Now(),
		},
	}

	streak := &quest.DailyQuestStreak{
		ID:         1,
		PlayerID:   1,
		StreakDays: 5,
		LastDate:   time.Now(),
	}

	sessionRepo := &mockGetDailyQuestsSessionRepo{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			return activeSession, nil
		},
	}

	dailyQuestRepo := &mockGetDailyQuestsDailyQuestRepo{
		getTodayQuestsFunc: func(ctx context.Context) ([]*quest.DailyQuest, error) {
			return dailyQuests, nil
		},
		getPlayerProgressFunc: func(ctx context.Context, playerID uint, date time.Time) ([]*quest.DailyQuestProgress, error) {
			return progress, nil
		},
		getStreakFunc: func(ctx context.Context, playerID uint) (*quest.DailyQuestStreak, error) {
			return streak, nil
		},
	}

	playerRepo := &mockGetDailyQuestsPlayerRepo{
		getByTgUserIDAndSessionIDFunc: func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
			return testPlayer, nil
		},
	}

	uc := NewGetDailyQuestsUseCase(sessionRepo, dailyQuestRepo, playerRepo)

	result, err := uc.Execute(ctx, chatID, tgUserID)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if !strings.Contains(result, "📅 Ежедневные задания") {
		t.Error("Execute() result should contain '📅 Ежедневные задания'")
	}

	if !strings.Contains(result, "🔥 Стрик: 5 дней подряд") {
		t.Error("Execute() result should contain streak information")
	}

	if !strings.Contains(result, "Завершить квест") {
		t.Error("Execute() result should contain quest titles")
	}

	if !strings.Contains(result, "🔄") || !strings.Contains(result, "✅") {
		t.Error("Execute() result should contain status emojis")
	}

	if !strings.Contains(result, "💎 Еженедельный бонус") {
		t.Error("Execute() result should contain weekly bonus information")
	}
}

func TestGetDailyQuestsUseCase_Execute_NoSession(t *testing.T) {
	ctx := context.Background()
	chatID := int64(123)
	tgUserID := int64(456)

	sessionRepo := &mockGetDailyQuestsSessionRepo{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			return nil, nil
		},
	}

	dailyQuestRepo := &mockGetDailyQuestsDailyQuestRepo{}
	playerRepo := &mockGetDailyQuestsPlayerRepo{}

	uc := NewGetDailyQuestsUseCase(sessionRepo, dailyQuestRepo, playerRepo)

	_, err := uc.Execute(ctx, chatID, tgUserID)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}

	if !strings.Contains(err.Error(), "game session not found") {
		t.Errorf("Execute() error = %v, want 'game session not found'", err)
	}
}

func TestGetDailyQuestsUseCase_Execute_InactiveSession(t *testing.T) {
	ctx := context.Background()
	chatID := int64(123)
	tgUserID := int64(456)

	inactiveSession := &session.GameSession{
		ChatID: chatID,
		State:  session.StateDone,
	}
	inactiveSession.ID = 1

	sessionRepo := &mockGetDailyQuestsSessionRepo{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			return inactiveSession, nil
		},
	}

	dailyQuestRepo := &mockGetDailyQuestsDailyQuestRepo{}
	playerRepo := &mockGetDailyQuestsPlayerRepo{}

	uc := NewGetDailyQuestsUseCase(sessionRepo, dailyQuestRepo, playerRepo)

	_, err := uc.Execute(ctx, chatID, tgUserID)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}

	if !strings.Contains(err.Error(), "not active") {
		t.Errorf("Execute() error = %v, want 'not active'", err)
	}
}

func TestGetDailyQuestsUseCase_Execute_NoPlayer(t *testing.T) {
	ctx := context.Background()
	chatID := int64(123)
	tgUserID := int64(456)

	activeSession := &session.GameSession{
		ChatID: chatID,
		State:  session.StateActive,
	}
	activeSession.ID = 1

	sessionRepo := &mockGetDailyQuestsSessionRepo{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			return activeSession, nil
		},
	}

	dailyQuestRepo := &mockGetDailyQuestsDailyQuestRepo{}
	playerRepo := &mockGetDailyQuestsPlayerRepo{
		getByTgUserIDAndSessionIDFunc: func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
			return nil, nil
		},
	}

	uc := NewGetDailyQuestsUseCase(sessionRepo, dailyQuestRepo, playerRepo)

	_, err := uc.Execute(ctx, chatID, tgUserID)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}

	if !strings.Contains(err.Error(), "character not created") {
		t.Errorf("Execute() error = %v, want 'character not created'", err)
	}
}

func TestGetDailyQuestsUseCase_Execute_NoStreak(t *testing.T) {
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

	dailyQuests := []*quest.DailyQuest{
		quest.NewDailyQuest(
			quest.DailyQuestTypeCompleteQuest,
			"Завершить квест",
			"Завершите любой активный квест",
			1,
			50,
			10,
		),
	}
	dailyQuests[0].ID = 1

	sessionRepo := &mockGetDailyQuestsSessionRepo{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			return activeSession, nil
		},
	}

	dailyQuestRepo := &mockGetDailyQuestsDailyQuestRepo{
		getTodayQuestsFunc: func(ctx context.Context) ([]*quest.DailyQuest, error) {
			return dailyQuests, nil
		},
		getPlayerProgressFunc: func(ctx context.Context, playerID uint, date time.Time) ([]*quest.DailyQuestProgress, error) {
			return []*quest.DailyQuestProgress{}, nil
		},
		getStreakFunc: func(ctx context.Context, playerID uint) (*quest.DailyQuestStreak, error) {
			return &quest.DailyQuestStreak{StreakDays: 0}, nil
		},
	}

	playerRepo := &mockGetDailyQuestsPlayerRepo{
		getByTgUserIDAndSessionIDFunc: func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
			return testPlayer, nil
		},
	}

	uc := NewGetDailyQuestsUseCase(sessionRepo, dailyQuestRepo, playerRepo)

	result, err := uc.Execute(ctx, chatID, tgUserID)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	// Стрик не должен отображаться, если он равен 0
	if strings.Contains(result, "🔥 Стрик:") {
		t.Error("Execute() result should not contain streak when it's 0")
	}
}

func TestGetDailyQuestsUseCase_Execute_RepositoryError(t *testing.T) {
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

	sessionRepo := &mockGetDailyQuestsSessionRepo{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			return activeSession, nil
		},
	}

	dailyQuestRepo := &mockGetDailyQuestsDailyQuestRepo{
		getTodayQuestsFunc: func(ctx context.Context) ([]*quest.DailyQuest, error) {
			return nil, errors.New("database error")
		},
	}

	playerRepo := &mockGetDailyQuestsPlayerRepo{
		getByTgUserIDAndSessionIDFunc: func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
			return testPlayer, nil
		},
	}

	uc := NewGetDailyQuestsUseCase(sessionRepo, dailyQuestRepo, playerRepo)

	_, err := uc.Execute(ctx, chatID, tgUserID)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}

	if !strings.Contains(err.Error(), "failed to get daily quests") {
		t.Errorf("Execute() error = %v, want 'failed to get daily quests'", err)
	}
}
