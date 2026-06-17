package persistence

import (
	"context"

	"gorm.io/gorm"

	"dungeons-and-dragons-ai/internal/game/domain/player"
)

type PlayerRepository struct {
	db *gorm.DB
}

func NewPlayerRepository(db *gorm.DB) *PlayerRepository {
	return &PlayerRepository{db: db}
}

func (r *PlayerRepository) GetByTgUserIDAndSessionID(
	ctx context.Context,
	tgUserID int64,
	sessionID uint,
) (*player.Player, error) {
	var p player.Player

	err := r.db.WithContext(ctx).
		Preload("Character").
		Preload("Character.Stats").
		Where("tg_user_id = ? AND game_session_id = ?", tgUserID, sessionID).
		First(&p).Error

	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &p, nil
}

func (r *PlayerRepository) GetByTgUserID(ctx context.Context, tgUserID int64) (*player.Player, error) {
	var p player.Player

	err := r.db.WithContext(ctx).
		Preload("Character").
		Preload("Character.Stats").
		Where("tg_user_id = ?", tgUserID).
		First(&p).Error

	if err != nil {
		return nil, err
	}

	return &p, nil
}

func (r *PlayerRepository) Save(ctx context.Context, p *player.Player) error {
	return r.db.WithContext(ctx).
		Session(&gorm.Session{FullSaveAssociations: true}).
		Save(p).Error
}
