package spell

import "testing"

func TestSpell_IsCantrip(t *testing.T) {
	tests := []struct {
		name     string
		spell    *Spell
		expected bool
	}{
		{
			name: "cantrip level 0",
			spell: &Spell{
				Level: 0,
			},
			expected: true,
		},
		{
			name: "level 1 spell",
			spell: &Spell{
				Level: 1,
			},
			expected: false,
		},
		{
			name: "level 5 spell",
			spell: &Spell{
				Level: 5,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.spell.IsCantrip()
			if result != tt.expected {
				t.Errorf("IsCantrip() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestSpell_IsAvailableForClass(t *testing.T) {
	tests := []struct {
		name     string
		spell    *Spell
		class    string
		expected bool
	}{
		{
			name: "wizard spell for wizard",
			spell: &Spell{
				AvailableForWizard: true,
			},
			class:    "wizard",
			expected: true,
		},
		{
			name: "wizard spell for cleric",
			spell: &Spell{
				AvailableForWizard: true,
			},
			class:    "cleric",
			expected: false,
		},
		{
			name: "cleric spell for cleric",
			spell: &Spell{
				AvailableForCleric: true,
			},
			class:    "cleric",
			expected: true,
		},
		{
			name: "ranger spell for ranger",
			spell: &Spell{
				AvailableForRanger: true,
			},
			class:    "ranger",
			expected: true,
		},
		{
			name: "multi-class spell",
			spell: &Spell{
				AvailableForWizard: true,
				AvailableForCleric: true,
			},
			class:    "wizard",
			expected: true,
		},
		{
			name: "multi-class spell for other class",
			spell: &Spell{
				AvailableForWizard: true,
				AvailableForCleric: true,
			},
			class:    "ranger",
			expected: false,
		},
		{
			name: "unknown class",
			spell: &Spell{
				AvailableForWizard: true,
			},
			class:    "fighter",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.spell.IsAvailableForClass(tt.class)
			if result != tt.expected {
				t.Errorf("IsAvailableForClass(%s) = %v, expected %v", tt.class, result, tt.expected)
			}
		})
	}
}

func TestSpellSlots_GetSlotsByLevel(t *testing.T) {
	slots := &SpellSlots{
		Level1: 2,
		Level2: 3,
		Level3: 4,
		Level4: 2,
		Level5: 1,
	}

	tests := []struct {
		name     string
		level    int
		expected int
	}{
		{"level 1", 1, 2},
		{"level 2", 2, 3},
		{"level 3", 3, 4},
		{"level 4", 4, 2},
		{"level 5", 5, 1},
		{"level 6", 6, 0},
		{"level 9", 9, 0},
		{"invalid level 0", 0, 0},
		{"invalid level 10", 10, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := slots.GetSlotsByLevel(tt.level)
			if result != tt.expected {
				t.Errorf("GetSlotsByLevel(%d) = %d, expected %d", tt.level, result, tt.expected)
			}
		})
	}
}

func TestSpellSlots_GetUsedSlotsByLevel(t *testing.T) {
	slots := &SpellSlots{
		UsedLevel1: 1,
		UsedLevel2: 2,
		UsedLevel3: 0,
	}

	tests := []struct {
		name     string
		level    int
		expected int
	}{
		{"level 1 used", 1, 1},
		{"level 2 used", 2, 2},
		{"level 3 used", 3, 0},
		{"level 4 used", 4, 0},
		{"invalid level", 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := slots.GetUsedSlotsByLevel(tt.level)
			if result != tt.expected {
				t.Errorf("GetUsedSlotsByLevel(%d) = %d, expected %d", tt.level, result, tt.expected)
			}
		})
	}
}

func TestSpellSlots_UseSpellSlot(t *testing.T) {
	tests := []struct {
		name         string
		slots        *SpellSlots
		level        int
		expected     bool
		expectedUsed int
	}{
		{
			name: "use slot when available",
			slots: &SpellSlots{
				Level1:     3,
				UsedLevel1: 1,
			},
			level:        1,
			expected:     true,
			expectedUsed: 2,
		},
		{
			name: "cannot use when all slots used",
			slots: &SpellSlots{
				Level1:     2,
				UsedLevel1: 2,
			},
			level:        1,
			expected:     false,
			expectedUsed: 2,
		},
		{
			name: "use last slot",
			slots: &SpellSlots{
				Level1:     1,
				UsedLevel1: 0,
			},
			level:        1,
			expected:     true,
			expectedUsed: 1,
		},
		{
			name: "invalid level",
			slots: &SpellSlots{
				Level1: 2,
			},
			level:        0,
			expected:     false,
			expectedUsed: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.slots.UseSpellSlot(tt.level)
			if result != tt.expected {
				t.Errorf("UseSpellSlot(%d) = %v, expected %v", tt.level, result, tt.expected)
			}
			used := tt.slots.GetUsedSlotsByLevel(tt.level)
			if used != tt.expectedUsed {
				t.Errorf("After UseSpellSlot, used slots = %d, expected %d", used, tt.expectedUsed)
			}
		})
	}
}

func TestSpellSlots_RestoreSpellSlots(t *testing.T) {
	slots := &SpellSlots{
		Level1:     2,
		Level2:     3,
		Level3:     4,
		UsedLevel1: 2,
		UsedLevel2: 3,
		UsedLevel3: 1,
		UsedLevel4: 1,
	}

	slots.RestoreSpellSlots()

	// All used slots should be 0
	for i := 1; i <= 9; i++ {
		used := slots.GetUsedSlotsByLevel(i)
		if used != 0 {
			t.Errorf("After RestoreSpellSlots, level %d used slots = %d, expected 0", i, used)
		}
	}

	// Max slots should remain unchanged
	if slots.Level1 != 2 || slots.Level2 != 3 || slots.Level3 != 4 {
		t.Error("RestoreSpellSlots should not change max slots")
	}
}

func TestCalculateSpellSlotsForLevel(t *testing.T) {
	tests := []struct {
		name           string
		class          string
		level          int
		expectSlots    bool
		expectedLevel1 int
	}{
		{"wizard level 1", "wizard", 1, true, 2},
		{"wizard level 3", "wizard", 3, true, 4},
		{"wizard level 5", "wizard", 5, true, 4},
		{"cleric level 1", "cleric", 1, true, 2},
		{"cleric level 3", "cleric", 3, true, 4},
		{"ranger level 1", "ranger", 1, false, 0}, // Ranger gets spells at level 2
		{"ranger level 2", "ranger", 2, true, 2},
		{"ranger level 5", "ranger", 5, true, 4},
		{"fighter level 5", "fighter", 5, false, 0}, // Non-caster class
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateSpellSlotsForLevel(tt.class, tt.level)
			if result == nil {
				t.Fatal("CalculateSpellSlotsForLevel returned nil")
			}

			level1Slots := result.GetSlotsByLevel(1)
			if tt.expectSlots && level1Slots != tt.expectedLevel1 {
				t.Errorf("Level 1 slots = %d, expected %d", level1Slots, tt.expectedLevel1)
			}
			if !tt.expectSlots && level1Slots != 0 {
				t.Errorf("Expected no slots, but got %d for level 1", level1Slots)
			}
		})
	}
}
