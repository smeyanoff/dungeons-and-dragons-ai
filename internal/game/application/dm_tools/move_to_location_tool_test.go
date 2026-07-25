package dm_tools

import (
	"context"
	"errors"
	"testing"

	mapapp "dungeons-and-dragons-ai/internal/game/application/worldmap"
	"dungeons-and-dragons-ai/internal/game/domain/world"
)

type mockLocationMover struct {
	executeFunc func(ctx context.Context, req mapapp.MoveToLocationRequest) (*mapapp.MoveToLocationResponse, error)
	lastReq     mapapp.MoveToLocationRequest
}

func (m *mockLocationMover) Execute(ctx context.Context, req mapapp.MoveToLocationRequest) (*mapapp.MoveToLocationResponse, error) {
	m.lastReq = req
	if m.executeFunc != nil {
		return m.executeFunc(ctx, req)
	}
	return &mapapp.MoveToLocationResponse{
		From:    &world.Location{ID: 1, Name: "Деревня"},
		To:      &world.Location{ID: 2, Name: "Лес"},
		Message: "Вы переместились: Деревня → Лес",
	}, nil
}

var testLocations = []world.Location{
	{ID: 1, Name: "Деревня"},
	{ID: 2, Name: "Тёмный лес"},
	{ID: 3, Name: "Пещера драконов"},
}

func TestMoveToLocationTool_Name(t *testing.T) {
	tool := NewMoveToLocationTool(nil, 1, nil)
	if tool.Name() != "move_to_location" {
		t.Errorf("expected name 'move_to_location', got '%s'", tool.Name())
	}
}

func TestMoveToLocationTool_Description(t *testing.T) {
	tool := NewMoveToLocationTool(nil, 1, nil)
	if tool.Description() == "" {
		t.Error("expected non-empty description")
	}
}

func TestMoveToLocationTool_Parameters(t *testing.T) {
	tool := NewMoveToLocationTool(nil, 1, nil)
	if len(tool.Parameters()) == 0 {
		t.Error("expected non-empty parameters")
	}
}

func TestMoveToLocationTool_Execute(t *testing.T) {
	tests := []struct {
		name          string
		args          map[string]interface{}
		setupMock     func(*mockLocationMover)
		expectedError bool
		validate      func(*testing.T, *mockLocationMover, interface{})
	}{
		{
			name: "moves by exact target location name",
			args: map[string]interface{}{"target_location_name": "Тёмный лес"},
			validate: func(t *testing.T, mover *mockLocationMover, result interface{}) {
				if mover.lastReq.ToLocationID == nil || *mover.lastReq.ToLocationID != 2 {
					t.Fatalf("expected resolved ToLocationID=2, got %v", mover.lastReq.ToLocationID)
				}
				resultMap, ok := result.(map[string]interface{})
				if !ok || resultMap["success"] != true {
					t.Errorf("expected success result, got %v", result)
				}
			},
		},
		{
			name: "moves by partial location name match",
			args: map[string]interface{}{"target_location_name": "лес"},
			validate: func(t *testing.T, mover *mockLocationMover, result interface{}) {
				if mover.lastReq.ToLocationID == nil || *mover.lastReq.ToLocationID != 2 {
					t.Fatalf("expected resolved ToLocationID=2, got %v", mover.lastReq.ToLocationID)
				}
			},
		},
		{
			name: "moves by direction when no name given",
			args: map[string]interface{}{"direction": "north"},
			validate: func(t *testing.T, mover *mockLocationMover, result interface{}) {
				if mover.lastReq.Direction != "north" {
					t.Errorf("expected Direction 'north', got %q", mover.lastReq.Direction)
				}
				if mover.lastReq.ToLocationID != nil {
					t.Errorf("expected ToLocationID nil when using direction, got %v", mover.lastReq.ToLocationID)
				}
			},
		},
		{
			name:          "no target and no direction returns error",
			args:          map[string]interface{}{},
			expectedError: true,
		},
		{
			name: "unknown location name returns success=false without Go error",
			args: map[string]interface{}{"target_location_name": "Несуществующий Замок"},
			validate: func(t *testing.T, mover *mockLocationMover, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map result, got %T", result)
				}
				if success, _ := resultMap["success"].(bool); success {
					t.Error("expected success=false for unresolved location name")
				}
			},
		},
		{
			name: "unreachable location error from mover is returned in result, not as Go error",
			args: map[string]interface{}{"direction": "south"},
			setupMock: func(m *mockLocationMover) {
				m.executeFunc = func(ctx context.Context, req mapapp.MoveToLocationRequest) (*mapapp.MoveToLocationResponse, error) {
					return nil, errors.New("target location is not reachable from current location")
				}
			},
			validate: func(t *testing.T, mover *mockLocationMover, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map result, got %T", result)
				}
				if success, _ := resultMap["success"].(bool); success {
					t.Error("expected success=false when mover returns an error")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mover := &mockLocationMover{}
			if tt.setupMock != nil {
				tt.setupMock(mover)
			}
			tool := NewMoveToLocationTool(mover, 42, testLocations)

			result, err := tool.Execute(context.Background(), tt.args)

			if tt.expectedError && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.expectedError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.validate != nil {
				tt.validate(t, mover, result)
			}
		})
	}
}
