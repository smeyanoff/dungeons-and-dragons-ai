package gigachat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
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

	// Устанавливаем max_tokens только если указано (nil означает отсутствие ограничения)
	if maxTokens != nil {
		reqBody.MaxTokens = maxTokens
	}

	body, _ := json.Marshal(reqBody)

	apiURL := c.cfg.APIBaseURL
	// Fix incorrect URL if it points to auth endpoint
	if strings.Contains(apiURL, ":9443") && !strings.Contains(apiURL, "/api/") {
		apiURL = strings.Replace(apiURL, ":9443", ":9443/api/v1", 1)
		log.Printf("[GigaChat] Fixed API URL from auth endpoint to API endpoint: %s", apiURL)
	}

	url := fmt.Sprintf("%s/chat/completions", apiURL)
	log.Printf("[GigaChat] APIBaseURL: %s", c.cfg.APIBaseURL)
	log.Printf("[GigaChat] Fixed API URL: %s", apiURL)
	log.Printf("[GigaChat] LLM request URL: %s", url)
	log.Printf("[GigaChat] Request body size: %d bytes, model: %s", len(body), model)

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
