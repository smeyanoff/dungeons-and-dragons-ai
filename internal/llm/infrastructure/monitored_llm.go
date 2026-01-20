package infrastructure

import (
	"context"
	"encoding/json"
	"time"

	"dungeons-and-dragons-ai/internal/game/application/dm_tools"
	"dungeons-and-dragons-ai/internal/game/domain/llm_log"
	"dungeons-and-dragons-ai/internal/llm/domain"
	"dungeons-and-dragons-ai/pkg/logger"
)

// MonitoredLLM является прокси-оберткой для LLM с логированием запросов/ответов
type MonitoredLLM struct {
	llm     domain.LLM
	logRepo LLMLogRepository
}

// LLMLogRepository интерфейс для сохранения логов
type LLMLogRepository interface {
	Save(ctx context.Context, log *llm_log.LLMLog) error
}

// NewMonitoredLLM создает новый MonitoredLLM
func NewMonitoredLLM(llm domain.LLM, logRepo LLMLogRepository) domain.LLM {
	return &MonitoredLLM{
		llm:     llm,
		logRepo: logRepo,
	}
}

// Generate генерирует ответ и логирует запрос/ответ
func (m *MonitoredLLM) Generate(ctx context.Context, prompt string) (string, error) {
	startTime := time.Now()

	// Извлекаем контекст из ctx (chatID, tgUserID, sessionID)
	chatID, _ := ctx.Value("chat_id").(int64)
	tgUserID, _ := ctx.Value("tg_user_id").(int64)
	sessionID, _ := ctx.Value("session_id").(uint)

	// Вызываем оригинальный LLM
	response, err := m.llm.Generate(ctx, prompt)

	duration := time.Since(startTime)

	// Логируем запрос/ответ
	logEntry := &llm_log.LLMLog{
		ChatID:     chatID,
		TgUserID:   tgUserID,
		SessionID:  &sessionID,
		Model:      "GigaChat", // Можно извлечь из конфига или контекста
		Prompt:     prompt,
		Response:   response,
		DurationMs: duration.Milliseconds(),
		HasTools:   false,
	}

	if err != nil {
		errStr := err.Error()
		logEntry.Error = &errStr
	} else {
		logEntry.Response = response
	}

	// Сохраняем лог асинхронно (не блокируем основной поток)
	go func() {
		logCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if saveErr := m.logRepo.Save(logCtx, logEntry); saveErr != nil {
			logger.Warn("Failed to save LLM log",
				logger.ErrorField(saveErr),
			)
		}
	}()

	return response, err
}

// GenerateWithMaxTokens генерирует ответ с ограничением токенов и логирует запрос/ответ
func (m *MonitoredLLM) GenerateWithMaxTokens(ctx context.Context, prompt string, maxTokens int) (string, error) {
	startTime := time.Now()

	// Извлекаем контекст из ctx
	chatID, _ := ctx.Value("chat_id").(int64)
	tgUserID, _ := ctx.Value("tg_user_id").(int64)
	sessionID, _ := ctx.Value("session_id").(uint)

	// Вызываем оригинальный LLM
	response, err := m.llm.GenerateWithMaxTokens(ctx, prompt, maxTokens)

	duration := time.Since(startTime)

	// Логируем запрос/ответ
	logEntry := &llm_log.LLMLog{
		ChatID:     chatID,
		TgUserID:   tgUserID,
		SessionID:  &sessionID,
		Model:      "GigaChat",
		Prompt:     prompt,
		MaxTokens:  &maxTokens,
		Response:   response,
		DurationMs: duration.Milliseconds(),
		HasTools:   false,
	}

	if err != nil {
		errStr := err.Error()
		logEntry.Error = &errStr
	} else {
		logEntry.Response = response
	}

	// Сохраняем лог асинхронно
	go func() {
		logCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if saveErr := m.logRepo.Save(logCtx, logEntry); saveErr != nil {
			logger.Warn("Failed to save LLM log",
				logger.ErrorField(saveErr),
			)
		}
	}()

	return response, err
}

// GenerateWithTools генерирует ответ с инструментами и логирует запрос/ответ
func (m *MonitoredLLM) GenerateWithTools(ctx context.Context, prompt string, tools []dm_tools.Tool) (*domain.LLMResponseWithTools, error) {
	startTime := time.Now()

	// Извлекаем контекст из ctx
	chatID, _ := ctx.Value("chat_id").(int64)
	tgUserID, _ := ctx.Value("tg_user_id").(int64)
	sessionID, _ := ctx.Value("session_id").(uint)

	// Вызываем оригинальный LLM
	response, err := m.llm.GenerateWithTools(ctx, prompt, tools)

	duration := time.Since(startTime)

	// Сериализуем вызовы инструментов в JSON
	var toolsCallsJSON *string
	var responseContent string
	var hasTools bool
	if response != nil {
		responseContent = response.Content
		if len(response.ToolCalls) > 0 {
			hasTools = true
			toolsJSON, marshalErr := json.Marshal(response.ToolCalls)
			if marshalErr == nil {
				jsonStr := string(toolsJSON)
				toolsCallsJSON = &jsonStr
			}
		}
	}

	// Логируем запрос/ответ
	logEntry := &llm_log.LLMLog{
		ChatID:     chatID,
		TgUserID:   tgUserID,
		SessionID:  &sessionID,
		Model:      "GigaChat",
		Prompt:     prompt,
		Response:   responseContent,
		DurationMs: duration.Milliseconds(),
		HasTools:   hasTools,
		ToolsCalls: toolsCallsJSON,
	}

	if err != nil {
		errStr := err.Error()
		logEntry.Error = &errStr
	}

	// Сохраняем лог асинхронно
	go func() {
		logCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if saveErr := m.logRepo.Save(logCtx, logEntry); saveErr != nil {
			logger.Warn("Failed to save LLM log",
				logger.ErrorField(saveErr),
			)
		}
	}()

	return response, err
}
