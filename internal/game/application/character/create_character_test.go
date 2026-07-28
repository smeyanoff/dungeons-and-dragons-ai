package character

import (
	"context"
	"errors"
	"testing"

	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/inventory"
	"dungeons-and-dragons-ai/internal/game/domain/player"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/game/domain/spell"
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

// Mock Player Repository
type mockPlayerRepo struct {
	getByTgUserIDAndSessionIDFunc func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error)
	saveFunc                      func(ctx context.Context, p *player.Player) error
	savedPlayers                  []*player.Player
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
	if m.savedPlayers == nil {
		m.savedPlayers = make([]*player.Player, 0)
	}
	m.savedPlayers = append(m.savedPlayers, p)
	return nil
}

// Mock Inventory Repository
type mockInventoryRepo struct {
	saveFunc         func(ctx context.Context, inv *inventory.Inventory) error
	savedInventories []*inventory.Inventory
}

func (m *mockInventoryRepo) Save(ctx context.Context, inv *inventory.Inventory) error {
	if m.saveFunc != nil {
		return m.saveFunc(ctx, inv)
	}
	m.savedInventories = append(m.savedInventories, inv)
	return nil
}

// Mock Spell Repository
type mockSpellRepo struct {
	getByClassFunc       func(ctx context.Context, class character.Class) ([]*spell.Spell, error)
	savedCharacterSpells []*spell.CharacterSpell
}

func (m *mockSpellRepo) GetByClass(ctx context.Context, class character.Class) ([]*spell.Spell, error) {
	if m.getByClassFunc != nil {
		return m.getByClassFunc(ctx, class)
	}
	return nil, nil
}

func (m *mockSpellRepo) SaveCharacterSpell(ctx context.Context, cs *spell.CharacterSpell) error {
	m.savedCharacterSpells = append(m.savedCharacterSpells, cs)
	return nil
}

