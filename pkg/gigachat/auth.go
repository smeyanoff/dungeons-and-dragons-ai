package gigachat

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresAt   int64  `json:"expires_at"` // unix timestamp
}

type authClient struct {
	cfg    Config
	token  *TokenResponse
	mu     sync.RWMutex
	client *http.Client
}

func newAuthClient(cfg Config) *authClient {
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
	
	return &authClient{
		cfg: cfg,
		client: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		},
	}
}

// getToken получает новый токен и кэширует его.
// Автоматически перезапрашивает токен при истечении (expires_at).
func (a *authClient) getToken(ctx context.Context) (string, error) {
	a.mu.RLock()
	// Проверяем, есть ли валидный токен
	if a.token != nil {
		// Проверяем expires_at (оставляем запас 60 секунд)
		now := time.Now().Unix()
		if a.token.ExpiresAt > 0 {
			// Токен валиден, если до истечения остается больше 60 секунд
			timeUntilExpiry := a.token.ExpiresAt - now
			if timeUntilExpiry > 60 {
				token := a.token.AccessToken
				a.mu.RUnlock()
				return token, nil
			}
			// Токен истекает или истек, логируем это
			if timeUntilExpiry > 0 {
				log.Printf("GigaChat token expires soon (in %d seconds), refreshing...", timeUntilExpiry)
			} else {
				log.Printf("GigaChat token expired (%d seconds ago), refreshing...", -timeUntilExpiry)
			}
		} else {
			// expires_at не установлен, используем токен (но лучше перезапросить)
			log.Printf("GigaChat token has no expires_at, using cached token (may be invalid)")
			token := a.token.AccessToken
			a.mu.RUnlock()
			return token, nil
		}
	}
	a.mu.RUnlock()

	// Токена нет или он истек, получаем новый
	a.mu.Lock()
	defer a.mu.Unlock()

	// Двойная проверка (double-check locking)
	now := time.Now().Unix()
	if a.token != nil && a.token.ExpiresAt > 0 {
		timeUntilExpiry := a.token.ExpiresAt - now
		if timeUntilExpiry > 60 {
			return a.token.AccessToken, nil
		}
	}

	// Логируем перезапрос токена
	if a.token != nil {
		log.Printf("GigaChat: Requesting new token (old token expired or expiring soon)")
	} else {
		log.Printf("GigaChat: Requesting new token (no cached token)")
	}

	// Валидация credentials перед запросом
	if a.cfg.ClientID == "" || a.cfg.ClientSecret == "" {
		return "", fmt.Errorf("gigachat credentials are empty: ClientID or ClientSecret is missing")
	}

	if a.cfg.Scope == "" {
		return "", fmt.Errorf("gigachat scope is empty")
	}

	// Обрезаем пробелы и переносы строк из credentials (могут появиться при чтении из .env)
	clientID := strings.TrimSpace(a.cfg.ClientID)
	clientSecret := strings.TrimSpace(a.cfg.ClientSecret)
	scope := strings.TrimSpace(a.cfg.Scope)

	// Согласно документации GigaChat API, CLIENT_SECRET в .env уже содержит
	// base64-закодированную строку формата "client_id:real_secret"
	// Поэтому используем CLIENT_SECRET напрямую как ключ авторизации
	authHeader := clientSecret
	
	// Логируем информацию для диагностики (без самих credentials)
	log.Printf("GigaChat auth: ClientID length=%d, ClientSecret (auth key) length=%d",
		len(clientID), len(authHeader))

	// Пытаемся получить токен с retry механизмом
	token, err := a.getTokenWithRetry(ctx, clientID, authHeader, scope)
	if err != nil {
		return "", err
	}

	// Сохраняем токен в кэш
	a.token = token

	// Логируем успешное получение токена с информацией об истечении
	if token.ExpiresAt > 0 {
		now := time.Now().Unix()
		// Безопасное вычисление разницы времени (может быть отрицательным, если токен уже истек)
		expiresIn := token.ExpiresAt - now
		log.Printf("GigaChat: New token obtained, expires in %d seconds (at %s)",
			expiresIn, time.Unix(token.ExpiresAt, 0).Format(time.RFC3339))
	} else {
		log.Printf("GigaChat: New token obtained (expires_at not provided by API)")
	}

	return token.AccessToken, nil
}

