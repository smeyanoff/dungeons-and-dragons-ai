package dm_tools

import (
	"context"
	"errors"
	"testing"

	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/combat"
	"dungeons-and-dragons-ai/internal/game/domain/player"
	"dungeons-and-dragons-ai/internal/game/domain/session"
)

// Mock Combat Repository
type mockCombatRepo struct {
	getActiveBySessionIDFunc func(ctx context.Context, sessionID uint) (*combat.Combat, error)
	saveFunc                  func(ctx context.Context, c *combat.Combat) error
}

func (m *mockCombatRepo) GetActiveBySessionID(ctx context.Context, sessionID uint) (*combat.Combat, error) {
	if m.getActiveBySessionIDFunc != nil {
		return m.getActiveBySessionIDFunc(ctx, sessionID)
	}
	return nil, nil
}

func (m *mockCombatRepo) Save(ctx context.Context, c *combat.Combat) error {
	if m.saveFunc != nil {
		return m.saveFunc(ctx, c)
	}
	return nil
}

// Mock Game Session Repository
type mockGameSessionRepo struct {
	getByChatIDFunc func(ctx context.Context, chatID int64) (*session.GameSession, error)
}

func (m *mockGameSessionRepo) GetByChatID(ctx context.Context, chatID int64) (*session.GameSession, error) {
	if m.getByChatIDFunc != nil {
		return m.getByChatIDFunc(ctx, chatID)
	}
	return nil, nil
}

// Mock Player Repository
type mockPlayerRepo struct {
	getByTgUserIDAndSessionIDFunc func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error)
	saveFunc                       func(ctx context.Context, p *player.Player) error
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

// Helper functions
func createTestCharacter() *character.Character {
	char, _ := character.NewCharacter("Test Hero", character.ClassFighter, character.RaceHuman, character.Stats{
		Strength:     16,
		Dexterity:    14,
		Constitution: 15,
		Intelligence: 12,
		Wisdom:       13,
		Charisma:     10,
	})
	char.ID = 1
	return char
}

func createTestCombat() *combat.Combat {
	char := createTestCharacter()
	charID := char.ID

	c := &combat.Combat{
		GameSessionID: 1,
		State:         combat.CombatStateActive,
		Participants: []combat.CombatParticipant{
			{
				IsPlayer:    true,
				Character:   char,
				CharacterID: &charID,
			},
			{
				IsPlayer:     false,
				MonsterName:  "Goblin",
				MonsterHP:    10,
				MonsterMaxHP: 10,
				MonsterAC:    15,
			},
		},
		CurrentTurn: 0,
	}
	c.ID = 1
	return c
}

// Test CheckCombatStatusTool
func TestCheckCombatStatusTool_Name(t *testing.T) {
	tool := NewCheckCombatStatusTool(nil, 1)
	if tool.Name() != "check_combat_status" {
		t.Errorf("expected name 'check_combat_status', got '%s'", tool.Name())
	}
}

func TestCheckCombatStatusTool_Description(t *testing.T) {
	tool := NewCheckCombatStatusTool(nil, 1)
	if tool.Description() == "" {
		t.Error("expected non-empty description")
	}
}

