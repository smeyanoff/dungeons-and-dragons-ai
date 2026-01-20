package location_event

import (
	"context"
	"errors"
	"math/rand"
	"testing"
	"time"

	"dungeons-and-dragons-ai/internal/game/domain/world"
)

type mockLocationEventRepo struct {
	getByLocationIDFunc func(ctx context.Context, locationID uint) ([]world.WorldEvent, error)
	saveFunc            func(ctx context.Context, e *world.WorldEvent) error

	saved []*world.WorldEvent
}

func (m *mockLocationEventRepo) GetByLocationID(ctx context.Context, locationID uint) ([]world.WorldEvent, error) {
	if m.getByLocationIDFunc != nil {
		return m.getByLocationIDFunc(ctx, locationID)
	}
	return nil, nil
}

func (m *mockLocationEventRepo) Save(ctx context.Context, e *world.WorldEvent) error {
	if m.saveFunc != nil {
		return m.saveFunc(ctx, e)
	}
	m.saved = append(m.saved, e)
	return nil
}

func TestLocationEventGenerator_Execute(t *testing.T) {
	// Make randomness deterministic for this test file.
	orig := rng
	rng = rand.New(rand.NewSource(1))
	t.Cleanup(func() { rng = orig })

	tests := []struct {
		name          string
		req           GenerateLocationEventRequest
		setupRepo     func(*mockLocationEventRepo)
		wantRespNil   bool
		wantErr       bool
		wantSaveCalls int
	}{
		{
			name: "not first visit - no event generated",
			req: GenerateLocationEventRequest{
				WorldID:      1,
				LocationID:   10,
				LocationName: "Tavern",
				IsFirstVisit: false,
			},
			setupRepo:     func(r *mockLocationEventRepo) {},
			wantRespNil:   true,
			wantErr:       false,
			wantSaveCalls: 0,
		},
		{
			name: "existing event already present - no event generated",
			req: GenerateLocationEventRequest{
				WorldID:      1,
				LocationID:   10,
				LocationName: "Tavern",
				IsFirstVisit: true,
			},
			setupRepo: func(r *mockLocationEventRepo) {
				r.getByLocationIDFunc = func(ctx context.Context, locationID uint) ([]world.WorldEvent, error) {
					return []world.WorldEvent{{ID: 123, WorldID: 1, RequiredLocationID: &locationID}}, nil
				}
			},
			wantRespNil:   true,
			wantErr:       false,
			wantSaveCalls: 0,
		},
		{
			name: "repo get error",
			req: GenerateLocationEventRequest{
				WorldID:      1,
				LocationID:   10,
				LocationName: "Tavern",
				IsFirstVisit: true,
			},
			setupRepo: func(r *mockLocationEventRepo) {
				r.getByLocationIDFunc = func(ctx context.Context, locationID uint) ([]world.WorldEvent, error) {
					return nil, errors.New("db down")
				}
			},
			wantRespNil:   true,
			wantErr:       true,
			wantSaveCalls: 0,
		},
		{
			name: "save error is returned",
			req: GenerateLocationEventRequest{
				WorldID:      1,
				LocationID:   10,
				LocationName: "Tavern",
				IsFirstVisit: true,
			},
			setupRepo: func(r *mockLocationEventRepo) {
				r.saveFunc = func(ctx context.Context, e *world.WorldEvent) error {
					return errors.New("write failed")
				}
			},
			wantRespNil:   true,
			wantErr:       true,
			wantSaveCalls: 0,
		},
		{
			name: "success - event generated and saved",
			req: GenerateLocationEventRequest{
				WorldID:      1,
				LocationID:   10,
				LocationName: "Tavern",
				IsFirstVisit: true,
			},
			setupRepo:     func(r *mockLocationEventRepo) {},
			wantRespNil:   false,
			wantErr:       false,
			wantSaveCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockLocationEventRepo{}
			if tt.setupRepo != nil {
				tt.setupRepo(repo)
			}
			gen := NewLocationEventGenerator(repo)

			resp, err := gen.Execute(context.Background(), tt.req)

			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantRespNil {
				if resp != nil {
					t.Fatalf("expected nil response, got: %+v", resp)
				}
			} else {
				if resp == nil || resp.Event == nil {
					t.Fatalf("expected non-nil response with event")
				}
				if resp.Description == "" {
					t.Fatalf("expected non-empty description")
				}

				ev := resp.Event
				if ev.WorldID != tt.req.WorldID {
					t.Fatalf("expected WorldID=%d, got %d", tt.req.WorldID, ev.WorldID)
				}
				if ev.RequiredLocationID == nil || *ev.RequiredLocationID != tt.req.LocationID {
					t.Fatalf("expected RequiredLocationID=%d, got %+v", tt.req.LocationID, ev.RequiredLocationID)
				}
				if ev.Status != world.WorldEventStatusActive {
					t.Fatalf("expected Status=%s, got %s", world.WorldEventStatusActive, ev.Status)
				}
				if ev.Type != world.WorldEventTypeLocationNPC &&
					ev.Type != world.WorldEventTypeLocationItem &&
					ev.Type != world.WorldEventTypeLocationTrap &&
					ev.Type != world.WorldEventTypeLocationPuzzle &&
					ev.Type != world.WorldEventTypeLocationEncounter {
					t.Fatalf("unexpected event type: %s", ev.Type)
				}
				if ev.Name == "" || ev.Description == "" {
					t.Fatalf("expected non-empty name/description")
				}
				if ev.CreatedAt.IsZero() || ev.UpdatedAt.IsZero() {
					t.Fatalf("expected timestamps to be set")
				}
				if ev.ActivatedAt == nil || ev.ActivatedAt.IsZero() {
					t.Fatalf("expected ActivatedAt to be set")
				}
			}

			if tt.wantSaveCalls != len(repo.saved) {
				t.Fatalf("expected %d save calls, got %d", tt.wantSaveCalls, len(repo.saved))
			}
		})
	}
}

