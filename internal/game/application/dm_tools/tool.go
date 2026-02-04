package dm_tools

import (
	"context"
	"encoding/json"

	"dungeons-and-dragons-ai/pkg/logger"
)

// Tool представляет функцию, которую может вызвать DM
type Tool interface {
	// Name возвращает имя инструмента (используется для вызова)
	Name() string

	// Description возвращает описание инструмента для промпта DM
	Description() string

	// Parameters возвращает схему параметров в формате JSON Schema
	Parameters() json.RawMessage

	// Execute выполняет инструмент и возвращает результат
	Execute(ctx context.Context, args map[string]interface{}) (interface{}, error)
}

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

// ToolCall представляет вызов инструмента от DM
type ToolCall struct {
	Name      string                 `json:"name"`      // Имя вызываемого инструмента
	Arguments map[string]interface{} `json:"arguments"` // Аргументы вызова
}

// ToolResult представляет результат выполнения инструмента
type ToolResult struct {
	ToolName string      `json:"tool_name"` // Имя вызванного инструмента
	Success  bool        `json:"success"`   // Успешность выполнения
	Result   interface{} `json:"result"`    // Результат выполнения (если успешно)
	Error    string      `json:"error"`     // Ошибка (если неуспешно)
}

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

// JSONSchemaParam представляет параметр в формате JSON Schema
type JSONSchemaParam struct {
	Type        string        `json:"type"`               // "string", "number", "integer", "boolean", "object", "array"
	Description string        `json:"description"`        // Описание параметра
	Required    bool          `json:"required,omitempty"` // Обязателен ли параметр
	Enum        []interface{} `json:"enum,omitempty"`     // Возможные значения (для enum)
}

// JSONSchemaProperties представляет свойства объекта в JSON Schema
type JSONSchemaProperties map[string]JSONSchemaParam

// BuildJSONSchema создает JSON Schema для параметров инструмента
func BuildJSONSchema(properties JSONSchemaProperties, required []string) json.RawMessage {
	if properties == nil {
		properties = JSONSchemaProperties{}
	}
	if required == nil {
		required = []string{}
	}
	schema := map[string]interface{}{
		"type":       "object",
		"properties": properties,
		"required":   required,
	}

	data, _ := json.Marshal(schema)
	return json.RawMessage(data)
}
