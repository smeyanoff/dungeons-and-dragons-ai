package session

import (
	"testing"

	"dungeons-and-dragons-ai/internal/game/domain/player"
)

func TestGameSession_EnableCooperativeMode(t *testing.T) {
	tests := []struct {
		name       string
		maxPlayers int
		expected   int
	}{
		{
			name:       "enable with 2 players",
			maxPlayers: 2,
			expected:   2,
		},
		{
			name:       "enable with 3 players",
			maxPlayers: 3,
			expected:   3,
		},
		{
			name:       "clamp max players to 3",
			maxPlayers: 5,
			expected:   3,
		},
		{
			name:       "clamp min players to 1",
			maxPlayers: 0,
			expected:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &GameSession{}

			s.EnableCooperativeMode(tt.maxPlayers)

			if !s.IsCooperative {
				t.Error("EnableCooperativeMode() should set IsCooperative to true")
			}

			if s.MaxPlayers != tt.expected {
				t.Errorf("EnableCooperativeMode() MaxPlayers = %d, want %d", s.MaxPlayers, tt.expected)
			}

			if len(s.PlayerTurnOrder) != 0 {
				t.Errorf("EnableCooperativeMode() PlayerTurnOrder length = %d, want 0", len(s.PlayerTurnOrder))
			}
		})
	}
}

func TestGameSession_DisableCooperativeMode(t *testing.T) {
	s := &GameSession{
		IsCooperative:  true,
		MaxPlayers:     3,
		ActivePlayerID: func() *uint { id := uint(1); return &id }(),
		PlayerTurnOrder: []uint{1, 2, 3},
	}

	s.DisableCooperativeMode()

	if s.IsCooperative {
		t.Error("DisableCooperativeMode() should set IsCooperative to false")
	}

	if s.MaxPlayers != 1 {
		t.Errorf("DisableCooperativeMode() MaxPlayers = %d, want 1", s.MaxPlayers)
	}

	if s.ActivePlayerID != nil {
		t.Error("DisableCooperativeMode() should set ActivePlayerID to nil")
	}

	if s.PlayerTurnOrder != nil {
		t.Error("DisableCooperativeMode() should set PlayerTurnOrder to nil")
	}
}

func TestGameSession_AddPlayerToSession(t *testing.T) {
	tests := []struct {
		name          string
		setup         func() *GameSession
		player        *player.Player
		expectedError bool
		expectedCount int
	}{
		{
			name: "success - add player to cooperative session",
			setup: func() *GameSession {
				s := &GameSession{
					IsCooperative: true,
					MaxPlayers:    2,
					Players: []player.Player{
						{ID: 1, TgUserID: 111, Name: "Player1"},
					},
				}
				return s
			},
			player: &player.Player{
				ID:       2,
				TgUserID: 222,
				Name:     "Player2",
			},
			expectedError: false,
			expectedCount: 2,
		},
		{
			name: "success - first player becomes active",
			setup: func() *GameSession {
				s := &GameSession{
					IsCooperative: true,
					MaxPlayers:    2,
					Players:       []player.Player{},
				}
				return s
			},
			player: &player.Player{
				ID:       1,
				TgUserID: 111,
				Name:     "Player1",
			},
			expectedError: false,
			expectedCount: 1,
		},
		{
			name: "error - session not cooperative",
			setup: func() *GameSession {
				s := &GameSession{
					IsCooperative: false,
					MaxPlayers:    1,
					Players: []player.Player{
						{ID: 1, TgUserID: 111, Name: "Player1"},
					},
				}
				return s
			},
			player: &player.Player{
				ID:       2,
				TgUserID: 222,
				Name:     "Player2",
			},
			expectedError: true,
			expectedCount: 1,
		},
		{
			name: "error - max players reached",
			setup: func() *GameSession {
				s := &GameSession{
					IsCooperative: true,
					MaxPlayers:    2,
					Players: []player.Player{
						{ID: 1, TgUserID: 111, Name: "Player1"},
						{ID: 2, TgUserID: 222, Name: "Player2"},
					},
				}
				return s
			},
			player: &player.Player{
				ID:       3,
				TgUserID: 333,
				Name:     "Player3",
			},
			expectedError: true,
			expectedCount: 2,
		},
		{
			name: "error - player already in session",
			setup: func() *GameSession {
				s := &GameSession{
					IsCooperative: true,
					MaxPlayers:    2,
					Players: []player.Player{
						{ID: 1, TgUserID: 111, Name: "Player1"},
					},
				}
				return s
			},
			player: &player.Player{
				ID:       1,
				TgUserID: 111,
				Name:     "Player1",
			},
			expectedError: true,
			expectedCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.setup()

			err := s.AddPlayerToSession(tt.player)

			if tt.expectedError && err == nil {
				t.Error("expected error, but got none")
			}
			if !tt.expectedError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if len(s.Players) != tt.expectedCount {
				t.Errorf("expected %d players, got %d", tt.expectedCount, len(s.Players))
			}

			if !tt.expectedError && tt.expectedCount > 0 {
				// Check if player was added
				found := false
				for _, p := range s.Players {
					if p.ID == tt.player.ID {
						found = true
						break
					}
				}
				if !found {
					t.Error("player was not added to session")
				}

				// Check if player is in turn order
				found = false
				for _, id := range s.PlayerTurnOrder {
					if id == tt.player.ID {
						found = true
						break
					}
				}
				if !found {
					t.Error("player was not added to turn order")
				}
			}
		})
	}
}

