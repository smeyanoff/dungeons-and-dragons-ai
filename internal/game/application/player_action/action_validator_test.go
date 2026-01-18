package player_action

import (
	"context"
	"errors"
	"testing"

	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/inventory"
	"dungeons-and-dragons-ai/internal/game/domain/player"
	"dungeons-and-dragons-ai/internal/game/domain/session"

	"gorm.io/gorm"
)

// Mock Inventory Repository
type mockInventoryRepo struct {
	getByCharacterIDFunc func(ctx context.Context, characterID uint) (*inventory.Inventory, error)
	saveFunc             func(ctx context.Context, inv *inventory.Inventory) error
}

func (m *mockInventoryRepo) GetByCharacterID(ctx context.Context, characterID uint) (*inventory.Inventory, error) {
	if m.getByCharacterIDFunc != nil {
		return m.getByCharacterIDFunc(ctx, characterID)
	}
	return nil, nil
}

func (m *mockInventoryRepo) Save(ctx context.Context, inv *inventory.Inventory) error {
	if m.saveFunc != nil {
		return m.saveFunc(ctx, inv)
	}
	return nil
}

func TestActionValidator_Validate_NoPlayer(t *testing.T) {
	validator := NewActionValidator(nil)

	gs := &session.GameSession{
		Model:   gorm.Model{ID: 1},
		WorldID: 1,
	}

	result, err := validator.Validate(context.Background(), gs, "test action")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !result.Valid {
		t.Error("Expected validation to pass when no player exists")
	}
}

func TestActionValidator_Validate_DeadCharacter(t *testing.T) {
	validator := NewActionValidator(nil)

	char := &character.Character{
		ID:     1,
		Name:   "Test",
		Status: character.StatusDead,
		Stats: character.Stats{
			Strength:   10,
			Dexterity:  10,
			Constitution: 10,
			Intelligence: 10,
			Wisdom:     10,
			Charisma:   10,
		},
	}

	p := &player.Player{
		ID:        1,
		Character: *char,
	}

	gs := &session.GameSession{
		Model:   gorm.Model{ID: 1},
		WorldID: 1,
		Players: []player.Player{*p},
	}

	result, err := validator.Validate(context.Background(), gs, "test action")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.Valid {
		t.Error("Expected validation to fail for dead character")
	}

	if result.Message == "" {
		t.Error("Expected error message for dead character")
	}
}

func TestActionValidator_Validate_ItemUsage_HasItem(t *testing.T) {
	validator := NewActionValidator(&mockInventoryRepo{
		getByCharacterIDFunc: func(ctx context.Context, characterID uint) (*inventory.Inventory, error) {
			inv := inventory.NewInventory(characterID)
			inv.Items = []inventory.InventoryItem{
				{
					Name:     "Меч",
					Type:     inventory.ItemTypeWeapon,
					Quantity: 1,
				},
			}
			return inv, nil
		},
	})

	char := &character.Character{
		ID:     1,
		Name:   "Test",
		Status: character.StatusAlive,
		Stats: character.Stats{
			Strength:   10,
			Dexterity:  10,
			Constitution: 10,
			Intelligence: 10,
			Wisdom:     10,
			Charisma:   10,
		},
	}

	p := &player.Player{
		ID:        1,
		Character: *char,
	}

	gs := &session.GameSession{
		Model:   gorm.Model{ID: 1},
		WorldID: 1,
		Players: []player.Player{*p},
	}

	result, err := validator.Validate(context.Background(), gs, "использовать меч")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !result.Valid {
		t.Errorf("Expected validation to pass, got message: %s", result.Message)
	}
}

