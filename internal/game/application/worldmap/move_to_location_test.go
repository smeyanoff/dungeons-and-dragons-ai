package worldmap

import (
	"context"
	"errors"
	"strings"
	"testing"

	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/game/domain/world"
)

type mockSessionRepoMove struct {
	session       *session.GameSession
	getErr        error
	saveErr       error
	saveCallCount int
}

func (m *mockSessionRepoMove) GetByChatID(ctx context.Context, chatID int64) (*session.GameSession, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.session, nil
}

func (m *mockSessionRepoMove) Save(ctx context.Context, s *session.GameSession) error {
	m.saveCallCount++
	m.session = s
	if m.saveErr != nil {
		return m.saveErr
	}
	return nil
}

func (m *mockSessionRepoMove) Delete(ctx context.Context, chatID int64) error { return nil }

func TestMoveToLocationUseCase_Execute(t *testing.T) {
	loc1 := world.Location{
		ID:          1,
		WorldID:     1,
		Name:        "Town",
		Description: "Town desc",
		Connections: []world.LocationConnection{
			{FromLocationID: 1, ToLocationID: 2, Direction: "north", Description: "Road"},
		},
	}
	loc2 := world.Location{
		ID:          2,
		WorldID:     1,
		Name:        "Forest",
		Description: "Forest desc",
		Connections: []world.LocationConnection{
			{FromLocationID: 2, ToLocationID: 1, Direction: "south"},
		},
	}
	loc3 := world.Location{
		ID:          3,
		WorldID:     1,
		Name:        "Lake",
		Description: "Lake desc",
	}

	activeSession := &session.GameSession{
		ChatID: 123,
		State:  session.StateActive,
		World: world.World{
			ID:        1,
			Name:      "W",
			Locations: []world.Location{loc1, loc2, loc3},
		},
		CurrentLocationID: uintPtr(1),
	}

	tests := []struct {
		name        string
		repo        *mockSessionRepoMove
		req         MoveToLocationRequest
		wantErr     bool
		wantLocID   *uint
		wantContains []string
	}{
		{
			name:    "session not found",
			repo:    &mockSessionRepoMove{session: nil},
			req:     MoveToLocationRequest{ChatID: 123, Direction: "north"},
			wantErr: true,
		},
		{
			name:    "repo get error",
			repo:    &mockSessionRepoMove{session: nil, getErr: errors.New("db error")},
			req:     MoveToLocationRequest{ChatID: 123, Direction: "north"},
			wantErr: true,
		},
		{
			name: "inactive session",
			repo: &mockSessionRepoMove{session: &session.GameSession{ChatID: 123, State: session.StateDone}},
			req:  MoveToLocationRequest{ChatID: 123, Direction: "north"},
			wantErr: true,
		},
		{
			name: "world has no locations",
			repo: &mockSessionRepoMove{session: &session.GameSession{
				ChatID: 123,
				State:  session.StateActive,
				World:  world.World{Name: "Empty", Locations: nil},
			}},
			req:     MoveToLocationRequest{ChatID: 123, Direction: "north"},
			wantErr: true,
		},
		{
			name: "neither ToLocationID nor Direction provided",
			repo: &mockSessionRepoMove{session: cloneSession(activeSession)},
			req:  MoveToLocationRequest{ChatID: 123},
			wantErr: true,
		},
		{
			name: "move by direction (normalized short form)",
			repo: &mockSessionRepoMove{session: cloneSession(activeSession)},
			req:  MoveToLocationRequest{ChatID: 123, Direction: "N"},
			wantErr: false,
			wantLocID: uintPtr(2),
			wantContains: []string{"🧭", "Town", "Forest", "Forest desc"},
		},
		{
			name: "move by ToLocationID",
			repo: &mockSessionRepoMove{session: cloneSession(activeSession)},
			req:  MoveToLocationRequest{ChatID: 123, ToLocationID: uintPtr(2)},
			wantErr: false,
			wantLocID: uintPtr(2),
			wantContains: []string{"🧭", "Town", "Forest"},
		},
		{
			name: "direction not found -> error",
			repo: &mockSessionRepoMove{session: cloneSession(activeSession)},
			req:  MoveToLocationRequest{ChatID: 123, Direction: "west"},
			wantErr: true,
		},
		{
			name: "target exists but not reachable -> error",
			repo: &mockSessionRepoMove{session: cloneSession(activeSession)},
			req:  MoveToLocationRequest{ChatID: 123, ToLocationID: uintPtr(3)}, // Lake exists but no edge from Town
			wantErr: true,
		},
		{
			name: "target not found in world -> error",
			repo: &mockSessionRepoMove{session: cloneSession(activeSession)},
			req:  MoveToLocationRequest{ChatID: 123, ToLocationID: uintPtr(999)},
			wantErr: true,
		},
		{
			name: "save error is returned on final save",
			repo: &mockSessionRepoMove{session: cloneSession(activeSession), saveErr: errors.New("write failed")},
			req:  MoveToLocationRequest{ChatID: 123, ToLocationID: uintPtr(2)},
			wantErr: true,
		},
		{
			name: "current location nil -> initialized to first location",
			repo: &mockSessionRepoMove{session: func() *session.GameSession {
				s := cloneSession(activeSession)
				s.CurrentLocationID = nil
				return s
			}()},
			req:  MoveToLocationRequest{ChatID: 123, ToLocationID: uintPtr(2)},
			wantErr: false,
			wantLocID: uintPtr(2),
		},
	}

	uc := NewMoveToLocationUseCase(nil)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc.sessionRepo = tt.repo
			resp, err := uc.Execute(context.Background(), tt.req)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (resp=%+v)", resp)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp == nil || resp.To == nil || resp.From == nil {
				t.Fatalf("expected non-nil response with from/to")
			}
			if tt.wantLocID != nil {
				if tt.repo.session.CurrentLocationID == nil || *tt.repo.session.CurrentLocationID != *tt.wantLocID {
					t.Fatalf("expected CurrentLocationID=%d, got %+v", *tt.wantLocID, tt.repo.session.CurrentLocationID)
				}
			}
			for _, s := range tt.wantContains {
				if !strings.Contains(resp.Message, s) {
					t.Fatalf("expected response message to contain %q, got: %s", s, resp.Message)
				}
			}
		})
	}
}

func uintPtr(u uint) *uint { return &u }

func cloneSession(s *session.GameSession) *session.GameSession {
	// Minimal deep-ish copy for these tests.
	if s == nil {
		return nil
	}
	cp := *s
	cp.World = s.World
	cp.World.Locations = append([]world.Location(nil), s.World.Locations...)
	if s.CurrentLocationID != nil {
		cp.CurrentLocationID = uintPtr(*s.CurrentLocationID)
	}
	return &cp
}

