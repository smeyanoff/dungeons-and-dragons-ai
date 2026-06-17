package gigachat

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// GenerateImageRequest представляет запрос на генерацию изображения
type GenerateImageRequest struct {
	Model        string `json:"model"`         // Имя модели
	Messages     []Message `json:"messages"`      // Массив сообщений с промптом для генерации
	FunctionCall string `json:"function_call"` // "auto" для активации text2image функции
	Stream       *bool  `json:"stream,omitempty"` // Потоковая генерация для долгих операций
}

// GenerateImageResponse представляет ответ на запрос генерации изображения
type GenerateImageResponse struct {
	ID      string   `json:"id"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
}

// ExtractImageID извлекает file_id изображения из ответа модели
// Ответ может содержать <img src="file_id" fuse="true"/> или просто file_id
func ExtractImageID(content string) (string, error) {
	log.Printf("[GigaChat] Extracting image ID from content: %s", content)

	// Сначала пробуем найти в HTML теге <img>
	re := regexp.MustCompile(`<img\s+src="([^"]+)"`)
	matches := re.FindStringSubmatch(content)
	if len(matches) >= 2 {
		fileID := matches[1]
		log.Printf("[GigaChat] Successfully extracted image ID from HTML: %s", fileID)
		return fileID, nil
	}

	// Если не нашли в HTML, ищем UUID паттерн напрямую
	uuidPattern := regexp.MustCompile(`[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}`)
	matches = uuidPattern.FindStringSubmatch(content)
	if len(matches) > 0 {
		fileID := matches[0]
		log.Printf("[GigaChat] Successfully extracted image ID as raw UUID: %s", fileID)
		return fileID, nil
	}

	log.Printf("[GigaChat] No image ID found in content")
	return "", fmt.Errorf("no image ID found in content: %s", content)
}

// TestExtractImageID тестирует извлечение image ID из различных форматов ответов
func TestExtractImageID(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
		hasError bool
	}{
		{
			name:     "standard format",
			content:  `<img src="b28fbd4f-105a-43e0-ba5a-2faa80b1f43c" fuse="true"/>`,
			expected: "b28fbd4f-105a-43e0-ba5a-2faa80b1f43c",
			hasError: false,
		},
		{
			name:     "with text around",
			content:  `Запускаю генерацию изображения. <img src="a598ae3d-d3eb-454e-a8bf-0848193a603e" fuse="true"/> - вот розовый кот`,
			expected: "a598ae3d-d3eb-454e-a8bf-0848193a603e",
			hasError: false,
		},
		{
			name:     "no image tag",
			content:  "Обычный текст без изображения",
			expected: "",
			hasError: true,
		},
		{
			name:     "empty src",
			content:  `<img src="" fuse="true"/>`,
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExtractImageID(tt.content)
			if tt.hasError {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if result != tt.expected {
					t.Errorf("expected %s, got %s", tt.expected, result)
				}
			}
		})
	}
}

