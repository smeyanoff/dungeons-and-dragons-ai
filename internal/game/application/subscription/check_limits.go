package subscription

import (
	"context"
	"fmt"

	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/game/infrastructure/persistence"
	"dungeons-and-dragons-ai/pkg/logger"
)

type CheckLimitsUseCase struct {
	subscriptionRepo *persistence.SubscriptionRepository
	sessionRepo      session.Repository
	sessionCountRepo SessionCountRepository // Для подсчета активных игр и сохранений
	eventCountRepo   EventCountRepository   // Для подсчета сообщений
}

// SessionCountRepository интерфейс для подсчета сессий
type SessionCountRepository interface {
	CountActiveGamesByTgUserID(ctx context.Context, tgUserID int64) (int, error)
	CountSavesByTgUserID(ctx context.Context, tgUserID int64) (int, error)
}

// EventCountRepository интерфейс для подсчета событий
type EventCountRepository interface {
	CountMessagesPerDayByTgUserID(ctx context.Context, tgUserID int64) (int, error)
}

func NewCheckLimitsUseCase(
	subscriptionRepo *persistence.SubscriptionRepository,
	sessionRepo session.Repository,
	sessionCountRepo SessionCountRepository,
	eventCountRepo EventCountRepository,
) *CheckLimitsUseCase {
	return &CheckLimitsUseCase{
		subscriptionRepo: subscriptionRepo,
		sessionRepo:      sessionRepo,
		sessionCountRepo: sessionCountRepo,
		eventCountRepo:   eventCountRepo,
	}
}

type LimitType string

const (
	LimitTypeActiveGames    LimitType = "active_games"
	LimitTypeMessagesPerDay LimitType = "messages_per_day"
	LimitTypeImagesPerGame  LimitType = "images_per_game"
	LimitTypeSaves          LimitType = "saves"
)

type CheckLimitRequest struct {
	TgUserID int64
	// ChatID — требуется для лимитов, привязанных к конкретной игре (например, LimitTypeImagesPerGame)
	ChatID    int64
	LimitType LimitType
}

type CheckLimitResponse struct {
	Allowed   bool
	Limit     int
	Current   int
	Remaining int
	Message   string
}

