package subscription

import (
	"context"
	"fmt"

	imageapp "dungeons-and-dragons-ai/internal/game/application/image"
)

// SubscriptionImageLimiter адаптирует CheckLimitsUseCase для работы с ImageGenerationLimiter
// Это позволяет использовать систему подписок для проверки лимитов генерации изображений
type SubscriptionImageLimiter struct {
	checkLimitsUC *CheckLimitsUseCase
	fallbackLimiter imageapp.ImageGenerationLimiter // Fallback для подсчета использования
}

// NewSubscriptionImageLimiter создает новый адаптер для проверки лимитов изображений через подписки
func NewSubscriptionImageLimiter(
	checkLimitsUC *CheckLimitsUseCase,
	fallbackLimiter imageapp.ImageGenerationLimiter,
) *SubscriptionImageLimiter {
	return &SubscriptionImageLimiter{
		checkLimitsUC:   checkLimitsUC,
		fallbackLimiter: fallbackLimiter,
	}
}

// CheckLimit проверяет, можно ли генерировать изображение на основе подписки
func (l *SubscriptionImageLimiter) CheckLimit(ctx context.Context, userID int64) (bool, error) {
	if l.checkLimitsUC == nil {
		// Если use case недоступен, используем fallback
		if l.fallbackLimiter != nil {
			return l.fallbackLimiter.CheckLimit(ctx, userID)
		}
		return true, nil
	}

	// Проверяем лимит изображений через систему подписок
	req := CheckLimitRequest{
		TgUserID:  userID,
		LimitType: LimitTypeImagesPerDay,
	}

	resp, err := l.checkLimitsUC.Execute(ctx, req)
	if err != nil {
		// При ошибке используем fallback
		if l.fallbackLimiter != nil {
			return l.fallbackLimiter.CheckLimit(ctx, userID)
		}
		return false, fmt.Errorf("failed to check image limit: %w", err)
	}

	// Если лимит 0 (безлимит) или осталось доступных генераций
	return resp.Allowed, nil
}

// RecordGeneration записывает факт генерации изображения
func (l *SubscriptionImageLimiter) RecordGeneration(ctx context.Context, userID int64) error {
	// Используем fallback лимитер для записи использования (для статистики)
	if l.fallbackLimiter != nil {
		return l.fallbackLimiter.RecordGeneration(ctx, userID)
	}
	return nil
}

// GetRemainingQuota возвращает оставшуюся квоту на день
func (l *SubscriptionImageLimiter) GetRemainingQuota(ctx context.Context, userID int64) (int, error) {
	if l.checkLimitsUC == nil {
		// Если use case недоступен, используем fallback
		if l.fallbackLimiter != nil {
			return l.fallbackLimiter.GetRemainingQuota(ctx, userID)
		}
		return 0, nil
	}

	// Получаем оставшуюся квоту через систему подписок
	req := CheckLimitRequest{
		TgUserID:  userID,
		LimitType: LimitTypeImagesPerDay,
	}

	resp, err := l.checkLimitsUC.Execute(ctx, req)
	if err != nil {
		// При ошибке используем fallback
		if l.fallbackLimiter != nil {
			return l.fallbackLimiter.GetRemainingQuota(ctx, userID)
		}
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
