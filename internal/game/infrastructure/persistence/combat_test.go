package persistence

import (
	"context"
	"testing"
	"time"

	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/combat"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/game/domain/world"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupCombatTestDB(t *testing.T) *gorm.DB {
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
		&combat.Combat{},
		&combat.CombatParticipant{},
	)
	if err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	return db
}

func TestCombatRepository_GetActiveBySessionID(t *testing.T) {
	db := setupCombatTestDB(t)
	repo := NewCombatRepository(db)
	ctx := context.Background()

	t.Run("returns nil when no active combat found", func(t *testing.T) {
		// Создаем сессию
		w := &world.World{Name: "Test World", Description: "Test"}
		if err := db.Create(w).Error; err != nil {
			t.Fatalf("Failed to create world: %v", err)
		}

		gs := &session.GameSession{ChatID: 12345, State: session.StateActive, WorldID: w.ID}
		if err := db.Create(gs).Error; err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}

		result, err := repo.GetActiveBySessionID(ctx, gs.ID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if result != nil {
			t.Fatalf("Expected nil, got: %v", result)
		}
	})

	t.Run("returns active combat with participants", func(t *testing.T) {
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

		// Создаем активный бой
		c := &combat.Combat{
			GameSessionID: gs.ID,
			State:         combat.CombatStateActive,
			CurrentTurn:   0,
			Participants: []combat.CombatParticipant{
				{
					IsPlayer:    true,
					CharacterID: &char.ID,
					Character:   char,
					Initiative:  15,
				},
				{
					IsPlayer:      false,
					MonsterName:    "Goblin",
					MonsterHP:     5,
					MonsterMaxHP:  5,
					MonsterAC:     12,
					MonsterAttackBonus: 2,
					Initiative:     10,
				},
			},
		}
		if err := db.Create(c).Error; err != nil {
			t.Fatalf("Failed to create combat: %v", err)
		}

		// Тестируем получение
		result, err := repo.GetActiveBySessionID(ctx, gs.ID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if result == nil {
			t.Fatal("Expected combat, got nil")
		}
		if result.State != combat.CombatStateActive {
			t.Fatalf("Expected CombatStateActive, got: %s", result.State)
		}
		if len(result.Participants) != 2 {
			t.Fatalf("Expected 2 participants, got: %d", len(result.Participants))
		}
		if result.Participants[0].Character == nil {
			t.Fatal("Expected Character to be loaded")
		}
		if result.Participants[0].Character.Name != "Test Character" {
			t.Fatalf("Expected Character.Name 'Test Character', got: %s", result.Participants[0].Character.Name)
		}
	})

	t.Run("does not return finished combat", func(t *testing.T) {
		// Создаем сессию
		w := &world.World{Name: "Test World 3", Description: "Test 3"}
		if err := db.Create(w).Error; err != nil {
			t.Fatalf("Failed to create world: %v", err)
		}

		gs := &session.GameSession{ChatID: 11111, State: session.StateActive, WorldID: w.ID}
		if err := db.Create(gs).Error; err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}

		// Создаем завершенный бой
		c := &combat.Combat{
			GameSessionID: gs.ID,
			State:         combat.CombatStateFinished,
			CurrentTurn:   0,
		}
		if err := db.Create(c).Error; err != nil {
			t.Fatalf("Failed to create combat: %v", err)
		}

		// Тестируем получение
		result, err := repo.GetActiveBySessionID(ctx, gs.ID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if result != nil {
			t.Fatalf("Expected nil for finished combat, got: %v", result)
		}
	})
}

func TestCombatRepository_Save(t *testing.T) {
	db := setupCombatTestDB(t)
	repo := NewCombatRepository(db)
	ctx := context.Background()

	t.Run("saves new combat", func(t *testing.T) {
		// Создаем сессию
		w := &world.World{Name: "Save World", Description: "Save"}
		if err := db.Create(w).Error; err != nil {
			t.Fatalf("Failed to create world: %v", err)
		}

		gs := &session.GameSession{ChatID: 22222, State: session.StateActive, WorldID: w.ID}
		if err := db.Create(gs).Error; err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}

		c := &combat.Combat{
			GameSessionID: gs.ID,
			State:         combat.CombatStateActive,
			CurrentTurn:   0,
		}

		err := repo.Save(ctx, c)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if c.ID == 0 {
			t.Fatal("Expected ID to be set")
		}

		// Проверяем, что бой сохранен
		var saved combat.Combat
		if err := db.First(&saved, c.ID).Error; err != nil {
			t.Fatalf("Failed to find saved combat: %v", err)
		}
		if saved.State != combat.CombatStateActive {
			t.Fatalf("Expected CombatStateActive, got: %s", saved.State)
		}
	})

	t.Run("updates existing combat", func(t *testing.T) {
		// Создаем сессию
		w := &world.World{Name: "Update World", Description: "Update"}
		if err := db.Create(w).Error; err != nil {
			t.Fatalf("Failed to create world: %v", err)
		}

		gs := &session.GameSession{ChatID: 33333, State: session.StateActive, WorldID: w.ID}
		if err := db.Create(gs).Error; err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}

		c := &combat.Combat{
			GameSessionID: gs.ID,
			State:         combat.CombatStateActive,
			CurrentTurn:   0,
		}
		if err := db.Create(c).Error; err != nil {
			t.Fatalf("Failed to create combat: %v", err)
		}

		// Обновляем состояние
		c.State = combat.CombatStateFinished
		c.CurrentTurn = 1
		err := repo.Save(ctx, c)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Проверяем обновление
		var updated combat.Combat
		if err := db.First(&updated, c.ID).Error; err != nil {
			t.Fatalf("Failed to find updated combat: %v", err)
		}
		if updated.State != combat.CombatStateFinished {
			t.Fatalf("Expected CombatStateFinished, got: %s", updated.State)
		}
		if updated.CurrentTurn != 1 {
			t.Fatalf("Expected CurrentTurn 1, got: %d", updated.CurrentTurn)
		}
	})

	t.Run("saves combat with participants", func(t *testing.T) {
		// Создаем сессию
		w := &world.World{Name: "Participants World", Description: "Participants"}
		if err := db.Create(w).Error; err != nil {
			t.Fatalf("Failed to create world: %v", err)
		}

		gs := &session.GameSession{ChatID: 44444, State: session.StateActive, WorldID: w.ID}
		if err := db.Create(gs).Error; err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}

		// Создаем персонажа
		char := &character.Character{
			Name:   "Combat Character",
			Race:   character.RaceElf,
			Class:  character.ClassWizard,
			Level:  2,
			HP:     15,
			MaxHP:  15,
			Status: character.StatusAlive,
			Stats: character.Stats{
				Strength:     12,
				Dexterity:    14,
				Constitution: 13,
				Intelligence: 10,
				Wisdom:       11,
				Charisma:     15,
			},
		}
		if err := db.Create(char).Error; err != nil {
			t.Fatalf("Failed to create character: %v", err)
		}

		c := &combat.Combat{
			GameSessionID: gs.ID,
			State:         combat.CombatStateActive,
			CurrentTurn:   0,
			Participants: []combat.CombatParticipant{
				{
					IsPlayer:    true,
					CharacterID: &char.ID,
					Character:   char,
					Initiative:  18,
				},
				{
					IsPlayer:      false,
					MonsterName:    "Orc",
					MonsterHP:     10,
					MonsterMaxHP:  10,
					MonsterAC:     14,
					MonsterAttackBonus: 3,
					Initiative:     12,
				},
			},
		}

		err := repo.Save(ctx, c)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Проверяем, что участники сохранены
		var saved combat.Combat
		if err := db.Preload("Participants.Character").First(&saved, c.ID).Error; err != nil {
			t.Fatalf("Failed to find saved combat: %v", err)
		}
		if len(saved.Participants) != 2 {
			t.Fatalf("Expected 2 participants, got: %d", len(saved.Participants))
		}
		if saved.Participants[0].Character.Name != "Combat Character" {
			t.Fatalf("Expected Character.Name 'Combat Character', got: %s", saved.Participants[0].Character.Name)
		}
		if saved.Participants[1].MonsterName != "Orc" {
			t.Fatalf("Expected MonsterName 'Orc', got: %s", saved.Participants[1].MonsterName)
		}
	})
}

