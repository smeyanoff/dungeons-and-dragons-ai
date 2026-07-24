package inventory

import (
	"context"
	"errors"
	"strings"
	"testing"

	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/inventory"
	"dungeons-and-dragons-ai/internal/game/domain/player"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/game/domain/world"
)

// Mock Session Repository
type mockSessionRepo struct {
	getByChatIDFunc func(ctx context.Context, chatID int64) (*session.GameSession, error)
}

func (m *mockSessionRepo) GetByChatID(ctx context.Context, chatID int64) (*session.GameSession, error) {
	if m.getByChatIDFunc != nil {
		return m.getByChatIDFunc(ctx, chatID)
	}
	return nil, nil
}

func (m *mockSessionRepo) Save(ctx context.Context, s *session.GameSession) error {
	return nil
}

func (m *mockSessionRepo) Delete(ctx context.Context, chatID int64) error {
	return nil
}

// Mock Inventory Repository
type mockInventoryRepo struct {
	getByCharacterIDFunc func(ctx context.Context, characterID uint) (*inventory.Inventory, error)
}

func (m *mockInventoryRepo) GetByCharacterID(ctx context.Context, characterID uint) (*inventory.Inventory, error) {
	if m.getByCharacterIDFunc != nil {
		return m.getByCharacterIDFunc(ctx, characterID)
	}
	return nil, nil
}

func TestGetInventoryUseCase_Execute(t *testing.T) {
	tests := []struct {
		name          string
		chatID        int64
		setupMocks    func(*mockSessionRepo, *mockInventoryRepo)
		expectedError bool
		validate      func(*testing.T, string)
	}{
		{
			name:   "successful inventory retrieval",
			chatID: 12345,
			setupMocks: func(sessionRepo *mockSessionRepo, inventoryRepo *mockInventoryRepo) {
				char, _ := character.NewCharacter("Test Hero", character.ClassFighter, character.RaceHuman, character.Stats{
					Strength: 16,
				})
				char.ID = 1

				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					gs := &session.GameSession{
						ChatID: chatID,
						State:  session.StateActive,
						World:  world.World{Name: "Test World"},
						Players: []player.Player{
							{
								Character: *char,
							},
						},
					}
					gs.Model.ID = 1
					return gs, nil
				}
				inventoryRepo.getByCharacterIDFunc = func(ctx context.Context, characterID uint) (*inventory.Inventory, error) {
					inv := inventory.NewInventory(characterID)
					inv.AddItem("Меч", "Обычный меч", 2.0, 1, inventory.ItemTypeWeapon, 0)
					inv.AddItem("Зелье лечения", "Восстанавливает HP", 0.5, 2, inventory.ItemTypePotion, 0)
					return inv, nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result string) {
				if result == "" {
					t.Error("expected non-empty result")
				}
				if !strings.Contains(result, "Инвентарь") {
					t.Error("result should contain 'Инвентарь'")
				}
				if !strings.Contains(result, "Test Hero") {
					t.Error("result should contain character name")
				}
				if !strings.Contains(result, "Меч") {
					t.Error("result should contain item name")
				}
			},
		},
		{
			name:   "no session",
			chatID: 12345,
			setupMocks: func(sessionRepo *mockSessionRepo, inventoryRepo *mockInventoryRepo) {
				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return nil, nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result string) {
				if result == "" {
					t.Error("expected error message")
				}
			},
		},
		{
			name:   "no character",
			chatID: 12345,
			setupMocks: func(sessionRepo *mockSessionRepo, inventoryRepo *mockInventoryRepo) {
				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					gs := &session.GameSession{
						ChatID:  chatID,
						State:   session.StateActive,
						Players: []player.Player{},
					}
					gs.Model.ID = 1
					return gs, nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result string) {
				if !strings.Contains(result, "Персонаж не создан") {
					t.Errorf("expected character creation message, got: %s", result)
				}
			},
		},
		{
			name:   "empty inventory",
			chatID: 12345,
			setupMocks: func(sessionRepo *mockSessionRepo, inventoryRepo *mockInventoryRepo) {
				char, _ := character.NewCharacter("Test Hero", character.ClassFighter, character.RaceHuman, character.Stats{
					Strength: 16,
				})
				char.ID = 1

				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					gs := &session.GameSession{
						ChatID: chatID,
						State:  session.StateActive,
						Players: []player.Player{
							{
								Character: *char,
							},
						},
					}
					gs.Model.ID = 1
					return gs, nil
				}
				inventoryRepo.getByCharacterIDFunc = func(ctx context.Context, characterID uint) (*inventory.Inventory, error) {
					return inventory.NewInventory(characterID), nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result string) {
				if !strings.Contains(result, "Инвентарь пуст") {
					t.Errorf("expected empty inventory message, got: %s", result)
				}
			},
		},
		{
			name:   "session repo error",
			chatID: 12345,
			setupMocks: func(sessionRepo *mockSessionRepo, inventoryRepo *mockInventoryRepo) {
				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return nil, errors.New("database error")
				}
			},
			expectedError: true,
		},
		{
			name:   "inventory repo error",
			chatID: 12345,
			setupMocks: func(sessionRepo *mockSessionRepo, inventoryRepo *mockInventoryRepo) {
				char, _ := character.NewCharacter("Test Hero", character.ClassFighter, character.RaceHuman, character.Stats{
					Strength: 16,
				})
				char.ID = 1

				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					gs := &session.GameSession{
						ChatID: chatID,
						State:  session.StateActive,
						Players: []player.Player{
							{
								Character: *char,
							},
						},
					}
					gs.Model.ID = 1
					return gs, nil
				}
				inventoryRepo.getByCharacterIDFunc = func(ctx context.Context, characterID uint) (*inventory.Inventory, error) {
					return nil, errors.New("inventory repo error")
				}
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessionRepo := &mockSessionRepo{}
			inventoryRepo := &mockInventoryRepo{}

			if tt.setupMocks != nil {
				tt.setupMocks(sessionRepo, inventoryRepo)
			}

			uc := NewGetInventoryUseCase(sessionRepo, inventoryRepo)

			// Используем chatID как tgUserID для тестов (в приватных чатах они совпадают)
			result, err := uc.Execute(context.Background(), tt.chatID, tt.chatID)

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
