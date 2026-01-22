package integration

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	abilitycheck "dungeons-and-dragons-ai/internal/game/application/ability_check"
	"dungeons-and-dragons-ai/internal/game/infrastructure/persistence"
	mapapp "dungeons-and-dragons-ai/internal/game/application/worldmap"
	telegrambot "dungeons-and-dragons-ai/internal/telegram"
)

// TestTelegramGameplay_RealLLM_ComprehensiveGameplay
// Комплексный end-to-end тест всех основных механик игры с реальными LLM вызовами
// Симулирует полный пользовательский journey от создания игры до завершения,
// тестируя все фичи: создание мира, персонажа, исследование, бой, инвентарь, квесты,
// ежедневные задания, заклинания, достижения, карту, историю и location events
func TestTelegramGameplay_RealLLM_ComprehensiveGameplay(t *testing.T) {
	cfg := setupTelegramGameplayTest(t)
	defer cleanupTest(t, cfg.testConfig)

	ctx := cfg.ctx
	chatID := cfg.chatID
	tgUserID := cfg.tgUserID

	var problems []string
	var llmFeedback []string

	// Fake Telegram API server
	fakeAPI := newFakeTelegramAPI()
	srv := httptest.NewServer(fakeAPI.handler(t))
	defer srv.Close()
	apiEndpointFmt := strings.TrimRight(srv.URL, "/") + "/bot%s/%s"

	// Bot repositories and use cases
	eventRepo := persistence.NewGameEventRepository(cfg.db)
	combatRepo := persistence.NewCombatRepository(cfg.db)
	feedbackRepo := persistence.NewFeedbackRepository(cfg.db)
	worldEventRepo := persistence.NewWorldEventRepository(cfg.db)
	moveToLocationUC := mapapp.NewMoveToLocationUseCase(cfg.sessionRepo, worldEventRepo, nil, nil)
	performAbilityCheckUC := abilitycheck.NewPerformAbilityCheckUseCase(cfg.sessionRepo, eventRepo, nil)

	// Create bot with real LLM for gameplay
	bot, err := telegrambot.NewBotWithAPIEndpoint(
		"TEST_TOKEN",
		apiEndpointFmt,
		cfg.initCampaignUC,    // Real LLM for campaign creation
		cfg.handleActionUC,    // Real LLM for player actions
		cfg.createCharacterUC, // Real LLM for character creation
		cfg.getHistoryUC,
		cfg.getInventoryUC,
		cfg.addItemUC,
		cfg.handleCombatUC,
		cfg.rollDiceUC,
		cfg.getQuestsUC,
		cfg.getDailyQuestsUC,
		cfg.checkDailyProgressUC,
		cfg.getMapUC,
		moveToLocationUC,
		cfg.getAchievementsUC,
		cfg.getSpellsUC,
		cfg.useSpellUC,
		nil, // generateImageUC - disabled to avoid costs
		nil, // getSubscriptionUC
		nil, // checkLimitsUC
		nil, // getLeaderboardUC
		nil, // updateRatingUC
		performAbilityCheckUC,
		cfg.sessionRepo,
		combatRepo,
		feedbackRepo,
		eventRepo,
		nil, // indexDocUC - disabled to avoid costs
	)
	if err != nil {
		t.Fatalf("Failed to create Telegram bot: %v", err)
	}

	t.Run("Step 1: Help command (/help)", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/help")); err != nil {
			problems = append(problems, fmt.Sprintf("/help command failed: %v", err))
		}

		// Check for help response
		calls := fakeAPI.snapshotCalls()
		hasHelpResponse := false
		for _, call := range calls {
			if call.ChatID == chatID && strings.Contains(call.Text, "Доступные команды") {
				hasHelpResponse = true
				break
			}
		}
		if !hasHelpResponse {
			problems = append(problems, "Bot did not respond to /help command")
		}
	})

	t.Run("Step 2: New game creation (/newgame) - REAL LLM", func(t *testing.T) {
		if err := cfg.waitForRateLimit(ctx); err != nil {
			problems = append(problems, fmt.Sprintf("Rate limiter before /newgame: %v", err))
		}

		start := time.Now()
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/newgame эпическая фэнтези сага с драконами, эльфами и древними артефактами")); err != nil {
			problems = append(problems, fmt.Sprintf("/newgame failed: %v", err))
			t.Fatalf("/newgame failed: %v", err)
		}
		duration := time.Since(start)

		// Verify game session was created
		gs, err := cfg.sessionRepo.GetByChatID(ctx, chatID)
		if err != nil || gs == nil || !gs.IsActive() {
			problems = append(problems, "Game session not created after /newgame")
			t.Fatalf("Game session not created after /newgame")
		}

		t.Logf("✅ Game created in %.2fs, World ID=%d, Locations=%d", duration.Seconds(), gs.WorldID, len(gs.World.Locations))

		// Check LLM response quality
		dmResponse := lastNonThinkingPlayerFacingText(fakeAPI.snapshotCalls(), chatID)
		if dmResponse != "" {
			if len([]rune(dmResponse)) < 100 {
				llmFeedback = append(llmFeedback, fmt.Sprintf("DM response too short on game creation (%d chars): %s", len([]rune(dmResponse)), dmResponse))
			}
			if strings.Contains(dmResponse, "error") || strings.Contains(dmResponse, "Error") {
				llmFeedback = append(llmFeedback, fmt.Sprintf("DM response contains error indicators: %s", dmResponse))
			}
		}
	})

	t.Run("Step 3: Character creation (/createcharacter)", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/createcharacter Аэлар Светлое Крыло elf wizard")); err != nil {
			problems = append(problems, fmt.Sprintf("/createcharacter failed: %v", err))
			t.Fatalf("/createcharacter failed: %v", err)
		}

		// Verify character was created
		gs, _ := cfg.sessionRepo.GetByChatID(ctx, chatID)
		if gs == nil || gs.GetFirstPlayer() == nil {
			problems = append(problems, "Character not created after /createcharacter")
			t.Fatal("Character not created after /createcharacter")
		}

		player := gs.GetFirstPlayer()
		t.Logf("✅ Character created: %s (%s %s), Stats: STR=%d DEX=%d CON=%d INT=%d WIS=%d CHA=%d",
			player.Character.Name, player.Character.Race, player.Character.Class,
			player.Character.Stats.Strength, player.Character.Stats.Dexterity,
			player.Character.Stats.Constitution, player.Character.Stats.Intelligence,
			player.Character.Stats.Wisdom, player.Character.Stats.Charisma)
	})

	t.Run("Step 4: Exploration action - REAL LLM", func(t *testing.T) {
		if err := cfg.waitForRateLimit(ctx); err != nil {
			problems = append(problems, fmt.Sprintf("Rate limiter before exploration: %v", err))
		}

		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "Осматриваюсь вокруг, внимательно изучаю местность и ищу следы магии или опасности")); err != nil {
			problems = append(problems, fmt.Sprintf("Exploration action failed: %v", err))
			t.Fatalf("Exploration action failed: %v", err)
		}

		// Check DM response quality
		dmResponse := lastNonThinkingPlayerFacingText(fakeAPI.snapshotCalls(), chatID)
		if dmResponse != "" {
			if len([]rune(dmResponse)) < 50 {
				llmFeedback = append(llmFeedback, fmt.Sprintf("DM response too short on exploration (%d chars): %s", len([]rune(dmResponse)), dmResponse))
			}
		}

		// Check if location events were generated (first visit)
		gs, _ := cfg.sessionRepo.GetByChatID(ctx, chatID)
		if gs != nil && gs.CurrentLocationID != nil {
			events, _ := worldEventRepo.GetByLocationID(ctx, *gs.CurrentLocationID)
			if len(events) > 0 {
				t.Logf("✅ Location events generated: %d events at location %d", len(events), *gs.CurrentLocationID)
			}
		}
	})

	t.Run("Step 5: Ability check resolution (/roll)", func(t *testing.T) {
		// Create a pending ability check manually for testing
		gs, err := cfg.sessionRepo.GetByChatID(ctx, chatID)
		if err != nil || gs == nil {
			t.Fatalf("Session not found for pending check creation")
		}

		gs.SetPendingAbilityCheck("test_perception", "wisdom", 15)
		if err := cfg.sessionRepo.Save(ctx, gs); err != nil {
			t.Fatalf("Failed to save pending ability check: %v", err)
		}

		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/roll d20")); err != nil {
			problems = append(problems, fmt.Sprintf("/roll d20 failed: %v", err))
		}

		// Verify pending check was cleared
		gs, _ = cfg.sessionRepo.GetByChatID(ctx, chatID)
		if gs != nil && gs.HasPendingAbilityCheck() {
			problems = append(problems, "Pending ability check not cleared after /roll d20")
		}
	})

	t.Run("Step 6: Combat initiation action - REAL LLM", func(t *testing.T) {
		if err := cfg.waitForRateLimit(ctx); err != nil {
			problems = append(problems, fmt.Sprintf("Rate limiter before combat: %v", err))
		}

		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "Вызываю монстра на бой и готовлюсь к атаке!")); err != nil {
			problems = append(problems, fmt.Sprintf("Combat initiation action failed: %v", err))
		}

		// Check if combat was created
		gs, _ := cfg.sessionRepo.GetByChatID(ctx, chatID)
		activeCombat, _ := combatRepo.GetActiveBySessionID(ctx, gs.ID)
		if activeCombat != nil {
			t.Logf("✅ Combat initiated: %d participants", len(activeCombat.Participants))
		}
	})

	t.Run("Step 7: Battlefield display (/battlefield)", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/battlefield table")); err != nil {
			problems = append(problems, fmt.Sprintf("/battlefield failed: %v", err))
		}

		lastMsg := lastNonThinkingPlayerFacingText(fakeAPI.snapshotCalls(), chatID)
		if lastMsg == "" || !strings.Contains(lastMsg, "Поле боя") {
			problems = append(problems, "Battlefield message not found after /battlefield command")
		}
	})

	t.Run("Step 8: Combat attack (/attack)", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/attack посохом магии")); err != nil {
			problems = append(problems, fmt.Sprintf("/attack failed: %v", err))
		}

		// Allow time for async operations
		time.Sleep(200 * time.Millisecond)

		// Check combat state
		gs, _ := cfg.sessionRepo.GetByChatID(ctx, chatID)
		activeCombat, _ := combatRepo.GetActiveBySessionID(ctx, gs.ID)
		if activeCombat != nil {
			t.Logf("ℹ️  Combat continues: %d participants", len(activeCombat.Participants))
		}
	})

	t.Run("Step 9: Inventory check (/inventory)", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/inventory")); err != nil {
			problems = append(problems, fmt.Sprintf("/inventory failed: %v", err))
		}
	})

	t.Run("Step 10: Quests display (/quests)", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/quests")); err != nil {
			problems = append(problems, fmt.Sprintf("/quests failed: %v", err))
		}
	})

	t.Run("Step 11: Daily quests (/daily)", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/daily")); err != nil {
			problems = append(problems, fmt.Sprintf("/daily failed: %v", err))
		}
	})

	t.Run("Step 12: Spells display (/spells)", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/spells")); err != nil {
			problems = append(problems, fmt.Sprintf("/spells failed: %v", err))
		}
	})

	t.Run("Step 13: Achievements display (/achievements)", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/achievements")); err != nil {
			problems = append(problems, fmt.Sprintf("/achievements failed: %v", err))
		}
	})

	t.Run("Step 14: Map display (/map)", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/map")); err != nil {
			problems = append(problems, fmt.Sprintf("/map failed: %v", err))
		}

		// Check for navigation buttons
		calls := fakeAPI.snapshotCalls()
		hasNavigationButtons := false
		for _, call := range calls {
			if call.ChatID == chatID && strings.Contains(call.Text, "map_to_") {
				hasNavigationButtons = true
				break
			}
		}
		if !hasNavigationButtons {
			problems = append(problems, "No navigation buttons found after /map command")
		}
	})

	t.Run("Step 15: History display (/history)", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/history")); err != nil {
			problems = append(problems, fmt.Sprintf("/history failed: %v", err))
		}
	})

	t.Run("Step 16: Location movement test", func(t *testing.T) {
		// Try to move to another location if available
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/move_to_location 2")); err != nil {
			// This might fail if location doesn't exist - that's OK
			t.Logf("ℹ️  Location movement failed (expected if location doesn't exist): %v", err)
		}
	})

	t.Run("Step 17: Check for tool text leaks", func(t *testing.T) {
		if leak := findToolLeak(fakeAPI.snapshotCalls(), chatID); leak != "" {
			problems = append(problems, fmt.Sprintf("Tool text leak detected in player-facing message: %s", leak))
		}
	})

	t.Run("Step 18: Game end (/endgame)", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/endgame")); err != nil {
			problems = append(problems, fmt.Sprintf("/endgame failed: %v", err))
		}

		// Verify game was ended
		gs, _ := cfg.sessionRepo.GetByChatID(ctx, chatID)
		if gs != nil && gs.IsActive() {
			problems = append(problems, "Game not ended after /endgame command")
		} else {
			t.Log("✅ Game successfully ended")
		}
	})

	// ===== RECORD RESULTS =====

	if len(problems) > 0 {
		writeToTestingReport(problems)
		t.Logf("❌ Found problems: %d", len(problems))
		for i, problem := range problems {
			t.Logf("  %d. %s", i+1, problem)
		}
	} else {
		t.Log("✅ All core mechanics working correctly")
	}

	if len(llmFeedback) > 0 {
		writeToFeedback(llmFeedback)
		t.Logf("📝 Collected LLM feedback: %d entries", len(llmFeedback))
		for i, feedback := range llmFeedback {
			t.Logf("  %d. %s", i+1, feedback)
		}
	}
}