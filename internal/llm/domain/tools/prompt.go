package tools

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ExtractToolCalls анализирует текст ответа и извлекает вызовы инструментов.
// Формат: <tool_call name="tool_name">{json arguments}</tool_call>
func ExtractToolCalls(response string) ([]ToolCall, error) {
	toolCallPattern := regexp.MustCompile(`<tool_call\s+name=["']([^"']+)["']\s*>\s*(\{.*?\})\s*</tool_call>`)
	matches := toolCallPattern.FindAllStringSubmatch(response, -1)

	if len(matches) == 0 {
		return nil, nil
	}

	toolCalls := make([]ToolCall, 0, len(matches))
	for _, match := range matches {
		if len(match) != 3 {
			continue
		}
		toolName := match[1]
		argsJSON := match[2]

		var args map[string]interface{}
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			continue
		}

		toolCalls = append(toolCalls, ToolCall{
			Name:      toolName,
			Arguments: args,
		})
	}

	return toolCalls, nil
}

// BuildToolsPrompt создает промпт с описанием доступных инструментов.
// ВАЖНО: DM может вызывать несколько инструментов одновременно в одном ответе.
func BuildToolsPrompt(tools []Tool) string {
	if len(tools) == 0 {
		return ""
	}

	var parts []string
	parts = append(parts, "\n--- Доступные инструменты ---")
	parts = append(parts, "Ты можешь вызывать следующие инструменты для работы с игровым состоянием:")
	parts = append(parts, "")

	for _, tool := range tools {
		parts = append(parts, fmt.Sprintf("### %s", tool.Name()))
		parts = append(parts, tool.Description())

		var schema map[string]interface{}
		if err := json.Unmarshal(tool.Parameters(), &schema); err == nil {
			if props, ok := schema["properties"].(map[string]interface{}); ok && len(props) > 0 {
				parts = append(parts, "Параметры:")
				for paramName, paramDef := range props {
					if paramMap, ok := paramDef.(map[string]interface{}); ok {
						paramDesc := paramMap["description"]
						required := ""
						if requiredList, ok := schema["required"].([]interface{}); ok {
							for _, req := range requiredList {
								if req == paramName {
									required = " (обязательный)"
									break
								}
							}
						}
						parts = append(parts, fmt.Sprintf("  - %s: %v%s", paramName, paramDesc, required))
					}
				}
			}
		}

		parts = append(parts, "")
		parts = append(parts, fmt.Sprintf("Формат вызова: <tool_call name=\"%s\">{...json параметры...}</tool_call>", tool.Name()))
		parts = append(parts, "")
	}

	parts = append(parts, "Важно:")
	parts = append(parts, "- Используй инструменты, когда нужно получить актуальную информацию об игровом состоянии")
	parts = append(parts, "- После вызова инструмента дождись результата и используй его в своем ответе")
	parts = append(parts, "")
	parts = append(parts, "⚠️ МНОЖЕСТВЕННЫЕ ВЫЗОВЫ:")
	parts = append(parts, "- Ты можешь вызывать НЕСКОЛЬКО инструментов ОДНОВРЕМЕННО в одном ответе")
	parts = append(parts, "- Просто добавь несколько тегов <tool_call> один за другим")
	parts = append(parts, "- Пример: если нужно проверить инвентарь И атаковать, вызови оба инструмента сразу")
	parts = append(parts, "- Все инструменты будут выполнены параллельно, и ты получишь результаты всех вызовов")
	parts = append(parts, "")
	parts = append(parts, "Пример множественного вызова:")
	parts = append(parts, `<tool_call name="get_inventory">{}</tool_call>`)
	parts = append(parts, `<tool_call name="perform_combat_attack">{...}</tool_call>`)
	parts = append(parts, "")
	parts = append(parts, "- Если инструмент вернул ошибку, учти это в своем ответе")

	return strings.Join(parts, "\n")
}

