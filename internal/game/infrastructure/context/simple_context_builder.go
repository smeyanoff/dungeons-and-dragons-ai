package context

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/game/domain/world"
)

func truncateRunes(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	if maxRunes <= 0 || s == "" {
		return ""
	}
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	if maxRunes > len(runes) {
		maxRunes = len(runes)
	}
	return strings.TrimSpace(string(runes[:maxRunes])) + "…"
}

// getAbilityNameForContext возвращает русское название характеристики для отображения в контексте
func getAbilityNameForContext(ability string) string {
	switch ability {
	case "strength":
		return "Проверка Силы (STR)"
	case "dexterity":
		return "Проверка Ловкости (DEX)"
	case "constitution":
		return "Проверка Телосложения (CON)"
	case "intelligence":
		return "Проверка Интеллекта (INT)"
	case "wisdom":
		return "Проверка Мудрости (WIS)"
	case "charisma":
		return "Проверка Харизмы (CHA)"
	default:
		return "Проверка характеристики"
	}
}

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

	// Локации (список) — БЕЗ предопределенных проверок, чтобы не раздувать контекст и не провоцировать обрезание.
	if len(gs.World.Locations) > 0 {
		parts = append(parts, "\nЛокации (карта мира):")
		for _, loc := range gs.World.Locations {
			desc := truncateRunes(loc.Description, 24) // коротко: держим контекст компактным
			if desc != "" {
				parts = append(parts, fmt.Sprintf("- %s: %s", loc.Name, desc))
			} else {
				parts = append(parts, fmt.Sprintf("- %s", loc.Name))
			}
		}
	}

	// Текущая локация — подробности + проверки + выходы.
	// Важно: показываем предопределенные проверки ТОЛЬКО для текущей локации.
	var currentLoc *world.Location
	if gs.CurrentLocationID != nil {
		for i := range gs.World.Locations {
			if gs.World.Locations[i].ID == *gs.CurrentLocationID {
				currentLoc = &gs.World.Locations[i]
				break
			}
		}
	}
	if currentLoc == nil && len(gs.World.Locations) > 0 {
		currentLoc = &gs.World.Locations[0]
	}
	if currentLoc != nil {
		parts = append(parts, "\n--- Текущая локация ---")
		parts = append(parts, fmt.Sprintf("Локация: %s", currentLoc.Name))
		if strings.TrimSpace(currentLoc.Description) != "" {
			parts = append(parts, fmt.Sprintf("Описание: %s", truncateRunes(currentLoc.Description, 80)))
		}

		// Карта ID → название, чтобы выводить связи читаемо.
		idToName := map[uint]string{}
		for i := range gs.World.Locations {
			idToName[gs.World.Locations[i].ID] = gs.World.Locations[i].Name
		}
		if len(currentLoc.Connections) > 0 {
			parts = append(parts, "Выходы/пути:")
			for _, c := range currentLoc.Connections {
				toName := idToName[c.ToLocationID]
				if toName == "" {
					toName = fmt.Sprintf("location#%d", c.ToLocationID)
				}
				desc := truncateRunes(c.Description, 40)
				if desc != "" {
					parts = append(parts, fmt.Sprintf("- %s → %s: %s", c.Direction, toName, desc))
				} else {
					parts = append(parts, fmt.Sprintf("- %s → %s", c.Direction, toName))
				}
			}
		}

		// Предопределенные проверки удалены - система использует естественное распознавание проверок

		// Информация о сценарии локации
		if scenario := currentLoc.GetScenario(); scenario != nil {
			parts = append(parts, "")
			parts = append(parts, fmt.Sprintf("🎯 Сценарий: %s", scenario.Title))
			if scenario.Objective != "" {
				parts = append(parts, fmt.Sprintf("Цель: %s", scenario.Objective))
			}
			if scenario.Status != "not_started" {
				parts = append(parts, fmt.Sprintf("Статус: %s", scenario.Status))
			}
		}
	}

	if gs.HasPendingAbilityCheck() {
		parts = append(parts, "\n--- Ожидается проверка навыка ---")
		parts = append(parts, fmt.Sprintf("%s, DC %d", getAbilityNameForContext(gs.PendingAbilityCheckAbility), gs.PendingAbilityCheckDC))
	}

	return strings.Join(parts, "\n"), nil
}
