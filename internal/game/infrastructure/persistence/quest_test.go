package persistence

import (
	"context"
	"testing"
	"time"

	"dungeons-and-dragons-ai/internal/game/domain/item"
	"dungeons-and-dragons-ai/internal/game/domain/quest"
	"dungeons-and-dragons-ai/internal/game/domain/world"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupQuestTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	// Автомиграции
	err = db.AutoMigrate(
		&world.World{},
		&quest.Quest{},
		&item.Item{},
	)
	if err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	return db
}

func TestQuestRepository_GetByWorldID(t *testing.T) {
	db := setupQuestTestDB(t)
	repo := NewQuestRepository(db)
	ctx := context.Background()

	t.Run("returns empty slice when no quests found", func(t *testing.T) {
		// Создаем мир
		w := &world.World{Name: "Empty World", Description: "Empty"}
		if err := db.Create(w).Error; err != nil {
			t.Fatalf("Failed to create world: %v", err)
		}

		result, err := repo.GetByWorldID(ctx, w.ID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if len(result) != 0 {
			t.Fatalf("Expected empty slice, got: %d quests", len(result))
		}
	})

	t.Run("returns quests for world", func(t *testing.T) {
		// Создаем мир
		w := &world.World{Name: "Quest World", Description: "Quest"}
		if err := db.Create(w).Error; err != nil {
			t.Fatalf("Failed to create world: %v", err)
		}

		// Создаем квесты
		q1 := quest.New("Quest 1", "Description 1")
		q1.WorldID = w.ID
		if err := db.Create(q1).Error; err != nil {
			t.Fatalf("Failed to create quest: %v", err)
		}

		q2 := quest.New("Quest 2", "Description 2")
		q2.WorldID = w.ID
		if err := db.Create(q2).Error; err != nil {
			t.Fatalf("Failed to create quest: %v", err)
		}

		// Получаем квесты
		result, err := repo.GetByWorldID(ctx, w.ID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if len(result) != 2 {
			t.Fatalf("Expected 2 quests, got: %d", len(result))
		}

		// Проверяем, что все квесты принадлежат миру
		for _, q := range result {
			if q.WorldID != w.ID {
				t.Fatalf("Expected WorldID %d, got: %d", w.ID, q.WorldID)
			}
		}
	})

	t.Run("returns only quests for specified world", func(t *testing.T) {
		// Создаем два мира
		w1 := &world.World{Name: "World 1", Description: "World 1"}
		if err := db.Create(w1).Error; err != nil {
			t.Fatalf("Failed to create world: %v", err)
		}

		w2 := &world.World{Name: "World 2", Description: "World 2"}
		if err := db.Create(w2).Error; err != nil {
			t.Fatalf("Failed to create world: %v", err)
		}

		// Создаем квесты для первого мира
		q1 := quest.New("World 1 Quest 1", "Description")
		q1.WorldID = w1.ID
		if err := db.Create(q1).Error; err != nil {
			t.Fatalf("Failed to create quest: %v", err)
		}

		q2 := quest.New("World 1 Quest 2", "Description")
		q2.WorldID = w1.ID
		if err := db.Create(q2).Error; err != nil {
			t.Fatalf("Failed to create quest: %v", err)
		}

		// Создаем квест для второго мира
		q3 := quest.New("World 2 Quest 1", "Description")
		q3.WorldID = w2.ID
		if err := db.Create(q3).Error; err != nil {
			t.Fatalf("Failed to create quest: %v", err)
		}

		// Получаем квесты для первого мира
		result, err := repo.GetByWorldID(ctx, w1.ID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if len(result) != 2 {
			t.Fatalf("Expected 2 quests for world 1, got: %d", len(result))
		}

		// Проверяем, что все квесты принадлежат первому миру
		for _, q := range result {
			if q.WorldID != w1.ID {
				t.Fatalf("Expected WorldID %d, got: %d", w1.ID, q.WorldID)
			}
		}
	})

	t.Run("returns quests with items", func(t *testing.T) {
		// Создаем мир
		w := &world.World{Name: "Items World", Description: "Items"}
		if err := db.Create(w).Error; err != nil {
			t.Fatalf("Failed to create world: %v", err)
		}

		// Создаем предметы
		item1 := &item.Item{Name: "Item 1"}
		if err := db.Create(item1).Error; err != nil {
			t.Fatalf("Failed to create item: %v", err)
		}

		item2 := &item.Item{Name: "Item 2"}
		if err := db.Create(item2).Error; err != nil {
			t.Fatalf("Failed to create item: %v", err)
		}

		// Создаем квест с предметами
		q := quest.New("Quest with Items", "Description")
		q.WorldID = w.ID
		q.AddItem(item1)
		q.AddItem(item2)
		if err := db.Create(q).Error; err != nil {
			t.Fatalf("Failed to create quest: %v", err)
		}

		// Получаем квесты
		result, err := repo.GetByWorldID(ctx, w.ID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("Expected 1 quest, got: %d", len(result))
		}
		if len(result[0].Items) != 2 {
			t.Fatalf("Expected 2 items, got: %d", len(result[0].Items))
		}
	})
}

func TestQuestRepository_Save(t *testing.T) {
	db := setupQuestTestDB(t)
	repo := NewQuestRepository(db)
	ctx := context.Background()

	t.Run("saves new quest", func(t *testing.T) {
		// Создаем мир
		w := &world.World{Name: "Save World", Description: "Save"}
		if err := db.Create(w).Error; err != nil {
			t.Fatalf("Failed to create world: %v", err)
		}

		q := quest.New("Save Quest", "Save Description")
		q.WorldID = w.ID
		q.SetExperienceReward(200)

		err := repo.Save(ctx, q)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if q.ID == 0 {
			t.Fatal("Expected ID to be set")
		}

		// Проверяем, что квест сохранен
		var saved quest.Quest
		if err := db.First(&saved, q.ID).Error; err != nil {
			t.Fatalf("Failed to find saved quest: %v", err)
		}
		if saved.Title != "Save Quest" {
			t.Fatalf("Expected Title 'Save Quest', got: '%s'", saved.Title)
		}
		if saved.ExperienceReward != 200 {
			t.Fatalf("Expected ExperienceReward 200, got: %d", saved.ExperienceReward)
		}
	})

	t.Run("updates existing quest", func(t *testing.T) {
		// Создаем мир
		w := &world.World{Name: "Update World", Description: "Update"}
		if err := db.Create(w).Error; err != nil {
			t.Fatalf("Failed to create world: %v", err)
		}

		q := quest.New("Update Quest", "Update Description")
		q.WorldID = w.ID
		if err := db.Create(q).Error; err != nil {
			t.Fatalf("Failed to create quest: %v", err)
		}

		// Обновляем квест
		q.Complete()
		q.SetExperienceReward(300)
		err := repo.Save(ctx, q)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Проверяем обновление
		var updated quest.Quest
		if err := db.First(&updated, q.ID).Error; err != nil {
			t.Fatalf("Failed to find updated quest: %v", err)
		}
		if updated.Status != quest.QuestStatusCompleted {
			t.Fatalf("Expected Status Completed, got: %s", updated.Status)
		}
		if updated.ExperienceReward != 300 {
			t.Fatalf("Expected ExperienceReward 300, got: %d", updated.ExperienceReward)
		}
	})

	t.Run("saves quest with items", func(t *testing.T) {
		// Создаем мир
		w := &world.World{Name: "Items World", Description: "Items"}
		if err := db.Create(w).Error; err != nil {
			t.Fatalf("Failed to create world: %v", err)
		}

		// Создаем предметы
		item1 := &item.Item{Name: "Reward Item 1"}
		if err := db.Create(item1).Error; err != nil {
			t.Fatalf("Failed to create item: %v", err)
		}

		item2 := &item.Item{Name: "Reward Item 2"}
		if err := db.Create(item2).Error; err != nil {
			t.Fatalf("Failed to create item: %v", err)
		}

		q := quest.New("Quest with Items", "Description")
		q.WorldID = w.ID
		q.AddItem(item1)
		q.AddItem(item2)

		err := repo.Save(ctx, q)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Проверяем сохранение
		var saved quest.Quest
		if err := db.Preload("Items").First(&saved, q.ID).Error; err != nil {
			t.Fatalf("Failed to find saved quest: %v", err)
		}
		if len(saved.Items) != 2 {
			t.Fatalf("Expected 2 items, got: %d", len(saved.Items))
		}
	})
}

func TestQuestRepository_GetByID(t *testing.T) {
	db := setupQuestTestDB(t)
	repo := NewQuestRepository(db)
	ctx := context.Background()

	t.Run("returns nil when quest not found", func(t *testing.T) {
		result, err := repo.GetByID(ctx, 99999)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if result != nil {
			t.Fatalf("Expected nil, got: %v", result)
		}
	})

	t.Run("returns quest with items", func(t *testing.T) {
		// Создаем мир
		w := &world.World{Name: "Get World", Description: "Get"}
		if err := db.Create(w).Error; err != nil {
			t.Fatalf("Failed to create world: %v", err)
		}

		// Создаем предмет
		item1 := &item.Item{Name: "Get Item"}
		if err := db.Create(item1).Error; err != nil {
			t.Fatalf("Failed to create item: %v", err)
		}

		// Создаем квест
		q := quest.New("Get Quest", "Get Description")
		q.WorldID = w.ID
		q.AddItem(item1)
		if err := db.Create(q).Error; err != nil {
			t.Fatalf("Failed to create quest: %v", err)
		}

		// Тестируем получение
		result, err := repo.GetByID(ctx, q.ID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if result == nil {
			t.Fatal("Expected quest, got nil")
		}
		if result.ID != q.ID {
			t.Fatalf("Expected ID %d, got: %d", q.ID, result.ID)
		}
		if result.Title != "Get Quest" {
			t.Fatalf("Expected Title 'Get Quest', got: '%s'", result.Title)
		}
		if len(result.Items) != 1 {
			t.Fatalf("Expected 1 item, got: %d", len(result.Items))
		}
		if result.Items[0].Name != "Get Item" {
			t.Fatalf("Expected Item.Name 'Get Item', got: '%s'", result.Items[0].Name)
		}
	})
}

func TestQuestRepository_ContextTimeout(t *testing.T) {
	db := setupQuestTestDB(t)
	repo := NewQuestRepository(db)

	t.Run("respects context timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()

		// Даем контексту время истечь
		time.Sleep(10 * time.Millisecond)

		_, err := repo.GetByWorldID(ctx, 1)
		if err == nil {
			t.Fatal("Expected context deadline exceeded error")
		}
		if ctx.Err() == nil {
			t.Fatal("Expected context to be cancelled")
		}
	})
}
