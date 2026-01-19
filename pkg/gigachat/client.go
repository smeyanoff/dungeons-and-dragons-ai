package gigachat

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
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
		return nil, err
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

		return c.client.Do(req)
	}

	return resp, nil
}