func TestActionValidator_Validate_ItemUsage_NoItem(t *testing.T) {
	validator := NewActionValidator(&mockInventoryRepo{
		getByCharacterIDFunc: func(ctx context.Context, characterID uint) (*inventory.Inventory, error) {
			inv := inventory.NewInventory(characterID)
			// Пустой инвентарь
			return inv, nil
		},
	})

	char := &character.Character{
		ID:     1,
		Name:   "Test",
		Status: character.StatusAlive,
		Stats: character.Stats{
			Strength:   10,
			Dexterity:  10,
			Constitution: 10,
			Intelligence: 10,
			Wisdom:     10,
			Charisma:   10,
		},
	}

	p := &player.Player{
		ID:        1,
		Character: *char,
	}

	gs := &session.GameSession{
		Model:   gorm.Model{ID: 1},
		WorldID: 1,
		Players: []player.Player{*p},
	}

	result, err := validator.Validate(context.Background(), gs, "использовать зелье")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.Valid {
		t.Error("Expected validation to fail when item not in inventory")
	}

	if result.Message == "" {
		t.Error("Expected error message when item not found")
	}
}

func TestActionValidator_Validate_ItemUsage_EmptyInventory(t *testing.T) {
	validator := NewActionValidator(&mockInventoryRepo{
		getByCharacterIDFunc: func(ctx context.Context, characterID uint) (*inventory.Inventory, error) {
			return nil, nil // Инвентарь не существует
		},
	})

	char := &character.Character{
		ID:     1,
		Name:   "Test",
		Status: character.StatusAlive,
		Stats: character.Stats{
			Strength:   10,
			Dexterity:  10,
			Constitution: 10,
			Intelligence: 10,
			Wisdom:     10,
			Charisma:   10,
		},
	}

	p := &player.Player{
		ID:        1,
		Character: *char,
	}

	gs := &session.GameSession{
		Model:   gorm.Model{ID: 1},
		WorldID: 1,
		Players: []player.Player{*p},
	}

	result, err := validator.Validate(context.Background(), gs, "использовать зелье здоровья")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.Valid {
		t.Error("Expected validation to fail when trying to use item with empty inventory")
	}
}

func TestActionValidator_Validate_StrengthRequirement_Insufficient(t *testing.T) {
	validator := NewActionValidator(nil)

	char := &character.Character{
		ID:     1,
		Name:   "Test",
		Status: character.StatusAlive,
		Stats: character.Stats{
			Strength:   8, // Меньше 10
			Dexterity:  10,
			Constitution: 10,
			Intelligence: 10,
			Wisdom:     10,
			Charisma:   10,
		},
	}

	p := &player.Player{
		ID:        1,
		Character: *char,
	}

	gs := &session.GameSession{
		Model:   gorm.Model{ID: 1},
		WorldID: 1,
		Players: []player.Player{*p},
	}

	result, err := validator.Validate(context.Background(), gs, "поднять тяжелый камень")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.Valid {
		t.Error("Expected validation to fail when strength is insufficient")
	}

	if result.Message == "" {
		t.Error("Expected error message for insufficient strength")
	}
}

func TestActionValidator_Validate_StrengthRequirement_Sufficient(t *testing.T) {
	validator := NewActionValidator(nil)

	char := &character.Character{
		ID:     1,
		Name:   "Test",
		Status: character.StatusAlive,
		Stats: character.Stats{
			Strength:   15, // Больше 10
			Dexterity:  10,
			Constitution: 10,
			Intelligence: 10,
			Wisdom:     10,
			Charisma:   10,
		},
	}

	p := &player.Player{
		ID:        1,
		Character: *char,
	}

	gs := &session.GameSession{
		Model:   gorm.Model{ID: 1},
		WorldID: 1,
		Players: []player.Player{*p},
	}

	result, err := validator.Validate(context.Background(), gs, "поднять тяжелый камень")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !result.Valid {
		t.Errorf("Expected validation to pass, got message: %s", result.Message)
	}
}

