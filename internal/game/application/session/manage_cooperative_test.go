package session

import (
	"context"
	"errors"
	"testing"

	"dungeons-and-dragons-ai/internal/game/domain/player"
	"dungeons-and-dragons-ai/internal/game/domain/session"
)

// Mock Player Repository
type mockPlayerRepo struct {
	getByTgUserIDFunc func(ctx context.Context, tgUserID int64) (*player.Player, error)
	saveFunc          func(ctx context.Context, p *player.Player) error
}

func (m *mockPlayerRepo) GetByTgUserID(ctx context.Context, tgUserID int64) (*player.Player, error) {
	if m.getByTgUserIDFunc != nil {
		return m.getByTgUserIDFunc(ctx, tgUserID)
	}
	return nil, nil
}

func (m *mockPlayerRepo) Save(ctx context.Context, p *player.Player) error {
	if m.saveFunc != nil {
		return m.saveFunc(ctx, p)
	}
	return nil
}

func TestManageCooperativeUseCase_EnableCooperativeMode(t *testing.T) {
	tests := []struct {
		name          string
		setupMock     func() *mockSessionRepo
		request       EnableCooperativeRequest
		expectedError bool
		expectedSaved bool
	}{
		{
			name: "success - enable cooperative mode with 2 players",
			setupMock: func() *mockSessionRepo {
				gs := &session.GameSession{
					State: session.StateActive,
					Players: []player.Player{
						{ID: 1, TgUserID: 111},
					},
				}
				return &mockSessionRepo{
					getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
						return gs, nil
					},
					saveFunc: func(ctx context.Context, gs *session.GameSession) error {
						return nil
					},
				}
			},
			request: EnableCooperativeRequest{
				ChatID:     12345,
				MaxPlayers: 2,
			},
			expectedError: false,
			expectedSaved: true,
		},
		{
			name: "success - enable cooperative mode with 3 players",
			setupMock: func() *mockSessionRepo {
				gs := &session.GameSession{
					State: session.StateActive,
					Players: []player.Player{
						{ID: 1, TgUserID: 111},
						{ID: 2, TgUserID: 222},
					},
				}
				return &mockSessionRepo{
					getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
						return gs, nil
					},
					saveFunc: func(ctx context.Context, gs *session.GameSession) error {
						return nil
					},
				}
			},
			request: EnableCooperativeRequest{
				ChatID:     12345,
				MaxPlayers: 3,
			},
			expectedError: false,
			expectedSaved: true,
		},
		{
			name: "error - too many players for max limit",
			setupMock: func() *mockSessionRepo {
				gs := &session.GameSession{
					State: session.StateActive,
					Players: []player.Player{
						{ID: 1, TgUserID: 111},
						{ID: 2, TgUserID: 222},
						{ID: 3, TgUserID: 333},
						{ID: 4, TgUserID: 444},
					},
				}
				return &mockSessionRepo{
					getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
						return gs, nil
					},
				}
			},
			request: EnableCooperativeRequest{
				ChatID:     12345,
				MaxPlayers: 3,
			},
			expectedError: true,
			expectedSaved: false,
		},
		{
			name: "error - session not found",
			setupMock: func() *mockSessionRepo {
				return &mockSessionRepo{
					getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
						return nil, errors.New("session not found")
					},
				}
			},
			request: EnableCooperativeRequest{
				ChatID:     12345,
				MaxPlayers: 2,
			},
			expectedError: true,
			expectedSaved: false,
		},
		{
			name: "error - session not active",
			setupMock: func() *mockSessionRepo {
				gs := &session.GameSession{
					State: session.StateDone,
				}
				return &mockSessionRepo{
					getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
						return gs, nil
					},
				}
			},
			request: EnableCooperativeRequest{
				ChatID:     12345,
				MaxPlayers: 2,
			},
			expectedError: true,
			expectedSaved: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSessionRepo := tt.setupMock()
			mockPlayerRepo := &mockPlayerRepo{}
			uc := NewManageCooperativeUseCase(mockSessionRepo, mockPlayerRepo)

			saved := false
			if mockSessionRepo.saveFunc != nil {
				originalSave := mockSessionRepo.saveFunc
				mockSessionRepo.saveFunc = func(ctx context.Context, gs *session.GameSession) error {
					saved = true
					return originalSave(ctx, gs)
				}
			}

			err := uc.EnableCooperativeMode(context.Background(), tt.request)

			if tt.expectedError && err == nil {
				t.Error("expected error, but got none")
			}
			if !tt.expectedError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if saved != tt.expectedSaved {
				t.Errorf("expected saved=%v, got saved=%v", tt.expectedSaved, saved)
			}

			if !tt.expectedError && saved {
				gs, _ := mockSessionRepo.GetByChatID(context.Background(), tt.request.ChatID)
				if !gs.IsCooperative {
					t.Error("expected cooperative mode to be enabled")
				}
				if gs.MaxPlayers != tt.request.MaxPlayers {
					t.Errorf("expected MaxPlayers=%d, got %d", tt.request.MaxPlayers, gs.MaxPlayers)
				}
			}
		})
	}
}

