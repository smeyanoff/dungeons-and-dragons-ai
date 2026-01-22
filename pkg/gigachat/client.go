package gigachat

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/time/rate"
)

type Client struct {
	auth           *authClient
	cfg            Config
	client         *http.Client
	imageClient    *http.Client    // HTTP client with extended timeout for image operations
	semaphore      chan struct{}   // Concurrency control semaphore
	rateLimiter    *rate.Limiter   // Global rate limiter for all requests (10 RPS by default)
	requestCount   int64           // Counter for metrics
	errorCount     int64           // Counter for error metrics
	rateLimitCount int64           // Counter for 429 responses
	startTime      time.Time       // Time when client was created for RPS calculation
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
	concurrencyLimit := 1
	if cfg.ConcurrencyLimit > 0 {
		concurrencyLimit = cfg.ConcurrencyLimit
	}

	// Global rate limiter: configurable RPS with burst to prevent DDoS-like behavior
	rpsLimit := rate.Limit(10.0) // Default 10 requests per second
	if cfg.RPSLimit > 0 {
		rpsLimit = rate.Limit(cfg.RPSLimit)
	}
	burstLimit := 5 // Default burst of 5 requests
	if cfg.RateBurst > 0 {
		burstLimit = cfg.RateBurst
	}
	rateLimiter := rate.NewLimiter(rpsLimit, burstLimit)

	return &Client{
		auth: newAuthClient(cfg),
		cfg:  cfg,
		client: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		},
		imageClient: &http.Client{
			Timeout:   5 * time.Minute, // Extended timeout for image generation and download
			Transport: transport,
		},
		semaphore:      make(chan struct{}, concurrencyLimit),
		rateLimiter:    rateLimiter,
		requestCount:   0,
		errorCount:     0,
		rateLimitCount: 0,
		startTime:      time.Now(),
	}
}

// GetToken получает токен доступа для проверки credentials
// Публичный метод для валидации credentials при старте приложения
func (c *Client) GetToken(ctx context.Context) (string, error) {
	return c.auth.getToken(ctx)
}

