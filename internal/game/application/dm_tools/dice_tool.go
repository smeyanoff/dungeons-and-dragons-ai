package dm_tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"dungeons-and-dragons-ai/internal/game/application/dice"
	"dungeons-and-dragons-ai/internal/game/domain/event"
	"dungeons-and-dragons-ai/pkg/logger"
)

// DiceEventRepository интерфейс для сохранения событий бросков кубиков
type DiceEventRepository interface {
	Save(ctx context.Context, e *event.StoryEvent) error
}

// RollDiceTool позволяет DM бросать кубики для определения случайных событий
// Результаты бросков сохраняются в историю событий для консистентности
type RollDiceTool struct {
	rollDiceUC *dice.RollDiceUseCase
	eventRepo  DiceEventRepository
	sessionID  uint
}

// NewRollDiceTool создает новый инструмент для бросков кубиков
func NewRollDiceTool(rollDiceUC *dice.RollDiceUseCase, eventRepo DiceEventRepository, sessionID uint) *RollDiceTool {
	return &RollDiceTool{
		rollDiceUC: rollDiceUC,
		eventRepo:  eventRepo,
		sessionID:  sessionID,
	}
}

func (t *RollDiceTool) Name() string {
	return "roll_dice"
}

func (t *RollDiceTool) Description() string {
	return `Бросить кубик для определения случайных событий или результатов. 
Используй этот инструмент, когда нужно определить:
- Случайное событие (например, погода, встреча с NPC)
- Результат действия, которое должно быть случайным (например, падение, провал попытки)
- Случайный выбор из нескольких вариантов

Параметры:
- dice_expression: выражение для броска кубика (например, "d20", "2d6+3", "d100")
  Если не указано, используется d20 по умолчанию.

Результат возвращает число (или сумму чисел) для использования в твоем ответе.`
}

func (t *RollDiceTool) Parameters() json.RawMessage {
	properties := JSONSchemaProperties{
		"dice_expression": JSONSchemaParam{
			Type:        "string",
			Description: "Выражение для броска кубика (например, 'd20', '2d6+3', 'd100'). Если не указано, используется d20.",
			Required:    false,
		},
	}
	return BuildJSONSchema(properties, []string{})
}

func (t *RollDiceTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	// Извлекаем выражение для броска
	diceExpr := "d20" // По умолчанию d20
	if expr, ok := args["dice_expression"].(string); ok && expr != "" {
		diceExpr = expr
	}

	logger.Info("RollDiceTool: executing",
		logger.Uint("session_id", t.sessionID),
		logger.String("dice_expression", diceExpr),
	)

	// Бросаем кубик
	resultText, err := t.rollDiceUC.Execute(ctx, diceExpr)
	if err != nil {
		logger.Error("RollDiceTool: failed to roll dice",
			logger.Uint("session_id", t.sessionID),
			logger.String("dice_expression", diceExpr),
			logger.ErrorField(err),
		)
		return nil, fmt.Errorf("failed to roll dice: %w", err)
	}

	// Сохраняем результат броска в историю событий для консистентности
	// Это важно, чтобы игрок мог видеть результаты бросков DM
	if t.eventRepo != nil {
		dmEvent := &event.StoryEvent{
			GameSessionID: t.sessionID,
			AuthorType:    event.AuthorTypeDM,
			Content:       fmt.Sprintf("🎲 DM бросает кубик %s: %s", diceExpr, resultText),
			CreatedAt:     time.Now(),
		}
		if err := t.eventRepo.Save(ctx, dmEvent); err != nil {
			logger.Warn("RollDiceTool: failed to save event",
				logger.Uint("session_id", t.sessionID),
				logger.ErrorField(err),
			)
			// Не прерываем выполнение при ошибке сохранения события
		}
	}

	// Возвращаем результат броска для использования DM
	// Парсим результат из текста для возврата числового значения
	result := map[string]interface{}{
		"dice_expression": diceExpr,
		"result_text":     resultText,
		"message":         fmt.Sprintf("Бросок %s выполнен. Результат сохранен в историю.", diceExpr),
	}

	logger.Info("RollDiceTool: completed successfully",
		logger.Uint("session_id", t.sessionID),
		logger.String("dice_expression", diceExpr),
	)

	return result, nil
}
