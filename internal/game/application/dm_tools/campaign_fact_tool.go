package dm_tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"dungeons-and-dragons-ai/internal/game/domain/world"
	"dungeons-and-dragons-ai/pkg/logger"
)

// CampaignFactRepository интерфейс для сохранения ключевых фактов кампании
type CampaignFactRepository interface {
	Save(ctx context.Context, fact *world.CampaignFact) error
}

// SaveCampaignFactTool позволяет DM явно зафиксировать устойчивый факт кампании
// (репутация, веха квеста, решение с долгими последствиями, отношения с NPC/фракцией).
// В отличие от пассивного анализатора ответов, это явный вызов, которым DM может
// сознательно управлять — использовать сразу, когда происходит что-то значимое.
type SaveCampaignFactTool struct {
	factRepo CampaignFactRepository
	worldID  uint
}

// NewSaveCampaignFactTool создает новый инструмент для сохранения фактов кампании
func NewSaveCampaignFactTool(factRepo CampaignFactRepository, worldID uint) *SaveCampaignFactTool {
	return &SaveCampaignFactTool{
		factRepo: factRepo,
		worldID:  worldID,
	}
}

func (t *SaveCampaignFactTool) Name() string {
	return "save_campaign_fact"
}

func (t *SaveCampaignFactTool) Description() string {
	return `Сохраняет устойчивый факт кампании, который должен помниться независимо от текущей локации игрока.

Используй ТОЛЬКО для значимых для ВСЕЙ кампании вещей:
- Устойчивое изменение репутации игрока (например, "прославился как убийца дракона в этих землях")
- Решение с долгими последствиями (например, "отпустил пленника, вместо того чтобы казнить")
- Веха главного или побочного квеста (например, "нашел первый фрагмент древней карты")
- Заметный сдвиг в отношениях с NPC или фракцией (например, "гильдия воров теперь считает игрока предателем")

НЕ используй для рутинных или локальных действий (осмотр комнаты, обычный диалог, находка мелкого предмета) —
это не факт для памяти всей кампании, а часть текущей сцены.

Параметры:
- category (обязательно): "reputation", "quest", "decision" или "relationship"
- text (обязательно): краткая формулировка факта (1 предложение, без пересказа диалога)`
}

func (t *SaveCampaignFactTool) Parameters() json.RawMessage {
	properties := JSONSchemaProperties{
		"category": {
			Type:        "string",
			Description: "Категория факта",
			Required:    true,
			Enum:        []interface{}{"reputation", "quest", "decision", "relationship"},
		},
		"text": {
			Type:        "string",
			Description: "Краткая формулировка факта (1 предложение)",
			Required:    true,
		},
	}
	return BuildJSONSchema(properties, []string{"category", "text"})
}

func (t *SaveCampaignFactTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	text := strings.TrimSpace(getStringParam(args, "text"))
	if text == "" {
		return nil, fmt.Errorf("text is required and must be a non-empty string")
	}

	category := world.CampaignFactCategory(strings.TrimSpace(getStringParam(args, "category")))
	switch category {
	case world.FactCategoryReputation, world.FactCategoryQuest, world.FactCategoryDecision, world.FactCategoryRelationship:
		// валидная категория
	default:
		category = world.FactCategoryDecision
	}

	if t.factRepo == nil {
		return map[string]interface{}{
			"success": false,
			"message": "campaign fact repository is not configured",
		}, nil
	}

	fact := &world.CampaignFact{
		WorldID:  t.worldID,
		Category: category,
		Text:     text,
	}

	if err := t.factRepo.Save(ctx, fact); err != nil {
		logger.Error("SaveCampaignFactTool: failed to save fact",
			logger.Uint("world_id", t.worldID),
			logger.ErrorField(err),
		)
		return map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		}, nil
	}

	logger.Info("SaveCampaignFactTool: fact saved",
		logger.Uint("world_id", t.worldID),
		logger.String("category", string(category)),
	)

	return map[string]interface{}{
		"success":  true,
		"category": string(category),
		"message":  "Факт кампании сохранён.",
	}, nil
}
