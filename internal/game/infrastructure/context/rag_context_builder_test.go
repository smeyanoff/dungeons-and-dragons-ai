package context

import (
	"context"
	"errors"
	"testing"

	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/combat"
	"dungeons-and-dragons-ai/internal/game/domain/event"
	"dungeons-and-dragons-ai/internal/game/domain/inventory"
	"dungeons-and-dragons-ai/internal/game/domain/player"
	"dungeons-and-dragons-ai/internal/game/domain/quest"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/game/domain/world"
	"dungeons-and-dragons-ai/internal/rag/application"
	"dungeons-and-dragons-ai/internal/rag/domain"

	"gorm.io/gorm"
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
	searchFunc func(ctx context.Context, sessionID uint, locationID *uint, embedding []float32, limit int) ([]domain.Document, error)
}

func (m *mockVectorStore) Search(ctx context.Context, sessionID uint, locationID *uint, embedding []float32, limit int) ([]domain.Document, error) {
	if m.searchFunc != nil {
		return m.searchFunc(ctx, sessionID, locationID, embedding, limit)
	}
	return nil, nil
}

func (m *mockVectorStore) EnsureCollection(ctx context.Context) error {
	return nil
}

func (m *mockVectorStore) Upsert(ctx context.Context, doc domain.Document, embedding []float32) error {
	return nil
}

func (m *mockVectorStore) Delete(ctx context.Context, sessionID uint) error {
	return nil
}

// Mock EventRepository
type mockEventRepository struct {
	getBySessionIDFunc      func(ctx context.Context, sessionID uint, limit int) ([]event.StoryEvent, error)
	getRecentByLocationFunc func(ctx context.Context, sessionID uint, locationID uint, limit int) ([]event.StoryEvent, error)
}

func (m *mockEventRepository) GetBySessionID(ctx context.Context, sessionID uint, limit int) ([]event.StoryEvent, error) {
	if m.getBySessionIDFunc != nil {
		return m.getBySessionIDFunc(ctx, sessionID, limit)
	}
	return []event.StoryEvent{}, nil
}

func (m *mockEventRepository) GetRecentByLocation(ctx context.Context, sessionID uint, locationID uint, limit int) ([]event.StoryEvent, error) {
	if m.getRecentByLocationFunc != nil {
		return m.getRecentByLocationFunc(ctx, sessionID, locationID, limit)
	}
	return []event.StoryEvent{}, nil
}

// Mock InventoryRepository
type mockInventoryRepository struct {
	getByCharacterIDFunc func(ctx context.Context, characterID uint) (*inventory.Inventory, error)
}

func (m *mockInventoryRepository) GetByCharacterID(ctx context.Context, characterID uint) (*inventory.Inventory, error) {
	if m.getByCharacterIDFunc != nil {
		return m.getByCharacterIDFunc(ctx, characterID)
	}
	return nil, nil
}

// Mock CombatRepository
type mockCombatRepository struct {
	getActiveBySessionIDFunc func(ctx context.Context, sessionID uint) (*combat.Combat, error)
}

func (m *mockCombatRepository) GetActiveBySessionID(ctx context.Context, sessionID uint) (*combat.Combat, error) {
	if m.getActiveBySessionIDFunc != nil {
		return m.getActiveBySessionIDFunc(ctx, sessionID)
	}
	return nil, nil
}

