package persistence

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"dungeons-and-dragons-ai/internal/game/domain/llm_log"
)

type LLMLogRepository struct {
	db *gorm.DB
}

func NewLLMLogRepository(db *gorm.DB) *LLMLogRepository {
	return &LLMLogRepository{db: db}
}

// Save сохраняет лог LLM запроса/ответа
func (r *LLMLogRepository) Save(ctx context.Context, log *llm_log.LLMLog) error {
	return r.db.WithContext(ctx).Create(log).Error
}

// GetByID получает лог по ID
func (r *LLMLogRepository) GetByID(ctx context.Context, id uint) (*llm_log.LLMLog, error) {
	var log llm_log.LLMLog
	err := r.db.WithContext(ctx).First(&log, id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &log, nil
}

// GetRecent получает последние N логов
func (r *LLMLogRepository) GetRecent(ctx context.Context, limit int) ([]*llm_log.LLMLog, error) {
	var logs []*llm_log.LLMLog
	err := r.db.WithContext(ctx).
		Order("created_at DESC").
		Limit(limit).
		Find(&logs).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get recent logs: %w", err)
	}
	return logs, nil
}

// GetByChatID получает логи по Chat ID
func (r *LLMLogRepository) GetByChatID(ctx context.Context, chatID int64, limit int) ([]*llm_log.LLMLog, error) {
	var logs []*llm_log.LLMLog
	err := r.db.WithContext(ctx).
		Where("chat_id = ?", chatID).
		Order("created_at DESC").
		Limit(limit).
		Find(&logs).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get logs by chat_id: %w", err)
	}
	return logs, nil
}

// GetByTgUserID получает логи по Telegram User ID
func (r *LLMLogRepository) GetByTgUserID(ctx context.Context, tgUserID int64, limit int) ([]*llm_log.LLMLog, error) {
	var logs []*llm_log.LLMLog
	err := r.db.WithContext(ctx).
		Where("tg_user_id = ?", tgUserID).
		Order("created_at DESC").
		Limit(limit).
		Find(&logs).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get logs by tg_user_id: %w", err)
	}
	return logs, nil
}

// GetByDateRange получает логи за указанный период
func (r *LLMLogRepository) GetByDateRange(ctx context.Context, from, to time.Time, limit int) ([]*llm_log.LLMLog, error) {
	var logs []*llm_log.LLMLog
	err := r.db.WithContext(ctx).
		Where("created_at >= ? AND created_at <= ?", from, to).
		Order("created_at DESC").
		Limit(limit).
		Find(&logs).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get logs by date range: %w", err)
	}
	return logs, nil
}

// CountByChatID подсчитывает количество логов для Chat ID
func (r *LLMLogRepository) CountByChatID(ctx context.Context, chatID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&llm_log.LLMLog{}).
		Where("chat_id = ?", chatID).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("failed to count logs: %w", err)
	}
	return count, nil
}

// GetWithErrors получает логи с ошибками
func (r *LLMLogRepository) GetWithErrors(ctx context.Context, limit int) ([]*llm_log.LLMLog, error) {
	var logs []*llm_log.LLMLog
	err := r.db.WithContext(ctx).
		Where("error IS NOT NULL").
		Order("created_at DESC").
		Limit(limit).
		Find(&logs).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get error logs: %w", err)
	}
	return logs, nil
}

// GetStats получает статистику использования LLM
func (r *LLMLogRepository) GetStats(ctx context.Context, from, to time.Time) (*LLMStats, error) {
	var stats LLMStats
	
	// Общее количество запросов
	err := r.db.WithContext(ctx).
		Model(&llm_log.LLMLog{}).
		Where("created_at >= ? AND created_at <= ?", from, to).
		Count(&stats.TotalRequests).Error
	if err != nil {
		return nil, fmt.Errorf("failed to count total requests: %w", err)
	}
	
	// Количество ошибок
	err = r.db.WithContext(ctx).
		Model(&llm_log.LLMLog{}).
		Where("created_at >= ? AND created_at <= ? AND error IS NOT NULL", from, to).
		Count(&stats.TotalErrors).Error
	if err != nil {
		return nil, fmt.Errorf("failed to count errors: %w", err)
	}
	
	// Среднее время выполнения
	var avgDuration float64
	err = r.db.WithContext(ctx).
		Model(&llm_log.LLMLog{}).
		Where("created_at >= ? AND created_at <= ?", from, to).
		Select("AVG(duration_ms)").
		Scan(&avgDuration).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get average duration: %w", err)
	}
	stats.AverageDurationMs = int64(avgDuration)
	
	// Общее количество использованных токенов
	var totalTokens int64
	err = r.db.WithContext(ctx).
		Model(&llm_log.LLMLog{}).
		Where("created_at >= ? AND created_at <= ? AND tokens_used IS NOT NULL", from, to).
		Select("SUM(tokens_used)").
		Scan(&totalTokens).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get total tokens: %w", err)
	}
	stats.TotalTokens = totalTokens
	
	return &stats, nil
}

// LLMStats представляет статистику использования LLM
type LLMStats struct {
	TotalRequests     int64 `json:"total_requests"`
	TotalErrors       int64 `json:"total_errors"`
	AverageDurationMs int64 `json:"average_duration_ms"`
	TotalTokens       int64 `json:"total_tokens"`
}
