package player_action

import (
	"context"
	"errors"
	"testing"
	"time"

	characterapp "dungeons-and-dragons-ai/internal/game/application/character"
	"dungeons-and-dragons-ai/internal/game/application/dm_tools"
	worldeventapp "dungeons-and-dragons-ai/internal/game/application/world_event"
	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/combat"
	"dungeons-and-dragons-ai/internal/game/domain/event"
	"dungeons-and-dragons-ai/internal/game/domain/inventory"
	"dungeons-and-dragons-ai/internal/game/domain/player"
	"dungeons-and-dragons-ai/internal/game/domain/quest"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/game/domain/world"
	"dungeons-and-dragons-ai/internal/llm/domain"
	ragapp "dungeons-and-dragons-ai/internal/rag/application"
	ragdomain "dungeons-and-dragons-ai/internal/rag/domain"
)

// Mock Embedder
type mockEmbedder struct {
	embedFunc func(ctx context.Context, text string) ([]float32, error)
}

func (m *mockEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	if m.embedFunc != nil {
		return m.embedFunc(ctx, text)
	}
	return []float32{0.1, 0.2, 0.3}, nil
}

// Mock VectorStore
type mockVectorStore struct {
	ensureCollectionFunc func(ctx context.Context) error
	upsertFunc           func(ctx context.Context, doc ragdomain.Document, embedding []float32) error
	searchFunc           func(ctx context.Context, sessionID uint, embedding []float32, limit int) ([]ragdomain.Document, error)
}

func (m *mockVectorStore) EnsureCollection(ctx context.Context) error {
	if m.ensureCollectionFunc != nil {
		return m.ensureCollectionFunc(ctx)
	}
	return nil
}

func (m *mockVectorStore) Upsert(ctx context.Context, doc ragdomain.Document, embedding []float32) error {
	if m.upsertFunc != nil {
		return m.upsertFunc(ctx, doc, embedding)
	}
	return nil
}

func (m *mockVectorStore) Search(ctx context.Context, sessionID uint, embedding []float32, limit int) ([]ragdomain.Document, error) {
	if m.searchFunc != nil {
		return m.searchFunc(ctx, sessionID, embedding, limit)
	}
	return nil, nil
}

// Mock LLM
type mockLLM struct {
	generateFunc              func(ctx context.Context, prompt string) (string, error)
	generateWithMaxTokensFunc func(ctx context.Context, prompt string, maxTokens int) (string, error)
	generateWithToolsFunc     func(ctx context.Context, prompt string, tools []dm_tools.Tool) (*domain.LLMResponseWithTools, error)
}

func (m *mockLLM) Generate(ctx context.Context, prompt string) (string, error) {
	if m.generateFunc != nil {
		return m.generateFunc(ctx, prompt)
	}
	return "Test DM response", nil
}

func (m *mockLLM) GenerateWithMaxTokens(ctx context.Context, prompt string, maxTokens int) (string, error) {
	if m.generateWithMaxTokensFunc != nil {
		return m.generateWithMaxTokensFunc(ctx, prompt, maxTokens)
	}
	// Fallback на обычный Generate для обратной совместимости
	if m.generateFunc != nil {
		return m.generateFunc(ctx, prompt)
	}
	return "Test DM response", nil
}

func (m *mockLLM) GenerateWithTools(ctx context.Context, prompt string, tools []dm_tools.Tool) (*domain.LLMResponseWithTools, error) {
	if m.generateWithToolsFunc != nil {
		return m.generateWithToolsFunc(ctx, prompt, tools)
	}
	// Fallback на обычный Generate для обратной совместимости
	content, err := m.Generate(ctx, prompt)
	if err != nil {
		return nil, err
	}
	return &domain.LLMResponseWithTools{
		Content:   content,
		ToolCalls: nil,
		Finished:  true,
	}, nil
}

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

