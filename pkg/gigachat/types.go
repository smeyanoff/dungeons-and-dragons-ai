package gigachat

// Message представляет сообщение в формате GigaChat API
type Message struct {
	Role    string `json:"role"`    // "user", "assistant", "system"
	Content string `json:"content"` // Текст сообщения
}

// ChatRequest представляет запрос к GigaChat API для генерации текста
type ChatRequest struct {
	Model     string    `json:"model"`                // Имя модели
	Messages  []Message `json:"messages"`             // Массив сообщений
	MaxTokens *int      `json:"max_tokens,omitempty"` // Максимальное количество токенов (опционально)
}

// Choice представляет выбор ответа от API
type Choice struct {
	Message      Message `json:"message"`                 // Сообщение с ответом
	Index        int     `json:"index"`                   // Индекс выбора
	FinishReason string  `json:"finish_reason,omitempty"` // Причина остановки генерации
}

// ChatResponse представляет ответ от GigaChat API
type ChatResponse struct {
	ID      string   `json:"id"`      // ID ответа
	Model   string   `json:"model"`   // Модель, которая сгенерировала ответ
	Choices []Choice `json:"choices"` // Массив возможных ответов
	Usage   *Usage   `json:"usage,omitempty"`
	// Для обратной совместимости - поле Output извлекается из choices[0].message.content
	Output string `json:"-"` // Вычисляется после парсинга
}

// Usage представляет статистику по токенам в ответе модели.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type EmbeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type EmbeddingData struct {
	Embedding []float32 `json:"embedding"`
	Index     int       `json:"index"`
}

type EmbeddingResponse struct {
	Data []EmbeddingData `json:"data"`
}
