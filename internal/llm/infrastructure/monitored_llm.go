package infrastructure

import (
	"context"
	"encoding/json"
	"math"
	"time"
	"unicode/utf8"

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
	var sessionIDPtr *uint
	if sessionID != 0 {
		sessionIDPtr = &sessionID
	}
	ctxWithUsage, usage := domain.WithUsage(ctx)

	// Вызываем оригинальный LLM
	response, err := m.llm.Generate(ctxWithUsage, prompt)

	duration := time.Since(startTime)

	// Логируем запрос/ответ
	logEntry := &llm_log.LLMLog{
		ChatID:     chatID,
		TgUserID:   tgUserID,
		SessionID:  sessionIDPtr,
		Model:      "GigaChat", // Можно извлечь из конфига или контекста
		Prompt:     prompt,
		Response:   response,
		DurationMs: duration.Milliseconds(),
		HasTools:   false,
	}
	applyTokensUsage(usage, prompt, response, logEntry)

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
	var sessionIDPtr *uint
	if sessionID != 0 {
		sessionIDPtr = &sessionID
	}
	ctxWithUsage, usage := domain.WithUsage(ctx)

	// Вызываем оригинальный LLM
	response, err := m.llm.GenerateWithMaxTokens(ctxWithUsage, prompt, maxTokens)

	duration := time.Since(startTime)

	// Логируем запрос/ответ
	logEntry := &llm_log.LLMLog{
		ChatID:     chatID,
		TgUserID:   tgUserID,
		SessionID:  sessionIDPtr,
		Model:      "GigaChat",
		Prompt:     prompt,
		MaxTokens:  &maxTokens,
		Response:   response,
		DurationMs: duration.Milliseconds(),
		HasTools:   false,
	}
	applyTokensUsage(usage, prompt, response, logEntry)

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
	var sessionIDPtr *uint
	if sessionID != 0 {
		sessionIDPtr = &sessionID
	}
	ctxWithUsage, usage := domain.WithUsage(ctx)

	// Вызываем оригинальный LLM
	response, err := m.llm.GenerateWithTools(ctxWithUsage, prompt, tools)

	duration := time.Since(startTime)

	// Сериализуем вызовы инструментов в JSON
	var toolsCallsJSON *string
	var responseContent string
	var hasTools bool
	var toolsCallsCount *int
	if response != nil {
		responseContent = response.Content
		count := len(response.ToolCalls)
		toolsCallsCount = &count
		if count > 0 {
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
		ChatID:          chatID,
		TgUserID:        tgUserID,
		SessionID:       sessionIDPtr,
		Model:           "GigaChat",
		Prompt:          prompt,
		Response:        responseContent,
		DurationMs:      duration.Milliseconds(),
		HasTools:        hasTools,
		ToolsCalls:      toolsCallsJSON,
		ToolsCallsCount: toolsCallsCount,
	}
	applyTokensUsage(usage, prompt, responseContent, logEntry)

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

func applyTokensUsage(usage *domain.Usage, prompt, response string, logEntry *llm_log.LLMLog) {
	tokensUsed := 0
	if usage != nil {
		tokensUsed = usage.TotalTokens
		if tokensUsed == 0 {
			tokensUsed = usage.PromptTokens + usage.CompletionTokens
		}
	}
	if tokensUsed == 0 {
		tokensUsed = estimateTokens(prompt, response)
	}
	if tokensUsed > 0 {
		logEntry.TokensUsed = &tokensUsed
	}
}

func estimateTokens(prompt, response string) int {
	textLen := utf8.RuneCountInString(prompt) + utf8.RuneCountInString(response)
	if textLen == 0 {
		return 0
	}
	const charsPerToken = 4
	return int(math.Ceil(float64(textLen) / float64(charsPerToken)))
}
