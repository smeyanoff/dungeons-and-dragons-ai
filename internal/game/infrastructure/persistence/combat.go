package persistence

import (
	"context"
	"errors"

	"dungeons-and-dragons-ai/internal/game/domain/combat"
	"gorm.io/gorm"
)

type CombatRepository struct {
	db *gorm.DB
}

func NewCombatRepository(db *gorm.DB) *CombatRepository {
	return &CombatRepository{db: db}
}

func (r *CombatRepository) GetActiveBySessionID(ctx context.Context, sessionID uint) (*combat.Combat, error) {
	var c combat.Combat
	err := r.db.WithContext(ctx).
		Preload("Participants.Character").
		Where("game_session_id = ? AND state = ?", sessionID, combat.CombatStateActive).
		First(&c).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	return &c, nil
}

func (r *CombatRepository) Save(ctx context.Context, c *combat.Combat) error {
	return r.db.WithContext(ctx).
		Session(&gorm.Session{FullSaveAssociations: true}).
		Save(c).Error
}

func (r *CombatRepository) GetByID(ctx context.Context, id uint) (*combat.Combat, error) {
	var c combat.Combat
	err := r.db.WithContext(ctx).
		Preload("Participants.Character").
		First(&c, id).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	return &c, nil
}