// GetMetrics возвращает текущие метрики клиента
func (c *Client) GetMetrics() map[string]int64 {
	metrics := map[string]int64{
		"requests":    atomic.LoadInt64(&c.requestCount),
		"errors":      atomic.LoadInt64(&c.errorCount),
		"rate_limits": atomic.LoadInt64(&c.rateLimitCount),
		"rps":         c.calculateRPS(), // Добавляем RPS метрику
	}

	// Add rate limiter tokens info if available
	if c.rateLimiter != nil {
		metrics["rate_limiter_tokens"] = int64(c.rateLimiter.Tokens())
	}

	// Add auth rate limiter tokens info if available
	if c.auth != nil && c.auth.rateLimiter != nil {
		metrics["auth_rate_limiter_tokens"] = int64(c.auth.rateLimiter.Tokens())
	}

	return metrics
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

// doRequestWithClient выполняет HTTP запрос с указанным клиентом
func (c *Client) doRequestWithClient(ctx context.Context, client *http.Client, method, url string, body []byte) (*http.Response, error) {
	// Acquire semaphore for concurrency control
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }() // Release semaphore
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Apply global rate limiter to prevent DDoS-like behavior
	rateLimiterTokens := c.rateLimiter.Tokens()
	log.Printf("[GigaChat] Request: %s %s, rate limiter tokens=%.1f", method, url, rateLimiterTokens)

	if err := c.rateLimiter.Wait(ctx); err != nil {
		log.Printf("[GigaChat] Rate limiter wait failed for %s %s: %v", method, url, err)
		return nil, fmt.Errorf("rate limiter wait failed: %w", err)
	}

	// Добавляем X-Client-ID для всех image-related запросов
	// Важно: один и тот же X-Client-ID должен использоваться для генерации и скачивания изображений
	isImageRequest := strings.Contains(url, "/files/") ||
		(strings.Contains(url, "/chat/completions") && len(body) > 0 && strings.Contains(string(body), "function_call"))
	log.Printf("[GigaChat] Request analysis: URL=%s, hasBody=%v, containsFunctionCall=%v, isImageRequest=%v",
		url, len(body) > 0, len(body) > 0 && strings.Contains(string(body), "function_call"), isImageRequest)

	// Логируем body для image запросов
	if isImageRequest && len(body) > 0 {
		log.Printf("[GigaChat] Image request body: %s", string(body))
	}
	const maxRetries = 3
	const initialBackoff = 5 * time.Second // Увеличиваем начальную задержку для rate limiting
	const maxBackoff = 60 * time.Second    // Увеличиваем максимальную задержку
	const jitterFactor = 0.1               // 10% jitter

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

			log.Printf("GigaChat: Network error, retry attempt %d/%d after %v (backoff: %v + jitter: %v)...",
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

		// Устанавливаем правильный Accept header в зависимости от типа запроса
		if isImageRequest && strings.Contains(url, "/files/") {
			// Для скачивания изображений
			req.Header.Set("Accept", "application/jpg")
		} else {
			// Для обычных API запросов
			req.Header.Set("Accept", "application/json")
		}

		// Устанавливаем Content-Type только если есть тело запроса
		if len(body) > 0 {
			req.Header.Set("Content-Type", "application/json")
		}

			// Добавляем X-Client-ID для image-related запросов
		if isImageRequest && c.cfg.ClientID != "" {
			req.Header.Set("X-Client-ID", strings.TrimSpace(c.cfg.ClientID))
			log.Printf("[GigaChat] Set X-Client-ID for image request: %s", strings.TrimSpace(c.cfg.ClientID))
		} else if isImageRequest {
			log.Printf("[GigaChat] Image request detected but ClientID empty: cfg.ClientID='%s'", c.cfg.ClientID)
		}

		// Логируем заголовки для диагностики
		log.Printf("[GigaChat] Headers: Accept=%s, Content-Type=%s, X-Client-ID=%s",
			req.Header.Get("Accept"),
			req.Header.Get("Content-Type"),
			req.Header.Get("X-Client-ID"))

		// Increment request counter
		atomic.AddInt64(&c.requestCount, 1)

		resp, err := client.Do(req)
		if err != nil {
			atomic.AddInt64(&c.errorCount, 1)
			log.Printf("[GigaChat] Network error for %s %s: %v", method, url, err)
			lastErr = err

			// Проверяем тип network ошибки
			if isTimeoutError(err) {
				log.Printf("[GigaChat] Timeout error detected, will retry without backoff: %v", err)
				// Для timeout ошибок делаем retry без задержки (но с учетом общего rate limiter)
				continue
			} else {
				log.Printf("[GigaChat] Network error (non-timeout), will retry with backoff: %v", err)
				// Для других сетевых ошибок продолжаем retry с обычной логикой
				continue
			}
		}

		// Логируем статус ответа
		log.Printf("[GigaChat] Response status: %d for %s %s", resp.StatusCode, method, url)

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

			resp, err = client.Do(req)
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
			return nil, fmt.Errorf("gigachat error status 429: Too Many Requests. Response: %s", string(data))
		}

		// Если получили другую ошибку или успешный ответ, возвращаем
		if resp.StatusCode >= 400 && resp.StatusCode != 401 && resp.StatusCode != 403 && resp.StatusCode != 429 {
			// Для других ошибок не делаем retry, но логируем детали
			data, _ := io.ReadAll(resp.Body)
			log.Printf("GigaChat: Error status %d: %s", resp.StatusCode, string(data))
			// Возвращаем ответ для дальнейшей обработки
			return resp, nil
		}

		// Специальная обработка 403 Forbidden
		if resp.StatusCode == 403 {
			data, _ := io.ReadAll(resp.Body)
			log.Printf("[GigaChat] Forbidden (403) - possible token expiry or X-Client-ID issue")
			log.Printf("[GigaChat] 403 Details: URL=%s, Method=%s, X-Client-ID=%s, Response=%s",
				url, method, req.Header.Get("X-Client-ID"), string(data))
			resp.Body.Close()
			return nil, fmt.Errorf("gigachat forbidden (403): %s", string(data))
		}

		// Логируем успешные ответы
		log.Printf("[GigaChat] Success response: %d for %s %s", resp.StatusCode, method, url)

		// Успешный ответ или 401 (уже обработан)
		return resp, nil
	}

	// Если все попытки исчерпаны
	if lastErr != nil {
		return nil, fmt.Errorf("failed after %d retries: %w", maxRetries, lastErr)
	}
	return nil, fmt.Errorf("failed after %d retries: unknown error", maxRetries)
}

