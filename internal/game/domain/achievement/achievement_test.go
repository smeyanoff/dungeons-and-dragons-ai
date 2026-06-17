package achievement

import "testing"

func TestAchievement_IsCompleted(t *testing.T) {
	tests := []struct {
		name         string
		achievement  *Achievement
		currentValue int
		expected     bool
	}{
		{
			name: "value equals requirement",
			achievement: &Achievement{
				RequirementValue: 10,
			},
			currentValue: 10,
			expected:     true,
		},
		{
			name: "value exceeds requirement",
			achievement: &Achievement{
				RequirementValue: 10,
			},
			currentValue: 15,
			expected:     true,
		},
		{
			name: "value less than requirement",
			achievement: &Achievement{
				RequirementValue: 10,
			},
			currentValue: 5,
			expected:     false,
		},
		{
			name: "zero requirement always completed",
			achievement: &Achievement{
				RequirementValue: 0,
			},
			currentValue: 0,
			expected:     true,
		},
		{
			name: "zero requirement with positive value",
			achievement: &Achievement{
				RequirementValue: 0,
			},
			currentValue: 5,
			expected:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.achievement.IsCompleted(tt.currentValue)
			if result != tt.expected {
				t.Errorf("IsCompleted(%d) = %v, expected %v", tt.currentValue, result, tt.expected)
			}
		})
	}
}

func TestAchievement_GetProgressPercentage(t *testing.T) {
	tests := []struct {
		name         string
		achievement  *Achievement
		currentValue int
		expected     int
	}{
		{
			name: "zero progress",
			achievement: &Achievement{
				RequirementValue: 10,
			},
			currentValue: 0,
			expected:     0,
		},
		{
			name: "half progress",
			achievement: &Achievement{
				RequirementValue: 10,
			},
			currentValue: 5,
			expected:     50,
		},
		{
			name: "full progress",
			achievement: &Achievement{
				RequirementValue: 10,
			},
			currentValue: 10,
			expected:     100,
		},
		{
			name: "over completion capped at 100",
			achievement: &Achievement{
				RequirementValue: 10,
			},
			currentValue: 15,
			expected:     100,
		},
		{
			name: "zero requirement returns 100",
			achievement: &Achievement{
				RequirementValue: 0,
			},
			currentValue: 0,
			expected:     100,
		},
		{
			name: "zero requirement with value returns 100",
			achievement: &Achievement{
				RequirementValue: 0,
			},
			currentValue: 5,
			expected:     100,
		},
		{
			name: "partial progress 33%",
			achievement: &Achievement{
				RequirementValue: 3,
			},
			currentValue: 1,
			expected:     33,
		},
		{
			name: "partial progress 66%",
			achievement: &Achievement{
				RequirementValue: 3,
			},
			currentValue: 2,
			expected:     66,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.achievement.GetProgressPercentage(tt.currentValue)
			if result != tt.expected {
				t.Errorf("GetProgressPercentage(%d) = %d%%, expected %d%%", tt.currentValue, result, tt.expected)
			}
		})
	}
}
