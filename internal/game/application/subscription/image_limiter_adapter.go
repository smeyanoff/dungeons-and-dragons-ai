package subscription

import (
	"context"
	"fmt"
)

// SubscriptionImageLimiter адаптирует CheckLimitsUseCase к интерфейсу imageapp.ImageGenerationLimiter.
// Лимит считается "за игру" (GameSession), а не "за день" — счётчик хранится прямо в сессии
// (см. OptionalActivityUsage) и не сбрасывается по времени. Premium/Pro получают безлимит.
type SubscriptionImageLimiter struct {
	checkLimitsUC *CheckLimitsUseCase
}

// NewSubscriptionImageLimiter создает новый адаптер для проверки лимитов изображений через подписки
func NewSubscriptionImageLimiter(checkLimitsUC *CheckLimitsUseCase) *SubscriptionImageLimiter {
	return &SubscriptionImageLimiter{
		checkLimitsUC: checkLimitsUC,
	}
}

// CheckLimit проверяет, можно ли генерировать изображение на основе подписки
func (l *SubscriptionImageLimiter) CheckLimit(ctx context.Context, chatID int64, userID int64) (bool, error) {
	if l.checkLimitsUC == nil {
		return true, nil
	}

	resp, err := l.checkLimitsUC.Execute(ctx, CheckLimitRequest{
		TgUserID:  userID,
		ChatID:    chatID,
		LimitType: LimitTypeImagesPerGame,
	})
	if err != nil {
		return false, fmt.Errorf("failed to check image limit: %w", err)
	}

	return resp.Allowed, nil
}

// RecordGeneration записывает факт генерации изображения в счётчик текущей игры
func (l *SubscriptionImageLimiter) RecordGeneration(ctx context.Context, chatID int64, userID int64) error {
	if l.checkLimitsUC == nil {
		return nil
	}
	return l.checkLimitsUC.RecordImageGeneration(ctx, chatID)
}

// GetRemainingQuota возвращает оставшуюся квоту на эту игру (-1 = безлимит)
func (l *SubscriptionImageLimiter) GetRemainingQuota(ctx context.Context, chatID int64, userID int64) (int, error) {
	if l.checkLimitsUC == nil {
		return -1, nil
	}

	resp, err := l.checkLimitsUC.Execute(ctx, CheckLimitRequest{
		TgUserID:  userID,
		ChatID:    chatID,
		LimitType: LimitTypeImagesPerGame,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to get remaining quota: %w", err)
	}

	if resp.Limit == 0 {
		// Безлимит
		return -1, nil
	}
	if !resp.Allowed {
		return 0, nil
	}
	return resp.Remaining, nil
}