func TestManageCooperativeUseCase_JoinCooperativeSession(t *testing.T) {
	tests := []struct {
		name          string
		setupMock     func() (*mockSessionRepo, *mockPlayerRepo)
		request       JoinCooperativeSessionRequest
		expectedError bool
		expectedSaved bool
	}{
		{
			name: "success - join cooperative session",
			setupMock: func() (*mockSessionRepo, *mockPlayerRepo) {
				gs := &session.GameSession{
					State:        session.StateActive,
					IsCooperative: true,
					MaxPlayers:    2,
					Players: []player.Player{
						{ID: 1, TgUserID: 111, Name: "Player1"},
					},
				}
				player2 := &player.Player{
					ID:       2,
					TgUserID: 222,
					Name:     "Player2",
				}
				return &mockSessionRepo{
					getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
						return gs, nil
					},
					saveFunc: func(ctx context.Context, gs *session.GameSession) error {
						return nil
					},
				}, &mockPlayerRepo{
					getByTgUserIDFunc: func(ctx context.Context, tgUserID int64) (*player.Player, error) {
						if tgUserID == 222 {
							return player2, nil
						}
						return nil, nil
					},
				}
			},
			request: JoinCooperativeSessionRequest{
				ChatID:   12345,
				TgUserID: 222,
			},
			expectedError: false,
			expectedSaved: true,
		},
		{
			name: "error - session not cooperative",
			setupMock: func() (*mockSessionRepo, *mockPlayerRepo) {
				gs := &session.GameSession{
					State:        session.StateActive,
					IsCooperative: false,
					MaxPlayers:    1,
				}
				player2 := &player.Player{
					ID:       2,
					TgUserID: 222,
					Name:     "Player2",
				}
				return &mockSessionRepo{
					getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
						return gs, nil
					},
				}, &mockPlayerRepo{
					getByTgUserIDFunc: func(ctx context.Context, tgUserID int64) (*player.Player, error) {
						return player2, nil
					},
				}
			},
			request: JoinCooperativeSessionRequest{
				ChatID:   12345,
				TgUserID: 222,
			},
			expectedError: true,
			expectedSaved: false,
		},
		{
			name: "error - session is full",
			setupMock: func() (*mockSessionRepo, *mockPlayerRepo) {
				gs := &session.GameSession{
					State:        session.StateActive,
					IsCooperative: true,
					MaxPlayers:    2,
					Players: []player.Player{
						{ID: 1, TgUserID: 111, Name: "Player1"},
						{ID: 2, TgUserID: 222, Name: "Player2"},
					},
				}
				player2 := &player.Player{
					ID:       3,
					TgUserID: 333,
					Name:     "Player3",
				}
				return &mockSessionRepo{
					getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
						return gs, nil
					},
				}, &mockPlayerRepo{
					getByTgUserIDFunc: func(ctx context.Context, tgUserID int64) (*player.Player, error) {
						return player2, nil
					},
				}
			},
			request: JoinCooperativeSessionRequest{
				ChatID:   12345,
				TgUserID: 333,
			},
			expectedError: true,
			expectedSaved: false,
		},
		{
			name: "error - player already in session",
			setupMock: func() (*mockSessionRepo, *mockPlayerRepo) {
				gs := &session.GameSession{
					State:        session.StateActive,
					IsCooperative: true,
					MaxPlayers:    2,
					Players: []player.Player{
						{ID: 1, TgUserID: 111, Name: "Player1"},
					},
				}
				player2 := &player.Player{
					ID:       1,
					TgUserID: 111,
					Name:     "Player1",
				}
				return &mockSessionRepo{
					getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
						return gs, nil
					},
				}, &mockPlayerRepo{
					getByTgUserIDFunc: func(ctx context.Context, tgUserID int64) (*player.Player, error) {
						return player2, nil
					},
				}
			},
			request: JoinCooperativeSessionRequest{
				ChatID:   12345,
				TgUserID: 111,
			},
			expectedError: true,
			expectedSaved: false,
		},
		{
			name: "error - player not found",
			setupMock: func() (*mockSessionRepo, *mockPlayerRepo) {
				gs := &session.GameSession{
					State:        session.StateActive,
					IsCooperative: true,
					MaxPlayers:    2,
					Players: []player.Player{
						{ID: 1, TgUserID: 111, Name: "Player1"},
					},
				}
				return &mockSessionRepo{
					getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
						return gs, nil
					},
				}, &mockPlayerRepo{
					getByTgUserIDFunc: func(ctx context.Context, tgUserID int64) (*player.Player, error) {
						return nil, errors.New("player not found")
					},
				}
			},
			request: JoinCooperativeSessionRequest{
				ChatID:   12345,
				TgUserID: 222,
			},
			expectedError: true,
			expectedSaved: false,
		},
		{
			name: "error - player not created",
			setupMock: func() (*mockSessionRepo, *mockPlayerRepo) {
				gs := &session.GameSession{
					State:        session.StateActive,
					IsCooperative: true,
					MaxPlayers:    2,
					Players: []player.Player{
						{ID: 1, TgUserID: 111, Name: "Player1"},
					},
				}
				return &mockSessionRepo{
					getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
						return gs, nil
					},
				}, &mockPlayerRepo{
					getByTgUserIDFunc: func(ctx context.Context, tgUserID int64) (*player.Player, error) {
						return nil, nil
					},
				}
			},
			request: JoinCooperativeSessionRequest{
				ChatID:   12345,
				TgUserID: 222,
			},
			expectedError: true,
			expectedSaved: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSessionRepo, mockPlayerRepo := tt.setupMock()
			uc := NewManageCooperativeUseCase(mockSessionRepo, mockPlayerRepo)

			saved := false
			if mockSessionRepo.saveFunc != nil {
				originalSave := mockSessionRepo.saveFunc
				mockSessionRepo.saveFunc = func(ctx context.Context, gs *session.GameSession) error {
					saved = true
					return originalSave(ctx, gs)
				}
			}

			err := uc.JoinCooperativeSession(context.Background(), tt.request)

			if tt.expectedError && err == nil {
				t.Error("expected error, but got none")
			}
			if !tt.expectedError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if saved != tt.expectedSaved {
				t.Errorf("expected saved=%v, got saved=%v", tt.expectedSaved, saved)
			}
		})
	}
}

