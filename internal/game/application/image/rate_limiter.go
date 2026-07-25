package image

import (
	"context"
)

// ImageGenerationLimiter интерфейс для ограничения генерации изображений (generate_image).
// chatID определяет конкретную игру, к которой привязан лимит (лимит "за игру", а не "за день").
type ImageGenerationLimiter interface {
	// CheckLimit проверяет, можно ли генерировать изображение
	CheckLimit(ctx context.Context, chatID int64, userID int64) (bool, error)
	// RecordGeneration записывает факт генерации изображения
	RecordGeneration(ctx context.Context, chatID int64, userID int64) error
	// GetRemainingQuota возвращает оставшуюся квоту на эту игру (-1 = безлимит)
	GetRemainingQuota(ctx context.Context, chatID int64, userID int64) (int, error)
}
