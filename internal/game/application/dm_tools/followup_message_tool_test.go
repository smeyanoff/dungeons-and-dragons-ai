package dm_tools

import (
	"context"
	"errors"
	"testing"

	"dungeons-and-dragons-ai/internal/game/domain/event"
)

// mockFollowupEventRepository мок для FollowupEventRepository
type mockFollowupEventRepository struct {
	saveFunc func(ctx context.Context, e *event.StoryEvent) error
}

func (m *mockFollowupEventRepository) Save(ctx context.Context, e *event.StoryEvent) error {
	if m.saveFunc != nil {
		return m.saveFunc(ctx, e)
	}
	return nil
}

func TestSendFollowupMessageTool_Name(t *testing.T) {
	tool := NewSendFollowupMessageTool(nil, 1, 123, nil)
	if tool.Name() != "send_followup_message" {
		t.Errorf("expected name 'send_followup_message', got '%s'", tool.Name())
	}
}

func TestSendFollowupMessageTool_Description(t *testing.T) {
	tool := NewSendFollowupMessageTool(nil, 1, 123, nil)
	if tool.Description() == "" {
		t.Error("expected non-empty description")
	}
}

func TestSendFollowupMessageTool_Parameters(t *testing.T) {
	tool := NewSendFollowupMessageTool(nil, 1, 123, nil)
	params := tool.Parameters()
	if len(params) == 0 {
		t.Error("expected non-empty parameters")
	}
}

func TestSendFollowupMessageTool_Execute(t *testing.T) {
	tests := []struct {
		name          string
		sessionID     uint
		chatID        int64
		args          map[string]interface{}
		setupMock     func(*mockFollowupEventRepository)
		expectedError bool
		validate      func(*testing.T, interface{})
	}{
		{
			name:      "successful followup message creation",
			sessionID: 123,
			chatID:    456,
			args: map[string]interface{}{
				"message_type": "item_description",
				"context":      "Золотой меч найден в сундуке",
			},
			setupMock:     func(repo *mockFollowupEventRepository) {},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if msgType, ok := resultMap["message_type"].(string); !ok || msgType != "item_description" {
					t.Errorf("expected message_type='item_description', got %v", resultMap["message_type"])
				}

				if context, ok := resultMap["context"].(string); !ok || context != "Золотой меч найден в сундуке" {
					t.Errorf("expected context='Золотой меч найден в сундуке', got %v", resultMap["context"])
				}

				if status, ok := resultMap["status"].(string); !ok || status != "followup_request_created" {
					t.Errorf("expected status='followup_request_created', got %v", resultMap["status"])
				}

				if message, ok := resultMap["message"].(string); !ok || message == "" {
					t.Error("expected non-empty message")
				}
			},
		},
		{
			name:      "saves followup event to repository",
			sessionID: 123,
			chatID:    456,
			args: map[string]interface{}{
				"message_type": "combat_turn",
				"context":      "Враг Гоблин атакует",
			},
			setupMock: func(repo *mockFollowupEventRepository) {
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
					// Проверяем, что Content содержит правильный формат
					if len(e.Content) < 20 {
						t.Error("expected Content to contain followup request format")
					}
					return nil
				}
			},
			expectedError: false,
		},
		{
			name:      "different message types",
			sessionID: 123,
			chatID:    456,
			args: map[string]interface{}{
				"message_type": "spell_effect",
				"context":      "Заклинание Огненный шар применено",
			},
			setupMock:     func(repo *mockFollowupEventRepository) {},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if msgType, ok := resultMap["message_type"].(string); !ok || msgType != "spell_effect" {
					t.Errorf("expected message_type='spell_effect', got %v", resultMap["message_type"])
				}
			},
		},
		{
			name:      "missing message_type",
			sessionID: 123,
			chatID:    456,
			args: map[string]interface{}{
				"context": "Some context",
			},
			setupMock:     func(repo *mockFollowupEventRepository) {},
			expectedError: true,
		},
		{
			name:      "missing context",
			sessionID: 123,
			chatID:    456,
			args: map[string]interface{}{
				"message_type": "item_description",
			},
			setupMock:     func(repo *mockFollowupEventRepository) {},
			expectedError: true,
		},
		{
			name:      "empty message_type",
			sessionID: 123,
			chatID:    456,
			args: map[string]interface{}{
				"message_type": "",
				"context":      "Some context",
			},
			setupMock:     func(repo *mockFollowupEventRepository) {},
			expectedError: true,
		},
		{
			name:      "empty context",
			sessionID: 123,
			chatID:    456,
			args: map[string]interface{}{
				"message_type": "item_description",
				"context":      "",
			},
			setupMock:     func(repo *mockFollowupEventRepository) {},
			expectedError: true,
		},
		{
			name:      "repository save error",
			sessionID: 123,
			chatID:    456,
			args: map[string]interface{}{
				"message_type": "item_description",
				"context":      "Some context",
			},
			setupMock: func(repo *mockFollowupEventRepository) {
				repo.saveFunc = func(ctx context.Context, e *event.StoryEvent) error {
					return errors.New("database error")
				}
			},
			expectedError: true, // Ошибка сохранения должна прерывать выполнение
		},
		{
			name:      "location_detail message type",
			sessionID: 123,
			chatID:    456,
			args: map[string]interface{}{
				"message_type": "location_detail",
				"context":      "Детальное описание таверны",
			},
			setupMock:     func(repo *mockFollowupEventRepository) {},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if msgType, ok := resultMap["message_type"].(string); !ok || msgType != "location_detail" {
					t.Errorf("expected message_type='location_detail', got %v", resultMap["message_type"])
				}
			},
		},
		{
			name:      "npc_dialogue message type",
			sessionID: 123,
			chatID:    456,
			args: map[string]interface{}{
				"message_type": "npc_dialogue",
				"context":      "Диалог с торговцем",
			},
			setupMock:     func(repo *mockFollowupEventRepository) {},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if msgType, ok := resultMap["message_type"].(string); !ok || msgType != "npc_dialogue" {
					t.Errorf("expected message_type='npc_dialogue', got %v", resultMap["message_type"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var eventRepo FollowupEventRepository
			if tt.setupMock != nil {
				mockRepo := &mockFollowupEventRepository{}
				tt.setupMock(mockRepo)
				eventRepo = mockRepo
			}

			tool := NewSendFollowupMessageTool(eventRepo, tt.sessionID, tt.chatID, nil)
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