// getTokenWithRetry получает токен с retry механизмом и exponential backoff
// Retry применяется только для временных ошибок (400, 401, 429) и сетевых ошибок
func (a *authClient) getTokenWithRetry(ctx context.Context, clientID, authHeader, scope string) (*TokenResponse, error) {
	const maxRetries = 3
	const initialBackoff = 1 * time.Second
	const maxBackoff = 10 * time.Second

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Вычисляем exponential backoff: 1s, 2s, 4s
			// Проверяем на overflow перед конвертацией int -> uint
			// attempt-1 всегда >= 0, так как attempt > 0
			shift := attempt - 1
			// Защита от overflow: ограничиваем сдвиг безопасными пределами (максимум 30 для time.Duration)
			// time.Duration это int64, поэтому 1<<30 безопасно
			const maxSafeShift = 30
			if shift < 0 {
				shift = 0
			} else if shift > maxSafeShift {
				shift = maxSafeShift
			}
			// #nosec G115 - защита от overflow реализована выше: shift ограничен до maxSafeShift=30
			// что безопасно для int64/time.Duration (максимальный безопасный сдвиг для int64)
			backoff := initialBackoff * time.Duration(1<<uint(shift))
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			log.Printf("GigaChat auth: Retry attempt %d/%d after %v...", attempt, maxRetries, backoff)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		token, err := a.requestToken(ctx, clientID, authHeader, scope)
		if err == nil {
			return token, nil
		}

		lastErr = err

		// Проверяем, стоит ли повторять попытку
		// Retry для: сетевых ошибок, 400 (Bad Request - может быть временная проблема),
		// 401 (Unauthorized - может быть проблема с форматом), 429 (Too Many Requests)
		errStr := err.Error()
		shouldRetry := false
		if strings.Contains(errStr, "context deadline exceeded") ||
			strings.Contains(errStr, "timeout") ||
			strings.Contains(errStr, "connection") ||
			strings.Contains(errStr, "status: 400") ||
			strings.Contains(errStr, "status: 401") ||
			strings.Contains(errStr, "status: 429") {
			shouldRetry = true
		}

		if !shouldRetry || attempt >= maxRetries {
			// Не retry или исчерпаны попытки
			if attempt >= maxRetries {
				log.Printf("GigaChat auth: Max retries (%d) exceeded", maxRetries)
			}
			break
		}
	}

	return nil, lastErr
}

// requestToken выполняет один запрос на получение токена
func (a *authClient) requestToken(ctx context.Context, clientID, authHeader, scope string) (*TokenResponse, error) {
	// Генерируем уникальный идентификатор запроса (RqUID) в формате UUID v4
	// Это обязательный заголовок согласно документации GigaChat API
	rqUID := uuid.New().String()

	form := url.Values{}
	form.Set("scope", scope)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api/v2/oauth", a.cfg.AuthBaseURL),
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return nil, err
	}

	// Устанавливаем обязательные заголовки согласно документации GigaChat API
	req.Header.Set("Authorization", "Basic "+authHeader)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("RqUID", rqUID) // Обязательный заголовок - уникальный идентификатор запроса в формате UUID v4

	// Логируем детали запроса для диагностики (без credentials)
	log.Printf("GigaChat auth request: Method=%s, URL=%s, Scope=%s, RqUID=%s, Content-Type=%s, Authorization header present=%v",
		req.Method, req.URL.String(), scope, rqUID, req.Header.Get("Content-Type"), req.Header.Get("Authorization") != "")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("network error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		// Читаем тело ответа для диагностики ошибки
		bodyBytes, readErr := io.ReadAll(resp.Body)
		bodyStr := ""
		if readErr == nil {
			if len(bodyBytes) > 0 {
				bodyStr = string(bodyBytes)
			}
		} else {
			log.Printf("GigaChat auth error: failed to read response body: %v", readErr)
		}

		// Логируем заголовки ответа для диагностики
		log.Printf("GigaChat auth error: Response headers: %v", resp.Header)

		// Пытаемся распарсить JSON ошибку для более понятного сообщения
		var errorDetail string
		if bodyStr != "" {
			// Всегда логируем полное тело ответа для диагностики
			log.Printf("GigaChat auth error: Full response body (%d bytes): %s", len(bodyBytes), bodyStr)

			// Проверяем, это JSON или просто текст
			trimmedBody := strings.TrimSpace(bodyStr)
			if strings.HasPrefix(trimmedBody, "{") || strings.HasPrefix(trimmedBody, "[") {
				var errorJSON map[string]interface{}
				if err := json.Unmarshal(bodyBytes, &errorJSON); err == nil {
					// Форматируем JSON ошибку
					if msg, ok := errorJSON["message"].(string); ok {
						errorDetail = msg
					} else if errDesc, ok := errorJSON["error_description"].(string); ok {
						errorDetail = errDesc
					} else if errMsg, ok := errorJSON["error"].(string); ok {
						errorDetail = errMsg
					} else {
						// Если не нашли стандартные поля, используем весь JSON
						errorDetail = bodyStr
					}
				} else {
					log.Printf("GigaChat auth error: Failed to parse JSON response: %v", err)
					errorDetail = bodyStr
				}
			} else {
				// Не JSON, используем как есть
				errorDetail = bodyStr
			}
		} else {
			log.Printf("GigaChat auth error: Response body is empty")
		}

		// Логируем детали ошибки (без credentials)
		log.Printf("GigaChat auth error: %s (status: %d). URL: %s, Scope: %s, ClientID length: %d",
			resp.Status, resp.StatusCode, req.URL.String(), a.cfg.Scope, len(a.cfg.ClientID))

		// Возвращаем ошибку с деталями
		if errorDetail != "" {
			return nil, fmt.Errorf("gigachat auth error: %s - %s", resp.Status, errorDetail)
		}
		return nil, fmt.Errorf("gigachat auth error: %s (empty response body)", resp.Status)
	}

	var token TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	return &token, nil
}

// invalidateToken инвалидирует текущий токен, заставляя перезапросить его при следующем вызове
func (a *authClient) invalidateToken() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.token != nil {
		log.Printf("GigaChat: Invalidating cached token")
		a.token = nil
	}
}
