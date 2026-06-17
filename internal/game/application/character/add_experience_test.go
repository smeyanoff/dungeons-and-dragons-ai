package character

import (
	"context"
	"errors"
	"testing"

	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/player"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/game/domain/world"
)

// Mock Player Repository
type mockAddExpPlayerRepo struct {
	getByTgUserIDAndSessionIDFunc func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error)
	saveFunc                      func(ctx context.Context, p *player.Player) error
	savedPlayers                  []*player.Player
}

func (m *mockAddExpPlayerRepo) GetByTgUserIDAndSessionID(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
	if m.getByTgUserIDAndSessionIDFunc != nil {
		return m.getByTgUserIDAndSessionIDFunc(ctx, tgUserID, sessionID)
	}
	return nil, nil
}

func (m *mockAddExpPlayerRepo) Save(ctx context.Context, p *player.Player) error {
	if m.saveFunc != nil {
		return m.saveFunc(ctx, p)
	}
	if m.savedPlayers == nil {
		m.savedPlayers = make([]*player.Player, 0)
	}
	m.savedPlayers = append(m.savedPlayers, p)
	return nil
}

// Mock Session Repository
type mockAddExpSessionRepo struct {
	getByChatIDFunc func(ctx context.Context, chatID int64) (*session.GameSession, error)
}

func (m *mockAddExpSessionRepo) GetByChatID(ctx context.Context, chatID int64) (*session.GameSession, error) {
	if m.getByChatIDFunc != nil {
		return m.getByChatIDFunc(ctx, chatID)
	}
	return nil, nil
}

func (m *mockAddExpSessionRepo) Save(ctx context.Context, s *session.GameSession) error {
	return nil
}

func (m *mockAddExpSessionRepo) Delete(ctx context.Context, chatID int64) error {
	return nil
}

