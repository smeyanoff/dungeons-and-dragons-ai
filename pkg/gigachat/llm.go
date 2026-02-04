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
	return c.ChatWithFunctions(ctx, model, message, maxTokens, nil)
}

// ChatWithFunctions выполняет запрос к GigaChat API с описаниями функций в теле запроса.
func (c *Client) ChatWithFunctions(ctx context.Context, model string, message string, maxTokens *int, functions []FunctionDefinition) (*ChatResponse, error) {
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

	if len(functions) > 0 {
		reqBody.Functions = functions
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
	log.Printf("[GigaChat] LLM request messages=%d functions=%d max_tokens_set=%v",
		len(reqBody.Messages), len(reqBody.Functions), reqBody.MaxTokens != nil)
	log.Printf("[GigaChat] Request body size: %d bytes, model: %s", len(body), model)
	log.Printf("[GigaChat] LLM request body: %s", truncateLogPayload(body, 2000))

	resp, err := c.doRequest(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gigachat error status %d: %s", resp.StatusCode, string(data))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	log.Printf("[GigaChat] LLM response body: %s", truncateLogPayload(data, 2000))

	var chatResp ChatResponse
	if err := json.Unmarshal(data, &chatResp); err != nil {
		return nil, err
	}

	// Извлекаем текст ответа из choices[0].message.content для обратной совместимости
	if len(chatResp.Choices) > 0 && len(chatResp.Choices[0].Message.Content) > 0 {
		chatResp.Output = chatResp.Choices[0].Message.Content
	}

	return &chatResp, nil
}

func truncateLogPayload(payload []byte, maxLen int) string {
	if maxLen <= 0 || len(payload) <= maxLen {
		return string(payload)
	}
	return string(payload[:maxLen]) + "...(truncated)"
}
