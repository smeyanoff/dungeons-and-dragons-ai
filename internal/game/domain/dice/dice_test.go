package dice

import (
	"testing"
)

func TestRoll(t *testing.T) {
	tests := []struct {
		name        string
		expression  string
		expectError bool
		validate    func(*testing.T, *RollResult)
	}{
		{
			name:        "simple d20",
			expression:  "d20",
			expectError: false,
			validate: func(t *testing.T, r *RollResult) {
				if len(r.Rolls) != 1 {
					t.Errorf("expected 1 roll, got %d", len(r.Rolls))
				}
				if r.Rolls[0] < 1 || r.Rolls[0] > 20 {
					t.Errorf("expected roll between 1-20, got %d", r.Rolls[0])
				}
				if r.Total != r.Rolls[0] {
					t.Errorf("expected total to equal roll, got total=%d, roll=%d", r.Total, r.Rolls[0])
				}
			},
		},
		{
			name:        "multiple dice 2d6",
			expression:  "2d6",
			expectError: false,
			validate: func(t *testing.T, r *RollResult) {
				if len(r.Rolls) != 2 {
					t.Errorf("expected 2 rolls, got %d", len(r.Rolls))
				}
				for i, roll := range r.Rolls {
					if roll < 1 || roll > 6 {
						t.Errorf("roll %d: expected 1-6, got %d", i, roll)
					}
				}
				expectedTotal := r.Rolls[0] + r.Rolls[1]
				if r.Total != expectedTotal {
					t.Errorf("expected total %d, got %d", expectedTotal, r.Total)
				}
			},
		},
		{
			name:        "dice with modifier 1d8+3",
			expression:  "1d8+3",
			expectError: false,
			validate: func(t *testing.T, r *RollResult) {
				if len(r.Rolls) != 1 {
					t.Errorf("expected 1 roll, got %d", len(r.Rolls))
				}
				if r.Rolls[0] < 1 || r.Rolls[0] > 8 {
					t.Errorf("expected roll 1-8, got %d", r.Rolls[0])
				}
				if r.Modifier != 3 {
					t.Errorf("expected modifier 3, got %d", r.Modifier)
				}
				expectedTotal := r.Rolls[0] + 3
				if r.Total != expectedTotal {
					t.Errorf("expected total %d, got %d", expectedTotal, r.Total)
				}
			},
		},
		{
			name:        "dice with negative modifier 2d6-1",
			expression:  "2d6-1",
			expectError: false,
			validate: func(t *testing.T, r *RollResult) {
				if len(r.Rolls) != 2 {
					t.Errorf("expected 2 rolls, got %d", len(r.Rolls))
				}
				if r.Modifier != -1 {
					t.Errorf("expected modifier -1, got %d", r.Modifier)
				}
				expectedTotal := r.Rolls[0] + r.Rolls[1] - 1
				if r.Total != expectedTotal {
					t.Errorf("expected total %d, got %d", expectedTotal, r.Total)
				}
			},
		},
		{
			name:        "invalid expression",
			expression:  "invalid",
			expectError: true,
			validate:    nil,
		},
		{
			name:        "empty expression",
			expression:  "",
			expectError: true,
			validate:    nil,
		},
		{
			name:        "zero dice",
			expression:  "0d6",
			expectError: true,
			validate:    nil,
		},
		{
			name:        "zero sides",
			expression:  "1d0",
			expectError: true,
			validate:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Roll(tt.expression)
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				if result != nil {
					t.Errorf("expected nil result on error, got %+v", result)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result == nil {
				t.Errorf("expected result, got nil")
				return
			}

			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

func TestRollAttack(t *testing.T) {
	modifier := 5
	result := RollAttack(modifier)

	if len(result.Rolls) != 1 {
		t.Errorf("expected 1 roll, got %d", len(result.Rolls))
	}

	if result.Rolls[0] < 1 || result.Rolls[0] > 20 {
		t.Errorf("expected d20 roll 1-20, got %d", result.Rolls[0])
	}

	expectedTotal := result.Rolls[0] + modifier
	if result.Total != expectedTotal {
		t.Errorf("expected total %d, got %d", expectedTotal, result.Total)
	}

	if result.Modifier != modifier {
		t.Errorf("expected modifier %d, got %d", modifier, result.Modifier)
	}
}

func TestCalculateModifier(t *testing.T) {
	tests := []struct {
		stat     int
		expected int
	}{
		{10, 0}, // Среднее значение
		{12, 1}, // +1
		{14, 2}, // +2
		{16, 3}, // +3
		{18, 4}, // +4
		{20, 5}, // +5
		{8, -1}, // -1
		{6, -2}, // -2
		{4, -3}, // -3
		{1, -4}, // -4
		{0, -5}, // -5
		{11, 0}, // 11 = +0
		{13, 1}, // 13 = +1
		{15, 2}, // 15 = +2
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := CalculateModifier(tt.stat)
			if result != tt.expected {
				t.Errorf("CalculateModifier(%d) = %d, expected %d", tt.stat, result, tt.expected)
			}
		})
	}
}

func TestRollWithModifier(t *testing.T) {
	baseExpression := "2d6"
	additionalModifier := 3

	result, err := RollWithModifier(baseExpression, additionalModifier)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Rolls) != 2 {
		t.Errorf("expected 2 rolls, got %d", len(result.Rolls))
	}

	baseTotal := result.Rolls[0] + result.Rolls[1]
	expectedTotal := baseTotal + additionalModifier
	if result.Total != expectedTotal {
		t.Errorf("expected total %d, got %d", expectedTotal, result.Total)
	}
}
