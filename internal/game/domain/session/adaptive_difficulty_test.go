package session

import (
	"testing"
)

func TestGameSession_GetAdaptiveDC(t *testing.T) {
	tests := []struct {
		name               string
		baseDC             int
		sessionDifficultyMod int
		expectedDC         int
	}{
		{
			name:               "normal difficulty, base DC 15",
			baseDC:             15,
			sessionDifficultyMod: 0,
			expectedDC:         15,
		},
		{
			name:               "easy difficulty, base DC 15",
			baseDC:             15,
			sessionDifficultyMod: -1,
			expectedDC:         14,
		},
		{
			name:               "very easy difficulty, base DC 15",
			baseDC:             15,
			sessionDifficultyMod: -2,
			expectedDC:         13,
		},
		{
			name:               "hard difficulty, base DC 15",
			baseDC:             15,
			sessionDifficultyMod: 1,
			expectedDC:         16,
		},
		{
			name:               "very hard difficulty, base DC 15",
			baseDC:             15,
			sessionDifficultyMod: 2,
			expectedDC:         17,
		},
		{
			name:               "DC clamped to minimum 8",
			baseDC:             10,
			sessionDifficultyMod: -3,
			expectedDC:         8,
		},
		{
			name:               "DC clamped to maximum 20",
			baseDC:             18,
			sessionDifficultyMod: 3,
			expectedDC:         20,
		},
		{
			name:               "DC already at minimum",
			baseDC:             8,
			sessionDifficultyMod: -1,
			expectedDC:         8,
		},
		{
			name:               "DC already at maximum",
			baseDC:             20,
			sessionDifficultyMod: 1,
			expectedDC:         20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &GameSession{
				SessionDifficultyMod: tt.sessionDifficultyMod,
			}

			result := s.GetAdaptiveDC(tt.baseDC)
			if result != tt.expectedDC {
				t.Errorf("GetAdaptiveDC(%d) = %d, want %d", tt.baseDC, result, tt.expectedDC)
			}
		})
	}
}

func TestGameSession_RecordAbilityCheckResult(t *testing.T) {
	t.Run("first success", func(t *testing.T) {
		s := &GameSession{
			SessionSuccessCount: 0,
			SessionFailureCount: 0,
			SessionChecksCount:  0,
		}

		s.RecordAbilityCheckResult(true)

		if s.SessionSuccessCount != 1 {
			t.Errorf("SessionSuccessCount = %d, want 1", s.SessionSuccessCount)
		}
		if s.SessionFailureCount != 0 {
			t.Errorf("SessionFailureCount = %d, want 0", s.SessionFailureCount)
		}
		if s.SessionChecksCount != 1 {
			t.Errorf("SessionChecksCount = %d, want 1", s.SessionChecksCount)
		}
	})

	t.Run("first failure", func(t *testing.T) {
		s := &GameSession{
			SessionSuccessCount: 0,
			SessionFailureCount: 0,
			SessionChecksCount:  0,
		}

		s.RecordAbilityCheckResult(false)

		if s.SessionSuccessCount != 0 {
			t.Errorf("SessionSuccessCount = %d, want 0", s.SessionSuccessCount)
		}
		if s.SessionFailureCount != 1 {
			t.Errorf("SessionFailureCount = %d, want 1", s.SessionFailureCount)
		}
		if s.SessionChecksCount != 1 {
			t.Errorf("SessionChecksCount = %d, want 1", s.SessionChecksCount)
		}
	})

	t.Run("multiple results", func(t *testing.T) {
		s := &GameSession{
			SessionSuccessCount: 2,
			SessionFailureCount: 1,
			SessionChecksCount:  3,
		}

		s.RecordAbilityCheckResult(true)
		s.RecordAbilityCheckResult(false)

		if s.SessionSuccessCount != 3 {
			t.Errorf("SessionSuccessCount = %d, want 3", s.SessionSuccessCount)
		}
		if s.SessionFailureCount != 2 {
			t.Errorf("SessionFailureCount = %d, want 2", s.SessionFailureCount)
		}
		if s.SessionChecksCount != 5 {
			t.Errorf("SessionChecksCount = %d, want 5", s.SessionChecksCount)
		}
	})
}