func TestCombatRepository_GetByID(t *testing.T) {
	db := setupCombatTestDB(t)
	repo := NewCombatRepository(db)
	ctx := context.Background()

	t.Run("returns nil when combat not found", func(t *testing.T) {
		result, err := repo.GetByID(ctx, 99999)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if result != nil {
			t.Fatalf("Expected nil, got: %v", result)
		}
	})

	t.Run("returns combat with participants", func(t *testing.T) {
		// Создаем сессию
		w := &world.World{Name: "Get World", Description: "Get"}
		if err := db.Create(w).Error; err != nil {
			t.Fatalf("Failed to create world: %v", err)
		}

		gs := &session.GameSession{ChatID: 55555, State: session.StateActive, WorldID: w.ID}
		if err := db.Create(gs).Error; err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}

		// Создаем персонажа
		char := &character.Character{
			Name:   "Get Character",
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

		// Создаем бой
		c := &combat.Combat{
			GameSessionID: gs.ID,
			State:         combat.CombatStateActive,
			CurrentTurn:   0,
			Participants: []combat.CombatParticipant{
				{
					IsPlayer:    true,
					CharacterID: &char.ID,
					Character:   char,
					Initiative:  16,
				},
			},
		}
		if err := db.Create(c).Error; err != nil {
			t.Fatalf("Failed to create combat: %v", err)
		}

		// Тестируем получение
		result, err := repo.GetByID(ctx, c.ID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if result == nil {
			t.Fatal("Expected combat, got nil")
		}
		if result.ID != c.ID {
			t.Fatalf("Expected ID %d, got: %d", c.ID, result.ID)
		}
		if len(result.Participants) != 1 {
			t.Fatalf("Expected 1 participant, got: %d", len(result.Participants))
		}
		if result.Participants[0].Character == nil {
			t.Fatal("Expected Character to be loaded")
		}
		if result.Participants[0].Character.Name != "Get Character" {
			t.Fatalf("Expected Character.Name 'Get Character', got: %s", result.Participants[0].Character.Name)
		}
	})
}

func TestCombatRepository_ContextTimeout(t *testing.T) {
	db := setupCombatTestDB(t)
	repo := NewCombatRepository(db)

	t.Run("respects context timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()

		// Даем контексту время истечь
		time.Sleep(10 * time.Millisecond)

		_, err := repo.GetActiveBySessionID(ctx, 12345)
		if err == nil {
			t.Fatal("Expected context deadline exceeded error")
		}
		if ctx.Err() == nil {
			t.Fatal("Expected context to be cancelled")
		}
	})
}

