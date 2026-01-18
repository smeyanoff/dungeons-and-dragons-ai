package cache

import (
	"context"
	"testing"
	"time"
)

func TestNewDMResponseCache(t *testing.T) {
	ttl := 10 * time.Minute
	cache := NewDMResponseCache(ttl)

	if cache == nil {
		t.Fatal("Expected cache to be created, got nil")
	}

	if cache.ttl != ttl {
		t.Errorf("Expected TTL %v, got %v", ttl, cache.ttl)
	}

	if cache.cache == nil {
		t.Error("Expected cache map to be initialized")
	}

	// Даем время на запуск cleanup горутины
	time.Sleep(100 * time.Millisecond)
}

func TestDMResponseCache_Get_NotFound(t *testing.T) {
	cache := NewDMResponseCache(10 * time.Minute)
	ctx := context.Background()

	response, found := cache.Get(ctx, 1, "context", "message")
	if found {
		t.Error("Expected response not found, got found=true")
	}
	if response != "" {
		t.Errorf("Expected empty response, got %q", response)
	}
}

func TestDMResponseCache_SetAndGet(t *testing.T) {
	cache := NewDMResponseCache(10 * time.Minute)
	ctx := context.Background()

	sessionID := uint(1)
	gameContext := "test context"
	playerMessage := "test message"
	expectedResponse := "test response"

	cache.Set(ctx, sessionID, gameContext, playerMessage, expectedResponse)

	response, found := cache.Get(ctx, sessionID, gameContext, playerMessage)
	if !found {
		t.Error("Expected response to be found")
	}
	if response != expectedResponse {
		t.Errorf("Expected response %q, got %q", expectedResponse, response)
	}
}

func TestDMResponseCache_Expiration(t *testing.T) {
	ttl := 100 * time.Millisecond
	cache := NewDMResponseCache(ttl)
	ctx := context.Background()

	sessionID := uint(1)
	cache.Set(ctx, sessionID, "context", "message", "response")

	// Проверяем, что ответ есть сразу после сохранения
	_, found := cache.Get(ctx, sessionID, "context", "message")
	if !found {
		t.Error("Expected response to be found immediately after setting")
	}

	// Ждем истечения TTL
	time.Sleep(150 * time.Millisecond)

	// Проверяем, что ответ больше не найден
	_, found = cache.Get(ctx, sessionID, "context", "message")
	if found {
		t.Error("Expected response to be expired and not found")
	}
}

func TestDMResponseCache_KeyGeneration_Consistent(t *testing.T) {
	cache := NewDMResponseCache(10 * time.Minute)
	ctx := context.Background()

	sessionID := uint(1)
	gameContext := "test context"
	playerMessage := "test message"
	response := "test response"

	cache.Set(ctx, sessionID, gameContext, playerMessage, response)

	// Генерируем тот же ключ с теми же параметрами
	response1, found1 := cache.Get(ctx, sessionID, gameContext, playerMessage)

	// Проверяем с нормализованным сообщением (пробелы, регистр)
	// normalizeMessage приводит к нижнему регистру и нормализует пробелы, но не обрезает их
	response2, found2 := cache.Get(ctx, sessionID, gameContext, "TEST  MESSAGE")

	if found1 != found2 {
		t.Error("Expected both lookups to have same result (key normalization)")
	}

	if response1 != response2 {
		t.Error("Expected both lookups to return same response")
	}
}

func TestDMResponseCache_KeyGeneration_Different(t *testing.T) {
	cache := NewDMResponseCache(10 * time.Minute)
	ctx := context.Background()

	sessionID := uint(1)
	cache.Set(ctx, sessionID, "context1", "message1", "response1")
	cache.Set(ctx, sessionID, "context2", "message2", "response2")

	response1, found1 := cache.Get(ctx, sessionID, "context1", "message1")
	response2, found2 := cache.Get(ctx, sessionID, "context2", "message2")

	if !found1 || !found2 {
		t.Error("Expected both responses to be found")
	}

	if response1 == response2 {
		t.Error("Expected different responses for different keys")
	}
}

