package dm_tools

import (
	"context"
	"encoding/json"

	llmtools "dungeons-and-dragons-ai/internal/llm/domain/tools"
	"dungeons-and-dragons-ai/pkg/logger"
)

// Tool представляет функцию, которую может вызвать DM.
type Tool = llmtools.Tool

// ToolRegistry регистрирует и управляет доступными инструментами
type ToolRegistry struct {
	tools map[string]Tool
}

// NewToolRegistry создает новый реестр инструментов
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]Tool),
	}
}

// Register регистрирует инструмент
func (r *ToolRegistry) Register(tool Tool) {
	r.tools[tool.Name()] = tool
}

// Get возвращает инструмент по имени
func (r *ToolRegistry) Get(name string) (Tool, bool) {
	tool, ok := r.tools[name]
	return tool, ok
}

// GetAll возвращает все зарегистрированные инструменты
func (r *ToolRegistry) GetAll() []Tool {
	tools := make([]Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		tools = append(tools, tool)
	}
	return tools
}

// ToolCall представляет вызов инструмента от DM.
type ToolCall = llmtools.ToolCall

// ToolResult представляет результат выполнения инструмента.
type ToolResult = llmtools.ToolResult

// ExecuteToolCall выполняет вызов инструмента через реестр
func (r *ToolRegistry) ExecuteToolCall(ctx context.Context, call ToolCall) ToolResult {
	logger.Debug("Executing tool call via registry",
		logger.String("tool_name", call.Name),
	)

	tool, ok := r.Get(call.Name)
	if !ok {
		logger.Warn("Tool not found in registry",
			logger.String("tool_name", call.Name),
			logger.Int("available_tools", len(r.tools)),
		)
		return ToolResult{
			ToolName: call.Name,
			Success:  false,
			Error:    "tool not found: " + call.Name,
		}
	}

	result, err := tool.Execute(ctx, call.Arguments)
	if err != nil {
		logger.Error("Tool execution failed",
			logger.String("tool_name", call.Name),
			logger.ErrorField(err),
		)
		return ToolResult{
			ToolName: call.Name,
			Success:  false,
			Error:    err.Error(),
		}
	}

	logger.Debug("Tool execution completed",
		logger.String("tool_name", call.Name),
	)

	return ToolResult{
		ToolName: call.Name,
		Success:  true,
		Result:   result,
	}
}

// JSONSchemaParam представляет параметр в формате JSON Schema.
type JSONSchemaParam = llmtools.JSONSchemaParam

// JSONSchemaProperties представляет свойства объекта в JSON Schema.
type JSONSchemaProperties = llmtools.JSONSchemaProperties

// BuildJSONSchema создает JSON Schema для параметров инструмента
func BuildJSONSchema(properties JSONSchemaProperties, required []string) json.RawMessage {
	return llmtools.BuildJSONSchema(properties, required)
}
