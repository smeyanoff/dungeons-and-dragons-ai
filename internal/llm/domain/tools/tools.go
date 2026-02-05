package tools

import (
	"context"
	"encoding/json"
)

// Tool представляет функцию, которую может вызвать LLM/DM.
type Tool interface {
	// Name возвращает имя инструмента (используется для вызова)
	Name() string
	// Description возвращает описание инструмента для промпта
	Description() string
	// Parameters возвращает схему параметров в формате JSON Schema
	Parameters() json.RawMessage
	// Execute выполняет инструмент и возвращает результат
	Execute(ctx context.Context, args map[string]interface{}) (interface{}, error)
}

// ToolCall представляет вызов инструмента от LLM/DM.
type ToolCall struct {
	Name      string                 `json:"name"`      // Имя вызываемого инструмента
	Arguments map[string]interface{} `json:"arguments"` // Аргументы вызова
}

// ToolResult представляет результат выполнения инструмента.
type ToolResult struct {
	ToolName string      `json:"tool_name"` // Имя вызванного инструмента
	Success  bool        `json:"success"`   // Успешность выполнения
	Result   interface{} `json:"result"`    // Результат выполнения (если успешно)
	Error    string      `json:"error"`     // Ошибка (если неуспешно)
}

// JSONSchemaParam представляет параметр в формате JSON Schema.
type JSONSchemaParam struct {
	Type        string        `json:"type"`               // "string", "number", "integer", "boolean", "object", "array"
	Description string        `json:"description"`        // Описание параметра
	Required    bool          `json:"required,omitempty"` // Обязателен ли параметр
	Enum        []interface{} `json:"enum,omitempty"`     // Возможные значения (для enum)
}

// JSONSchemaProperties представляет свойства объекта в JSON Schema.
type JSONSchemaProperties map[string]JSONSchemaParam

// BuildJSONSchema создает JSON Schema для параметров инструмента.
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
