package dm_tools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"dungeons-and-dragons-ai/pkg/logger"
)

// ToolExecutor выполняет инструменты на основе анализа текстового ответа DM
type ToolExecutor struct {
	registry *ToolRegistry
}

// NewToolExecutor создает новый исполнитель инструментов
func NewToolExecutor(registry *ToolRegistry) *ToolExecutor {
	return &ToolExecutor{
		registry: registry,
	}
}

// ExtractToolCalls анализирует текст ответа DM и извлекает вызовы инструментов
// Формат: <tool_call name="tool_name">{json arguments}</tool_call>
func ExtractToolCalls(dmResponse string) ([]ToolCall, error) {
	// Регулярное выражение для поиска вызовов инструментов
	// Формат: <tool_call name="tool_name">{...json...}</tool_call>
	toolCallPattern := regexp.MustCompile(`<tool_call\s+name=["']([^"']+)["']\s*>\s*(\{.*?\})\s*</tool_call>`)
	matches := toolCallPattern.FindAllStringSubmatch(dmResponse, -1)

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

		// Парсим JSON аргументы
		var args map[string]interface{}
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			// Если не удалось распарсить JSON, пропускаем этот вызов
			continue
		}

		toolCalls = append(toolCalls, ToolCall{
			Name:      toolName,
			Arguments: args,
		})
	}

	return toolCalls, nil
}

// ExecuteToolCalls выполняет список вызовов инструментов
func (e *ToolExecutor) ExecuteToolCalls(ctx context.Context, calls []ToolCall) ([]ToolResult, error) {
	logger.Info("Executing tool calls",
		logger.Int("count", len(calls)),
	)

	results := make([]ToolResult, 0, len(calls))

	for i, call := range calls {
		argsJSON, _ := json.Marshal(call.Arguments)
		logger.Info("Executing tool call",
			logger.Int("index", i+1),
			logger.Int("total", len(calls)),
			logger.String("tool_name", call.Name),
			logger.String("arguments", string(argsJSON)),
		)

		result := e.registry.ExecuteToolCall(ctx, call)

		if result.Success {
			resultJSON, _ := json.Marshal(result.Result)
			logger.Info("Tool call executed successfully",
				logger.String("tool_name", result.ToolName),
				logger.String("result", string(resultJSON)),
			)
		} else {
			logger.Warn("Tool call failed",
				logger.String("tool_name", result.ToolName),
				logger.String("error", result.Error),
			)
		}

		results = append(results, result)
	}

	logger.Info("All tool calls executed",
		logger.Int("total", len(results)),
		logger.Int("successful", countSuccessful(results)),
		logger.Int("failed", countFailed(results)),
	)

	return results, nil
}

func countSuccessful(results []ToolResult) int {
	count := 0
	for _, r := range results {
		if r.Success {
			count++
		}
	}
	return count
}

func countFailed(results []ToolResult) int {
	count := 0
	for _, r := range results {
		if !r.Success {
			count++
		}
	}
	return count
}

// FormatToolResults форматирует результаты выполнения инструментов для передачи обратно DM
func FormatToolResults(results []ToolResult) string {
	if len(results) == 0 {
		return ""
	}

	var parts []string
	parts = append(parts, "\n--- Результаты вызова инструментов ---")

	for _, result := range results {
		if result.Success {
			// Для combat tools форматируем результаты более читаемо
			if result.ToolName == "perform_combat_attack" || result.ToolName == "apply_damage" {
				formatted := formatCombatToolResult(result)
				if formatted != "" {
					parts = append(parts, formatted)
					continue
				}
			}

			// Для остальных tools - стандартное форматирование
			resultJSON, _ := json.Marshal(result.Result)
			parts = append(parts, fmt.Sprintf("Инструмент %s выполнен успешно: %s", result.ToolName, string(resultJSON)))
		} else {
			parts = append(parts, fmt.Sprintf("Инструмент %s выполнен с ошибкой: %s", result.ToolName, result.Error))
		}
	}

	return strings.Join(parts, "\n")
}

// extractNumber безопасно извлекает число из interface{}, обрабатывая int и float64
func extractNumber(v interface{}) float64 {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case float64:
		return val
	case float32:
		return float64(val)
	default:
		return 0
	}
}

