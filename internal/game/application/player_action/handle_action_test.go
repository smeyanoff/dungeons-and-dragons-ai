package player_action

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	characterapp "dungeons-and-dragons-ai/internal/game/application/character"
	"dungeons-and-dragons-ai/internal/game/application/dm_analyzer"
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
	llmtools "dungeons-and-dragons-ai/internal/llm/domain/tools"
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
	searchFunc           func(ctx context.Context, sessionID uint, locationID *uint, embedding []float32, limit int) ([]ragdomain.Document, error)
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

func (m *mockVectorStore) Delete(ctx context.Context, sessionID uint) error {
	return nil
}

func (m *mockVectorStore) Search(ctx context.Context, sessionID uint, locationID *uint, embedding []float32, limit int) ([]ragdomain.Document, error) {
	if m.searchFunc != nil {
		return m.searchFunc(ctx, sessionID, locationID, embedding, limit)
	}
	return nil, nil
}

// Mock LLM
type mockLLM struct {
	generateFunc              func(ctx context.Context, prompt string) (string, error)
	generateWithMaxTokensFunc func(ctx context.Context, prompt string, maxTokens int) (string, error)
	generateWithToolsFunc     func(ctx context.Context, prompt string, tools []llmtools.Tool) (*domain.LLMResponseWithTools, error)
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

func (m *mockLLM) GenerateWithTools(ctx context.Context, prompt string, tools []llmtools.Tool) (*domain.LLMResponseWithTools, error) {
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
	saveFunc        func(ctx context.Context, s *session.GameSession) error
}

func (m *mockSessionRepo) GetByChatID(ctx context.Context, chatID int64) (*session.GameSession, error) {
	if m.getByChatIDFunc != nil {
		return m.getByChatIDFunc(ctx, chatID)
	}
	return nil, nil
}

func (m *mockSessionRepo) Save(ctx context.Context, s *session.GameSession) error {
	if m.saveFunc != nil {
		return m.saveFunc(ctx, s)
	}
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
	saveFunc              func(ctx context.Context, e *event.StoryEvent) error
	saveInTransactionFunc func(ctx context.Context, e *event.StoryEvent, fn func(tx interface{}) error) error
	getBySessionIDFunc    func(ctx context.Context, sessionID uint, limit int) ([]event.StoryEvent, error)
	events                []*event.StoryEvent
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

func (m *mockEventRepo) SaveInTransaction(ctx context.Context, e *event.StoryEvent, fn func(tx interface{}) error) error {
	if m.saveInTransactionFunc != nil {
		return m.saveInTransactionFunc(ctx, e, fn)
	}
	// Простая реализация без реальной транзакции для тестов
	if err := m.Save(ctx, e); err != nil {
		return err
	}
	if fn != nil {
		return fn(nil) // Передаем nil как транзакцию для тестов
	}
	return nil
}

func (m *mockEventRepo) GetBySessionID(ctx context.Context, sessionID uint, limit int) ([]event.StoryEvent, error) {
	if m.getBySessionIDFunc != nil {
		return m.getBySessionIDFunc(ctx, sessionID, limit)
	}
	// Простая реализация: возвращаем события из внутреннего массива
	result := make([]event.StoryEvent, 0)
	count := 0
	for _, e := range m.events {
		if e != nil && e.GameSessionID == sessionID {
			result = append(result, *e)
			count++
			if limit > 0 && count >= limit {
				break
			}
		}
	}
	return result, nil
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

// Mock Daily Quest Progress Checker
type mockDailyQuestProgressChecker struct {
	executeFunc func(ctx context.Context, req CheckDailyQuestProgressRequest) error
}

func (m *mockDailyQuestProgressChecker) Execute(ctx context.Context, req CheckDailyQuestProgressRequest) error {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, req)
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
		userID         int64
		playerMessage  string
		setupMocks     func(*mockLLM, *mockSessionRepo, *mockContextBuilder, *mockEventRepo, *mockWorldEventRepo)
		expectedError  bool
		expectedResult string
	}{
		{
			name:          "successful action",
			chatID:        12345,
			userID:        11111,
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
								ID:       1,
								TgUserID: 11111,
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
			userID:        11111,
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
								ID:       1,
								TgUserID: 11111,
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
			expectedError:  true,
			expectedResult: "",
		},
		{
			name:          "LLM error - player event still saved",
			chatID:        12345,
			userID:        11111,
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
								ID:       1,
								TgUserID: 11111,
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
			expectedError:  true,
			expectedResult: "",
		},
		{
			name:          "world events checked",
			chatID:        12345,
			userID:        11111,
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
								ID:       1,
								TgUserID: 11111,
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
			userID:        11111,
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
								ID:       1,
								TgUserID: 11111,
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
			checkDailyProgressUC := &mockDailyQuestProgressChecker{}

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
				nil, // checkAchievementsUC - optional
				nil, // notificationService - optional
				nil, // generateImageUC - optional
				nil, // useSpellUC - optional
				nil, // responseCache - optional
				nil, // actionValidator - optional
				checkDailyProgressUC,
				nil, // getSubscriptionUC - optional
				nil, // updateRatingUC - optional
				nil, // analyzePlayerActionUC - optional
				nil, // generateLocationEventUC - optional
			)

			result, err := uc.Execute(context.Background(), tt.chatID, tt.userID, tt.playerMessage)

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

	prefs := player.UserPreferences{
		NarrativeStyle: player.NarrativeStyleBalanced,
		DetailLevel:    player.DetailLevelMedium,
		Language:       "ru",
		ShowStats:      true,
	}

	prompt := BuildDMPrompt(gameContext, playerMessage, prefs)

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
		userID         int64
		playerMessage  string
		setupMocks     func(*mockLLM, *mockSessionRepo, *mockContextBuilder, *mockEventRepo, *ActionValidator)
		expectedError  bool
		expectedResult string
	}{
		{
			name:          "action validation fails - dead character",
			chatID:        12345,
			userID:        11111,
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
					TgUserID:  11111,
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
			userID:        11111,
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
					TgUserID:  11111,
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
			checkDailyProgressUC := &mockDailyQuestProgressChecker{}

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
				nil, // checkAchievementsUC - optional
				nil, // notificationService - optional
				nil, // generateImageUC - optional
				nil, // useSpellUC - optional
				nil, // responseCache
				validator,
				checkDailyProgressUC,
				nil, // getSubscriptionUC - optional
				nil, // updateRatingUC - optional
				nil, // analyzePlayerActionUC - optional
				nil, // generateLocationEventUC - optional
			)

			result, err := uc.Execute(context.Background(), tt.chatID, tt.userID, tt.playerMessage)

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
		userID         int64
		playerMessage  string
		characterStats character.Stats
		setupMocks     func(*mockLLM, *mockSessionRepo, *mockContextBuilder, *mockEventRepo, *ActionValidator, character.Stats)
		expectedError  bool
		expectedResult string
		shouldContain  string // Текст, который должен содержаться в результате
	}{
		{
			name:          "action validation does not block - strength checks handled by DM tools",
			chatID:        12345,
			userID:        11111,
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
					TgUserID:  11111,
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
			expectedResult: "Test DM response",
			shouldContain:  "",
		},
		{
			name:          "action validation passes - sufficient strength",
			chatID:        12345,
			userID:        11111,
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
					TgUserID:  11111,
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
			checkDailyProgressUC := &mockDailyQuestProgressChecker{}

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
				nil, // checkAchievementsUC - optional
				nil, // notificationService - optional
				nil, // generateImageUC - optional
				nil, // useSpellUC - optional
				nil, // responseCache
				validator,
				checkDailyProgressUC,
				nil, // getSubscriptionUC - optional
				nil, // updateRatingUC - optional
				nil, // analyzePlayerActionUC - optional
				nil, // generateLocationEventUC - optional
			)

			result, err := uc.Execute(context.Background(), tt.chatID, tt.userID, tt.playerMessage)

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
		})
	}
}

