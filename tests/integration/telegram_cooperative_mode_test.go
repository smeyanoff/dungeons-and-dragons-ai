package integration

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"dungeons-and-dragons-ai/internal/game/application/campaign"
	characterapp "dungeons-and-dragons-ai/internal/game/application/character"
	sessionapp "dungeons-and-dragons-ai/internal/game/application/session"
)

// TestTelegramGameplay_BotSimulation_CooperativeMode tests cooperative gameplay functionality
// including enabling cooperative mode, joining players, and turn management
func TestTelegramGameplay_BotSimulation_CooperativeMode(t *testing.T) {
	cfg := setupInfraOnlyIntegrationTest(t)
	if cfg == nil {
		return
	}
	defer cleanupTest(t, &testConfig{db: cfg.db, chatID: cfg.chatID, tgUserID: cfg.tgUserID})

	ctx := cfg.ctx
	chatID := cfg.chatID
	tgUserID1 := cfg.tgUserID
	tgUserID2 := cfg.tgUserID + 1 // Second player
	tgUserID3 := cfg.tgUserID + 2 // Third player

	var problems []string

	// Fake Telegram API server
	fakeAPI := newFakeTelegramAPI()
	srv := httptest.NewServer(fakeAPI.handler(t))
	defer srv.Close()

	// Repos
	sessionRepo := cfg.sessionRepo
	playerRepo := cfg.playerRepo

	// Use-cases (no real LLM / no real embeddings)
	initCampaignUC := campaign.NewInitCampaignUseCase(&initCampaignStubLLM{}, cfg.worldRepo)
	createCharacterUC := characterapp.NewCreateCharacterUseCase(sessionRepo, playerRepo)

	// Cooperative use case
	manageCooperativeUC := sessionapp.NewManageCooperativeUseCase(sessionRepo, playerRepo)

	// Step 1: Start new game
	t.Log("Step 1: Starting new game")
	_, err := initCampaignUC.Execute(ctx, "fantasy")
	if err != nil {
		problems = append(problems, fmt.Sprintf("Failed to start new game: %v", err))
	}

	// Step 2: Create character for first player
	t.Log("Step 2: Creating character for first player")
	_, err = createCharacterUC.Execute(ctx, characterapp.CreateCharacterRequest{
		ChatID: chatID,
		Name:   "Player1",
	})
	if err != nil {
		problems = append(problems, fmt.Sprintf("Failed to create character for player 1: %v", err))
	}

	// Step 3: Enable cooperative mode
	t.Log("Step 3: Enabling cooperative mode for 3 players")
	err = manageCooperativeUC.EnableCooperativeMode(ctx, sessionapp.EnableCooperativeRequest{
		ChatID:     chatID,
		MaxPlayers: 3,
	})
	if err != nil {
		problems = append(problems, fmt.Sprintf("Failed to enable cooperative mode: %v", err))
	}

	// Verify cooperative mode was enabled
	session, err := sessionRepo.GetByChatID(ctx, chatID)
	if err != nil {
		problems = append(problems, fmt.Sprintf("Failed to get session after enabling cooperative mode: %v", err))
	} else {
		if !session.IsCooperative {
			problems = append(problems, "Cooperative mode was not enabled")
		}
		if session.MaxPlayers != 3 {
			problems = append(problems, fmt.Sprintf("Expected MaxPlayers=3, got %d", session.MaxPlayers))
		}
		if len(session.Players) != 1 {
			problems = append(problems, fmt.Sprintf("Expected 1 player initially, got %d", len(session.Players)))
		}
	}

	// Step 4: Create character for second player
	t.Log("Step 4: Creating character for second player")
	_, err = createCharacterUC.Execute(ctx, characterapp.CreateCharacterRequest{
		ChatID: chatID,
		Name:   "Player2",
	})
	if err != nil {
		problems = append(problems, fmt.Sprintf("Failed to create character for player 2: %v", err))
	}

	// Step 5: Join cooperative session with second player
	t.Log("Step 5: Joining cooperative session with second player")
	err = manageCooperativeUC.JoinCooperativeSession(ctx, sessionapp.JoinCooperativeSessionRequest{
		ChatID:   chatID,
		TgUserID: tgUserID2,
	})
	if err != nil {
		problems = append(problems, fmt.Sprintf("Failed to join cooperative session with player 2: %v", err))
	}

	// Step 6: Create character for third player
	t.Log("Step 6: Creating character for third player")
	_, err = createCharacterUC.Execute(ctx, characterapp.CreateCharacterRequest{
		ChatID: chatID,
		Name:   "Player3",
	})
	if err != nil {
		problems = append(problems, fmt.Sprintf("Failed to create character for player 3: %v", err))
	}

	// Step 7: Join cooperative session with third player
	t.Log("Step 7: Joining cooperative session with third player")
	err = manageCooperativeUC.JoinCooperativeSession(ctx, sessionapp.JoinCooperativeSessionRequest{
		ChatID:   chatID,
		TgUserID: tgUserID3,
	})
	if err != nil {
		problems = append(problems, fmt.Sprintf("Failed to join cooperative session with player 3: %v", err))
	}

	// Step 8: Verify all players joined
	t.Log("Step 8: Verifying cooperative session status")
	status, err := manageCooperativeUC.GetCooperativeStatus(ctx, chatID)
	if err != nil {
		problems = append(problems, fmt.Sprintf("Failed to get cooperative status: %v", err))
	} else {
		if !status.IsCooperative {
			problems = append(problems, "Session should be cooperative")
		}
		if status.MaxPlayers != 3 {
			problems = append(problems, fmt.Sprintf("Expected MaxPlayers=3, got %d", status.MaxPlayers))
		}
		if status.CurrentPlayers != 3 {
			problems = append(problems, fmt.Sprintf("Expected CurrentPlayers=3, got %d", status.CurrentPlayers))
		}
		if len(status.Players) != 3 {
			problems = append(problems, fmt.Sprintf("Expected 3 players in status, got %d", len(status.Players)))
		}
		if status.ActivePlayer == nil {
			problems = append(problems, "Active player should be set")
		}

		t.Logf("Cooperative session status: %d/%d players, active player: %s",
			status.CurrentPlayers, status.MaxPlayers,
			status.ActivePlayer.Name)
	}

	// Step 9: Test turn management
	t.Log("Step 9: Testing turn management")
	session, err = sessionRepo.GetByChatID(ctx, chatID)
	if err != nil {
		problems = append(problems, fmt.Sprintf("Failed to get session for turn test: %v", err))
	} else {
		// Check initial turn order
		if len(session.PlayerTurnOrder) != 3 {
			problems = append(problems, fmt.Sprintf("Expected 3 players in turn order, got %d", len(session.PlayerTurnOrder)))
		}

		// Test turn progression
		initialActive := session.GetActivePlayer()
		if initialActive == nil {
			problems = append(problems, "No initial active player")
		} else {
			t.Logf("Initial active player: %s (ID: %d)", initialActive.Name, initialActive.ID)

			// Next turn
			session.NextPlayerTurn()
			nextActive := session.GetActivePlayer()
			if nextActive == nil {
				problems = append(problems, "No active player after NextPlayerTurn")
			} else {
				t.Logf("Next active player: %s (ID: %d)", nextActive.Name, nextActive.ID)

				// Next turn again
				session.NextPlayerTurn()
				thirdActive := session.GetActivePlayer()
				if thirdActive == nil {
					problems = append(problems, "No active player after second NextPlayerTurn")
				} else {
					t.Logf("Third active player: %s (ID: %d)", thirdActive.Name, thirdActive.ID)

					// Check if turn order is correct
					if initialActive.ID == nextActive.ID {
						problems = append(problems, "Active player did not change after NextPlayerTurn")
					}
				}
			}
		}

		// Save session to persist turn changes
		err = sessionRepo.Save(ctx, session)
		if err != nil {
			problems = append(problems, fmt.Sprintf("Failed to save session after turn changes: %v", err))
		}
	}

	// Step 10: Test player turn checking
	t.Log("Step 10: Testing IsPlayerTurn functionality")
	session, err = sessionRepo.GetByChatID(ctx, chatID)
	if err != nil {
		problems = append(problems, fmt.Sprintf("Failed to get session for IsPlayerTurn test: %v", err))
	} else {
		activePlayer := session.GetActivePlayer()
		if activePlayer != nil {
			isActiveTurn := session.IsPlayerTurn(activePlayer.TgUserID)
			if !isActiveTurn {
				problems = append(problems, "IsPlayerTurn returned false for active player")
			}

			// Test non-active player
			nonActiveTgID := tgUserID1
			if activePlayer.TgUserID == tgUserID1 {
				nonActiveTgID = tgUserID2
			}
			isNonActiveTurn := session.IsPlayerTurn(nonActiveTgID)
			if isNonActiveTurn {
				problems = append(problems, "IsPlayerTurn returned true for non-active player")
			}
		}
	}

	// Check for messages sent to user
	fakeAPI.mu.Lock()
	messages := fakeAPI.calls
	fakeAPI.mu.Unlock()
	t.Logf("Bot sent %d messages during cooperative test", len(messages))

	// Report results
	if len(problems) > 0 {
		t.Errorf("Cooperative mode test failed with %d problems:", len(problems))
		for i, problem := range problems {
			t.Errorf("  %d. %s", i+1, problem)
		}
	} else {
		t.Log("Cooperative mode test PASSED")
	}
}