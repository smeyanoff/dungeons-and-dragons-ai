package gigachat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"
)

// GenerateImageRequest представляет запрос на генерацию изображения
type GenerateImageRequest struct {
	Model        string    `json:"model"`         // Имя модели
	Messages     []Message `json:"messages"`      // Массив сообщений с промптом для генерации
	FunctionCall string    `json:"function_call"` // "auto" для активации text2image функции
}

// GenerateImageResponse представляет ответ на запрос генерации изображения
type GenerateImageResponse struct {
	ID      string   `json:"id"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
}

// ExtractImageID извлекает file_id изображения из ответа модели
// Ответ содержит <img src="file_id" fuse="true"/>
func ExtractImageID(content string) (string, error) {
	// Регулярное выражение для поиска file_id в теге <img>
	re := regexp.MustCompile(`<img\s+src="([^"]+)"`)
	matches := re.FindStringSubmatch(content)
	if len(matches) < 2 {
		return "", fmt.Errorf("no image ID found in content: %s", content)
	}
	return matches[1], nil
}

// GenerateImage генерирует изображение через GigaChat API используя встроенную функцию text2image
func (c *Client) GenerateImage(ctx context.Context, model string, systemPrompt string, userPrompt string) (string, error) {
	// Создаем сообщения для генерации изображения
	messages := []Message{
		{
			Role:    "system",
			Content: systemPrompt,
		},
		{
			Role:    "user",
			Content: userPrompt,
		},
	}

	reqBody := GenerateImageRequest{
		Model:        model,
		Messages:     messages,
		FunctionCall: "auto", // Активируем text2image функцию
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/chat/completions", c.cfg.APIBaseURL)

	// Используем doRequest для автоматического retry при 429 ошибках
	resp, err := c.doRequest(ctx, http.MethodPost, url, body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("gigachat image generation error status %d: %s", resp.StatusCode, string(data))
	}

	var imageResp GenerateImageResponse
	if err := json.NewDecoder(resp.Body).Decode(&imageResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	// Извлекаем file_id из ответа
	if len(imageResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	content := imageResp.Choices[0].Message.Content
	if content == "" {
		return "", fmt.Errorf("empty content in response")
	}

	fileID, err := ExtractImageID(content)
	if err != nil {
		return "", fmt.Errorf("failed to extract image ID: %w", err)
	}

	return fileID, nil
}

// DownloadImage скачивает изображение по file_id с retry механизмом для 403 ошибок
// (изображение может быть еще не готово сразу после генерации)
func (c *Client) DownloadImage(ctx context.Context, fileID string) ([]byte, error) {
	url := fmt.Sprintf("%s/files/%s/content", c.cfg.APIBaseURL, fileID)

	const maxRetries = 3
	const retryDelay = 2 * time.Second

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Задержка перед повторной попыткой (изображение может быть еще не готово)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryDelay):
			}
		}

		// Используем doRequest для автоматического retry при 429 ошибках
		// doRequest сам добавит X-Client-ID для image-related запросов
		resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
		if err != nil {
			if attempt < maxRetries-1 {
				continue // Retry при сетевых ошибках
			}
			return nil, fmt.Errorf("failed to download image: %w", err)
		}

		// Если получили 403, пробуем еще раз (изображение может быть еще не готово)
		if resp.StatusCode == 403 && attempt < maxRetries-1 {
			if err := resp.Body.Close(); err != nil {
				// Логируем ошибку закрытия, но продолжаем retry
				fmt.Printf("warning: failed to close response body: %v\n", err)
			}
			continue
		}

		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			data, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("gigachat image download error status %d: %s", resp.StatusCode, string(data))
		}

		imageData, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read image data: %w", err)
		}

		return imageData, nil
	}

	return nil, fmt.Errorf("failed to download image after %d attempts", maxRetries)
}
