package persistence

import (
	"context"
	"testing"
	"time"

	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/player"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/game/domain/world"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupPlayerTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	// Автомиграции
	err = db.AutoMigrate(
		&session.GameSession{},
		&world.World{},
		&character.Character{},
		&character.Stats{},
		&player.Player{},
	)
	if err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	return db
}

func TestPlayerRepository_GetByTgUserIDAndSessionID(t *testing.T) {
	db := setupPlayerTestDB(t)
	repo := NewPlayerRepository(db)
	ctx := context.Background()

	t.Run("returns nil when player not found", func(t *testing.T) {
		// Создаем сессию
		w := &world.World{Name: "Test World", Description: "Test"}
		if err := db.Create(w).Error; err != nil {
			t.Fatalf("Failed to create world: %v", err)
		}

		gs := &session.GameSession{ChatID: 12345, State: session.StateActive, WorldID: w.ID}
		if err := db.Create(gs).Error; err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}

		result, err := repo.GetByTgUserIDAndSessionID(ctx, 99999, gs.ID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if result != nil {
			t.Fatalf("Expected nil, got: %v", result)
		}
	})

	t.Run("returns player with character and stats", func(t *testing.T) {
		// Создаем сессию
		w := &world.World{Name: "Test World 2", Description: "Test 2"}
		if err := db.Create(w).Error; err != nil {
			t.Fatalf("Failed to create world: %v", err)
		}

		gs := &session.GameSession{ChatID: 67890, State: session.StateActive, WorldID: w.ID}
		if err := db.Create(gs).Error; err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}

		// Создаем персонажа
		char := &character.Character{
			Name:   "Test Character",
			Race:   character.RaceHuman,
			Class:  character.ClassFighter,
			Level:  1,
			HP:     10,
			MaxHP:  10,
			Status: character.StatusAlive,
			Stats: character.Stats{
				Strength:     10,
				Dexterity:    10,
				Constitution: 10,
				Intelligence: 10,
				Wisdom:       10,
				Charisma:     10,
			},
		}
		if err := db.Create(char).Error; err != nil {
			t.Fatalf("Failed to create character: %v", err)
		}

		// Создаем игрока
		p := &player.Player{
			TgUserID:      11111,
			Name:          "Test Player",
			GameSessionID: gs.ID,
			CharacterID:   char.ID,
			Character:     *char,
		}
		if err := db.Create(p).Error; err != nil {
			t.Fatalf("Failed to create player: %v", err)
		}

		// Тестируем получение
		result, err := repo.GetByTgUserIDAndSessionID(ctx, 11111, gs.ID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if result == nil {
			t.Fatal("Expected player, got nil")
		}
		if result.TgUserID != 11111 {
			t.Fatalf("Expected TgUserID 11111, got: %d", result.TgUserID)
		}
		if result.Name != "Test Player" {
			t.Fatalf("Expected Name 'Test Player', got: %s", result.Name)
		}
		if result.Character.Name != "Test Character" {
			t.Fatalf("Expected Character.Name 'Test Character', got: %s", result.Character.Name)
		}
		if result.Character.Stats.Strength != 10 {
			t.Fatalf("Expected Character.Stats.Strength 10, got: %d", result.Character.Stats.Strength)
		}
	})

	t.Run("returns nil for wrong session ID", func(t *testing.T) {
		// Создаем две сессии
		w1 := &world.World{Name: "World 1", Description: "World 1"}
		if err := db.Create(w1).Error; err != nil {
			t.Fatalf("Failed to create world: %v", err)
		}

		w2 := &world.World{Name: "World 2", Description: "World 2"}
		if err := db.Create(w2).Error; err != nil {
			t.Fatalf("Failed to create world: %v", err)
		}

		gs1 := &session.GameSession{ChatID: 10000, State: session.StateActive, WorldID: w1.ID}
		if err := db.Create(gs1).Error; err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}

		gs2 := &session.GameSession{ChatID: 20000, State: session.StateActive, WorldID: w2.ID}
		if err := db.Create(gs2).Error; err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}

		// Создаем персонажа
		char := &character.Character{
			Name:   "Session Character",
			Race:   character.RaceElf,
			Class:  character.ClassWizard,
			Level:  1,
			HP:     8,
			MaxHP:  8,
			Status: character.StatusAlive,
			Stats: character.Stats{
				Strength:     8,
				Dexterity:    14,
				Constitution: 13,
				Intelligence: 15,
				Wisdom:       12,
				Charisma:     10,
			},
		}
		if err := db.Create(char).Error; err != nil {
			t.Fatalf("Failed to create character: %v", err)
		}

		// Создаем игрока в первой сессии
		p := &player.Player{
			TgUserID:      22222,
			Name:          "Session Player",
			GameSessionID: gs1.ID,
			CharacterID:   char.ID,
			Character:     *char,
		}
		if err := db.Create(p).Error; err != nil {
			t.Fatalf("Failed to create player: %v", err)
		}

		// Пытаемся найти игрока во второй сессии
		result, err := repo.GetByTgUserIDAndSessionID(ctx, 22222, gs2.ID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if result != nil {
			t.Fatalf("Expected nil for wrong session, got: %v", result)
		}
	})
}

