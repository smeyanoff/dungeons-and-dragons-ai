package inventory

import (
	"context"
	"fmt"
	"strings"

	"dungeons-and-dragons-ai/internal/game/domain/inventory"
	"dungeons-and-dragons-ai/internal/game/domain/session"
)

type GetInventoryUseCase struct {
	sessionRepo   session.Repository
	inventoryRepo InventoryRepository
}

type InventoryRepository interface {
	GetByCharacterID(ctx context.Context, characterID uint) (*inventory.Inventory, error)
}

func NewGetInventoryUseCase(
	sessionRepo session.Repository,
	inventoryRepo InventoryRepository,
) *GetInventoryUseCase {
	return &GetInventoryUseCase{
		sessionRepo:   sessionRepo,
		inventoryRepo: inventoryRepo,
	}
}

func (uc *GetInventoryUseCase) Execute(
	ctx context.Context,
	chatID int64,
	tgUserID int64,
) (string, error) {
	// Получаем сессию
	gs, err := uc.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("failed to get session: %w", err)
	}

	if gs == nil {
		return "Игра не начата. Используйте /newgame для начала новой игры.", nil
	}

	// Ищем игрока по TgUserID (для приватных чатов chatID == tgUserID)
	// В групповых чатах это позволит найти правильного игрока
	player := gs.FindPlayerByTgUserID(tgUserID)
	if player == nil {
		// Fallback: используем первого игрока для обратной совместимости
		player = gs.GetFirstPlayer()
		if player == nil {
			return "Персонаж не создан. Используйте /createcharacter для создания персонажа.", nil
		}
	}

	char := player.Character

	// Получаем инвентарь
	inv, err := uc.inventoryRepo.GetByCharacterID(ctx, char.ID)
	if err != nil {
		return "", fmt.Errorf("failed to get inventory: %w", err)
	}

	// Формируем текст инвентаря
	var parts []string
	parts = append(parts, fmt.Sprintf("🎒 Инвентарь персонажа %s", char.Name))
	parts = append(parts, fmt.Sprintf("Вес: %.2f / %.2f кг", inv.GetTotalWeight(), inventory.MaxWeight))
	parts = append(parts, fmt.Sprintf("Предметов: %d", inv.GetItemCount()))
	parts = append(parts, "")

	if len(inv.Items) == 0 {
		parts = append(parts, "Инвентарь пуст.")
	} else {
		parts = append(parts, "Предметы:")
		for _, item := range inv.Items {
			emoji := getItemEmoji(item.Type)
			parts = append(parts, fmt.Sprintf("%s %s x%d (%.2f кг)", emoji, item.Name, item.Quantity, item.Weight*float64(item.Quantity)))
			if item.Description != "" {
				parts = append(parts, fmt.Sprintf("   %s", item.Description))
			}
		}
	}

	return strings.Join(parts, "\n"), nil
}

func getItemEmoji(itemType inventory.ItemType) string {
	switch itemType {
	case inventory.ItemTypeWeapon:
		return "⚔️"
	case inventory.ItemTypeArmor:
		return "🛡️"
	case inventory.ItemTypePotion:
		return "🧪"
	case inventory.ItemTypeTool:
		return "🔧"
	case inventory.ItemTypeConsumable:
		return "🍖"
	default:
		return "📦"
	}
}
