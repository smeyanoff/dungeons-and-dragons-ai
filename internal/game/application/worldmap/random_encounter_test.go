package worldmap

import (
	"context"
	"math/rand"
	"strings"
	"testing"

	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/game/domain/world"
)

type mockWorldEventRepo struct {
	byLocation map[uint][]world.WorldEvent
	getErr     error
	saved      []*world.WorldEvent
}

func (m *mockWorldEventRepo) GetByLocationID(ctx context.Context, locationID uint) ([]world.WorldEvent, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.byLocation[locationID], nil
}

func (m *mockWorldEventRepo) Save(ctx context.Context, e *world.WorldEvent) error {
	m.saved = append(m.saved, e)
	return nil
}

func TestRandomEncounterChance(t *testing.T) {
	tests := []struct {
		name        string
		travelHours int
		want        int
	}{
		{"instant travel has no chance", 0, 0},
		{"negative travel time treated as instant", -1, 0},
		{"one hour", 1, 20},
		{"two hours", 2, 25},
		{"nine hours capped", 9, randomEncounterMaxChancePercent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := randomEncounterChance(tt.travelHours); got != tt.want {
				t.Fatalf("randomEncounterChance(%d) = %d, want %d", tt.travelHours, got, tt.want)
			}
		})
	}
}

func TestMaybeGenerateRandomEncounter(t *testing.T) {
	from := &world.Location{ID: 1, Name: "Town"}
	to := &world.Location{ID: 2, Name: "Forest"}
	gs := &session.GameSession{World: world.World{ID: 1}}

	t.Run("no worldEventRepo -> no encounter", func(t *testing.T) {
		uc := &MoveToLocationUseCase{}
		got := uc.maybeGenerateRandomEncounter(context.Background(), gs, from, to, 3)
		if got != "" {
			t.Fatalf("expected no encounter, got %q", got)
		}
	})

	t.Run("instant travel -> no encounter regardless of roll", func(t *testing.T) {
		orig := rng
		rng = rand.New(rand.NewSource(1))
		defer func() { rng = orig }()

		repo := &mockWorldEventRepo{}
		uc := &MoveToLocationUseCase{worldEventRepo: repo}
		got := uc.maybeGenerateRandomEncounter(context.Background(), gs, from, to, 0)
		if got != "" {
			t.Fatalf("expected no encounter for instant travel, got %q", got)
		}
		if len(repo.saved) != 0 {
			t.Fatalf("expected no saved events, got %d", len(repo.saved))
		}
	})

	t.Run("forced favorable roll -> encounter saved and appended", func(t *testing.T) {
		orig := rng
		rng = rand.New(rand.NewSource(1))
		defer func() { rng = orig }()

		// Найдём seed, дающий Intn(100) == 0, чтобы бросок гарантированно прошёл
		// при любом положительном шансе. Перебираем сиды, а не полагаемся на
		// конкретное значение реализации rand.
		var seed int64
		for s := int64(0); s < 1000; s++ {
			r := rand.New(rand.NewSource(s))
			if r.Intn(100) == 0 {
				seed = s
				break
			}
		}
		rng = rand.New(rand.NewSource(seed))

		repo := &mockWorldEventRepo{}
		uc := &MoveToLocationUseCase{worldEventRepo: repo}
		got := uc.maybeGenerateRandomEncounter(context.Background(), gs, from, to, 3)
		if got == "" {
			t.Fatalf("expected encounter text, got empty string")
		}
		if !strings.Contains(got, "Town") || !strings.Contains(got, "Forest") {
			t.Fatalf("expected encounter text to mention both locations, got: %s", got)
		}
		if len(repo.saved) != 1 {
			t.Fatalf("expected 1 saved event, got %d", len(repo.saved))
		}
		saved := repo.saved[0]
		if saved.Type != world.WorldEventTypeRandomEncounter {
			t.Fatalf("expected WorldEventTypeRandomEncounter, got %s", saved.Type)
		}
		if saved.Status != world.WorldEventStatusActive {
			t.Fatalf("expected active status, got %s", saved.Status)
		}
		if saved.RequiredLocationID == nil || *saved.RequiredLocationID != to.ID {
			t.Fatalf("expected RequiredLocationID=%d, got %+v", to.ID, saved.RequiredLocationID)
		}
	})

	t.Run("forced unfavorable roll -> no encounter", func(t *testing.T) {
		orig := rng
		rng = rand.New(rand.NewSource(1))
		defer func() { rng = orig }()

		var seed int64
		for s := int64(0); s < 1000; s++ {
			r := rand.New(rand.NewSource(s))
			if r.Intn(100) >= randomEncounterMaxChancePercent {
				seed = s
				break
			}
		}
		rng = rand.New(rand.NewSource(seed))

		repo := &mockWorldEventRepo{}
		uc := &MoveToLocationUseCase{worldEventRepo: repo}
		got := uc.maybeGenerateRandomEncounter(context.Background(), gs, from, to, 3)
		if got != "" {
			t.Fatalf("expected no encounter, got %q", got)
		}
	})

	t.Run("existing active location event blocks new encounter", func(t *testing.T) {
		orig := rng
		rng = rand.New(rand.NewSource(1))
		defer func() { rng = orig }()

		var seed int64
		for s := int64(0); s < 1000; s++ {
			r := rand.New(rand.NewSource(s))
			if r.Intn(100) == 0 {
				seed = s
				break
			}
		}
		rng = rand.New(rand.NewSource(seed))

		repo := &mockWorldEventRepo{
			byLocation: map[uint][]world.WorldEvent{
				to.ID: {
					{
						Type:   world.WorldEventTypeLocationNPC,
						Status: world.WorldEventStatusActive,
					},
				},
			},
		}
		uc := &MoveToLocationUseCase{worldEventRepo: repo}
		got := uc.maybeGenerateRandomEncounter(context.Background(), gs, from, to, 3)
		if got != "" {
			t.Fatalf("expected no encounter when active location event exists, got %q", got)
		}
		if len(repo.saved) != 0 {
			t.Fatalf("expected no saved events, got %d", len(repo.saved))
		}
	})
}

func TestMoveToLocationUseCase_Execute_WithRandomEncounter(t *testing.T) {
	orig := rng
	rng = rand.New(rand.NewSource(1))
	defer func() { rng = orig }()

	var seed int64
	for s := int64(0); s < 1000; s++ {
		r := rand.New(rand.NewSource(s))
		if r.Intn(100) == 0 {
			seed = s
			break
		}
	}
	rng = rand.New(rand.NewSource(seed))

	loc1 := world.Location{
		ID:   1,
		Name: "Town",
		Connections: []world.LocationConnection{
			{FromLocationID: 1, ToLocationID: 2, Direction: "north", Description: "дорога"},
		},
	}
	loc2 := world.Location{ID: 2, Name: "Forest"}

	repo := &mockWorldEventRepo{}
	sessionRepo := &mockSessionRepoMove{session: &session.GameSession{
		ChatID: 123,
		State:  session.StateActive,
		World: world.World{
			ID:        1,
			Locations: []world.Location{loc1, loc2},
		},
		CurrentLocationID: uintPtr(1),
	}}

	uc := NewMoveToLocationUseCase(nil, sessionRepo, repo, nil, nil, nil)
	resp, err := uc.Execute(context.Background(), MoveToLocationRequest{ChatID: 123, ToLocationID: uintPtr(2)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(resp.Message, "⚔️") {
		t.Fatalf("expected encounter marker in message, got: %s", resp.Message)
	}
	if len(repo.saved) != 1 {
		t.Fatalf("expected 1 saved world event, got %d", len(repo.saved))
	}
}