// Mock Context Builder
type mockContextBuilder struct {
	buildContextFunc func(ctx context.Context, gs *session.GameSession, playerMessage string) (string, error)
}

func (m *mockContextBuilder) BuildContext(ctx context.Context, gs *session.GameSession, playerMessage string) (string, error) {
	if m.buildContextFunc != nil {
		return m.buildContextFunc(ctx, gs, playerMessage)
	}
	return "Test context", nil
}

// Mock Event Repository
type mockEventRepo struct {
	saveFunc func(ctx context.Context, e *event.StoryEvent) error
	events   []*event.StoryEvent
}

func (m *mockEventRepo) Save(ctx context.Context, e *event.StoryEvent) error {
	if m.saveFunc != nil {
		return m.saveFunc(ctx, e)
	}
	if m.events == nil {
		m.events = make([]*event.StoryEvent, 0)
	}
	m.events = append(m.events, e)
	return nil
}

// Mock Combat Repository
type mockCombatRepo struct {
	saveFunc                 func(ctx context.Context, c *combat.Combat) error
	getActiveBySessionIDFunc func(ctx context.Context, sessionID uint) (*combat.Combat, error)
}

func (m *mockCombatRepo) Save(ctx context.Context, c *combat.Combat) error {
	if m.saveFunc != nil {
		return m.saveFunc(ctx, c)
	}
	return nil
}

func (m *mockCombatRepo) GetActiveBySessionID(ctx context.Context, sessionID uint) (*combat.Combat, error) {
	if m.getActiveBySessionIDFunc != nil {
		return m.getActiveBySessionIDFunc(ctx, sessionID)
	}
	return nil, nil
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

// Mock Player Repository for AddExperienceUseCase
type mockPlayerRepo struct {
	getByTgUserIDAndSessionIDFunc func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error)
	saveFunc                      func(ctx context.Context, p *player.Player) error
}

func (m *mockPlayerRepo) GetByTgUserIDAndSessionID(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
	if m.getByTgUserIDAndSessionIDFunc != nil {
		return m.getByTgUserIDAndSessionIDFunc(ctx, tgUserID, sessionID)
	}
	return nil, nil
}

func (m *mockPlayerRepo) Save(ctx context.Context, p *player.Player) error {
	if m.saveFunc != nil {
		return m.saveFunc(ctx, p)
	}
	return nil
}

// Mock World Event Repository
type mockWorldEventRepo struct {
	getScheduledByWorldIDFunc func(ctx context.Context, worldID uint) ([]world.WorldEvent, error)
	getActiveByWorldIDFunc    func(ctx context.Context, worldID uint) ([]world.WorldEvent, error)
	saveFunc                  func(ctx context.Context, e *world.WorldEvent) error
}

func (m *mockWorldEventRepo) GetScheduledByWorldID(ctx context.Context, worldID uint) ([]world.WorldEvent, error) {
	if m.getScheduledByWorldIDFunc != nil {
		return m.getScheduledByWorldIDFunc(ctx, worldID)
	}
	return []world.WorldEvent{}, nil
}

func (m *mockWorldEventRepo) GetActiveByWorldID(ctx context.Context, worldID uint) ([]world.WorldEvent, error) {
	if m.getActiveByWorldIDFunc != nil {
		return m.getActiveByWorldIDFunc(ctx, worldID)
	}
	return []world.WorldEvent{}, nil
}

func (m *mockWorldEventRepo) Save(ctx context.Context, e *world.WorldEvent) error {
	if m.saveFunc != nil {
		return m.saveFunc(ctx, e)
	}
	return nil
}

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

// Mock Check World Events Use Case - используем реальный use case с моковым репозиторием
func newMockCheckWorldEventsUC() *worldeventapp.CheckWorldEventsUseCase {
	repo := &mockWorldEventRepo{}
	return worldeventapp.NewCheckWorldEventsUseCase(repo)
}

