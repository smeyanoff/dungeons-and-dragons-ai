package cache

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	"dungeons-and-dragons-ai/pkg/logger"
)

// DMResponseCache кэширует ответы DM для похожих запросов
type DMResponseCache struct {
	cache map[string]*cacheEntry
	mu    sync.RWMutex
	ttl   time.Duration
}

type cacheEntry struct {
	response     string
	cachedAt     time.Time
	hitCount     int
	lastAccessed time.Time
}

// NewDMResponseCache создает новый кэш ответов DM
func NewDMResponseCache(ttl time.Duration) *DMResponseCache {
	c := &DMResponseCache{
		cache: make(map[string]*cacheEntry),
		ttl:   ttl,
	}

	// Запускаем горутину для очистки устаревших записей
	go c.cleanup()

	return c
}

// Get получает кэшированный ответ для запроса
func (c *DMResponseCache) Get(ctx context.Context, sessionID uint, gameContext string, playerMessage string) (string, bool) {
	key := c.generateKey(sessionID, gameContext, playerMessage)

	c.mu.RLock()
	entry, exists := c.cache[key]
	c.mu.RUnlock()

	if !exists {
		return "", false
	}

	// Проверяем, не истек ли кэш
	now := time.Now()
	if now.Sub(entry.cachedAt) > c.ttl {
		// Удаляем устаревшую запись
		c.mu.Lock()
		delete(c.cache, key)
		c.mu.Unlock()
		return "", false
	}

	// Обновляем статистику
	c.mu.Lock()
	entry.hitCount++
	entry.lastAccessed = now
	c.mu.Unlock()

	logger.Debug("DM response cache hit",
		logger.Uint("session_id", sessionID),
		logger.String("key", key[:16]+"..."),
		logger.Int("hit_count", entry.hitCount),
	)

	return entry.response, true
}

// Set сохраняет ответ DM в кэш
func (c *DMResponseCache) Set(ctx context.Context, sessionID uint, gameContext string, playerMessage string, response string) {
	key := c.generateKey(sessionID, gameContext, playerMessage)

	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache[key] = &cacheEntry{
		response:     response,
		cachedAt:     time.Now(),
		hitCount:     0,
		lastAccessed: time.Now(),
	}

	logger.Debug("DM response cached",
		logger.Uint("session_id", sessionID),
		logger.String("key", key[:16]+"..."),
	)
}

// generateKey генерирует ключ кэша на основе sessionID, контекста и сообщения игрока
func (c *DMResponseCache) generateKey(sessionID uint, gameContext string, playerMessage string) string {
	// Используем hash для нормализации похожих запросов
	// Берем только начало контекста (последние 500 символов) для нормализации похожих ситуаций
	contextSnippet := gameContext
	if len(contextSnippet) > 500 {
		contextSnippet = contextSnippet[len(contextSnippet)-500:]
	}

	// Нормализуем сообщение игрока (приводим к нижнему регистру, убираем лишние пробелы)
	normalizedMessage := normalizeMessage(playerMessage)

	data := fmt.Sprintf("%d|%s|%s", sessionID, contextSnippet, normalizedMessage)
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash)
}

// normalizeMessage нормализует сообщение для сравнения
func normalizeMessage(msg string) string {
	// Приводим к нижнему регистру и убираем лишние пробелы
	normalized := ""
	prevSpace := false
	for _, r := range msg {
		if r == ' ' || r == '\t' || r == '\n' {
			if !prevSpace {
				normalized += " "
				prevSpace = true
			}
		} else {
			if r >= 'A' && r <= 'Z' {
				r += 32 // К нижнему регистру
			}
			normalized += string(r)
			prevSpace = false
		}
	}
	return normalized
}

// cleanup периодически очищает устаревшие записи кэша
func (c *DMResponseCache) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for key, entry := range c.cache {
			if now.Sub(entry.cachedAt) > c.ttl {
				delete(c.cache, key)
				logger.Debug("Removed expired cache entry",
					logger.String("key", key[:16]+"..."),
				)
			}
		}
		c.mu.Unlock()

		logger.Debug("DM response cache cleanup completed",
			logger.Int("cache_size", len(c.cache)),
		)
	}
}

// GetStats возвращает статистику кэша
func (c *DMResponseCache) GetStats() (size int, totalHits int) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	size = len(c.cache)
	for _, entry := range c.cache {
		totalHits += entry.hitCount
	}
	return size, totalHits
}