// formatCombatToolResult форматирует результат combat tool для лучшей читаемости
func formatCombatToolResult(result ToolResult) string {
	resultMap, ok := result.Result.(map[string]interface{})
	if !ok {
		return ""
	}

	var parts []string

	if result.ToolName == "perform_combat_attack" {
		// Форматируем результат атаки
		hit, _ := resultMap["hit"].(bool)
		criticalHit, _ := resultMap["critical_hit"].(bool)
		attackerName, _ := resultMap["attacker_name"].(string)
		targetName, _ := resultMap["target_name"].(string)
		attackRoll := extractNumber(resultMap["attack_roll"])
		ac := extractNumber(resultMap["ac"])
		damage := extractNumber(resultMap["damage"])
		targetHP := extractNumber(resultMap["target_hp"])
		targetMaxHP := extractNumber(resultMap["target_max_hp"])

		if criticalHit {
			parts = append(parts, "🎯 КРИТИЧЕСКИЙ УДАР!")
		}

		if hit {
			parts = append(parts, fmt.Sprintf("✅ %s атакует %s и попадает! (бросок: %.0f против AC %.0f)", attackerName, targetName, attackRoll, ac))
			parts = append(parts, fmt.Sprintf("💥 Урон: %.0f", damage))
			parts = append(parts, fmt.Sprintf("❤️ %s: %.0f/%.0f HP", targetName, targetHP, targetMaxHP))
		} else {
			parts = append(parts, fmt.Sprintf("❌ %s атакует %s, но промахивается! (бросок: %.0f против AC %.0f)", attackerName, targetName, attackRoll, ac))
		}

		if combatFinished, ok := resultMap["combat_finished"].(bool); ok && combatFinished {
			if victory, ok := resultMap["victory"].(bool); ok {
				if victory {
					parts = append(parts, "🎉 Победа! Все враги повержены!")
				} else {
					parts = append(parts, "💀 Поражение! Все игроки повержены!")
				}
			}
		}
	} else if result.ToolName == "apply_damage" {
		// Форматируем результат нанесения урона
		if message, ok := resultMap["message"].(string); ok && message != "" {
			parts = append(parts, message)
		} else {
			// Если нет сообщения, формируем сами
			targetName, _ := resultMap["target_name"].(string)
			damage, _ := resultMap["damage"].(float64)
			newHP, _ := resultMap["new_hp"].(float64)
			maxHP, _ := resultMap["max_hp"].(float64)
			isDead, _ := resultMap["is_dead"].(bool)

			parts = append(parts, fmt.Sprintf("💥 %s получил(а) %.0f урона. HP: %.0f/%.0f", targetName, damage, newHP, maxHP))
			if isDead {
				parts = append(parts, fmt.Sprintf("💀 %s повержен(а)!", targetName))
			}
		}

		if combatFinished, ok := resultMap["combat_finished"].(bool); ok && combatFinished {
			if victory, ok := resultMap["victory"].(bool); ok {
				if victory {
					parts = append(parts, "🎉 Победа! Все враги повержены!")
				} else {
					parts = append(parts, "💀 Поражение! Все игроки повержены!")
				}
			}
		}
	}

	if len(parts) > 0 {
		return strings.Join(parts, "\n")
	}
	return ""
}

// BuildToolsPrompt создает промпт с описанием доступных инструментов
// ВАЖНО: DM может вызывать несколько инструментов одновременно в одном ответе
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

		// Парсим схему параметров
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

// CleanToolCallTags удаляет теги tool_call из текста ответа и заменяет их на понятные сообщения
func CleanToolCallTags(text string) string {
	// Паттерн для поиска вызовов инструментов:
	// 1. Самозакрывающийся: <tool_call name="tool_name" param="value"/>
	// 2. С закрывающим тегом: <tool_call name="tool_name">{json}</tool_call>
	// Обрабатываем оба формата отдельно

	// Сначала обрабатываем самозакрывающиеся теги
	selfClosingPattern := regexp.MustCompile(`<tool_call\s+name=["']([^"']+)["']([^/>]*?)/>`)
	text = selfClosingPattern.ReplaceAllStringFunc(text, func(match string) string {
		matches := selfClosingPattern.FindStringSubmatch(match)
		if len(matches) < 2 {
			return ""
		}
		return formatToolCallMessage(matches[1], matches[2], "")
	})

	// Затем обрабатываем теги с закрывающим тегом
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

	// Удаляем оставшиеся теги tool_call (на случай, если формат не совпал)
	re := regexp.MustCompile(`(?s)<tool_call[^>]*>.*?</tool_call>`)
	text = re.ReplaceAllString(text, "")

	// Удаляем лишние пробелы и переносы строк
	text = strings.TrimSpace(text)
	return text
}

// formatToolCallMessage формирует понятное сообщение для вызова инструмента
func formatToolCallMessage(toolName, attributes, jsonContent string) string {
	// Пытаемся извлечь параметры из атрибутов или JSON
	var params map[string]interface{}

	// Сначала пробуем распарсить JSON, если он есть
	if strings.TrimSpace(jsonContent) != "" {
		if err := json.Unmarshal([]byte(jsonContent), &params); err == nil {
			// Успешно распарсили JSON
		}
	}

	// Если JSON не удалось распарсить, пробуем извлечь из атрибутов
	if params == nil {
		params = make(map[string]interface{})
		// Ищем атрибуты вида param="value"
		attrPattern := regexp.MustCompile(`(\w+)=["']([^"']+)["']`)
		attrMatches := attrPattern.FindAllStringSubmatch(attributes, -1)
		for _, attrMatch := range attrMatches {
			if len(attrMatch) >= 3 {
				key := attrMatch[1]
				value := attrMatch[2]
				// Пытаемся преобразовать в число, если возможно
				if intVal, err := strconv.Atoi(value); err == nil {
					params[key] = intVal
				} else {
					params[key] = value
				}
			}
		}
	}

	// Формируем понятное сообщение в зависимости от инструмента
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

	case "get_character_stats":
		return "Получаются характеристики персонажа"

	case "get_inventory":
		return "Проверяется инвентарь"

	case "get_battlefield_status":
		return "Проверяется статус поля боя"

	case "get_character_abilities":
		return "Проверяются способности персонажа"

	default:
		// Для неизвестных инструментов просто удаляем
		return ""
	}
}

// getStringParam безопасно извлекает строковый параметр
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

// getIntParam безопасно извлекает целочисленный параметр
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

// getAbilityName возвращает русское название характеристики
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