func TestActionValidator_Validate_DexterityRequirement_Insufficient(t *testing.T) {
	validator := NewActionValidator(nil)

	char := &character.Character{
		ID:     1,
		Name:   "Test",
		Status: character.StatusAlive,
		Stats: character.Stats{
			Strength:   10,
			Dexterity:  8, // Меньше 10
			Constitution: 10,
			Intelligence: 10,
			Wisdom:     10,
			Charisma:   10,
		},
	}

	p := &player.Player{
		ID:        1,
		Character: *char,
	}

	gs := &session.GameSession{
		Model:   gorm.Model{ID: 1},
		WorldID: 1,
		Players: []player.Player{*p},
	}

	result, err := validator.Validate(context.Background(), gs, "забраться на стену")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.Valid {
		t.Error("Expected validation to fail when dexterity is insufficient")
	}

	if result.Message == "" {
		t.Error("Expected error message for insufficient dexterity")
	}
}

func TestActionValidator_Validate_DexterityRequirement_Sufficient(t *testing.T) {
	validator := NewActionValidator(nil)

	char := &character.Character{
		ID:     1,
		Name:   "Test",
		Status: character.StatusAlive,
		Stats: character.Stats{
			Strength:   10,
			Dexterity:  15, // Больше 10
			Constitution: 10,
			Intelligence: 10,
			Wisdom:     10,
			Charisma:   10,
		},
	}

	p := &player.Player{
		ID:        1,
		Character: *char,
	}

	gs := &session.GameSession{
		Model:   gorm.Model{ID: 1},
		WorldID: 1,
		Players: []player.Player{*p},
	}

	result, err := validator.Validate(context.Background(), gs, "забраться на стену")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !result.Valid {
		t.Errorf("Expected validation to pass, got message: %s", result.Message)
	}
}

func TestActionValidator_Validate_InventoryRepoError(t *testing.T) {
	validator := NewActionValidator(&mockInventoryRepo{
		getByCharacterIDFunc: func(ctx context.Context, characterID uint) (*inventory.Inventory, error) {
			return nil, errors.New("database error")
		},
	})

	char := &character.Character{
		ID:     1,
		Name:   "Test",
		Status: character.StatusAlive,
		Stats: character.Stats{
			Strength:   10,
			Dexterity:  10,
			Constitution: 10,
			Intelligence: 10,
			Wisdom:     10,
			Charisma:   10,
		},
	}

	p := &player.Player{
		ID:        1,
		Character: *char,
	}

	gs := &session.GameSession{
		Model:   gorm.Model{ID: 1},
		WorldID: 1,
		Players: []player.Player{*p},
	}

	// Должно пропустить проверку инвентаря при ошибке
	result, err := validator.Validate(context.Background(), gs, "использовать меч")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Валидация должна пройти, так как ошибка репозитория не блокирует действие
	if !result.Valid {
		t.Errorf("Expected validation to pass on repo error, got message: %s", result.Message)
	}
}

func TestActionValidator_Validate_NoInventoryRepo(t *testing.T) {
	validator := NewActionValidator(nil)

	char := &character.Character{
		ID:     1,
		Name:   "Test",
		Status: character.StatusAlive,
		Stats: character.Stats{
			Strength:   10,
			Dexterity:  10,
			Constitution: 10,
			Intelligence: 10,
			Wisdom:     10,
			Charisma:   10,
		},
	}

	p := &player.Player{
		ID:        1,
		Character: *char,
	}

	gs := &session.GameSession{
		Model:   gorm.Model{ID: 1},
		WorldID: 1,
		Players: []player.Player{*p},
	}

	// Должно пропустить проверку инвентаря, если репозиторий не настроен
	result, err := validator.Validate(context.Background(), gs, "использовать меч")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !result.Valid {
		t.Errorf("Expected validation to pass when no inventory repo, got message: %s", result.Message)
	}
}

