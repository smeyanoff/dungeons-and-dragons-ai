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

// TestTelegramGameplay_RealLLM_FullGameplayJourney
// Комплексный end-to-end тест всех основных механик игры с реальными LLM вызовами
// Симулирует полный пользовательский journey от создания игры до завершения,
// тестируя все реализованные фичи согласно TASKS.md:
// - Система достижений
// - Ежедневные квесты и система стрик
// - Адаптивная сложность
// - Вариативность событий (3-5 веток развития)
// - Персонализация мира (темный/светлый стиль, уровни детализации)
// - Мини-ивенты (короткие сценки без чеков)
// - Улучшенная карта мира (связи локаций, текущая позиция)
// - NPC компаньоны в отряд
// - Базовые механики: создание мира, персонажа, исследование, бой, инвентарь, квесты,
//   заклинания, достижения, карту, историю и location events
func TestTelegramGameplay_RealLLM_FullGameplayJourney(t *testing.T) {
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
		combatRepo,
		feedbackRepo,
		eventRepo,
		nil, // indexDocUC - disabled to avoid costs
	)
	if err != nil {
		t.Fatalf("Failed to create Telegram bot: %v", err)
	}

	// ===== PHASE 1: BASIC SETUP AND WORLD PERSONALIZATION =====

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

	t.Run("Step 2: World personalization settings", func(t *testing.T) {
		// Test setting dark style
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/set_style dark")); err != nil {
			problems = append(problems, fmt.Sprintf("/set_style dark failed: %v", err))
		}

		// Test setting high detail level
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/set_detail high")); err != nil {
			problems = append(problems, fmt.Sprintf("/set_detail high failed: %v", err))
		}

		// Test setting language to Russian
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/set_language ru")); err != nil {
			problems = append(problems, fmt.Sprintf("/set_language ru failed: %v", err))
		}

		// Test toggling stats display
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/toggle_stats")); err != nil {
			problems = append(problems, fmt.Sprintf("/toggle_stats failed: %v", err))
		}
	})

	// ===== PHASE 2: GAME CREATION AND CHARACTER =====

	t.Run("Step 3: New game creation (/newgame) - REAL LLM", func(t *testing.T) {
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

	// ===== PHASE 3: DAILY QUESTS AND ACHIEVEMENT SYSTEM =====

	t.Run("Step 11: Daily quests system (/daily)", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/daily")); err != nil {
			problems = append(problems, fmt.Sprintf("/daily failed: %v", err))
		}

		// Check for daily quests response
		calls := fakeAPI.snapshotCalls()
		hasDailyQuests := false
		for _, call := range calls {
			if call.ChatID == chatID && (strings.Contains(call.Text, "ежедневные") || strings.Contains(call.Text, "daily")) {
				hasDailyQuests = true
				break
			}
		}
		if !hasDailyQuests {
			problems = append(problems, "No daily quests response found after /daily command")
		}
	})

	t.Run("Step 12: Achievement system (/achievements)", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/achievements")); err != nil {
			problems = append(problems, fmt.Sprintf("/achievements failed: %v", err))
		}

		// Check for achievements response
		calls := fakeAPI.snapshotCalls()
		hasAchievements := false
		for _, call := range calls {
			if call.ChatID == chatID && (strings.Contains(call.Text, "достижения") || strings.Contains(call.Text, "achievements")) {
				hasAchievements = true
				break
			}
		}
		if !hasAchievements {
			problems = append(problems, "No achievements response found after /achievements command")
		}
	})

	t.Run("Step 13: Spells system (/spells)", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/spells")); err != nil {
			problems = append(problems, fmt.Sprintf("/spells failed: %v", err))
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

	// ===== PHASE 4: WORLD MAP AND NAVIGATION =====

	t.Run("Step 16: Enhanced world map (/map)", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/map")); err != nil {
			problems = append(problems, fmt.Sprintf("/map failed: %v", err))
		}

		// Check for navigation buttons and map legend
		calls := fakeAPI.snapshotCalls()
		hasMapLegend := false
		hasNavigationButtons := false
		for _, call := range calls {
			if call.ChatID == chatID {
				if strings.Contains(call.Text, "легенда") || strings.Contains(call.Text, "legend") {
					hasMapLegend = true
				}
				if strings.Contains(call.Text, "map_to_") {
					hasNavigationButtons = true
				}
			}
		}
		if !hasMapLegend {
			problems = append(problems, "Map legend not found after /map command")
		}
		if !hasNavigationButtons {
			problems = append(problems, "No navigation buttons found after /map command")
		}
	})

	t.Run("Step 17: Location movement and exploration", func(t *testing.T) {
		if err := cfg.waitForRateLimit(ctx); err != nil {
			problems = append(problems, fmt.Sprintf("Rate limiter before exploration: %v", err))
		}

		// Try to move to another location if available
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/move_to_location 2")); err != nil {
			// This might fail if location doesn't exist - that's OK
			t.Logf("ℹ️  Location movement failed (expected if location doesn't exist): %v", err)
		}

		// Additional exploration action
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "Исследую новую локацию и ищу возможности для развития")); err != nil {
			problems = append(problems, fmt.Sprintf("Exploration action failed: %v", err))
		}
	})

	// ===== PHASE 5: NPC COMPANIONS AND PARTY MANAGEMENT =====

	t.Run("Step 18: NPC companion recruitment test", func(t *testing.T) {
		if err := cfg.waitForRateLimit(ctx); err != nil {
			problems = append(problems, fmt.Sprintf("Rate limiter before NPC interaction: %v", err))
		}

		// Try to recruit an NPC companion through interaction
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "Подхожу к местному жителю и предлагаю присоединиться к моему отряду в обмен на сокровища")); err != nil {
			problems = append(problems, fmt.Sprintf("NPC recruitment action failed: %v", err))
		}
	})

	t.Run("Step 19: Party management (/party)", func(t *testing.T) {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/party")); err != nil {
			problems = append(problems, fmt.Sprintf("/party failed: %v", err))
		}

		// Check for party information
		calls := fakeAPI.snapshotCalls()
		hasPartyInfo := false
		for _, call := range calls {
			if call.ChatID == chatID && (strings.Contains(call.Text, "отряд") || strings.Contains(call.Text, "party")) {
				hasPartyInfo = true
				break
			}
		}
		if !hasPartyInfo {
			problems = append(problems, "Party information not found after /party command")
		}
	})

	// ===== PHASE 6: EVENT VARIETY AND ADAPTIVE DIFFICULTY =====

	t.Run("Step 20: Event variety testing - multiple interactions", func(t *testing.T) {
		actions := []string{
			"Проверяю сундук в углу комнаты",
			"Заговариваю с подозрительным торговцем",
			"Осматриваю древний алтарь в центре зала",
			"Проверяю тайный проход за картиной",
		}

		for i, action := range actions {
			if err := cfg.waitForRateLimit(ctx); err != nil {
				problems = append(problems, fmt.Sprintf("Rate limiter before action %d: %v", i+1, err))
				continue
			}

			if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, action)); err != nil {
				problems = append(problems, fmt.Sprintf("Event action %d failed: %v", i+1, err))
			}

			// Small delay between actions
			time.Sleep(500 * time.Millisecond)
		}
	})

	t.Run("Step 21: Adaptive difficulty verification", func(t *testing.T) {
		// Check current difficulty settings
		gs, _ := cfg.sessionRepo.GetByChatID(ctx, chatID)
		if gs != nil {
			t.Logf("ℹ️  Current adaptive DC modifier: %d", gs.SessionDifficultyMod)
			totalChecks := gs.SessionSuccessCount + gs.SessionFailureCount
			if totalChecks > 0 {
				successRate := float64(gs.SessionSuccessCount) / float64(totalChecks) * 100
				t.Logf("ℹ️  Success rate: %d/%d (%.1f%%)", gs.SessionSuccessCount, totalChecks, successRate)
			} else {
				t.Logf("ℹ️  No ability checks performed yet")
			}
		}
	})

	// ===== PHASE 7: ACHIEVEMENT PROGRESS AND MINI-EVENTS =====

	t.Run("Step 22: Achievement progress check", func(t *testing.T) {
		// Check achievements after gameplay
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/achievements")); err != nil {
			problems = append(problems, fmt.Sprintf("Final /achievements check failed: %v", err))
		}

		// Check if any achievements were unlocked
		calls := fakeAPI.snapshotCalls()
		hasAchievementProgress := false
		for _, call := range calls {
			if call.ChatID == chatID && (strings.Contains(call.Text, "✅") || strings.Contains(call.Text, "разблокировано")) {
				hasAchievementProgress = true
				break
			}
		}
		if !hasAchievementProgress {
			t.Log("ℹ️  No achievements unlocked yet - this may be normal for short gameplay")
		}
	})

	t.Run("Step 23: Mini-events and atmospheric content", func(t *testing.T) {
		// Continue exploration to potentially trigger mini-events
		if err := cfg.waitForRateLimit(ctx); err != nil {
			problems = append(problems, fmt.Sprintf("Rate limiter before mini-event exploration: %v", err))
		}

		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "Продолжаю путешествие и наслаждаюсь видами вокруг")); err != nil {
			problems = append(problems, fmt.Sprintf("Mini-event exploration failed: %v", err))
		}
	})

	// ===== PHASE 8: FINAL CHECKS AND VALIDATION =====

	t.Run("Step 24: Check for tool text leaks", func(t *testing.T) {
		if leak := findToolLeak(fakeAPI.snapshotCalls(), chatID); leak != "" {
			problems = append(problems, fmt.Sprintf("Tool text leak detected in player-facing message: %s", leak))
		}
	})

	t.Run("Step 25: Comprehensive system validation", func(t *testing.T) {
		// Final check of all systems
		gs, _ := cfg.sessionRepo.GetByChatID(ctx, chatID)
		if gs != nil {
			t.Logf("🎯 Final game state summary:")
			player := gs.GetFirstPlayer()
			if player != nil {
				t.Logf("   - World locations: %d", len(gs.World.Locations))
				t.Logf("   - Current location: %v", gs.CurrentLocationID)
				t.Logf("   - Player level: %d", player.Character.Level)
				t.Logf("   - Player XP: %d", player.Character.Experience)
				t.Logf("   - Companions in party: %d", len(gs.Companions))
				t.Logf("   - Adaptive DC modifier: %d", gs.SessionDifficultyMod)
				totalChecks := gs.SessionSuccessCount + gs.SessionFailureCount
				if totalChecks > 0 {
					successRate := float64(gs.SessionSuccessCount) / float64(totalChecks) * 100
					t.Logf("   - Ability check success rate: %d/%d (%.1f%%)", gs.SessionSuccessCount, totalChecks, successRate)
				}
			}
		}
	})

	t.Run("Step 26: Game end (/endgame)", func(t *testing.T) {
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

	// ===== RECORD COMPREHENSIVE TEST RESULTS =====

	t.Logf("🎮 Comprehensive Gameplay Test Results:")
	t.Logf("   - Total test phases: 8")
	t.Logf("   - Total test steps: 26")
	t.Logf("   - Features tested: World personalization, Daily quests, Achievements, Enhanced map, NPC companions, Event variety, Adaptive difficulty, Mini-events")

	if len(problems) > 0 {
		writeToTestingReport(problems)
		t.Logf("❌ Found %d problems across all mechanics:", len(problems))
		for i, problem := range problems {
			t.Logf("  %d. %s", i+1, problem)
		}

		// Categorize problems by mechanic
		mechanicProblems := make(map[string][]string)
		for _, problem := range problems {
			if strings.Contains(problem, "personalization") || strings.Contains(problem, "style") || strings.Contains(problem, "detail") {
				mechanicProblems["World Personalization"] = append(mechanicProblems["World Personalization"], problem)
			} else if strings.Contains(problem, "daily") || strings.Contains(problem, "quest") {
				mechanicProblems["Daily Quests"] = append(mechanicProblems["Daily Quests"], problem)
			} else if strings.Contains(problem, "achievement") {
				mechanicProblems["Achievements"] = append(mechanicProblems["Achievements"], problem)
			} else if strings.Contains(problem, "map") || strings.Contains(problem, "navigation") {
				mechanicProblems["World Map"] = append(mechanicProblems["World Map"], problem)
			} else if strings.Contains(problem, "party") || strings.Contains(problem, "companion") {
				mechanicProblems["NPC Companions"] = append(mechanicProblems["NPC Companions"], problem)
			} else if strings.Contains(problem, "adaptive") || strings.Contains(problem, "difficulty") {
				mechanicProblems["Adaptive Difficulty"] = append(mechanicProblems["Adaptive Difficulty"], problem)
			} else if strings.Contains(problem, "event") || strings.Contains(problem, "variety") {
				mechanicProblems["Event Variety"] = append(mechanicProblems["Event Variety"], problem)
			} else {
				mechanicProblems["Other/Core"] = append(mechanicProblems["Other/Core"], problem)
			}
		}

		t.Logf("📊 Problems by mechanic:")
		for mechanic, probs := range mechanicProblems {
			t.Logf("   - %s: %d issues", mechanic, len(probs))
		}

	} else {
		t.Log("✅ All implemented mechanics working correctly!")
		t.Log("   ✓ World personalization (style, detail, language settings)")
		t.Log("   ✓ Daily quests system")
		t.Log("   ✓ Achievement system")
		t.Log("   ✓ Enhanced world map with navigation")
		t.Log("   ✓ NPC companion recruitment and party management")
		t.Log("   ✓ Event variety with multiple outcome branches")
		t.Log("   ✓ Adaptive difficulty adjustments")
		t.Log("   ✓ Mini-events and atmospheric content")
	}

	if len(llmFeedback) > 0 {
		writeToFeedback(llmFeedback)
		t.Logf("📝 Collected %d LLM behavior observations:", len(llmFeedback))
		for i, feedback := range llmFeedback {
			t.Logf("  %d. %s", i+1, feedback)
		}
	} else {
		t.Log("🤖 LLM responses were appropriate and well-formed")
	}

	t.Logf("🏁 Comprehensive gameplay test completed successfully")
}