package image

import (
	"context"
	"time"
)

// ImageGenerationLimiter интерфейс для ограничения генерации изображений
type ImageGenerationLimiter interface {
	// CheckLimit проверяет, можно ли генерировать изображение
	CheckLimit(ctx context.Context, userID int64) (bool, error)
	// RecordGeneration записывает факт генерации изображения
	RecordGeneration(ctx context.Context, userID int64) error
	// GetRemainingQuota возвращает оставшуюся квоту на день
	GetRemainingQuota(ctx context.Context, userID int64) (int, error)
}

// InMemoryRateLimiter простая реализация лимитера в памяти
type InMemoryRateLimiter struct {
	dailyLimit int                   // Лимит на день (5 для Free)
	records    map[int64][]time.Time // Записи генераций по userID
}

// NewInMemoryRateLimiter создает новый InMemoryRateLimiter
func NewInMemoryRateLimiter(dailyLimit int) *InMemoryRateLimiter {
	return &InMemoryRateLimiter{
		dailyLimit: dailyLimit,
		records:    make(map[int64][]time.Time),
	}
}

// CheckLimit проверяет, можно ли генерировать изображение
func (r *InMemoryRateLimiter) CheckLimit(ctx context.Context, userID int64) (bool, error) {
	today := time.Now().Truncate(24 * time.Hour)

	// Получаем записи за сегодня
	records, exists := r.records[userID]
	if !exists {
		return true, nil // Нет записей - можно генерировать
	}

	// Подсчитываем количество генераций за сегодня
	count := 0
	for _, t := range records {
		if t.Truncate(24 * time.Hour).Equal(today) {
			count++
		}
	}

	return count < r.dailyLimit, nil
}

// RecordGeneration записывает факт генерации изображения
func (r *InMemoryRateLimiter) RecordGeneration(ctx context.Context, userID int64) error {
	now := time.Now()

	if r.records[userID] == nil {
		r.records[userID] = []time.Time{}
	}

	r.records[userID] = append(r.records[userID], now)

	// Очищаем старые записи (старше 7 дней)
	r.cleanupOldRecords(userID)

	return nil
}

// GetRemainingQuota возвращает оставшуюся квоту на день
func (r *InMemoryRateLimiter) GetRemainingQuota(ctx context.Context, userID int64) (int, error) {
	canGenerate, err := r.CheckLimit(ctx, userID)
	if err != nil {
		return 0, err
	}

	if !canGenerate {
		return 0, nil
	}

	today := time.Now().Truncate(24 * time.Hour)
	records, exists := r.records[userID]
	if !exists {
		return r.dailyLimit, nil
	}

	count := 0
	for _, t := range records {
		if t.Truncate(24 * time.Hour).Equal(today) {
			count++
		}
	}

	return r.dailyLimit - count, nil
}

// cleanupOldRecords очищает старые записи (старше 7 дней)
func (r *InMemoryRateLimiter) cleanupOldRecords(userID int64) {
	cutoff := time.Now().AddDate(0, 0, -7)
	records := r.records[userID]

	filtered := []time.Time{}
	for _, t := range records {
		if t.After(cutoff) {
			filtered = append(filtered, t)
		}
	}

	r.records[userID] = filtered
}