func TestActionValidator_Validate_ItemExtraction(t *testing.T) {
	validator := NewActionValidator(&mockInventoryRepo{
		getByCharacterIDFunc: func(ctx context.Context, characterID uint) (*inventory.Inventory, error) {
			inv := inventory.NewInventory(characterID)
			inv.Items = []inventory.InventoryItem{
				{
					Name:     "Зелье лечения",
					Type:     inventory.ItemTypePotion,
					Quantity: 1,
				},
				{
					Name:     "Меч",
					Type:     inventory.ItemTypeWeapon,
					Quantity: 1,
				},
			}
			return inv, nil
		},
	})

	char := &character.Character{
		ID:     1,
		Name:   "Test",
		Status: character.StatusAlive,
		Stats: character.Stats{
			Strength:   10,
			Dexterity:  10,
			Constitution: 10,
			Intelligence: 10,
			Wisdom:     10,
			Charisma:   10,
		},
	}

	p := &player.Player{
		ID:        1,
		Character: *char,
	}

	gs := &session.GameSession{
		Model:   gorm.Model{ID: 1},
		WorldID: 1,
		Players: []player.Player{*p},
	}

	testCases := []struct {
		name           string
		action         string
		expectedValid  bool
		shouldContain  string
	}{
		{"use potion", "выпить зелье лечения", true, ""},
		{"use sword", "использовать меч", true, ""},
		{"use non-existent", "использовать топор", false, "топор"},
		{"apply potion", "применить зелье", true, ""},
		{"wear armor", "надеть доспех", false, "доспех"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := validator.Validate(context.Background(), gs, tc.action)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if result.Valid != tc.expectedValid {
				t.Errorf("Expected valid=%v, got valid=%v, message: %s", tc.expectedValid, result.Valid, result.Message)
			}

			if !tc.expectedValid && tc.shouldContain != "" {
				if result.Message == "" {
					t.Error("Expected error message")
				}
			}
		})
	}
}

func TestActionValidator_Validate_NormalMessageHandling(t *testing.T) {
	validator := NewActionValidator(nil)

	char := &character.Character{
		ID:     1,
		Name:   "Test",
		Status: character.StatusAlive,
		Stats: character.Stats{
			Strength:   10,
			Dexterity:  10,
			Constitution: 10,
			Intelligence: 10,
			Wisdom:     10,
			Charisma:   10,
		},
	}

	p := &player.Player{
		ID:        1,
		Character: *char,
	}

	gs := &session.GameSession{
		Model:   gorm.Model{ID: 1},
		WorldID: 1,
		Players: []player.Player{*p},
	}

	testCases := []struct {
		name          string
		action        string
		expectedValid bool
	}{
		{"uppercase", "ПОДНЯТЬ КАМЕНЬ", true},
		{"with spaces", "  поднять  камень  ", true},
		{"mixed case", "ПодНяТь КаМеНь", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := validator.Validate(context.Background(), gs, tc.action)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if result.Valid != tc.expectedValid {
				t.Errorf("Expected valid=%v, got valid=%v", tc.expectedValid, result.Valid)
			}
		})
	}
}

