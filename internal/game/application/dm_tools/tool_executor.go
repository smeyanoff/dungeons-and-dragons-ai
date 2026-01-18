package dm_tools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
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
		attackRoll, _ := resultMap["attack_roll"].(float64)
		ac, _ := resultMap["ac"].(float64)
		damage, _ := resultMap["damage"].(float64)
		targetHP, _ := resultMap["target_hp"].(float64)
		targetMaxHP, _ := resultMap["target_max_hp"].(float64)
		
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
	parts = append(parts, "- Можно вызывать несколько инструментов одновременно")
	parts = append(parts, "- Если инструмент вернул ошибку, учти это в своем ответе")
	
	return strings.Join(parts, "\n")
}

// CleanToolCallTags удаляет теги tool_call из текста ответа
func CleanToolCallTags(text string) string {
	// Удаляем все теги tool_call (включая многострочные)
	re := regexp.MustCompile(`(?s)<tool_call[^>]*>.*?</tool_call>`)
	cleaned := re.ReplaceAllString(text, "")
	// Удаляем лишние пробелы и переносы строк
	cleaned = strings.TrimSpace(cleaned)
	return cleaned
}
