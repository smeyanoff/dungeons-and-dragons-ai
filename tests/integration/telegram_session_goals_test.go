package integration

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"dungeons-and-dragons-ai/internal/game/application/campaign"
	characterapp "dungeons-and-dragons-ai/internal/game/application/character"
	sessionapp "dungeons-and-dragons-ai/internal/game/application/session"
)

// TestTelegramGameplay_BotSimulation_SessionGoals tests session goals functionality
// including goal generation, progress tracking, and goal completion
func TestTelegramGameplay_BotSimulation_SessionGoals(t *testing.T) {
	cfg := setupInfraOnlyIntegrationTest(t)
	if cfg == nil {
		return
	}
	defer cleanupTest(t, &testConfig{db: cfg.db, chatID: cfg.chatID, tgUserID: cfg.tgUserID})

	ctx := cfg.ctx
	chatID := cfg.chatID

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

	// Session goals use case
	manageSessionGoalsUC := sessionapp.NewManageSessionGoalsUseCase(sessionRepo)

	// Step 1: Start new game (simulate via use case)
	t.Log("Step 1: Starting new game")
	_, err := initCampaignUC.Execute(ctx, "fantasy")
	if err != nil {
		problems = append(problems, fmt.Sprintf("Failed to start new game: %v", err))
	}

	// Check if session was created and goals were generated
	session, err := sessionRepo.GetByChatID(ctx, chatID)
	if err != nil {
		problems = append(problems, fmt.Sprintf("Failed to get session: %v", err))
	} else {
		if len(session.SessionGoals) == 0 {
			problems = append(problems, "Session goals were not generated")
		} else {
			t.Logf("Generated %d session goals", len(session.SessionGoals))
			for _, goal := range session.SessionGoals {
				t.Logf("Goal: %s - %s (Target: %d)", goal.Type, goal.Description, goal.TargetValue)
			}
		}
	}

	// Step 2: Create character
	t.Log("Step 2: Creating character")
	_, err = createCharacterUC.Execute(ctx, characterapp.CreateCharacterRequest{
		ChatID: chatID,
		Name:   "TestHero",
	})
	if err != nil {
		problems = append(problems, fmt.Sprintf("Failed to create character: %v", err))
	}

	// Step 3: Test getting session goals directly
	t.Log("Step 3: Testing session goals retrieval")
	_, err = manageSessionGoalsUC.GetSessionGoals(ctx, chatID)
	if err != nil {
		problems = append(problems, fmt.Sprintf("Failed to get session goals: %v", err))
	}

	// Step 4: Simulate actions that update goal progress
	// Manually update exploration goal progress (simulate 3 explorations)
	t.Log("Step 4: Simulating exploration actions")
	for i := 0; i < 3; i++ {
		err = manageSessionGoalsUC.UpdateGoalProgress(ctx, sessionapp.UpdateGoalProgressRequest{
			ChatID:      chatID,
			GoalType:    "exploration",
			IncrementBy: 1,
		})
		if err != nil {
			problems = append(problems, fmt.Sprintf("Failed to update exploration progress %d: %v", i+1, err))
		}
	}

	// Step 5: Check if exploration goal is completed
	t.Log("Step 5: Checking goal completion")
	goalsResponse, err := manageSessionGoalsUC.GetSessionGoals(ctx, chatID)
	if err != nil {
		problems = append(problems, fmt.Sprintf("Failed to get goals after updates: %v", err))
	} else {
		for _, goal := range goalsResponse.Goals {
			t.Logf("Goal status: %s - %s (%d/%d)", goal.Type, goal.Status, goal.Current, goal.Target)
			if goal.Type == "exploration" && goal.Status != "completed" {
				problems = append(problems, "Exploration goal should be completed after 3 explorations")
			}
		}
	}

	// Step 6: Test goal expiration (simulate time passing)
	t.Log("Step 6: Testing goal expiration")
	// Get current session and manually set expired time for one goal
	session, err = sessionRepo.GetByChatID(ctx, chatID)
	if err != nil {
		problems = append(problems, fmt.Sprintf("Failed to get session for expiration test: %v", err))
	} else {
		// Find a goal with time limit and set it to expired
		for i := range session.SessionGoals {
			goal := &session.SessionGoals[i]
			if goal.TimeLimit != nil {
				// Set to expired (1 hour ago)
				expiredTime := time.Now().Add(-time.Hour)
				goal.TimeLimit = &expiredTime
				break
			}
		}
		err = sessionRepo.Save(ctx, session)
		if err != nil {
			problems = append(problems, fmt.Sprintf("Failed to save session with expired goal: %v", err))
		}

		// Check expired goals
		err = manageSessionGoalsUC.CheckSessionExpiredGoals(ctx, chatID)
		if err != nil {
			problems = append(problems, fmt.Sprintf("Failed to check expired goals: %v", err))
		}

		// Verify goal was marked as expired
		goalsResponse, err = manageSessionGoalsUC.GetSessionGoals(ctx, chatID)
		if err != nil {
			problems = append(problems, fmt.Sprintf("Failed to get goals after expiration check: %v", err))
		} else {
			foundExpired := false
			for _, goal := range goalsResponse.Goals {
				if goal.Status == "expired" {
					foundExpired = true
					t.Logf("Found expired goal: %s", goal.Description)
					break
				}
			}
			if !foundExpired {
				problems = append(problems, "Goal expiration check did not work - no goals marked as expired")
			}
		}
	}

	// Check for messages sent to user
	fakeAPI.mu.Lock()
	messages := fakeAPI.calls
	fakeAPI.mu.Unlock()
	t.Logf("Bot sent %d messages during test", len(messages))

	// Check if goals were mentioned in messages
	goalsMentioned := false
	for _, msg := range messages {
		if strings.Contains(msg.Text, "цели") || strings.Contains(msg.Text, "цель") ||
		   strings.Contains(msg.Text, "goals") || strings.Contains(msg.Text, "goal") {
			goalsMentioned = true
			t.Logf("Goals mentioned in message: %s", msg.Text)
			break
		}
	}

	if !goalsMentioned && len(problems) == 0 {
		problems = append(problems, "Session goals were not communicated to user")
	}

	// Report results
	if len(problems) > 0 {
		t.Errorf("Session goals test failed with %d problems:", len(problems))
		for i, problem := range problems {
			t.Errorf("  %d. %s", i+1, problem)
		}
	} else {
		t.Log("Session goals test PASSED")
	}
}