// TestCombatRepository_PreloadStats проверяет, что Stats загружаются через Preload (#64)
func TestCombatRepository_PreloadStats(t *testing.T) {
	db := setupCombatTestDB(t)
	repo := NewCombatRepository(db)
	ctx := context.Background()

	t.Run("GetActiveBySessionID preloads Character.Stats", func(t *testing.T) {
		// Создаем сессию
		w := &world.World{Name: "Stats World", Description: "Stats"}
		if err := db.Create(w).Error; err != nil {
			t.Fatalf("Failed to create world: %v", err)
		}

		gs := &session.GameSession{ChatID: 99999, State: session.StateActive, WorldID: w.ID}
		if err := db.Create(gs).Error; err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}

		// Создаем персонажа с Stats
		char := &character.Character{
			Name:   "Stats Character",
			Race:   character.RaceHuman,
			Class:  character.ClassFighter,
			Level:  1,
			HP:     20,
			MaxHP:  20,
			Status: character.StatusAlive,
			Stats: character.Stats{
				Strength:     16,
				Dexterity:    14,
				Constitution: 15,
				Intelligence: 12,
				Wisdom:       13,
				Charisma:     10,
			},
		}
		if err := db.Create(char).Error; err != nil {
			t.Fatalf("Failed to create character: %v", err)
		}

		// Создаем активный бой
		c := &combat.Combat{
			GameSessionID: gs.ID,
			State:         combat.CombatStateActive,
			CurrentTurn:   0,
			Participants: []combat.CombatParticipant{
				{
					IsPlayer:    true,
					CharacterID: &char.ID,
					Character:   char,
					Initiative:  15,
				},
			},
		}
		if err := db.Create(c).Error; err != nil {
			t.Fatalf("Failed to create combat: %v", err)
		}

		// Тестируем получение - Stats должны быть загружены через Preload
		result, err := repo.GetActiveBySessionID(ctx, gs.ID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if result == nil {
			t.Fatal("Expected combat, got nil")
		}
		if len(result.Participants) != 1 {
			t.Fatalf("Expected 1 participant, got: %d", len(result.Participants))
		}
		if result.Participants[0].Character == nil {
			t.Fatal("Expected Character to be loaded")
		}
		if result.Participants[0].Character.Stats.Strength == 0 {
			t.Error("Expected Stats.Strength to be loaded via Preload, got 0")
		}
		if result.Participants[0].Character.Stats.Strength != 16 {
			t.Errorf("Expected Stats.Strength 16, got: %d", result.Participants[0].Character.Stats.Strength)
		}
		if result.Participants[0].Character.Stats.Dexterity != 14 {
			t.Errorf("Expected Stats.Dexterity 14, got: %d", result.Participants[0].Character.Stats.Dexterity)
		}
	})

	t.Run("GetByID preloads Character.Stats", func(t *testing.T) {
		// Создаем сессию
		w := &world.World{Name: "Stats World 2", Description: "Stats 2"}
		if err := db.Create(w).Error; err != nil {
			t.Fatalf("Failed to create world: %v", err)
		}

		gs := &session.GameSession{ChatID: 88888, State: session.StateActive, WorldID: w.ID}
		if err := db.Create(gs).Error; err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}

		// Создаем персонажа с Stats
		char := &character.Character{
			Name:   "Stats Character 2",
			Race:   character.RaceElf,
			Class:  character.ClassWizard,
			Level:  3,
			HP:     25,
			MaxHP:  25,
			Status: character.StatusAlive,
			Stats: character.Stats{
				Strength:     10,
				Dexterity:    16,
				Constitution: 12,
				Intelligence: 18,
				Wisdom:       15,
				Charisma:     14,
			},
		}
		if err := db.Create(char).Error; err != nil {
			t.Fatalf("Failed to create character: %v", err)
		}

		// Создаем бой
		c := &combat.Combat{
			GameSessionID: gs.ID,
			State:         combat.CombatStateActive,
			CurrentTurn:   0,
			Participants: []combat.CombatParticipant{
				{
					IsPlayer:    true,
					CharacterID: &char.ID,
					Character:   char,
					Initiative:  20,
				},
			},
		}
		if err := db.Create(c).Error; err != nil {
			t.Fatalf("Failed to create combat: %v", err)
		}

		// Тестируем получение по ID - Stats должны быть загружены через Preload
		result, err := repo.GetByID(ctx, c.ID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if result == nil {
			t.Fatal("Expected combat, got nil")
		}
		if len(result.Participants) != 1 {
			t.Fatalf("Expected 1 participant, got: %d", len(result.Participants))
		}
		if result.Participants[0].Character == nil {
			t.Fatal("Expected Character to be loaded")
		}
		if result.Participants[0].Character.Stats.Intelligence == 0 {
			t.Error("Expected Stats.Intelligence to be loaded via Preload, got 0")
		}
		if result.Participants[0].Character.Stats.Intelligence != 18 {
			t.Errorf("Expected Stats.Intelligence 18, got: %d", result.Participants[0].Character.Stats.Intelligence)
		}
		if result.Participants[0].Character.Stats.Dexterity != 16 {
			t.Errorf("Expected Stats.Dexterity 16, got: %d", result.Participants[0].Character.Stats.Dexterity)
		}
	})
}
