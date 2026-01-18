package gigachat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func (c *Client) Chat(ctx context.Context, model string, message string) (*ChatResponse, error) {
	return c.ChatWithMaxTokens(ctx, model, message, nil)
}

// ChatWithMaxTokens выполняет запрос к GigaChat API с указанием максимального количества токенов
func (c *Client) ChatWithMaxTokens(ctx context.Context, model string, message string, maxTokens *int) (*ChatResponse, error) {
	// Создаем запрос в формате GigaChat API с массивом сообщений
	reqBody := ChatRequest{
		Model: model,
		Messages: []Message{
			{
				Role:    "user",
				Content: message,
			},
		},
	}
	
	// Устанавливаем max_tokens если указано (по умолчанию 8192 для генерации кампании)
	if maxTokens != nil {
		reqBody.MaxTokens = maxTokens
	}
	
	body, _ := json.Marshal(reqBody)

	url := fmt.Sprintf("%s/chat/completions", c.cfg.APIBaseURL)

	resp, err := c.doRequest(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gigachat error status %d: %s", resp.StatusCode, string(data))
	}

	var chatResp ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, err
	}

	// Извлекаем текст ответа из choices[0].message.content для обратной совместимости
	if len(chatResp.Choices) > 0 && len(chatResp.Choices[0].Message.Content) > 0 {
		chatResp.Output = chatResp.Choices[0].Message.Content
	}

	return &chatResp, nil
}
