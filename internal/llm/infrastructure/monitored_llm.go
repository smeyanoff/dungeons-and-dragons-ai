package infrastructure

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"dungeons-and-dragons-ai/internal/game/domain/llm_log"
	"dungeons-and-dragons-ai/internal/llm/domain"
	llmtools "dungeons-and-dragons-ai/internal/llm/domain/tools"
	"dungeons-and-dragons-ai/pkg/logger"
)

// MonitoredLLM является прокси-оберткой для LLM с логированием запросов/ответов
type MonitoredLLM struct {
	llm     domain.LLM
	logRepo LLMLogRepository

	logCh   chan *llm_log.LLMLog
	wg      sync.WaitGroup
	closed  int32
	stopCh  chan struct{}
	closeMu sync.Once
}

// sanitizeUTF8ForDB гарантирует, что строка валидна UTF-8.
// Это важно: Postgres с UTF-8 кодировкой падает на невалидных байтах (SQLSTATE 22021).
func sanitizeUTF8ForDB(s string) string {
	if s == "" {
		return s
	}
	if utf8.ValidString(s) {
		return s
	}
	return strings.ToValidUTF8(s, "\uFFFD")
}

// LLMLogRepository интерфейс для сохранения логов
type LLMLogRepository interface {
	Save(ctx context.Context, log *llm_log.LLMLog) error
}

// NewMonitoredLLM создает новый MonitoredLLM
func NewMonitoredLLM(llm domain.LLM, logRepo LLMLogRepository) *MonitoredLLM {
	m := &MonitoredLLM{
		llm:     llm,
		logRepo: logRepo,
		logCh:   make(chan *llm_log.LLMLog, 256),
		stopCh:  make(chan struct{}),
	}
	m.startLogWorker()
	return m
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
		Prompt:     sanitizeUTF8ForDB(prompt),
		Response:   sanitizeUTF8ForDB(response),
		DurationMs: duration.Milliseconds(),
		HasTools:   false,
	}
	applyTokensUsage(usage, prompt, response, logEntry)

	if err != nil {
		errStr := sanitizeUTF8ForDB(err.Error())
		logEntry.Error = &errStr
	} else {
		logEntry.Response = sanitizeUTF8ForDB(response)
	}

	// Сохраняем лог асинхронно (не блокируем основной поток)
	m.enqueueLog(logEntry)

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
		Prompt:     sanitizeUTF8ForDB(prompt),
		MaxTokens:  &maxTokens,
		Response:   sanitizeUTF8ForDB(response),
		DurationMs: duration.Milliseconds(),
		HasTools:   false,
	}
	applyTokensUsage(usage, prompt, response, logEntry)

	if err != nil {
		errStr := sanitizeUTF8ForDB(err.Error())
		logEntry.Error = &errStr
	} else {
		logEntry.Response = sanitizeUTF8ForDB(response)
	}

	// Сохраняем лог асинхронно
	m.enqueueLog(logEntry)

	return response, err
}

// GenerateWithTools генерирует ответ с инструментами и логирует запрос/ответ
func (m *MonitoredLLM) GenerateWithTools(ctx context.Context, prompt string, tools []llmtools.Tool) (*domain.LLMResponseWithTools, error) {
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
				s := sanitizeUTF8ForDB(jsonStr)
				toolsCallsJSON = &s
			}
		}
	}

	// Логируем запрос/ответ
	logEntry := &llm_log.LLMLog{
		ChatID:          chatID,
		TgUserID:        tgUserID,
		SessionID:       sessionIDPtr,
		Model:           "GigaChat",
		Prompt:          sanitizeUTF8ForDB(prompt),
		Response:        sanitizeUTF8ForDB(responseContent),
		DurationMs:      duration.Milliseconds(),
		HasTools:        hasTools,
		ToolsCalls:      toolsCallsJSON,
		ToolsCallsCount: toolsCallsCount,
	}
	applyTokensUsage(usage, prompt, responseContent, logEntry)

	if err != nil {
		errStr := sanitizeUTF8ForDB(err.Error())
		logEntry.Error = &errStr
	}

	// Сохраняем лог асинхронно
	m.enqueueLog(logEntry)

	return response, err
}

// Shutdown завершает фонового воркера и дожидается сохранения логов.
func (m *MonitoredLLM) Shutdown(ctx context.Context) error {
	if m == nil {
		return nil
	}
	m.closeMu.Do(func() {
		atomic.StoreInt32(&m.closed, 1)
		close(m.stopCh)
	})

	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

func (m *MonitoredLLM) startLogWorker() {
	if m.logRepo == nil {
		return
	}
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		for {
			select {
			case <-m.stopCh:
				m.drainLogs()
				return
			case entry := <-m.logCh:
				if entry == nil {
					continue
				}
				m.saveLog(entry)
			}
		}
	}()
}

func (m *MonitoredLLM) enqueueLog(entry *llm_log.LLMLog) {
	if entry == nil || m.logRepo == nil || atomic.LoadInt32(&m.closed) == 1 {
		return
	}
	select {
	case m.logCh <- entry:
	default:
		logger.Warn("LLM log queue full, dropping log entry",
			logger.String("model", entry.Model),
			logger.Int64("chat_id", entry.ChatID),
		)
	}
}

func (m *MonitoredLLM) saveLog(entry *llm_log.LLMLog) {
	logCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if saveErr := m.logRepo.Save(logCtx, entry); saveErr != nil {
		logger.Warn("Failed to save LLM log",
			logger.ErrorField(saveErr),
		)
	}
}

func (m *MonitoredLLM) drainLogs() {
	for {
		select {
		case entry := <-m.logCh:
			if entry == nil {
				continue
			}
			m.saveLog(entry)
		default:
			return
		}
	}
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