func TestHandleActionUseCase_Execute(t *testing.T) {
	tests := []struct {
		name           string
		chatID         int64
		playerMessage  string
		setupMocks     func(*mockLLM, *mockSessionRepo, *mockContextBuilder, *mockEventRepo, *mockWorldEventRepo)
		expectedError  bool
		expectedResult string
	}{
		{
			name:          "successful action",
			chatID:        12345,
			playerMessage: "Иду на север",
			setupMocks: func(llm *mockLLM, sessionRepo *mockSessionRepo, ctxBuilder *mockContextBuilder, eventRepo *mockEventRepo, worldEventRepo *mockWorldEventRepo) {
				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return &session.GameSession{
						ChatID:  chatID,
						State:   session.StateActive,
						WorldID: 1,
						World:   world.World{ID: 1, Name: "Test World", StartedAt: time.Now()},
						Players: []player.Player{
							{
								Character: character.Character{
									Name:  "Test Hero",
									Race:  character.RaceHuman,
									Class: character.ClassFighter,
								},
							},
						},
					}, nil
				}
				llm.generateFunc = func(ctx context.Context, prompt string) (string, error) {
					return "Вы идете на север и видите замок", nil
				}
			},
			expectedError:  false,
			expectedResult: "Вы идете на север и видите замок",
		},
		{
			name:          "no session",
			chatID:        12345,
			playerMessage: "Иду на север",
			setupMocks: func(llm *mockLLM, sessionRepo *mockSessionRepo, ctxBuilder *mockContextBuilder, eventRepo *mockEventRepo, worldEventRepo *mockWorldEventRepo) {
				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return nil, nil
				}
			},
			expectedError:  false,
			expectedResult: "Игра не начата. Используйте /newgame для начала новой игры.",
		},
		{
			name:          "inactive session",
			chatID:        12345,
			playerMessage: "Иду на север",
			setupMocks: func(llm *mockLLM, sessionRepo *mockSessionRepo, ctxBuilder *mockContextBuilder, eventRepo *mockEventRepo, worldEventRepo *mockWorldEventRepo) {
				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return &session.GameSession{
						ChatID: chatID,
						State:  session.StateDone,
					}, nil
				}
			},
			expectedError:  false,
			expectedResult: "Игра завершена. Используйте /newgame для начала новой игры.",
		},
		{
			name:          "no character - returns error message",
			chatID:        12345,
			playerMessage: "Иду на север",
			setupMocks: func(llm *mockLLM, sessionRepo *mockSessionRepo, ctxBuilder *mockContextBuilder, eventRepo *mockEventRepo, worldEventRepo *mockWorldEventRepo) {
				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return &session.GameSession{
						ChatID:  chatID,
						State:   session.StateActive,
						WorldID: 1,
						World:   world.World{ID: 1, Name: "Test World", StartedAt: time.Now()},
						Players: []player.Player{}, // Empty players list - no character created
					}, nil
				}
			},
			expectedError:  false,
			expectedResult: "Персонаж не создан. Используйте /createcharacter для создания персонажа.",
		},
		{
			name:          "session repo error",
			chatID:        12345,
			playerMessage: "Иду на север",
			setupMocks: func(llm *mockLLM, sessionRepo *mockSessionRepo, ctxBuilder *mockContextBuilder, eventRepo *mockEventRepo, worldEventRepo *mockWorldEventRepo) {
				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return nil, errors.New("database error")
				}
			},
			expectedError: true,
		},
		{
			name:          "context builder error",
			chatID:        12345,
			playerMessage: "Иду на север",
			setupMocks: func(llm *mockLLM, sessionRepo *mockSessionRepo, ctxBuilder *mockContextBuilder, eventRepo *mockEventRepo, worldEventRepo *mockWorldEventRepo) {
				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return &session.GameSession{
						ChatID:  chatID,
						State:   session.StateActive,
						WorldID: 1,
						World:   world.World{ID: 1, StartedAt: time.Now()},
						Players: []player.Player{
							{
								Character: character.Character{
									Name:  "Test Hero",
									Race:  character.RaceHuman,
									Class: character.ClassFighter,
								},
							},
						},
					}, nil
				}
				ctxBuilder.buildContextFunc = func(ctx context.Context, gs *session.GameSession, playerMessage string) (string, error) {
					return "", errors.New("context builder error")
				}
			},
			expectedError: true,
		},
		{
			name:          "LLM error - player event still saved",
			chatID:        12345,
			playerMessage: "Иду на север",
			setupMocks: func(llm *mockLLM, sessionRepo *mockSessionRepo, ctxBuilder *mockContextBuilder, eventRepo *mockEventRepo, worldEventRepo *mockWorldEventRepo) {
				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return &session.GameSession{
						ChatID:  chatID,
						State:   session.StateActive,
						WorldID: 1,
						World:   world.World{ID: 1, Name: "Test World", StartedAt: time.Now()},
						Players: []player.Player{
							{
								Character: character.Character{
									Name:  "Test Hero",
									Race:  character.RaceHuman,
									Class: character.ClassFighter,
								},
							},
						},
					}, nil
				}
				llm.generateFunc = func(ctx context.Context, prompt string) (string, error) {
					return "", errors.New("LLM error")
				}
			},
			expectedError: true,
		},
		{
			name:          "world events checked",
			chatID:        12345,
			playerMessage: "Иду на север",
			setupMocks: func(llm *mockLLM, sessionRepo *mockSessionRepo, ctxBuilder *mockContextBuilder, eventRepo *mockEventRepo, worldEventRepo *mockWorldEventRepo) {
				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return &session.GameSession{
						ChatID:  chatID,
						State:   session.StateActive,
						WorldID: 1,
						World:   world.World{ID: 1, Name: "Test World", StartedAt: time.Now()},
						Players: []player.Player{
							{
								Character: character.Character{
									Name:  "Test Hero",
									Race:  character.RaceHuman,
									Class: character.ClassFighter,
								},
							},
						},
					}, nil
				}
				llm.generateFunc = func(ctx context.Context, prompt string) (string, error) {
					return "Вы идете на север", nil
				}
				worldEventRepo.getScheduledByWorldIDFunc = func(ctx context.Context, worldID uint) ([]world.WorldEvent, error) {
					return []world.WorldEvent{}, nil
				}
				worldEventRepo.getActiveByWorldIDFunc = func(ctx context.Context, worldID uint) ([]world.WorldEvent, error) {
					return []world.WorldEvent{}, nil
				}
			},
			expectedError:  false,
			expectedResult: "Вы идете на север",
		},
		{
			name:          "world events check error - continues execution",
			chatID:        12345,
			playerMessage: "Иду на север",
			setupMocks: func(llm *mockLLM, sessionRepo *mockSessionRepo, ctxBuilder *mockContextBuilder, eventRepo *mockEventRepo, worldEventRepo *mockWorldEventRepo) {
				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return &session.GameSession{
						ChatID:  chatID,
						State:   session.StateActive,
						WorldID: 1,
						World:   world.World{ID: 1, Name: "Test World", StartedAt: time.Now()},
						Players: []player.Player{
							{
								Character: character.Character{
									Name:  "Test Hero",
									Race:  character.RaceHuman,
									Class: character.ClassFighter,
								},
							},
						},
					}, nil
				}
				llm.generateFunc = func(ctx context.Context, prompt string) (string, error) {
					return "Вы идете на север", nil
				}
				worldEventRepo.getScheduledByWorldIDFunc = func(ctx context.Context, worldID uint) ([]world.WorldEvent, error) {
					return nil, errors.New("world events check error")
				}
			},
			expectedError:  false,
			expectedResult: "Вы идете на север",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			llm := &mockLLM{}
			sessionRepo := &mockSessionRepo{}
			ctxBuilder := &mockContextBuilder{}
			eventRepo := &mockEventRepo{}

			worldEventRepo := &mockWorldEventRepo{}
			if tt.setupMocks != nil {
				tt.setupMocks(llm, sessionRepo, ctxBuilder, eventRepo, worldEventRepo)
			}

			// Создаем реальный IndexDocument с моками для embedder и store
			mockEmbedder := &mockEmbedder{}
			mockVectorStore := &mockVectorStore{}
			indexDoc := ragapp.NewIndexDocument(mockEmbedder, mockVectorStore)

			// Создаем моки для новых зависимостей
			combatRepo := &mockCombatRepo{}
			questRepo := &mockQuestRepo{}
			inventoryRepo := &mockInventoryRepo{}
			playerRepo := &mockPlayerRepo{}
			addExperienceUC := characterapp.NewAddExperienceUseCase(playerRepo, sessionRepo)
			checkWorldEventsUC := worldeventapp.NewCheckWorldEventsUseCase(worldEventRepo)

			uc := NewHandleActionUseCase(
				llm,
				sessionRepo,
				ctxBuilder,
				eventRepo,
				indexDoc,
				combatRepo,
				questRepo,
				inventoryRepo,
				addExperienceUC,
				checkWorldEventsUC,
				nil, // responseCache - optional
				nil, // actionValidator - optional
			)

			result, err := uc.Execute(context.Background(), tt.chatID, tt.playerMessage)

			if tt.expectedError {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if result != tt.expectedResult {
					t.Errorf("expected result '%s', got '%s'", tt.expectedResult, result)
				}
			}

			// Проверяем, что событие игрока сохранено даже при ошибке LLM
			if tt.name == "LLM error - player event still saved" {
				if len(eventRepo.events) == 0 {
					t.Error("expected player event to be saved even when LLM fails")
				} else if eventRepo.events[0].AuthorType != event.AuthorTypePlayer {
					t.Errorf("expected player event, got %s", eventRepo.events[0].AuthorType)
				}
			}
		})
	}
}