func (c *Client) doRequest(ctx context.Context, method, url string, body []byte) (*http.Response, error) {
	// Acquire semaphore for concurrency control
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }() // Release semaphore
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Apply global rate limiter to prevent DDoS-like behavior
	rateLimiterTokens := c.rateLimiter.Tokens()
	log.Printf("[GigaChat] Request: %s %s, rate limiter tokens=%.1f", method, url, rateLimiterTokens)

	if err := c.rateLimiter.Wait(ctx); err != nil {
		log.Printf("[GigaChat] Rate limiter wait failed for %s %s: %v", method, url, err)
		return nil, fmt.Errorf("rate limiter wait failed: %w", err)
	}

	// Добавляем X-Client-ID для всех image-related запросов
	// Важно: один и тот же X-Client-ID должен использоваться для генерации и скачивания изображений
	isImageRequest := strings.Contains(url, "/files/") ||
		(strings.Contains(url, "/chat/completions") && len(body) > 0 && strings.Contains(string(body), "function_call"))
	log.Printf("[GigaChat] Request analysis: URL=%s, hasBody=%v, containsFunctionCall=%v, isImageRequest=%v",
		url, len(body) > 0, len(body) > 0 && strings.Contains(string(body), "function_call"), isImageRequest)

	// Логируем body для image запросов
	if isImageRequest && len(body) > 0 {
		log.Printf("[GigaChat] Image request body: %s", string(body))
	}

	// Для image generation используем увеличенный timeout (генерация может занимать до 2-3 минут)
	timeout := 30 * time.Second
	if isImageRequest {
		timeout = 3 * time.Minute // Увеличиваем timeout для генерации изображений
		log.Printf("[GigaChat] Using extended timeout for image request: %v", timeout)
	}
	const maxRetries = 3
	const initialBackoff = 5 * time.Second // Увеличиваем начальную задержку для rate limiting
	const maxBackoff = 60 * time.Second    // Увеличиваем максимальную задержку
	const jitterFactor = 0.1               // 10% jitter

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

			log.Printf("GigaChat: Network error, retry attempt %d/%d after %v (backoff: %v + jitter: %v)...",
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

		// Создаем запрос с увеличенным timeout для image requests
		reqCtx := ctx
		if isImageRequest {
			var cancel context.CancelFunc
			reqCtx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel() // Cleanup в конце функции
		}

		req, err := http.NewRequestWithContext(reqCtx, method, url, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)

		// Устанавливаем правильный Accept header в зависимости от типа запроса
		if isImageRequest && strings.Contains(url, "/files/") {
			// Для скачивания изображений
			req.Header.Set("Accept", "application/jpg")
		} else {
			// Для обычных API запросов
			req.Header.Set("Accept", "application/json")
		}

		// Устанавливаем Content-Type только если есть тело запроса
		if len(body) > 0 {
			req.Header.Set("Content-Type", "application/json")
		}

			// Добавляем X-Client-ID для image-related запросов
		if isImageRequest && c.cfg.ClientID != "" {
			req.Header.Set("X-Client-ID", strings.TrimSpace(c.cfg.ClientID))
			log.Printf("[GigaChat] Set X-Client-ID for image request: %s", strings.TrimSpace(c.cfg.ClientID))
		} else if isImageRequest {
			log.Printf("[GigaChat] Image request detected but ClientID empty: cfg.ClientID='%s'", c.cfg.ClientID)
		}

		// Логируем заголовки для диагностики
		log.Printf("[GigaChat] Headers: Accept=%s, Content-Type=%s, X-Client-ID=%s",
			req.Header.Get("Accept"),
			req.Header.Get("Content-Type"),
			req.Header.Get("X-Client-ID"))

		// Increment request counter
		atomic.AddInt64(&c.requestCount, 1)

		resp, err := c.client.Do(req)
		if err != nil {
			atomic.AddInt64(&c.errorCount, 1)
			log.Printf("[GigaChat] Network error for %s %s: %v", method, url, err)
			lastErr = err

			// Проверяем тип network ошибки
			if isTimeoutError(err) {
				log.Printf("[GigaChat] Timeout error detected, will retry without backoff: %v", err)
				// Для timeout ошибок делаем retry без задержки (но с учетом общего rate limiter)
				continue
			} else {
				log.Printf("[GigaChat] Network error (non-timeout), will retry with backoff: %v", err)
				// Для других сетевых ошибок продолжаем retry с обычной логикой
				continue
			}
		}

		// Логируем статус ответа
		log.Printf("[GigaChat] Response status: %d for %s %s", resp.StatusCode, method, url)

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

		// Специальная обработка 403 Forbidden
		if resp.StatusCode == 403 {
			data, _ := io.ReadAll(resp.Body)
			log.Printf("[GigaChat] Forbidden (403) - possible token expiry or X-Client-ID issue")
			log.Printf("[GigaChat] 403 Details: URL=%s, Method=%s, X-Client-ID=%s, Response=%s",
				url, method, req.Header.Get("X-Client-ID"), string(data))
			resp.Body.Close()
			return nil, fmt.Errorf("gigachat forbidden (403): %s", string(data))
		}

		// Логируем успешные ответы
		log.Printf("[GigaChat] Success response: %d for %s %s", resp.StatusCode, method, url)

		// Успешный ответ или 401 (уже обработан)
		return resp, nil
	}

	// Если все попытки исчерпаны
	if lastErr != nil {
		return nil, fmt.Errorf("failed after %d retries: %w", maxRetries, lastErr)
	}
	return nil, fmt.Errorf("failed after %d retries: unknown error", maxRetries)
}

// isTimeoutError проверяет, является ли ошибка таймаутом
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}

	// Проверяем на context deadline exceeded
	if err == context.DeadlineExceeded {
		return true
	}

	// Проверяем на network timeout errors
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return true
	}

	// Проверяем на syscall timeout (ETIMEDOUT)
	if errno, ok := err.(syscall.Errno); ok && errno == syscall.ETIMEDOUT {
		return true
	}

	// Проверяем сообщение об ошибке
	errStr := err.Error()
	return strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "deadline exceeded") ||
		strings.Contains(errStr, "Client.Timeout exceeded")
}
