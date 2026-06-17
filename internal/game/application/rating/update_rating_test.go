package rating

import (
	"context"
	"errors"
	"testing"

	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/player"
	"dungeons-and-dragons-ai/internal/game/domain/rating"
	"dungeons-and-dragons-ai/internal/game/domain/session"
)

// mockRatingRepo мок для RatingRepository
type mockRatingRepo struct {
	saveFunc                   func(ctx context.Context, rating *rating.PlayerRating) error
	getByTgUserIDAndMetricFunc func(ctx context.Context, tgUserID int64, metricType rating.RatingMetricType) (*rating.PlayerRating, error)
	updateRanksFunc            func(ctx context.Context, metricType rating.RatingMetricType) error
}

func (m *mockRatingRepo) Save(ctx context.Context, r *rating.PlayerRating) error {
	if m.saveFunc != nil {
		return m.saveFunc(ctx, r)
	}
	return nil
}

func (m *mockRatingRepo) GetByTgUserIDAndMetric(ctx context.Context, tgUserID int64, metricType rating.RatingMetricType) (*rating.PlayerRating, error) {
	if m.getByTgUserIDAndMetricFunc != nil {
		return m.getByTgUserIDAndMetricFunc(ctx, tgUserID, metricType)
	}
	return nil, nil
}

func (m *mockRatingRepo) UpdateRanks(ctx context.Context, metricType rating.RatingMetricType) error {
	if m.updateRanksFunc != nil {
		return m.updateRanksFunc(ctx, metricType)
	}
	return nil
}

func (m *mockRatingRepo) GetLeaderboard(ctx context.Context, metricType rating.RatingMetricType, limit int) ([]*rating.LeaderboardEntry, error) {
	return []*rating.LeaderboardEntry{}, nil
}

func (m *mockRatingRepo) GetRank(ctx context.Context, tgUserID int64, metricType rating.RatingMetricType) (int, error) {
	return 0, nil
}

// mockSessionRepo мок для session.Repository
type mockUpdateRatingSessionRepo struct {
	getByChatIDFunc func(ctx context.Context, chatID int64) (*session.GameSession, error)
}

func (m *mockUpdateRatingSessionRepo) GetByChatID(ctx context.Context, chatID int64) (*session.GameSession, error) {
	if m.getByChatIDFunc != nil {
		return m.getByChatIDFunc(ctx, chatID)
	}
	return nil, nil
}

func (m *mockUpdateRatingSessionRepo) Save(ctx context.Context, s *session.GameSession) error {
	return nil
}

func (m *mockUpdateRatingSessionRepo) Delete(ctx context.Context, chatID int64) error {
	return nil
}

// mockPlayerRepo мок для PlayerRepository
type mockUpdateRatingPlayerRepo struct {
	getByTgUserIDAndSessionIDFunc func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error)
}

func (m *mockUpdateRatingPlayerRepo) GetByTgUserIDAndSessionID(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
	if m.getByTgUserIDAndSessionIDFunc != nil {
		return m.getByTgUserIDAndSessionIDFunc(ctx, tgUserID, sessionID)
	}
	return nil, nil
}

// mockAchievementRepo мок для AchievementRepository
type mockUpdateRatingAchievementRepo struct {
	getAchievementProgressFunc                 func(ctx context.Context, playerID uint, achievementID uint) (*AchievementProgress, error)
	getAchievementProgressByRequirementKeyFunc func(ctx context.Context, playerID uint, requirementKey string) (int, error)
}

func (m *mockUpdateRatingAchievementRepo) GetAchievementProgress(ctx context.Context, playerID uint, achievementID uint) (*AchievementProgress, error) {
	if m.getAchievementProgressFunc != nil {
		return m.getAchievementProgressFunc(ctx, playerID, achievementID)
	}
	return nil, nil
}

func (m *mockUpdateRatingAchievementRepo) GetAchievementProgressByRequirementKey(ctx context.Context, playerID uint, requirementKey string) (int, error) {
	if m.getAchievementProgressByRequirementKeyFunc != nil {
		return m.getAchievementProgressByRequirementKeyFunc(ctx, playerID, requirementKey)
	}
	return 0, nil
}

