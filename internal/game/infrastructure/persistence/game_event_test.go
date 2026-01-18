package persistence

import (
	"context"
	"fmt"
	"testing"
	"time"

	"dungeons-and-dragons-ai/internal/game/domain/event"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/game/domain/world"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupGameEventTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	// Автомиграции
	err = db.AutoMigrate(
		&session.GameSession{},
		&world.World{},
		&event.StoryEvent{},
	)
	if err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	return db
}

func TestGameEventRepository_Save(t *testing.T) {
	db := setupGameEventTestDB(t)
	repo := NewGameEventRepository(db)
	ctx := context.Background()

	t.Run("saves new event", func(t *testing.T) {
		// Создаем сессию
		w := &world.World{Name: "Test World", Description: "Test"}
		if err := db.Create(w).Error; err != nil {
			t.Fatalf("Failed to create world: %v", err)
		}

		gs := &session.GameSession{ChatID: 12345, State: session.StateActive, WorldID: w.ID}
		if err := db.Create(gs).Error; err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}

		e := &event.StoryEvent{
			GameSessionID: gs.ID,
			AuthorType:    event.AuthorTypePlayer,
			AuthorName:    "Test Player",
			Content:       "Test event content",
		}

		err := repo.Save(ctx, e)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if e.ID == 0 {
			t.Fatal("Expected ID to be set")
		}

		// Проверяем, что событие сохранено
		var saved event.StoryEvent
		if err := db.First(&saved, e.ID).Error; err != nil {
			t.Fatalf("Failed to find saved event: %v", err)
		}
		if saved.AuthorType != event.AuthorTypePlayer {
			t.Fatalf("Expected AuthorType Player, got: %s", saved.AuthorType)
		}
		if saved.AuthorName != "Test Player" {
			t.Fatalf("Expected AuthorName 'Test Player', got: %s", saved.AuthorName)
		}
		if saved.Content != "Test event content" {
			t.Fatalf("Expected Content 'Test event content', got: %s", saved.Content)
		}
	})

	t.Run("saves event with different author types", func(t *testing.T) {
		// Создаем сессию
		w := &world.World{Name: "Test World 2", Description: "Test 2"}
		if err := db.Create(w).Error; err != nil {
			t.Fatalf("Failed to create world: %v", err)
		}

		gs := &session.GameSession{ChatID: 67890, State: session.StateActive, WorldID: w.ID}
		if err := db.Create(gs).Error; err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}

		// Тестируем разные типы авторов
		testCases := []struct {
			authorType event.AuthorType
			authorName string
			content    string
		}{
			{event.AuthorTypeDM, "Dungeon Master", "DM event"},
			{event.AuthorTypeNPC, "NPC Name", "NPC event"},
			{event.AuthorTypePlayer, "Player Name", "Player event"},
		}

		for _, tc := range testCases {
			e := &event.StoryEvent{
				GameSessionID: gs.ID,
				AuthorType:    tc.authorType,
				AuthorName:    tc.authorName,
				Content:       tc.content,
			}

			err := repo.Save(ctx, e)
			if err != nil {
				t.Fatalf("Expected no error for %s, got: %v", tc.authorType, err)
			}

			var saved event.StoryEvent
			if err := db.First(&saved, e.ID).Error; err != nil {
				t.Fatalf("Failed to find saved event: %v", err)
			}
			if saved.AuthorType != tc.authorType {
				t.Fatalf("Expected AuthorType %s, got: %s", tc.authorType, saved.AuthorType)
			}
			if saved.AuthorName != tc.authorName {
				t.Fatalf("Expected AuthorName '%s', got: '%s'", tc.authorName, saved.AuthorName)
			}
		}
	})
}