func TestManageCooperativeUseCase_GetCooperativeStatus(t *testing.T) {
	tests := []struct {
		name          string
		setupMock     func() (*mockSessionRepo, *mockPlayerRepo)
		chatID        int64
		expectedError bool
		expectedCoop  bool
		expectedMax   int
		expectedCurr  int
	}{
		{
			name: "success - cooperative session with players",
			setupMock: func() (*mockSessionRepo, *mockPlayerRepo) {
				player1ID := uint(1)
				gs := &session.GameSession{
					State:        session.StateActive,
					IsCooperative: true,
					MaxPlayers:    2,
					ActivePlayerID: &player1ID,
					Players: []player.Player{
						{ID: 1, TgUserID: 111, Name: "Player1"},
						{ID: 2, TgUserID: 222, Name: "Player2"},
					},
				}
				return &mockSessionRepo{
					getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
						return gs, nil
					},
				}, &mockPlayerRepo{}
			},
			chatID:        12345,
			expectedError: false,
			expectedCoop:  true,
			expectedMax:   2,
			expectedCurr:  2,
		},
		{
			name: "success - single player session",
			setupMock: func() (*mockSessionRepo, *mockPlayerRepo) {
				gs := &session.GameSession{
					State:        session.StateActive,
					IsCooperative: false,
					MaxPlayers:    1,
					Players: []player.Player{
						{ID: 1, TgUserID: 111, Name: "Player1"},
					},
				}
				return &mockSessionRepo{
					getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
						return gs, nil
					},
				}, &mockPlayerRepo{}
			},
			chatID:        12345,
			expectedError: false,
			expectedCoop:  false,
			expectedMax:   1,
			expectedCurr:  1,
		},
		{
			name: "error - session not found",
			setupMock: func() (*mockSessionRepo, *mockPlayerRepo) {
				return &mockSessionRepo{
					getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
						return nil, errors.New("session not found")
					},
				}, &mockPlayerRepo{}
			},
			chatID:        12345,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSessionRepo, mockPlayerRepo := tt.setupMock()
			uc := NewManageCooperativeUseCase(mockSessionRepo, mockPlayerRepo)

			response, err := uc.GetCooperativeStatus(context.Background(), tt.chatID)

			if tt.expectedError && err == nil {
				t.Error("expected error, but got none")
			}
			if !tt.expectedError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if !tt.expectedError {
				if response == nil {
					t.Error("expected response, got nil")
					return
				}
				if response.IsCooperative != tt.expectedCoop {
					t.Errorf("expected IsCooperative=%v, got %v", tt.expectedCoop, response.IsCooperative)
				}
				if response.MaxPlayers != tt.expectedMax {
					t.Errorf("expected MaxPlayers=%d, got %d", tt.expectedMax, response.MaxPlayers)
				}
				if response.CurrentPlayers != tt.expectedCurr {
					t.Errorf("expected CurrentPlayers=%d, got %d", tt.expectedCurr, response.CurrentPlayers)
				}
			}
		})
	}
}