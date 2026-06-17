package achievement

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"dungeons-and-dragons-ai/internal/game/domain/achievement"
	"dungeons-and-dragons-ai/internal/game/domain/player"
	"dungeons-and-dragons-ai/internal/game/domain/session"
)

// Mock Session Repository
type mockGetAchievementsSessionRepo struct {
	getByChatIDFunc func(ctx context.Context, chatID int64) (*session.GameSession, error)
}

func (m *mockGetAchievementsSessionRepo) GetByChatID(ctx context.Context, chatID int64) (*session.GameSession, error) {
	if m.getByChatIDFunc != nil {
		return m.getByChatIDFunc(ctx, chatID)
	}
	return nil, nil
}

func (m *mockGetAchievementsSessionRepo) Save(ctx context.Context, session *session.GameSession) error {
	return nil
}

func (m *mockGetAchievementsSessionRepo) Delete(ctx context.Context, chatID int64) error {
	return nil
}

// Mock Achievement Repository
type mockGetAchievementsRepo struct {
	getPlayerAchievementsFunc  func(ctx context.Context, playerID uint) ([]*achievement.PlayerAchievement, error)
	getAllFunc                 func(ctx context.Context) ([]*achievement.Achievement, error)
	getByCodeFunc              func(ctx context.Context, code string) (*achievement.Achievement, error)
	getAchievementProgressFunc func(ctx context.Context, playerID uint, achievementID uint) (*achievement.AchievementProgress, error)
	saveAchievementProgressFunc func(ctx context.Context, progress *achievement.AchievementProgress) error
	savePlayerAchievementFunc  func(ctx context.Context, playerAchievement *achievement.PlayerAchievement) error
}

func (m *mockGetAchievementsRepo) GetAll(ctx context.Context) ([]*achievement.Achievement, error) {
	if m.getAllFunc != nil {
		return m.getAllFunc(ctx)
	}
	return []*achievement.Achievement{}, nil
}

func (m *mockGetAchievementsRepo) GetByCode(ctx context.Context, code string) (*achievement.Achievement, error) {
	if m.getByCodeFunc != nil {
		return m.getByCodeFunc(ctx, code)
	}
	return nil, nil
}

func (m *mockGetAchievementsRepo) GetPlayerAchievements(ctx context.Context, playerID uint) ([]*achievement.PlayerAchievement, error) {
	if m.getPlayerAchievementsFunc != nil {
		return m.getPlayerAchievementsFunc(ctx, playerID)
	}
	return []*achievement.PlayerAchievement{}, nil
}

func (m *mockGetAchievementsRepo) GetPlayerAchievementByCode(ctx context.Context, playerID uint, code string) (*achievement.PlayerAchievement, error) {
	return nil, nil
}

func (m *mockGetAchievementsRepo) GetAchievementProgress(ctx context.Context, playerID uint, achievementID uint) (*achievement.AchievementProgress, error) {
	if m.getAchievementProgressFunc != nil {
		return m.getAchievementProgressFunc(ctx, playerID, achievementID)
	}
	return nil, nil
}

func (m *mockGetAchievementsRepo) SaveAchievementProgress(ctx context.Context, progress *achievement.AchievementProgress) error {
	if m.saveAchievementProgressFunc != nil {
		return m.saveAchievementProgressFunc(ctx, progress)
	}
	return nil
}

func (m *mockGetAchievementsRepo) SavePlayerAchievement(ctx context.Context, playerAchievement *achievement.PlayerAchievement) error {
	if m.savePlayerAchievementFunc != nil {
		return m.savePlayerAchievementFunc(ctx, playerAchievement)
	}
	return nil
}

