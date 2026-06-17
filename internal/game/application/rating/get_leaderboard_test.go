package rating

import (
	"context"
	"errors"
	"testing"

	"dungeons-and-dragons-ai/internal/game/domain/rating"
)

// mockGetLeaderboardRatingRepo мок для RatingRepository для GetLeaderboardUseCase
type mockGetLeaderboardRatingRepo struct {
	getLeaderboardFunc func(ctx context.Context, metricType rating.RatingMetricType, limit int) ([]*rating.LeaderboardEntry, error)
	getRankFunc        func(ctx context.Context, tgUserID int64, metricType rating.RatingMetricType) (int, error)
}

func (m *mockGetLeaderboardRatingRepo) GetLeaderboard(ctx context.Context, metricType rating.RatingMetricType, limit int) ([]*rating.LeaderboardEntry, error) {
	if m.getLeaderboardFunc != nil {
		return m.getLeaderboardFunc(ctx, metricType, limit)
	}
	return []*rating.LeaderboardEntry{}, nil
}

func (m *mockGetLeaderboardRatingRepo) GetRank(ctx context.Context, tgUserID int64, metricType rating.RatingMetricType) (int, error) {
	if m.getRankFunc != nil {
		return m.getRankFunc(ctx, tgUserID, metricType)
	}
	return 0, nil
}

func (m *mockGetLeaderboardRatingRepo) Save(ctx context.Context, r *rating.PlayerRating) error {
	return nil
}

func (m *mockGetLeaderboardRatingRepo) GetByTgUserIDAndMetric(ctx context.Context, tgUserID int64, metricType rating.RatingMetricType) (*rating.PlayerRating, error) {
	return nil, nil
}

func (m *mockGetLeaderboardRatingRepo) UpdateRanks(ctx context.Context, metricType rating.RatingMetricType) error {
	return nil
}

// TestGetLeaderboardUseCase_Execute_Success проверяет успешное получение лидерборда (#7)
func TestGetLeaderboardUseCase_Execute_Success(t *testing.T) {
	ctx := context.Background()
	tgUserID := int64(123)

	entries := []*rating.LeaderboardEntry{
		{
			TgUserID:    456,
			RatingScore: 1000,
			Rank:        1,
		},
		{
			TgUserID:    789,
			RatingScore: 900,
			Rank:        2,
		},
		{
			TgUserID:    tgUserID,
			RatingScore: 800,
			Rank:        3,
		},
	}

	ratingRepo := &mockGetLeaderboardRatingRepo{
		getLeaderboardFunc: func(ctx context.Context, metricType rating.RatingMetricType, limit int) ([]*rating.LeaderboardEntry, error) {
			return entries, nil
		},
		getRankFunc: func(ctx context.Context, tgUserID int64, metricType rating.RatingMetricType) (int, error) {
			return 3, nil
		},
	}

	uc := NewGetLeaderboardUseCase(ratingRepo)

	req := GetLeaderboardRequest{
		MetricType: rating.MetricTypeLevel,
		Limit:      10,
		TgUserID:   tgUserID,
	}

	resp, err := uc.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if resp == nil {
		t.Fatal("Execute() response = nil, want non-nil")
	}

	if len(resp.Entries) != 3 {
		t.Errorf("Execute() Entries count = %d, want 3", len(resp.Entries))
	}

	if resp.UserRank != 3 {
		t.Errorf("Execute() UserRank = %d, want 3", resp.UserRank)
	}

	if resp.UserRating != 800 {
		t.Errorf("Execute() UserRating = %d, want 800", resp.UserRating)
	}
}

