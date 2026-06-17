package dm_tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"dungeons-and-dragons-ai/internal/game/domain/inventory"
	"dungeons-and-dragons-ai/pkg/logger"
)

// ValidationRepository интерфейс для работы с данными для валидации
type ValidationRepository interface {
	GetByCharacterID(ctx context.Context, characterID uint) (*inventory.Inventory, error)
}

// ValidateItemUsageTool позволяет DM проверить наличие предмета в инвентаре персонажа
type ValidateItemUsageTool struct {
	inventoryRepo ValidationRepository
	characterID   uint
}

// NewValidateItemUsageTool создает новый инструмент для проверки наличия предмета
func NewValidateItemUsageTool(inventoryRepo ValidationRepository, characterID uint) *ValidateItemUsageTool {
	return &ValidateItemUsageTool{
		inventoryRepo: inventoryRepo,
		characterID:   characterID,
	}
}

func (t *ValidateItemUsageTool) Name() string {
	return "validate_item_usage"
}

func (t *ValidateItemUsageTool) Description() string {
	return "Проверить наличие предмета в инвентаре персонажа перед его использованием. " +
		"Используй этот инструмент ТОЛЬКО когда игрок пытается ИСПОЛЬЗОВАТЬ предмет из инвентаря (выпить зелье, надеть доспех, применить предмет и т.д.). " +
		"НЕ используй этот инструмент для действий 'взять [предмет]' или 'подобрать [предмет]' - это подбор предметов из мира, а не использование из инвентаря. " +
		"Для подбора предметов используй инструмент 'add_item_to_inventory'. " +
		"Возвращает информацию о наличии предмета и его количестве."
}

func (t *ValidateItemUsageTool) Parameters() json.RawMessage {
	return BuildJSONSchema(
		JSONSchemaProperties{
			"item_name": JSONSchemaParam{
				Type:        "string",
				Description: "Название предмета, который игрок пытается использовать (например: 'зелье лечения', 'меч', 'щит')",
				Required:    true,
			},
		},
		[]string{"item_name"},
	)
}

func (t *ValidateItemUsageTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	itemName, ok := args["item_name"].(string)
	if !ok || itemName == "" {
		return nil, fmt.Errorf("item_name is required and must be a string")
	}

	logger.Info("ValidateItemUsageTool: executing",
		logger.Uint("character_id", t.characterID),
		logger.String("item_name", itemName),
	)

	inv, err := t.inventoryRepo.GetByCharacterID(ctx, t.characterID)
	if err != nil {
		logger.Error("ValidateItemUsageTool: failed to get inventory",
			logger.Uint("character_id", t.characterID),
			logger.ErrorField(err),
		)
		return map[string]interface{}{
			"has_item": false,
			"error":    "failed to get inventory",
			"message":  "Не удалось проверить инвентарь.",
		}, nil
	}

	// Проверяем наличие предмета
	itemNameLower := strings.ToLower(itemName)
	var foundItem *inventory.InventoryItem
	for i := range inv.Items {
		if strings.Contains(strings.ToLower(inv.Items[i].Name), itemNameLower) {
			item := inv.Items[i]
			foundItem = &item
			break
		}
	}

	if foundItem == nil {
		return map[string]interface{}{
			"has_item":   false,
			"quantity":   0,
			"item_name":  itemName,
			"message":    fmt.Sprintf("У игрока нет предмета '%s' в инвентаре.", itemName),
			"suggestion": "Сообщи игроку, что у него нет этого предмета, или опиши, что он пытается использовать предмет, которого нет.",
		}, nil
	}

	return map[string]interface{}{
		"has_item":   true,
		"item_name":  foundItem.Name,
		"quantity":   foundItem.Quantity,
		"weight":     foundItem.Weight,
		"type":       string(foundItem.Type),
		"message":    fmt.Sprintf("У игрока есть предмет '%s' (количество: %d).", foundItem.Name, foundItem.Quantity),
		"suggestion": "Игрок может использовать этот предмет. Опиши результат использования.",
	}, nil
}

// CheckStatRequirementsTool позволяет DM проверить, достаточны ли характеристики персонажа для действия
type CheckStatRequirementsTool struct {
	sessionRepo SessionRepository
	chatID      int64
}

// NewCheckStatRequirementsTool создает новый инструмент для проверки характеристик
// SessionRepository интерфейс уже определен в character_tool.go
func NewCheckStatRequirementsTool(sessionRepo SessionRepository, chatID int64) *CheckStatRequirementsTool {
	return &CheckStatRequirementsTool{
		sessionRepo: sessionRepo,
		chatID:      chatID,
	}
}

func (t *CheckStatRequirementsTool) Name() string {
	return "check_stat_requirements"
}