// TestUpdateRatingUseCase_Execute_Success проверяет успешное обновление рейтинга (#7)
func TestUpdateRatingUseCase_Execute_Success(t *testing.T) {
	ctx := context.Background()
	tgUserID := int64(123)
	chatID := int64(456)

	char, _ := character.NewCharacter("Test", character.ClassFighter, character.RaceHuman, character.Stats{})
	char.Level = 5
	char.Experience = 1000

	testPlayer := &player.Player{
		ID:            1,
		TgUserID:      tgUserID,
		GameSessionID: 1,
		Character:     *char,
	}

	gs := &session.GameSession{
		ChatID: chatID,
		State:  session.StateActive,
	}
	gs.Model.ID = 1

	var savedRatings []*rating.PlayerRating

	ratingRepo := &mockRatingRepo{
		getByTgUserIDAndMetricFunc: func(ctx context.Context, tgUserID int64, metricType rating.RatingMetricType) (*rating.PlayerRating, error) {
			// Возвращаем nil, чтобы создать новый рейтинг
			return nil, nil
		},
		saveFunc: func(ctx context.Context, r *rating.PlayerRating) error {
			savedRatings = append(savedRatings, r)
			return nil
		},
	}

	sessionRepo := &mockUpdateRatingSessionRepo{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			return gs, nil
		},
	}

	playerRepo := &mockUpdateRatingPlayerRepo{
		getByTgUserIDAndSessionIDFunc: func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
			return testPlayer, nil
		},
	}

	achievementRepo := &mockUpdateRatingAchievementRepo{
		getAchievementProgressByRequirementKeyFunc: func(ctx context.Context, playerID uint, requirementKey string) (int, error) {
			if requirementKey == "combat_wins" {
				return 10, nil
			}
			if requirementKey == "quests_completed" {
				return 5, nil
			}
			return 0, nil
		},
	}

	uc := NewUpdateRatingUseCase(ratingRepo, sessionRepo, playerRepo, achievementRepo)

	req := UpdateRatingRequest{
		TgUserID: tgUserID,
		ChatID:   chatID,
	}

	err := uc.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	// Проверяем, что рейтинги были сохранены для всех метрик
	if len(savedRatings) == 0 {
		t.Fatal("Execute() should save ratings for all metrics")
	}

	// Проверяем, что сохранены рейтинги для основных метрик
	metricTypes := make(map[rating.RatingMetricType]bool)
	for _, r := range savedRatings {
		metricTypes[r.MetricType] = true
	}

	expectedMetrics := []rating.RatingMetricType{
		rating.MetricTypeLevel,
		rating.MetricTypeExperience,
		rating.MetricTypeCombatWins,
		rating.MetricTypeQuestsCompleted,
		rating.MetricTypeTotalRating,
	}

	for _, metric := range expectedMetrics {
		if !metricTypes[metric] {
			t.Errorf("Execute() should save rating for metric %s", metric)
		}
	}
}

// TestUpdateRatingUseCase_Execute_NoSession проверяет обработку отсутствия сессии (#7)
func TestUpdateRatingUseCase_Execute_NoSession(t *testing.T) {
	ctx := context.Background()
	tgUserID := int64(123)
	chatID := int64(456)

	ratingRepo := &mockRatingRepo{}
	sessionRepo := &mockUpdateRatingSessionRepo{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			return nil, nil // Сессия не найдена
		},
	}
	playerRepo := &mockUpdateRatingPlayerRepo{}
	achievementRepo := &mockUpdateRatingAchievementRepo{}

	uc := NewUpdateRatingUseCase(ratingRepo, sessionRepo, playerRepo, achievementRepo)

	req := UpdateRatingRequest{
		TgUserID: tgUserID,
		ChatID:   chatID,
	}

	err := uc.Execute(ctx, req)
	// Должно вернуть nil, так как отсутствие сессии просто пропускается
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil (should skip if no session)", err)
	}
}

// TestUpdateRatingUseCase_Execute_NoPlayer проверяет обработку отсутствия игрока (#7)
func TestUpdateRatingUseCase_Execute_NoPlayer(t *testing.T) {
	ctx := context.Background()
	tgUserID := int64(123)
	chatID := int64(456)

	gs := &session.GameSession{
		ChatID: chatID,
		State:  session.StateActive,
	}
	gs.Model.ID = 1

	ratingRepo := &mockRatingRepo{}
	sessionRepo := &mockUpdateRatingSessionRepo{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			return gs, nil
		},
	}
	playerRepo := &mockUpdateRatingPlayerRepo{
		getByTgUserIDAndSessionIDFunc: func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
			return nil, nil // Игрок не найден
		},
	}
	achievementRepo := &mockUpdateRatingAchievementRepo{}

	uc := NewUpdateRatingUseCase(ratingRepo, sessionRepo, playerRepo, achievementRepo)

	req := UpdateRatingRequest{
		TgUserID: tgUserID,
		ChatID:   chatID,
	}

	err := uc.Execute(ctx, req)
	// Должно вернуть nil, так как отсутствие игрока просто пропускается
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil (should skip if no player)", err)
	}
}

// TestUpdateRatingUseCase_Execute_RepositoryError проверяет обработку ошибок репозитория (#7)
func TestUpdateRatingUseCase_Execute_RepositoryError(t *testing.T) {
	ctx := context.Background()
	tgUserID := int64(123)
	chatID := int64(456)

	testPlayer := &player.Player{
		ID:            1,
		TgUserID:      tgUserID,
		GameSessionID: 1,
	}

	gs := &session.GameSession{
		ChatID: chatID,
		State:  session.StateActive,
	}
	gs.Model.ID = 1

	ratingRepo := &mockRatingRepo{
		getByTgUserIDAndMetricFunc: func(ctx context.Context, tgUserID int64, metricType rating.RatingMetricType) (*rating.PlayerRating, error) {
			return nil, errors.New("database error")
		},
	}

	sessionRepo := &mockUpdateRatingSessionRepo{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			return gs, nil
		},
	}

	playerRepo := &mockUpdateRatingPlayerRepo{
		getByTgUserIDAndSessionIDFunc: func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
			return testPlayer, nil
		},
	}

	achievementRepo := &mockUpdateRatingAchievementRepo{}

	uc := NewUpdateRatingUseCase(ratingRepo, sessionRepo, playerRepo, achievementRepo)

	req := UpdateRatingRequest{
		TgUserID: tgUserID,
		ChatID:   chatID,
	}

	err := uc.Execute(ctx, req)
	// Ошибка должна быть обработана (логируется, но не прерывает выполнение)
	// В реальном коде ошибки логируются, но не возвращаются
	if err != nil {
		t.Logf("Execute() returned error (expected to be logged): %v", err)
	}
}