func TestDMResponseCache_ConcurrentAccess(t *testing.T) {
	cache := NewDMResponseCache(10 * time.Minute)
	ctx := context.Background()

	// Запускаем несколько горутин для параллельного доступа
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		sessionID := uint(i + 1)
		go func(id uint) {
			cache.Set(ctx, id, "context", "message", "response")
			_, _ = cache.Get(ctx, id, "context", "message")
			done <- true
		}(sessionID)
	}

	// Ждем завершения всех горутин
	for i := 0; i < 10; i++ {
		<-done
	}

	// Проверяем, что все записи сохранились
	for i := 0; i < 10; i++ {
		sessionID := uint(i + 1)
		_, found := cache.Get(ctx, sessionID, "context", "message")
		if !found {
			t.Errorf("Expected response for session %d to be found", sessionID)
		}
	}
}

func TestDMResponseCache_HitCount(t *testing.T) {
	cache := NewDMResponseCache(10 * time.Minute)
	ctx := context.Background()

	sessionID := uint(1)
	cache.Set(ctx, sessionID, "context", "message", "response")

	// Получаем несколько раз
	for i := 0; i < 5; i++ {
		_, _ = cache.Get(ctx, sessionID, "context", "message")
	}

	// Проверяем статистику
	size, totalHits := cache.GetStats()
	if size != 1 {
		t.Errorf("Expected cache size 1, got %d", size)
	}
	if totalHits != 5 {
		t.Errorf("Expected total hits 5, got %d", totalHits)
	}
}

func TestDMResponseCache_Cleanup(t *testing.T) {
	ttl := 200 * time.Millisecond
	cache := NewDMResponseCache(ttl)
	ctx := context.Background()

	// Добавляем несколько записей
	for i := 0; i < 5; i++ {
		cache.Set(ctx, uint(i+1), "context", "message", "response")
	}

	// Проверяем размер
	size, _ := cache.GetStats()
	if size != 5 {
		t.Errorf("Expected cache size 5, got %d", size)
	}

	// Ждем истечения TTL + время на cleanup (cleanup работает каждые 5 минут, но записи удаляются при Get)
	// Поэтому проверяем удаление через Get вместо ожидания cleanup
	time.Sleep(250 * time.Millisecond)

	// Проверяем, что записи истекли через Get (который удаляет истекшие)
	for i := 0; i < 5; i++ {
		_, found := cache.Get(ctx, uint(i+1), "context", "message")
		if found {
			t.Errorf("Expected entry %d to be expired", i+1)
		}
	}
}

func TestDMResponseCache_ContextSnippet(t *testing.T) {
	cache := NewDMResponseCache(10 * time.Minute)
	ctx := context.Background()

	sessionID := uint(1)
	// Создаем длинный контекст (>500 символов)
	longContext := make([]rune, 600)
	for i := range longContext {
		longContext[i] = 'a'
	}
	longContextStr := string(longContext)

	cache.Set(ctx, sessionID, longContextStr, "message", "response")

	// Проверяем, что ответ все равно можно получить
	_, found := cache.Get(ctx, sessionID, longContextStr, "message")
	if !found {
		t.Error("Expected response to be found even with long context")
	}
}

func TestNormalizeMessage(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"TEST", "test"},
		{"  test  message  ", " test message "}, // normalizeMessage не обрезает пробелы по краям
		{"Test\nMessage\tWith\t\tSpaces", "test message with spaces"},
		{"Mixed   Case   String", "mixed case string"},
		{"", ""},
		{"   ", " "},
	}

	for _, tc := range testCases {
		result := normalizeMessage(tc.input)
		if result != tc.expected {
			t.Errorf("normalizeMessage(%q) = %q, expected %q", tc.input, result, tc.expected)
		}
	}
}

func TestDMResponseCache_DifferentSessions(t *testing.T) {
	cache := NewDMResponseCache(10 * time.Minute)
	ctx := context.Background()

	cache.Set(ctx, 1, "context", "message", "response1")
	cache.Set(ctx, 2, "context", "message", "response2")

	response1, found1 := cache.Get(ctx, 1, "context", "message")
	response2, found2 := cache.Get(ctx, 2, "context", "message")

	if !found1 || !found2 {
		t.Error("Expected both responses to be found")
	}

	if response1 == response2 {
		t.Error("Expected different responses for different sessions")
	}
}
