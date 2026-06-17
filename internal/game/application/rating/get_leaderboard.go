package rating

import (
	"context"
	"fmt"

	"dungeons-and-dragons-ai/internal/game/domain/rating"
)

type GetLeaderboardUseCase struct {
	ratingRepo RatingRepository
}

type RatingRepository interface {
	GetLeaderboard(ctx context.Context, metricType rating.RatingMetricType, limit int) ([]*rating.LeaderboardEntry, error)
	GetRank(ctx context.Context, tgUserID int64, metricType rating.RatingMetricType) (int, error)
	Save(ctx context.Context, rating *rating.PlayerRating) error
	GetByTgUserIDAndMetric(ctx context.Context, tgUserID int64, metricType rating.RatingMetricType) (*rating.PlayerRating, error)
	UpdateRanks(ctx context.Context, metricType rating.RatingMetricType) error
}

func NewGetLeaderboardUseCase(ratingRepo RatingRepository) *GetLeaderboardUseCase {
	return &GetLeaderboardUseCase{
		ratingRepo: ratingRepo,
	}
}

// GetLeaderboardRequest запрос на получение лидерборда
type GetLeaderboardRequest struct {
	MetricType rating.RatingMetricType // Тип метрики для лидерборда
	Limit      int                     // Количество записей (по умолчанию 10)
	TgUserID   int64                   // ID пользователя для получения его ранга
}

// GetLeaderboardResponse ответ с лидербордом
type GetLeaderboardResponse struct {
	MetricType string                     // Тип метрики (для отображения)
	Entries    []*rating.LeaderboardEntry // Записи лидерборда
	UserRank   int                        // Ранг текущего пользователя (0 если не в топе)
	UserRating int                        // Рейтинг текущего пользователя
}

// Execute получает лидерборд по указанной метрике
func (uc *GetLeaderboardUseCase) Execute(ctx context.Context, req GetLeaderboardRequest) (*GetLeaderboardResponse, error) {
	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 10 // По умолчанию топ-10
	}

	// Получаем лидерборд
	entries, err := uc.ratingRepo.GetLeaderboard(ctx, req.MetricType, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get leaderboard: %w", err)
	}

	// Получаем ранг пользователя
	userRank := 0
	userRating := 0
	if req.TgUserID > 0 {
		rank, err := uc.ratingRepo.GetRank(ctx, req.TgUserID, req.MetricType)
		if err == nil {
			userRank = rank
			// Находим рейтинг пользователя в лидерборде
			for _, entry := range entries {
				if entry.TgUserID == req.TgUserID {
					userRating = entry.RatingScore
					break
				}
			}
		}
	}

	// Получаем название метрики для отображения
	metricName := getMetricDisplayName(req.MetricType)

	return &GetLeaderboardResponse{
		MetricType: metricName,
		Entries:    entries,
		UserRank:   userRank,
		UserRating: userRating,
	}, nil
}

// getMetricDisplayName возвращает отображаемое название метрики
func getMetricDisplayName(metricType rating.RatingMetricType) string {
	names := map[rating.RatingMetricType]string{
		rating.MetricTypeLevel:           "Уровень",
		rating.MetricTypeExperience:      "Опыт",
		rating.MetricTypeCombatWins:      "Победы в боях",
		rating.MetricTypeQuestsCompleted: "Завершенные квесты",
		rating.MetricTypeTotalRating:     "Общий рейтинг",
	}

	if name, ok := names[metricType]; ok {
		return name
	}
	return string(metricType)
}
