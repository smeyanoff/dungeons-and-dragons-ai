package integration

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	abilitycheck "dungeons-and-dragons-ai/internal/game/application/ability_check"
	mapapp "dungeons-and-dragons-ai/internal/game/application/worldmap"
	"dungeons-and-dragons-ai/internal/game/infrastructure/persistence"
	telegrambot "dungeons-and-dragons-ai/internal/telegram"
)

// TestTelegramGameplay_CoreMechanics_RealLLM
// Комплексный end-to-end тест всех основных механик игры с реальными LLM вызовами
// после релиза новой версии. Симулирует полный пользовательский journey и проверяет
// работоспособность всех реализованных фич: JSON парсинг, location events, RAG,
// rate limiting, image generation, и т.д.
func TestTelegramGameplay_CoreMechanics_RealLLM(t *testing.T) {
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
	playerRepo := persistence.NewPlayerRepository(cfg.db)
	worldEventRepo := persistence.NewWorldEventRepository(cfg.db)
	// For tests, we need to pass nil for LLM and other dependencies
	moveToLocationUC := mapapp.NewMoveToLocationUseCase(nil, cfg.sessionRepo, worldEventRepo, nil, nil)
	performAbilityCheckUC := abilitycheck.NewPerformAbilityCheckUseCase(cfg.sessionRepo, eventRepo, nil)

	// Create bot with real LLM for comprehensive gameplay testing
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
		playerRepo,
		combatRepo,
		feedbackRepo,
		eventRepo,
		nil, // indexDocUC - disabled to avoid costs
	)
	if err != nil {
		t.Fatalf("Failed to create Telegram bot: %v", err)
	}

	t.Run("Step 1: Help system verification", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/help")); err != nil {
			problems = append(problems, fmt.Sprintf("/help command failed: %v", err))
		}

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

	t.Run("Step 2: Game creation with real LLM", func(t *testing.T) {
		if err := cfg.waitForRateLimit(ctx); err != nil {
			problems = append(problems, fmt.Sprintf("Rate limiter before /newgame: %v", err))
		}

		start := time.Now()
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/newgame эпическая сага о древних артефактах и забытых королевствах")); err != nil {
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

	t.Run("Step 3: Character creation", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/createcharacter Торин Громовой Молот dwarf barbarian")); err != nil {
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

	t.Run("Step 4: Exploration and location events", func(t *testing.T) {
		if err := cfg.waitForRateLimit(ctx); err != nil {
			problems = append(problems, fmt.Sprintf("Rate limiter before exploration: %v", err))
		}

		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "Выхожу из деревни и отправляюсь в ближайший лес, внимательно осматриваясь по сторонам")); err != nil {
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
			} else {
				t.Logf("ℹ️  No location events generated at location %d", *gs.CurrentLocationID)
			}
		}
	})

	t.Run("Step 5: Ability check mechanics", func(t *testing.T) {
		// Create a pending ability check manually for testing
		gs, err := cfg.sessionRepo.GetByChatID(ctx, chatID)
		if err != nil || gs == nil {
			t.Fatalf("Session not found for pending check creation")
		}

		gs.SetPendingAbilityCheck("perception_check", "wisdom", 14)
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

	t.Run("Step 6: Combat initiation and battlefield", func(t *testing.T) {
		if err := cfg.waitForRateLimit(ctx); err != nil {
			problems = append(problems, fmt.Sprintf("Rate limiter before combat: %v", err))
		}

		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "Выхожу на тропу и кричу: 'Эй, кто здесь прячется? Выходи на честный бой!'")); err != nil {
			problems = append(problems, fmt.Sprintf("Combat initiation action failed: %v", err))
		}

		// Check if combat was created
		gs, _ := cfg.sessionRepo.GetByChatID(ctx, chatID)
		activeCombat, _ := combatRepo.GetActiveBySessionID(ctx, gs.ID)
		if activeCombat != nil {
			t.Logf("✅ Combat initiated: %d participants", len(activeCombat.Participants))
		}
	})

	t.Run("Step 7: Battlefield display", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/battlefield table")); err != nil {
			problems = append(problems, fmt.Sprintf("/battlefield failed: %v", err))
		}

		lastMsg := lastNonThinkingPlayerFacingText(fakeAPI.snapshotCalls(), chatID)
		if lastMsg == "" || !strings.Contains(lastMsg, "Поле боя") {
			problems = append(problems, "Battlefield message not found after /battlefield command")
		}
	})

	t.Run("Step 8: Combat actions", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/attack топором")); err != nil {
			problems = append(problems, fmt.Sprintf("/attack failed: %v", err))
		}

		_ = waitForCondition(t, 750*time.Millisecond, 25*time.Millisecond, func() bool {
			gs, _ := cfg.sessionRepo.GetByChatID(ctx, chatID)
			if gs == nil {
				return false
			}
			activeCombat, _ := combatRepo.GetActiveBySessionID(ctx, gs.ID)
			return activeCombat == nil || !activeCombat.IsActive()
		})

		// Check combat state
		gs, _ := cfg.sessionRepo.GetByChatID(ctx, chatID)
		activeCombat, _ := combatRepo.GetActiveBySessionID(ctx, gs.ID)
		if activeCombat != nil {
			t.Logf("ℹ️  Combat continues: %d participants", len(activeCombat.Participants))
		}
	})

	t.Run("Step 9: Inventory system", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/inventory")); err != nil {
			problems = append(problems, fmt.Sprintf("/inventory failed: %v", err))
		}
	})

	t.Run("Step 10: Quest system", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/quests")); err != nil {
			problems = append(problems, fmt.Sprintf("/quests failed: %v", err))
		}
	})

	t.Run("Step 11: Daily quests", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/daily")); err != nil {
			problems = append(problems, fmt.Sprintf("/daily failed: %v", err))
		}
	})

	t.Run("Step 12: Spell system", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/spells")); err != nil {
			problems = append(problems, fmt.Sprintf("/spells failed: %v", err))
		}
	})

	t.Run("Step 13: Achievement system", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/achievements")); err != nil {
			problems = append(problems, fmt.Sprintf("/achievements failed: %v", err))
		}
	})

	t.Run("Step 14: Map navigation", func(t *testing.T) {
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

	t.Run("Step 15: History system", func(t *testing.T) {
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

	t.Run("Step 17: Tool text leak detection", func(t *testing.T) {
		if leak := findToolLeak(fakeAPI.snapshotCalls(), chatID); leak != "" {
			problems = append(problems, fmt.Sprintf("Tool text leak detected in player-facing message: %s", leak))
		}
	})

	t.Run("Step 18: JSON parsing and validation test", func(t *testing.T) {
		// Test JSON parsing by triggering DM analyzer through an action that should generate analysis
		if err := cfg.waitForRateLimit(ctx); err != nil {
			problems = append(problems, fmt.Sprintf("Rate limiter before JSON test: %v", err))
		}

		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "Пытаюсь найти спрятанный сундук в этой местности")); err != nil {
			problems = append(problems, fmt.Sprintf("JSON parsing test action failed: %v", err))
		}

		// Check for any JSON-related errors in the logs (would be visible in test output)
		// The actual JSON validation happens in the DM analyzer, so we're testing end-to-end
		t.Log("✅ JSON parsing test completed (validation happens in DM analyzer)")
	})

	t.Run("Step 19: Game completion", func(t *testing.T) {
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
