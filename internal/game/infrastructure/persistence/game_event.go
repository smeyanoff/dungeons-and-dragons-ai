package persistence

import (
	"context"
	"time"

	"gorm.io/gorm"

	"dungeons-and-dragons-ai/internal/game/domain/event"
)

type GameEventRepository struct {
	db *gorm.DB
}

func NewGameEventRepository(db *gorm.DB) *GameEventRepository {
	return &GameEventRepository{db: db}
}

// CountMessagesPerDayByTgUserID подсчитывает количество сообщений игрока за текущий день
// Сообщения - это события с AuthorType = "player" в сессиях, где есть игрок с указанным TgUserID
func (r *GameEventRepository) CountMessagesPerDayByTgUserID(
	ctx context.Context,
	tgUserID int64,
) (int, error) {
	// Начало текущего дня (00:00:00)
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	var count int64

	// Подсчитываем сообщения игрока за день
	// Соединяем события с сессиями через игроков
	err := r.db.WithContext(ctx).
		Model(&event.StoryEvent{}).
		Joins("INNER JOIN game_sessions ON game_sessions.id = story_events.game_session_id").
		Joins("INNER JOIN players ON players.game_session_id = game_sessions.id").
		Where("story_events.author_type = ? AND players.tg_user_id = ? AND story_events.created_at >= ?",
			event.AuthorTypePlayer, tgUserID, startOfDay).
		Count(&count).Error

	if err != nil {
		return 0, err
	}

	return int(count), nil
}

func (r *GameEventRepository) Save(ctx context.Context, e *event.StoryEvent) error {
	return r.db.WithContext(ctx).Create(e).Error
}

// SaveInTransaction сохраняет событие в транзакции
// Позволяет выполнить дополнительные операции в той же транзакции
func (r *GameEventRepository) SaveInTransaction(
	ctx context.Context,
	e *event.StoryEvent,
	fn func(tx interface{}) error,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Сохраняем событие в транзакции
		if err := tx.Create(e).Error; err != nil {
			return err
		}

		// Выполняем дополнительные операции, если они указаны
		if fn != nil {
			if err := fn(tx); err != nil {
				return err
			}
		}

		return nil
	})
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