func TestCheckCombatStatusTool_Execute(t *testing.T) {
	tests := []struct {
		name          string
		sessionID     uint
		setupMock     func(*mockCombatRepo)
		expectedError bool
		validate      func(*testing.T, interface{})
	}{
		{
			name:      "successful check with active combat",
			sessionID: 1,
			setupMock: func(repo *mockCombatRepo) {
				repo.getActiveBySessionIDFunc = func(ctx context.Context, sessionID uint) (*combat.Combat, error) {
					return createTestCombat(), nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if active, ok := resultMap["active"].(bool); !ok || !active {
					t.Errorf("expected active=true, got %v", resultMap["active"])
				}

				if state, ok := resultMap["state"].(string); !ok || state != "active" {
					t.Errorf("expected state='active', got %v", resultMap["state"])
				}
			},
		},
		{
			name:      "no active combat",
			sessionID: 1,
			setupMock: func(repo *mockCombatRepo) {
				repo.getActiveBySessionIDFunc = func(ctx context.Context, sessionID uint) (*combat.Combat, error) {
					return nil, nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if active, ok := resultMap["active"].(bool); !ok || active {
					t.Errorf("expected active=false, got %v", resultMap["active"])
				}
			},
		},
		{
			name:      "repository error",
			sessionID: 1,
			setupMock: func(repo *mockCombatRepo) {
				repo.getActiveBySessionIDFunc = func(ctx context.Context, sessionID uint) (*combat.Combat, error) {
					return nil, errors.New("database error")
				}
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockCombatRepo{}
			if tt.setupMock != nil {
				tt.setupMock(repo)
			}

			tool := NewCheckCombatStatusTool(repo, tt.sessionID)
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

// Test PerformCombatAttackTool
func TestPerformCombatAttackTool_Name(t *testing.T) {
	tool := NewPerformCombatAttackTool(nil, nil, 1)
	if tool.Name() != "perform_combat_attack" {
		t.Errorf("expected name 'perform_combat_attack', got '%s'", tool.Name())
	}
}

func TestPerformCombatAttackTool_Description(t *testing.T) {
	tool := NewPerformCombatAttackTool(nil, nil, 1)
	if tool.Description() == "" {
		t.Error("expected non-empty description")
	}
}

func TestPerformCombatAttackTool_Execute(t *testing.T) {
	tests := []struct {
		name          string
		sessionID     uint
		setupMock     func(*mockCombatRepo, *mockGameSessionRepo)
		expectedError bool
		validate      func(*testing.T, interface{})
	}{
		{
			name:      "successful attack",
			sessionID: 1,
			setupMock: func(combatRepo *mockCombatRepo, sessionRepo *mockGameSessionRepo) {
				activeCombat := createTestCombat()
				combatRepo.getActiveBySessionIDFunc = func(ctx context.Context, sessionID uint) (*combat.Combat, error) {
					return activeCombat, nil
				}
				combatRepo.saveFunc = func(ctx context.Context, c *combat.Combat) error {
					return nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if _, ok := resultMap["attacker_name"]; !ok {
					t.Error("expected attacker_name in result")
				}

				if _, ok := resultMap["target_name"]; !ok {
					t.Error("expected target_name in result")
				}

				if _, ok := resultMap["attack_roll"]; !ok {
					t.Error("expected attack_roll in result")
				}
			},
		},
		{
			name:      "no active combat",
			sessionID: 1,
			setupMock: func(combatRepo *mockCombatRepo, sessionRepo *mockGameSessionRepo) {
				combatRepo.getActiveBySessionIDFunc = func(ctx context.Context, sessionID uint) (*combat.Combat, error) {
					return nil, nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if _, ok := resultMap["error"]; !ok {
					t.Error("expected error in result when no combat")
				}
			},
		},
		{
			name:      "no player participant",
			sessionID: 1,
			setupMock: func(combatRepo *mockCombatRepo, sessionRepo *mockGameSessionRepo) {
				char := createTestCharacter()
				char.Kill()
				charID := char.ID

				activeCombat := &combat.Combat{
					GameSessionID: 1,
					State:         combat.CombatStateActive,
					Participants: []combat.CombatParticipant{
						{
							IsPlayer:    true,
							Character:   char,
							CharacterID: &charID,
						},
						{
							IsPlayer:     false,
							MonsterName:  "Goblin",
							MonsterHP:    10,
							MonsterMaxHP: 10,
							MonsterAC:    15,
						},
					},
					CurrentTurn: 0,
				}
				activeCombat.ID = 1

				combatRepo.getActiveBySessionIDFunc = func(ctx context.Context, sessionID uint) (*combat.Combat, error) {
					return activeCombat, nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if _, ok := resultMap["error"]; !ok {
					t.Error("expected error in result when player is dead")
				}
			},
		},
		{
			name:      "all enemies defeated",
			sessionID: 1,
			setupMock: func(combatRepo *mockCombatRepo, sessionRepo *mockGameSessionRepo) {
				char := createTestCharacter()
				charID := char.ID

				activeCombat := &combat.Combat{
					GameSessionID: 1,
					State:         combat.CombatStateActive,
					Participants: []combat.CombatParticipant{
						{
							IsPlayer:    true,
							Character:   char,
							CharacterID: &charID,
						},
						{
							IsPlayer:     false,
							MonsterName:  "Goblin",
							MonsterHP:    0,
							MonsterMaxHP: 10,
							MonsterAC:    15,
						},
					},
					CurrentTurn: 0,
				}
				activeCombat.ID = 1

				combatRepo.getActiveBySessionIDFunc = func(ctx context.Context, sessionID uint) (*combat.Combat, error) {
					return activeCombat, nil
				}
				combatRepo.saveFunc = func(ctx context.Context, c *combat.Combat) error {
					return nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if finished, ok := resultMap["combat_finished"].(bool); !ok || !finished {
					t.Error("expected combat_finished=true when all enemies defeated")
				}
			},
		},
		{
			name:      "repository error",
			sessionID: 1,
			setupMock: func(combatRepo *mockCombatRepo, sessionRepo *mockGameSessionRepo) {
				combatRepo.getActiveBySessionIDFunc = func(ctx context.Context, sessionID uint) (*combat.Combat, error) {
					return nil, errors.New("database error")
				}
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			combatRepo := &mockCombatRepo{}
			sessionRepo := &mockGameSessionRepo{}
			if tt.setupMock != nil {
				tt.setupMock(combatRepo, sessionRepo)
			}

			tool := NewPerformCombatAttackTool(combatRepo, sessionRepo, tt.sessionID)
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

// Test ApplyDamageTool
func TestApplyDamageTool_Name(t *testing.T) {
	tool := NewApplyDamageTool(nil, nil, nil, 1, 12345)
	if tool.Name() != "apply_damage" {
		t.Errorf("expected name 'apply_damage', got '%s'", tool.Name())
	}
}

func TestApplyDamageTool_Description(t *testing.T) {
	tool := NewApplyDamageTool(nil, nil, nil, 1, 12345)
	if tool.Description() == "" {
		t.Error("expected non-empty description")
	}
}

func TestApplyDamageTool_Execute(t *testing.T) {
	tests := []struct {
		name          string
		sessionID     uint
		chatID        int64
		args          map[string]interface{}
		setupMock     func(*mockCombatRepo, *mockGameSessionRepo, *mockPlayerRepo)
		expectedError bool
		validate      func(*testing.T, interface{})
	}{
		{
			name:      "successful damage to player",
			sessionID: 1,
			chatID:    12345,
			args: map[string]interface{}{
				"target_type":   "player",
				"target_name":   "player",
				"damage_amount": 5.0,
			},
			setupMock: func(combatRepo *mockCombatRepo, sessionRepo *mockGameSessionRepo, playerRepo *mockPlayerRepo) {
				activeCombat := createTestCombat()
				combatRepo.getActiveBySessionIDFunc = func(ctx context.Context, sessionID uint) (*combat.Combat, error) {
					return activeCombat, nil
				}
				combatRepo.saveFunc = func(ctx context.Context, c *combat.Combat) error {
					return nil
				}
				playerRepo.getByTgUserIDAndSessionIDFunc = func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
					char := createTestCharacter()
					return &player.Player{
						Character:   *char,
						CharacterID: char.ID,
					}, nil
				}
				playerRepo.saveFunc = func(ctx context.Context, p *player.Player) error {
					return nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if damage, ok := resultMap["damage"].(int); !ok || damage != 5 {
					t.Errorf("expected damage=5, got %v", resultMap["damage"])
				}

				if _, ok := resultMap["message"]; !ok {
					t.Error("expected message in result")
				}

				if newHP, ok := resultMap["new_hp"].(int); !ok || newHP <= 0 {
					t.Errorf("expected new_hp > 0, got %v", resultMap["new_hp"])
				}
			},
		},
		{
			name:      "successful damage to monster",
			sessionID: 1,
			chatID:    12345,
			args: map[string]interface{}{
				"target_type":   "monster",
				"target_name":   "Goblin",
				"damage_amount": 3.0,
			},
			setupMock: func(combatRepo *mockCombatRepo, sessionRepo *mockGameSessionRepo, playerRepo *mockPlayerRepo) {
				activeCombat := createTestCombat()
				combatRepo.getActiveBySessionIDFunc = func(ctx context.Context, sessionID uint) (*combat.Combat, error) {
					return activeCombat, nil
				}
				combatRepo.saveFunc = func(ctx context.Context, c *combat.Combat) error {
					return nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if targetName, ok := resultMap["target_name"].(string); !ok || targetName != "Goblin" {
					t.Errorf("expected target_name='Goblin', got %v", resultMap["target_name"])
				}

				if damage, ok := resultMap["damage"].(int); !ok || damage != 3 {
					t.Errorf("expected damage=3, got %v", resultMap["damage"])
				}
			},
		},
		{
			name:      "no active combat",
			sessionID: 1,
			chatID:    12345,
			args: map[string]interface{}{
				"target_type":   "player",
				"target_name":   "player",
				"damage_amount": 5.0,
			},
			setupMock: func(combatRepo *mockCombatRepo, sessionRepo *mockGameSessionRepo, playerRepo *mockPlayerRepo) {
				combatRepo.getActiveBySessionIDFunc = func(ctx context.Context, sessionID uint) (*combat.Combat, error) {
					return nil, nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if _, ok := resultMap["error"]; !ok {
					t.Error("expected error in result when no combat")
				}
			},
		},
		{
			name:      "invalid target_type",
			sessionID: 1,
			chatID:    12345,
			args: map[string]interface{}{
				"target_type":   "invalid",
				"target_name":   "player",
				"damage_amount": 5.0,
			},
			setupMock: func(combatRepo *mockCombatRepo, sessionRepo *mockGameSessionRepo, playerRepo *mockPlayerRepo) {
				activeCombat := createTestCombat()
				combatRepo.getActiveBySessionIDFunc = func(ctx context.Context, sessionID uint) (*combat.Combat, error) {
					return activeCombat, nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if _, ok := resultMap["error"]; !ok {
					t.Error("expected error in result when invalid target_type")
				}
			},
		},
		{
			name:      "missing target_type",
			sessionID: 1,
			chatID:    12345,
			args: map[string]interface{}{
				"target_name":   "player",
				"damage_amount": 5.0,
			},
			setupMock: func(combatRepo *mockCombatRepo, sessionRepo *mockGameSessionRepo, playerRepo *mockPlayerRepo) {
			},
			expectedError: true,
		},
		{
			name:      "missing damage_amount",
			sessionID: 1,
			chatID:    12345,
			args: map[string]interface{}{
				"target_type": "player",
				"target_name": "player",
			},
			setupMock: func(combatRepo *mockCombatRepo, sessionRepo *mockGameSessionRepo, playerRepo *mockPlayerRepo) {
			},
			expectedError: true,
		},
		{
			name:      "zero damage_amount",
			sessionID: 1,
			chatID:    12345,
			args: map[string]interface{}{
				"target_type":   "player",
				"target_name":   "player",
				"damage_amount": 0.0,
			},
			setupMock: func(combatRepo *mockCombatRepo, sessionRepo *mockGameSessionRepo, playerRepo *mockPlayerRepo) {
				activeCombat := createTestCombat()
				combatRepo.getActiveBySessionIDFunc = func(ctx context.Context, sessionID uint) (*combat.Combat, error) {
					return activeCombat, nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if _, ok := resultMap["error"]; !ok {
					t.Error("expected error in result when damage_amount is zero")
				}
			},
		},
		{
			name:      "target not found",
			sessionID: 1,
			chatID:    12345,
			args: map[string]interface{}{
				"target_type":   "monster",
				"target_name":   "Dragon",
				"damage_amount": 5.0,
			},
			setupMock: func(combatRepo *mockCombatRepo, sessionRepo *mockGameSessionRepo, playerRepo *mockPlayerRepo) {
				char := createTestCharacter()
				charID := char.ID

				activeCombat := &combat.Combat{
					GameSessionID: 1,
					State:         combat.CombatStateActive,
					Participants: []combat.CombatParticipant{
						{
							IsPlayer:    true,
							Character:   char,
							CharacterID: &charID,
						},
						{
							IsPlayer:     false,
							MonsterName:  "Goblin",
							MonsterHP:    0,
							MonsterMaxHP: 10,
							MonsterAC:    15,
						},
					},
					CurrentTurn: 0,
				}
				activeCombat.ID = 1

				combatRepo.getActiveBySessionIDFunc = func(ctx context.Context, sessionID uint) (*combat.Combat, error) {
					return activeCombat, nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if _, ok := resultMap["error"]; !ok {
					t.Error("expected error in result when target not found")
				}
			},
		},
		{
			name:      "damage kills monster - combat ends",
			sessionID: 1,
			chatID:    12345,
			args: map[string]interface{}{
				"target_type":   "monster",
				"target_name":   "Goblin",
				"damage_amount": 20.0,
			},
			setupMock: func(combatRepo *mockCombatRepo, sessionRepo *mockGameSessionRepo, playerRepo *mockPlayerRepo) {
				activeCombat := createTestCombat()
				combatRepo.getActiveBySessionIDFunc = func(ctx context.Context, sessionID uint) (*combat.Combat, error) {
					return activeCombat, nil
				}
				combatRepo.saveFunc = func(ctx context.Context, c *combat.Combat) error {
					return nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if isDead, ok := resultMap["is_dead"].(bool); !ok || !isDead {
					t.Error("expected is_dead=true when monster is killed")
				}

				if finished, ok := resultMap["combat_finished"].(bool); !ok || !finished {
					t.Error("expected combat_finished=true when all enemies defeated")
				}

				if victory, ok := resultMap["victory"].(bool); !ok || !victory {
					t.Error("expected victory=true when all enemies defeated")
				}
			},
		},
		{
			name:      "repository error getting combat",
			sessionID: 1,
			chatID:    12345,
			args: map[string]interface{}{
				"target_type":   "player",
				"target_name":   "player",
				"damage_amount": 5.0,
			},
			setupMock: func(combatRepo *mockCombatRepo, sessionRepo *mockGameSessionRepo, playerRepo *mockPlayerRepo) {
				combatRepo.getActiveBySessionIDFunc = func(ctx context.Context, sessionID uint) (*combat.Combat, error) {
					return nil, errors.New("database error")
				}
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			combatRepo := &mockCombatRepo{}
			sessionRepo := &mockGameSessionRepo{}
			playerRepo := &mockPlayerRepo{}
			if tt.setupMock != nil {
				tt.setupMock(combatRepo, sessionRepo, playerRepo)
			}

			tool := NewApplyDamageTool(combatRepo, sessionRepo, playerRepo, tt.sessionID, tt.chatID)
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

// Test GetCombatParticipantStatsTool
func TestGetCombatParticipantStatsTool_Name(t *testing.T) {
	tool := NewGetCombatParticipantStatsTool(nil, 1)
	if tool.Name() != "get_combat_participant_stats" {
		t.Errorf("expected name 'get_combat_participant_stats', got '%s'", tool.Name())
	}
}

func TestGetCombatParticipantStatsTool_Execute(t *testing.T) {
	tests := []struct {
		name          string
		sessionID     uint
		args          map[string]interface{}
		setupMock     func(*mockCombatRepo)
		expectedError bool
		validate      func(*testing.T, interface{})
	}{
		{
			name:      "successful stats retrieval - current participant",
			sessionID: 1,
			args:      map[string]interface{}{},
			setupMock: func(repo *mockCombatRepo) {
				activeCombat := createTestCombat()
				repo.getActiveBySessionIDFunc = func(ctx context.Context, sessionID uint) (*combat.Combat, error) {
					return activeCombat, nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if _, ok := resultMap["name"]; !ok {
					t.Error("expected name in result")
				}

				if _, ok := resultMap["hp"]; !ok {
					t.Error("expected hp in result")
				}

				if _, ok := resultMap["ac"]; !ok {
					t.Error("expected ac in result")
				}

				if _, ok := resultMap["attack_bonus"]; !ok {
					t.Error("expected attack_bonus in result")
				}
			},
		},
		{
			name:      "no active combat",
			sessionID: 1,
			args:      map[string]interface{}{},
			setupMock: func(repo *mockCombatRepo) {
				repo.getActiveBySessionIDFunc = func(ctx context.Context, sessionID uint) (*combat.Combat, error) {
					return nil, nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if active, ok := resultMap["active"].(bool); !ok || active {
					t.Error("expected active=false when no combat")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockCombatRepo{}
			if tt.setupMock != nil {
				tt.setupMock(repo)
			}

			tool := NewGetCombatParticipantStatsTool(repo, tt.sessionID)
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

// Test CompareAttackVsDefenseTool
func TestCompareAttackVsDefenseTool_Name(t *testing.T) {
	tool := NewCompareAttackVsDefenseTool(nil, 1)
	if tool.Name() != "compare_attack_vs_defense" {
		t.Errorf("expected name 'compare_attack_vs_defense', got '%s'", tool.Name())
	}
}

func TestCompareAttackVsDefenseTool_Execute(t *testing.T) {
	tests := []struct {
		name          string
		sessionID     uint
		args          map[string]interface{}
		setupMock     func(*mockCombatRepo)
		expectedError bool
		validate      func(*testing.T, interface{})
	}{
		{
			name:      "successful comparison",
			sessionID: 1,
			args:      map[string]interface{}{},
			setupMock: func(repo *mockCombatRepo) {
				activeCombat := createTestCombat()
				repo.getActiveBySessionIDFunc = func(ctx context.Context, sessionID uint) (*combat.Combat, error) {
					return activeCombat, nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if _, ok := resultMap["attacker_name"]; !ok {
					t.Error("expected attacker_name in result")
				}

				if _, ok := resultMap["min_roll_to_hit"]; !ok {
					t.Error("expected min_roll_to_hit in result")
				}
			},
		},
		{
			name:      "no active combat",
			sessionID: 1,
			args:      map[string]interface{}{},
			setupMock: func(repo *mockCombatRepo) {
				repo.getActiveBySessionIDFunc = func(ctx context.Context, sessionID uint) (*combat.Combat, error) {
					return nil, nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if _, ok := resultMap["error"]; !ok {
					t.Error("expected error in result when no combat")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockCombatRepo{}
			if tt.setupMock != nil {
				tt.setupMock(repo)
			}

			tool := NewCompareAttackVsDefenseTool(repo, tt.sessionID)
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

// Test PerformEnemyAttackTool
func TestPerformEnemyAttackTool_Name(t *testing.T) {
	tool := NewPerformEnemyAttackTool(nil, nil, nil, 1, 12345)
	if tool.Name() != "perform_enemy_attack" {
		t.Errorf("expected name 'perform_enemy_attack', got '%s'", tool.Name())
	}
}

func TestPerformEnemyAttackTool_Execute(t *testing.T) {
	tests := []struct {
		name          string
		sessionID     uint
		chatID        int64
		args          map[string]interface{}
		setupMock     func(*mockCombatRepo, *mockGameSessionRepo, *mockPlayerRepo)
		expectedError bool
		validate      func(*testing.T, interface{})
	}{
		{
			name:      "successful enemy attack",
			sessionID: 1,
			chatID:    12345,
			args:      map[string]interface{}{},
			setupMock: func(combatRepo *mockCombatRepo, sessionRepo *mockGameSessionRepo, playerRepo *mockPlayerRepo) {
				activeCombat := createTestCombat()
				activeCombat.CurrentTurn = 1
				combatRepo.getActiveBySessionIDFunc = func(ctx context.Context, sessionID uint) (*combat.Combat, error) {
					return activeCombat, nil
				}
				combatRepo.saveFunc = func(ctx context.Context, c *combat.Combat) error {
					return nil
				}
				playerRepo.getByTgUserIDAndSessionIDFunc = func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
					char := createTestCharacter()
					return &player.Player{Character: *char, CharacterID: char.ID}, nil
				}
				playerRepo.saveFunc = func(ctx context.Context, p *player.Player) error {
					return nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if _, ok := resultMap["attacker_name"]; !ok {
					t.Error("expected attacker_name in result")
				}
			},
		},
		{
			name:      "no active combat",
			sessionID: 1,
			chatID:    12345,
			args:      map[string]interface{}{},
			setupMock: func(combatRepo *mockCombatRepo, sessionRepo *mockGameSessionRepo, playerRepo *mockPlayerRepo) {
				combatRepo.getActiveBySessionIDFunc = func(ctx context.Context, sessionID uint) (*combat.Combat, error) {
					return nil, nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if _, ok := resultMap["error"]; !ok {
					t.Error("expected error in result when no combat")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			combatRepo := &mockCombatRepo{}
			sessionRepo := &mockGameSessionRepo{}
			playerRepo := &mockPlayerRepo{}
			if tt.setupMock != nil {
				tt.setupMock(combatRepo, sessionRepo, playerRepo)
			}

			tool := NewPerformEnemyAttackTool(combatRepo, sessionRepo, playerRepo, tt.sessionID, tt.chatID)
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

// Test GetBattlefieldStatusTool
func TestGetBattlefieldStatusTool_Name(t *testing.T) {
	tool := NewGetBattlefieldStatusTool(nil, 1)
	if tool.Name() != "get_battlefield_status" {
		t.Errorf("expected name 'get_battlefield_status', got '%s'", tool.Name())
	}
}

func TestGetBattlefieldStatusTool_Execute(t *testing.T) {
	tests := []struct {
		name          string
		sessionID     uint
		args          map[string]interface{}
		setupMock     func(*mockCombatRepo)
		expectedError bool
		validate      func(*testing.T, interface{})
	}{
		{
			name:      "successful battlefield status - table format",
			sessionID: 1,
			args: map[string]interface{}{
				"format": "table",
			},
			setupMock: func(repo *mockCombatRepo) {
				activeCombat := createTestCombat()
				repo.getActiveBySessionIDFunc = func(ctx context.Context, sessionID uint) (*combat.Combat, error) {
					return activeCombat, nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if active, ok := resultMap["active"].(bool); !ok || !active {
					t.Error("expected active=true")
				}

				if battlefield, ok := resultMap["battlefield"].(string); !ok || battlefield == "" {
					t.Error("expected non-empty battlefield visualization")
				}
			},
		},
		{
			name:      "no active combat",
			sessionID: 1,
			args:      map[string]interface{}{},
			setupMock: func(repo *mockCombatRepo) {
				repo.getActiveBySessionIDFunc = func(ctx context.Context, sessionID uint) (*combat.Combat, error) {
					return nil, nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map[string]interface{}, got %T", result)
				}

				if active, ok := resultMap["active"].(bool); !ok || active {
					t.Error("expected active=false when no combat")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockCombatRepo{}
			if tt.setupMock != nil {
				tt.setupMock(repo)
			}

			tool := NewGetBattlefieldStatusTool(repo, tt.sessionID)
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
