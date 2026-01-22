package integration

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"dungeons-and-dragons-ai/internal/game/application/dm_analyzer"
	"dungeons-and-dragons-ai/internal/game/infrastructure/persistence"
	llminfrastructure "dungeons-and-dragons-ai/internal/llm/infrastructure"
	"dungeons-and-dragons-ai/pkg/gigachat"
)

// TestLLM_RealIntegration_CombatAnalysis тестирует анализ боя с реальной LLM
// Проверяет работу DM Analyzer с реальными ответами GigaChat
func TestLLM_RealIntegration_CombatAnalysis(t *testing.T) {
	cfg := setupIntegrationTest(t)
	defer cleanupTest(t, cfg)

	ctx := cfg.ctx

	// Check credentials first
	clientID := getEnv("GIGACHAT_CLIENT_ID", "")
	clientSecret := getEnv("GIGACHAT_CLIENT_SECRET", "")
	if clientID == "" || clientSecret == "" {
		t.Skip("GIGACHAT_CLIENT_ID and GIGACHAT_CLIENT_SECRET not set, skipping real LLM test")
	}

	// Setup GigaChat LLM
	gigachatClient := gigachat.NewClient(gigachat.Config{
		AuthBaseURL:   getEnv("GIGACHAT_AUTH_URL", "https://ngw.devices.sberbank.ru:9443"),
		APIBaseURL:    getEnv("GIGACHAT_API_URL", "https://gigachat.devices.sberbank.ru/api/v1"),
		ClientID:      clientID,
		ClientSecret:  clientSecret,
		Scope:         getEnv("GIGACHAT_SCOPE", "GIGACHAT_API_PERS"),
		SkipTLSVerify: getEnv("GIGACHAT_SKIP_TLS_VERIFY", "false") == "true",
	})

	llm := llminfrastructure.NewGigachatLLM(gigachatClient, getEnv("GIGACHAT_MODEL", "GigaChat"))
	llm = wrapLLMWithTestRateLimit(llm)

	// Setup repositories
	combatRepo := persistence.NewCombatRepository(cfg.db)
	questRepo := persistence.NewQuestRepository(cfg.db)
	inventoryRepo := persistence.NewInventoryRepository(cfg.db)

	var problems []string
	var llmFeedback []string

	testCases := []struct {
		name         string
		dmResponse   string
		expectCombat bool
		description  string
	}{
		{
			name:         "Combat detection - goblin attack",
			dmResponse:   "Внезапно из темноты пещеры выскакивает гоблин и бросается на героя с ржавым кинжалом!",
			expectCombat: true,
			description:  "Test basic combat detection",
		},
		{
			name:         "No combat - peaceful exploration",
			dmResponse:   "Вы осматриваете древний храм. Стены покрыты пылью веков, но здесь тихо и спокойно.",
			expectCombat: false,
			description:  "Test non-combat scenario recognition",
		},
		{
			name:         "Combat with multiple enemies",
			dmResponse:   "Из засады выскакивают два орка и тролль! Они окружают героя, готовясь к атаке.",
			expectCombat: true,
			description:  "Test multiple enemy combat detection",
		},
		{
			name:         "Location discovery",
			dmResponse:   "Вы входите в скрытую комнату, полную древних артефактов. Это место кажется неизведанным.",
			expectCombat: false,
			description:  "Test location discovery without combat",
		},
		{
			name:         "Quest completion",
			dmResponse:   "Наконец-то вы нашли потерянный артефакт! Квест выполнен, и вы чувствуете прилив сил.",
			expectCombat: false,
			description:  "Test quest completion recognition",
		},
	}

	for i, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create separate analyzer for each test case to avoid session conflicts
			sessionID := uint(i + 1)
			analyzer := dm_analyzer.NewAnalyzeDMResponseUseCase(
				llm,
				combatRepo,
				questRepo,
				inventoryRepo,
				sessionID,
				cfg.chatID,
				1, // worldID
				1, // characterID
				1, // playerID
			)

			// Rate limiting - wait 2 seconds between requests
			time.Sleep(2 * time.Second)

			start := time.Now()
			result, err := analyzer.Execute(ctx, tc.dmResponse)
			duration := time.Since(start)

			if err != nil {
				problems = append(problems, fmt.Sprintf("%s failed: %v", tc.name, err))
				llmFeedback = append(llmFeedback, fmt.Sprintf("%s: LLM error after %.2fs - %v", tc.name, duration.Seconds(), err))
				return
			}

			t.Logf("✅ %s completed in %.2fs", tc.name, duration.Seconds())

			// Validate combat detection
			if result.CombatDetected != tc.expectCombat {
				problems = append(problems, fmt.Sprintf("%s: Expected combat=%v, got combat=%v", tc.name, tc.expectCombat, result.CombatDetected))
				llmFeedback = append(llmFeedback, fmt.Sprintf("%s: Incorrect combat detection (expected %v, got %v)", tc.name, tc.expectCombat, result.CombatDetected))
			}

			// Validate enemy parsing when combat expected
			if tc.expectCombat && len(result.Enemies) == 0 {
				llmFeedback = append(llmFeedback, fmt.Sprintf("%s: Combat detected but no enemies parsed", tc.name))
			}

			// Check response quality
			if result.CombatDetected && len(result.Enemies) > 0 {
				for _, enemy := range result.Enemies {
					if enemy.Name == "" || (enemy.HP != nil && *enemy.HP <= 0) {
						hp := 0
						if enemy.HP != nil {
							hp = *enemy.HP
						}
						llmFeedback = append(llmFeedback, fmt.Sprintf("%s: Invalid enemy data - Name: '%s', HP: %d", tc.name, enemy.Name, hp))
					}
				}
			}

			// Check for truncated JSON patterns
			if strings.Contains(tc.dmResponse, "items_received") && !strings.Contains(tc.dmResponse, "]") {
				llmFeedback = append(llmFeedback, fmt.Sprintf("%s: Potential truncated JSON detected", tc.name))
			}
		})
	}

	// ===== RECORD RESULTS =====

	if len(problems) > 0 {
		writeToTestingReport(problems)
		t.Logf("❌ Real LLM integration problems: %d", len(problems))
		for i, problem := range problems {
			t.Logf("  %d. %s", i+1, problem)
		}
	} else {
		t.Log("✅ All real LLM integration tests PASSED")
	}

	if len(llmFeedback) > 0 {
		writeToFeedback(llmFeedback)
		t.Logf("📝 Collected LLM feedback: %d entries", len(llmFeedback))
		for i, feedback := range llmFeedback {
			t.Logf("  %d. %s", i+1, feedback)
		}
	}

	// Summary
	summary := fmt.Sprintf("Real LLM Combat Analysis: %d problems, %d feedback items from %d test cases", len(problems), len(llmFeedback), len(testCases))
	writeToTestingReport([]string{summary})
}