// GenerateImage генерирует изображение через GigaChat API используя встроенную функцию text2image
func (c *Client) GenerateImage(ctx context.Context, model string, systemPrompt string, userPrompt string) (string, error) {
	log.Printf("[GigaChat] Starting image generation with model: %s, systemPrompt: %s, userPrompt: %s", model, systemPrompt, userPrompt)

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

	// Пока отключаем stream для изображений, так как GigaChat может не поддерживать его для function calls
	stream := false
	reqBody := GenerateImageRequest{
		Model:        model,
		Messages:     messages,
		FunctionCall: "auto", // Активируем text2image функцию
		Stream:       &stream,
	}

	reqBodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	log.Printf("[GigaChat] Request body: %s", string(reqBodyBytes))

	apiURL := c.cfg.APIBaseURL
	// Fix incorrect URL if it points to auth endpoint
	if strings.Contains(apiURL, ":9443") && !strings.Contains(apiURL, "/api/") {
		apiURL = strings.Replace(apiURL, ":9443", ":9443/api/v1", 1)
		log.Printf("[GigaChat] Fixed API URL from auth endpoint to API endpoint: %s", apiURL)
	}

	url := fmt.Sprintf("%s/chat/completions", apiURL)

	// Используем doRequestWithClient с imageClient для автоматического retry при 429 ошибках
	resp, err := c.doRequestWithClient(ctx, c.imageClient, http.MethodPost, url, reqBodyBytes)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("gigachat image generation error status %d: %s", resp.StatusCode, string(data))
	}

	// Обрабатываем ответ в зависимости от того, используется ли stream
	var imageResp *GenerateImageResponse
	if stream {
		var parseErr error
		imageResp, parseErr = c.parseStreamResponse(ctx, resp.Body)
		if parseErr != nil {
			return "", fmt.Errorf("failed to parse stream response: %w", parseErr)
		}
	} else {
		// Обычный JSON ответ
		var regularResp GenerateImageResponse
		if decodeErr := json.NewDecoder(resp.Body).Decode(&regularResp); decodeErr != nil {
			return "", fmt.Errorf("failed to decode JSON response: %w", decodeErr)
		}
		imageResp = &regularResp
	}

	// Логируем полный ответ
	respJson, _ := json.Marshal(imageResp)
	log.Printf("[GigaChat] Full response: %s", string(respJson))

	// Извлекаем file_id из ответа
	if len(imageResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	content := imageResp.Choices[0].Message.Content
	if content == "" {
		return "", fmt.Errorf("empty content in response")
	}

	log.Printf("[GigaChat] Response content: %s", content)

	fileID, err := ExtractImageID(content)
	if err != nil {
		// Если не удалось извлечь из HTML, попробуем найти UUID напрямую в тексте
		log.Printf("[GigaChat] Failed to extract from HTML, trying direct UUID extraction: %v", err)

		// Ищем UUID паттерн в тексте
		uuidPattern := regexp.MustCompile(`[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}`)
		matches := uuidPattern.FindStringSubmatch(content)
		if len(matches) > 0 {
			fileID = matches[0]
			log.Printf("[GigaChat] Extracted file ID directly from text: %s", fileID)
		} else {
			return "", fmt.Errorf("failed to extract image ID: %w", err)
		}
	}

	log.Printf("[GigaChat] Final file ID: %s", fileID)
	return fileID, nil
}

