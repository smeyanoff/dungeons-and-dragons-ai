package gigachat

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"
)

type Client struct {
	auth   *authClient
	cfg    Config
	client *http.Client
}

func NewClient(cfg Config) *Client {
	transport := &http.Transport{
		MaxIdleConns:        100,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	// Если нужно пропустить проверку TLS (только для тестов)
	// #nosec G402 - преднамеренно отключаем проверку TLS для работы с GigaChat API,
	// который использует корневые сертификаты Сбербанка, устанавливаемые отдельно в образе
	if cfg.SkipTLSVerify {
		transport.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	}

	return &Client{
		auth: newAuthClient(cfg),
		cfg:  cfg,
		client: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		},
	}
}

// GetToken получает токен доступа для проверки credentials
// Публичный метод для валидации credentials при старте приложения
func (c *Client) GetToken(ctx context.Context) (string, error) {
	return c.auth.getToken(ctx)
}

func (c *Client) doRequest(ctx context.Context, method, url string, body []byte) (*http.Response, error) {
	const maxRetries = 3
	const initialBackoff = 2 * time.Second
	const maxBackoff = 30 * time.Second

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Вычисляем exponential backoff: 2s, 4s, 8s
			shift := attempt - 1
			const maxSafeShift = 30
			if shift < 0 {
				shift = 0
			} else if shift > maxSafeShift {
				shift = maxSafeShift
			}
			backoff := initialBackoff * time.Duration(1<<uint(shift))
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			log.Printf("GigaChat: Rate limited (429), retry attempt %d/%d after %v...", attempt, maxRetries, backoff)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		token, err := c.auth.getToken(ctx)
		if err != nil {
			return nil, err
		}

		req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.client.Do(req)
		if err != nil {
			lastErr = err
			// Для сетевых ошибок продолжаем retry
			continue
		}

		// Если получили 401 Unauthorized, токен мог истек - пробуем обновить и повторить запрос
		if resp.StatusCode == 401 {
			// Закрываем предыдущий ответ (игнорируем ошибку, так как это cleanup)
			if closeErr := resp.Body.Close(); closeErr != nil {
				log.Printf("Warning: failed to close response body: %v", closeErr)
			}

			// Инвалидируем старый токен
			c.auth.invalidateToken()

			// Получаем новый токен
			newToken, err := c.auth.getToken(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to refresh token after 401: %w", err)
			}

			log.Printf("GigaChat: Token was invalid (401), refreshed and retrying request")

			// Повторяем запрос с новым токеном
			req, err = http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
			if err != nil {
				return nil, err
			}
			req.Header.Set("Authorization", "Bearer "+newToken)
			req.Header.Set("Accept", "application/json")
			req.Header.Set("Content-Type", "application/json")

			resp, err = c.client.Do(req)
			if err != nil {
				lastErr = err
				continue
			}
		}

		// Если получили 403 Forbidden, пробуем обновить токен и повторить
		// 403 может возникнуть из-за устаревшего токена или временных ограничений доступа
		if resp.StatusCode == 403 {
			// Читаем тело ответа для диагностики
			data, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			log.Printf("GigaChat: Permission denied (403), attempt %d/%d. Response: %s", attempt+1, maxRetries+1, string(data))

			// Если это не последняя попытка, пробуем обновить токен и повторить
			if attempt < maxRetries {
				// Инвалидируем старый токен
				c.auth.invalidateToken()

				// Получаем новый токен
				newToken, err := c.auth.getToken(ctx)
				if err != nil {
					log.Printf("GigaChat: Failed to refresh token after 403: %v", err)
					lastErr = fmt.Errorf("gigachat error status 403: Permission denied (failed to refresh token: %w)", err)
					// Добавляем небольшую задержку перед следующей попыткой
					select {
					case <-ctx.Done():
						return nil, ctx.Err()
					case <-time.After(1 * time.Second):
					}
					continue
				}

				log.Printf("GigaChat: Token refreshed after 403, retrying request")

				// Добавляем небольшую задержку перед повторным запросом (может быть временная проблема)
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(1 * time.Second):
				}

				// Повторяем запрос с новым токеном
				req, err = http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
				if err != nil {
					return nil, err
				}
				req.Header.Set("Authorization", "Bearer "+newToken)
				req.Header.Set("Accept", "application/json")
				req.Header.Set("Content-Type", "application/json")

				resp, err = c.client.Do(req)
				if err != nil {
					lastErr = err
					continue
				}

				// Если после обновления токена все еще 403, продолжаем retry цикл
				if resp.StatusCode == 403 {
					lastErr = fmt.Errorf("gigachat error status 403: Permission denied")
					continue
				}
				// Если получили другой статус, продолжаем обработку
			} else {
				// Если это последняя попытка, возвращаем ошибку с деталями
				return nil, fmt.Errorf("gigachat error status 403: Permission denied - возможно, отсутствуют необходимые права доступа или неверные учетные данные. Ответ API: %s", string(data))
			}
		}

		// Если получили 429 Too Many Requests, пробуем повторить с задержкой
		if resp.StatusCode == 429 {
			// Проверяем заголовок Retry-After, если он есть
			retryAfter := resp.Header.Get("Retry-After")
			if retryAfter != "" {
				// Retry-After обычно содержит число секунд
				if retryAfterSeconds, err := strconv.Atoi(retryAfter); err == nil && retryAfterSeconds > 0 {
					retryAfterDuration := time.Duration(retryAfterSeconds) * time.Second
					log.Printf("GigaChat: Rate limited (429), Retry-After header: %d seconds", retryAfterSeconds)
					if closeErr := resp.Body.Close(); closeErr != nil {
						log.Printf("Warning: failed to close response body: %v", closeErr)
					}
					select {
					case <-ctx.Done():
						return nil, ctx.Err()
					case <-time.After(retryAfterDuration):
					}
					continue
				}
			}

			// Если заголовка Retry-After нет, используем exponential backoff
			if attempt < maxRetries {
				if closeErr := resp.Body.Close(); closeErr != nil {
					log.Printf("Warning: failed to close response body: %v", closeErr)
				}
				lastErr = fmt.Errorf("gigachat error status 429: Too Many Requests")
				continue
			}
			// Если это последняя попытка, возвращаем ошибку
			data, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("gigachat error status 429: %s (after %d retries)", string(data), maxRetries)
		}

		// Если получили другую ошибку или успешный ответ, возвращаем
		if resp.StatusCode >= 400 && resp.StatusCode != 401 && resp.StatusCode != 403 && resp.StatusCode != 429 {
			// Для других ошибок не делаем retry, но логируем детали
			data, _ := io.ReadAll(resp.Body)
			log.Printf("GigaChat: Error status %d: %s", resp.StatusCode, string(data))
			// Возвращаем ответ для дальнейшей обработки
			return resp, nil
		}

		// Успешный ответ или 401 (уже обработан)
		return resp, nil
	}

	// Если все попытки исчерпаны
	if lastErr != nil {
		return nil, fmt.Errorf("failed after %d retries: %w", maxRetries, lastErr)
	}
	return nil, fmt.Errorf("failed after %d retries: unknown error", maxRetries)
}