func TestBuildDMPrompt(t *testing.T) {
	gameContext := "Мир: Тестовый мир\nОписание: Красивый мир"
	playerMessage := "Иду на север"

	prompt := buildDMPrompt(gameContext, playerMessage)

	if prompt == "" {
		t.Error("expected non-empty prompt")
	}

	if !contains(prompt, gameContext) {
		t.Error("prompt should contain game context")
	}

	if !contains(prompt, playerMessage) {
		t.Error("prompt should contain player message")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > len(substr) && (s[:len(substr)] == substr ||
			s[len(s)-len(substr):] == substr ||
			containsMiddle(s, substr))))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestHandleActionUseCase_Execute_WithActionValidator(t *testing.T) {
	tests := []struct {
		name           string
		chatID         int64
		playerMessage  string
		setupMocks     func(*mockLLM, *mockSessionRepo, *mockContextBuilder, *mockEventRepo, *ActionValidator)
		expectedError  bool
		expectedResult string
	}{
		{
			name:          "action validation fails - dead character",
			chatID:        12345,
			playerMessage: "Иду на север",
			setupMocks: func(llm *mockLLM, sessionRepo *mockSessionRepo, ctxBuilder *mockContextBuilder, eventRepo *mockEventRepo, validator *ActionValidator) {
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
				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return &session.GameSession{
						ChatID:  chatID,
						State:   session.StateActive,
						WorldID: 1,
						Players: []player.Player{*p},
						World:   world.World{ID: 1, Name: "Test World", StartedAt: time.Now()},
					}, nil
				}
			},
			expectedError:  false,
			expectedResult: "", // Валидатор должен вернуть сообщение об ошибке
		},
		{
			name:          "action validation passes",
			chatID:        12345,
			playerMessage: "Иду на север",
			setupMocks: func(llm *mockLLM, sessionRepo *mockSessionRepo, ctxBuilder *mockContextBuilder, eventRepo *mockEventRepo, validator *ActionValidator) {
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
				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return &session.GameSession{
						ChatID:  chatID,
						State:   session.StateActive,
						WorldID: 1,
						Players: []player.Player{*p},
						World:   world.World{ID: 1, Name: "Test World", StartedAt: time.Now()},
					}, nil
				}
				llm.generateFunc = func(ctx context.Context, prompt string) (string, error) {
					return "Вы идете на север", nil
				}
			},
			expectedError:  false,
			expectedResult: "Вы идете на север",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			llm := &mockLLM{}
			sessionRepo := &mockSessionRepo{}
			ctxBuilder := &mockContextBuilder{}
			eventRepo := &mockEventRepo{}
			worldEventRepo := &mockWorldEventRepo{}

			validator := NewActionValidator()
			if tt.setupMocks != nil {
				tt.setupMocks(llm, sessionRepo, ctxBuilder, eventRepo, validator)
			}

			mockEmbedder := &mockEmbedder{}
			mockVectorStore := &mockVectorStore{}
			indexDoc := ragapp.NewIndexDocument(mockEmbedder, mockVectorStore)

			combatRepo := &mockCombatRepo{}
			questRepo := &mockQuestRepo{}
			inventoryRepo := &mockInventoryRepo{}
			playerRepo := &mockPlayerRepo{}
			addExperienceUC := characterapp.NewAddExperienceUseCase(playerRepo, sessionRepo)
			checkWorldEventsUC := worldeventapp.NewCheckWorldEventsUseCase(worldEventRepo)

			uc := NewHandleActionUseCase(
				llm,
				sessionRepo,
				ctxBuilder,
				eventRepo,
				indexDoc,
				combatRepo,
				questRepo,
				inventoryRepo,
				addExperienceUC,
				checkWorldEventsUC,
				nil, // responseCache
				validator,
			)

			result, err := uc.Execute(context.Background(), tt.chatID, tt.playerMessage)

			if tt.expectedError {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if tt.name == "action validation fails - dead character" {
					// Валидация должна вернуть сообщение об ошибке
					if result == "" {
						t.Error("expected validation error message")
					}
				} else if result != tt.expectedResult {
					t.Errorf("expected result '%s', got '%s'", tt.expectedResult, result)
				}
			}
		})
	}
}

