package context

import (
	"context"
	"fmt"
	"strings"

	"dungeons-and-dragons-ai/internal/game/domain/session"
)

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

	// Локации с предопределенными проверками
	if len(gs.World.Locations) > 0 {
		parts = append(parts, "\nЛокации:")
		for _, loc := range gs.World.Locations {
			parts = append(parts, fmt.Sprintf("- %s: %s", loc.Name, loc.Description))
			
			// Добавляем предопределенные проверки для локации
			predefinedChecks := loc.PredefinedChecks()
			if len(predefinedChecks) > 0 {
				parts = append(parts, fmt.Sprintf("  Предопределенные проверки в локации '%s':", loc.Name))
				for _, check := range predefinedChecks {
					abilityName := getAbilityNameForContext(check.Ability)
					hint := ""
					if check.LocationHint != "" {
						hint = fmt.Sprintf(" (%s)", check.LocationHint)
					}
					parts = append(parts, fmt.Sprintf("    • %s - %s (DC %d)%s", abilityName, check.Description, check.DC, hint))
				}
				parts = append(parts, "  ⚠️ КРИТИЧЕСКИ ВАЖНО: Предопределенные проверки можно использовать ТОЛЬКО когда игрок находится в указанном месте (см. LocationHint).")
				parts = append(parts, "  ⚠️ НЕ используй предопределенные проверки просто так - они должны срабатывать только когда игрок находится в конкретном месте локации.")
				parts = append(parts, "  ⚠️ Для предопределенных проверок игрок должен использовать команду /roll d20.")
			}
		}
	}

	return strings.Join(parts, "\n"), nil
}
