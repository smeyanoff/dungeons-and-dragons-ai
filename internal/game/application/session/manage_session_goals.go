package session

import (
	"context"
	"fmt"
	"time"

	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/pkg/logger"
)

// Repository интерфейс для работы с сессиями
type Repository interface {
	GetByChatID(ctx context.Context, chatID int64) (*session.GameSession, error)
	Save(ctx context.Context, gs *session.GameSession) error
}

// ManageSessionGoalsUseCase управляет сессионными целями
type ManageSessionGoalsUseCase struct {
	sessionRepo Repository
}

// NewManageSessionGoalsUseCase создает новый use case
func NewManageSessionGoalsUseCase(sessionRepo Repository) *ManageSessionGoalsUseCase {
	return &ManageSessionGoalsUseCase{
		sessionRepo: sessionRepo,
	}
}

// UpdateGoalProgressRequest запрос на обновление прогресса цели
type UpdateGoalProgressRequest struct {
	ChatID      int64
	GoalType    session.SessionGoalType
	IncrementBy int // На сколько увеличить прогресс
}

// UpdateGoalProgress обновляет прогресс сессионной цели
func (uc *ManageSessionGoalsUseCase) UpdateGoalProgress(ctx context.Context, req UpdateGoalProgressRequest) error {
	gs, err := uc.sessionRepo.GetByChatID(ctx, req.ChatID)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}

	if !gs.IsActive() {
		// Не обновляем цели для завершенных сессий
		return nil
	}

	updated := false
	for i := range gs.SessionGoals {
		goal := &gs.SessionGoals[i]
		if goal.Type == req.GoalType && goal.Status == session.GoalStatusActive {
			goal.CurrentValue += req.IncrementBy
			goal.UpdatedAt = time.Now()

			// Проверяем завершение цели
			if goal.CurrentValue >= goal.TargetValue {
				goal.Status = session.GoalStatusCompleted
				logger.Info("Session goal completed",
					logger.Int64("chat_id", req.ChatID),
					logger.String("goal_type", string(goal.Type)),
					logger.Int("target", goal.TargetValue),
					logger.Int("current", goal.CurrentValue),
				)
			}
			updated = true
		}
	}

	if updated {
		if err := uc.sessionRepo.Save(ctx, gs); err != nil {
			return fmt.Errorf("failed to save session goals: %w", err)
		}
	}

	return nil
}

// CheckExpiredGoals проверяет истекшие цели и отмечает их как failed
func (uc *ManageSessionGoalsUseCase) CheckExpiredGoals(ctx context.Context) error {
	// Получаем все активные сессии (в будущем можно оптимизировать запрос)
	// Пока что оставляем как заглушку - проверка будет реализована в будущем
	// через отдельный сервис или cron job
	return nil
}

// CheckSessionExpiredGoals проверяет истекшие цели для конкретной сессии
func (uc *ManageSessionGoalsUseCase) CheckSessionExpiredGoals(ctx context.Context, chatID int64) error {
	gs, err := uc.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}

	if !gs.IsActive() {
		// Не проверяем цели для завершенных сессий
		return nil
	}

	now := time.Now()
	updated := false

	for i := range gs.SessionGoals {
		goal := &gs.SessionGoals[i]
		if goal.Status == session.GoalStatusActive && goal.TimeLimit != nil && now.After(*goal.TimeLimit) {
			goal.Status = session.GoalStatusExpired
			goal.UpdatedAt = now
			updated = true

			logger.Info("Session goal expired",
				logger.Int64("chat_id", chatID),
				logger.String("goal_type", string(goal.Type)),
				logger.String("description", goal.Description),
				logger.Time("expired_at", *goal.TimeLimit),
			)
		}
	}

	if updated {
		if err := uc.sessionRepo.Save(ctx, gs); err != nil {
			return fmt.Errorf("failed to save expired goals: %w", err)
		}
	}

	return nil
}

// GetSessionGoalsResponse ответ с целями сессии
type GetSessionGoalsResponse struct {
	Goals []SessionGoalDTO
}

// SessionGoalDTO DTO для сессионной цели
type SessionGoalDTO struct {
	Type        string
	Description string
	Status      string
	Current     int
	Target      int
	TimeLimit   *time.Time
}

// GetSessionGoals получает текущие цели сессии
func (uc *ManageSessionGoalsUseCase) GetSessionGoals(ctx context.Context, chatID int64) (*GetSessionGoalsResponse, error) {
	gs, err := uc.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	response := &GetSessionGoalsResponse{
		Goals: make([]SessionGoalDTO, 0, len(gs.SessionGoals)),
	}

	for _, goal := range gs.SessionGoals {
		dto := SessionGoalDTO{
			Type:        string(goal.Type),
			Description: goal.Description,
			Status:      string(goal.Status),
			Current:     goal.CurrentValue,
			Target:      goal.TargetValue,
			TimeLimit:   goal.TimeLimit,
		}
		response.Goals = append(response.Goals, dto)
	}

	return response, nil
}