func TestRAGContextBuilder_BuildContext(t *testing.T) {
	tests := []struct {
		name      string
		gs        *session.GameSession
		message   string
		setupMock func(*mockEmbedder, *mockVectorStore)
		wantError bool
		validate  func(*testing.T, string)
	}{
		{
			name: "successful context building with RAG",
			gs: &session.GameSession{
				Model: gorm.Model{ID: 1},
				World: world.World{
					Name:        "Test World",
					Description: "A test world description",
					MainQuest: &quest.Quest{
						Title:       "Main Quest",
						Description: "Main quest description",
					},
					Locations: []world.Location{
						{
							Name:        "Location 1",
							Description: "Description 1",
						},
					},
				},
				Players: []player.Player{
					{
						Character: character.Character{
							Name:  "Test Hero",
							Race:  character.RaceHuman,
							Class: character.ClassFighter,
							Level: 5,
							HP:    50,
							MaxHP: 60,
							Stats: character.Stats{
								Strength:     16,
								Dexterity:    14,
								Constitution: 15,
								Intelligence: 12,
								Wisdom:       13,
								Charisma:     10,
							},
						},
					},
				},
			},
			message: "I want to explore",
			setupMock: func(e *mockEmbedder, v *mockVectorStore) {
				e.embedFunc = func(ctx context.Context, text string) ([]float32, error) {
					return []float32{0.1, 0.2, 0.3}, nil
				}
				v.searchFunc = func(ctx context.Context, sessionID uint, locationID *uint, embedding []float32, limit int) ([]domain.Document, error) {
					return []domain.Document{
						{Text: "Previous event 1"},
						{Text: "Previous event 2"},
					}, nil
				}
			},
			wantError: false,
			validate: func(t *testing.T, result string) {
				if result == "" {
					t.Error("expected non-empty context")
				}
				if !contains(result, "Test World") {
					t.Error("expected world name in context")
				}
				if !contains(result, "Previous event 1") {
					t.Error("expected RAG document in context")
				}
				if !contains(result, "Test Hero") {
					t.Error("expected player name in context")
				}
				if !contains(result, "Уровень: 5") {
					t.Error("expected player level in context")
				}
			},
		},
		{
			name: "RAG error falls back to simple context",
			gs: &session.GameSession{
				Model: gorm.Model{ID: 1},
				World: world.World{
					Name:        "Test World",
					Description: "A test world description",
				},
				Players: []player.Player{
					{
						Character: character.Character{
							Name:  "Test Hero",
							Race:  character.RaceHuman,
							Class: character.ClassFighter,
						},
					},
				},
			},
			message: "I want to explore",
			setupMock: func(e *mockEmbedder, v *mockVectorStore) {
				e.embedFunc = func(ctx context.Context, text string) ([]float32, error) {
					return nil, errors.New("RAG error")
				}
			},
			wantError: false,
			validate: func(t *testing.T, result string) {
				if result == "" {
					t.Error("expected non-empty context")
				}
				if !contains(result, "Test World") {
					t.Error("expected world name in context (fallback to simple)")
				}
				// When RAG fails, only base context is returned (without player info)
				// This is current behavior - player info is only added when RAG succeeds
			},
		},
		{
			name: "no RAG documents",
			gs: &session.GameSession{
				Model: gorm.Model{ID: 1},
				World: world.World{
					Name:        "Test World",
					Description: "A test world description",
				},
				Players: []player.Player{
					{
						Character: character.Character{
							Name:  "Test Hero",
							Race:  character.RaceHuman,
							Class: character.ClassFighter,
						},
					},
				},
			},
			message: "I want to explore",
			setupMock: func(e *mockEmbedder, v *mockVectorStore) {
				e.embedFunc = func(ctx context.Context, text string) ([]float32, error) {
					return []float32{0.1, 0.2, 0.3}, nil
				}
				v.searchFunc = func(ctx context.Context, sessionID uint, locationID *uint, embedding []float32, limit int) ([]domain.Document, error) {
					return []domain.Document{}, nil
				}
			},
			wantError: false,
			validate: func(t *testing.T, result string) {
				if result == "" {
					t.Error("expected non-empty context")
				}
				if contains(result, "Релевантная история игры") {
					t.Error("should not contain RAG section when no documents")
				}
			},
		},
		{
			name: "no players",
			gs: &session.GameSession{
				Model: gorm.Model{ID: 1},
				World: world.World{
					Name:        "Test World",
					Description: "A test world description",
				},
				Players: []player.Player{},
			},
			message: "I want to explore",
			setupMock: func(e *mockEmbedder, v *mockVectorStore) {
				e.embedFunc = func(ctx context.Context, text string) ([]float32, error) {
					return []float32{0.1, 0.2, 0.3}, nil
				}
				v.searchFunc = func(ctx context.Context, sessionID uint, locationID *uint, embedding []float32, limit int) ([]domain.Document, error) {
					return []domain.Document{
						{Text: "Previous event"},
					}, nil
				}
			},
			wantError: false,
			validate: func(t *testing.T, result string) {
				if result == "" {
					t.Error("expected non-empty context")
				}
				if contains(result, "Персонаж игрока") {
					t.Error("should not contain player section when no players")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			simpleBuilder := NewSimpleContextBuilder()
			mockEmbedder := &mockEmbedder{}
			mockVectorStore := &mockVectorStore{}
			if tt.setupMock != nil {
				tt.setupMock(mockEmbedder, mockVectorStore)
			}

			// Create RetrieveContext with mocks
			retrieveUC := application.NewRetrieveContext(mockEmbedder, mockVectorStore)
			mockEventRepo := &mockEventRepository{}
			mockCombatRepo := &mockCombatRepository{}
			builder := NewRAGContextBuilder(simpleBuilder, retrieveUC, mockEventRepo, nil, mockCombatRepo) // inventoryRepo не нужен для этих тестов

			result, err := builder.BuildContext(context.Background(), tt.gs, tt.message)

			if tt.wantError {
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

func TestRAGContextBuilder_isInventoryQuery(t *testing.T) {
	builder := &RAGContextBuilder{}

	tests := []struct {
		name    string
		message string
		want    bool
	}{
		{
			name:    "detects 'инвентарь'",
			message: "покажи мой инвентарь",
			want:    true,
		},
		{
			name:    "detects 'что у меня'",
			message: "что у меня есть?",
			want:    true,
		},
		{
			name:    "detects 'что в инвентаре'",
			message: "что в инвентаре?",
			want:    true,
		},
		{
			name:    "detects 'что в сумке'",
			message: "что в сумке?",
			want:    true,
		},
		{
			name:    "detects 'что у меня в сумке'",
			message: "что у меня в сумке?",
			want:    true,
		},
		{
			name:    "detects 'что у меня в карманах'",
			message: "что у меня в карманах",
			want:    true,
		},
		{
			name:    "detects 'что я ношу'",
			message: "что я ношу",
			want:    true,
		},
		{
			name:    "detects 'что у меня с собой'",
			message: "что у меня с собой",
			want:    true,
		},
		{
			name:    "detects 'покажи инвентарь'",
			message: "покажи инвентарь",
			want:    true,
		},
		{
			name:    "detects 'предметы'",
			message: "какие у меня предметы?",
			want:    true,
		},
		{
			name:    "not detected for non-inventory query",
			message: "я хочу идти на север",
			want:    false,
		},
		{
			name:    "case insensitive",
			message: "ИнВенТаРь",
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := builder.isInventoryQuery(tt.message)
			if got != tt.want {
				t.Errorf("isInventoryQuery(%q) = %v, want %v", tt.message, got, tt.want)
			}
		})
	}
}

func TestRAGContextBuilder_BuildContext_WithInventoryQuery(t *testing.T) {
	simpleBuilder := NewSimpleContextBuilder()
	mockEmbedder := &mockEmbedder{
		embedFunc: func(ctx context.Context, text string) ([]float32, error) {
			return []float32{0.1, 0.2, 0.3}, nil
		},
	}
	mockVectorStore := &mockVectorStore{
		searchFunc: func(ctx context.Context, sessionID uint, locationID *uint, embedding []float32, limit int) ([]domain.Document, error) {
			return []domain.Document{}, nil
		},
	}

	retrieveUC := application.NewRetrieveContext(mockEmbedder, mockVectorStore)
	mockEventRepo := &mockEventRepository{}

	t.Run("adds inventory to context when query matches", func(t *testing.T) {
		characterID := uint(123)
		mockInvRepo := &mockInventoryRepository{
			getByCharacterIDFunc: func(ctx context.Context, charID uint) (*inventory.Inventory, error) {
				if charID != characterID {
					t.Errorf("expected characterID %d, got %d", characterID, charID)
				}
				return &inventory.Inventory{
					ID:          1,
					CharacterID: characterID,
					Items: []inventory.InventoryItem{
						{
							Name:        "Sword",
							Description: "A sharp sword",
							Weight:      2.5,
							Quantity:    1,
							Type:        inventory.ItemTypeWeapon,
						},
						{
							Name:        "Health Potion",
							Description: "Restores 10 HP",
							Weight:      0.5,
							Quantity:    3,
							Type:        inventory.ItemTypePotion,
						},
					},
				}, nil
			},
		}

		gs := &session.GameSession{
			Model: gorm.Model{ID: 1},
			World: world.World{
				Name:        "Test World",
				Description: "A test world description",
			},
			Players: []player.Player{
				{
					Character: character.Character{
						ID:    characterID,
						Name:  "Test Hero",
						Race:  character.RaceHuman,
						Class: character.ClassFighter,
					},
				},
			},
		}

		mockCombatRepo := &mockCombatRepository{}
		builder := NewRAGContextBuilder(simpleBuilder, retrieveUC, mockEventRepo, mockInvRepo, mockCombatRepo)
		result, err := builder.BuildContext(context.Background(), gs, "покажи мой инвентарь")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !contains(result, "Инвентарь персонажа") {
			t.Error("expected inventory section in context")
		}
		if !contains(result, "Sword") {
			t.Error("expected 'Sword' in inventory context")
		}
		if !contains(result, "Health Potion") {
			t.Error("expected 'Health Potion' in inventory context")
		}
		if !contains(result, "(x3)") {
			t.Error("expected quantity '(x3)' for Health Potion")
		}
	})

	t.Run("handles empty inventory", func(t *testing.T) {
		characterID := uint(456)
		mockInvRepo := &mockInventoryRepository{
			getByCharacterIDFunc: func(ctx context.Context, charID uint) (*inventory.Inventory, error) {
				return &inventory.Inventory{
					ID:          2,
					CharacterID: characterID,
					Items:       []inventory.InventoryItem{},
				}, nil
			},
		}

		gs := &session.GameSession{
			Model: gorm.Model{ID: 1},
			World: world.World{
				Name:        "Test World",
				Description: "A test world description",
			},
			Players: []player.Player{
				{
					Character: character.Character{
						ID:    characterID,
						Name:  "Test Hero",
						Race:  character.RaceHuman,
						Class: character.ClassFighter,
					},
				},
			},
		}

		mockCombatRepo := &mockCombatRepository{}
		builder := NewRAGContextBuilder(simpleBuilder, retrieveUC, mockEventRepo, mockInvRepo, mockCombatRepo)
		result, err := builder.BuildContext(context.Background(), gs, "что у меня есть")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !contains(result, "Инвентарь персонажа") {
			t.Error("expected inventory section in context")
		}
		if !contains(result, "Инвентарь пуст") {
			t.Error("expected 'Инвентарь пуст' in context")
		}
	})

	t.Run("does not add inventory for non-inventory query", func(t *testing.T) {
		mockInvRepo := &mockInventoryRepository{
			getByCharacterIDFunc: func(ctx context.Context, charID uint) (*inventory.Inventory, error) {
				t.Error("GetByCharacterID should not be called for non-inventory query")
				return nil, nil
			},
		}

		gs := &session.GameSession{
			Model: gorm.Model{ID: 1},
			World: world.World{
				Name:        "Test World",
				Description: "A test world description",
			},
			Players: []player.Player{
				{
					Character: character.Character{
						ID:    789,
						Name:  "Test Hero",
						Race:  character.RaceHuman,
						Class: character.ClassFighter,
					},
				},
			},
		}

		mockCombatRepo := &mockCombatRepository{}
		builder := NewRAGContextBuilder(simpleBuilder, retrieveUC, mockEventRepo, mockInvRepo, mockCombatRepo)
		result, err := builder.BuildContext(context.Background(), gs, "я хочу идти на север")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if contains(result, "Инвентарь персонажа") {
			t.Error("should not contain inventory section for non-inventory query")
		}
	})

	t.Run("handles inventory repository error gracefully", func(t *testing.T) {
		characterID := uint(999)
		mockInvRepo := &mockInventoryRepository{
			getByCharacterIDFunc: func(ctx context.Context, charID uint) (*inventory.Inventory, error) {
				return nil, errors.New("database error")
			},
		}

		gs := &session.GameSession{
			Model: gorm.Model{ID: 1},
			World: world.World{
				Name:        "Test World",
				Description: "A test world description",
			},
			Players: []player.Player{
				{
					Character: character.Character{
						ID:    characterID,
						Name:  "Test Hero",
						Race:  character.RaceHuman,
						Class: character.ClassFighter,
					},
				},
			},
		}

		mockCombatRepo := &mockCombatRepository{}
		builder := NewRAGContextBuilder(simpleBuilder, retrieveUC, mockEventRepo, mockInvRepo, mockCombatRepo)
		result, err := builder.BuildContext(context.Background(), gs, "что у меня есть")

		// Should not return error, but just log warning and continue without inventory
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should still have other context (world, character info)
		if !contains(result, "Test World") {
			t.Error("expected world info in context even when inventory fails")
		}
	})

	t.Run("does not add inventory when inventoryRepo is nil", func(t *testing.T) {
		gs := &session.GameSession{
			Model: gorm.Model{ID: 1},
			World: world.World{
				Name:        "Test World",
				Description: "A test world description",
			},
			Players: []player.Player{
				{
					Character: character.Character{
						ID:    111,
						Name:  "Test Hero",
						Race:  character.RaceHuman,
						Class: character.ClassFighter,
					},
				},
			},
		}

		mockCombatRepo := &mockCombatRepository{}
		builder := NewRAGContextBuilder(simpleBuilder, retrieveUC, mockEventRepo, nil, mockCombatRepo) // nil inventoryRepo
		result, err := builder.BuildContext(context.Background(), gs, "покажи инвентарь")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if contains(result, "Инвентарь персонажа") {
			t.Error("should not contain inventory section when inventoryRepo is nil")
		}
	})
}

// TestRAGContextBuilder_BuildContext_PlayerCount проверяет добавление информации о количестве игроков (#65)
func TestRAGContextBuilder_BuildContext_PlayerCount(t *testing.T) {
	simpleBuilder := NewSimpleContextBuilder()
	mockEmbedder := &mockEmbedder{
		embedFunc: func(ctx context.Context, text string) ([]float32, error) {
			return []float32{0.1, 0.2, 0.3}, nil
		},
	}
	mockVectorStore := &mockVectorStore{
		searchFunc: func(ctx context.Context, sessionID uint, locationID *uint, embedding []float32, limit int) ([]domain.Document, error) {
			return []domain.Document{}, nil
		},
	}

	retrieveUC := application.NewRetrieveContext(mockEmbedder, mockVectorStore)
	mockEventRepo := &mockEventRepository{}
	mockCombatRepo := &mockCombatRepository{}

	t.Run("single player - shows player count and warning", func(t *testing.T) {
		gs := &session.GameSession{
			Model: gorm.Model{ID: 1},
			World: world.World{
				Name:        "Test World",
				Description: "A test world",
			},
			Players: []player.Player{
				{
					Character: character.Character{
						ID:    1,
						Name:  "Solo Hero",
						Race:  character.RaceHuman,
						Class: character.ClassFighter,
					},
				},
			},
		}

		mockCombatRepo := &mockCombatRepository{
			getActiveBySessionIDFunc: func(ctx context.Context, sessionID uint) (*combat.Combat, error) {
				// Создаем бой с одним игроком и одним врагом
				char := &character.Character{
					ID:     1,
					Name:   "Solo Hero",
					HP:     20,
					MaxHP:  20,
					Status: character.StatusAlive,
				}
				return &combat.Combat{
					State:       combat.CombatStateActive,
					CurrentTurn: 0,
					Participants: []combat.CombatParticipant{
						{
							IsPlayer:    true,
							CharacterID: &char.ID,
							Character:   char,
							Initiative:  15,
						},
						{
							IsPlayer:     false,
							MonsterName:  "Goblin",
							MonsterHP:    10,
							MonsterMaxHP: 10,
							MonsterAC:    12,
							Initiative:   10,
						},
					},
				}, nil
			},
		}

		builder := NewRAGContextBuilder(simpleBuilder, retrieveUC, mockEventRepo, nil, mockCombatRepo)
		result, err := builder.BuildContext(context.Background(), gs, "test message")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Проверяем наличие информации о количестве игроков
		if !contains(result, "Количество игроков: 1 (игрок один)") {
			t.Error("expected player count '1 (игрок один)' in context")
		}

		// Проверяем наличие информации о бое
		if !contains(result, "Игроков в бою: 1, Врагов: 1") {
			t.Error("expected 'Игроков в бою: 1, Врагов: 1' in context")
		}

		// Проверяем предупреждение о единственном игроке
		if !contains(result, "⚠️ ВАЖНО: Игрок один в бою") {
			t.Error("expected warning about single player in combat")
		}

		// Проверяем критическую инструкцию для DM
		if !contains(result, "⚠️ КРИТИЧЕСКИ ВАЖНО: Используй ТОЛЬКО реальных участников боя") {
			t.Error("expected critical instruction about not inventing allies")
		}
	})

	t.Run("multiple players - shows player count", func(t *testing.T) {
		gs := &session.GameSession{
			Model: gorm.Model{ID: 1},
			World: world.World{
				Name:        "Test World",
				Description: "A test world",
			},
			Players: []player.Player{
				{
					Character: character.Character{
						ID:    1,
						Name:  "Hero 1",
						Race:  character.RaceHuman,
						Class: character.ClassFighter,
					},
				},
				{
					Character: character.Character{
						ID:    2,
						Name:  "Hero 2",
						Race:  character.RaceElf,
						Class: character.ClassWizard,
					},
				},
			},
		}

		builder := NewRAGContextBuilder(simpleBuilder, retrieveUC, mockEventRepo, nil, mockCombatRepo)
		result, err := builder.BuildContext(context.Background(), gs, "test message")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Проверяем наличие информации о количестве игроков (2 игрока)
		if !contains(result, "Количество игроков: 2") {
			t.Error("expected player count '2' in context")
		}

		// Для нескольких игроков не должно быть предупреждения о единственном игроке
		if contains(result, "⚠️ ВАЖНО: Игрок один в бою") {
			t.Error("should not show single player warning for multiple players")
		}
	})

	t.Run("combat context includes participants count", func(t *testing.T) {
		gs := &session.GameSession{
			Model: gorm.Model{ID: 1},
			World: world.World{
				Name:        "Test World",
				Description: "A test world",
			},
			Players: []player.Player{
				{
					Character: character.Character{
						ID:     1,
						Name:   "Combat Hero",
						Race:   character.RaceHuman,
						Class:  character.ClassFighter,
						HP:     20,
						MaxHP:  20,
						Status: character.StatusAlive,
					},
				},
			},
		}

		mockCombatRepo := &mockCombatRepository{
			getActiveBySessionIDFunc: func(ctx context.Context, sessionID uint) (*combat.Combat, error) {
				char := &character.Character{
					ID:     1,
					Name:   "Combat Hero",
					HP:     20,
					MaxHP:  20,
					Status: character.StatusAlive,
				}
				return &combat.Combat{
					State:       combat.CombatStateActive,
					CurrentTurn: 0,
					Participants: []combat.CombatParticipant{
						{
							IsPlayer:    true,
							CharacterID: &char.ID,
							Character:   char,
							Initiative:  15,
						},
						{
							IsPlayer:     false,
							MonsterName:  "Goblin 1",
							MonsterHP:    10,
							MonsterMaxHP: 10,
							MonsterAC:    12,
							Initiative:   12,
						},
						{
							IsPlayer:     false,
							MonsterName:  "Goblin 2",
							MonsterHP:    8,
							MonsterMaxHP: 8,
							MonsterAC:    12,
							Initiative:   10,
						},
					},
				}, nil
			},
		}

		builder := NewRAGContextBuilder(simpleBuilder, retrieveUC, mockEventRepo, nil, mockCombatRepo)
		result, err := builder.BuildContext(context.Background(), gs, "test message")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Проверяем наличие информации о количестве игроков и врагов в бою
		if !contains(result, "Игроков в бою: 1, Врагов: 2") {
			t.Error("expected 'Игроков в бою: 1, Врагов: 2' in context")
		}

		// Проверяем критическую инструкцию
		if !contains(result, "НЕ выдумывай союзников, товарищей или NPC") {
			t.Error("expected instruction about not inventing allies")
		}
	})
}

func TestEventLinesTiered(t *testing.T) {
	longContent := "Очень длинный текст сообщения, который должен обрезаться по-разному в зависимости от того, насколько старое это событие в списке — свежие сообщения получают больше символов, старые заметно меньше."

	makeEvents := func(n int) []event.StoryEvent {
		events := make([]event.StoryEvent, n)
		for i := range events {
			events[i] = event.StoryEvent{AuthorType: event.AuthorTypePlayer, Content: longContent}
		}
		return events
	}

	t.Run("budget decreases with age and floors at minChars", func(t *testing.T) {
		events := makeEvents(5)
		lines := eventLinesTiered(events, 100, 20)

		if len(lines) != 5 {
			t.Fatalf("expected 5 lines, got %d", len(lines))
		}

		lengths := make([]int, len(lines))
		for i, l := range lines {
			lengths[i] = len(l)
		}

		// Каждая следующая (более старая) строка не длиннее предыдущей.
		for i := 1; i < len(lengths); i++ {
			if lengths[i] > lengths[i-1] {
				t.Errorf("expected non-increasing lengths by age, got %v", lengths)
				break
			}
		}
		// Самая свежая строка должна использовать заметно больший бюджет, чем самая старая.
		if lengths[0] <= lengths[len(lengths)-1] {
			t.Errorf("expected the newest line to be longer than the oldest, got lengths %v", lengths)
		}
	})

	t.Run("minChars greater than maxChars falls back to maxChars for every line", func(t *testing.T) {
		events := makeEvents(3)
		untruncated := formatStoryEventLine(events[0], len(longContent))
		lines := eventLinesTiered(events, 50, 500)
		for i, l := range lines {
			if len(l) >= len(untruncated) {
				t.Errorf("line %d expected to be truncated (shorter than %q), got %q", i, untruncated, l)
			}
		}
	})

	t.Run("empty input returns empty slice", func(t *testing.T) {
		lines := eventLinesTiered(nil, 100, 20)
		if len(lines) != 0 {
			t.Errorf("expected no lines for empty input, got %d", len(lines))
		}
	})
}
