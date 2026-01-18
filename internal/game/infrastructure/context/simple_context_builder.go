package context

import (
	"context"
	"fmt"
	"strings"

	"dungeons-and-dragons-ai/internal/game/domain/session"
)

type SimpleContextBuilder struct{}

func NewSimpleContextBuilder() *SimpleContextBuilder {
	return &SimpleContextBuilder{}
}

func (b *SimpleContextBuilder) BuildContext(ctx context.Context, gs *session.GameSession, playerMessage string) (string, error) {
	var parts []string

	// Мир
	parts = append(parts, fmt.Sprintf("Мир: %s", gs.World.Name))
	parts = append(parts, fmt.Sprintf("Описание: %s", gs.World.Description))

	// Календарь и время
	parts = append(parts, fmt.Sprintf("\nВремя: %s, Погода: %s", gs.World.TimeOfDay, gs.World.Weather))
	parts = append(parts, fmt.Sprintf("Календарь: День %d, Неделя %d, Месяц %d", gs.World.Day, gs.World.Week, gs.World.Month))
	if gs.World.Season != "" {
		parts = append(parts, fmt.Sprintf("Сезон: %s", gs.World.Season))
	}

	// Главный квест
	if gs.World.MainQuest != nil {
		parts = append(parts, fmt.Sprintf("\nГлавный квест: %s", gs.World.MainQuest.Title))
		parts = append(parts, fmt.Sprintf("Описание квеста: %s", gs.World.MainQuest.Description))
	}

	// Активные мировые события
	if len(gs.World.Events) > 0 {
		activeEvents := make([]string, 0)
		for _, event := range gs.World.Events {
			if event.Status == "active" {
				activeEvents = append(activeEvents, fmt.Sprintf("- %s (%s): %s", event.Name, event.Type, event.Description))
			}
		}
		if len(activeEvents) > 0 {
			parts = append(parts, "\nАктивные события в мире:")
			parts = append(parts, activeEvents...)
		}
	}

	// Локации
	if len(gs.World.Locations) > 0 {
		parts = append(parts, "\nЛокации:")
		for _, loc := range gs.World.Locations {
			parts = append(parts, fmt.Sprintf("- %s: %s", loc.Name, loc.Description))
		}
	}

	return strings.Join(parts, "\n"), nil
}