// CleanToolCallTags удаляет теги tool_call из текста ответа и заменяет их на понятные сообщения.
func CleanToolCallTags(text string) string {
	selfClosingPattern := regexp.MustCompile(`<tool_call\s+name=["']([^"']+)["']([^/>]*?)/>`)
	text = selfClosingPattern.ReplaceAllStringFunc(text, func(match string) string {
		matches := selfClosingPattern.FindStringSubmatch(match)
		if len(matches) < 2 {
			return ""
		}
		return formatToolCallMessage(matches[1], matches[2], "")
	})

	toolCallPattern := regexp.MustCompile(`(?s)<tool_call\s+name=["']([^"']+)["']([^>]*)>(.*?)</tool_call>`)
	text = toolCallPattern.ReplaceAllStringFunc(text, func(match string) string {
		matches := toolCallPattern.FindStringSubmatch(match)
		if len(matches) < 2 {
			return ""
		}
		jsonContent := ""
		if len(matches) > 3 {
			jsonContent = matches[3]
		}
		return formatToolCallMessage(matches[1], matches[2], jsonContent)
	})

	re := regexp.MustCompile(`(?s)<tool_call[^>]*>.*?</tool_call>`)
	text = re.ReplaceAllString(text, "")

	return strings.TrimSpace(text)
}

func formatToolCallMessage(toolName, attributes, jsonContent string) string {
	var params map[string]interface{}

	if strings.TrimSpace(jsonContent) != "" {
		if err := json.Unmarshal([]byte(jsonContent), &params); err == nil {
			// parsed
		}
	}

	if params == nil {
		params = make(map[string]interface{})
		attrPattern := regexp.MustCompile(`(\w+)=["']([^"']+)["']`)
		attrMatches := attrPattern.FindAllStringSubmatch(attributes, -1)
		for _, attrMatch := range attrMatches {
			if len(attrMatch) >= 3 {
				key := attrMatch[1]
				value := attrMatch[2]
				if intVal, err := strconv.Atoi(value); err == nil {
					params[key] = intVal
				} else {
					params[key] = value
				}
			}
		}
	}

	switch toolName {
	case "request_ability_check":
		ability := getStringParam(params, "ability")
		dc := getIntParam(params, "dc")
		abilityName := getAbilityName(ability)
		if dc > 0 {
			return fmt.Sprintf("Выполняется проверка %s (DC %d)", abilityName, dc)
		} else if ability != "" {
			return fmt.Sprintf("Выполняется проверка %s", abilityName)
		}
		return "Выполняется проверка характеристики"
	case "request_saving_throw":
		ability := getStringParam(params, "ability")
		dc := getIntParam(params, "dc")
		abilityName := getAbilityName(ability)
		if dc > 0 {
			return fmt.Sprintf("Выполняется спасбросок %s (DC %d)", abilityName, dc)
		} else if ability != "" {
			return fmt.Sprintf("Выполняется спасбросок %s", abilityName)
		}
		return "Выполняется спасбросок"
	case "evaluate_check":
		ability := getStringParam(params, "ability")
		abilityName := getAbilityName(ability)
		return fmt.Sprintf("Оценивается результат проверки %s", abilityName)
	case "perform_combat_attack":
		return "Выполняется боевая атака"
	case "perform_enemy_attack":
		target := getStringParam(params, "target_name")
		if target != "" {
			return fmt.Sprintf("Враг атакует %s", target)
		}
		return "Враг выполняет атаку"
	case "apply_damage":
		return "Применяется урон"
	case "get_character_stats", "get_inventory", "get_battlefield_status", "get_character_abilities":
		return ""
	default:
		return ""
	}
}

func getStringParam(params map[string]interface{}, key string) string {
	if params == nil {
		return ""
	}
	if val, ok := params[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func getIntParam(params map[string]interface{}, key string) int {
	if params == nil {
		return 0
	}
	if val, ok := params[key]; ok {
		switch v := val.(type) {
		case int:
			return v
		case float64:
			return int(v)
		case string:
			if intVal, err := strconv.Atoi(v); err == nil {
				return intVal
			}
		}
	}
	return 0
}

func getAbilityName(ability string) string {
	switch ability {
	case "strength":
		return "Силы (STR)"
	case "dexterity":
		return "Ловкости (DEX)"
	case "constitution":
		return "Телосложения (CON)"
	case "intelligence":
		return "Интеллекта (INT)"
	case "wisdom":
		return "Мудрости (WIS)"
	case "charisma":
		return "Харизмы (CHA)"
	default:
		return "характеристики"
	}
}
