package persistence

import (
	"context"
	"errors"

	"dungeons-and-dragons-ai/internal/game/domain/combat"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
		Preload("Participants.Character.Stats").
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

// WithLockedActiveCombat читает активный бой сессии с блокировкой строки (SELECT ... FOR UPDATE)
// внутри транзакции, даёт вызывающему коду его изменить и сохраняет результат в той же транзакции.
//
// Нужен для операций, которые атомарно читают и модифицируют бой (атака, урон): без блокировки
// два параллельных запроса на один и тот же бой (например, из-за повторной доставки апдейта
// Telegram или двух одновременно работающих инстансов бота) читают одно и то же состояние,
// и один из них тихо перезаписывает результат другого ("потерянное обновление") — это
// приводило к повторному/задвоенному применению урона и необъяснимым скачкам HP.
// fn(nil) вызывается, если активного боя нет — сохранение в этом случае не выполняется.
func (r *CombatRepository) WithLockedActiveCombat(ctx context.Context, sessionID uint, fn func(*combat.Combat) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var c combat.Combat
		err := tx.
			Preload("Participants.Character").
			Preload("Participants.Character.Stats").
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("game_session_id = ? AND state = ?", sessionID, combat.CombatStateActive).
			First(&c).Error

		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fn(nil)
		}
		if err != nil {
			return err
		}

		if err := fn(&c); err != nil {
			return err
		}

		return tx.Session(&gorm.Session{FullSaveAssociations: true}).Save(&c).Error
	})
}

func (r *CombatRepository) GetByID(ctx context.Context, id uint) (*combat.Combat, error) {
	var c combat.Combat
	err := r.db.WithContext(ctx).
		Preload("Participants.Character").
		Preload("Participants.Character.Stats").
		First(&c, id).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	return &c, nil
}
