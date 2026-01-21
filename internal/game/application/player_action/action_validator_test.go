package player_action

import (
	"context"
	"strings"
	"testing"

	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/player"
	"dungeons-and-dragons-ai/internal/game/domain/session"

	"gorm.io/gorm"
)

// Mock Inventory Repository больше не используется - валидация предметов теперь через DM tools

func TestActionValidator_Validate_NoPlayer(t *testing.T) {
	validator := NewActionValidator()

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
	validator := NewActionValidator()

	char := &character.Character{
		ID:     1,
		Name:   "Test",
		Status: character.StatusDead,
		Stats: character.Stats{
			Strength:     10,
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
	validator := NewActionValidator()
	// Валидация предметов теперь выполняется DM через tools, поэтому базовый валидатор всегда возвращает true
	// (кроме случаев, когда персонаж мертв)

	char := &character.Character{
		ID:     1,
		Name:   "Test",
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
		t.Errorf("Expected validation to pass (validation now done via DM tools), got message: %s", result.Message)
	}
}

func TestActionValidator_Validate_ItemUsage_NoItem(t *testing.T) {
	validator := NewActionValidator()
	// Валидация предметов теперь выполняется DM через tools, поэтому базовый валидатор всегда возвращает true
	// (кроме случаев, когда персонаж мертв)

	char := &character.Character{
		ID:     1,
		Name:   "Test",
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

	// Валидация предметов теперь выполняется DM через tools, поэтому базовый валидатор всегда возвращает true
	// (кроме случаев, когда персонаж мертв)
	if !result.Valid {
		t.Errorf("Expected validation to pass (validation now done via DM tools), got message: %s", result.Message)
	}
}

func TestActionValidator_Validate_ItemUsage_EmptyInventory(t *testing.T) {
	validator := NewActionValidator()
	// Валидация предметов теперь выполняется DM через tools, поэтому базовый валидатор всегда возвращает true
	// (кроме случаев, когда персонаж мертв)

	char := &character.Character{
		ID:     1,
		Name:   "Test",
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

	// Валидация предметов теперь выполняется DM через tools, поэтому базовый валидатор всегда возвращает true
	// (кроме случаев, когда персонаж мертв)
	if !result.Valid {
		t.Errorf("Expected validation to pass (validation now done via DM tools), got message: %s", result.Message)
	}
}

func TestActionValidator_Validate_StrengthRequirement_Insufficient(t *testing.T) {
	validator := NewActionValidator()
	// Валидация характеристик теперь выполняется DM через tools, поэтому базовый валидатор всегда возвращает true
	// (кроме случаев, когда персонаж мертв)

	char := &character.Character{
		ID:     1,
		Name:   "Test",
		Status: character.StatusAlive,
		Stats: character.Stats{
			Strength:     8, // Меньше 10
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

	// Валидация характеристик теперь выполняется DM через tools
	if !result.Valid {
		t.Errorf("Expected validation to pass (validation now done via DM tools), got message: %s", result.Message)
	}
}

func TestActionValidator_Validate_StrengthRequirement_Sufficient(t *testing.T) {
	validator := NewActionValidator()
	// Валидация характеристик теперь выполняется DM через tools, поэтому базовый валидатор всегда возвращает true
	// (кроме случаев, когда персонаж мертв)

	char := &character.Character{
		ID:     1,
		Name:   "Test",
		Status: character.StatusAlive,
		Stats: character.Stats{
			Strength:     15, // Больше 10
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

	// Валидация характеристик теперь выполняется DM через tools
	if !result.Valid {
		t.Errorf("Expected validation to pass (validation now done via DM tools), got message: %s", result.Message)
	}
}

func TestActionValidator_Validate_DexterityRequirement_Insufficient(t *testing.T) {
	validator := NewActionValidator()
	// Валидация характеристик теперь выполняется DM через tools, поэтому базовый валидатор всегда возвращает true
	// (кроме случаев, когда персонаж мертв)

	char := &character.Character{
		ID:     1,
		Name:   "Test",
		Status: character.StatusAlive,
		Stats: character.Stats{
			Strength:     10,
			Dexterity:    8, // Меньше 10
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

	result, err := validator.Validate(context.Background(), gs, "забраться на стену")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Валидация характеристик теперь выполняется DM через tools
	if !result.Valid {
		t.Errorf("Expected validation to pass (validation now done via DM tools), got message: %s", result.Message)
	}
}

func TestActionValidator_Validate_DexterityRequirement_Sufficient(t *testing.T) {
	validator := NewActionValidator()
	// Валидация характеристик теперь выполняется DM через tools, поэтому базовый валидатор всегда возвращает true
	// (кроме случаев, когда персонаж мертв)

	char := &character.Character{
		ID:     1,
		Name:   "Test",
		Status: character.StatusAlive,
		Stats: character.Stats{
			Strength:     10,
			Dexterity:    15, // Больше 10
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

	result, err := validator.Validate(context.Background(), gs, "забраться на стену")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Валидация характеристик теперь выполняется DM через tools
	if !result.Valid {
		t.Errorf("Expected validation to pass (validation now done via DM tools), got message: %s", result.Message)
	}
}

func TestActionValidator_Validate_InventoryRepoError(t *testing.T) {
	validator := NewActionValidator()
	// Валидация предметов теперь выполняется DM через tools, поэтому базовый валидатор всегда возвращает true
	// (кроме случаев, когда персонаж мертв)

	char := &character.Character{
		ID:     1,
		Name:   "Test",
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

	p := &player.Player{
		ID:        1,
		Character: *char,
	}

	gs := &session.GameSession{
		Model:   gorm.Model{ID: 1},
		WorldID: 1,
		Players: []player.Player{*p},
	}

	// Валидация теперь не зависит от инвентаря - это делается через tools
	result, err := validator.Validate(context.Background(), gs, "использовать меч")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Валидация должна пройти, так как проверка предметов теперь делается через tools
	if !result.Valid {
		t.Errorf("Expected validation to pass (validation now done via DM tools), got message: %s", result.Message)
	}
}

func TestActionValidator_Validate_NoInventoryRepo(t *testing.T) {
	validator := NewActionValidator()
	// Валидация предметов теперь выполняется DM через tools, поэтому базовый валидатор всегда возвращает true
	// (кроме случаев, когда персонаж мертв)

	char := &character.Character{
		ID:     1,
		Name:   "Test",
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

	p := &player.Player{
		ID:        1,
		Character: *char,
	}

	gs := &session.GameSession{
		Model:   gorm.Model{ID: 1},
		WorldID: 1,
		Players: []player.Player{*p},
	}

	// Валидация теперь не зависит от инвентаря - это делается через tools
	result, err := validator.Validate(context.Background(), gs, "использовать меч")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !result.Valid {
		t.Errorf("Expected validation to pass (validation now done via DM tools), got message: %s", result.Message)
	}
}

func TestActionValidator_Validate_ItemExtraction(t *testing.T) {
	validator := NewActionValidator()
	// Валидация предметов теперь выполняется DM через tools, поэтому базовый валидатор всегда возвращает true
	// (кроме случаев, когда персонаж мертв)

	char := &character.Character{
		ID:     1,
		Name:   "Test",
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

	p := &player.Player{
		ID:        1,
		Character: *char,
	}

	gs := &session.GameSession{
		Model:   gorm.Model{ID: 1},
		WorldID: 1,
		Players: []player.Player{*p},
	}

	// Все действия должны проходить валидацию, так как проверка предметов теперь делается через tools
	testCases := []struct {
		name          string
		action        string
		expectedValid bool
	}{
		{"use potion", "выпить зелье лечения", true},
		{"use sword", "использовать меч", true},
		{"use non-existent", "использовать топор", true}, // Теперь проходит, так как валидация через tools
		{"apply potion", "применить зелье", true},
		{"wear armor", "надеть доспех", true}, // Теперь проходит, так как валидация через tools
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := validator.Validate(context.Background(), gs, tc.action)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if result.Valid != tc.expectedValid {
				t.Errorf("Expected valid=%v, got valid=%v, message: %s (validation now done via DM tools)", tc.expectedValid, result.Valid, result.Message)
			}
		})
	}
}

func TestActionValidator_Validate_NormalMessageHandling(t *testing.T) {
	validator := NewActionValidator()

	char := &character.Character{
		ID:     1,
		Name:   "Test",
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
	validator := NewActionValidator()

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
		// Валидация характеристик теперь выполняется DM через tools
		result, err := validator.Validate(context.Background(), gs, "поднять тяжелый камень")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// Должно быть валидным, так как валидация характеристик теперь делается через tools
		if !result.Valid {
			t.Errorf("Expected validation to pass (validation now done via DM tools), got message: %s", result.Message)
		}

		// Проверяем, что персонаж с силой 0 также проходит базовую валидацию
		// (проверка характеристик теперь делается через tools)
		char2 := &character.Character{
			ID:     2,
			Name:   "Weak Hero",
			Status: character.StatusAlive,
			Stats: character.Stats{
				Strength:     0, // Сила 0 < 10, но валидация теперь через tools
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

		// Валидация характеристик теперь делается через tools, поэтому должна проходить
		if !result2.Valid {
			t.Errorf("Expected validation to pass (validation now done via DM tools), got message: %s", result2.Message)
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

		// Валидация характеристик теперь делается через tools, поэтому должна проходить
		if !result.Valid {
			t.Errorf("Expected validation to pass (validation now done via DM tools), got message: %s", result.Message)
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

		// Валидация характеристик теперь делается через tools, поэтому должна проходить
		if !result.Valid {
			t.Errorf("Expected validation to pass (validation now done via DM tools), got message: %s", result.Message)
		}
	})
}

// TestActionValidator_Validate_TakeAction_NotBlocked проверяет, что действие "взять" не блокируется валидатором (#66)
func TestActionValidator_Validate_TakeAction_NotBlocked(t *testing.T) {
	validator := NewActionValidator()

	char := &character.Character{
		ID:     1,
		Name:   "Test",
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

	p := &player.Player{
		ID:        1,
		Character: *char,
	}

	gs := &session.GameSession{
		Model:   gorm.Model{ID: 1},
		WorldID: 1,
		Players: []player.Player{*p},
	}

	// Тест: действие "взять" не должно блокироваться валидатором, даже при пустом инвентаре
	// Это действие означает подбор предмета из мира, а не использование из инвентаря
	testCases := []struct {
		name          string
		action        string
		expectedValid bool
	}{
		{"take item", "взять меч", true},
		{"take potion", "взять зелье", true},
		{"take from ground", "взять ключ с земли", true},
		{"take from chest", "взять предмет из сундука", true},
		{"take uppercase", "ВЗЯТЬ МЕЧ", true},
		{"take with spaces", "  взять  меч  ", true},
		{"pickup item", "поднять меч", true}, // Синоним "взять"
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := validator.Validate(context.Background(), gs, tc.action)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if result.Valid != tc.expectedValid {
				t.Errorf("Action '%s': expected valid=%v, got valid=%v, message: %s",
					tc.action, tc.expectedValid, result.Valid, result.Message)
			}

			// Действие "взять" не должно блокироваться, даже если предмета нет в инвентаре
			// так как это подбор из мира, а не использование из инвентаря
			if !result.Valid && strings.Contains(tc.action, "взять") {
				t.Errorf("Action 'взять' should not be blocked by validator (it's pickup from world, not inventory usage)")
			}
		})
	}
}

// contains определен в handle_action_test.go (общая функция для пакета)