// TestCreateAbilityCheckFromAnalyzer_RepeatInScene_DoesNotCreatePendingCheck — регрессионный тест:
// если проверка той же характеристики в той же сцене (локации) уже выполнялась,
// RequestAbilityCheckTool отклоняет её ключом "repeat_in_scene" и не создаёт pending check.
// createAbilityCheckFromAnalyzer должен вернуть предупреждение как есть, а не спутать
// отказ с успешным созданием проверки (баг: "repeat_in_scene" отсутствовал в списке
// распознаваемых отказов, из-за чего игрок получал "Напишите /roll", хотя pending check
// в БД так и не появлялся — /roll потом не находил, что резолвить).
func TestCreateAbilityCheckFromAnalyzer_RepeatInScene_DoesNotCreatePendingCheck(t *testing.T) {
	locationID := uint(4)
	gs := &session.GameSession{
		ChatID:                     698225384,
		CurrentLocationID:          &locationID,
		LastAbilityCheckLocationID: &locationID,
		LastAbilityCheckAbility:    "wisdom",
		Players: []player.Player{
			{
				TgUserID: 1,
				Character: character.Character{
					Name:  "Hero",
					Stats: character.Stats{Wisdom: 14},
				},
			},
		},
	}

	sessionRepo := &mockSessionRepo{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			return gs, nil
		},
		saveFunc: func(ctx context.Context, s *session.GameSession) error {
			t.Fatal("Save must not be called when the check is rejected as repeat_in_scene — no pending check should be persisted")
			return nil
		},
	}
	eventRepo := &mockEventRepo{}

	uc := NewHandleActionUseCase(
		nil, sessionRepo, nil, eventRepo, nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)

	msg, handled, err := uc.createAbilityCheckFromAnalyzer(context.Background(), gs, &dm_analyzer.AbilityCheckDetails{
		Ability: "wisdom",
		DC:      12,
		Reason:  "новая попытка осмотреться",
		Stakes:  "найти улики",
	}, "осмотреться в комнате")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Fatal("expected handled=true (rejection is still a handled outcome)")
	}
	if gs.HasPendingAbilityCheck() {
		t.Fatal("no pending check should have been created for a repeat-in-scene rejection")
	}
	if strings.Contains(msg, "Напишите /roll") {
		t.Fatalf("rejection message must not tell the player to /roll when no pending check was created, got: %q", msg)
	}
}

