package persistence

import (
	"context"

	"gorm.io/gorm"

	"dungeons-and-dragons-ai/internal/game/domain/event"
)

type GameEventRepository struct {
	db *gorm.DB
}

func NewGameEventRepository(db *gorm.DB) *GameEventRepository {
	return &GameEventRepository{db: db}
}

func (r *GameEventRepository) Save(ctx context.Context, e *event.StoryEvent) error {
	return r.db.WithContext(ctx).Create(e).Error
}

func (r *GameEventRepository) GetBySessionID(
	ctx context.Context,
	sessionID uint,
	limit int,
) ([]event.StoryEvent, error) {
	var events []event.StoryEvent

	err := r.db.WithContext(ctx).
		Where("game_session_id = ?", sessionID).
		Order("created_at DESC").
		Limit(limit).
		Find(&events).Error

	if err != nil {
		return nil, err
	}

	// Переворачиваем, чтобы получить хронологический порядок
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}

	return events, nil
}

func (r *GameEventRepository) GetAllBySessionID(
	ctx context.Context,
	sessionID uint,
) ([]event.StoryEvent, error) {
	var events []event.StoryEvent

	err := r.db.WithContext(ctx).
		Where("game_session_id = ?", sessionID).
		Order("created_at ASC").
		Find(&events).Error

	return events, err
}