func TestLocationEventGenerator_createLocationEvent_Mapping(t *testing.T) {
	gen := NewLocationEventGenerator(&mockLocationEventRepo{})
	req := GenerateLocationEventRequest{
		WorldID:      7,
		LocationID:   42,
		LocationName: "Ancient Ruins",
		IsFirstVisit: true,
	}

	tests := []struct {
		name      string
		eventType LocationEventType
		wantType  world.WorldEventType
	}{
		{"npc", EventTypeNPC, world.WorldEventTypeLocationNPC},
		{"item", EventTypeItem, world.WorldEventTypeLocationItem},
		{"trap", EventTypeTrap, world.WorldEventTypeLocationTrap},
		{"puzzle", EventTypePuzzle, world.WorldEventTypeLocationPuzzle},
		{"encounter", EventTypeEncounter, world.WorldEventTypeLocationEncounter},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev, desc := gen.createLocationEvent(req, tt.eventType)
			if ev == nil {
				t.Fatalf("expected event")
			}
			if ev.Type != tt.wantType {
				t.Fatalf("expected type %s, got %s", tt.wantType, ev.Type)
			}
			if ev.RequiredLocationID == nil || *ev.RequiredLocationID != req.LocationID {
				t.Fatalf("expected RequiredLocationID=%d", req.LocationID)
			}
			if ev.WorldID != req.WorldID {
				t.Fatalf("expected WorldID=%d, got %d", req.WorldID, ev.WorldID)
			}
			if ev.Status != world.WorldEventStatusActive {
				t.Fatalf("expected status active, got %s", ev.Status)
			}
			if desc == "" || ev.Description == "" {
				t.Fatalf("expected non-empty description")
			}
		})
	}
}

func TestLocationEventGenerator_Execute_TimestampsStable(t *testing.T) {
	// Deterministic RNG isn't important here; we only verify timestamps are "now-ish".
	repo := &mockLocationEventRepo{}
	gen := NewLocationEventGenerator(repo)
	start := time.Now()

	resp, err := gen.Execute(context.Background(), GenerateLocationEventRequest{
		WorldID:      1,
		LocationID:   1,
		LocationName: "Test",
		IsFirstVisit: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil || resp.Event == nil {
		t.Fatalf("expected event")
	}
	if resp.Event.CreatedAt.Before(start.Add(-time.Second)) || resp.Event.CreatedAt.After(time.Now().Add(time.Second)) {
		t.Fatalf("unexpected CreatedAt: %v", resp.Event.CreatedAt)
	}
}