// TestCreateAbilityCheckFromAnalyzer_EmptyReason_FallsBackToPlayerAction — если анализатор
// не заполнил reason, проверка всё равно должна ссылаться на конкретное действие игрока
// (взятое из его реплики), а не на обезличенную формулировку.
func TestCreateAbilityCheckFromAnalyzer_EmptyReason_FallsBackToPlayerAction(t *testing.T) {
	locationID := uint(5)
	gs := &session.GameSession{
		ChatID:            698225385,
		CurrentLocationID: &locationID,
		Players: []player.Player{
			{
				TgUserID: 1,
				Character: character.Character{
					Name:  "Hero",
					Stats: character.Stats{Wisdom: 14},
				},
			},
		},
	}

	sessionRepo := &mockSessionRepo{
		getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
			return gs, nil
		},
		saveFunc: func(ctx context.Context, s *session.GameSession) error {
			return nil
		},
	}
	eventRepo := &mockEventRepo{}

	uc := NewHandleActionUseCase(
		nil, sessionRepo, nil, eventRepo, nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)

	msg, handled, err := uc.createAbilityCheckFromAnalyzer(context.Background(), gs, &dm_analyzer.AbilityCheckDetails{
		Ability: "wisdom",
		DC:      12,
	}, "прислушаться у двери в подвал")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Fatal("expected handled=true")
	}
	if strings.Contains(msg, "неопределенным исходом") {
		t.Fatalf("reason must be tied to the player's concrete action, not a generic placeholder, got: %q", msg)
	}
	if !strings.Contains(msg, "прислушаться у двери в подвал") {
		t.Fatalf("reason must reference the player's specific action, got: %q", msg)
	}
}