// parseStreamResponse читает потоковый ответ и извлекает финальный ответ с изображением
func (c *Client) parseStreamResponse(ctx context.Context, body io.Reader) (*GenerateImageResponse, error) {
	log.Printf("[GigaChat] Starting to parse stream response")

	// Создаем контекст с дополнительным таймаутом для чтения stream (5 минут для изображений)
	streamCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	scanner := bufio.NewScanner(body)
	var lastCompleteResponse *GenerateImageResponse

	for scanner.Scan() {
		// Проверяем, не отменен ли контекст
		select {
		case <-streamCtx.Done():
			if lastCompleteResponse != nil {
				log.Printf("[GigaChat] Context canceled but we have complete response with image")
				return lastCompleteResponse, nil
			}
			return nil, streamCtx.Err()
		default:
		}
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Удаляем префикс "data: " если есть
		if strings.HasPrefix(line, "data: ") {
			line = line[6:]
		}

		// Проверяем на завершение потока
		if strings.TrimSpace(line) == "[DONE]" {
			break
		}

		// Парсим JSON
		var chunk map[string]interface{}
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			log.Printf("[GigaChat] Failed to parse stream chunk: %v, line: %s", err, line)
			continue
		}

		// Проверяем, есть ли choices в ответе
		if choices, ok := chunk["choices"].([]interface{}); ok && len(choices) > 0 {
			if choice, ok := choices[0].(map[string]interface{}); ok {
				if message, ok := choice["message"].(map[string]interface{}); ok {
					if content, ok := message["content"].(string); ok && content != "" {
						// Создаем полный ответ из chunk
						var imageResp GenerateImageResponse
						if id, ok := chunk["id"].(string); ok {
							imageResp.ID = id
						}
						if model, ok := chunk["model"].(string); ok {
							imageResp.Model = model
						}

						// Создаем choice с message
						imageResp.Choices = []Choice{
							{
								Message: Message{
									Role:    "assistant",
									Content: content,
								},
							},
						}

						// Проверяем, содержит ли сообщение изображение (не промежуточное сообщение о прогрессе)
						if strings.Contains(content, "<img src=") {
							lastCompleteResponse = &imageResp
							log.Printf("[GigaChat] Found image in stream response: %s", content)
							// Выходим из цикла, так как нашли финальный ответ
							break
						} else {
							log.Printf("[GigaChat] Progress message in stream: %s", content)
						}
					}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		// Если у нас есть ответ, но произошла ошибка чтения, возвращаем ответ
		if lastCompleteResponse != nil {
			log.Printf("[GigaChat] Scanner error but we have complete response: %v", err)
			return lastCompleteResponse, nil
		}
		return nil, fmt.Errorf("error reading stream: %w", err)
	}

	if lastCompleteResponse == nil {
		return nil, fmt.Errorf("no complete image response found in stream")
	}

	return lastCompleteResponse, nil
}

// DownloadImage скачивает изображение по file_id с улучшенной retry логикой для 403 ошибок
// (изображение может быть еще не готово сразу после генерации)
func (c *Client) DownloadImage(ctx context.Context, fileID string) ([]byte, error) {
	log.Printf("[GigaChat] Starting image download for fileID: %s", fileID)
	url := fmt.Sprintf("%s/files/%s/content", c.cfg.APIBaseURL, fileID)

	const maxRetries = 3                 // Уменьшаем количество попыток для изображений
	const initialDelay = 2 * time.Second // Увеличиваем начальную задержку
	const maxDelay = 10 * time.Second    // Максимальная задержка

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff для изображений: 2s, 4s, 8s
			delay := initialDelay * time.Duration(1<<uint(attempt-1))
			if delay > maxDelay {
				delay = maxDelay
			}

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		log.Printf("[GigaChat] Download attempt %d/%d for fileID: %s", attempt+1, maxRetries, fileID)

		// Используем doRequestWithClient с imageClient для автоматического retry
		// doRequestWithClient сам добавит X-Client-ID для image-related запросов
		resp, err := c.doRequestWithClient(ctx, c.imageClient, http.MethodGet, url, nil)
		if err != nil {
			log.Printf("[GigaChat] Download attempt %d failed with network error: %v", attempt+1, err)
			if attempt < maxRetries-1 {
				continue // Retry при сетевых ошибках
			}
			return nil, fmt.Errorf("failed to download image: %w", err)
		}

		// Если получили 403, пробуем еще раз (изображение может быть еще не готово)
		if resp.StatusCode == 403 && attempt < maxRetries-1 {
			if err := resp.Body.Close(); err != nil {
				// Логируем ошибку закрытия, но продолжаем retry
				log.Printf("warning: failed to close response body: %v", err)
			}
			continue
		}

		defer resp.Body.Close()

		// Логируем заголовки ответа
		contentLength := resp.Header.Get("Content-Length")
		log.Printf("[GigaChat] Download response headers: Content-Length=%s, Content-Type=%s",
			contentLength, resp.Header.Get("Content-Type"))

		if resp.StatusCode >= 400 {
			data, _ := io.ReadAll(resp.Body)
			log.Printf("[GigaChat] Download error response body: %s", string(data))
			return nil, fmt.Errorf("gigachat image download error status %d: %s", resp.StatusCode, string(data))
		}

		// Проверяем, не отменен ли контекст перед чтением
		select {
		case <-ctx.Done():
			log.Printf("[GigaChat] Context canceled before reading image data")
			return nil, ctx.Err()
		default:
		}

		// Читаем данные полностью
		imageData, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("[GigaChat] Failed to read image data after %d bytes: %v", len(imageData), err)

			// Проверяем, является ли это частичным чтением из-за отмены контекста
			if err == context.Canceled || err == context.DeadlineExceeded {
				log.Printf("[GigaChat] Context canceled during download, read %d bytes", len(imageData))

				// Если Content-Length указан и мы прочитали меньше - это действительно partial
				if contentLength != "" {
					if expectedLen, parseErr := strconv.Atoi(contentLength); parseErr == nil && len(imageData) < expectedLen {
						log.Printf("[GigaChat] Partial download: got %d bytes, expected %d bytes", len(imageData), expectedLen)
						return nil, fmt.Errorf("partial image download: got %d/%d bytes", len(imageData), expectedLen)
					}
				}

				// Если Content-Length не указан, пробуем проверить, является ли это валидным JPEG
				if len(imageData) > 0 && isValidJPEGHeader(imageData) {
					log.Printf("[GigaChat] Context canceled but data appears to be valid JPEG, returning partial result")
					return imageData, nil
				}

				return nil, fmt.Errorf("download interrupted and data appears incomplete")
			}

			return nil, fmt.Errorf("failed to read image data: %w", err)
		}

		log.Printf("[GigaChat] Successfully downloaded image, size: %d bytes", len(imageData))
		return imageData, nil
	}

	return nil, fmt.Errorf("failed to download image after %d attempts", maxRetries)
}

// isValidJPEGHeader проверяет, начинается ли данные с валидного JPEG заголовка
func isValidJPEGHeader(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	// JPEG файлы начинаются с FF D8 FF
	return data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF
}