func TestCreateCharacterUseCase_Execute(t *testing.T) {
	tests := []struct {
		name          string
		req           CreateCharacterRequest
		setupMocks    func(*mockSessionRepo, *mockPlayerRepo, *mockInventoryRepo)
		setupSpells   func(*mockSpellRepo)
		expectedError bool
		validate      func(*testing.T, *player.Player, *mockInventoryRepo)
		validateSpell func(*testing.T, *mockSpellRepo)
	}{
		{
			name: "successful character creation",
			req: CreateCharacterRequest{
				ChatID: 12345,
				Name:   "Test Hero",
				Race:   character.RaceHuman,
				Class:  character.ClassFighter,
			},
			setupMocks: func(sessionRepo *mockSessionRepo, playerRepo *mockPlayerRepo, inventoryRepo *mockInventoryRepo) {
				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return &session.GameSession{
						ChatID: chatID,
						State:  session.StateActive,
						World:  world.World{Name: "Test World"},
					}, nil
				}
				playerRepo.getByTgUserIDAndSessionIDFunc = func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
					return nil, nil // No existing player
				}
			},
			expectedError: false,
			validate: func(t *testing.T, p *player.Player, inventoryRepo *mockInventoryRepo) {
				if p == nil {
					t.Fatal("expected player, got nil")
				}
				if p.Name != "Test Hero" {
					t.Errorf("expected name 'Test Hero', got '%s'", p.Name)
				}
				if p.Character.Race != character.RaceHuman {
					t.Errorf("expected race %s, got %s", character.RaceHuman, p.Character.Race)
				}
				if p.Character.Class != character.ClassFighter {
					t.Errorf("expected class %s, got %s", character.ClassFighter, p.Character.Class)
				}
				if p.Character.Level != 1 {
					t.Errorf("expected level 1, got %d", p.Character.Level)
				}
				// Персонаж должен получить стартовое снаряжение и золото (starting_kit.go).
				if len(inventoryRepo.savedInventories) != 1 {
					t.Fatalf("expected starting inventory to be saved once, got %d saves", len(inventoryRepo.savedInventories))
				}
				startingInv := inventoryRepo.savedInventories[0]
				if startingInv.CharacterID != p.Character.ID {
					t.Errorf("expected starting inventory CharacterID=%d, got %d", p.Character.ID, startingInv.CharacterID)
				}
				if startingInv.Gold != 15 {
					t.Errorf("expected fighter starting gold=15, got %d", startingInv.Gold)
				}
				if len(startingInv.Items) == 0 {
					t.Error("expected starting inventory to contain items for a fighter")
				}
				if len(startingInv.GetEquippedItems()) == 0 {
					t.Error("expected fighter to start with an equipped weapon/armor")
				}
			},
		},
		{
			name: "no session",
			req: CreateCharacterRequest{
				ChatID: 12345,
				Name:   "Test Hero",
				Race:   character.RaceHuman,
				Class:  character.ClassFighter,
			},
			setupMocks: func(sessionRepo *mockSessionRepo, playerRepo *mockPlayerRepo, inventoryRepo *mockInventoryRepo) {
				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return nil, nil
				}
			},
			expectedError: true,
		},
		{
			name: "inactive session",
			req: CreateCharacterRequest{
				ChatID: 12345,
				Name:   "Test Hero",
				Race:   character.RaceHuman,
				Class:  character.ClassFighter,
			},
			setupMocks: func(sessionRepo *mockSessionRepo, playerRepo *mockPlayerRepo, inventoryRepo *mockInventoryRepo) {
				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return &session.GameSession{
						ChatID: chatID,
						State:  session.StateDone,
					}, nil
				}
			},
			expectedError: true,
		},
		{
			name: "character already exists",
			req: CreateCharacterRequest{
				ChatID: 12345,
				Name:   "Test Hero",
				Race:   character.RaceHuman,
				Class:  character.ClassFighter,
			},
			setupMocks: func(sessionRepo *mockSessionRepo, playerRepo *mockPlayerRepo, inventoryRepo *mockInventoryRepo) {
				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return &session.GameSession{
						ChatID: chatID,
						State:  session.StateActive,
					}, nil
				}
				playerRepo.getByTgUserIDAndSessionIDFunc = func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
					return &player.Player{
						TgUserID: tgUserID,
						Name:     "Existing Hero",
					}, nil
				}
			},
			expectedError: true,
		},
		{
			name: "auto-generated stats",
			req: CreateCharacterRequest{
				ChatID: 12345,
				Name:   "Test Hero",
				Race:   character.RaceHuman,
				Class:  character.ClassFighter,
				Stats:  nil, // Should be auto-generated
			},
			setupMocks: func(sessionRepo *mockSessionRepo, playerRepo *mockPlayerRepo, inventoryRepo *mockInventoryRepo) {
				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return &session.GameSession{
						ChatID: chatID,
						State:  session.StateActive,
					}, nil
				}
				playerRepo.getByTgUserIDAndSessionIDFunc = func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
					return nil, nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, p *player.Player, inventoryRepo *mockInventoryRepo) {
				if p == nil {
					t.Fatal("expected player, got nil")
				}
				// Проверяем, что характеристики были сгенерированы
				stats := p.Character.Stats
				if stats.Strength == 0 {
					t.Error("expected stats to be generated")
				}
				// Характеристики должны быть в разумных пределах (3-18 для 4d6 drop lowest)
				if stats.Strength < 3 || stats.Strength > 18 {
					t.Errorf("strength out of range: %d", stats.Strength)
				}
			},
		},
		{
			name: "custom stats",
			req: CreateCharacterRequest{
				ChatID: 12345,
				Name:   "Test Hero",
				Race:   character.RaceHuman,
				Class:  character.ClassFighter,
				Stats: &character.Stats{
					Strength:     16,
					Dexterity:    14,
					Constitution: 15,
					Intelligence: 12,
					Wisdom:       13,
					Charisma:     10,
				},
			},
			setupMocks: func(sessionRepo *mockSessionRepo, playerRepo *mockPlayerRepo, inventoryRepo *mockInventoryRepo) {
				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return &session.GameSession{
						ChatID: chatID,
						State:  session.StateActive,
					}, nil
				}
				playerRepo.getByTgUserIDAndSessionIDFunc = func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
					return nil, nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, p *player.Player, inventoryRepo *mockInventoryRepo) {
				if p == nil {
					t.Fatal("expected player, got nil")
				}
				// Для Human применяется расовый бонус +1 ко всем характеристикам.
				if p.Character.Stats.Strength != 17 {
					t.Errorf("expected strength 17 (16 + human bonus), got %d", p.Character.Stats.Strength)
				}
			},
		},
		{
			name: "wizard gets fixed starting spellbook",
			req: CreateCharacterRequest{
				ChatID: 12345,
				Name:   "Test Wizard",
				Race:   character.RaceHuman,
				Class:  character.ClassWizard,
			},
			setupMocks: func(sessionRepo *mockSessionRepo, playerRepo *mockPlayerRepo, inventoryRepo *mockInventoryRepo) {
				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return &session.GameSession{
						ChatID: chatID,
						State:  session.StateActive,
					}, nil
				}
				playerRepo.getByTgUserIDAndSessionIDFunc = func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
					return nil, nil
				}
			},
			setupSpells: func(spellRepo *mockSpellRepo) {
				spellRepo.getByClassFunc = func(ctx context.Context, class character.Class) ([]*spell.Spell, error) {
					return []*spell.Spell{
						{ID: 1, Name: "Магическая стрела", Level: 1},
						{ID: 2, Name: "Щит", Level: 1},
						{ID: 3, Name: "Огненный шар", Level: 2}, // не входит в стартовый список абилок
					}, nil
				}
			},
			expectedError: false,
			validateSpell: func(t *testing.T, spellRepo *mockSpellRepo) {
				// Книга заклинаний должна содержать ровно те заклинания, что перечислены
				// в character.GetCharacterAbilities как стартовые способности мага —
				// не больше и не меньше, чтобы нельзя было скастовать что угодно.
				if len(spellRepo.savedCharacterSpells) != 2 {
					t.Fatalf("expected exactly 2 starting spells learned, got %d", len(spellRepo.savedCharacterSpells))
				}
				learnedIDs := map[uint]bool{}
				for _, cs := range spellRepo.savedCharacterSpells {
					learnedIDs[cs.SpellID] = true
					if !cs.Prepared {
						t.Errorf("expected starting spell %d to be Prepared", cs.SpellID)
					}
				}
				if !learnedIDs[1] || !learnedIDs[2] {
					t.Errorf("expected 'Магическая стрела' (id=1) and 'Щит' (id=2) to be learned, got %v", learnedIDs)
				}
				if learnedIDs[3] {
					t.Error("expected 'Огненный шар' (id=3) NOT to be learned automatically")
				}
			},
		},
		{
			name: "non-wizard spellcaster is not restricted to a fixed spellbook",
			req: CreateCharacterRequest{
				ChatID: 12345,
				Name:   "Test Cleric",
				Race:   character.RaceHuman,
				Class:  character.ClassCleric,
			},
			setupMocks: func(sessionRepo *mockSessionRepo, playerRepo *mockPlayerRepo, inventoryRepo *mockInventoryRepo) {
				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return &session.GameSession{
						ChatID: chatID,
						State:  session.StateActive,
					}, nil
				}
				playerRepo.getByTgUserIDAndSessionIDFunc = func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
					return nil, nil
				}
			},
			expectedError: false,
			validateSpell: func(t *testing.T, spellRepo *mockSpellRepo) {
				// Жрец — подготавливающий заклинатель (UsesSpellbook() == false), поэтому
				// при создании персонажа книга заклинаний не наполняется вообще.
				if len(spellRepo.savedCharacterSpells) != 0 {
					t.Errorf("expected no CharacterSpell records for a preparing caster, got %d", len(spellRepo.savedCharacterSpells))
				}
			},
		},
		{
			name: "session repo error",
			req: CreateCharacterRequest{
				ChatID: 12345,
				Name:   "Test Hero",
				Race:   character.RaceHuman,
				Class:  character.ClassFighter,
			},
			setupMocks: func(sessionRepo *mockSessionRepo, playerRepo *mockPlayerRepo, inventoryRepo *mockInventoryRepo) {
				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return nil, errors.New("database error")
				}
			},
			expectedError: true,
		},
		{
			name: "player repo save error",
			req: CreateCharacterRequest{
				ChatID: 12345,
				Name:   "Test Hero",
				Race:   character.RaceHuman,
				Class:  character.ClassFighter,
			},
			setupMocks: func(sessionRepo *mockSessionRepo, playerRepo *mockPlayerRepo, inventoryRepo *mockInventoryRepo) {
				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return &session.GameSession{
						ChatID: chatID,
						State:  session.StateActive,
					}, nil
				}
				playerRepo.getByTgUserIDAndSessionIDFunc = func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
					return nil, nil
				}
				playerRepo.saveFunc = func(ctx context.Context, p *player.Player) error {
					return errors.New("save error")
				}
			},
			expectedError: true,
		},
		{
			name: "inventory repo save error",
			req: CreateCharacterRequest{
				ChatID: 12345,
				Name:   "Test Hero",
				Race:   character.RaceHuman,
				Class:  character.ClassFighter,
			},
			setupMocks: func(sessionRepo *mockSessionRepo, playerRepo *mockPlayerRepo, inventoryRepo *mockInventoryRepo) {
				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return &session.GameSession{
						ChatID: chatID,
						State:  session.StateActive,
					}, nil
				}
				playerRepo.getByTgUserIDAndSessionIDFunc = func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
					return nil, nil
				}
				inventoryRepo.saveFunc = func(ctx context.Context, inv *inventory.Inventory) error {
					return errors.New("inventory save error")
				}
			},
			expectedError: true,
		},
		{
			name: "empty name - should fail validation",
			req: CreateCharacterRequest{
				ChatID: 12345,
				Name:   "", // Empty name
				Race:   character.RaceHuman,
				Class:  character.ClassFighter,
			},
			setupMocks: func(sessionRepo *mockSessionRepo, playerRepo *mockPlayerRepo, inventoryRepo *mockInventoryRepo) {
				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return &session.GameSession{
						ChatID: chatID,
						State:  session.StateActive,
					}, nil
				}
				playerRepo.getByTgUserIDAndSessionIDFunc = func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
					return nil, nil
				}
			},
			expectedError: true, // Should fail because empty name is invalid
		},
		{
			name: "whitespace only name - should fail validation",
			req: CreateCharacterRequest{
				ChatID: 12345,
				Name:   "   ", // Whitespace only
				Race:   character.RaceHuman,
				Class:  character.ClassFighter,
			},
			setupMocks: func(sessionRepo *mockSessionRepo, playerRepo *mockPlayerRepo, inventoryRepo *mockInventoryRepo) {
				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return &session.GameSession{
						ChatID: chatID,
						State:  session.StateActive,
					}, nil
				}
				playerRepo.getByTgUserIDAndSessionIDFunc = func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
					return nil, nil
				}
			},
			expectedError: true, // Should fail because whitespace-only name is invalid
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessionRepo := &mockSessionRepo{}
			playerRepo := &mockPlayerRepo{}
			inventoryRepo := &mockInventoryRepo{}
			spellRepo := &mockSpellRepo{}

			if tt.setupMocks != nil {
				tt.setupMocks(sessionRepo, playerRepo, inventoryRepo)
			}
			if tt.setupSpells != nil {
				tt.setupSpells(spellRepo)
			}

			uc := NewCreateCharacterUseCase(sessionRepo, playerRepo, inventoryRepo, spellRepo)

			result, err := uc.Execute(context.Background(), tt.req)

			if tt.expectedError {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if tt.validate != nil {
					tt.validate(t, result, inventoryRepo)
				}
				if tt.validateSpell != nil {
					tt.validateSpell(t, spellRepo)
				}
			}
		})
	}
}