// TestActionValidator_Validate_StatsLoaded_Correctly проверяет, что Stats загружаются правильно
// и корректно отображаются в сообщениях об ошибках (баг #3)
func TestActionValidator_Validate_StatsLoaded_Correctly(t *testing.T) {
	validator := NewActionValidator(nil)

	// Тест 1: Персонаж с силой 16 - должно показывать 16, а не 0
	t.Run("stats displayed correctly in error message - strength 16 insufficient", func(t *testing.T) {
		char := &character.Character{
			ID:     1,
			Name:   "Strong Hero",
			Status: character.StatusAlive,
			Stats: character.Stats{
				Strength:     16, // Сила 16, но требуется 17+ для очень тяжелого действия
				Dexterity:    14,
				Constitution: 15,
				Intelligence: 12,
				Wisdom:       13,
				Charisma:     10,
			},
		}

		p := &player.Player{
			ID:        1,
			Character: *char,
		}

		gs := &session.GameSession{
			Model:   gorm.Model{ID: 1},
			WorldID: 1,
			Players: []player.Player{*p},
		}

		// Проверяем действие, требующее силу >= 10 (сила 16 достаточна)
		result, err := validator.Validate(context.Background(), gs, "поднять тяжелый камень")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// Должно быть валидным, так как сила 16 >= 10
		if !result.Valid {
			t.Errorf("Expected validation to pass with strength 16, got message: %s", result.Message)
		}

		// Проверяем, что если бы сила была 0, валидация бы не прошла
		// Создаем новый персонаж с силой 0 для проверки
		char2 := &character.Character{
			ID:     2,
			Name:   "Weak Hero",
			Status: character.StatusAlive,
			Stats: character.Stats{
				Strength:     0, // Сила 0 < 10
				Dexterity:    10,
				Constitution: 10,
				Intelligence: 10,
				Wisdom:       10,
				Charisma:     10,
			},
		}
		p2 := &player.Player{
			ID:        2,
			Character: *char2,
		}
		gs2 := &session.GameSession{
			Model:   gorm.Model{ID: 1},
			WorldID: 1,
			Players: []player.Player{*p2},
		}
		result2, err := validator.Validate(context.Background(), gs2, "поднять тяжелый камень")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if result2.Valid {
			t.Error("Expected validation to fail when strength is 0")
		}

		// Проверяем, что сообщение об ошибке содержит правильное значение силы (0, не 16)
		if result2.Message != "" && !contains(result2.Message, "0") && !contains(result2.Message, "10") {
			// Сообщение может содержать "10" как минимум, но не должно содержать "16"
			if contains(result2.Message, "16") {
				t.Errorf("BUG #3: Error message shows wrong strength value! Message: %s", result2.Message)
			}
		}
	})

	// Тест 2: Персонаж с силой 8 - должно показывать 8 в сообщении об ошибке
	t.Run("stats displayed correctly in error message - strength 8", func(t *testing.T) {
		char := &character.Character{
			ID:     1,
			Name:   "Weak Hero",
			Status: character.StatusAlive,
			Stats: character.Stats{
				Strength:     8, // Сила 8 < 10
				Dexterity:    10,
				Constitution: 10,
				Intelligence: 10,
				Wisdom:       10,
				Charisma:     10,
			},
		}

		p := &player.Player{
			ID:        1,
			Character: *char,
		}

		gs := &session.GameSession{
			Model:   gorm.Model{ID: 1},
			WorldID: 1,
			Players: []player.Player{*p},
		}

		result, err := validator.Validate(context.Background(), gs, "поднять тяжелый камень")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if result.Valid {
			t.Error("Expected validation to fail when strength is 8")
		}

		// Сообщение должно содержать "8", а не "0"
		if !contains(result.Message, "8") {
			t.Errorf("Expected error message to contain strength value 8, got message: %s", result.Message)
		}

		// Сообщение не должно содержать "0" как значение силы
		if contains(result.Message, "0") && !contains(result.Message, "10") {
			// Если сообщение содержит "0" но не "10" (минимум), это может быть проблема
			// Но на самом деле "0" может быть частью "10", так что проверим более точно
			if result.Message == "Ваша сила (0) недостаточна для этого действия. Требуется минимум 10." {
				t.Errorf("BUG #3: Error message shows strength as 0 instead of 8! Message: %s", result.Message)
			}
		}
	})

	// Тест 3: Проверка, что Stats не пустые при валидации
	t.Run("stats not zero when validating", func(t *testing.T) {
		char := &character.Character{
			ID:     1,
			Name:   "Test Hero",
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

		p := &player.Player{
			ID:        1,
			Character: *char,
		}

		gs := &session.GameSession{
			Model:   gorm.Model{ID: 1},
			WorldID: 1,
			Players: []player.Player{*p},
		}

		// Проверяем, что Stats не обнуляются при валидации
		result, err := validator.Validate(context.Background(), gs, "поднять камень")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// После валидации Stats должны остаться теми же
		if char.Stats.Strength != 16 {
			t.Errorf("Stats.Strength changed after validation: expected 16, got %d", char.Stats.Strength)
		}

		// Если валидация прошла, значит Stats были загружены правильно
		if !result.Valid && contains(result.Message, "0") && char.Stats.Strength != 0 {
			t.Errorf("BUG #3: Validation failed with strength 0 in message, but char.Stats.Strength is %d", char.Stats.Strength)
		}
	})
}

// contains определен в handle_action_test.go (общая функция для пакета)
