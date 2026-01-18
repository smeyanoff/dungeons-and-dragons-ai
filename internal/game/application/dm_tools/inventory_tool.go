package dm_tools

import (
	"context"
	"encoding/json"
	"fmt"

	"dungeons-and-dragons-ai/internal/game/domain/inventory"
	"dungeons-and-dragons-ai/pkg/logger"
)

// InventoryRepository интерфейс для работы с инвентарем
type InventoryRepository interface {
	GetByCharacterID(ctx context.Context, characterID uint) (*inventory.Inventory, error)
	Save(ctx context.Context, inv *inventory.Inventory) error
}

// GetInventoryTool позволяет DM получить информацию об инвентаре персонажа
type GetInventoryTool struct {
	inventoryRepo InventoryRepository
	characterID   uint
}

// NewGetInventoryTool создает новый инструмент для получения инвентаря
func NewGetInventoryTool(inventoryRepo InventoryRepository, characterID uint) *GetInventoryTool {
	return &GetInventoryTool{
		inventoryRepo: inventoryRepo,
		characterID:   characterID,
	}
}

func (t *GetInventoryTool) Name() string {
	return "get_inventory"
}

func (t *GetInventoryTool) Description() string {
	return "Получить информацию об инвентаре персонажа. Возвращает список предметов, их количество, вес и общий вес инвентаря."
}

func (t *GetInventoryTool) Parameters() json.RawMessage {
	// Этот инструмент не требует параметров
	return BuildJSONSchema(nil, nil)
}

func (t *GetInventoryTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	logger.Info("GetInventoryTool: executing",
		logger.Uint("character_id", t.characterID),
	)

	inv, err := t.inventoryRepo.GetByCharacterID(ctx, t.characterID)
	if err != nil {
		logger.Error("GetInventoryTool: failed to get inventory",
			logger.Uint("character_id", t.characterID),
			logger.ErrorField(err),
		)
		return nil, fmt.Errorf("failed to get inventory: %w", err)
	}

	items := make([]map[string]interface{}, 0, len(inv.Items))
	for _, item := range inv.Items {
		items = append(items, map[string]interface{}{
			"name":        item.Name,
			"description": item.Description,
			"weight":      item.Weight,
			"quantity":    item.Quantity,
			"type":        string(item.Type),
		})
	}

	result := map[string]interface{}{
		"total_weight": inv.GetTotalWeight(),
		"max_weight":   inventory.MaxWeight,
		"items":        items,
		"item_count":   len(inv.Items),
	}

	logger.Info("GetInventoryTool: completed successfully",
		logger.Uint("character_id", t.characterID),
		logger.Int("item_count", len(inv.Items)),
		logger.Float64("total_weight", inv.GetTotalWeight()),
	)

	return result, nil
}

// AddItemTool позволяет DM добавить предмет в инвентарь персонажа
type AddItemTool struct {
	inventoryRepo InventoryRepository
	characterID   uint
}

// NewAddItemTool создает новый инструмент для добавления предмета
func NewAddItemTool(inventoryRepo InventoryRepository, characterID uint) *AddItemTool {
	return &AddItemTool{
		inventoryRepo: inventoryRepo,
		characterID:   characterID,
	}
}

func (t *AddItemTool) Name() string {
	return "add_item_to_inventory"
}

func (t *AddItemTool) Description() string {
	return "Добавить предмет в инвентарь персонажа. Возвращает успех операции и текущее состояние инвентаря."
}

func (t *AddItemTool) Parameters() json.RawMessage {
	properties := JSONSchemaProperties{
		"name": {
			Type:        "string",
			Description: "Название предмета",
			Required:    true,
		},
		"description": {
			Type:        "string",
			Description: "Описание предмета",
			Required:    false,
		},
		"weight": {
			Type:        "number",
			Description: "Вес предмета в кг (по умолчанию определяется по типу)",
			Required:    false,
		},
		"quantity": {
			Type:        "integer",
			Description: "Количество предметов (по умолчанию 1)",
			Required:    false,
		},
		"type": {
			Type:        "string",
			Description: "Тип предмета: weapon, armor, potion, tool, consumable, misc",
			Required:    false,
			Enum:        []interface{}{"weapon", "armor", "potion", "tool", "consumable", "misc"},
		},
	}
	return BuildJSONSchema(properties, []string{"name"})
}

