package persistence

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"dungeons-and-dragons-ai/internal/game/domain/rating"
)

type RatingRepository struct {
	db *gorm.DB
}

func NewRatingRepository(db *gorm.DB) *RatingRepository {
	return &RatingRepository{db: db}
}

// Save сохраняет или обновляет рейтинг игрока
func (r *RatingRepository) Save(ctx context.Context, rating *rating.PlayerRating) error {
	return r.db.WithContext(ctx).
		Where("tg_user_id = ? AND metric_type = ?", rating.TgUserID, rating.MetricType).
		Assign(*rating).
		FirstOrCreate(rating).Error
}

// GetByTgUserIDAndMetric получает рейтинг игрока по метрике
func (r *RatingRepository) GetByTgUserIDAndMetric(ctx context.Context, tgUserID int64, metricType rating.RatingMetricType) (*rating.PlayerRating, error) {
	var rt rating.PlayerRating
	err := r.db.WithContext(ctx).
		Where("tg_user_id = ? AND metric_type = ?", tgUserID, metricType).
		First(&rt).Error

	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &rt, nil
}

// GetLeaderboard получает лидерборд по метрике с лимитом
func (r *RatingRepository) GetLeaderboard(ctx context.Context, metricType rating.RatingMetricType, limit int) ([]*rating.LeaderboardEntry, error) {
	// Получаем рейтинги, отсортированные по RatingScore (по убыванию)
	var ratings []rating.PlayerRating
	err := r.db.WithContext(ctx).
		Where("metric_type = ?", metricType).
		Order("rating_score DESC, last_updated DESC").
		Limit(limit).
		Find(&ratings).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get leaderboard: %w", err)
	}

	// Преобразуем в LeaderboardEntry с рангами
	entries := make([]*rating.LeaderboardEntry, 0, len(ratings))
	for i, rt := range ratings {
		// Получаем имя игрока из таблицы players
		// Используем Order("id DESC") вместо Order("created_at DESC"), так как в модели Player нет поля created_at
		// Это дает нам последнего созданного игрока с этим tg_user_id
		var playerName string
		r.db.WithContext(ctx).
			Table("players").
			Select("name").
			Where("tg_user_id = ?", rt.TgUserID).
			Order("id DESC").
			Limit(1).
			Scan(&playerName)

		if playerName == "" {
			playerName = fmt.Sprintf("Игрок #%d", rt.TgUserID)
		}

		// Определяем значение метрики для отображения
		metricValue := rt.RatingScore
		switch metricType {
		case rating.MetricTypeLevel:
			metricValue = rt.Level
		case rating.MetricTypeExperience:
			metricValue = rt.Experience
		case rating.MetricTypeCombatWins:
			metricValue = rt.CombatWins
		case rating.MetricTypeQuestsCompleted:
			metricValue = rt.QuestsCompleted
		case rating.MetricTypeTotalRating:
			metricValue = rt.RatingScore
		}

		entries = append(entries, &rating.LeaderboardEntry{
			Rank:        i + 1,
			TgUserID:    rt.TgUserID,
			PlayerName:  playerName,
			RatingScore: rt.RatingScore,
			MetricValue: metricValue,
		})
	}

	return entries, nil
}

// GetRank получает ранг игрока по метрике
func (r *RatingRepository) GetRank(ctx context.Context, tgUserID int64, metricType rating.RatingMetricType) (int, error) {
	var count int64

	// Получаем рейтинг игрока
	playerRating, err := r.GetByTgUserIDAndMetric(ctx, tgUserID, metricType)
	if err != nil {
		return 0, err
	}

	if playerRating == nil {
		return 0, nil // Игрок не в рейтинге
	}

	// Подсчитываем, сколько игроков имеют больший рейтинг
	err = r.db.WithContext(ctx).
		Model(&rating.PlayerRating{}).
		Where("metric_type = ? AND rating_score > ?", metricType, playerRating.RatingScore).
		Count(&count).Error

	if err != nil {
		return 0, fmt.Errorf("failed to get rank: %w", err)
	}

	return int(count) + 1, nil
}

// UpdateRanks обновляет ранги всех игроков по метрике
func (r *RatingRepository) UpdateRanks(ctx context.Context, metricType rating.RatingMetricType) error {
	// Получаем все рейтинги, отсортированные по убыванию
	var ratings []rating.PlayerRating
	err := r.db.WithContext(ctx).
		Where("metric_type = ?", metricType).
		Order("rating_score DESC, last_updated DESC").
		Find(&ratings).Error

	if err != nil {
		return fmt.Errorf("failed to get ratings: %w", err)
	}

	// Обновляем ранги
	for i := range ratings {
		newRank := i + 1
		ratings[i].PreviousRank = ratings[i].Rank
		ratings[i].Rank = newRank

		if err := r.db.WithContext(ctx).
			Model(&ratings[i]).
			Updates(map[string]interface{}{
				"rank":          newRank,
				"previous_rank": ratings[i].PreviousRank,
			}).Error; err != nil {
			return fmt.Errorf("failed to update rank: %w", err)
		}
	}

	return nil
}

// GetAllMetrics возвращает список всех типов метрик
func GetAllMetrics() []rating.RatingMetricType {
	return []rating.RatingMetricType{
		rating.MetricTypeLevel,
		rating.MetricTypeExperience,
		rating.MetricTypeCombatWins,
		rating.MetricTypeQuestsCompleted,
		rating.MetricTypeTotalRating,
	}
}
