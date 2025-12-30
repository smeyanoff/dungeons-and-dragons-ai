package persistence

import (
	"context"

	"gorm.io/gorm"

	"dungeons-and-dragons-ai/internal/game/domain/session"
)

type GameSessionRepository struct {
	db *gorm.DB
}

func NewGameSessionRepository(db *gorm.DB) *GameSessionRepository {
	return &GameSessionRepository{db: db}
}

func (r *GameSessionRepository) GetByChatID(
	ctx context.Context,
	chatID int64,
) (*session.GameSession, error) {

	var gs session.GameSession

	err := r.db.WithContext(ctx).
		Preload("World").
		Preload("World.Locations").
		Preload("World.Locations.NPCs").
		Preload("World.Locations.Monsters").
		Preload("World.Locations.Items").
		Preload("Players").
		Where("chat_id = ?", chatID).
		First(&gs).Error

	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &gs, nil
}

func (r *GameSessionRepository) Save(
	ctx context.Context,
	gs *session.GameSession,
) error {

	return r.db.WithContext(ctx).
		Transaction(func(tx *gorm.DB) error {
			return tx.Session(&gorm.Session{FullSaveAssociations: true}).
				Save(gs).Error
		})
}
