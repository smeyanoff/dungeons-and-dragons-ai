package session

import (
	"testing"
)

func TestGameSession_AddCompanion(t *testing.T) {
	s := &GameSession{
		Companions: []Companion{},
	}

	companion := &Companion{
		ID:          1,
		Name:        "Гарольд",
		Description: "Опытный воин из далёких земель",
		Class:       "Воин",
		Level:       3,
		HP:          25,
		MaxHP:       25,
		AC:          16,
		AttackBonus: 5,
		DamageDice:  "1d8+3",
	}

	s.AddCompanion(companion)

	if len(s.Companions) != 1 {
		t.Errorf("AddCompanion() length = %d, want 1", len(s.Companions))
	}

	if s.Companions[0].Name != "Гарольд" {
		t.Errorf("AddCompanion() name = %q, want %q", s.Companions[0].Name, "Гарольд")
	}

	if s.Companions[0].Class != "Воин" {
		t.Errorf("AddCompanion() class = %q, want %q", s.Companions[0].Class, "Воин")
	}
}

func TestGameSession_RemoveCompanion(t *testing.T) {
	s := &GameSession{
		Companions: []Companion{
			{ID: 1, Name: "Гарольд"},
			{ID: 2, Name: "Эльза"},
			{ID: 3, Name: "Торвальд"},
		},
	}

	t.Run("remove existing companion", func(t *testing.T) {
		removed := s.RemoveCompanion(2)

		if !removed {
			t.Error("RemoveCompanion() should return true for existing companion")
		}

		if len(s.Companions) != 2 {
			t.Errorf("RemoveCompanion() length = %d, want 2", len(s.Companions))
		}

		// Check that the correct companion was removed
		found := false
		for _, c := range s.Companions {
			if c.ID == 2 {
				found = true
				break
			}
		}
		if found {
			t.Error("RemoveCompanion() should remove companion with ID 2")
		}
	})

	t.Run("remove non-existing companion", func(t *testing.T) {
		removed := s.RemoveCompanion(99)

		if removed {
			t.Error("RemoveCompanion() should return false for non-existing companion")
		}

		if len(s.Companions) != 2 {
			t.Errorf("RemoveCompanion() length = %d, want 2", len(s.Companions))
		}
	})
}

func TestGameSession_GetCompanionByID(t *testing.T) {
	s := &GameSession{
		Companions: []Companion{
			{ID: 1, Name: "Гарольд", Class: "Воин"},
			{ID: 2, Name: "Эльза", Class: "Маг"},
			{ID: 3, Name: "Торвальд", Class: "Разбойник"},
		},
	}

	t.Run("get existing companion", func(t *testing.T) {
		companion := s.GetCompanionByID(2)

		if companion == nil {
			t.Fatal("GetCompanionByID() should return companion")
		}

		if companion.ID != 2 {
			t.Errorf("GetCompanionByID() ID = %d, want 2", companion.ID)
		}

		if companion.Name != "Эльза" {
			t.Errorf("GetCompanionByID() name = %q, want %q", companion.Name, "Эльза")
		}

		if companion.Class != "Маг" {
			t.Errorf("GetCompanionByID() class = %q, want %q", companion.Class, "Маг")
		}
	})

	t.Run("get non-existing companion", func(t *testing.T) {
		companion := s.GetCompanionByID(99)

		if companion != nil {
			t.Error("GetCompanionByID() should return nil for non-existing companion")
		}
	})
}

func TestGameSession_AddCompanion_MultipleCompanions(t *testing.T) {
	s := &GameSession{
		Companions: []Companion{},
	}

	companions := []*Companion{
		{ID: 1, Name: "Гарольд", Class: "Воин"},
		{ID: 2, Name: "Эльза", Class: "Маг"},
		{ID: 3, Name: "Торвальд", Class: "Разбойник"},
	}

	for _, c := range companions {
		s.AddCompanion(c)
	}

	if len(s.Companions) != 3 {
		t.Errorf("AddCompanion() length = %d, want 3", len(s.Companions))
	}

	// Check all companions are present
	for i, expected := range companions {
		actual := s.Companions[i]
		if actual.ID != expected.ID {
			t.Errorf("Companion %d ID = %d, want %d", i, actual.ID, expected.ID)
		}
		if actual.Name != expected.Name {
			t.Errorf("Companion %d name = %q, want %q", i, actual.Name, expected.Name)
		}
		if actual.Class != expected.Class {
			t.Errorf("Companion %d class = %q, want %q", i, actual.Class, expected.Class)
		}
	}
}

func TestGameSession_CompanionOperations_ComplexScenario(t *testing.T) {
	s := &GameSession{
		Companions: []Companion{},
	}

	// Add initial companions
	s.AddCompanion(&Companion{ID: 1, Name: "Гарольд", Class: "Воин", Level: 1})
	s.AddCompanion(&Companion{ID: 2, Name: "Эльза", Class: "Маг", Level: 2})
	s.AddCompanion(&Companion{ID: 3, Name: "Торвальд", Class: "Разбойник", Level: 1})

	// Verify all added
	if len(s.Companions) != 3 {
		t.Errorf("Initial companions count = %d, want 3", len(s.Companions))
	}

	// Get specific companion
	companion := s.GetCompanionByID(2)
	if companion == nil || companion.Name != "Эльза" {
		t.Error("GetCompanionByID() failed to retrieve correct companion")
	}

	// Remove a companion
	removed := s.RemoveCompanion(1)
	if !removed {
		t.Error("RemoveCompanion() should succeed for existing companion")
	}

	if len(s.Companions) != 2 {
		t.Errorf("After removal companions count = %d, want 2", len(s.Companions))
	}

	// Try to get removed companion
	removedCompanion := s.GetCompanionByID(1)
	if removedCompanion != nil {
		t.Error("GetCompanionByID() should return nil for removed companion")
	}

	// Add another companion
	s.AddCompanion(&Companion{ID: 4, Name: "Лира", Class: "Бард", Level: 3})

	if len(s.Companions) != 3 {
		t.Errorf("After adding new companion count = %d, want 3", len(s.Companions))
	}

	// Final verification
	finalCompanion := s.GetCompanionByID(4)
	if finalCompanion == nil || finalCompanion.Name != "Лира" {
		t.Error("Final companion retrieval failed")
	}
}