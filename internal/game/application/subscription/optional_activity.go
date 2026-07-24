package subscription

import (
	"encoding/json"

	"dungeons-and-dragons-ai/internal/game/domain/session"
)

// GetOptionalActivityUsage возвращает текущее значение счётчика "необязательной" активности
// (например, "image_generation") для этой игры. Отсутствующий счётчик считается за 0.
func GetOptionalActivityUsage(gs *session.GameSession, key string) int {
	if gs == nil || len(gs.OptionalActivityUsage) == 0 {
		return 0
	}
	var counts map[string]int
	if err := json.Unmarshal(gs.OptionalActivityUsage, &counts); err != nil {
		return 0
	}
	return counts[key]
}

// IncrementOptionalActivityUsage увеличивает счётчик "необязательной" активности на 1
// и возвращает новое значение. Не сохраняет сессию — вызывающий код должен сделать Save.
func IncrementOptionalActivityUsage(gs *session.GameSession, key string) int {
	counts := map[string]int{}
	if len(gs.OptionalActivityUsage) > 0 {
		_ = json.Unmarshal(gs.OptionalActivityUsage, &counts)
	}
	counts[key]++
	updated, err := json.Marshal(counts)
	if err == nil {
		gs.OptionalActivityUsage = updated
	}
	return counts[key]
}