func TestGameSession_updateDifficultyModifier(t *testing.T) {
	tests := []struct {
		name                string
		successCount        int
		failureCount        int
		checksCount         int
		expectedModifier    int
		description         string
	}{
		{
			name:             "insufficient data",
			successCount:     1,
			failureCount:     1,
			checksCount:      2,
			expectedModifier: 0,
			description:      "modifier should remain 0 with < 3 checks",
		},
		{
			name:             "very high success rate",
			successCount:     10,
			failureCount:     0,
			checksCount:      10,
			expectedModifier: 2,
			description:      ">90% success rate should increase difficulty significantly",
		},
		{
			name:             "high success rate",
			successCount:     9,
			failureCount:     1,
			checksCount:      10,
			expectedModifier: 1,
			description:      ">80% success rate should moderately increase difficulty",
		},
		{
			name:             "very low success rate",
			successCount:     1,
			failureCount:     9,
			checksCount:      10,
			expectedModifier: -2,
			description:      "<15% success rate should decrease difficulty significantly",
		},
		{
			name:             "low success rate",
			successCount:     2,
			failureCount:     8,
			checksCount:      10,
			expectedModifier: -1,
			description:      "<30% success rate should moderately decrease difficulty",
		},
		{
			name:             "balanced success rate",
			successCount:     5,
			failureCount:     5,
			checksCount:      10,
			expectedModifier: 0,
			description:      "50% success rate should keep difficulty normal",
		},
		{
			name:             "exactly 80 percent",
			successCount:     8,
			failureCount:     2,
			checksCount:      10,
			expectedModifier: 0,
			description:      "exactly 80% should not trigger modifier (needs >80%)",
		},
		{
			name:             "exactly 90 percent",
			successCount:     9,
			failureCount:     1,
			checksCount:      10,
			expectedModifier: 1,
			description:      "exactly 90% should trigger +1 modifier (needs >90% for +2)",
		},
		{
			name:             "exactly 15 percent",
			successCount:     3,
			failureCount:     17,
			checksCount:      20,
			expectedModifier: -1,
			description:      "exactly 15% should trigger -1 modifier (needs <15% for -2)",
		},
		{
			name:             "exactly 30 percent",
			successCount:     6,
			failureCount:     14,
			checksCount:      20,
			expectedModifier: 0,
			description:      "exactly 30% should not trigger modifier (needs <30%)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &GameSession{
				SessionSuccessCount: tt.successCount,
				SessionFailureCount: tt.failureCount,
				SessionChecksCount:  tt.checksCount,
			}

			s.updateDifficultyModifier()

			if s.SessionDifficultyMod != tt.expectedModifier {
				t.Errorf("updateDifficultyModifier() with %s: SessionDifficultyMod = %d, want %d",
					tt.description, s.SessionDifficultyMod, tt.expectedModifier)
			}
		})
	}
}

func TestGameSession_GetDifficultyDescription(t *testing.T) {
	tests := []struct {
		name             string
		checksCount      int
		difficultyMod    int
		expectedDesc     string
	}{
		{
			name:          "insufficient data",
			checksCount:   2,
			difficultyMod: 0,
			expectedDesc:  "адаптация в процессе",
		},
		{
			name:          "very easy",
			checksCount:   5,
			difficultyMod: -2,
			expectedDesc:  "очень легко",
		},
		{
			name:          "easy",
			checksCount:   5,
			difficultyMod: -1,
			expectedDesc:  "легко",
		},
		{
			name:          "normal",
			checksCount:   5,
			difficultyMod: 0,
			expectedDesc:  "нормально",
		},
		{
			name:          "hard",
			checksCount:   5,
			difficultyMod: 1,
			expectedDesc:  "сложно",
		},
		{
			name:          "very hard",
			checksCount:   5,
			difficultyMod: 2,
			expectedDesc:  "очень сложно",
		},
		{
			name:          "unknown modifier",
			checksCount:   5,
			difficultyMod: 99,
			expectedDesc:  "нормально",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &GameSession{
				SessionChecksCount:  tt.checksCount,
				SessionDifficultyMod: tt.difficultyMod,
			}

			result := s.GetDifficultyDescription()
			if result != tt.expectedDesc {
				t.Errorf("GetDifficultyDescription() = %q, want %q", result, tt.expectedDesc)
			}
		})
	}
}