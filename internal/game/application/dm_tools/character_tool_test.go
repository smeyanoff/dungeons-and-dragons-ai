package dm_tools

import (
	"context"
	"errors"
	"testing"

	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/player"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/game/domain/world"
)

// Mock Session Repository for character tool
type mockSessionRepoForCharacter struct {
	getByChatIDFunc func(ctx context.Context, chatID int64) (*session.GameSession, error)
}

func (m *mockSessionRepoForCharacter) GetByChatID(ctx context.Context, chatID int64) (*session.GameSession, error) {
	if m.getByChatIDFunc != nil {
		return m.getByChatIDFunc(ctx, chatID)
	}
	return nil, nil
}

func TestGetCharacterStatsTool_Name(t *testing.T) {
	tool := NewGetCharacterStatsTool(nil, 1)
	if tool.Name() != "get_character_stats" {
		t.Errorf("expected name 'get_character_stats', got '%s'", tool.Name())
	}
}

func TestGetCharacterStatsTool_Description(t *testing.T) {
	tool := NewGetCharacterStatsTool(nil, 1)
	if tool.Description() == "" {
		t.Error("expected non-empty description")
	}
}

func TestGetCharacterStatsTool_Parameters(t *testing.T) {
	tool := NewGetCharacterStatsTool(nil, 1)
	params := tool.Parameters()
	if len(params) == 0 {
		t.Error("expected non-empty parameters")
	}
}