// TestGetLeaderboardUseCase_Execute_DefaultLimit проверяет использование лимита по умолчанию (#7)
func TestGetLeaderboardUseCase_Execute_DefaultLimit(t *testing.T) {
	ctx := context.Background()

	var receivedLimit int

	ratingRepo := &mockGetLeaderboardRatingRepo{
		getLeaderboardFunc: func(ctx context.Context, metricType rating.RatingMetricType, limit int) ([]*rating.LeaderboardEntry, error) {
			receivedLimit = limit
			return []*rating.LeaderboardEntry{}, nil
		},
	}

	uc := NewGetLeaderboardUseCase(ratingRepo)

	req := GetLeaderboardRequest{
		MetricType: rating.MetricTypeExperience,
		Limit:      0, // Невалидный лимит
		TgUserID:   0,
	}

	resp, err := uc.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if receivedLimit != 10 {
		t.Errorf("Execute() Limit = %d, want 10 (default)", receivedLimit)
	}

	if resp == nil {
		t.Fatal("Execute() response = nil, want non-nil")
	}
}

// TestGetLeaderboardUseCase_Execute_MaxLimit проверяет ограничение максимального лимита (#7)
func TestGetLeaderboardUseCase_Execute_MaxLimit(t *testing.T) {
	ctx := context.Background()

	var receivedLimit int

	ratingRepo := &mockGetLeaderboardRatingRepo{
		getLeaderboardFunc: func(ctx context.Context, metricType rating.RatingMetricType, limit int) ([]*rating.LeaderboardEntry, error) {
			receivedLimit = limit
			return []*rating.LeaderboardEntry{}, nil
		},
	}

	uc := NewGetLeaderboardUseCase(ratingRepo)

	req := GetLeaderboardRequest{
		MetricType: rating.MetricTypeCombatWins,
		Limit:      200, // Превышает максимум
		TgUserID:   0,
	}

	resp, err := uc.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if receivedLimit != 10 {
		t.Errorf("Execute() Limit = %d, want 10 (max limit)", receivedLimit)
	}

	if resp == nil {
		t.Fatal("Execute() response = nil, want non-nil")
	}
}

// TestGetLeaderboardUseCase_Execute_RepositoryError проверяет обработку ошибок репозитория (#7)
func TestGetLeaderboardUseCase_Execute_RepositoryError(t *testing.T) {
	ctx := context.Background()

	ratingRepo := &mockGetLeaderboardRatingRepo{
		getLeaderboardFunc: func(ctx context.Context, metricType rating.RatingMetricType, limit int) ([]*rating.LeaderboardEntry, error) {
			return nil, errors.New("database error")
		},
	}

	uc := NewGetLeaderboardUseCase(ratingRepo)

	req := GetLeaderboardRequest{
		MetricType: rating.MetricTypeQuestsCompleted,
		Limit:      10,
		TgUserID:   0,
	}

	resp, err := uc.Execute(ctx, req)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}

	if resp != nil {
		t.Error("Execute() response should be nil on error")
	}
}

// TestGetLeaderboardUseCase_Execute_NoUserRank проверяет работу без указания пользователя (#7)
func TestGetLeaderboardUseCase_Execute_NoUserRank(t *testing.T) {
	ctx := context.Background()

	entries := []*rating.LeaderboardEntry{
		{
			TgUserID:    456,
			RatingScore: 1000,
			Rank:        1,
		},
	}

	ratingRepo := &mockGetLeaderboardRatingRepo{
		getLeaderboardFunc: func(ctx context.Context, metricType rating.RatingMetricType, limit int) ([]*rating.LeaderboardEntry, error) {
			return entries, nil
		},
		// getRankFunc не вызывается, так как TgUserID = 0
	}

	uc := NewGetLeaderboardUseCase(ratingRepo)

	req := GetLeaderboardRequest{
		MetricType: rating.MetricTypeTotalRating,
		Limit:      10,
		TgUserID:   0, // Не указан пользователь
	}

	resp, err := uc.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if resp == nil {
		t.Fatal("Execute() response = nil, want non-nil")
	}

	if resp.UserRank != 0 {
		t.Errorf("Execute() UserRank = %d, want 0 (not in leaderboard)", resp.UserRank)
	}
}
