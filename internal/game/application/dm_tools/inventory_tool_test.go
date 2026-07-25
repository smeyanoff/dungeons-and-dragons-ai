package dm_tools

import (
	"context"
	"errors"
	"testing"

	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/inventory"
	"dungeons-and-dragons-ai/internal/game/domain/player"
	"dungeons-and-dragons-ai/internal/game/domain/session"
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

func TestGetInventoryTool_Name(t *testing.T) {
	tool := NewGetInventoryTool(nil, 1)
	if tool.Name() != "get_inventory" {
		t.Errorf("expected name 'get_inventory', got '%s'", tool.Name())
	}
}

func TestGetInventoryTool_Description(t *testing.T) {
	tool := NewGetInventoryTool(nil, 1)
	if tool.Description() == "" {
		t.Error("expected non-empty description")
	}
}

func TestGetInventoryTool_Parameters(t *testing.T) {
	tool := NewGetInventoryTool(nil, 1)
	params := tool.Parameters()
	if len(params) == 0 {
		t.Error("expected non-empty parameters")
	}
}

func TestGetInventoryTool_Execute(t *testing.T) {
	tests := []struct {
		name          string
		characterID   uint
		setupMock     func(*mockInventoryRepo)
		expectedError bool
		validate      func(*testing.T, interface{})
	}{
		{
			name:        "successful inventory retrieval",
			characterID: 1,
			setupMock: func(repo *mockInventoryRepo) {
				repo.getByCharacterIDFunc = func(ctx context.Context, characterID uint) (*inventory.Inventory, error) {
					inv := inventory.NewInventory(characterID)
					inv.AddItem("Меч", "Обычный меч", 2.0, 1, inventory.ItemTypeWeapon, 0)
					inv.AddItem("Зелье", "Зелье лечения", 0.5, 2, inventory.ItemTypePotion, 0)
					return inv, nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if totalWeight, ok := resultMap["total_weight"].(float64); !ok || totalWeight != 3.0 {
					t.Errorf("expected total_weight 3.0, got %v", resultMap["total_weight"])
				}

				if maxWeight, ok := resultMap["max_weight"].(float64); !ok || maxWeight != inventory.MaxWeight {
					t.Errorf("expected max_weight %f, got %v", inventory.MaxWeight, resultMap["max_weight"])
				}

				if itemCount, ok := resultMap["item_count"].(int); !ok || itemCount != 2 {
					t.Errorf("expected item_count 2, got %v", resultMap["item_count"])
				}

				items, ok := resultMap["items"].([]map[string]interface{})
				if !ok || len(items) != 2 {
					t.Errorf("expected 2 items, got %v", items)
				}
			},
		},
		{
			name:        "empty inventory",
			characterID: 1,
			setupMock: func(repo *mockInventoryRepo) {
				repo.getByCharacterIDFunc = func(ctx context.Context, characterID uint) (*inventory.Inventory, error) {
					return inventory.NewInventory(characterID), nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if totalWeight, ok := resultMap["total_weight"].(float64); !ok || totalWeight != 0.0 {
					t.Errorf("expected total_weight 0.0, got %v", resultMap["total_weight"])
				}

				if itemCount, ok := resultMap["item_count"].(int); !ok || itemCount != 0 {
					t.Errorf("expected item_count 0, got %v", resultMap["item_count"])
				}
			},
		},
		{
			name:        "repository error",
			characterID: 1,
			setupMock: func(repo *mockInventoryRepo) {
				repo.getByCharacterIDFunc = func(ctx context.Context, characterID uint) (*inventory.Inventory, error) {
					return nil, errors.New("database error")
				}
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockInventoryRepo{}
			if tt.setupMock != nil {
				tt.setupMock(repo)
			}

			tool := NewGetInventoryTool(repo, tt.characterID)
			result, err := tool.Execute(context.Background(), map[string]interface{}{})

			if tt.expectedError {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if tt.validate != nil {
					tt.validate(t, result)
				}
			}
		})
	}
}

func TestAddItemTool_Name(t *testing.T) {
	tool := NewAddItemTool(nil, 1)
	if tool.Name() != "add_item_to_inventory" {
		t.Errorf("expected name 'add_item_to_inventory', got '%s'", tool.Name())
	}
}

func TestAddItemTool_Description(t *testing.T) {
	tool := NewAddItemTool(nil, 1)
	if tool.Description() == "" {
		t.Error("expected non-empty description")
	}
}

func TestAddItemTool_Parameters(t *testing.T) {
	tool := NewAddItemTool(nil, 1)
	params := tool.Parameters()
	if len(params) == 0 {
		t.Error("expected non-empty parameters")
	}
}

func TestAddItemTool_Execute(t *testing.T) {
	tests := []struct {
		name          string
		characterID   uint
		args          map[string]interface{}
		setupMock     func(*mockInventoryRepo)
		expectedError bool
		validate      func(*testing.T, interface{})
	}{
		{
			name:        "successfully add item with all fields",
			characterID: 1,
			args: map[string]interface{}{
				"name":        "Меч",
				"description": "Обычный меч",
				"weight":      2.0,
				"quantity":    1.0,
				"type":        "weapon",
			},
			setupMock: func(repo *mockInventoryRepo) {
				repo.getByCharacterIDFunc = func(ctx context.Context, characterID uint) (*inventory.Inventory, error) {
					return inventory.NewInventory(characterID), nil
				}
				repo.saveFunc = func(ctx context.Context, inv *inventory.Inventory) error {
					return nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if success, ok := resultMap["success"].(bool); !ok || !success {
					t.Errorf("expected success=true, got %v", resultMap["success"])
				}

				if itemAdded, ok := resultMap["item_added"].(string); !ok || itemAdded != "Меч" {
					t.Errorf("expected item_added='Меч', got %v", resultMap["item_added"])
				}
			},
		},
		{
			name:        "add item with default weight and type",
			characterID: 1,
			args: map[string]interface{}{
				"name": "Зелье лечения",
			},
			setupMock: func(repo *mockInventoryRepo) {
				repo.getByCharacterIDFunc = func(ctx context.Context, characterID uint) (*inventory.Inventory, error) {
					return inventory.NewInventory(characterID), nil
				}
				repo.saveFunc = func(ctx context.Context, inv *inventory.Inventory) error {
					return nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if success, ok := resultMap["success"].(bool); !ok || !success {
					t.Errorf("expected success=true, got %v", resultMap["success"])
				}
			},
		},
		{
			name:        "add item with quantity > 1",
			characterID: 1,
			args: map[string]interface{}{
				"name":     "Стрела",
				"quantity": 5.0,
				"type":     "misc",
			},
			setupMock: func(repo *mockInventoryRepo) {
				repo.getByCharacterIDFunc = func(ctx context.Context, characterID uint) (*inventory.Inventory, error) {
					return inventory.NewInventory(characterID), nil
				}
				repo.saveFunc = func(ctx context.Context, inv *inventory.Inventory) error {
					return nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if quantity, ok := resultMap["quantity"].(int); !ok || quantity != 5 {
					t.Errorf("expected quantity=5, got %v", resultMap["quantity"])
				}
			},
		},
		{
			name:        "error when name is missing",
			characterID: 1,
			args:        map[string]interface{}{},
			setupMock: func(repo *mockInventoryRepo) {
				repo.getByCharacterIDFunc = func(ctx context.Context, characterID uint) (*inventory.Inventory, error) {
					return inventory.NewInventory(characterID), nil
				}
			},
			expectedError: true,
		},
		{
			name:        "error when name is empty",
			characterID: 1,
			args: map[string]interface{}{
				"name": "",
			},
			setupMock: func(repo *mockInventoryRepo) {
				repo.getByCharacterIDFunc = func(ctx context.Context, characterID uint) (*inventory.Inventory, error) {
					return inventory.NewInventory(characterID), nil
				}
			},
			expectedError: true,
		},
		{
			name:        "error when inventory is full",
			characterID: 1,
			args: map[string]interface{}{
				"name":   "Тяжелый предмет",
				"weight": 50.0,
			},
			setupMock: func(repo *mockInventoryRepo) {
				repo.getByCharacterIDFunc = func(ctx context.Context, characterID uint) (*inventory.Inventory, error) {
					inv := inventory.NewInventory(characterID)
					// Заполняем почти весь инвентарь
					inv.AddItem("Предмет", "Описание", 25.0, 1, inventory.ItemTypeMisc, 0)
					return inv, nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if success, ok := resultMap["success"].(bool); !ok || success {
					t.Errorf("expected success=false, got %v", resultMap["success"])
				}

				if _, ok := resultMap["error"]; !ok {
					t.Error("expected error field in result")
				}
			},
		},
		{
			name:        "error when repository get fails",
			characterID: 1,
			args: map[string]interface{}{
				"name": "Предмет",
			},
			setupMock: func(repo *mockInventoryRepo) {
				repo.getByCharacterIDFunc = func(ctx context.Context, characterID uint) (*inventory.Inventory, error) {
					return nil, errors.New("database error")
				}
			},
			expectedError: true,
		},
		{
			name:        "error when repository save fails",
			characterID: 1,
			args: map[string]interface{}{
				"name": "Предмет",
			},
			setupMock: func(repo *mockInventoryRepo) {
				repo.getByCharacterIDFunc = func(ctx context.Context, characterID uint) (*inventory.Inventory, error) {
					return inventory.NewInventory(characterID), nil
				}
				repo.saveFunc = func(ctx context.Context, inv *inventory.Inventory) error {
					return errors.New("save error")
				}
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockInventoryRepo{}
			if tt.setupMock != nil {
				tt.setupMock(repo)
			}

			tool := NewAddItemTool(repo, tt.characterID)
			result, err := tool.Execute(context.Background(), tt.args)

			if tt.expectedError {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if tt.validate != nil {
					tt.validate(t, result)
				}
			}
		})
	}
}

func TestRemoveItemTool_Name(t *testing.T) {
	tool := NewRemoveItemTool(nil, nil, 1, 1)
	if tool.Name() != "remove_item_from_inventory" {
		t.Errorf("expected name 'remove_item_from_inventory', got '%s'", tool.Name())
	}
}

func TestRemoveItemTool_Description(t *testing.T) {
	tool := NewRemoveItemTool(nil, nil, 1, 1)
	if tool.Description() == "" {
		t.Error("expected non-empty description")
	}
}

func TestRemoveItemTool_Parameters(t *testing.T) {
	tool := NewRemoveItemTool(nil, nil, 1, 1)
	params := tool.Parameters()
	if len(params) == 0 {
		t.Error("expected non-empty parameters")
	}
}

func TestRemoveItemTool_Execute(t *testing.T) {
	tests := []struct {
		name          string
		characterID   uint
		args          map[string]interface{}
		setupMock     func(*mockInventoryRepo)
		expectedError bool
		validate      func(*testing.T, interface{})
	}{
		{
			name:        "successfully remove item",
			characterID: 1,
			args: map[string]interface{}{
				"name": "Меч",
			},
			setupMock: func(repo *mockInventoryRepo) {
				repo.getByCharacterIDFunc = func(ctx context.Context, characterID uint) (*inventory.Inventory, error) {
					inv := inventory.NewInventory(characterID)
					inv.AddItem("Меч", "Обычный меч", 2.0, 1, inventory.ItemTypeWeapon, 0)
					return inv, nil
				}
				repo.saveFunc = func(ctx context.Context, inv *inventory.Inventory) error {
					return nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if success, ok := resultMap["success"].(bool); !ok || !success {
					t.Errorf("expected success=true, got %v", resultMap["success"])
				}

				if itemRemoved, ok := resultMap["item_removed"].(string); !ok || itemRemoved != "Меч" {
					t.Errorf("expected item_removed='Меч', got %v", resultMap["item_removed"])
				}
			},
		},
		{
			name:        "remove item with quantity > 1",
			characterID: 1,
			args: map[string]interface{}{
				"name":     "Стрела",
				"quantity": 2.0,
			},
			setupMock: func(repo *mockInventoryRepo) {
				repo.getByCharacterIDFunc = func(ctx context.Context, characterID uint) (*inventory.Inventory, error) {
					inv := inventory.NewInventory(characterID)
					inv.AddItem("Стрела", "Обычная стрела", 0.1, 5, inventory.ItemTypeMisc, 0)
					return inv, nil
				}
				repo.saveFunc = func(ctx context.Context, inv *inventory.Inventory) error {
					return nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if quantity, ok := resultMap["quantity"].(int); !ok || quantity != 2 {
					t.Errorf("expected quantity=2, got %v", resultMap["quantity"])
				}
			},
		},
		{
			name:        "error when name is missing",
			characterID: 1,
			args:        map[string]interface{}{},
			setupMock: func(repo *mockInventoryRepo) {
				repo.getByCharacterIDFunc = func(ctx context.Context, characterID uint) (*inventory.Inventory, error) {
					return inventory.NewInventory(characterID), nil
				}
			},
			expectedError: true,
		},
		{
			name:        "error when item not found",
			characterID: 1,
			args: map[string]interface{}{
				"name": "Несуществующий предмет",
			},
			setupMock: func(repo *mockInventoryRepo) {
				repo.getByCharacterIDFunc = func(ctx context.Context, characterID uint) (*inventory.Inventory, error) {
					return inventory.NewInventory(characterID), nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if success, ok := resultMap["success"].(bool); !ok || success {
					t.Errorf("expected success=false, got %v", resultMap["success"])
				}

				if _, ok := resultMap["error"]; !ok {
					t.Error("expected error field in result")
				}
			},
		},
		{
			name:        "error when not enough items",
			characterID: 1,
			args: map[string]interface{}{
				"name":     "Стрела",
				"quantity": 10.0,
			},
			setupMock: func(repo *mockInventoryRepo) {
				repo.getByCharacterIDFunc = func(ctx context.Context, characterID uint) (*inventory.Inventory, error) {
					inv := inventory.NewInventory(characterID)
					inv.AddItem("Стрела", "Обычная стрела", 0.1, 3, inventory.ItemTypeMisc, 0)
					return inv, nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if success, ok := resultMap["success"].(bool); !ok || success {
					t.Errorf("expected success=false, got %v", resultMap["success"])
				}
			},
		},
		{
			name:        "error when repository get fails",
			characterID: 1,
			args: map[string]interface{}{
				"name": "Предмет",
			},
			setupMock: func(repo *mockInventoryRepo) {
				repo.getByCharacterIDFunc = func(ctx context.Context, characterID uint) (*inventory.Inventory, error) {
					return nil, errors.New("database error")
				}
			},
			expectedError: true,
		},
		{
			name:        "error when repository save fails",
			characterID: 1,
			args: map[string]interface{}{
				"name": "Меч",
			},
			setupMock: func(repo *mockInventoryRepo) {
				repo.getByCharacterIDFunc = func(ctx context.Context, characterID uint) (*inventory.Inventory, error) {
					inv := inventory.NewInventory(characterID)
					inv.AddItem("Меч", "Обычный меч", 2.0, 1, inventory.ItemTypeWeapon, 0)
					return inv, nil
				}
				repo.saveFunc = func(ctx context.Context, inv *inventory.Inventory) error {
					return errors.New("save error")
				}
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockInventoryRepo{}
			if tt.setupMock != nil {
				tt.setupMock(repo)
			}

			tool := NewRemoveItemTool(repo, nil, tt.characterID, 1)
			result, err := tool.Execute(context.Background(), tt.args)

			if tt.expectedError {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if tt.validate != nil {
					tt.validate(t, result)
				}
			}
		})
	}
}

func TestRemoveItemTool_Execute_UsingHealingPotion_HealsAndDecrementsQuantity(t *testing.T) {
	char, err := character.NewCharacter("Test Hero", character.ClassFighter, character.RaceHuman, character.Stats{
		Strength: 10, Dexterity: 10, Constitution: 10, Intelligence: 10, Wisdom: 10, Charisma: 10,
	})
	if err != nil {
		t.Fatalf("failed to create character: %v", err)
	}
	char.ID = 1
	char.MaxHP = 30
	char.HP = 10 // раненый персонаж

	gs := &session.GameSession{
		ChatID: 42,
		Players: []player.Player{
			{Character: *char},
		},
	}
	gs.Model.ID = 1

	inventoryRepo := &mockInventoryRepo{
		getByCharacterIDFunc: func(ctx context.Context, characterID uint) (*inventory.Inventory, error) {
			inv := inventory.NewInventory(characterID)
			// В инвентаре 2 зелья, каждое лечит 15 HP.
			if err := inv.AddItem("Зелье лечения", "Восстанавливает HP", 0.5, 2, inventory.ItemTypePotion, 15); err != nil {
				t.Fatalf("setup: failed to add item: %v", err)
			}
			return inv, nil
		},
		saveFunc: func(ctx context.Context, inv *inventory.Inventory) error {
			return nil
		},
	}

	sessionRepo := &mockSessionRepoForCharacter{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			return gs, nil
		},
	}

	tool := NewRemoveItemTool(inventoryRepo, sessionRepo, char.ID, gs.ChatID)
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"name":     "Зелье лечения",
		"quantity": 1.0,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map[string]interface{}, got %T", result)
	}

	if success, _ := resultMap["success"].(bool); !success {
		t.Fatalf("expected success=true, got %v", resultMap["success"])
	}
	if quantity, _ := resultMap["quantity"].(int); quantity != 1 {
		t.Errorf("expected quantity removed=1, got %v", resultMap["quantity"])
	}
	if healed, _ := resultMap["healed"].(bool); !healed {
		t.Fatalf("expected healed=true in result, got %v", resultMap["healed"])
	}
	if healing, _ := resultMap["healing_amount"].(int); healing != 15 {
		t.Errorf("expected healing_amount=15, got %v", resultMap["healing_amount"])
	}

	// HP персонажа должно вырасти с 10 до 25 (не быть неизменным и уж тем более не упасть).
	if gs.Players[0].Character.HP != 25 {
		t.Errorf("expected character HP=25 after healing, got %d", gs.Players[0].Character.HP)
	}
	if sessionRepo.savedSession == nil {
		t.Error("expected session to be saved after healing")
	}

	// Инвентарь должен сохранять сокращённое количество зелий (репозиторий сохраняет весь inv через saveFunc,
	// здесь достаточно убедиться, что вызов не сообщает об увеличении).
	if item, _ := resultMap["item_removed"].(string); item != "Зелье лечения" {
		t.Errorf("expected item_removed='Зелье лечения', got %v", resultMap["item_removed"])
	}
}

func TestRemoveItemTool_Execute_NonHealingItem_DoesNotTouchHP(t *testing.T) {
	char, err := character.NewCharacter("Test Hero", character.ClassFighter, character.RaceHuman, character.Stats{
		Strength: 10, Dexterity: 10, Constitution: 10, Intelligence: 10, Wisdom: 10, Charisma: 10,
	})
	if err != nil {
		t.Fatalf("failed to create character: %v", err)
	}
	char.ID = 1
	char.MaxHP = 30
	char.HP = 10

	gs := &session.GameSession{
		ChatID:  42,
		Players: []player.Player{{Character: *char}},
	}
	gs.Model.ID = 1

	inventoryRepo := &mockInventoryRepo{
		getByCharacterIDFunc: func(ctx context.Context, characterID uint) (*inventory.Inventory, error) {
			inv := inventory.NewInventory(characterID)
			if err := inv.AddItem("Факел", "Освещает путь", 0.5, 1, inventory.ItemTypeTool, 0); err != nil {
				t.Fatalf("setup: failed to add item: %v", err)
			}
			return inv, nil
		},
		saveFunc: func(ctx context.Context, inv *inventory.Inventory) error { return nil },
	}
	sessionRepo := &mockSessionRepoForCharacter{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			return gs, nil
		},
	}

	tool := NewRemoveItemTool(inventoryRepo, sessionRepo, char.ID, gs.ChatID)
	result, err := tool.Execute(context.Background(), map[string]interface{}{"name": "Факел"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap := result.(map[string]interface{})
	if _, ok := resultMap["healed"]; ok {
		t.Errorf("expected no 'healed' field for a non-healing item, got %v", resultMap["healed"])
	}
	if gs.Players[0].Character.HP != 10 {
		t.Errorf("expected HP to stay unchanged at 10, got %d", gs.Players[0].Character.HP)
	}
	if sessionRepo.savedSession != nil {
		t.Error("expected session NOT to be saved when item has no healing effect")
	}
}
