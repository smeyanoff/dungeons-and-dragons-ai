package quest

import (
	"context"
	"errors"
	"strings"
	"testing"

	"dungeons-and-dragons-ai/internal/game/domain/item"
	"dungeons-and-dragons-ai/internal/game/domain/quest"
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

// Mock Quest Repository
type mockQuestRepo struct {
	getByWorldIDFunc func(ctx context.Context, worldID uint) ([]*quest.Quest, error)
	saveFunc         func(ctx context.Context, q *quest.Quest) error
}

func (m *mockQuestRepo) GetByWorldID(ctx context.Context, worldID uint) ([]*quest.Quest, error) {
	if m.getByWorldIDFunc != nil {
		return m.getByWorldIDFunc(ctx, worldID)
	}
	return nil, nil
}

func (m *mockQuestRepo) Save(ctx context.Context, q *quest.Quest) error {
	if m.saveFunc != nil {
		return m.saveFunc(ctx, q)
	}
	return nil
}

func TestGetQuestsUseCase_Execute(t *testing.T) {
	tests := []struct {
		name          string
		chatID        int64
		setupMocks    func(*mockSessionRepo, *mockQuestRepo)
		expectedError bool
		validate      func(*testing.T, string)
	}{
		{
			name:   "successful quest retrieval",
			chatID: 12345,
			setupMocks: func(sessionRepo *mockSessionRepo, questRepo *mockQuestRepo) {
				mainQuest := quest.New("Спасти королевство", "Победить дракона")
				mainQuest.SetExperienceReward(500)
				mainQuest.AddItem(item.New("Меч дракона", "Легендарное оружие"))

				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					gs := &session.GameSession{
						ChatID: chatID,
						State:  session.StateActive,
						World: world.World{
							Name:      "Test World",
							MainQuest: mainQuest,
						},
					}
					gs.Model.ID = 1
					return gs, nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result string) {
				if result == "" {
					t.Error("expected non-empty result")
				}
				if !strings.Contains(result, "Активные квесты") {
					t.Error("result should contain 'Активные квесты'")
				}
				if !strings.Contains(result, "Спасти королевство") {
					t.Error("result should contain quest title")
				}
				if !strings.Contains(result, "Победить дракона") {
					t.Error("result should contain quest description")
				}
			},
		},
		{
			name:   "no session",
			chatID: 12345,
			setupMocks: func(sessionRepo *mockSessionRepo, questRepo *mockQuestRepo) {
				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return nil, nil
				}
			},
			expectedError: true,
		},
		{
			name:   "inactive session",
			chatID: 12345,
			setupMocks: func(sessionRepo *mockSessionRepo, questRepo *mockQuestRepo) {
				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					gs := &session.GameSession{
						ChatID: chatID,
						State:  session.StateDone,
					}
					gs.Model.ID = 1
					return gs, nil
				}
			},
			expectedError: true,
		},
		{
			name:   "no main quest",
			chatID: 12345,
			setupMocks: func(sessionRepo *mockSessionRepo, questRepo *mockQuestRepo) {
				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					gs := &session.GameSession{
						ChatID: chatID,
						State:  session.StateActive,
						World: world.World{
							Name:      "Test World",
							MainQuest: nil,
						},
					}
					gs.Model.ID = 1
					return gs, nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result string) {
				if !strings.Contains(result, "Нет активных квестов") {
					t.Errorf("expected no quests message, got: %s", result)
				}
			},
		},
		{
			name:   "completed quest",
			chatID: 12345,
			setupMocks: func(sessionRepo *mockSessionRepo, questRepo *mockQuestRepo) {
				mainQuest := quest.New("Спасти королевство", "Победить дракона")
				mainQuest.Complete()

				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					gs := &session.GameSession{
						ChatID: chatID,
						State:  session.StateActive,
						World: world.World{
							Name:      "Test World",
							MainQuest: mainQuest,
						},
					}
					gs.Model.ID = 1
					return gs, nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result string) {
				// Завершенные квесты не должны показываться
				if !strings.Contains(result, "Нет активных квестов") {
					t.Errorf("completed quest should not be shown, got: %s", result)
				}
			},
		},
		{
			name:   "quest with items",
			chatID: 12345,
			setupMocks: func(sessionRepo *mockSessionRepo, questRepo *mockQuestRepo) {
				mainQuest := quest.New("Найти артефакт", "Найти древний артефакт")
				mainQuest.AddItem(item.New("Ключ", "Открывает дверь"))
				mainQuest.AddItem(item.New("Карта", "Показывает путь"))

				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					gs := &session.GameSession{
						ChatID: chatID,
						State:  session.StateActive,
						World: world.World{
							Name:      "Test World",
							MainQuest: mainQuest,
						},
					}
					gs.Model.ID = 1
					return gs, nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result string) {
				if !strings.Contains(result, "Предметы") {
					t.Error("result should contain items section")
				}
			},
		},
		{
			name:   "session repo error",
			chatID: 12345,
			setupMocks: func(sessionRepo *mockSessionRepo, questRepo *mockQuestRepo) {
				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return nil, errors.New("database error")
				}
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessionRepo := &mockSessionRepo{}
			questRepo := &mockQuestRepo{}

			if tt.setupMocks != nil {
				tt.setupMocks(sessionRepo, questRepo)
			}

			uc := NewGetQuestsUseCase(sessionRepo, questRepo)

			result, err := uc.Execute(context.Background(), tt.chatID)

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
