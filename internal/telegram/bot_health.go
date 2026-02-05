package telegram

import (
	"context"
	"fmt"
	"time"
)

// HealthCheck проверяет подключение к Telegram API
func (b *Bot) HealthCheck(ctx context.Context) error {
	if b == nil || b.api == nil {
		return fmt.Errorf("telegram bot not initialized")
	}

	b.healthCheckMu.RLock()
	if time.Since(b.lastHealthCheck) < 30*time.Second {
		b.healthCheckMu.RUnlock()
		return nil
	}
	b.healthCheckMu.RUnlock()

	_, err := b.api.GetMe()
	if err != nil {
		return fmt.Errorf("telegram API health check failed: %w", err)
	}

	b.healthCheckMu.Lock()
	b.lastHealthCheck = time.Now()
	b.healthCheckMu.Unlock()

	return nil
}
