package world

import (
	"encoding/json"
	"testing"
)

func TestLocation_PredefinedChecks(t *testing.T) {
	tests := []struct {
		name           string
		jsonData       json.RawMessage
		expectedCount  int
		expectedChecks []PredefinedCheck
	}{
		{
			name:          "empty JSON returns empty slice",
			jsonData:      nil,
			expectedCount: 0,
		},
		{
			name:          "empty array returns empty slice",
			jsonData:      json.RawMessage("[]"),
			expectedCount: 0,
		},
		{
			name:          "single predefined check",
			jsonData:      json.RawMessage(`[{"ability":"strength","dc":15,"description":"Поднять камень","location_hint":"У входа"}]`),
			expectedCount: 1,
			expectedChecks: []PredefinedCheck{
				{
					Ability:      "strength",
					DC:           15,
					Description:  "Поднять камень",
					LocationHint: "У входа",
				},
			},
		},
		{
			name:          "multiple predefined checks",
			jsonData:      json.RawMessage(`[{"ability":"wisdom","dc":12,"description":"Заметить скрытую дверь","location_hint":"Северная стена"},{"ability":"dexterity","dc":15,"description":"Пройти по узкому мосту","location_hint":"Над пропастью"}]`),
			expectedCount: 2,
			expectedChecks: []PredefinedCheck{
				{
					Ability:      "wisdom",
					DC:           12,
					Description:  "Заметить скрытую дверь",
					LocationHint: "Северная стена",
				},
				{
					Ability:      "dexterity",
					DC:           15,
					Description:  "Пройти по узкому мосту",
					LocationHint: "Над пропастью",
				},
			},
		},
		{
			name:          "invalid JSON returns empty slice",
			jsonData:      json.RawMessage(`invalid json`),
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc := &Location{
				PredefinedChecksJSON: tt.jsonData,
			}

			checks := loc.PredefinedChecks()

			if len(checks) != tt.expectedCount {
				t.Errorf("PredefinedChecks() returned %d checks, expected %d", len(checks), tt.expectedCount)
			}

			if len(tt.expectedChecks) > 0 {
				if len(checks) != len(tt.expectedChecks) {
					t.Fatalf("PredefinedChecks() returned %d checks, expected %d", len(checks), len(tt.expectedChecks))
				}

				for i, expected := range tt.expectedChecks {
					if checks[i].Ability != expected.Ability {
						t.Errorf("PredefinedChecks()[%d].Ability = %s, expected %s", i, checks[i].Ability, expected.Ability)
					}
					if checks[i].DC != expected.DC {
						t.Errorf("PredefinedChecks()[%d].DC = %d, expected %d", i, checks[i].DC, expected.DC)
					}
					if checks[i].Description != expected.Description {
						t.Errorf("PredefinedChecks()[%d].Description = %s, expected %s", i, checks[i].Description, expected.Description)
					}
					if checks[i].LocationHint != expected.LocationHint {
						t.Errorf("PredefinedChecks()[%d].LocationHint = %s, expected %s", i, checks[i].LocationHint, expected.LocationHint)
					}
				}
			}
		})
	}
}

