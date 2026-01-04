package gigachat

type ChatRequest struct {
	Model   string `json:"model"`   // Имя модели
	Message string `json:"message"` // Вопрос
}

type ChatResponse struct {
	ID     string `json:"id"`
	Output string `json:"output"` // Ответ текста
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
