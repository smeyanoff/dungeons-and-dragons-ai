package persistence

import (
	"context"
	"testing"
	"time"

	"dungeons-and-dragons-ai/internal/game/domain/quest"
	"dungeons-and-dragons-ai/internal/game/domain/world"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupWorldTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	// Автомиграции
	err = db.AutoMigrate(
		&world.World{},
		&world.Location{},
		&world.LocationConnection{},
		&world.NPC{},
		&world.Monster{},
		&quest.Quest{},
	)
	if err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	return db
}

func TestWorldRepository_Save(t *testing.T) {
	db := setupWorldTestDB(t)
	repo := NewWorldRepository(db)
	ctx := context.Background()

	t.Run("saves new world", func(t *testing.T) {
		w := world.New("Test World")
		w.Description = "Test Description"

		err := repo.Save(ctx, w)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if w.ID == 0 {
			t.Fatal("Expected ID to be set")
		}

		// Проверяем, что мир сохранен
		var saved world.World
		if err := db.First(&saved, w.ID).Error; err != nil {
			t.Fatalf("Failed to find saved world: %v", err)
		}
		if saved.Name != "Test World" {
			t.Fatalf("Expected Name 'Test World', got: '%s'", saved.Name)
		}
		if saved.Description != "Test Description" {
			t.Fatalf("Expected Description 'Test Description', got: '%s'", saved.Description)
		}
	})

	t.Run("updates existing world", func(t *testing.T) {
		w := world.New("Update World")
		w.Description = "Initial Description"
		if err := db.Create(w).Error; err != nil {
			t.Fatalf("Failed to create world: %v", err)
		}

		// Обновляем описание
		w.Description = "Updated Description"
		w.TimeOfDay = "night"
		w.Weather = "stormy"

		err := repo.Save(ctx, w)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Проверяем обновление
		var updated world.World
		if err := db.First(&updated, w.ID).Error; err != nil {
			t.Fatalf("Failed to find updated world: %v", err)
		}
		if updated.Description != "Updated Description" {
			t.Fatalf("Expected Description 'Updated Description', got: '%s'", updated.Description)
		}
		if updated.TimeOfDay != "night" {
			t.Fatalf("Expected TimeOfDay 'night', got: '%s'", updated.TimeOfDay)
		}
		if updated.Weather != "stormy" {
			t.Fatalf("Expected Weather 'stormy', got: '%s'", updated.Weather)
		}
	})

	t.Run("saves world with main quest", func(t *testing.T) {
		// Создаем главный квест
		mainQuest := quest.New("Main Quest", "Main Quest Description")
		mainQuest.SetExperienceReward(500)
		if err := db.Create(mainQuest).Error; err != nil {
			t.Fatalf("Failed to create quest: %v", err)
		}

		w := world.New("Quest World")
		w.Description = "World with Main Quest"
		w.SetMainQuest(mainQuest)

		err := repo.Save(ctx, w)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Проверяем сохранение
		var saved world.World
		if err := db.Preload("MainQuest").First(&saved, w.ID).Error; err != nil {
			t.Fatalf("Failed to find saved world: %v", err)
		}
		if saved.MainQuest == nil {
			t.Fatal("Expected MainQuest to be set")
		}
		if saved.MainQuest.Title != "Main Quest" {
			t.Fatalf("Expected MainQuest.Title 'Main Quest', got: '%s'", saved.MainQuest.Title)
		}
		// MainQuestID может быть не установлен автоматически, проверяем через MainQuest.ID
		if saved.MainQuest.ID != mainQuest.ID {
			t.Fatalf("Expected MainQuest.ID %d, got: %d", mainQuest.ID, saved.MainQuest.ID)
		}
	})

	t.Run("saves world with locations", func(t *testing.T) {
		w := world.New("Locations World")
		w.Description = "World with Locations"

		// Создаем локации
		loc1 := &world.Location{
			Name:        "Location 1",
			Description: "Description 1",
		}
		loc2 := &world.Location{
			Name:        "Location 2",
			Description: "Description 2",
		}

		w.Locations = []world.Location{*loc1, *loc2}

		err := repo.Save(ctx, w)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Проверяем сохранение
		var saved world.World
		if err := db.Preload("Locations").First(&saved, w.ID).Error; err != nil {
			t.Fatalf("Failed to find saved world: %v", err)
		}
		if len(saved.Locations) != 2 {
			t.Fatalf("Expected 2 locations, got: %d", len(saved.Locations))
		}
		if saved.Locations[0].Name != "Location 1" {
			t.Fatalf("Expected first location name 'Location 1', got: '%s'", saved.Locations[0].Name)
		}
		if saved.Locations[1].Name != "Location 2" {
			t.Fatalf("Expected second location name 'Location 2', got: '%s'", saved.Locations[1].Name)
		}
	})

	t.Run("saves world with locations and NPCs", func(t *testing.T) {
		w := world.New("NPCs World")
		w.Description = "World with Locations and NPCs"

		// Создаем локацию с NPC
		loc := &world.Location{
			Name:        "NPC Location",
			Description: "Location with NPC",
			NPCs: []world.NPC{
				{
					Name:        "NPC 1",
					Role:        "Merchant",
					Personality: "Friendly",
				},
				{
					Name:        "NPC 2",
					Role:        "Guard",
					Personality: "Strict",
				},
			},
		}

		w.Locations = []world.Location{*loc}

		err := repo.Save(ctx, w)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Проверяем сохранение
		var saved world.World
		if err := db.Preload("Locations.NPCs").First(&saved, w.ID).Error; err != nil {
			t.Fatalf("Failed to find saved world: %v", err)
		}
		if len(saved.Locations) != 1 {
			t.Fatalf("Expected 1 location, got: %d", len(saved.Locations))
		}
		if len(saved.Locations[0].NPCs) != 2 {
			t.Fatalf("Expected 2 NPCs, got: %d", len(saved.Locations[0].NPCs))
		}
		if saved.Locations[0].NPCs[0].Name != "NPC 1" {
			t.Fatalf("Expected first NPC name 'NPC 1', got: '%s'", saved.Locations[0].NPCs[0].Name)
		}
		if saved.Locations[0].NPCs[1].Name != "NPC 2" {
			t.Fatalf("Expected second NPC name 'NPC 2', got: '%s'", saved.Locations[0].NPCs[1].Name)
		}
	})
}

func TestWorldRepository_ContextTimeout(t *testing.T) {
	db := setupWorldTestDB(t)
	repo := NewWorldRepository(db)

	t.Run("respects context timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()

		// Даем контексту время истечь
		time.Sleep(10 * time.Millisecond)

		w := world.New("Timeout World")
		err := repo.Save(ctx, w)
		if err == nil {
			t.Fatal("Expected context deadline exceeded error")
		}
		if ctx.Err() == nil {
			t.Fatal("Expected context to be cancelled")
		}
	})
}
