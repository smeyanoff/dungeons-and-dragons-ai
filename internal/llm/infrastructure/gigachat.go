package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"dungeons-and-dragons-ai/internal/game/application/dm_tools"
	"dungeons-and-dragons-ai/internal/llm/domain"
	"dungeons-and-dragons-ai/pkg/gigachat"
	"dungeons-and-dragons-ai/pkg/logger"
)

type GigachatLLM struct {
	client      *gigachat.Client
	model       string
	rateLimiter LLMRateLimiter // Rate limiter для предотвращения ддос атак
}

func NewGigachatLLM(client *gigachat.Client, model string) domain.LLM {
	// Создаем rate limiter с минимальной задержкой 2 секунды между запросами
	rateLimiter := NewSimpleLLMRateLimiter(2 * time.Second)
	return NewGigachatLLMWithRateLimiter(client, model, rateLimiter)
}

func NewGigachatLLMWithRateLimiter(client *gigachat.Client, model string, rateLimiter LLMRateLimiter) domain.LLM {
	return &GigachatLLM{
		client:      client,
		model:       model,
		rateLimiter: rateLimiter,
	}
}

func (g *GigachatLLM) Generate(ctx context.Context, prompt string) (string, error) {
	// Ожидаем rate limit перед запросом
	if g.rateLimiter != nil {
		if err := g.rateLimiter.Wait(ctx); err != nil {
			return "", fmt.Errorf("rate limiter wait failed: %w", err)
		}
	}

	resp, err := g.client.Chat(ctx, g.model, prompt)
	if err != nil {
		if isTLSVerificationError(err) {
			logger.Warn("GigaChat TLS verification failed, returning stub content",
				logger.ErrorField(err),
			)
			return tlsFallbackContent(), nil
		}
		return "", err
	}
	if resp != nil && len(resp.Choices) > 0 && resp.Choices[0].FinishReason != "" &&
		resp.Choices[0].FinishReason != "stop" {
		logger.Warn("GigaChat finish_reason indicates possible truncation",
			logger.String("finish_reason", resp.Choices[0].FinishReason),
			logger.Int("prompt_length", len(prompt)),
		)
	}
	if usage, ok := domain.UsageFromContext(ctx); ok && resp != nil && resp.Usage != nil {
		usage.PromptTokens = resp.Usage.PromptTokens
		usage.CompletionTokens = resp.Usage.CompletionTokens
		usage.TotalTokens = resp.Usage.TotalTokens
	}
	return resp.Output, nil
}

func (g *GigachatLLM) GenerateWithMaxTokens(ctx context.Context, prompt string, maxTokens int) (string, error) {
	// Ожидаем rate limit перед запросом
	if g.rateLimiter != nil {
		if err := g.rateLimiter.Wait(ctx); err != nil {
			return "", fmt.Errorf("rate limiter wait failed: %w", err)
		}
	}

	resp, err := g.client.ChatWithMaxTokens(ctx, g.model, prompt, &maxTokens)
	if err != nil {
		if isTLSVerificationError(err) {
			logger.Warn("GigaChat TLS verification failed, returning stub content",
				logger.ErrorField(err),
				logger.Int("max_tokens", maxTokens),
			)
			return tlsFallbackContent(), nil
		}
		return "", err
	}
	if resp != nil && len(resp.Choices) > 0 && resp.Choices[0].FinishReason != "" &&
		resp.Choices[0].FinishReason != "stop" {
		logger.Warn("GigaChat finish_reason indicates possible truncation",
			logger.String("finish_reason", resp.Choices[0].FinishReason),
			logger.Int("max_tokens", maxTokens),
			logger.Int("prompt_length", len(prompt)),
		)
	}
	if usage, ok := domain.UsageFromContext(ctx); ok && resp != nil && resp.Usage != nil {
		usage.PromptTokens = resp.Usage.PromptTokens
		usage.CompletionTokens = resp.Usage.CompletionTokens
		usage.TotalTokens = resp.Usage.TotalTokens
	}
	return resp.Output, nil
}

