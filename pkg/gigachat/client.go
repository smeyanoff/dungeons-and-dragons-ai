package gigachat

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type Client struct {
	auth          *authClient
	cfg           Config
	client        *http.Client
	semaphore     chan struct{} // Concurrency control semaphore
	requestCount  int64         // Counter for metrics
	errorCount    int64         // Counter for error metrics
	rateLimitCount int64        // Counter for 429 responses
	startTime     time.Time     // Time when client was created for RPS calculation
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

	// Concurrency limit: уменьшаем до 2 для предотвращения rate limiting
	concurrencyLimit := 2
	if cfg.ConcurrencyLimit > 0 {
		concurrencyLimit = cfg.ConcurrencyLimit
	}

	return &Client{
		auth:          newAuthClient(cfg),
		cfg:           cfg,
		client: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		},
		semaphore:     make(chan struct{}, concurrencyLimit),
		requestCount:  0,
		errorCount:    0,
		rateLimitCount: 0,
		startTime:     time.Now(),
	}
}

// GetToken получает токен доступа для проверки credentials
// Публичный метод для валидации credentials при старте приложения
func (c *Client) GetToken(ctx context.Context) (string, error) {
	return c.auth.getToken(ctx)
}

// GetMetrics возвращает текущие метрики клиента
func (c *Client) GetMetrics() map[string]int64 {
	return map[string]int64{
		"requests":     atomic.LoadInt64(&c.requestCount),
		"errors":       atomic.LoadInt64(&c.errorCount),
		"rate_limits":  atomic.LoadInt64(&c.rateLimitCount),
		"rps":          c.calculateRPS(), // Добавляем RPS метрику
	}
}

// calculateRPS вычисляет requests per second на основе общего времени работы
func (c *Client) calculateRPS() int64 {
	elapsed := time.Since(c.startTime)
	if elapsed.Seconds() == 0 {
		return 0
	}
	requests := atomic.LoadInt64(&c.requestCount)
	return int64(float64(requests) / elapsed.Seconds())
}

func (c *Client) doRequest(ctx context.Context, method, url string, body []byte) (*http.Response, error) {
	// Acquire semaphore for concurrency control
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }() // Release semaphore
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Добавляем X-Client-ID для image-related запросов (генерация и скачивание изображений)
	isImageRequest := strings.Contains(url, "/files/") || (strings.Contains(url, "/chat/completions") && len(body) > 0 && strings.Contains(string(body), "text2image"))
	const maxRetries = 3
	const initialBackoff = 5 * time.Second // Увеличиваем начальную задержку для rate limiting
	const maxBackoff = 60 * time.Second    // Увеличиваем максимальную задержку
	const jitterFactor = 0.1 // 10% jitter

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Вычисляем exponential backoff с jitter: 2s, 4s, 8s
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

			// Add jitter to prevent thundering herd
			jitter := time.Duration(float64(backoff) * jitterFactor * rand.Float64())
			totalBackoff := backoff + jitter

			log.Printf("GigaChat: Rate limited (429), retry attempt %d/%d after %v (backoff: %v + jitter: %v)...",
				attempt, maxRetries, totalBackoff, backoff, jitter)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(totalBackoff):
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

		// Добавляем X-Client-ID для image-related запросов
		if isImageRequest && c.cfg.ClientID != "" {
			req.Header.Set("X-Client-ID", strings.TrimSpace(c.cfg.ClientID))
		}

		// Increment request counter
		atomic.AddInt64(&c.requestCount, 1)

		resp, err := c.client.Do(req)
		if err != nil {
			atomic.AddInt64(&c.errorCount, 1)
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

		// Добавляем X-Client-ID для image-related запросов
		if isImageRequest && c.cfg.ClientID != "" {
			req.Header.Set("X-Client-ID", strings.TrimSpace(c.cfg.ClientID))
		}

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

			log.Printf("GigaChat: Permission denied (403) for URL %s, attempt %d/%d. Response: %s", url, attempt+1, maxRetries+1, string(data))
			log.Printf("GigaChat: NOT refreshing token for 403 error - this indicates permissions issue, not token expiry")

			// Для 403 ошибок НЕ обновляем токен, так как это проблема прав доступа
			// 403 = Forbidden, что означает недостаточные права для данного ClientID/токена
			lastErr = fmt.Errorf("gigachat error status 403: Permission denied - check ClientID permissions, X-Client-ID, and API access rights. Response: %s", string(data))
			break
		}

		// Если получили 429 Too Many Requests, пробуем повторить с задержкой
		if resp.StatusCode == 429 {
			// Increment rate limit counter
			atomic.AddInt64(&c.rateLimitCount, 1)

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