// TestLLM_RealIntegration_RateLimit тестирует rate limiting с реальной LLM
func TestLLM_RealIntegration_RateLimit(t *testing.T) {
	cfg := setupIntegrationTest(t)
	defer cleanupTest(t, cfg)

	ctx := cfg.ctx

	// Check credentials first
	clientID := getEnv("GIGACHAT_CLIENT_ID", "")
	clientSecret := getEnv("GIGACHAT_CLIENT_SECRET", "")
	if clientID == "" || clientSecret == "" {
		t.Skip("GIGACHAT_CLIENT_ID and GIGACHAT_CLIENT_SECRET not set, skipping real LLM test")
	}

	// Setup GigaChat LLM
	gigachatClient := gigachat.NewClient(gigachat.Config{
		AuthBaseURL:   getEnv("GIGACHAT_AUTH_URL", "https://ngw.devices.sberbank.ru:9443"),
		APIBaseURL:    getEnv("GIGACHAT_API_URL", "https://gigachat.devices.sberbank.ru/api/v1"),
		ClientID:      clientID,
		ClientSecret:  clientSecret,
		Scope:         getEnv("GIGACHAT_SCOPE", "GIGACHAT_API_PERS"),
		SkipTLSVerify: getEnv("GIGACHAT_SKIP_TLS_VERIFY", "false") == "true",
	})

	llm := llminfrastructure.NewGigachatLLM(gigachatClient, getEnv("GIGACHAT_MODEL", "GigaChat"))

	var problems []string
	var rateLimitFeedback []string

	// Test rapid requests to check rate limiting
	t.Run("Rapid requests rate limiting", func(t *testing.T) {
		start := time.Now()

		// Make 5 rapid requests
		for i := 0; i < 5; i++ {
			_, err := llm.Generate(ctx, fmt.Sprintf("Test request %d: Generate a simple greeting", i+1))
			if err != nil {
				if strings.Contains(err.Error(), "429") || strings.Contains(err.Error(), "rate limit") {
					rateLimitFeedback = append(rateLimitFeedback, fmt.Sprintf("Rate limit hit on request %d after %.2fs", i+1, time.Since(start).Seconds()))
					break
				} else {
					problems = append(problems, fmt.Sprintf("Request %d failed: %v", i+1, err))
				}
			} else {
				t.Logf("✅ Request %d successful", i+1)
			}

			// Small delay between requests
			time.Sleep(100 * time.Millisecond)
		}

		totalDuration := time.Since(start)
		t.Logf("Rate limit test completed in %.2fs", totalDuration.Seconds())

		if len(rateLimitFeedback) == 0 {
			rateLimitFeedback = append(rateLimitFeedback, "No rate limiting detected in rapid requests test")
		}
	})

	// Test with proper rate limiting
	t.Run("Properly rate limited requests", func(t *testing.T) {
		llm = wrapLLMWithTestRateLimit(llm)

		start := time.Now()

		// Make 3 requests with proper rate limiting
		for i := 0; i < 3; i++ {
			reqStart := time.Now()
			_, err := llm.Generate(ctx, fmt.Sprintf("Rate limited test request %d", i+1))
			reqDuration := time.Since(reqStart)

			if err != nil {
				problems = append(problems, fmt.Sprintf("Rate limited request %d failed: %v", i+1, err))
			} else {
				t.Logf("✅ Rate limited request %d completed in %.2fs", i+1, reqDuration.Seconds())

				// Check if rate limiting is working (should take at least 2 seconds per request)
				if reqDuration < 2*time.Second {
					rateLimitFeedback = append(rateLimitFeedback, fmt.Sprintf("Rate limiting not effective for request %d (took %.2fs)", i+1, reqDuration.Seconds()))
				}
			}
		}

		totalDuration := time.Since(start)
		t.Logf("Proper rate limiting test completed in %.2fs", totalDuration.Seconds())
	})

	// ===== RECORD RESULTS =====

	if len(problems) > 0 {
		writeToTestingReport(problems)
		t.Logf("❌ Rate limiting test problems: %d", len(problems))
		for i, problem := range problems {
			t.Logf("  %d. %s", i+1, problem)
		}
	}

	writeToFeedback(rateLimitFeedback)
	t.Logf("📝 Rate limiting feedback: %d entries", len(rateLimitFeedback))
	for i, feedback := range rateLimitFeedback {
		t.Logf("  %d. %s", i+1, feedback)
	}
}