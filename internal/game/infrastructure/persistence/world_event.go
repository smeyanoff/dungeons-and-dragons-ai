package persistence

import (
	"context"

	"gorm.io/gorm"

	"dungeons-and-dragons-ai/internal/game/domain/world"
)

type WorldEventRepository struct {
	db *gorm.DB
}

func NewWorldEventRepository(db *gorm.DB) *WorldEventRepository {
	return &WorldEventRepository{db: db}
}

func (r *WorldEventRepository) Save(ctx context.Context, e *world.WorldEvent) error {
	return r.db.WithContext(ctx).Save(e).Error
}

func (r *WorldEventRepository) GetByWorldID(ctx context.Context, worldID uint) ([]world.WorldEvent, error) {
	var events []world.WorldEvent
	err := r.db.WithContext(ctx).
		Where("world_id = ?", worldID).
		Order("created_at ASC").
		Find(&events).Error
	return events, err
}

func (r *WorldEventRepository) GetScheduledByWorldID(ctx context.Context, worldID uint) ([]world.WorldEvent, error) {
	var events []world.WorldEvent
	err := r.db.WithContext(ctx).
		Where("world_id = ? AND status = ?", worldID, world.WorldEventStatusScheduled).
		Order("scheduled_for ASC").
		Find(&events).Error
	return events, err
}

func (r *WorldEventRepository) GetActiveByWorldID(ctx context.Context, worldID uint) ([]world.WorldEvent, error) {
	var events []world.WorldEvent
	err := r.db.WithContext(ctx).
		Where("world_id = ? AND status = ?", worldID, world.WorldEventStatusActive).
		Order("activated_at ASC").
		Find(&events).Error
	return events, err
}

func (r *WorldEventRepository) GetByID(ctx context.Context, id uint) (*world.WorldEvent, error) {
	var e world.WorldEvent
	err := r.db.WithContext(ctx).First(&e, id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *WorldEventRepository) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&world.WorldEvent{}, id).Error
}

// GetByLocationID получает события для конкретной локации
func (r *WorldEventRepository) GetByLocationID(ctx context.Context, locationID uint) ([]world.WorldEvent, error) {
	var events []world.WorldEvent
	err := r.db.WithContext(ctx).
		Where("required_location_id = ?", locationID).
		Find(&events).Error
	return events, err
}