func TestPlayerRepository_Save(t *testing.T) {
	db := setupPlayerTestDB(t)
	repo := NewPlayerRepository(db)
	ctx := context.Background()

	t.Run("saves new player", func(t *testing.T) {
		// Создаем сессию
		w := &world.World{Name: "Save World", Description: "Save"}
		if err := db.Create(w).Error; err != nil {
			t.Fatalf("Failed to create world: %v", err)
		}

		gs := &session.GameSession{ChatID: 33333, State: session.StateActive, WorldID: w.ID}
		if err := db.Create(gs).Error; err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}

		// Создаем персонажа
		char := &character.Character{
			Name:   "Save Character",
			Race:   character.RaceDwarf,
			Class:  character.ClassCleric,
			Level:  1,
			HP:     12,
			MaxHP:  12,
			Status: character.StatusAlive,
			Stats: character.Stats{
				Strength:     14,
				Dexterity:    10,
				Constitution: 15,
				Intelligence: 8,
				Wisdom:       16,
				Charisma:     12,
			},
		}
		if err := db.Create(char).Error; err != nil {
			t.Fatalf("Failed to create character: %v", err)
		}

		p := &player.Player{
			TgUserID:      33333,
			Name:          "Save Player",
			GameSessionID: gs.ID,
			CharacterID:   char.ID,
			Character:     *char,
		}

		err := repo.Save(ctx, p)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if p.ID == 0 {
			t.Fatal("Expected ID to be set")
		}

		// Проверяем, что игрок сохранен
		var saved player.Player
		if err := db.First(&saved, p.ID).Error; err != nil {
			t.Fatalf("Failed to find saved player: %v", err)
		}
		if saved.TgUserID != 33333 {
			t.Fatalf("Expected TgUserID 33333, got: %d", saved.TgUserID)
		}
		if saved.Name != "Save Player" {
			t.Fatalf("Expected Name 'Save Player', got: %s", saved.Name)
		}
	})

	t.Run("updates existing player", func(t *testing.T) {
		// Создаем сессию
		w := &world.World{Name: "Update World", Description: "Update"}
		if err := db.Create(w).Error; err != nil {
			t.Fatalf("Failed to create world: %v", err)
		}

		gs := &session.GameSession{ChatID: 44444, State: session.StateActive, WorldID: w.ID}
		if err := db.Create(gs).Error; err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}

		// Создаем персонажа
		char := &character.Character{
			Name:   "Update Character",
			Race:   character.RaceHalfling,
			Class:  character.ClassRogue,
			Level:  1,
			HP:     8,
			MaxHP:  8,
			Status: character.StatusAlive,
			Stats: character.Stats{
				Strength:     8,
				Dexterity:    16,
				Constitution: 14,
				Intelligence: 12,
				Wisdom:       10,
				Charisma:     15,
			},
		}
		if err := db.Create(char).Error; err != nil {
			t.Fatalf("Failed to create character: %v", err)
		}

		p := &player.Player{
			TgUserID:      44444,
			Name:          "Update Player",
			GameSessionID: gs.ID,
			CharacterID:   char.ID,
			Character:     *char,
		}
		if err := db.Create(p).Error; err != nil {
			t.Fatalf("Failed to create player: %v", err)
		}

		// Обновляем имя
		p.Name = "Updated Player Name"
		err := repo.Save(ctx, p)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Проверяем обновление
		var updated player.Player
		if err := db.First(&updated, p.ID).Error; err != nil {
			t.Fatalf("Failed to find updated player: %v", err)
		}
		if updated.Name != "Updated Player Name" {
			t.Fatalf("Expected Name 'Updated Player Name', got: %s", updated.Name)
		}
	})

	t.Run("saves player with character", func(t *testing.T) {
		// Создаем сессию
		w := &world.World{Name: "Character World", Description: "Character"}
		if err := db.Create(w).Error; err != nil {
			t.Fatalf("Failed to create world: %v", err)
		}

		gs := &session.GameSession{ChatID: 55555, State: session.StateActive, WorldID: w.ID}
		if err := db.Create(gs).Error; err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}

		// Создаем персонажа
		char := &character.Character{
			Name:   "Character Test",
			Race:   character.RaceOrc,
			Class:  character.ClassRanger,
			Level:  2,
			HP:     16,
			MaxHP:  16,
			Status: character.StatusAlive,
			Stats: character.Stats{
				Strength:     16,
				Dexterity:    14,
				Constitution: 15,
				Intelligence: 10,
				Wisdom:       13,
				Charisma:     8,
			},
		}
		if err := db.Create(char).Error; err != nil {
			t.Fatalf("Failed to create character: %v", err)
		}

		p := &player.Player{
			TgUserID:      55555,
			Name:          "Character Player",
			GameSessionID: gs.ID,
			CharacterID:   char.ID,
			Character:     *char,
		}

		err := repo.Save(ctx, p)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Проверяем, что персонаж сохранен
		var saved player.Player
		if err := db.Preload("Character").Preload("Character.Stats").First(&saved, p.ID).Error; err != nil {
			t.Fatalf("Failed to find saved player: %v", err)
		}
		if saved.Character.Name != "Character Test" {
			t.Fatalf("Expected Character.Name 'Character Test', got: %s", saved.Character.Name)
		}
		if saved.Character.Stats.Strength != 16 {
			t.Fatalf("Expected Character.Stats.Strength 16, got: %d", saved.Character.Stats.Strength)
		}
	})
}

func TestPlayerRepository_ContextTimeout(t *testing.T) {
	db := setupPlayerTestDB(t)
	repo := NewPlayerRepository(db)

	t.Run("respects context timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()

		waitForContextDone(t, ctx)

		_, err := repo.GetByTgUserIDAndSessionID(ctx, 12345, 1)
		if err == nil {
			t.Fatal("Expected context deadline exceeded error")
		}
		if ctx.Err() == nil {
			t.Fatal("Expected context to be cancelled")
		}
	})
}