// Execute проверяет, не превышен ли лимит для пользователя
func (uc *CheckLimitsUseCase) Execute(ctx context.Context, req CheckLimitRequest) (*CheckLimitResponse, error) {
	// Получаем подписку пользователя
	sub, err := uc.subscriptionRepo.GetByTgUserID(ctx, req.TgUserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get subscription: %w", err)
	}

	planDetails := sub.GetPlanDetails()
	var limit int
	var current int
	var remaining int

	switch req.LimitType {
	case LimitTypeActiveGames:
		limit = planDetails.MaxActiveGames
		if limit == 0 {
			// безлимит
			return &CheckLimitResponse{
				Allowed:   true,
				Limit:     0,
				Current:   0,
				Remaining: -1,
				Message:   "У вас безлимит активных игр",
			}, nil
		}
		// Считаем активные игры пользователя
		if uc.sessionCountRepo != nil {
			count, err := uc.sessionCountRepo.CountActiveGamesByTgUserID(ctx, req.TgUserID)
			if err != nil {
				logger.Warn("CheckLimitsUseCase: failed to count active games",
					logger.ErrorField(err),
					logger.Int64("tg_user_id", req.TgUserID),
				)
				// В случае ошибки считаем 0 для безопасности
				current = 0
			} else {
				current = count
			}
		} else {
			current = 0
		}
		remaining = limit - current

	case LimitTypeMessagesPerDay:
		limit = planDetails.MaxMessagesPerDay
		if limit == 0 {
			// безлимит
			return &CheckLimitResponse{
				Allowed:   true,
				Limit:     0,
				Current:   0,
				Remaining: -1,
				Message:   "У вас безлимит сообщений",
			}, nil
		}
		// Считаем сообщения за день
		if uc.eventCountRepo != nil {
			count, err := uc.eventCountRepo.CountMessagesPerDayByTgUserID(ctx, req.TgUserID)
			if err != nil {
				logger.Warn("CheckLimitsUseCase: failed to count messages",
					logger.ErrorField(err),
					logger.Int64("tg_user_id", req.TgUserID),
				)
				// В случае ошибки считаем 0 для безопасности
				current = 0
			} else {
				current = count
			}
		} else {
			current = 0
		}
		remaining = limit - current

	case LimitTypeImagesPerGame:
		limit = planDetails.MaxImagesPerGame
		if limit == 0 {
			// безлимит
			return &CheckLimitResponse{
				Allowed:   true,
				Limit:     0,
				Current:   0,
				Remaining: -1,
				Message:   "У вас безлимит изображений",
			}, nil
		}
		// Считаем изображения за ЭТУ игру (счётчик хранится в GameSession, не сбрасывается по дням)
		current = 0
		if uc.sessionRepo != nil && req.ChatID != 0 {
			gs, err := uc.sessionRepo.GetByChatID(ctx, req.ChatID)
			if err != nil {
				logger.Warn("CheckLimitsUseCase: failed to get session for image limit",
					logger.ErrorField(err),
					logger.Int64("chat_id", req.ChatID),
				)
			} else if gs != nil {
				current = GetOptionalActivityUsage(gs, "image_generation")
			}
		}
		remaining = limit - current

	case LimitTypeSaves:
		limit = planDetails.MaxSaves
		if limit == 0 {
			// безлимит
			return &CheckLimitResponse{
				Allowed:   true,
				Limit:     0,
				Current:   0,
				Remaining: -1,
				Message:   "У вас безлимит сохранений",
			}, nil
		}
		// Считаем сохранения пользователя (завершенные сессии)
		if uc.sessionCountRepo != nil {
			count, err := uc.sessionCountRepo.CountSavesByTgUserID(ctx, req.TgUserID)
			if err != nil {
				logger.Warn("CheckLimitsUseCase: failed to count saves",
					logger.ErrorField(err),
					logger.Int64("tg_user_id", req.TgUserID),
				)
				// В случае ошибки считаем 0 для безопасности
				current = 0
			} else {
				current = count
			}
		} else {
			current = 0
		}
		remaining = limit - current

	default:
		return nil, fmt.Errorf("unknown limit type: %s", req.LimitType)
	}

	allowed := remaining > 0
	var message string

	if allowed {
		message = fmt.Sprintf("Лимит: %d/%d использовано, осталось: %d", current, limit, remaining)
	} else {
		message = fmt.Sprintf("❌ Лимит превышен: %d/%d. Используйте /subscribe для увеличения лимитов.", current, limit)
	}

	logger.Debug("Limit check",
		logger.Int64("tg_user_id", req.TgUserID),
		logger.String("limit_type", string(req.LimitType)),
		logger.Int("limit", limit),
		logger.Int("current", current),
		logger.Int("remaining", remaining),
		logger.Bool("allowed", allowed),
	)

	return &CheckLimitResponse{
		Allowed:   allowed,
		Limit:     limit,
		Current:   current,
		Remaining: remaining,
		Message:   message,
	}, nil
}

// RecordImageGeneration увеличивает счётчик использования generate_image для этой игры (chatID).
func (uc *CheckLimitsUseCase) RecordImageGeneration(ctx context.Context, chatID int64) error {
	if uc.sessionRepo == nil || chatID == 0 {
		return nil
	}
	gs, err := uc.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}
	if gs == nil {
		return nil
	}
	IncrementOptionalActivityUsage(gs, "image_generation")
	return uc.sessionRepo.Save(ctx, gs)
}

// CanUseFeature проверяет, может ли пользователь использовать функцию
func (uc *CheckLimitsUseCase) CanUseFeature(ctx context.Context, tgUserID int64, feature string) (bool, error) {
	sub, err := uc.subscriptionRepo.GetByTgUserID(ctx, tgUserID)
	if err != nil {
		return false, fmt.Errorf("failed to get subscription: %w", err)
	}

	return sub.CanUseFeature(feature), nil
}
