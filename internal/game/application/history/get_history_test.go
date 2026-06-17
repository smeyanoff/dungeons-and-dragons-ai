package history

import (
	"context"
	"errors"
	"strings"
	"testing"

	"dungeons-and-dragons-ai/internal/game/domain/event"
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

// Mock Event Repository
type mockEventRepo struct {
	getBySessionIDFunc func(ctx context.Context, sessionID uint, limit int) ([]event.StoryEvent, error)
}

func (m *mockEventRepo) GetBySessionID(ctx context.Context, sessionID uint, limit int) ([]event.StoryEvent, error) {
	if m.getBySessionIDFunc != nil {
		return m.getBySessionIDFunc(ctx, sessionID, limit)
	}
	return nil, nil
}

func TestGetHistoryUseCase_Execute(t *testing.T) {
	tests := []struct {
		name          string
		chatID        int64
		limit         int
		setupMocks    func(*mockSessionRepo, *mockEventRepo)
		expectedError bool
		validate      func(*testing.T, string)
	}{
		{
			name:   "successful history retrieval",
			chatID: 12345,
			limit:  10,
			setupMocks: func(sessionRepo *mockSessionRepo, eventRepo *mockEventRepo) {
				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					gs := &session.GameSession{
						ChatID: chatID,
						State:  session.StateActive,
						World:  world.World{Name: "Test World"},
					}
					gs.Model.ID = 1
					return gs, nil
				}
				eventRepo.getBySessionIDFunc = func(ctx context.Context, sessionID uint, limit int) ([]event.StoryEvent, error) {
					return []event.StoryEvent{
						{
							ID:            1,
							GameSessionID: 1,
							AuthorType:    event.AuthorTypePlayer,
							Content:       "Иду на север",
						},
						{
							ID:            2,
							GameSessionID: 1,
							AuthorType:    event.AuthorTypeDM,
							Content:       "Вы видите замок",
						},
					}, nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result string) {
				if result == "" {
					t.Error("expected non-empty result")
				}
				if !strings.Contains(result, "История игры") {
					t.Error("result should contain 'История игры'")
				}
				if !strings.Contains(result, "Иду на север") {
					t.Error("result should contain player message")
				}
				if !strings.Contains(result, "Вы видите замок") {
					t.Error("result should contain DM message")
				}
			},
		},
		{
			name:   "no session",
			chatID: 12345,
			limit:  10,
			setupMocks: func(sessionRepo *mockSessionRepo, eventRepo *mockEventRepo) {
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
			name:   "empty history",
			chatID: 12345,
			limit:  10,
			setupMocks: func(sessionRepo *mockSessionRepo, eventRepo *mockEventRepo) {
				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					gs := &session.GameSession{
						ChatID: chatID,
						State:  session.StateActive,
					}
					gs.Model.ID = 1
					return gs, nil
				}
				eventRepo.getBySessionIDFunc = func(ctx context.Context, sessionID uint, limit int) ([]event.StoryEvent, error) {
					return []event.StoryEvent{}, nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result string) {
				if !strings.Contains(result, "История игры пуста") {
					t.Errorf("expected empty history message, got: %s", result)
				}
			},
		},
		{
			name:   "NPC event",
			chatID: 12345,
			limit:  10,
			setupMocks: func(sessionRepo *mockSessionRepo, eventRepo *mockEventRepo) {
				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					gs := &session.GameSession{
						ChatID: chatID,
						State:  session.StateActive,
					}
					gs.Model.ID = 1
					return gs, nil
				}
				eventRepo.getBySessionIDFunc = func(ctx context.Context, sessionID uint, limit int) ([]event.StoryEvent, error) {
					return []event.StoryEvent{
						{
							ID:            1,
							GameSessionID: 1,
							AuthorType:    event.AuthorTypeNPC,
							AuthorName:    "Мэр",
							Content:       "Добро пожаловать в город!",
						},
					}, nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result string) {
				if !strings.Contains(result, "Мэр") {
					t.Error("result should contain NPC name")
				}
				if !strings.Contains(result, "Добро пожаловать в город!") {
					t.Error("result should contain NPC message")
				}
			},
		},
		{
			name:   "session repo error",
			chatID: 12345,
			limit:  10,
			setupMocks: func(sessionRepo *mockSessionRepo, eventRepo *mockEventRepo) {
				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return nil, errors.New("database error")
				}
			},
			expectedError: true,
		},
		{
			name:   "event repo error",
			chatID: 12345,
			limit:  10,
			setupMocks: func(sessionRepo *mockSessionRepo, eventRepo *mockEventRepo) {
				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					gs := &session.GameSession{
						ChatID: chatID,
						State:  session.StateActive,
					}
					gs.Model.ID = 1
					return gs, nil
				}
				eventRepo.getBySessionIDFunc = func(ctx context.Context, sessionID uint, limit int) ([]event.StoryEvent, error) {
					return nil, errors.New("event repo error")
				}
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessionRepo := &mockSessionRepo{}
			eventRepo := &mockEventRepo{}

			if tt.setupMocks != nil {
				tt.setupMocks(sessionRepo, eventRepo)
			}

			uc := NewGetHistoryUseCase(sessionRepo, eventRepo)

			result, err := uc.Execute(context.Background(), tt.chatID, tt.limit)

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