func TestGetAchievementsUseCase_Execute_Success(t *testing.T) {
	ctx := context.Background()
	chatID := int64(123)
	tgUserID := int64(456)

	activeSession := &session.GameSession{
		ChatID: chatID,
		State:  session.StateActive,
	}
	activeSession.ID = 1

	testPlayer := &player.Player{
		ID:        1,
		TgUserID:  tgUserID,
		GameSessionID: 1,
	}

	allAchievements := []*achievement.Achievement{
		{
			ID:          1,
			Code:        "first_quest",
			Title:       "Первый квест",
			Description: "Завершите первый квест",
			Type:        achievement.AchievementTypeQuest,
			Rarity:      achievement.RarityCommon,
			RequirementValue: 1,
			Icon:        "🏆",
		},
		{
			ID:          2,
			Code:        "combat_master",
			Title:       "Мастер боя",
			Description: "Победите в 10 боях",
			Type:        achievement.AchievementTypeCombat,
			Rarity:      achievement.RarityRare,
			RequirementValue: 10,
			Icon:        "⚔️",
		},
		{
			ID:          3,
			Code:        "explorer",
			Title:       "Исследователь",
			Description: "Посетите 20 локаций",
			Type:        achievement.AchievementTypeExploration,
			Rarity:      achievement.RarityEpic,
			RequirementValue: 20,
			IsHidden:    true,
		},
	}

	playerAchievements := []*achievement.PlayerAchievement{
		{
			ID:            1,
			PlayerID:      1,
			AchievementID: 1,
			Achievement:   *allAchievements[0],
			Progress:      1,
			EarnedAt:      time.Now().Add(-24 * time.Hour),
			EarnedCount:   1,
		},
	}

	sessionRepo := &mockGetAchievementsSessionRepo{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			gs := activeSession
			gs.Players = []player.Player{*testPlayer}
			return gs, nil
		},
	}

	achievementRepo := &mockGetAchievementsRepo{
		getPlayerAchievementsFunc: func(ctx context.Context, playerID uint) ([]*achievement.PlayerAchievement, error) {
			return playerAchievements, nil
		},
		getAllFunc: func(ctx context.Context) ([]*achievement.Achievement, error) {
			return allAchievements, nil
		},
		getAchievementProgressFunc: func(ctx context.Context, playerID uint, achievementID uint) (*achievement.AchievementProgress, error) {
			if achievementID == 2 {
				return &achievement.AchievementProgress{
					PlayerID:     1,
					AchievementID: 2,
					CurrentValue: 7,
				}, nil
			}
			return nil, nil
		},
	}

	uc := NewGetAchievementsUseCase(achievementRepo, sessionRepo)

	result, err := uc.Execute(ctx, GetAchievementsRequest{
		ChatID:   chatID,
		TgUserID: tgUserID,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if !strings.Contains(result, "🏆 Ваши достижения") {
		t.Error("Execute() result should contain '🏆 Ваши достижения'")
	}

	if !strings.Contains(result, "Получено: 1 из 3 достижений") {
		t.Error("Execute() result should contain achievement count")
	}

	if !strings.Contains(result, "📜 Квесты:") {
		t.Error("Execute() result should contain quest category")
	}

	if !strings.Contains(result, "Первый квест") {
		t.Error("Execute() result should contain earned achievement title")
	}

	if !strings.Contains(result, "Мастер боя") {
		t.Error("Execute() result should contain progress achievement title")
	}

	if !strings.Contains(result, "(7/10 - 70%)") {
		t.Error("Execute() result should contain progress percentage")
	}

	if !strings.Contains(result, "??? - ???") {
		t.Error("Execute() result should contain hidden achievement placeholder")
	}
}

func TestGetAchievementsUseCase_Execute_NoSession(t *testing.T) {
	ctx := context.Background()
	chatID := int64(123)
	tgUserID := int64(456)

	sessionRepo := &mockGetAchievementsSessionRepo{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			return nil, nil
		},
	}

	achievementRepo := &mockGetAchievementsRepo{}

	uc := NewGetAchievementsUseCase(achievementRepo, sessionRepo)

	result, err := uc.Execute(ctx, GetAchievementsRequest{
		ChatID:   chatID,
		TgUserID: tgUserID,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if !strings.Contains(result, "Игра не начата") {
		t.Error("Execute() result should contain 'Игра не начата'")
	}
}

func TestGetAchievementsUseCase_Execute_NoPlayer(t *testing.T) {
	ctx := context.Background()
	chatID := int64(123)
	tgUserID := int64(456)

	activeSession := &session.GameSession{
		ChatID: chatID,
		State:  session.StateActive,
	}
	activeSession.ID = 1

	sessionRepo := &mockGetAchievementsSessionRepo{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			return activeSession, nil // Session without players
		},
	}

	achievementRepo := &mockGetAchievementsRepo{}

	uc := NewGetAchievementsUseCase(achievementRepo, sessionRepo)

	result, err := uc.Execute(ctx, GetAchievementsRequest{
		ChatID:   chatID,
		TgUserID: tgUserID,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if !strings.Contains(result, "Персонаж не создан") {
		t.Error("Execute() result should contain 'Персонаж не создан'")
	}
}

func TestGetAchievementsUseCase_Execute_RepositoryError(t *testing.T) {
	ctx := context.Background()
	chatID := int64(123)
	tgUserID := int64(456)

	activeSession := &session.GameSession{
		ChatID: chatID,
		State:  session.StateActive,
	}
	activeSession.ID = 1

	testPlayer := &player.Player{
		ID:       1,
		TgUserID: tgUserID,
		GameSessionID: 1,
	}

	sessionRepo := &mockGetAchievementsSessionRepo{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			gs := activeSession
			gs.Players = []player.Player{*testPlayer}
			return gs, nil
		},
	}

	achievementRepo := &mockGetAchievementsRepo{
		getPlayerAchievementsFunc: func(ctx context.Context, playerID uint) ([]*achievement.PlayerAchievement, error) {
			return nil, errors.New("database error")
		},
	}

	uc := NewGetAchievementsUseCase(achievementRepo, sessionRepo)

	_, err := uc.Execute(ctx, GetAchievementsRequest{
		ChatID:   chatID,
		TgUserID: tgUserID,
	})
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}

	if !strings.Contains(err.Error(), "failed to get player achievements") {
		t.Errorf("Execute() error = %v, want 'failed to get player achievements'", err)
	}
}

func TestGetAchievementsUseCase_Execute_EmptyAchievements(t *testing.T) {
	ctx := context.Background()
	chatID := int64(123)
	tgUserID := int64(456)

	activeSession := &session.GameSession{
		ChatID: chatID,
		State:  session.StateActive,
	}
	activeSession.ID = 1

	testPlayer := &player.Player{
		ID:       1,
		TgUserID: tgUserID,
		GameSessionID: 1,
	}

	sessionRepo := &mockGetAchievementsSessionRepo{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			gs := activeSession
			gs.Players = []player.Player{*testPlayer}
			return gs, nil
		},
	}

	achievementRepo := &mockGetAchievementsRepo{
		getPlayerAchievementsFunc: func(ctx context.Context, playerID uint) ([]*achievement.PlayerAchievement, error) {
			return []*achievement.PlayerAchievement{}, nil
		},
		getAllFunc: func(ctx context.Context) ([]*achievement.Achievement, error) {
			return []*achievement.Achievement{}, nil
		},
	}

	uc := NewGetAchievementsUseCase(achievementRepo, sessionRepo)

	result, err := uc.Execute(ctx, GetAchievementsRequest{
		ChatID:   chatID,
		TgUserID: tgUserID,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if !strings.Contains(result, "🏆 Ваши достижения") {
		t.Error("Execute() result should contain header")
	}

	if !strings.Contains(result, "Получено: 0 из 0 достижений") {
		t.Error("Execute() result should contain zero achievement count")
	}
}