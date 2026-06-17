package subscription

import (
	"context"
	"fmt"

	"dungeons-and-dragons-ai/internal/game/domain/subscription"
	"dungeons-and-dragons-ai/internal/game/infrastructure/persistence"
)

type GetSubscriptionUseCase struct {
	subscriptionRepo *persistence.SubscriptionRepository
}

func NewGetSubscriptionUseCase(subscriptionRepo *persistence.SubscriptionRepository) *GetSubscriptionUseCase {
	return &GetSubscriptionUseCase{
		subscriptionRepo: subscriptionRepo,
	}
}

type GetSubscriptionRequest struct {
	TgUserID int64
}

type GetSubscriptionResponse struct {
	Subscription  *subscription.Subscription
	PlanDetails   subscription.PlanDetails
	IsActive      bool
	DaysRemaining int
	Message       string
}

// Execute получает информацию о подписке пользователя
func (uc *GetSubscriptionUseCase) Execute(ctx context.Context, req GetSubscriptionRequest) (*GetSubscriptionResponse, error) {
	sub, err := uc.subscriptionRepo.GetByTgUserID(ctx, req.TgUserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get subscription: %w", err)
	}

	planDetails := sub.GetPlanDetails()
	isActive := sub.IsActive()
	daysRemaining := sub.DaysRemaining()

	// Формируем сообщение о подписке
	var message string
	if isActive {
		if daysRemaining == -1 {
			message = fmt.Sprintf("✨ У вас активная подписка %s (бессрочная)", planDetails.Name)
		} else {
			message = fmt.Sprintf("✨ У вас активная подписка %s (осталось дней: %d)", planDetails.Name, daysRemaining)
		}
	} else {
		message = fmt.Sprintf("ℹ️ Ваша подписка %s неактивна. Используйте /subscribe для оформления подписки.", planDetails.Name)
	}

	return &GetSubscriptionResponse{
		Subscription:  sub,
		PlanDetails:   planDetails,
		IsActive:      isActive,
		DaysRemaining: daysRemaining,
		Message:       message,
	}, nil
}