func TestGameEventRepository_GetBySessionID(t *testing.T) {
	db := setupGameEventTestDB(t)
	repo := NewGameEventRepository(db)
	ctx := context.Background()

	t.Run("returns empty slice when no events found", func(t *testing.T) {
		// Создаем сессию
		w := &world.World{Name: "Empty World", Description: "Empty"}
		if err := db.Create(w).Error; err != nil {
			t.Fatalf("Failed to create world: %v", err)
		}

		gs := &session.GameSession{ChatID: 11111, State: session.StateActive, WorldID: w.ID}
		if err := db.Create(gs).Error; err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}

		result, err := repo.GetBySessionID(ctx, gs.ID, 10)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if len(result) != 0 {
			t.Fatalf("Expected empty slice, got: %d events", len(result))
		}
	})

	t.Run("returns events in chronological order", func(t *testing.T) {
		// Создаем сессию
		w := &world.World{Name: "Chronological World", Description: "Chronological"}
		if err := db.Create(w).Error; err != nil {
			t.Fatalf("Failed to create world: %v", err)
		}

		gs := &session.GameSession{ChatID: 22222, State: session.StateActive, WorldID: w.ID}
		if err := db.Create(gs).Error; err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}

		// Создаем события с задержкой, чтобы они имели разные временные метки
		events := []*event.StoryEvent{
			{GameSessionID: gs.ID, AuthorType: event.AuthorTypeDM, AuthorName: "DM", Content: "Event 1"},
			{GameSessionID: gs.ID, AuthorType: event.AuthorTypePlayer, AuthorName: "Player", Content: "Event 2"},
			{GameSessionID: gs.ID, AuthorType: event.AuthorTypeNPC, AuthorName: "NPC", Content: "Event 3"},
		}

		for i, e := range events {
			if err := repo.Save(ctx, e); err != nil {
				t.Fatalf("Failed to save event %d: %v", i+1, err)
			}
			// Небольшая задержка для разных временных меток
			time.Sleep(10 * time.Millisecond)
		}

		// Получаем события
		result, err := repo.GetBySessionID(ctx, gs.ID, 10)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if len(result) != 3 {
			t.Fatalf("Expected 3 events, got: %d", len(result))
		}

		// Проверяем хронологический порядок (от старых к новым)
		if result[0].Content != "Event 1" {
			t.Fatalf("Expected first event to be 'Event 1', got: '%s'", result[0].Content)
		}
		if result[1].Content != "Event 2" {
			t.Fatalf("Expected second event to be 'Event 2', got: '%s'", result[1].Content)
		}
		if result[2].Content != "Event 3" {
			t.Fatalf("Expected third event to be 'Event 3', got: '%s'", result[2].Content)
		}
	})

	t.Run("respects limit", func(t *testing.T) {
		// Создаем сессию
		w := &world.World{Name: "Limit World", Description: "Limit"}
		if err := db.Create(w).Error; err != nil {
			t.Fatalf("Failed to create world: %v", err)
		}

		gs := &session.GameSession{ChatID: 33333, State: session.StateActive, WorldID: w.ID}
		if err := db.Create(gs).Error; err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}

		// Создаем 5 событий
		for i := 1; i <= 5; i++ {
			e := &event.StoryEvent{
				GameSessionID: gs.ID,
				AuthorType:    event.AuthorTypeDM,
				AuthorName:    "DM",
				Content:       fmt.Sprintf("Event %d", i),
			}
			if err := repo.Save(ctx, e); err != nil {
				t.Fatalf("Failed to save event %d: %v", i, err)
			}
			time.Sleep(10 * time.Millisecond)
		}

		// Получаем только последние 3 события
		result, err := repo.GetBySessionID(ctx, gs.ID, 3)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if len(result) != 3 {
			t.Fatalf("Expected 3 events, got: %d", len(result))
		}

		// Проверяем, что получили последние 3 события
		if result[0].Content != "Event 3" {
			t.Fatalf("Expected first event to be 'Event 3', got: '%s'", result[0].Content)
		}
		if result[1].Content != "Event 4" {
			t.Fatalf("Expected second event to be 'Event 4', got: '%s'", result[1].Content)
		}
		if result[2].Content != "Event 5" {
			t.Fatalf("Expected third event to be 'Event 5', got: '%s'", result[2].Content)
		}
	})

	t.Run("returns only events for specified session", func(t *testing.T) {
		// Создаем две сессии
		w1 := &world.World{Name: "Session 1 World", Description: "Session 1"}
		if err := db.Create(w1).Error; err != nil {
			t.Fatalf("Failed to create world: %v", err)
		}

		w2 := &world.World{Name: "Session 2 World", Description: "Session 2"}
		if err := db.Create(w2).Error; err != nil {
			t.Fatalf("Failed to create world: %v", err)
		}

		gs1 := &session.GameSession{ChatID: 44444, State: session.StateActive, WorldID: w1.ID}
		if err := db.Create(gs1).Error; err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}

		gs2 := &session.GameSession{ChatID: 55555, State: session.StateActive, WorldID: w2.ID}
		if err := db.Create(gs2).Error; err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}

		// Создаем события для первой сессии
		for i := 1; i <= 3; i++ {
			e := &event.StoryEvent{
				GameSessionID: gs1.ID,
				AuthorType:    event.AuthorTypeDM,
				AuthorName:    "DM",
				Content:       fmt.Sprintf("Session 1 Event %d", i),
			}
			if err := repo.Save(ctx, e); err != nil {
				t.Fatalf("Failed to save event: %v", err)
			}
		}

		// Создаем события для второй сессии
		for i := 1; i <= 2; i++ {
			e := &event.StoryEvent{
				GameSessionID: gs2.ID,
				AuthorType:    event.AuthorTypeDM,
				AuthorName:    "DM",
				Content:       fmt.Sprintf("Session 2 Event %d", i),
			}
			if err := repo.Save(ctx, e); err != nil {
				t.Fatalf("Failed to save event: %v", err)
			}
		}

		// Получаем события только для первой сессии
		result, err := repo.GetBySessionID(ctx, gs1.ID, 10)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if len(result) != 3 {
			t.Fatalf("Expected 3 events for session 1, got: %d", len(result))
		}

		// Проверяем, что все события принадлежат первой сессии
		for _, e := range result {
			if e.GameSessionID != gs1.ID {
				t.Fatalf("Expected GameSessionID %d, got: %d", gs1.ID, e.GameSessionID)
			}
		}
	})
}

