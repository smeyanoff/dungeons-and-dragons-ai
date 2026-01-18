package gigachat

// Message представляет сообщение в формате GigaChat API
type Message struct {
	Role    string `json:"role"`    // "user", "assistant", "system"
	Content string `json:"content"` // Текст сообщения
}

// ChatRequest представляет запрос к GigaChat API для генерации текста
type ChatRequest struct {
	Model     string    `json:"model"`               // Имя модели
	Messages  []Message `json:"messages"`            // Массив сообщений
	MaxTokens *int      `json:"max_tokens,omitempty"` // Максимальное количество токенов (опционально)
}

// Choice представляет выбор ответа от API
type Choice struct {
	Message Message `json:"message"` // Сообщение с ответом
	Index   int     `json:"index"`   // Индекс выбора
}

// ChatResponse представляет ответ от GigaChat API
type ChatResponse struct {
	ID      string   `json:"id"`      // ID ответа
	Model   string   `json:"model"`   // Модель, которая сгенерировала ответ
	Choices []Choice `json:"choices"` // Массив возможных ответов
	// Для обратной совместимости - поле Output извлекается из choices[0].message.content
	Output string `json:"-"` // Вычисляется после парсинга
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