func TestGameSession_GetActivePlayer(t *testing.T) {
	tests := []struct {
		name           string
		setup          func() *GameSession
		expectedTgID   int64
		expectedNil    bool
	}{
		{
			name: "get active player by ID",
			setup: func() *GameSession {
				activeID := uint(2)
				s := &GameSession{
					ActivePlayerID: &activeID,
					Players: []player.Player{
						{ID: 1, TgUserID: 111, Name: "Player1"},
						{ID: 2, TgUserID: 222, Name: "Player2"},
					},
				}
				return s
			},
			expectedTgID: 222,
			expectedNil:  false,
		},
		{
			name: "fallback to first player when active ID is nil",
			setup: func() *GameSession {
				s := &GameSession{
					ActivePlayerID: nil,
					Players: []player.Player{
						{ID: 1, TgUserID: 111, Name: "Player1"},
						{ID: 2, TgUserID: 222, Name: "Player2"},
					},
				}
				return s
			},
			expectedTgID: 111,
			expectedNil:  false,
		},
		{
			name: "return nil when no players",
			setup: func() *GameSession {
				s := &GameSession{
					ActivePlayerID: nil,
					Players:        []player.Player{},
				}
				return s
			},
			expectedTgID: 0,
			expectedNil:  true,
		},
		{
			name: "return nil when active player not found",
			setup: func() *GameSession {
				activeID := uint(999)
				s := &GameSession{
					ActivePlayerID: &activeID,
					Players: []player.Player{
						{ID: 1, TgUserID: 111, Name: "Player1"},
					},
				}
				return s
			},
			expectedTgID: 0,
			expectedNil:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.setup()

			activePlayer := s.GetActivePlayer()

			if tt.expectedNil {
				if activePlayer != nil {
					t.Error("expected nil active player")
				}
				return
			}

			if activePlayer == nil {
				t.Error("expected active player, got nil")
				return
			}

			if activePlayer.TgUserID != tt.expectedTgID {
				t.Errorf("expected active player TgUserID %d, got %d", tt.expectedTgID, activePlayer.TgUserID)
			}
		})
	}
}

