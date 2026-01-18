package persistence

import (
	"context"

	"dungeons-and-dragons-ai/internal/game/domain/quest"
	"gorm.io/gorm"
)

type QuestRepository struct {
	db *gorm.DB
}

func NewQuestRepository(db *gorm.DB) *QuestRepository {
	return &QuestRepository{db: db}
}

func (r *QuestRepository) GetByWorldID(ctx context.Context, worldID uint) ([]*quest.Quest, error) {
	var quests []*quest.Quest
	err := r.db.WithContext(ctx).
		Where("world_id = ?", worldID).
		Preload("Items").
		Find(&quests).Error
	return quests, err
}

func (r *QuestRepository) Save(ctx context.Context, q *quest.Quest) error {
	return r.db.WithContext(ctx).
		Session(&gorm.Session{FullSaveAssociations: true}).
		Save(q).Error
}

func (r *QuestRepository) GetByID(ctx context.Context, id uint) (*quest.Quest, error) {
	var q quest.Quest
	err := r.db.WithContext(ctx).
		Preload("Items").
		First(&q, id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &q, nil
}