func (t *CheckStatRequirementsTool) Description() string {
	return "Проверить, достаточны ли характеристики персонажа для выполнения действия. " +
		"Используй этот инструмент, когда игрок пытается выполнить действие, требующее физических характеристик " +
		"(поднять тяжелый предмет, перепрыгнуть препятствие, проявить ловкость и т.д.). " +
		"Возвращает информацию о характеристиках персонажа и достаточно ли их для действия."
}

func (t *CheckStatRequirementsTool) Parameters() json.RawMessage {
	return BuildJSONSchema(
		JSONSchemaProperties{
			"action_type": JSONSchemaParam{
				Type:        "string",
				Description: "Тип действия: 'strength' (сила - поднять, толкнуть, сломать), 'dexterity' (ловкость - прыжок, баланс, ловкость рук), 'constitution' (телосложение - выносливость, сопротивление), 'intelligence' (интеллект - решение задач, анализ), 'wisdom' (мудрость - восприятие, интуиция), 'charisma' (харизма - убеждение, обаяние)",
				Required:    true,
				Enum: []interface{}{
					"strength",
					"dexterity",
					"constitution",
					"intelligence",
					"wisdom",
					"charisma",
				},
			},
			"min_required": JSONSchemaParam{
				Type:        "integer",
				Description: "Минимальное значение характеристики, требуемое для действия (по умолчанию 10 для простых действий, 12 для сложных, 15 для очень сложных)",
				Required:    false,
			},
			"action_description": JSONSchemaParam{
				Type:        "string",
				Description: "Описание действия для контекста (например: 'поднять камень', 'перепрыгнуть пропасть')",
				Required:    false,
			},
		},
		[]string{"action_type"},
	)
}

func (t *CheckStatRequirementsTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	actionType, ok := args["action_type"].(string)
	if !ok || actionType == "" {
		return nil, fmt.Errorf("action_type is required and must be a string")
	}

	minRequired := 10 // По умолчанию
	if minReq, ok := args["min_required"].(float64); ok {
		minRequired = int(minReq)
	}

	actionDesc := ""
	if desc, ok := args["action_description"].(string); ok {
		actionDesc = desc
	}

	logger.Info("CheckStatRequirementsTool: executing",
		logger.Int64("chat_id", t.chatID),
		logger.String("action_type", actionType),
		logger.Int("min_required", minRequired),
	)

	gs, err := t.sessionRepo.GetByChatID(ctx, t.chatID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	if gs == nil {
		return map[string]interface{}{
			"has_requirement": false,
			"error":           "session not found",
			"message":         "Сессия игры не найдена.",
		}, nil
	}

	player := gs.GetFirstPlayer()
	if player == nil {
		return map[string]interface{}{
			"has_requirement": false,
			"error":           "player not found",
			"message":         "Персонаж не найден.",
		}, nil
	}

	stats := player.Character.Stats
	var statValue int
	var statName string

	switch actionType {
	case "strength":
		statValue = stats.Strength
		statName = "Сила"
	case "dexterity":
		statValue = stats.Dexterity
		statName = "Ловкость"
	case "constitution":
		statValue = stats.Constitution
		statName = "Телосложение"
	case "intelligence":
		statValue = stats.Intelligence
		statName = "Интеллект"
	case "wisdom":
		statValue = stats.Wisdom
		statName = "Мудрость"
	case "charisma":
		statValue = stats.Charisma
		statName = "Харизма"
	default:
		return nil, fmt.Errorf("invalid action_type: %s", actionType)
	}

	hasRequirement := statValue >= minRequired
	diff := statValue - minRequired

	var message string
	var suggestion string

	if hasRequirement {
		message = fmt.Sprintf("Характеристика %s (%d) достаточна для действия (требуется: %d).", statName, statValue, minRequired)
		if diff >= 3 {
			suggestion = "Игрок легко выполняет действие. Опиши успешное выполнение."
		} else {
			suggestion = "Игрок выполняет действие, но с некоторым усилием. Опиши выполнение с небольшими трудностями."
		}
	} else {
		message = fmt.Sprintf("Характеристика %s (%d) недостаточна для действия (требуется: %d).", statName, statValue, minRequired)
		suggestion = fmt.Sprintf("Игрок не может выполнить действие из-за недостаточной %s. Опиши неудачу или альтернативный результат.", statName)
	}

	return map[string]interface{}{
		"has_requirement":    hasRequirement,
		"stat_name":          statName,
		"stat_value":         statValue,
		"min_required":       minRequired,
		"difference":         diff,
		"action_type":        actionType,
		"action_description": actionDesc,
		"message":            message,
		"suggestion":         suggestion,
	}, nil
}