func TestGameSession_NextPlayerTurn(t *testing.T) {
	tests := []struct {
		name         string
		setup        func() *GameSession
		expectedTgID int64
	}{
		{
			name: "next turn - move to second player",
			setup: func() *GameSession {
				activeID := uint(1)
				s := &GameSession{
					IsCooperative:  true,
					ActivePlayerID: &activeID,
					PlayerTurnOrder: []uint{1, 2, 3},
					Players: []player.Player{
						{ID: 1, TgUserID: 111, Name: "Player1"},
						{ID: 2, TgUserID: 222, Name: "Player2"},
						{ID: 3, TgUserID: 333, Name: "Player3"},
					},
				}
				return s
			},
			expectedTgID: 222,
		},
		{
			name: "next turn - wrap around to first player",
			setup: func() *GameSession {
				activeID := uint(3)
				s := &GameSession{
					IsCooperative:  true,
					ActivePlayerID: &activeID,
					PlayerTurnOrder: []uint{1, 2, 3},
					Players: []player.Player{
						{ID: 1, TgUserID: 111, Name: "Player1"},
						{ID: 2, TgUserID: 222, Name: "Player2"},
						{ID: 3, TgUserID: 333, Name: "Player3"},
					},
				}
				return s
			},
			expectedTgID: 111,
		},
		{
			name: "next turn - set to first when active is nil",
			setup: func() *GameSession {
				s := &GameSession{
					IsCooperative:  true,
					ActivePlayerID: nil,
					PlayerTurnOrder: []uint{1, 2, 3},
					Players: []player.Player{
						{ID: 1, TgUserID: 111, Name: "Player1"},
						{ID: 2, TgUserID: 222, Name: "Player2"},
						{ID: 3, TgUserID: 333, Name: "Player3"},
					},
				}
				return s
			},
			expectedTgID: 111,
		},
		{
			name: "next turn - reset when active not in turn order",
			setup: func() *GameSession {
				activeID := uint(999)
				s := &GameSession{
					IsCooperative:  true,
					ActivePlayerID: &activeID,
					PlayerTurnOrder: []uint{1, 2, 3},
					Players: []player.Player{
						{ID: 1, TgUserID: 111, Name: "Player1"},
						{ID: 2, TgUserID: 222, Name: "Player2"},
						{ID: 3, TgUserID: 333, Name: "Player3"},
					},
				}
				return s
			},
			expectedTgID: 111,
		},
		{
			name: "no action - not cooperative",
			setup: func() *GameSession {
				activeID := uint(1)
				s := &GameSession{
					IsCooperative:  false,
					ActivePlayerID: &activeID,
					PlayerTurnOrder: []uint{1, 2, 3},
					Players: []player.Player{
						{ID: 1, TgUserID: 111, Name: "Player1"},
						{ID: 2, TgUserID: 222, Name: "Player2"},
					},
				}
				return s
			},
			expectedTgID: 111,
		},
		{
			name: "no action - only one player",
			setup: func() *GameSession {
				activeID := uint(1)
				s := &GameSession{
					IsCooperative:  true,
					ActivePlayerID: &activeID,
					PlayerTurnOrder: []uint{1},
					Players: []player.Player{
						{ID: 1, TgUserID: 111, Name: "Player1"},
					},
				}
				return s
			},
			expectedTgID: 111,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.setup()

			s.NextPlayerTurn()

			activePlayer := s.GetActivePlayer()
			if activePlayer == nil {
				t.Error("expected active player after NextPlayerTurn")
				return
			}

			if activePlayer.TgUserID != tt.expectedTgID {
				t.Errorf("expected active player TgUserID %d, got %d", tt.expectedTgID, activePlayer.TgUserID)
			}
		})
	}
}

func TestGameSession_IsPlayerTurn(t *testing.T) {
	tests := []struct {
		name         string
		setup        func() *GameSession
		tgUserID     int64
		expectedTurn bool
	}{
		{
			name: "player turn - cooperative mode, active player",
			setup: func() *GameSession {
				activeID := uint(1)
				s := &GameSession{
					IsCooperative:  true,
					ActivePlayerID: &activeID,
					Players: []player.Player{
						{ID: 1, TgUserID: 111, Name: "Player1"},
						{ID: 2, TgUserID: 222, Name: "Player2"},
					},
				}
				return s
			},
			tgUserID:     111,
			expectedTurn: true,
		},
		{
			name: "not player turn - cooperative mode, different player",
			setup: func() *GameSession {
				activeID := uint(1)
				s := &GameSession{
					IsCooperative:  true,
					ActivePlayerID: &activeID,
					Players: []player.Player{
						{ID: 1, TgUserID: 111, Name: "Player1"},
						{ID: 2, TgUserID: 222, Name: "Player2"},
					},
				}
				return s
			},
			tgUserID:     222,
			expectedTurn: false,
		},
		{
			name: "always player turn - single player mode",
			setup: func() *GameSession {
				s := &GameSession{
					IsCooperative: false,
					Players: []player.Player{
						{ID: 1, TgUserID: 111, Name: "Player1"},
					},
				}
				return s
			},
			tgUserID:     111,
			expectedTurn: true,
		},
		{
			name: "not player turn - no active player",
			setup: func() *GameSession {
				s := &GameSession{
					IsCooperative:  true,
					ActivePlayerID: nil,
					Players:        []player.Player{},
				}
				return s
			},
			tgUserID:     111,
			expectedTurn: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.setup()

			isTurn := s.IsPlayerTurn(tt.tgUserID)

			if isTurn != tt.expectedTurn {
				t.Errorf("IsPlayerTurn() = %v, want %v", isTurn, tt.expectedTurn)
			}
		})
	}
}