func TestGenerateStats(t *testing.T) {
	stats := generateStats()

	if stats == nil {
		t.Fatal("expected stats, got nil")
	}

	// Проверяем, что все характеристики сгенерированы
	if stats.Strength == 0 {
		t.Error("strength should be generated")
	}
	if stats.Dexterity == 0 {
		t.Error("dexterity should be generated")
	}
	if stats.Constitution == 0 {
		t.Error("constitution should be generated")
	}
	if stats.Intelligence == 0 {
		t.Error("intelligence should be generated")
	}
	if stats.Wisdom == 0 {
		t.Error("wisdom should be generated")
	}
	if stats.Charisma == 0 {
		t.Error("charisma should be generated")
	}

	// Характеристики должны быть в разумных пределах
	// Минимум 8 (стандарт D&D 5e для предотвращения критически слабых персонажей)
	// Максимум 18 (максимум для 4d6 drop lowest)
	allStats := []int{
		stats.Strength,
		stats.Dexterity,
		stats.Constitution,
		stats.Intelligence,
		stats.Wisdom,
		stats.Charisma,
	}

	for i, stat := range allStats {
		if stat < 8 || stat > 18 {
			t.Errorf("stat %d out of range: %d (expected 8-18, minimum 8 to prevent critically weak characters)", i, stat)
		}
	}
}
