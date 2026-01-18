package persistence

import (
	"context"

	"gorm.io/gorm"

	"dungeons-and-dragons-ai/internal/game/domain/inventory"
)

type InventoryRepository struct {
	db *gorm.DB
}

func NewInventoryRepository(db *gorm.DB) *InventoryRepository {
	return &InventoryRepository{db: db}
}

func (r *InventoryRepository) GetByCharacterID(
	ctx context.Context,
	characterID uint,
) (*inventory.Inventory, error) {
	var inv inventory.Inventory

	err := r.db.WithContext(ctx).
		Preload("Items").
		Where("character_id = ?", characterID).
		First(&inv).Error

	if err == gorm.ErrRecordNotFound {
		// Создаем новый инвентарь, если его нет
		inv = *inventory.NewInventory(characterID)
		if err := r.Save(ctx, &inv); err != nil {
			return nil, err
		}
		return &inv, nil
	}
	if err != nil {
		return nil, err
	}

	return &inv, nil
}

func (r *InventoryRepository) Save(ctx context.Context, inv *inventory.Inventory) error {
	return r.db.WithContext(ctx).
		Session(&gorm.Session{FullSaveAssociations: true}).
		Save(inv).Error
}
