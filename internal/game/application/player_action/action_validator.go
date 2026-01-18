package player_action

import (
	"context"

	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/session"
)

// ActionValidator валидирует действия игрока перед отправкой в LLM
// Теперь выполняет только базовую валидацию (статус персонажа)
// Валидация предметов и характеристик выполняется DM через MCP-style tools
type ActionValidator struct {
	// inventoryRepo больше не используется - валидация предметов через tools
}

func NewActionValidator() *ActionValidator {
	return &ActionValidator{}
}

// ValidationResult результат валидации действия
type ValidationResult struct {
	Valid   bool
	Message string
}

// Validate проверяет возможность выполнения действия
func (v *ActionValidator) Validate(
	ctx context.Context,
	gs *session.GameSession,
	playerMessage string,
) (*ValidationResult, error) {
	player := gs.GetFirstPlayer()
	if player == nil {
		return &ValidationResult{
			Valid:   true, // Разрешаем действие, если нет персонажа (для обратной совместимости)
			Message: "",
		}, nil
	}

	char := player.Character

	// Проверяем, жив ли персонаж
	if char.Status == character.StatusDead {
		return &ValidationResult{
			Valid:   false,
			Message: "Ваш персонаж мертв. Вы не можете выполнять действия.",
		}, nil
	}

	// Базовая валидация: только проверка статуса персонажа
	// Валидация предметов и характеристик теперь выполняется DM через MCP-style tools
	// (validate_item_usage, check_stat_requirements) на основе контекста действия игрока

	return &ValidationResult{
		Valid:   true,
		Message: "",
	}, nil
}

// Валидация предметов и характеристик теперь выполняется DM через MCP-style tools
// validate_item_usage - для проверки наличия предметов в инвентаре
// check_stat_requirements - для проверки достаточности характеристик
// Эти tools вызываются DM на основе контекста действия игрока, а не строкового поиска
