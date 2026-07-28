package context

import (
	"context"
	"testing"

	"dungeons-and-dragons-ai/internal/game/domain/quest"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/game/domain/world"
)

func TestSimpleContextBuilder_BuildContext(t *testing.T) {
	tests := []struct {
		name      string
		gs        *session.GameSession
		message   string
		wantError bool
		validate  func(*testing.T, string)
	}{
		{
			name: "successful context building",
			gs: &session.GameSession{
				World: world.World{
					Name:        "Test World",
					Description: "A test world description",
					MainQuest: &quest.Quest{
						Title:       "Main Quest",
						Description: "Main quest description",
					},
					Locations: []world.Location{
						{
							Name:        "Location 1",
							Description: "Description 1",
						},
						{
							Name:        "Location 2",
							Description: "Description 2",
						},
					},
				},
			},
			message:   "I want to explore",
			wantError: false,
			validate: func(t *testing.T, result string) {
				if result == "" {
					t.Error("expected non-empty context")
				}
				if !contains(result, "Test World") {
					t.Error("expected world name in context")
				}
				if !contains(result, "A test world description") {
					t.Error("expected world description in context")
				}
				if !contains(result, "Main Quest") {
					t.Error("expected main quest title in context")
				}
				if !contains(result, "Location 1") {
					t.Error("expected location 1 in context")
				}
				if !contains(result, "Location 2") {
					t.Error("expected location 2 in context")
				}
			},
		},
		{
			name: "world without main quest",
			gs: &session.GameSession{
				World: world.World{
					Name:        "Test World",
					Description: "A test world description",
					MainQuest:   nil,
					Locations: []world.Location{
						{
							Name:        "Location 1",
							Description: "Description 1",
						},
					},
				},
			},
			message:   "I want to explore",
			wantError: false,
			validate: func(t *testing.T, result string) {
				if result == "" {
					t.Error("expected non-empty context")
				}
				if contains(result, "Главный квест") {
					t.Error("should not contain main quest when it's nil")
				}
			},
		},
		{
			name: "world without locations",
			gs: &session.GameSession{
				World: world.World{
					Name:        "Test World",
					Description: "A test world description",
					MainQuest: &quest.Quest{
						Title:       "Main Quest",
						Description: "Main quest description",
					},
					Locations: []world.Location{},
				},
			},
			message:   "I want to explore",
			wantError: false,
			validate: func(t *testing.T, result string) {
				if result == "" {
					t.Error("expected non-empty context")
				}
				if contains(result, "Локации:") {
					t.Error("should not contain locations section when empty")
				}
			},
		},
		{
			name: "location NPCs with attitude are included in context",
			gs: func() *session.GameSession {
				locID := uint(1)
				return &session.GameSession{
					CurrentLocationID: &locID,
					World: world.World{
						Name:        "Test World",
						Description: "A test world description",
						Locations: []world.Location{
							{
								ID:          1,
								Name:        "Тёмное подземелье",
								Description: "Description 1",
								NPCs: []world.NPC{
									{Name: "Злой некромант", Role: "враг", Attitude: "hostile"},
									{Name: "Торговец", Role: "торговец", Attitude: "friendly"},
								},
							},
						},
					},
				}
			}(),
			message:   "I want to explore",
			wantError: false,
			validate: func(t *testing.T, result string) {
				if !contains(result, "Злой некромант") {
					t.Error("expected NPC name in context")
				}
				if !contains(result, "враждебно настроен") {
					t.Error("expected hostile attitude instruction in context for hostile NPC")
				}
				if !contains(result, "дружелюбно настроен") {
					t.Error("expected friendly attitude instruction in context for friendly NPC")
				}
			},
		},
		{
			name: "minimal world",
			gs: &session.GameSession{
				World: world.World{
					Name:        "Test World",
					Description: "A test world description",
					MainQuest:   nil,
					Locations:   []world.Location{},
				},
			},
			message:   "I want to explore",
			wantError: false,
			validate: func(t *testing.T, result string) {
				if result == "" {
					t.Error("expected non-empty context")
				}
				if !contains(result, "Test World") {
					t.Error("expected world name in context")
				}
			},
		},
	}

	builder := NewSimpleContextBuilder()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := builder.BuildContext(context.Background(), tt.gs, tt.message)

			if tt.wantError {
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

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
