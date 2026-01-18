package infrastructure

import (
	"context"
	"fmt"

	"dungeons-and-dragons-ai/internal/game/application/dm_tools"
	"dungeons-and-dragons-ai/internal/llm/domain"
	"dungeons-and-dragons-ai/pkg/gigachat"
)

type GigachatLLM struct {
	client *gigachat.Client
	model  string
}

func NewGigachatLLM(client *gigachat.Client, model string) domain.LLM {
	return &GigachatLLM{
		client: client,
		model:  model,
	}
}

func (g *GigachatLLM) Generate(ctx context.Context, prompt string) (string, error) {
	resp, err := g.client.Chat(ctx, g.model, prompt)
	if err != nil {
		return "", err
	}
	return resp.Output, nil
}

func (g *GigachatLLM) GenerateWithMaxTokens(ctx context.Context, prompt string, maxTokens int) (string, error) {
	resp, err := g.client.ChatWithMaxTokens(ctx, g.model, prompt, &maxTokens)
	if err != nil {
		return "", err
	}
	return resp.Output, nil
}

// GenerateWithTools реализует multi-step loop для вызова инструментов
// Формат: генерация → анализ → вызов функций → повторная генерация
func (g *GigachatLLM) GenerateWithTools(ctx context.Context, prompt string, tools []dm_tools.Tool) (*domain.LLMResponseWithTools, error) {
	// Добавляем описание инструментов в промпт
	toolsPrompt := dm_tools.BuildToolsPrompt(tools)
	enhancedPrompt := prompt + toolsPrompt
	
	// Первый шаг: генерируем ответ с возможными вызовами инструментов
	resp, err := g.client.ChatWithMaxTokens(ctx, g.model, enhancedPrompt, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to generate initial response: %w", err)
	}
	
	response := resp.Output
	
	// Анализируем ответ и извлекаем вызовы инструментов
	toolCalls, err := dm_tools.ExtractToolCalls(response)
	if err != nil {
		return nil, fmt.Errorf("failed to extract tool calls: %w", err)
	}
	
	// Если вызовов инструментов нет, возвращаем обычный ответ
	if len(toolCalls) == 0 {
		// Удаляем теги tool_call из ответа, если они есть
		cleanedResponse := dm_tools.CleanToolCallTags(response)
		return &domain.LLMResponseWithTools{
			Content:   cleanedResponse,
			ToolCalls: nil,
			Finished:  true,
		}, nil
	}
	
	// Если есть вызовы инструментов, возвращаем их для выполнения
	// Вызовы будут выполнены в HandleActionUseCase
	return &domain.LLMResponseWithTools{
		Content:   response,
		ToolCalls: toolCalls,
		Finished:  false,
	}, nil
}
