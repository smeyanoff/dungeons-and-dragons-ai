package persistence

import (
	"context"

	"dungeons-and-dragons-ai/internal/game/domain/world"
	"gorm.io/gorm"
)

type WorldRepository struct {
	db *gorm.DB
}

func NewWorldRepository(db *gorm.DB) *WorldRepository {
	return &WorldRepository{db: db}
}

func (r *WorldRepository) Save(ctx context.Context, w *world.World) error {
	return r.db.WithContext(ctx).
		Session(&gorm.Session{FullSaveAssociations: true}).
		Save(w).Error
}
