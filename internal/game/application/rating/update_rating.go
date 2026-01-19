package rating

import (
	"context"
	"fmt"

	"dungeons-and-dragons-ai/internal/game/domain/player"
	"dungeons-and-dragons-ai/internal/game/domain/rating"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/pkg/logger"
)

type UpdateRatingUseCase struct {
	ratingRepo     RatingRepository
	sessionRepo    session.Repository
	playerRepo     PlayerRepository
	achievementRepo AchievementRepository // Опциональная зависимость для сбора статистик
}

type PlayerRepository interface {
	GetByTgUserIDAndSessionID(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error)
}

type AchievementRepository interface {
	GetAchievementProgress(ctx context.Context, playerID uint, achievementID uint) (*AchievementProgress, error)
	GetAchievementProgressByRequirementKey(ctx context.Context, playerID uint, requirementKey string) (int, error)
}

// AchievementProgress представляет прогресс по достижению (локальный тип)
type AchievementProgress struct {
	PlayerID     uint
	AchievementID uint
	CurrentValue int
}

func NewUpdateRatingUseCase(
	ratingRepo RatingRepository,
	sessionRepo session.Repository,
	playerRepo PlayerRepository,
	achievementRepo AchievementRepository, // Опциональная зависимость
) *UpdateRatingUseCase {
	return &UpdateRatingUseCase{
		ratingRepo:     ratingRepo,
		sessionRepo:    sessionRepo,
		playerRepo:     playerRepo,
		achievementRepo: achievementRepo,
	}
}

// UpdateRatingRequest запрос на обновление рейтинга
type UpdateRatingRequest struct {
	TgUserID int64 // Telegram User ID игрока
	ChatID   int64 // Chat ID для получения сессии
}

// Execute обновляет рейтинг игрока на основе текущих статистик
func (uc *UpdateRatingUseCase) Execute(ctx context.Context, req UpdateRatingRequest) error {
	// Получаем сессию для получения статистик
	gs, err := uc.sessionRepo.GetByChatID(ctx, req.ChatID)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}
	
	if gs == nil {
		return nil // Сессия не найдена, пропускаем
	}
	
	// Получаем игрока
	player, err := uc.playerRepo.GetByTgUserIDAndSessionID(ctx, req.TgUserID, gs.ID)
	if err != nil {
		return fmt.Errorf("failed to get player: %w", err)
	}
	
	if player == nil {
		return nil // Игрок не найден, пропускаем
	}
	
	// Собираем статистики игрока
	stats := uc.collectPlayerStats(ctx, player)
	
	// Обновляем рейтинги по всем метрикам
	if err := uc.updateAllRatings(ctx, req.TgUserID, stats); err != nil {
		return fmt.Errorf("failed to update ratings: %w", err)
	}
	
	return nil
}

// collectPlayerStats собирает статистики игрока из различных источников
func (uc *UpdateRatingUseCase) collectPlayerStats(ctx context.Context, p *player.Player) rating.PlayerStats {
	stats := rating.PlayerStats{
		TgUserID:        p.TgUserID,
		Level:           p.Character.Level,
		Experience:      p.Character.Experience,
		CombatWins:      0,
		QuestsCompleted: 0,
	}
	
	// Собираем статистики из достижений, если репозиторий доступен
	if uc.achievementRepo != nil {
		// Получаем прогресс по победам в боях
		combatWins, err := uc.achievementRepo.GetAchievementProgressByRequirementKey(ctx, p.ID, "combat_wins")
		if err == nil {
			stats.CombatWins = combatWins
		}
		
		// Получаем прогресс по завершенным квестам
		questsCompleted, err := uc.achievementRepo.GetAchievementProgressByRequirementKey(ctx, p.ID, "quests_completed")
		if err == nil {
			stats.QuestsCompleted = questsCompleted
		}
	}
	
	return stats
}

// updateAllRatings обновляет все рейтинги игрока
func (uc *UpdateRatingUseCase) updateAllRatings(ctx context.Context, tgUserID int64, stats rating.PlayerStats) error {
	metrics := []rating.RatingMetricType{
		rating.MetricTypeLevel,
		rating.MetricTypeExperience,
		rating.MetricTypeCombatWins,
		rating.MetricTypeQuestsCompleted,
		rating.MetricTypeTotalRating,
	}
	
	for _, metricType := range metrics {
		// Получаем или создаем рейтинг
		playerRating, err := uc.ratingRepo.GetByTgUserIDAndMetric(ctx, tgUserID, metricType)
		if err != nil {
			logger.Warn("Failed to get rating",
				logger.ErrorField(err),
				logger.Int64("tg_user_id", tgUserID),
				logger.String("metric_type", string(metricType)),
			)
			continue
		}
		
		if playerRating == nil {
			// Создаем новый рейтинг
			playerRating = &rating.PlayerRating{
				TgUserID:   tgUserID,
				MetricType: metricType,
			}
		}
		
		// Обновляем из статистик
		playerRating.UpdateFromStats(stats)
		
		// Сохраняем
		if err := uc.ratingRepo.Save(ctx, playerRating); err != nil {
			logger.Warn("Failed to save rating",
				logger.ErrorField(err),
				logger.Int64("tg_user_id", tgUserID),
				logger.String("metric_type", string(metricType)),
			)
			continue
		}
	}
	
	return nil
}