func TestLocation_SetPredefinedChecks(t *testing.T) {
	tests := []struct {
		name          string
		checks        []PredefinedCheck
		expectedError bool
		validateJSON  func(*testing.T, json.RawMessage)
	}{
		{
			name:          "empty slice sets nil JSON",
			checks:        []PredefinedCheck{},
			expectedError: false,
			validateJSON: func(t *testing.T, jsonData json.RawMessage) {
				if jsonData != nil {
					t.Errorf("SetPredefinedChecks() with empty slice should set JSON to nil, got %v", jsonData)
				}
			},
		},
		{
			name: "single check sets valid JSON",
			checks: []PredefinedCheck{
				{
					Ability:      "strength",
					DC:           15,
					Description:  "Поднять камень",
					LocationHint: "У входа",
				},
			},
			expectedError: false,
			validateJSON: func(t *testing.T, jsonData json.RawMessage) {
				if len(jsonData) == 0 {
					t.Error("SetPredefinedChecks() should set non-empty JSON")
				}
				var checks []PredefinedCheck
				if err := json.Unmarshal(jsonData, &checks); err != nil {
					t.Errorf("SetPredefinedChecks() created invalid JSON: %v", err)
				}
				if len(checks) != 1 {
					t.Errorf("SetPredefinedChecks() JSON should contain 1 check, got %d", len(checks))
				}
				if checks[0].Ability != "strength" {
					t.Errorf("SetPredefinedChecks() JSON check ability = %s, expected strength", checks[0].Ability)
				}
			},
		},
		{
			name: "multiple checks sets valid JSON",
			checks: []PredefinedCheck{
				{
					Ability:      "wisdom",
					DC:           12,
					Description:  "Заметить скрытую дверь",
					LocationHint: "Северная стена",
				},
				{
					Ability:      "dexterity",
					DC:           15,
					Description:  "Пройти по узкому мосту",
					LocationHint: "Над пропастью",
				},
			},
			expectedError: false,
			validateJSON: func(t *testing.T, jsonData json.RawMessage) {
				if len(jsonData) == 0 {
					t.Error("SetPredefinedChecks() should set non-empty JSON")
				}
				var checks []PredefinedCheck
				if err := json.Unmarshal(jsonData, &checks); err != nil {
					t.Errorf("SetPredefinedChecks() created invalid JSON: %v", err)
				}
				if len(checks) != 2 {
					t.Errorf("SetPredefinedChecks() JSON should contain 2 checks, got %d", len(checks))
				}
			},
		},
		{
			name: "check with all fields",
			checks: []PredefinedCheck{
				{
					Ability:      "intelligence",
					DC:           18,
					Description:  "Разгадать загадку",
					LocationHint: "На стене",
				},
			},
			expectedError: false,
			validateJSON: func(t *testing.T, jsonData json.RawMessage) {
				var checks []PredefinedCheck
				if err := json.Unmarshal(jsonData, &checks); err != nil {
					t.Errorf("SetPredefinedChecks() created invalid JSON: %v", err)
				}
				if checks[0].Ability != "intelligence" || checks[0].DC != 18 {
					t.Error("SetPredefinedChecks() did not preserve all fields correctly")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc := &Location{}

			err := loc.SetPredefinedChecks(tt.checks)

			if tt.expectedError {
				if err == nil {
					t.Error("SetPredefinedChecks() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("SetPredefinedChecks() unexpected error: %v", err)
				}
				if tt.validateJSON != nil {
					tt.validateJSON(t, loc.PredefinedChecksJSON)
				}
			}
		})
	}
}

func TestLocation_SetAndGetPredefinedChecks_RoundTrip(t *testing.T) {
	originalChecks := []PredefinedCheck{
		{
			Ability:      "charisma",
			DC:           14,
			Description:  "Убедить стражника",
			LocationHint: "У ворот",
		},
		{
			Ability:      "constitution",
			DC:           13,
			Description:  "Выдержать яд",
			LocationHint: "В подвале",
		},
	}

	loc := &Location{}

	// Устанавливаем проверки
	if err := loc.SetPredefinedChecks(originalChecks); err != nil {
		t.Fatalf("SetPredefinedChecks() error = %v", err)
	}

	// Получаем проверки обратно
	retrievedChecks := loc.PredefinedChecks()

	if len(retrievedChecks) != len(originalChecks) {
		t.Fatalf("Round trip: got %d checks, expected %d", len(retrievedChecks), len(originalChecks))
	}

	for i, original := range originalChecks {
		if retrievedChecks[i].Ability != original.Ability {
			t.Errorf("Round trip: check[%d].Ability = %s, expected %s", i, retrievedChecks[i].Ability, original.Ability)
		}
		if retrievedChecks[i].DC != original.DC {
			t.Errorf("Round trip: check[%d].DC = %d, expected %d", i, retrievedChecks[i].DC, original.DC)
		}
		if retrievedChecks[i].Description != original.Description {
			t.Errorf("Round trip: check[%d].Description = %s, expected %s", i, retrievedChecks[i].Description, original.Description)
		}
		if retrievedChecks[i].LocationHint != original.LocationHint {
			t.Errorf("Round trip: check[%d].LocationHint = %s, expected %s", i, retrievedChecks[i].LocationHint, original.LocationHint)
		}
	}
}