func TestGameEventRepository_GetAllBySessionID(t *testing.T) {
	db := setupGameEventTestDB(t)
	repo := NewGameEventRepository(db)
	ctx := context.Background()

	t.Run("returns all events in ascending order", func(t *testing.T) {
		// Создаем сессию
		w := &world.World{Name: "All Events World", Description: "All Events"}
		if err := db.Create(w).Error; err != nil {
			t.Fatalf("Failed to create world: %v", err)
		}

		gs := &session.GameSession{ChatID: 66666, State: session.StateActive, WorldID: w.ID}
		if err := db.Create(gs).Error; err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}

		// Создаем события
		for i := 1; i <= 5; i++ {
			e := &event.StoryEvent{
				GameSessionID: gs.ID,
				AuthorType:    event.AuthorTypeDM,
				AuthorName:    "DM",
				Content:       fmt.Sprintf("All Event %d", i),
			}
			if err := repo.Save(ctx, e); err != nil {
				t.Fatalf("Failed to save event %d: %v", i, err)
			}
			time.Sleep(10 * time.Millisecond)
		}

		// Получаем все события
		result, err := repo.GetAllBySessionID(ctx, gs.ID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if len(result) != 5 {
			t.Fatalf("Expected 5 events, got: %d", len(result))
		}

		// Проверяем порядок (от старых к новым)
		for i, e := range result {
			expected := fmt.Sprintf("All Event %d", i+1)
			if e.Content != expected {
				t.Fatalf("Expected event %d to be '%s', got: '%s'", i+1, expected, e.Content)
			}
		}
	})
}

func TestGameEventRepository_ContextTimeout(t *testing.T) {
	db := setupGameEventTestDB(t)
	repo := NewGameEventRepository(db)

	t.Run("respects context timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()

		// Даем контексту время истечь
		time.Sleep(10 * time.Millisecond)

		_, err := repo.GetBySessionID(ctx, 1, 10)
		if err == nil {
			t.Fatal("Expected context deadline exceeded error")
		}
		if ctx.Err() == nil {
			t.Fatal("Expected context to be cancelled")
		}
	})
}
