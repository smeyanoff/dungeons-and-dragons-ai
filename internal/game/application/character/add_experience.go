package character

import (
	"context"
	"fmt"

	achievementapp "dungeons-and-dragons-ai/internal/game/application/achievement"
	"dungeons-and-dragons-ai/internal/game/domain/player"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/pkg/logger"
)

type AddExperienceUseCase struct {
	playerRepo          PlayerRepository
	sessionRepo         session.Repository
	checkAchievementsUC *achievementapp.CheckAchievementsUseCase // Опциональная зависимость для проверки достижений
	notificationService achievementapp.NotificationService       // Опциональная зависимость для отправки уведомлений
	updateRatingUC      RatingUpdater                            // Опциональная зависимость для обновления рейтингов
}

// RatingUpdater интерфейс для обновления рейтингов
type RatingUpdater interface {
	Execute(ctx context.Context, req RatingUpdateRequest) error
}

// RatingUpdateRequest запрос на обновление рейтинга
type RatingUpdateRequest struct {
	TgUserID int64
	ChatID   int64
}

func NewAddExperienceUseCase(
	playerRepo PlayerRepository,
	sessionRepo session.Repository,
) *AddExperienceUseCase {
	return &AddExperienceUseCase{
		playerRepo:  playerRepo,
		sessionRepo: sessionRepo,
	}
}

// SetCheckAchievementsUseCase устанавливает CheckAchievementsUseCase для проверки достижений
func (uc *AddExperienceUseCase) SetCheckAchievementsUseCase(checkAchievementsUC *achievementapp.CheckAchievementsUseCase) {
	uc.checkAchievementsUC = checkAchievementsUC
}

// SetNotificationService устанавливает NotificationService для отправки уведомлений
func (uc *AddExperienceUseCase) SetNotificationService(notificationService achievementapp.NotificationService) {
	uc.notificationService = notificationService
}

// SetRatingUpdater устанавливает RatingUpdater для обновления рейтингов
func (uc *AddExperienceUseCase) SetRatingUpdater(updateRatingUC RatingUpdater) {
	uc.updateRatingUC = updateRatingUC
}

type AddExperienceRequest struct {
	ChatID int64
	Amount int
	Reason string // Причина начисления опыта (например, "quest_completed", "combat_victory")
}

func (uc *AddExperienceUseCase) Execute(
	ctx context.Context,
	req AddExperienceRequest,
) (*player.Player, bool, error) {
	// Получаем сессию
	gs, err := uc.sessionRepo.GetByChatID(ctx, req.ChatID)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get session: %w", err)
	}

	if gs == nil {
		return nil, false, fmt.Errorf("game session not found")
	}

	// Получаем игрока
	p, err := uc.playerRepo.GetByTgUserIDAndSessionID(ctx, req.ChatID, gs.Model.ID)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get player: %w", err)
	}

	if p == nil {
		return nil, false, fmt.Errorf("player not found, create character first")
	}

	// Начисляем опыт
	oldLevel := p.Character.Level
	leveledUp, err := p.Character.AddExperience(req.Amount)
	if err != nil {
		return nil, false, fmt.Errorf("failed to add experience: %w", err)
	}

	// Сохраняем изменения
	if err := uc.playerRepo.Save(ctx, p); err != nil {
		return nil, false, fmt.Errorf("failed to save player: %w", err)
	}

	// Проверяем достижения по уровню после повышения уровня
	if uc.checkAchievementsUC != nil && leveledUp {
		// Проверяем достижения по уровню персонажа
		achievementReq := achievementapp.CheckAchievementsRequest{
			PlayerID:       p.ID,
			RequirementKey: "character_level",
			CurrentValue:   p.Character.Level,
		}

		unlocked, err := uc.checkAchievementsUC.Execute(ctx, achievementReq)
		if err != nil {
			// Логируем ошибку, но не прерываем выполнение
			logger.Warn("Failed to check achievements after level up",
				logger.ErrorField(err),
				logger.Uint("player_id", p.ID),
				logger.Int("new_level", p.Character.Level),
			)
		} else if len(unlocked) > 0 {
			// Логируем и отправляем уведомления о разблокированных достижениях
			for _, achievement := range unlocked {
				logger.Info("Achievement unlocked after level up",
					logger.Uint("player_id", p.ID),
					logger.String("achievement_code", achievement.Achievement.Code),
					logger.String("achievement_title", achievement.Achievement.Title),
					logger.Int("new_level", p.Character.Level),
				)

				// Отправляем уведомление пользователю, если есть notification service
				if uc.notificationService != nil {
					if err := uc.notificationService.SendAchievementNotification(ctx, req.ChatID, achievement.Message); err != nil {
						logger.Warn("Failed to send achievement notification",
							logger.ErrorField(err),
							logger.Uint("player_id", p.ID),
							logger.String("achievement_code", achievement.Achievement.Code),
						)
					}
				}
			}
		}

		// Также проверяем достижения при каждом изменении уровня (на случай, если достижения привязаны к конкретным уровням)
		if oldLevel != p.Character.Level {
			// Проверка уже выполнена выше для нового уровня
			// Здесь можно добавить дополнительную логику, если нужно
		}
	}

	// Обновляем рейтинг игрока после изменения опыта/уровня
	if uc.updateRatingUC != nil {
		ratingReq := RatingUpdateRequest{
			TgUserID: p.TgUserID,
			ChatID:   req.ChatID,
		}
		if err := uc.updateRatingUC.Execute(ctx, ratingReq); err != nil {
			// Логируем ошибку, но не прерываем выполнение
			logger.Warn("Failed to update rating after experience gain",
				logger.ErrorField(err),
				logger.Uint("player_id", p.ID),
				logger.Int64("tg_user_id", p.TgUserID),
			)
		}
	}

	return p, leveledUp, nil
}
