package infrastructure

import (
	"context"
	"sync"
	"time"
)

// LLMRateLimiter интерфейс для ограничения запросов к LLM
type LLMRateLimiter interface {
	// Wait ждет, пока не будет разрешено сделать запрос
	Wait(ctx context.Context) error
}

// SimpleLLMRateLimiter простая реализация rate limiter для LLM запросов
// Ограничивает частоту запросов, чтобы не ддосить модель
type SimpleLLMRateLimiter struct {
	mu       sync.Mutex
	lastCall time.Time
	minDelay time.Duration // Минимальная задержка между запросами
}

// NewSimpleLLMRateLimiter создает новый SimpleLLMRateLimiter
// minDelay - минимальная задержка между запросами (например, 2 секунды)
func NewSimpleLLMRateLimiter(minDelay time.Duration) *SimpleLLMRateLimiter {
	return &SimpleLLMRateLimiter{
		minDelay: minDelay,
	}
}

// Wait ждет, пока не будет разрешено сделать запрос
func (r *SimpleLLMRateLimiter) Wait(ctx context.Context) error {
	r.mu.Lock()

	now := time.Now()
	timeSinceLastCall := now.Sub(r.lastCall)

	// Если прошло достаточно времени, разрешаем сразу
	if timeSinceLastCall >= r.minDelay {
		r.lastCall = now
		r.mu.Unlock()
		return nil
	}

	// Нужно подождать
	waitTime := r.minDelay - timeSinceLastCall

	// Обновляем lastCall перед ожиданием, чтобы следующий запрос учел это ожидание
	r.lastCall = now.Add(waitTime)

	r.mu.Unlock()

	// Ждем
	select {
	case <-ctx.Done():
		r.mu.Lock()
		// Откатываем lastCall, если запрос был отменен
		r.lastCall = now
		r.mu.Unlock()
		return ctx.Err()
	case <-time.After(waitTime):
		r.mu.Lock()
		// Убеждаемся, что lastCall установлен правильно
		if r.lastCall.Before(now.Add(waitTime)) {
			r.lastCall = now.Add(waitTime)
		}
		r.mu.Unlock()
		return nil
	}
}
