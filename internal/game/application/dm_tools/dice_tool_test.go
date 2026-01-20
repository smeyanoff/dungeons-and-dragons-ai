package dm_tools

import (
	"context"
	"errors"
	"testing"

	"dungeons-and-dragons-ai/internal/game/application/dice"
	"dungeons-and-dragons-ai/internal/game/domain/event"
)

// mockDiceEventRepository мок для DiceEventRepository
type mockDiceEventRepository struct {
	saveFunc func(ctx context.Context, e *event.StoryEvent) error
}

func (m *mockDiceEventRepository) Save(ctx context.Context, e *event.StoryEvent) error {
	if m.saveFunc != nil {
		return m.saveFunc(ctx, e)
	}
	return nil
}

func TestRollDiceTool_Name(t *testing.T) {
	rollDiceUC := dice.NewRollDiceUseCase()
	tool := NewRollDiceTool(rollDiceUC, nil, 1)
	if tool.Name() != "roll_dice" {
		t.Errorf("expected name 'roll_dice', got '%s'", tool.Name())
	}
}

func TestRollDiceTool_Description(t *testing.T) {
	rollDiceUC := dice.NewRollDiceUseCase()
	tool := NewRollDiceTool(rollDiceUC, nil, 1)
	if tool.Description() == "" {
		t.Error("expected non-empty description")
	}
}

func TestRollDiceTool_Parameters(t *testing.T) {
	rollDiceUC := dice.NewRollDiceUseCase()
	tool := NewRollDiceTool(rollDiceUC, nil, 1)
	params := tool.Parameters()
	if len(params) == 0 {
		t.Error("expected non-empty parameters")
	}
}

func TestRollDiceTool_Execute(t *testing.T) {
	tests := []struct {
		name          string
		sessionID    uint
		args          map[string]interface{}
		setupMock     func(*mockDiceEventRepository)
		expectedError bool
		validate      func(*testing.T, interface{})
	}{
		{
			name:       "successful dice roll with d20",
			sessionID:  123,
			args:       map[string]interface{}{"dice_expression": "d20"},
			setupMock:  func(repo *mockDiceEventRepository) {},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if expr, ok := resultMap["dice_expression"].(string); !ok || expr != "d20" {
					t.Errorf("expected dice_expression='d20', got %v", resultMap["dice_expression"])
				}

				if resultText, ok := resultMap["result_text"].(string); !ok || resultText == "" {
					t.Error("expected non-empty result_text")
				}

				if message, ok := resultMap["message"].(string); !ok || message == "" {
					t.Error("expected non-empty message")
				}
			},
		},
		{
			name:       "successful dice roll with default d20 (no expression)",
			sessionID:  123,
			args:       map[string]interface{}{},
			setupMock:  func(repo *mockDiceEventRepository) {},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if expr, ok := resultMap["dice_expression"].(string); !ok || expr != "d20" {
					t.Errorf("expected dice_expression='d20' (default), got %v", resultMap["dice_expression"])
				}
			},
		},
		{
			name:       "successful dice roll with 2d6+3",
			sessionID:  123,
			args:       map[string]interface{}{"dice_expression": "2d6+3"},
			setupMock:  func(repo *mockDiceEventRepository) {},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if expr, ok := resultMap["dice_expression"].(string); !ok || expr != "2d6+3" {
					t.Errorf("expected dice_expression='2d6+3', got %v", resultMap["dice_expression"])
				}
			},
		},
		{
			name:       "saves event to repository",
			sessionID:  123,
			args:       map[string]interface{}{"dice_expression": "d20"},
			setupMock: func(repo *mockDiceEventRepository) {
				repo.saveFunc = func(ctx context.Context, e *event.StoryEvent) error {
					if e.GameSessionID != 123 {
						t.Errorf("expected GameSessionID=123, got %d", e.GameSessionID)
					}
					if e.AuthorType != event.AuthorTypeDM {
						t.Errorf("expected AuthorType=DM, got %v", e.AuthorType)
					}
					if e.Content == "" {
						t.Error("expected non-empty Content")
					}
					return nil
				}
			},
			expectedError: false,
		},
		{
			name:       "handles repository save error gracefully",
			sessionID:  123,
			args:       map[string]interface{}{"dice_expression": "d20"},
			setupMock: func(repo *mockDiceEventRepository) {
				repo.saveFunc = func(ctx context.Context, e *event.StoryEvent) error {
					return errors.New("database error")
				}
			},
			expectedError: false, // Ошибка сохранения не должна прерывать выполнение
		},
		{
			name:       "invalid dice expression",
			sessionID:  123,
			args:       map[string]interface{}{"dice_expression": "invalid"},
			setupMock:  func(repo *mockDiceEventRepository) {},
			expectedError: true,
		},
		{
			name:       "works without event repository",
			sessionID:  123,
			args:       map[string]interface{}{"dice_expression": "d20"},
			setupMock:  nil, // nil repository
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}
				if resultMap["dice_expression"] != "d20" {
					t.Error("expected dice_expression='d20'")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rollDiceUC := dice.NewRollDiceUseCase()
			var eventRepo DiceEventRepository
			if tt.setupMock != nil {
				mockRepo := &mockDiceEventRepository{}
				tt.setupMock(mockRepo)
				eventRepo = mockRepo
			}

			tool := NewRollDiceTool(rollDiceUC, eventRepo, tt.sessionID)
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
