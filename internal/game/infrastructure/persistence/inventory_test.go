package persistence

import (
	"context"
	"testing"
	"time"

	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/inventory"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupInventoryTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	// Автомиграции
	err = db.AutoMigrate(
		&character.Character{},
		&character.Stats{},
		&inventory.Inventory{},
		&inventory.InventoryItem{},
	)
	if err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	return db
}

func TestInventoryRepository_GetByCharacterID(t *testing.T) {
	db := setupInventoryTestDB(t)
	repo := NewInventoryRepository(db)
	ctx := context.Background()

	t.Run("creates new inventory when not found", func(t *testing.T) {
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

		// Получаем инвентарь (должен быть создан автоматически)
		result, err := repo.GetByCharacterID(ctx, char.ID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if result == nil {
			t.Fatal("Expected inventory, got nil")
		}
		if result.CharacterID != char.ID {
			t.Fatalf("Expected CharacterID %d, got: %d", char.ID, result.CharacterID)
		}
		if len(result.Items) != 0 {
			t.Fatalf("Expected empty inventory, got: %d items", len(result.Items))
		}

		// Проверяем, что инвентарь сохранен в БД
		var saved inventory.Inventory
		if err := db.First(&saved, result.ID).Error; err != nil {
			t.Fatalf("Failed to find saved inventory: %v", err)
		}
		if saved.CharacterID != char.ID {
			t.Fatalf("Expected CharacterID %d in saved inventory, got: %d", char.ID, saved.CharacterID)
		}
	})

	t.Run("returns existing inventory with items", func(t *testing.T) {
		// Создаем персонажа
		char := &character.Character{
			Name:   "Existing Character",
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

		// Создаем инвентарь с предметами
		inv := inventory.NewInventory(char.ID)
		inv.Items = []inventory.InventoryItem{
			{
				Name:        "Sword",
				Description: "A sharp sword",
				Weight:      2.5,
				Quantity:    1,
				Type:        inventory.ItemTypeWeapon,
			},
			{
				Name:        "Health Potion",
				Description: "Restores 10 HP",
				Weight:      0.5,
				Quantity:    3,
				Type:        inventory.ItemTypePotion,
			},
		}
		if err := db.Create(inv).Error; err != nil {
			t.Fatalf("Failed to create inventory: %v", err)
		}

		// Получаем инвентарь
		result, err := repo.GetByCharacterID(ctx, char.ID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if result == nil {
			t.Fatal("Expected inventory, got nil")
		}
		if len(result.Items) != 2 {
			t.Fatalf("Expected 2 items, got: %d", len(result.Items))
		}
		if result.Items[0].Name != "Sword" {
			t.Fatalf("Expected first item to be 'Sword', got: '%s'", result.Items[0].Name)
		}
		if result.Items[1].Name != "Health Potion" {
			t.Fatalf("Expected second item to be 'Health Potion', got: '%s'", result.Items[1].Name)
		}
		if result.Items[1].Quantity != 3 {
			t.Fatalf("Expected Health Potion quantity 3, got: %d", result.Items[1].Quantity)
		}
	})

	t.Run("returns different inventories for different characters", func(t *testing.T) {
		// Создаем двух персонажей
		char1 := &character.Character{
			Name:   "Character 1",
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
		if err := db.Create(char1).Error; err != nil {
			t.Fatalf("Failed to create character: %v", err)
		}

		char2 := &character.Character{
			Name:   "Character 2",
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
		if err := db.Create(char2).Error; err != nil {
			t.Fatalf("Failed to create character: %v", err)
		}

		// Создаем инвентарь для первого персонажа
		inv1 := inventory.NewInventory(char1.ID)
		inv1.Items = []inventory.InventoryItem{
			{Name: "Sword", Description: "Sword", Weight: 2.5, Quantity: 1, Type: inventory.ItemTypeWeapon},
		}
		if err := db.Create(inv1).Error; err != nil {
			t.Fatalf("Failed to create inventory: %v", err)
		}

		// Создаем инвентарь для второго персонажа
		inv2 := inventory.NewInventory(char2.ID)
		inv2.Items = []inventory.InventoryItem{
			{Name: "Staff", Description: "Staff", Weight: 1.5, Quantity: 1, Type: inventory.ItemTypeWeapon},
		}
		if err := db.Create(inv2).Error; err != nil {
			t.Fatalf("Failed to create inventory: %v", err)
		}

		// Получаем инвентари
		result1, err := repo.GetByCharacterID(ctx, char1.ID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if len(result1.Items) != 1 || result1.Items[0].Name != "Sword" {
			t.Fatalf("Expected Character 1 to have Sword, got: %v", result1.Items)
		}

		result2, err := repo.GetByCharacterID(ctx, char2.ID)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if len(result2.Items) != 1 || result2.Items[0].Name != "Staff" {
			t.Fatalf("Expected Character 2 to have Staff, got: %v", result2.Items)
		}
	})
}

func TestInventoryRepository_Save(t *testing.T) {
	db := setupInventoryTestDB(t)
	repo := NewInventoryRepository(db)
	ctx := context.Background()

	t.Run("saves new inventory", func(t *testing.T) {
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

		inv := inventory.NewInventory(char.ID)
		inv.Items = []inventory.InventoryItem{
			{
				Name:        "Shield",
				Description: "A sturdy shield",
				Weight:      3.0,
				Quantity:    1,
				Type:        inventory.ItemTypeArmor,
			},
		}

		err := repo.Save(ctx, inv)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if inv.ID == 0 {
			t.Fatal("Expected ID to be set")
		}

		// Проверяем, что инвентарь сохранен
		var saved inventory.Inventory
		if err := db.Preload("Items").First(&saved, inv.ID).Error; err != nil {
			t.Fatalf("Failed to find saved inventory: %v", err)
		}
		if saved.CharacterID != char.ID {
			t.Fatalf("Expected CharacterID %d, got: %d", char.ID, saved.CharacterID)
		}
		if len(saved.Items) != 1 {
			t.Fatalf("Expected 1 item, got: %d", len(saved.Items))
		}
		if saved.Items[0].Name != "Shield" {
			t.Fatalf("Expected item name 'Shield', got: '%s'", saved.Items[0].Name)
		}
	})

	t.Run("updates existing inventory", func(t *testing.T) {
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

		// Создаем инвентарь
		inv := inventory.NewInventory(char.ID)
		inv.Items = []inventory.InventoryItem{
			{Name: "Dagger", Description: "Dagger", Weight: 0.5, Quantity: 1, Type: inventory.ItemTypeWeapon},
		}
		if err := db.Create(inv).Error; err != nil {
			t.Fatalf("Failed to create inventory: %v", err)
		}

		// Добавляем новый предмет
		inv.Items = append(inv.Items, inventory.InventoryItem{
			Name:        "Lockpick",
			Description: "A set of lockpicks",
			Weight:      0.1,
			Quantity:    1,
			Type:        inventory.ItemTypeTool,
		})

		err := repo.Save(ctx, inv)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Проверяем обновление
		var updated inventory.Inventory
		if err := db.Preload("Items").First(&updated, inv.ID).Error; err != nil {
			t.Fatalf("Failed to find updated inventory: %v", err)
		}
		if len(updated.Items) != 2 {
			t.Fatalf("Expected 2 items, got: %d", len(updated.Items))
		}
	})

	t.Run("saves inventory with multiple items", func(t *testing.T) {
		// Создаем персонажа
		char := &character.Character{
			Name:   "Multiple Items Character",
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

		inv := inventory.NewInventory(char.ID)
		inv.Items = []inventory.InventoryItem{
			{Name: "Bow", Description: "Longbow", Weight: 1.0, Quantity: 1, Type: inventory.ItemTypeWeapon},
			{Name: "Arrows", Description: "20 arrows", Weight: 0.5, Quantity: 20, Type: inventory.ItemTypeMisc},
			{Name: "Rope", Description: "50ft rope", Weight: 2.0, Quantity: 1, Type: inventory.ItemTypeTool},
		}

		err := repo.Save(ctx, inv)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Проверяем сохранение
		var saved inventory.Inventory
		if err := db.Preload("Items").First(&saved, inv.ID).Error; err != nil {
			t.Fatalf("Failed to find saved inventory: %v", err)
		}
		if len(saved.Items) != 3 {
			t.Fatalf("Expected 3 items, got: %d", len(saved.Items))
		}
	})
}

func TestInventoryRepository_ContextTimeout(t *testing.T) {
	db := setupInventoryTestDB(t)
	repo := NewInventoryRepository(db)

	t.Run("respects context timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()

		// Даем контексту время истечь
		time.Sleep(10 * time.Millisecond)

		_, err := repo.GetByCharacterID(ctx, 1)
		if err == nil {
			t.Fatal("Expected context deadline exceeded error")
		}
		if ctx.Err() == nil {
			t.Fatal("Expected context to be cancelled")
		}
	})
}