func TestGetCharacterStatsTool_Execute(t *testing.T) {
	tests := []struct {
		name          string
		chatID        int64
		setupMock     func(*mockSessionRepoForCharacter)
		expectedError bool
		validate      func(*testing.T, interface{})
	}{
		{
			name:   "successful character stats retrieval",
			chatID: 12345,
			setupMock: func(repo *mockSessionRepoForCharacter) {
				char, _ := character.NewCharacter("Test Hero", character.ClassFighter, character.RaceHuman, character.Stats{
					Strength:     16,
					Dexterity:    14,
					Constitution: 15,
					Intelligence: 10,
					Wisdom:       12,
					Charisma:     13,
				})
				char.ID = 1
				char.Level = 3
				char.Experience = 500
				char.HP = 30
				char.MaxHP = 35

				repo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
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
			},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if name, ok := resultMap["name"].(string); !ok || name != "Test Hero" {
					t.Errorf("expected name='Test Hero', got %v", resultMap["name"])
				}

				if race, ok := resultMap["race"].(string); !ok || race != string(character.RaceHuman) {
					t.Errorf("expected race='%s', got %v", character.RaceHuman, resultMap["race"])
				}

				if class, ok := resultMap["class"].(string); !ok || class != string(character.ClassFighter) {
					t.Errorf("expected class='%s', got %v", character.ClassFighter, resultMap["class"])
				}

				if level, ok := resultMap["level"].(int); !ok || level != 3 {
					t.Errorf("expected level=3, got %v", resultMap["level"])
				}

				if hp, ok := resultMap["hp"].(int); !ok || hp != 30 {
					t.Errorf("expected hp=30, got %v", resultMap["hp"])
				}

				if maxHP, ok := resultMap["max_hp"].(int); !ok || maxHP != 35 {
					t.Errorf("expected max_hp=35, got %v", resultMap["max_hp"])
				}

				if experience, ok := resultMap["experience"].(int); !ok || experience != 500 {
					t.Errorf("expected experience=500, got %v", resultMap["experience"])
				}

				stats, ok := resultMap["stats"].(map[string]interface{})
				if !ok {
					t.Fatalf("expected stats map, got %T", resultMap["stats"])
				}

				if str, ok := stats["strength"].(int); !ok || str != 16 {
					t.Errorf("expected strength=16, got %v", stats["strength"])
				}
			},
		},
		{
			name:   "error when session not found",
			chatID: 12345,
			setupMock: func(repo *mockSessionRepoForCharacter) {
				repo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return nil, nil
				}
			},
			expectedError: true,
		},
		{
			name:   "error when session is nil",
			chatID: 12345,
			setupMock: func(repo *mockSessionRepoForCharacter) {
				repo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return nil, nil
				}
			},
			expectedError: true,
		},
		{
			name:   "error when no player",
			chatID: 12345,
			setupMock: func(repo *mockSessionRepoForCharacter) {
				repo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					gs := &session.GameSession{
						ChatID:  chatID,
						State:   session.StateActive,
						Players: []player.Player{},
					}
					gs.Model.ID = 1
					return gs, nil
				}
			},
			expectedError: true,
		},
		{
			name:   "error when repository returns error",
			chatID: 12345,
			setupMock: func(repo *mockSessionRepoForCharacter) {
				repo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return nil, errors.New("database error")
				}
			},
			expectedError: true,
		},
		{
			name:   "character with different class and race",
			chatID: 12345,
			setupMock: func(repo *mockSessionRepoForCharacter) {
				char, _ := character.NewCharacter("Elf Wizard", character.ClassWizard, character.RaceElf, character.Stats{
					Strength:     8,
					Dexterity:    14,
					Constitution: 10,
					Intelligence: 16,
					Wisdom:       12,
					Charisma:     13,
				})
				char.ID = 1
				char.Level = 1
				char.Experience = 0

				repo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
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
			},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if race, ok := resultMap["race"].(string); !ok || race != string(character.RaceElf) {
					t.Errorf("expected race='%s', got %v", character.RaceElf, resultMap["race"])
				}

				if class, ok := resultMap["class"].(string); !ok || class != string(character.ClassWizard) {
					t.Errorf("expected class='%s', got %v", character.ClassWizard, resultMap["class"])
				}

				stats, ok := resultMap["stats"].(map[string]interface{})
				if !ok {
					t.Fatalf("expected stats map, got %T", resultMap["stats"])
				}

				if intel, ok := stats["intelligence"].(int); !ok || intel != 16 {
					t.Errorf("expected intelligence=16, got %v", stats["intelligence"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockSessionRepoForCharacter{}
			if tt.setupMock != nil {
				tt.setupMock(repo)
			}

			tool := NewGetCharacterStatsTool(repo, tt.chatID)
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

// Helper function to create test game session with character
func createTestSessionWithCharacter(t *testing.T, char *character.Character) *session.GameSession {
	gs := &session.GameSession{
		ChatID: 12345,
		State:  session.StateActive,
		World:  world.World{Name: "Test World"},
		Players: []player.Player{
			{
				Character: *char,
			},
		},
	}
	gs.Model.ID = 1
	return gs
}

// Test RequestAbilityCheckTool
func TestRequestAbilityCheckTool_Name(t *testing.T) {
	tool := NewRequestAbilityCheckTool(nil, 1)
	if tool.Name() != "request_ability_check" {
		t.Errorf("expected name 'request_ability_check', got '%s'", tool.Name())
	}
}

func TestRequestAbilityCheckTool_Description(t *testing.T) {
	tool := NewRequestAbilityCheckTool(nil, 1)
	if tool.Description() == "" {
		t.Error("expected non-empty description")
	}
}

func TestRequestAbilityCheckTool_Execute(t *testing.T) {
	tests := []struct {
		name          string
		chatID        int64
		args          map[string]interface{}
		setupMock     func(*mockSessionRepoForCharacter)
		expectedError bool
		validate      func(*testing.T, interface{})
	}{
		{
			name:   "successful ability check without DC",
			chatID: 12345,
			args: map[string]interface{}{
				"ability": "strength",
			},
			setupMock: func(repo *mockSessionRepoForCharacter) {
				char, _ := character.NewCharacter("Test Hero", character.ClassFighter, character.RaceHuman, character.Stats{
					Strength: 16,
				})
				repo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return createTestSessionWithCharacter(t, char), nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if ability, ok := resultMap["ability"].(string); !ok || ability != "strength" {
					t.Errorf("expected ability='strength', got %v", resultMap["ability"])
				}

				if modifier, ok := resultMap["modifier"].(int); !ok || modifier != 3 {
					t.Errorf("expected modifier=3 for STR 16, got %v", modifier)
				}
			},
		},
		{
			name:   "successful ability check with DC",
			chatID: 12345,
			args: map[string]interface{}{
				"ability": "dexterity",
				"dc":      15,
			},
			setupMock: func(repo *mockSessionRepoForCharacter) {
				char, _ := character.NewCharacter("Test Hero", character.ClassRogue, character.RaceHuman, character.Stats{
					Dexterity: 14,
				})
				repo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return createTestSessionWithCharacter(t, char), nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if dc, ok := resultMap["dc"].(int); !ok || dc != 15 {
					t.Errorf("expected dc=15, got %v", resultMap["dc"])
				}

				if _, ok := resultMap["min_roll_to_succeed"]; !ok {
					t.Error("expected min_roll_to_succeed in result")
				}

				if _, ok := resultMap["success_chance_percent"]; !ok {
					t.Error("expected success_chance_percent in result")
				}
			},
		},
		{
			name:   "invalid ability",
			chatID: 12345,
			args: map[string]interface{}{
				"ability": "invalid",
			},
			setupMock: func(repo *mockSessionRepoForCharacter) {
				char, _ := character.NewCharacter("Test Hero", character.ClassFighter, character.RaceHuman, character.Stats{})
				repo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return createTestSessionWithCharacter(t, char), nil
				}
			},
			expectedError: true,
		},
		{
			name:   "missing ability parameter",
			chatID: 12345,
			args:   map[string]interface{}{},
			setupMock: func(repo *mockSessionRepoForCharacter) {
				repo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return nil, nil
				}
			},
			expectedError: true,
		},
		{
			name:   "session not found",
			chatID: 12345,
			args: map[string]interface{}{
				"ability": "strength",
			},
			setupMock: func(repo *mockSessionRepoForCharacter) {
				repo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return nil, nil
				}
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockSessionRepoForCharacter{}
			if tt.setupMock != nil {
				tt.setupMock(repo)
			}

			tool := NewRequestAbilityCheckTool(repo, tt.chatID)
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

// Test RequestSavingThrowTool
func TestRequestSavingThrowTool_Name(t *testing.T) {
	tool := NewRequestSavingThrowTool(nil, 1)
	if tool.Name() != "request_saving_throw" {
		t.Errorf("expected name 'request_saving_throw', got '%s'", tool.Name())
	}
}

func TestRequestSavingThrowTool_Execute(t *testing.T) {
	tests := []struct {
		name          string
		chatID        int64
		args          map[string]interface{}
		setupMock     func(*mockSessionRepoForCharacter)
		expectedError bool
		validate      func(*testing.T, interface{})
	}{
		{
			name:   "successful saving throw without DC",
			chatID: 12345,
			args: map[string]interface{}{
				"ability": "constitution",
			},
			setupMock: func(repo *mockSessionRepoForCharacter) {
				char, _ := character.NewCharacter("Test Hero", character.ClassFighter, character.RaceHuman, character.Stats{
					Constitution: 15,
				})
				char.Level = 1
				repo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return createTestSessionWithCharacter(t, char), nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if modifier, ok := resultMap["modifier"].(int); !ok || modifier != 2 {
					t.Errorf("expected modifier=2 for CON 15, got %v", modifier)
				}

				if savingThrowModifier, ok := resultMap["saving_throw_modifier"].(int); !ok || savingThrowModifier != 4 {
					t.Errorf("expected saving_throw_modifier=4 (modifier 2 + proficiency 2), got %v", savingThrowModifier)
				}
			},
		},
		{
			name:   "successful saving throw with DC",
			chatID: 12345,
			args: map[string]interface{}{
				"ability": "wisdom",
				"dc":      12,
			},
			setupMock: func(repo *mockSessionRepoForCharacter) {
				char, _ := character.NewCharacter("Test Hero", character.ClassCleric, character.RaceHuman, character.Stats{
					Wisdom: 13,
				})
				char.Level = 1
				repo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return createTestSessionWithCharacter(t, char), nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if dc, ok := resultMap["dc"].(int); !ok || dc != 12 {
					t.Errorf("expected dc=12, got %v", resultMap["dc"])
				}
			},
		},
		{
			name:   "session not found",
			chatID: 12345,
			args: map[string]interface{}{
				"ability": "constitution",
			},
			setupMock: func(repo *mockSessionRepoForCharacter) {
				repo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return nil, nil
				}
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockSessionRepoForCharacter{}
			if tt.setupMock != nil {
				tt.setupMock(repo)
			}

			tool := NewRequestSavingThrowTool(repo, tt.chatID)
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

// Test EvaluateCheckTool
func TestEvaluateCheckTool_Name(t *testing.T) {
	tool := NewEvaluateCheckTool(nil, 1)
	if tool.Name() != "evaluate_check" {
		t.Errorf("expected name 'evaluate_check', got '%s'", tool.Name())
	}
}

func TestEvaluateCheckTool_Execute(t *testing.T) {
	tests := []struct {
		name          string
		chatID        int64
		args          map[string]interface{}
		setupMock     func(*mockSessionRepoForCharacter)
		expectedError bool
		validate      func(*testing.T, interface{})
	}{
		{
			name:   "successful check evaluation - success",
			chatID: 12345,
			args: map[string]interface{}{
				"ability":     "strength",
				"dc":          15,
				"roll_result": 18,
			},
			setupMock: func(repo *mockSessionRepoForCharacter) {
				char, _ := character.NewCharacter("Test Hero", character.ClassFighter, character.RaceHuman, character.Stats{})
				repo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return createTestSessionWithCharacter(t, char), nil
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
			name:   "successful check evaluation - failure",
			chatID: 12345,
			args: map[string]interface{}{
				"ability":     "strength",
				"dc":          15,
				"roll_result": 12,
			},
			setupMock: func(repo *mockSessionRepoForCharacter) {
				char, _ := character.NewCharacter("Test Hero", character.ClassFighter, character.RaceHuman, character.Stats{})
				repo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return createTestSessionWithCharacter(t, char), nil
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
			name:   "missing required parameters",
			chatID: 12345,
			args:   map[string]interface{}{},
			setupMock: func(repo *mockSessionRepoForCharacter) {
				repo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return nil, nil
				}
			},
			expectedError: true,
		},
		{
			name:   "invalid ability",
			chatID: 12345,
			args: map[string]interface{}{
				"ability":     "invalid",
				"dc":          15,
				"roll_result": 18,
			},
			setupMock: func(repo *mockSessionRepoForCharacter) {
				char, _ := character.NewCharacter("Test Hero", character.ClassFighter, character.RaceHuman, character.Stats{})
				repo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return createTestSessionWithCharacter(t, char), nil
				}
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockSessionRepoForCharacter{}
			if tt.setupMock != nil {
				tt.setupMock(repo)
			}

			tool := NewEvaluateCheckTool(repo, tt.chatID)
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

// Test GetCharacterAbilitiesTool
func TestGetCharacterAbilitiesTool_Name(t *testing.T) {
	tool := NewGetCharacterAbilitiesTool(nil, 1)
	if tool.Name() != "get_character_abilities" {
		t.Errorf("expected name 'get_character_abilities', got '%s'", tool.Name())
	}
}

func TestGetCharacterAbilitiesTool_Execute(t *testing.T) {
	tests := []struct {
		name          string
		chatID        int64
		args          map[string]interface{}
		setupMock     func(*mockSessionRepoForCharacter)
		expectedError bool
		validate      func(*testing.T, interface{})
	}{
		{
			name:   "successful abilities retrieval - all",
			chatID: 12345,
			args: map[string]interface{}{
				"filter_type": "all",
			},
			setupMock: func(repo *mockSessionRepoForCharacter) {
				char, _ := character.NewCharacter("Test Hero", character.ClassFighter, character.RaceHuman, character.Stats{})
				char.Level = 1
				repo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return createTestSessionWithCharacter(t, char), nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				abilities, ok := resultMap["abilities"].([]interface{})
				if !ok {
					t.Fatalf("expected abilities list, got %T", resultMap["abilities"])
				}

				if len(abilities) == 0 {
					t.Error("expected at least one ability for Fighter")
				}
			},
		},
		{
			name:   "successful abilities retrieval - spells filter",
			chatID: 12345,
			args: map[string]interface{}{
				"filter_type": "spells",
			},
			setupMock: func(repo *mockSessionRepoForCharacter) {
				char, _ := character.NewCharacter("Test Wizard", character.ClassWizard, character.RaceElf, character.Stats{})
				char.Level = 1
				repo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return createTestSessionWithCharacter(t, char), nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				abilities, ok := resultMap["abilities"].([]interface{})
				if !ok {
					t.Fatalf("expected abilities list, got %T", resultMap["abilities"])
				}

				// Wizard should have spells
				hasSpell := false
				for _, ab := range abilities {
					abMap, ok := ab.(map[string]interface{})
					if !ok {
						continue
					}
					if abType, ok := abMap["type"].(string); ok && abType == "spell" {
						hasSpell = true
						break
					}
				}
				if !hasSpell {
					t.Error("expected at least one spell for Wizard")
				}
			},
		},
		{
			name:   "session not found",
			chatID: 12345,
			args:   map[string]interface{}{},
			setupMock: func(repo *mockSessionRepoForCharacter) {
				repo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return nil, nil
				}
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockSessionRepoForCharacter{}
			if tt.setupMock != nil {
				tt.setupMock(repo)
			}

			tool := NewGetCharacterAbilitiesTool(repo, tt.chatID)
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
