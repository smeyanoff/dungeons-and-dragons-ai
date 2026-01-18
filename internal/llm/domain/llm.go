package domain

import (
	"context"
	"dungeons-and-dragons-ai/internal/game/application/dm_tools"
)

type LLM interface {
	Generate(ctx context.Context, prompt string) (string, error)
	GenerateWithMaxTokens(ctx context.Context, prompt string, maxTokens int) (string, error)
	
	// GenerateWithTools генерирует ответ с поддержкой вызова инструментов
	// Если модель поддерживает нативный function calling, использует его
	// Иначе возвращает ответ с инструкциями о вызове функций
	GenerateWithTools(ctx context.Context, prompt string, tools []dm_tools.Tool) (*LLMResponseWithTools, error)
}

// LLMResponseWithTools представляет ответ LLM с возможными вызовами инструментов
type LLMResponseWithTools struct {
	Content    string               // Текстовый ответ
	ToolCalls  []dm_tools.ToolCall  // Вызовы инструментов, которые нужно выполнить
	Finished   bool                 // Закончена ли генерация (true если не нужно вызывать tools)
}
