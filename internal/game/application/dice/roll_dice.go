package dice

import (
	"context"
	"fmt"

	"dungeons-and-dragons-ai/internal/game/domain/dice"
)

type RollDiceUseCase struct{}

func NewRollDiceUseCase() *RollDiceUseCase {
	return &RollDiceUseCase{}
}

// Execute выполняет бросок кубика
func (uc *RollDiceUseCase) Execute(ctx context.Context, expression string) (string, error) {
	if expression == "" {
		// По умолчанию бросаем d20
		expression = "d20"
	}

	result, err := dice.Roll(expression)
	if err != nil {
		return "", fmt.Errorf("invalid dice expression: %w", err)
	}

	// Формируем красивое описание результата
	var resultText string

	if len(result.Rolls) == 1 {
		resultText = fmt.Sprintf("🎲 Бросок %s: **%d**", expression, result.Total)
		if result.Modifier != 0 {
			resultText += fmt.Sprintf(" (кубик: %d, модификатор: %+d)", result.Rolls[0], result.Modifier)
		}
	} else {
		resultText = fmt.Sprintf("🎲 Бросок %s:\n", expression)
		for i, roll := range result.Rolls {
			resultText += fmt.Sprintf("  Кубик %d: %d\n", i+1, roll)
		}
		if result.Modifier != 0 {
			resultText += fmt.Sprintf("  Модификатор: %+d\n", result.Modifier)
		}
		resultText += fmt.Sprintf("  **Итого: %d**", result.Total)
	}

	return resultText, nil
}