func (t *AddItemTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	argsJSON, _ := json.Marshal(args)
	logger.Info("AddItemTool: executing",
		logger.Uint("character_id", t.characterID),
		logger.String("arguments", string(argsJSON)),
	)

	// Парсим аргументы
	name, ok := args["name"].(string)
	if !ok || name == "" {
		logger.Warn("AddItemTool: invalid arguments - name is required",
			logger.Uint("character_id", t.characterID),
		)
		return nil, fmt.Errorf("name is required and must be a string")
	}

	description := ""
	if desc, ok := args["description"].(string); ok {
		description = desc
	}

	var weight float64
	if w, ok := args["weight"].(float64); ok {
		weight = w
	}

	quantity := 1
	if q, ok := args["quantity"].(float64); ok {
		quantity = int(q)
	}

	itemType := inventory.ItemTypeMisc
	if typeStr, ok := args["type"].(string); ok {
		switch typeStr {
		case "weapon":
			itemType = inventory.ItemTypeWeapon
		case "armor":
			itemType = inventory.ItemTypeArmor
		case "potion":
			itemType = inventory.ItemTypePotion
		case "tool":
			itemType = inventory.ItemTypeTool
		case "consumable":
			itemType = inventory.ItemTypeConsumable
		default:
			itemType = inventory.ItemTypeMisc
		}
	}

	// Определяем вес по умолчанию, если не указан
	if weight <= 0 {
		switch itemType {
		case inventory.ItemTypeWeapon:
			weight = 2.0
		case inventory.ItemTypeArmor:
			weight = 5.0
		case inventory.ItemTypePotion:
			weight = 0.5
		case inventory.ItemTypeTool:
			weight = 1.5
		case inventory.ItemTypeConsumable:
			weight = 0.3
		default:
			weight = 1.0
		}
	}

	if description == "" {
		description = fmt.Sprintf("Предмет типа %s", itemType)
	}

	// Получаем инвентарь
	inv, err := t.inventoryRepo.GetByCharacterID(ctx, t.characterID)
	if err != nil {
		logger.Error("AddItemTool: failed to get inventory",
			logger.Uint("character_id", t.characterID),
			logger.ErrorField(err),
		)
		return nil, fmt.Errorf("failed to get inventory: %w", err)
	}

	// Добавляем предмет
	if err := inv.AddItem(name, description, weight, quantity, itemType); err != nil {
		logger.Warn("AddItemTool: failed to add item",
			logger.Uint("character_id", t.characterID),
			logger.String("item_name", name),
			logger.ErrorField(err),
		)
		return map[string]interface{}{
			"success":      false,
			"error":        err.Error(),
			"total_weight": inv.GetTotalWeight(),
		}, nil
	}

	// Сохраняем инвентарь
	if err := t.inventoryRepo.Save(ctx, inv); err != nil {
		logger.Error("AddItemTool: failed to save inventory",
			logger.Uint("character_id", t.characterID),
			logger.String("item_name", name),
			logger.ErrorField(err),
		)
		return nil, fmt.Errorf("failed to save inventory: %w", err)
	}

	result := map[string]interface{}{
		"success":      true,
		"item_added":   name,
		"quantity":     quantity,
		"total_weight": inv.GetTotalWeight(),
		"max_weight":   inventory.MaxWeight,
	}

	logger.Info("AddItemTool: completed successfully",
		logger.Uint("character_id", t.characterID),
		logger.String("item_name", name),
		logger.Int("quantity", quantity),
		logger.Float64("weight", weight),
		logger.Float64("total_weight", inv.GetTotalWeight()),
	)

	return result, nil
}

// RemoveItemTool позволяет DM удалить предмет из инвентаря персонажа
type RemoveItemTool struct {
	inventoryRepo InventoryRepository
	characterID   uint
}

// NewRemoveItemTool создает новый инструмент для удаления предмета
func NewRemoveItemTool(inventoryRepo InventoryRepository, characterID uint) *RemoveItemTool {
	return &RemoveItemTool{
		inventoryRepo: inventoryRepo,
		characterID:   characterID,
	}
}

func (t *RemoveItemTool) Name() string {
	return "remove_item_from_inventory"
}

func (t *RemoveItemTool) Description() string {
	return "Удалить предмет из инвентаря персонажа. Удаляет указанное количество предметов по имени."
}

func (t *RemoveItemTool) Parameters() json.RawMessage {
	properties := JSONSchemaProperties{
		"name": {
			Type:        "string",
			Description: "Название предмета для удаления",
			Required:    true,
		},
		"quantity": {
			Type:        "integer",
			Description: "Количество предметов для удаления (по умолчанию 1)",
			Required:    false,
		},
	}
	return BuildJSONSchema(properties, []string{"name"})
}

func (t *RemoveItemTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	argsJSON, _ := json.Marshal(args)
	logger.Info("RemoveItemTool: executing",
		logger.Uint("character_id", t.characterID),
		logger.String("arguments", string(argsJSON)),
	)

	name, ok := args["name"].(string)
	if !ok || name == "" {
		logger.Warn("RemoveItemTool: invalid arguments - name is required",
			logger.Uint("character_id", t.characterID),
		)
		return nil, fmt.Errorf("name is required and must be a string")
	}

	quantity := 1
	if q, ok := args["quantity"].(float64); ok {
		quantity = int(q)
	}

	// Получаем инвентарь
	inv, err := t.inventoryRepo.GetByCharacterID(ctx, t.characterID)
	if err != nil {
		logger.Error("RemoveItemTool: failed to get inventory",
			logger.Uint("character_id", t.characterID),
			logger.ErrorField(err),
		)
		return nil, fmt.Errorf("failed to get inventory: %w", err)
	}

	// Удаляем предмет
	if err := inv.RemoveItem(name, quantity); err != nil {
		logger.Warn("RemoveItemTool: failed to remove item",
			logger.Uint("character_id", t.characterID),
			logger.String("item_name", name),
			logger.Int("quantity", quantity),
			logger.ErrorField(err),
		)
		return map[string]interface{}{
			"success":      false,
			"error":        err.Error(),
			"total_weight": inv.GetTotalWeight(),
		}, nil
	}

	// Сохраняем инвентарь
	if err := t.inventoryRepo.Save(ctx, inv); err != nil {
		logger.Error("RemoveItemTool: failed to save inventory",
			logger.Uint("character_id", t.characterID),
			logger.String("item_name", name),
			logger.ErrorField(err),
		)
		return nil, fmt.Errorf("failed to save inventory: %w", err)
	}

	result := map[string]interface{}{
		"success":      true,
		"item_removed": name,
		"quantity":     quantity,
		"total_weight": inv.GetTotalWeight(),
	}

	logger.Info("RemoveItemTool: completed successfully",
		logger.Uint("character_id", t.characterID),
		logger.String("item_name", name),
		logger.Int("quantity", quantity),
		logger.Float64("total_weight", inv.GetTotalWeight()),
	)

	return result, nil
}
