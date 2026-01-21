package llm_log

import (
	"time"
)

// LLMLog представляет запись лога запроса/ответа к LLM
type LLMLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time `json:"created_at"`

	// Контекст запроса
	ChatID    int64 `gorm:"index" json:"chat_id"`    // Telegram Chat ID
	TgUserID  int64 `gorm:"index" json:"tg_user_id"` // Telegram User ID
	SessionID *uint `gorm:"index" json:"session_id"` // ID игровой сессии (опционально)

	// Детали запроса
	Model     string `json:"model"`                   // Модель LLM (например, "GigaChat")
	Prompt    string `gorm:"type:text" json:"prompt"` // Запрос к модели
	MaxTokens *int   `json:"max_tokens,omitempty"`    // Максимальное количество токенов

	// Детали ответа
	Response   string  `gorm:"type:text" json:"response"` // Ответ модели
	ResponseID *string `json:"response_id"`               // ID ответа от API (например, из GigaChat)

	// Метаданные
	DurationMs int64   `json:"duration_ms"`                      // Время выполнения в миллисекундах
	TokensUsed *int    `json:"tokens_used,omitempty"`            // Количество использованных токенов
	StatusCode *int    `json:"status_code,omitempty"`            // HTTP статус код (если была ошибка)
	Error      *string `gorm:"type:text" json:"error,omitempty"` // Ошибка (если была)

	// Дополнительная информация
	HasTools        bool    `json:"has_tools"`                              // Использовались ли инструменты
	ToolsCalls      *string `gorm:"type:text" json:"tools_calls,omitempty"` // Вызовы инструментов (JSON)
	ToolsCallsCount *int    `json:"tools_calls_count,omitempty"`            // Количество вызовов инструментов
}

// TableName указывает имя таблицы в БД
func (LLMLog) TableName() string {
	return "llm_logs"
}
