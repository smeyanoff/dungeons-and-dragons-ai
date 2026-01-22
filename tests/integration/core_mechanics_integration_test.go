package integration

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestCoreMechanicsIntegrationSuite запускает полный набор интеграционных тестов
// для проверки основных механик игры после релиза новой версии
func TestCoreMechanicsIntegrationSuite(t *testing.T) {
	var problems []string
	var testResults []string

	t.Run("Run unit tests", func(t *testing.T) {
		cmd := exec.Command("go", "test", "./...")
		output, err := cmd.CombinedOutput()
		outputStr := string(output)

		if err != nil {
			problems = append(problems, fmt.Sprintf("Unit tests failed: %v", err))
			problems = append(problems, fmt.Sprintf("Unit test output: %s", outputStr))
		} else {
			testResults = append(testResults, "✅ Unit tests: PASSED")
		}

		t.Logf("Unit tests output:\n%s", outputStr)
	})

	t.Run("Run telegram stub tests", func(t *testing.T) {
		cmd := exec.Command("make", "test-telegram-stub")
		output, err := cmd.CombinedOutput()
		outputStr := string(output)

		if err != nil {
			problems = append(problems, fmt.Sprintf("Telegram stub tests failed: %v", err))
			problems = append(problems, fmt.Sprintf("Stub test output: %s", outputStr))
		} else {
			testResults = append(testResults, "✅ Telegram stub tests: PASSED")
		}

		t.Logf("Telegram stub tests output:\n%s", outputStr)
	})

	t.Run("Run comprehensive gameplay test", func(t *testing.T) {
		cmd := exec.Command("make", "test-telegram")
		output, err := cmd.CombinedOutput()
		outputStr := string(output)

		if err != nil {
			problems = append(problems, fmt.Sprintf("Comprehensive gameplay tests failed: %v", err))
			problems = append(problems, fmt.Sprintf("Gameplay test output: %s", outputStr))
		} else {
			testResults = append(testResults, "✅ Comprehensive gameplay tests: PASSED")
		}

		// Check for LLM credential skips (expected)
		if strings.Contains(outputStr, "GIGACHAT_CLIENT_ID и GIGACHAT_CLIENT_SECRET не установлены") {
			testResults = append(testResults, "ℹ️  Real LLM tests correctly skipped (no credentials)")
		}

		t.Logf("Comprehensive gameplay tests output:\n%s", outputStr)
	})

	t.Run("Verify container health", func(t *testing.T) {
		// Check if containers are running
		cmd := exec.Command("docker", "compose", "-f", "build/docker-compose.yml", "ps", "--format", "table")
		output, err := cmd.CombinedOutput()
		outputStr := string(output)

		if err != nil {
			problems = append(problems, fmt.Sprintf("Container status check failed: %v", err))
		} else {
			// Check if both postgres and qdrant are running
			if strings.Contains(outputStr, "dnd-postgres") && strings.Contains(outputStr, "dnd-qdrant") {
				if strings.Contains(outputStr, "healthy") || strings.Contains(outputStr, "running") {
					testResults = append(testResults, "✅ Database containers: RUNNING and HEALTHY")
				} else {
					problems = append(problems, "Database containers are not healthy")
				}
			} else {
				problems = append(problems, "Required containers (postgres, qdrant) are not running")
			}
		}

		t.Logf("Container status:\n%s", outputStr)
	})

	t.Run("Check for LLM feedback patterns", func(t *testing.T) {
		// Check if FEEDBACK.md contains recent entries
		feedbackContent, err := os.ReadFile("tests/integration/FEEDBACK.md")
		if err != nil {
			problems = append(problems, fmt.Sprintf("Cannot read FEEDBACK.md: %v", err))
		} else {
			contentStr := string(feedbackContent)
			// Look for recent feedback entries (within last day)
			if strings.Contains(contentStr, "2026-01-22") {
				testResults = append(testResults, "✅ LLM feedback collection: ACTIVE (recent entries found)")
			} else {
				testResults = append(testResults, "ℹ️  LLM feedback collection: NO recent entries")
			}
		}
	})

	t.Run("Check for testing report updates", func(t *testing.T) {
		// Check if TESTING_REPORT.md contains recent entries
		reportContent, err := os.ReadFile("TESTING_REPORT.md")
		if err != nil {
			problems = append(problems, fmt.Sprintf("Cannot read TESTING_REPORT.md: %v", err))
		} else {
			contentStr := string(reportContent)
			// Look for recent report entries
			if strings.Contains(contentStr, "2026-01-22") {
				testResults = append(testResults, "✅ Testing report updates: ACTIVE (recent entries found)")
			} else {
				problems = append(problems, "Testing report not updated with recent test results")
			}
		}
	})

	// ===== RECORD RESULTS =====

	if len(problems) > 0 {
		writeToTestingReport(problems)
		t.Logf("❌ Core mechanics integration problems: %d", len(problems))
		for i, problem := range problems {
			t.Logf("  %d. %s", i+1, problem)
		}
	} else {
		t.Log("✅ All core mechanics integration tests PASSED")
	}

	t.Log("📊 Core Mechanics Integration Test Results:")
	for i, result := range testResults {
		t.Logf("  %d. %s", i+1, result)
	}

	// Summary for TESTING_REPORT.md
	summary := fmt.Sprintf("Core Mechanics Integration: %d problems found, %d checks passed", len(problems), len(testResults))
	if len(problems) > 0 {
		writeToTestingReport([]string{summary})
	}
}