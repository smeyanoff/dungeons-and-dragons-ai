package dice

import (
	"context"
	"regexp"
	"testing"
)

func TestRollDiceUseCase_Execute(t *testing.T) {
	tests := []struct {
		name          string
		expression    string
		expectedError bool
		validate      func(*testing.T, string)
	}{
		{
			name:       "roll d20",
			expression: "d20",
			validate: func(t *testing.T, result string) {
				if result == "" {
					t.Error("expected non-empty result")
				}
				// Проверяем, что результат содержит число
				matched, _ := regexp.MatchString(`\d+`, result)
				if !matched {
					t.Errorf("result should contain a number: %s", result)
				}
			},
		},
		{
			name:       "roll 2d6",
			expression: "2d6",
			validate: func(t *testing.T, result string) {
				if result == "" {
					t.Error("expected non-empty result")
				}
			},
		},
		{
			name:       "roll with modifier",
			expression: "d20+5",
			validate: func(t *testing.T, result string) {
				if result == "" {
					t.Error("expected non-empty result")
				}
			},
		},
		{
			name:       "empty expression defaults to d20",
			expression: "",
			validate: func(t *testing.T, result string) {
				if result == "" {
					t.Error("expected non-empty result")
				}
				// Должен быть d20
				if len(result) < 5 {
					t.Errorf("result too short: %s", result)
				}
			},
		},
		{
			name:          "invalid expression",
			expression:    "invalid",
			expectedError: true,
		},
		{
			name:          "negative dice",
			expression:    "-1d6",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewRollDiceUseCase()

			result, err := uc.Execute(context.Background(), tt.expression)

			if tt.expectedError {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if tt.validate != nil {
					tt.validate(t, result)
				}
			}
		})
	}
}