func TestAddExperienceUseCase_Execute(t *testing.T) {
	tests := []struct {
		name          string
		req           AddExperienceRequest
		setupMocks    func(*mockAddExpPlayerRepo, *mockAddExpSessionRepo)
		expectedError bool
		validate      func(*testing.T, *player.Player, bool)
	}{
		{
			name: "successful experience addition",
			req: AddExperienceRequest{
				ChatID: 12345,
				Amount: 100,
				Reason: "quest_completed",
			},
			setupMocks: func(playerRepo *mockAddExpPlayerRepo, sessionRepo *mockAddExpSessionRepo) {
				char, _ := character.NewCharacter("Test Hero", character.ClassFighter, character.RaceHuman, character.Stats{
					Strength: 16,
				})
				char.Experience = 0
				char.Level = 1

				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					gs := &session.GameSession{
						ChatID: chatID,
						State:  session.StateActive,
						World:  world.World{Name: "Test World"},
					}
					gs.Model.ID = 1
					return gs, nil
				}
				playerRepo.getByTgUserIDAndSessionIDFunc = func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
					return &player.Player{
						TgUserID:      tgUserID,
						GameSessionID: sessionID,
						Character:     *char,
					}, nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, p *player.Player, leveledUp bool) {
				if p == nil {
					t.Fatal("expected player, got nil")
				}
				if p.Character.Experience != 100 {
					t.Errorf("expected experience 100, got %d", p.Character.Experience)
				}
				// На 1 уровне 100 опыта недостаточно для повышения уровня (нужно 300)
				if leveledUp {
					t.Error("expected no level up with 100 XP at level 1")
				}
			},
		},
		{
			name: "level up",
			req: AddExperienceRequest{
				ChatID: 12345,
				Amount: 300,
				Reason: "quest_completed",
			},
			setupMocks: func(playerRepo *mockAddExpPlayerRepo, sessionRepo *mockAddExpSessionRepo) {
				char, _ := character.NewCharacter("Test Hero", character.ClassFighter, character.RaceHuman, character.Stats{
					Strength: 16,
				})
				char.Experience = 0
				char.Level = 1

				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					gs := &session.GameSession{
						ChatID: chatID,
						State:  session.StateActive,
					}
					gs.Model.ID = 1
					return gs, nil
				}
				playerRepo.getByTgUserIDAndSessionIDFunc = func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
					return &player.Player{
						TgUserID:      tgUserID,
						GameSessionID: sessionID,
						Character:     *char,
					}, nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, p *player.Player, leveledUp bool) {
				if p == nil {
					t.Fatal("expected player, got nil")
				}
				if !leveledUp {
					t.Error("expected level up with 300 XP at level 1")
				}
				if p.Character.Level != 2 {
					t.Errorf("expected level 2, got %d", p.Character.Level)
				}
			},
		},
		{
			name: "no session",
			req: AddExperienceRequest{
				ChatID: 12345,
				Amount: 100,
			},
			setupMocks: func(playerRepo *mockAddExpPlayerRepo, sessionRepo *mockAddExpSessionRepo) {
				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return nil, nil
				}
			},
			expectedError: true,
		},
		{
			name: "no player",
			req: AddExperienceRequest{
				ChatID: 12345,
				Amount: 100,
			},
			setupMocks: func(playerRepo *mockAddExpPlayerRepo, sessionRepo *mockAddExpSessionRepo) {
				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					gs := &session.GameSession{
						ChatID: chatID,
						State:  session.StateActive,
					}
					gs.Model.ID = 1
					return gs, nil
				}
				playerRepo.getByTgUserIDAndSessionIDFunc = func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
					return nil, nil
				}
			},
			expectedError: true,
		},
		{
			name: "dead character",
			req: AddExperienceRequest{
				ChatID: 12345,
				Amount: 100,
			},
			setupMocks: func(playerRepo *mockAddExpPlayerRepo, sessionRepo *mockAddExpSessionRepo) {
				char, _ := character.NewCharacter("Test Hero", character.ClassFighter, character.RaceHuman, character.Stats{
					Strength: 16,
				})
				char.Kill()

				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					gs := &session.GameSession{
						ChatID: chatID,
						State:  session.StateActive,
					}
					gs.Model.ID = 1
					return gs, nil
				}
				playerRepo.getByTgUserIDAndSessionIDFunc = func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
					return &player.Player{
						TgUserID:      tgUserID,
						GameSessionID: 1,
						Character:     *char,
					}, nil
				}
			},
			expectedError: true,
		},
		{
			name: "zero experience",
			req: AddExperienceRequest{
				ChatID: 12345,
				Amount: 0,
			},
			setupMocks: func(playerRepo *mockAddExpPlayerRepo, sessionRepo *mockAddExpSessionRepo) {
				char, _ := character.NewCharacter("Test Hero", character.ClassFighter, character.RaceHuman, character.Stats{
					Strength: 16,
				})

				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					gs := &session.GameSession{
						ChatID: chatID,
						State:  session.StateActive,
					}
					gs.Model.ID = 1
					return gs, nil
				}
				playerRepo.getByTgUserIDAndSessionIDFunc = func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
					return &player.Player{
						TgUserID:      tgUserID,
						GameSessionID: 1,
						Character:     *char,
					}, nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, p *player.Player, leveledUp bool) {
				if leveledUp {
					t.Error("expected no level up with 0 XP")
				}
			},
		},
		{
			name: "session repo error",
			req: AddExperienceRequest{
				ChatID: 12345,
				Amount: 100,
			},
			setupMocks: func(playerRepo *mockAddExpPlayerRepo, sessionRepo *mockAddExpSessionRepo) {
				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return nil, errors.New("database error")
				}
			},
			expectedError: true,
		},
		{
			name: "player repo error",
			req: AddExperienceRequest{
				ChatID: 12345,
				Amount: 100,
			},
			setupMocks: func(playerRepo *mockAddExpPlayerRepo, sessionRepo *mockAddExpSessionRepo) {
				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					gs := &session.GameSession{
						ChatID: chatID,
						State:  session.StateActive,
					}
					gs.Model.ID = 1
					return gs, nil
				}
				playerRepo.getByTgUserIDAndSessionIDFunc = func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
					return nil, errors.New("player repo error")
				}
			},
			expectedError: true,
		},
		{
			name: "save error",
			req: AddExperienceRequest{
				ChatID: 12345,
				Amount: 100,
			},
			setupMocks: func(playerRepo *mockAddExpPlayerRepo, sessionRepo *mockAddExpSessionRepo) {
				char, _ := character.NewCharacter("Test Hero", character.ClassFighter, character.RaceHuman, character.Stats{
					Strength: 16,
				})

				sessionRepo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					gs := &session.GameSession{
						ChatID: chatID,
						State:  session.StateActive,
					}
					gs.Model.ID = 1
					return gs, nil
				}
				playerRepo.getByTgUserIDAndSessionIDFunc = func(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
					return &player.Player{
						TgUserID:      tgUserID,
						GameSessionID: 1,
						Character:     *char,
					}, nil
				}
				playerRepo.saveFunc = func(ctx context.Context, p *player.Player) error {
					return errors.New("save error")
				}
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			playerRepo := &mockAddExpPlayerRepo{}
			sessionRepo := &mockAddExpSessionRepo{}

			if tt.setupMocks != nil {
				tt.setupMocks(playerRepo, sessionRepo)
			}

			uc := NewAddExperienceUseCase(playerRepo, sessionRepo)

			result, leveledUp, err := uc.Execute(context.Background(), tt.req)

			if tt.expectedError {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if tt.validate != nil {
					tt.validate(t, result, leveledUp)
				}
			}
		})
	}
}