// TestHandleActionUseCase_Execute_WithActionValidator_Stats проверяет, что Stats загружаются правильно
// при использовании валидатора в handle_action (баг #3)
func TestHandleActionUseCase_Execute_WithActionValidator_Stats(t *testing.T) {
	tests := []struct {
		name           string
		chatID         int64
		playerMessage  string
		characterStats character.Stats
		setupMocks     func(*mockLLM, *mockSessionRepo, *mockContextBuilder, *mockEventRepo, *ActionValidator, character.Stats)
		expectedError  bool
		expectedResult string
		shouldContain  string // Текст, который должен содержаться в результате
	}{
		{
			name:          "action validation fails - insufficient strength shows correct stat",
			chatID:        12345,
			playerMessage: "поднять тяжелый камень",
			characterStats: character.Stats{
				Strength:     8, // Сила 8 < 10
				Dexterity:    10,
				Constitution: 10,
				Intelligence: 10,
				Wisdom:       10,
				Charisma:     10,
			},
			setupMocks: func(llm *mockLLM, sessionRepo *mockSessionRepo, ctxBuilder *mockContextBuilder, eventRepo *mockEventRepo, validator *ActionValidator, stats character.Stats) {
				char := &character.Character{
					ID:     1,
					Name:   "Weak Hero",
					Status: character.StatusAlive,
					Stats:  stats,
				}
				p := &player.Player{
					ID:        1,
					Character: *char,
				}
				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return &session.GameSession{
						ChatID:  chatID,
						State:   session.StateActive,
						WorldID: 1,
						Players: []player.Player{*p},
						World:   world.World{ID: 1, Name: "Test World", StartedAt: time.Now()},
					}, nil
				}
			},
			expectedError:  false,
			expectedResult: "", // Валидатор должен вернуть сообщение об ошибке
			shouldContain:  "8", // Сообщение должно содержать "8", а не "0"
		},
		{
			name:          "action validation passes - sufficient strength",
			chatID:        12345,
			playerMessage: "поднять тяжелый камень",
			characterStats: character.Stats{
				Strength:     16, // Сила 16 >= 10
				Dexterity:    14,
				Constitution: 15,
				Intelligence: 12,
				Wisdom:       13,
				Charisma:     10,
			},
			setupMocks: func(llm *mockLLM, sessionRepo *mockSessionRepo, ctxBuilder *mockContextBuilder, eventRepo *mockEventRepo, validator *ActionValidator, stats character.Stats) {
				char := &character.Character{
					ID:     1,
					Name:   "Strong Hero",
					Status: character.StatusAlive,
					Stats:  stats,
				}
				p := &player.Player{
					ID:        1,
					Character: *char,
				}
				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return &session.GameSession{
						ChatID:  chatID,
						State:   session.StateActive,
						WorldID: 1,
						Players: []player.Player{*p},
						World:   world.World{ID: 1, Name: "Test World", StartedAt: time.Now()},
					}, nil
				}
				llm.generateFunc = func(ctx context.Context, prompt string) (string, error) {
					return "Вы поднимаете тяжелый камень", nil
				}
			},
			expectedError:  false,
			expectedResult: "Вы поднимаете тяжелый камень",
			shouldContain:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			llm := &mockLLM{}
			sessionRepo := &mockSessionRepo{}
			ctxBuilder := &mockContextBuilder{}
			eventRepo := &mockEventRepo{}
			worldEventRepo := &mockWorldEventRepo{}

			validator := NewActionValidator()
			if tt.setupMocks != nil {
				tt.setupMocks(llm, sessionRepo, ctxBuilder, eventRepo, validator, tt.characterStats)
			}

			mockEmbedder := &mockEmbedder{}
			mockVectorStore := &mockVectorStore{}
			indexDoc := ragapp.NewIndexDocument(mockEmbedder, mockVectorStore)

			combatRepo := &mockCombatRepo{}
			questRepo := &mockQuestRepo{}
			inventoryRepo := &mockInventoryRepo{}
			playerRepo := &mockPlayerRepo{}
			addExperienceUC := characterapp.NewAddExperienceUseCase(playerRepo, sessionRepo)
			checkWorldEventsUC := worldeventapp.NewCheckWorldEventsUseCase(worldEventRepo)

			uc := NewHandleActionUseCase(
				llm,
				sessionRepo,
				ctxBuilder,
				eventRepo,
				indexDoc,
				combatRepo,
				questRepo,
				inventoryRepo,
				addExperienceUC,
				checkWorldEventsUC,
				nil, // responseCache
				validator,
			)

			result, err := uc.Execute(context.Background(), tt.chatID, tt.playerMessage)

			if tt.expectedError {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}

				if tt.name == "action validation fails - insufficient strength shows correct stat" {
					// Валидация должна вернуть сообщение об ошибке с правильным значением силы (8, а не 0)
					if result == "" {
						t.Error("expected validation error message")
					}
					if tt.shouldContain != "" && !contains(result, tt.shouldContain) {
						t.Errorf("BUG #3: Expected error message to contain '%s' (actual strength), got message: %s", tt.shouldContain, result)
					}
					// Проверяем, что сообщение НЕ содержит "0" как значение силы (если только это не минимум "10")
					if contains(result, "(0)") && tt.characterStats.Strength != 0 {
						t.Errorf("BUG #3: Error message shows strength as 0 instead of %d! Message: %s", tt.characterStats.Strength, result)
					}
				} else if result != tt.expectedResult {
					t.Errorf("expected result '%s', got '%s'", tt.expectedResult, result)
				}
			}
		})
	}
}
