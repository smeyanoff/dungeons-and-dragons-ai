package persistence

import (
	"context"

	"gorm.io/gorm"

	"dungeons-and-dragons-ai/internal/game/domain/world"
)

type CampaignFactRepository struct {
	db *gorm.DB
}

func NewCampaignFactRepository(db *gorm.DB) *CampaignFactRepository {
	return &CampaignFactRepository{db: db}
}

func (r *CampaignFactRepository) Save(ctx context.Context, fact *world.CampaignFact) error {
	return r.db.WithContext(ctx).Create(fact).Error
}

// GetByWorldID возвращает последние факты кампании (от новых к старым), не более limit штук.
func (r *CampaignFactRepository) GetByWorldID(ctx context.Context, worldID uint, limit int) ([]world.CampaignFact, error) {
	var facts []world.CampaignFact
	err := r.db.WithContext(ctx).
		Where("world_id = ?", worldID).
		Order("created_at DESC").
		Limit(limit).
		Find(&facts).Error
	return facts, err
}