// GenerateWithTools реализует multi-step loop для вызова инструментов
// Формат: генерация → анализ → вызов функций → повторная генерация
func (g *GigachatLLM) GenerateWithTools(ctx context.Context, prompt string, tools []dm_tools.Tool) (*domain.LLMResponseWithTools, error) {
	functions := buildFunctionDefinitions(tools)
	toolsPrompt := ""

	// Ожидаем rate limit перед запросом
	if g.rateLimiter != nil {
		if err := g.rateLimiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limiter wait failed: %w", err)
		}
	}

	// Первый шаг: генерируем ответ с возможными вызовами инструментов
	resp, err := g.client.ChatWithFunctions(ctx, g.model, prompt, nil, functions)
	if err != nil {
		if isTLSVerificationError(err) {
			logger.Warn("GigaChat TLS verification failed, returning stub tools response",
				logger.ErrorField(err),
			)
			return &domain.LLMResponseWithTools{
				Content:   tlsFallbackContent(),
				ToolCalls: nil,
				Finished:  true,
			}, nil
		}
		if isGigaChatServerError(err) && len(functions) > 0 {
			// Fallback: используем старый формат с инструкциями в промпте.
			toolsPrompt = dm_tools.BuildToolsPrompt(tools)
			fallbackPrompt := prompt + toolsPrompt
			resp, err = g.client.ChatWithMaxTokens(ctx, g.model, fallbackPrompt, nil)
			if err != nil {
				if isTLSVerificationError(err) {
					logger.Warn("GigaChat TLS verification failed, returning stub tools response",
						logger.ErrorField(err),
					)
					return &domain.LLMResponseWithTools{
						Content:   tlsFallbackContent(),
						ToolCalls: nil,
						Finished:  true,
					}, nil
				}
				return nil, fmt.Errorf("failed to generate initial response: %w", err)
			}
		} else {
			return nil, fmt.Errorf("failed to generate initial response: %w", err)
		}
	}
	if usage, ok := domain.UsageFromContext(ctx); ok && resp != nil && resp.Usage != nil {
		usage.PromptTokens = resp.Usage.PromptTokens
		usage.CompletionTokens = resp.Usage.CompletionTokens
		usage.TotalTokens = resp.Usage.TotalTokens
	}

	response := resp.Output

	// Анализируем ответ и извлекаем вызовы инструментов
	toolCalls, found := extractToolCallsFromResponse(resp)
	if !found {
		var err error
		toolCalls, err = dm_tools.ExtractToolCalls(response)
		if err != nil {
			return nil, fmt.Errorf("failed to extract tool calls: %w", err)
		}
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

func isGigaChatServerError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "gigachat error status 500") ||
		strings.Contains(msg, "Internal Server Error")
}

func isTLSVerificationError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "tls: failed to verify certificate") ||
		strings.Contains(msg, "x509: certificate signed by unknown authority")
}

func tlsFallbackContent() string {
	return "LLM временно недоступен из-за ошибки TLS проверки сертификата. Используется заглушка."
}

func buildFunctionDefinitions(tools []dm_tools.Tool) []gigachat.FunctionDefinition {
	if len(tools) == 0 {
		return nil
	}

	defs := make([]gigachat.FunctionDefinition, 0, len(tools))
	for _, tool := range tools {
		defs = append(defs, gigachat.FunctionDefinition{
			Name:        tool.Name(),
			Description: tool.Description(),
			Parameters:  tool.Parameters(),
		})
	}

	return defs
}

func extractToolCallsFromResponse(resp *gigachat.ChatResponse) ([]dm_tools.ToolCall, bool) {
	if resp == nil || len(resp.Choices) == 0 {
		return nil, false
	}

	msg := resp.Choices[0].Message
	if len(msg.ToolCalls) > 0 {
		return convertFunctionCalls(msg.ToolCalls)
	}

	if msg.FunctionCall != nil {
		return convertFunctionCalls([]gigachat.FunctionCall{*msg.FunctionCall})
	}

	return nil, false
}

func convertFunctionCalls(calls []gigachat.FunctionCall) ([]dm_tools.ToolCall, bool) {
	if len(calls) == 0 {
		return nil, false
	}

	toolCalls := make([]dm_tools.ToolCall, 0, len(calls))
	for _, call := range calls {
		args, err := parseFunctionCallArgs(call.Arguments)
		if err != nil {
			continue
		}
		toolCalls = append(toolCalls, dm_tools.ToolCall{
			Name:      call.Name,
			Arguments: args,
		})
	}

	if len(toolCalls) == 0 {
		return nil, false
	}

	return toolCalls, true
}

func parseFunctionCallArgs(raw json.RawMessage) (map[string]interface{}, error) {
	if len(raw) == 0 {
		return map[string]interface{}{}, nil
	}

	var args map[string]interface{}
	if err := json.Unmarshal(raw, &args); err == nil {
		return args, nil
	}

	var argsString string
	if err := json.Unmarshal(raw, &argsString); err == nil {
		if argsString == "" {
			return map[string]interface{}{}, nil
		}
		if err := json.Unmarshal([]byte(argsString), &args); err == nil {
			return args, nil
		}
	}

	return nil, fmt.Errorf("unsupported function arguments format")
}
