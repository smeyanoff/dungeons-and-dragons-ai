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
	return "Получить информацию об инвентаре персонажа. Возвращает список предметов, их количество, вес, " +
		"общий вес инвентаря и признак equipped - экипирован ли предмет (надет как броня или используется как оружие) прямо сейчас."
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
			"equipped":    item.Equipped,
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
	return "Добавить предмет в инвентарь персонажа. " +
		"Используй этот инструмент, когда игрок ПОДБИРАЕТ предмет из мира (действия: 'взять [предмет]', 'подобрать [предмет]', 'поднять [предмет]'). " +
		"НЕ используй этот инструмент для использования предметов из инвентаря - для этого используй 'validate_item_usage' и 'remove_item_from_inventory'. " +
		"Если добавляешь зелье или другой предмет, восстанавливающий здоровье при использовании, ОБЯЗАТЕЛЬНО укажи healing_amount (сколько HP восстанавливает ОДНА единица предмета) - иначе при использовании предмет не будет лечить. " +
		"Возвращает успех операции и текущее состояние инвентаря."
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
		"healing_amount": {
			Type:        "integer",
			Description: "Сколько HP восстанавливает ОДНА единица предмета при использовании (0 или не указано - предмет не лечит)",
			Required:    false,
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

	var healingAmount int
	if h, ok := args["healing_amount"].(float64); ok && h > 0 {
		healingAmount = int(h)
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
	if err := inv.AddItem(name, description, weight, quantity, itemType, healingAmount); err != nil {
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
	sessionRepo   SessionRepository
	characterID   uint
	chatID        int64
}

// NewRemoveItemTool создает новый инструмент для удаления предмета.
// sessionRepo используется для применения эффекта предмета (например, лечения) при его использовании;
// может быть nil - тогда эффекты предметов просто не применяются (предмет удаляется без последствий).
func NewRemoveItemTool(inventoryRepo InventoryRepository, sessionRepo SessionRepository, characterID uint, chatID int64) *RemoveItemTool {
	return &RemoveItemTool{
		inventoryRepo: inventoryRepo,
		sessionRepo:   sessionRepo,
		characterID:   characterID,
		chatID:        chatID,
	}
}

func (t *RemoveItemTool) Name() string {
	return "remove_item_from_inventory"
}

func (t *RemoveItemTool) Description() string {
	return "Удалить предмет из инвентаря персонажа - используй, когда игрок ИСПОЛЬЗУЕТ/ПОТРЕБЛЯЕТ предмет (пьет зелье, ест еду, использует расходник). " +
		"Удаляет указанное количество предметов по имени. Если у предмета указан healing_amount (например, лечебное зелье), " +
		"здоровье персонажа автоматически восстанавливается на healing_amount * количество - отдельно вызывать лечение не нужно."
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
	removedItem, err := inv.RemoveItem(name, quantity)
	if err != nil {
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

	// Если предмет лечит (healing_amount > 0) - применяем эффект к персонажу
	if removedItem.HealingAmount > 0 && t.sessionRepo != nil {
		healed, newHP, maxHP, healErr := t.applyHealing(ctx, removedItem.HealingAmount*quantity)
		if healErr != nil {
			logger.Error("RemoveItemTool: failed to apply healing",
				logger.Uint("character_id", t.characterID),
				logger.String("item_name", name),
				logger.ErrorField(healErr),
			)
		} else if healed {
			result["healed"] = true
			result["healing_amount"] = removedItem.HealingAmount * quantity
			result["hp"] = newHP
			result["max_hp"] = maxHP
		}
	}

	logger.Info("RemoveItemTool: completed successfully",
		logger.Uint("character_id", t.characterID),
		logger.String("item_name", name),
		logger.Int("quantity", quantity),
		logger.Float64("total_weight", inv.GetTotalWeight()),
	)

	return result, nil
}

// applyHealing восстанавливает HP персонажа игрока на amount и сохраняет сессию.
// Возвращает healed=false (без ошибки), если сессию/игрока найти не удалось - вызывающий код
// в этом случае просто не проставляет поля лечения в результат тула.
func (t *RemoveItemTool) applyHealing(ctx context.Context, amount int) (healed bool, newHP int, maxHP int, err error) {
	gs, err := t.sessionRepo.GetByChatID(ctx, t.chatID)
	if err != nil {
		return false, 0, 0, fmt.Errorf("failed to get session: %w", err)
	}
	if gs == nil {
		return false, 0, 0, nil
	}

	player := gs.GetFirstPlayer()
	if player == nil {
		return false, 0, 0, nil
	}

	if err := player.Character.Heal(amount); err != nil {
		// Персонаж мертв или иное ожидаемое состояние - не считаем это ошибкой выполнения тула
		return false, 0, 0, nil
	}

	if err := t.sessionRepo.Save(ctx, gs); err != nil {
		return false, 0, 0, fmt.Errorf("failed to save session: %w", err)
	}

	return true, player.Character.HP, player.Character.MaxHP, nil
}

// EquipItemTool позволяет DM пометить предмет как надетый/взятый в руки персонажем
type EquipItemTool struct {
	inventoryRepo InventoryRepository
	characterID   uint
}

// NewEquipItemTool создает новый инструмент для экипировки предмета
func NewEquipItemTool(inventoryRepo InventoryRepository, characterID uint) *EquipItemTool {
	return &EquipItemTool{
		inventoryRepo: inventoryRepo,
		characterID:   characterID,
	}
}

func (t *EquipItemTool) Name() string {
	return "equip_item"
}

func (t *EquipItemTool) Description() string {
	return "Пометить предмет из инвентаря как экипированный (надетый доспех или оружие в руках персонажа). " +
		"Используй, когда игрок ЯВНО надевает броню или берет оружие в руки для использования (не при обычном подборе предмета). " +
		"Экипировать можно только оружие и броню - на каждый тип по одному предмету одновременно: " +
		"если уже что-то экипировано в этом слоте, оно автоматически снимается."
}

func (t *EquipItemTool) Parameters() json.RawMessage {
	properties := JSONSchemaProperties{
		"name": {
			Type:        "string",
			Description: "Название предмета, который нужно экипировать (должен уже быть в инвентаре)",
			Required:    true,
		},
	}
	return BuildJSONSchema(properties, []string{"name"})
}

func (t *EquipItemTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	name, ok := args["name"].(string)
	if !ok || name == "" {
		return nil, fmt.Errorf("name is required and must be a string")
	}

	inv, err := t.inventoryRepo.GetByCharacterID(ctx, t.characterID)
	if err != nil {
		logger.Error("EquipItemTool: failed to get inventory",
			logger.Uint("character_id", t.characterID),
			logger.ErrorField(err),
		)
		return nil, fmt.Errorf("failed to get inventory: %w", err)
	}

	equipped, err := inv.EquipItem(name)
	if err != nil {
		logger.Warn("EquipItemTool: failed to equip item",
			logger.Uint("character_id", t.characterID),
			logger.String("item_name", name),
			logger.ErrorField(err),
		)
		return map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		}, nil
	}

	if err := t.inventoryRepo.Save(ctx, inv); err != nil {
		return nil, fmt.Errorf("failed to save inventory: %w", err)
	}

	logger.Info("EquipItemTool: completed successfully",
		logger.Uint("character_id", t.characterID),
		logger.String("item_name", equipped.Name),
	)

	return map[string]interface{}{
		"success":     true,
		"item_name":   equipped.Name,
		"type":        string(equipped.Type),
		"equipped_ok": true,
	}, nil
}

// UnequipItemTool позволяет DM снять экипированный предмет
type UnequipItemTool struct {
	inventoryRepo InventoryRepository
	characterID   uint
}

// NewUnequipItemTool создает новый инструмент для снятия предмета
func NewUnequipItemTool(inventoryRepo InventoryRepository, characterID uint) *UnequipItemTool {
	return &UnequipItemTool{
		inventoryRepo: inventoryRepo,
		characterID:   characterID,
	}
}

func (t *UnequipItemTool) Name() string {
	return "unequip_item"
}

func (t *UnequipItemTool) Description() string {
	return "Снять экипированный предмет (доспех или оружие) с персонажа, оставив его в инвентаре. " +
		"Используй, когда игрок явно снимает броню или убирает оружие."
}

func (t *UnequipItemTool) Parameters() json.RawMessage {
	properties := JSONSchemaProperties{
		"name": {
			Type:        "string",
			Description: "Название предмета, который нужно снять",
			Required:    true,
		},
	}
	return BuildJSONSchema(properties, []string{"name"})
}

func (t *UnequipItemTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	name, ok := args["name"].(string)
	if !ok || name == "" {
		return nil, fmt.Errorf("name is required and must be a string")
	}

	inv, err := t.inventoryRepo.GetByCharacterID(ctx, t.characterID)
	if err != nil {
		logger.Error("UnequipItemTool: failed to get inventory",
			logger.Uint("character_id", t.characterID),
			logger.ErrorField(err),
		)
		return nil, fmt.Errorf("failed to get inventory: %w", err)
	}

	unequipped, err := inv.UnequipItem(name)
	if err != nil {
		logger.Warn("UnequipItemTool: failed to unequip item",
			logger.Uint("character_id", t.characterID),
			logger.String("item_name", name),
			logger.ErrorField(err),
		)
		return map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		}, nil
	}

	if err := t.inventoryRepo.Save(ctx, inv); err != nil {
		return nil, fmt.Errorf("failed to save inventory: %w", err)
	}

	logger.Info("UnequipItemTool: completed successfully",
		logger.Uint("character_id", t.characterID),
		logger.String("item_name", unequipped.Name),
	)

	return map[string]interface{}{
		"success":   true,
		"item_name": unequipped.Name,
	}, nil
}
