package persistence

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"gorm.io/gorm"

	"dungeons-and-dragons-ai/internal/game/domain/llm_log"
)

type LLMLogRepository struct {
	db *gorm.DB
}

// LLMLogFilters фильтры для выборки логов
type LLMLogFilters struct {
	ChatID    *int64
	TgUserID  *int64
	SessionID *uint
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

// GetByFilters получает логи по набору фильтров
func (r *LLMLogRepository) GetByFilters(ctx context.Context, filters LLMLogFilters, limit int) ([]*llm_log.LLMLog, error) {
	var logs []*llm_log.LLMLog
	query := r.db.WithContext(ctx).Model(&llm_log.LLMLog{})
	if filters.ChatID != nil {
		query = query.Where("chat_id = ?", *filters.ChatID)
	}
	if filters.TgUserID != nil {
		query = query.Where("tg_user_id = ?", *filters.TgUserID)
	}
	if filters.SessionID != nil {
		query = query.Where("session_id = ?", *filters.SessionID)
	}
	err := query.Order("created_at DESC").Limit(limit).Find(&logs).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get logs by filters: %w", err)
	}
	return logs, nil
}

// GetBranches получает агрегаты по веткам запросов (сессиям)
func (r *LLMLogRepository) GetBranches(ctx context.Context, filters LLMLogFilters, limit int) ([]*LLMLogBranch, error) {
	branches := make([]*LLMLogBranch, 0) // Initialize with empty slice
	query := r.db.WithContext(ctx).Model(&llm_log.LLMLog{}).
		Where("session_id IS NOT NULL")
	if filters.ChatID != nil {
		query = query.Where("chat_id = ?", *filters.ChatID)
	}
	if filters.TgUserID != nil {
		query = query.Where("tg_user_id = ?", *filters.TgUserID)
	}

	rows, err := query.Select(`
			session_id,
			chat_id,
			tg_user_id,
			COUNT(*) AS total_requests,
			SUM(CASE WHEN error IS NOT NULL OR (status_code IS NOT NULL AND status_code >= 400) THEN 1 ELSE 0 END) AS total_errors,
			COALESCE(SUM(COALESCE(tokens_used, 0)), 0) AS total_tokens,
			COALESCE(SUM(COALESCE(tools_calls_count, 0)), 0) AS total_tool_calls,
			MIN(created_at) AS first_seen,
			MAX(created_at) AS last_seen
		`).
		Group("session_id, chat_id, tg_user_id").
		Order("last_seen DESC").
		Limit(limit).
		Rows()
	if err != nil {
		return nil, fmt.Errorf("failed to get branches: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var branch LLMLogBranch
		err := rows.Scan(
			&branch.SessionID,
			&branch.ChatID,
			&branch.TgUserID,
			&branch.TotalRequests,
			&branch.TotalErrors,
			&branch.TotalTokens,
			&branch.TotalToolCalls,
			&branch.FirstSeen,
			&branch.LastSeen,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan branch: %w", err)
		}
		branches = append(branches, &branch)
	}

	return branches, nil
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
	var avgDuration sql.NullFloat64
	err = r.db.WithContext(ctx).
		Model(&llm_log.LLMLog{}).
		Where("created_at >= ? AND created_at <= ? AND duration_ms IS NOT NULL", from, to).
		Select("AVG(duration_ms)").
		Scan(&avgDuration).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get average duration: %w", err)
	}
	if avgDuration.Valid {
		stats.AverageDurationMs = int64(avgDuration.Float64)
	} else {
		stats.AverageDurationMs = 0
	}

	// Общее количество использованных токенов
	var totalTokens int64
	query := "SELECT COALESCE(SUM(COALESCE(tokens_used, 0)), 0) FROM llm_logs WHERE created_at >= $1 AND created_at <= $2"
	err = r.db.WithContext(ctx).Raw(query, from, to).Scan(&totalTokens).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get total tokens: %w", err)
	}
	stats.TotalTokens = totalTokens

	// Общее количество вызовов инструментов
	var totalToolCalls int64
	query = "SELECT COALESCE(SUM(COALESCE(tools_calls_count, 0)), 0) FROM llm_logs WHERE created_at >= $1 AND created_at <= $2"
	err = r.db.WithContext(ctx).Raw(query, from, to).Scan(&totalToolCalls).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get total tool calls: %w", err)
	}
	stats.TotalToolCalls = totalToolCalls

	// Проблемы считаем как ошибки или HTTP статусы >= 400 (если были сохранены)
	var totalProblems int64
	err = r.db.WithContext(ctx).
		Model(&llm_log.LLMLog{}).
		Where(
			"created_at >= ? AND created_at <= ? AND (error IS NOT NULL OR (status_code IS NOT NULL AND status_code >= 400))",
			from,
			to,
		).
		Count(&totalProblems).Error
	if err != nil {
		return nil, fmt.Errorf("failed to count problems: %w", err)
	}
	stats.TotalProblems = totalProblems

	return &stats, nil
}

// LLMStats представляет статистику использования LLM
type LLMStats struct {
	TotalRequests     int64 `json:"total_requests"`
	TotalErrors       int64 `json:"total_errors"`
	AverageDurationMs int64 `json:"average_duration_ms"`
	TotalTokens       int64 `json:"total_tokens"`
	TotalToolCalls    int64 `json:"total_tool_calls"`
	TotalProblems     int64 `json:"total_problems"`
}

// LLMLogBranch агрегированные метрики по ветке (сессии)
type LLMLogBranch struct {
	SessionID      uint      `json:"session_id"`
	ChatID         int64     `json:"chat_id"`
	TgUserID       int64     `json:"tg_user_id"`
	TotalRequests  int64     `json:"total_requests"`
	TotalErrors    int64     `json:"total_errors"`
	TotalTokens    int64     `json:"total_tokens"`
	TotalToolCalls int64     `json:"total_tool_calls"`
	FirstSeen      time.Time `json:"first_seen"`
	LastSeen       time.Time `json:"last_seen"`
}
