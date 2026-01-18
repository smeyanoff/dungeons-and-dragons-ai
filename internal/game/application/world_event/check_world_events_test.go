package world_event

import (
	"context"
	"errors"
	"testing"
	"time"

	"dungeons-and-dragons-ai/internal/game/domain/world"
)

// mockWorldEventRepository мок для репозитория мировых событий
type mockWorldEventRepository struct {
	getScheduledByWorldIDFunc func(ctx context.Context, worldID uint) ([]world.WorldEvent, error)
	getActiveByWorldIDFunc    func(ctx context.Context, worldID uint) ([]world.WorldEvent, error)
	saveFunc                  func(ctx context.Context, e *world.WorldEvent) error
}

func (m *mockWorldEventRepository) GetScheduledByWorldID(ctx context.Context, worldID uint) ([]world.WorldEvent, error) {
	if m.getScheduledByWorldIDFunc != nil {
		return m.getScheduledByWorldIDFunc(ctx, worldID)
	}
	return []world.WorldEvent{}, nil
}

func (m *mockWorldEventRepository) GetActiveByWorldID(ctx context.Context, worldID uint) ([]world.WorldEvent, error) {
	if m.getActiveByWorldIDFunc != nil {
		return m.getActiveByWorldIDFunc(ctx, worldID)
	}
	return []world.WorldEvent{}, nil
}

func (m *mockWorldEventRepository) Save(ctx context.Context, e *world.WorldEvent) error {
	if m.saveFunc != nil {
		return m.saveFunc(ctx, e)
	}
	return nil
}

func TestCheckWorldEventsUseCase_Execute(t *testing.T) {
	now := time.Now()
	testWorld := world.World{
		ID:        1,
		Name:      "Test World",
		Day:       5,
		TimeOfDay: "morning",
		Weather:   "clear",
		StartedAt: now.AddDate(0, 0, -4), // Начато 4 дня назад
	}

	tests := []struct {
		name           string
		req            CheckWorldEventsRequest
		world          *world.World
		setupMocks     func(*mockWorldEventRepository, *world.World)
		expectedError  bool
		expectedCount  int
		expectedActive int
	}{
		{
			name: "no scheduled events",
			req: CheckWorldEventsRequest{
				WorldID:          1,
				CurrentLocationID: nil,
			},
			world: &testWorld,
			setupMocks: func(repo *mockWorldEventRepository, w *world.World) {
				repo.getScheduledByWorldIDFunc = func(ctx context.Context, worldID uint) ([]world.WorldEvent, error) {
					return []world.WorldEvent{}, nil
				}
				repo.getActiveByWorldIDFunc = func(ctx context.Context, worldID uint) ([]world.WorldEvent, error) {
					return []world.WorldEvent{}, nil
				}
			},
			expectedError: false,
			expectedCount: 0,
		},
		{
			name: "activate event with matching conditions",
			req: CheckWorldEventsRequest{
				WorldID:          1,
				CurrentLocationID: nil,
			},
			world: &testWorld,
			setupMocks: func(repo *mockWorldEventRepository, w *world.World) {
				scheduledTime := time.Now().Add(-1 * time.Hour) // Прошло час назад
				dayOfWeek := w.GetDayOfWeek()
				
				repo.getScheduledByWorldIDFunc = func(ctx context.Context, worldID uint) ([]world.WorldEvent, error) {
					return []world.WorldEvent{
						{
							ID:               1,
							WorldID:          worldID,
							Type:             world.WorldEventTypeFestival,
							Status:           world.WorldEventStatusScheduled,
							Name:             "Morning Festival",
							Description:      "A festival in the morning",
							RequiredDayOfWeek: &dayOfWeek,
							RequiredTimeOfDay: "morning",
							RequiredWeather:   "clear",
							ScheduledFor:      &scheduledTime,
						},
					}, nil
				}
				repo.getActiveByWorldIDFunc = func(ctx context.Context, worldID uint) ([]world.WorldEvent, error) {
					return []world.WorldEvent{}, nil
				}
				repo.saveFunc = func(ctx context.Context, e *world.WorldEvent) error {
					if e.Status != world.WorldEventStatusActive {
						t.Errorf("expected status Active, got %s", e.Status)
					}
					if e.ActivatedAt == nil {
						t.Error("expected ActivatedAt to be set")
					}
					return nil
				}
			},
			expectedError: false,
			expectedCount: 1,
		},
		{
			name: "do not activate event with non-matching day of week",
			req: CheckWorldEventsRequest{
				WorldID:          1,
				CurrentLocationID: nil,
			},
			world: &testWorld,
			setupMocks: func(repo *mockWorldEventRepository, w *world.World) {
				scheduledTime := time.Now().Add(-1 * time.Hour)
				wrongDay := (w.GetDayOfWeek() + 1) % 7
				
				repo.getScheduledByWorldIDFunc = func(ctx context.Context, worldID uint) ([]world.WorldEvent, error) {
					return []world.WorldEvent{
						{
							ID:               1,
							WorldID:          worldID,
							Type:             world.WorldEventTypeFestival,
							Status:           world.WorldEventStatusScheduled,
							Name:             "Wrong Day Festival",
							RequiredDayOfWeek: &wrongDay,
							RequiredTimeOfDay: "morning",
							ScheduledFor:      &scheduledTime,
						},
					}, nil
				}
				repo.getActiveByWorldIDFunc = func(ctx context.Context, worldID uint) ([]world.WorldEvent, error) {
					return []world.WorldEvent{}, nil
				}
			},
			expectedError: false,
			expectedCount: 0,
		},
		{
			name: "do not activate event with non-matching time of day",
			req: CheckWorldEventsRequest{
				WorldID:          1,
				CurrentLocationID: nil,
			},
			world: &testWorld,
			setupMocks: func(repo *mockWorldEventRepository, w *world.World) {
				scheduledTime := time.Now().Add(-1 * time.Hour)
				dayOfWeek := w.GetDayOfWeek()
				
				repo.getScheduledByWorldIDFunc = func(ctx context.Context, worldID uint) ([]world.WorldEvent, error) {
					return []world.WorldEvent{
						{
							ID:               1,
							WorldID:          worldID,
							Type:             world.WorldEventTypeFestival,
							Status:           world.WorldEventStatusScheduled,
							Name:             "Evening Festival",
							RequiredDayOfWeek: &dayOfWeek,
							RequiredTimeOfDay: "evening", // Не совпадает с "morning"
							ScheduledFor:      &scheduledTime,
						},
					}, nil
				}
				repo.getActiveByWorldIDFunc = func(ctx context.Context, worldID uint) ([]world.WorldEvent, error) {
					return []world.WorldEvent{}, nil
				}
			},
			expectedError: false,
			expectedCount: 0,
		},
		{
			name: "do not activate event with non-matching weather",
			req: CheckWorldEventsRequest{
				WorldID:          1,
				CurrentLocationID: nil,
			},
			world: &testWorld,
			setupMocks: func(repo *mockWorldEventRepository, w *world.World) {
				scheduledTime := time.Now().Add(-1 * time.Hour)
				dayOfWeek := w.GetDayOfWeek()
				
				repo.getScheduledByWorldIDFunc = func(ctx context.Context, worldID uint) ([]world.WorldEvent, error) {
					return []world.WorldEvent{
						{
							ID:               1,
							WorldID:          worldID,
							Type:             world.WorldEventTypeFestival,
							Status:           world.WorldEventStatusScheduled,
							Name:             "Rainy Festival",
							RequiredDayOfWeek: &dayOfWeek,
							RequiredTimeOfDay: "morning",
							RequiredWeather:   "rainy", // Не совпадает с "clear"
							ScheduledFor:      &scheduledTime,
						},
					}, nil
				}
				repo.getActiveByWorldIDFunc = func(ctx context.Context, worldID uint) ([]world.WorldEvent, error) {
					return []world.WorldEvent{}, nil
				}
			},
			expectedError: false,
			expectedCount: 0,
		},
		{
			name: "do not activate event with non-matching location",
			req: CheckWorldEventsRequest{
				WorldID:          1,
				CurrentLocationID: uintPtr(2),
			},
			world: &testWorld,
			setupMocks: func(repo *mockWorldEventRepository, w *world.World) {
				scheduledTime := time.Now().Add(-1 * time.Hour)
				dayOfWeek := w.GetDayOfWeek()
				requiredLocID := uint(1)
				
				repo.getScheduledByWorldIDFunc = func(ctx context.Context, worldID uint) ([]world.WorldEvent, error) {
					return []world.WorldEvent{
						{
							ID:               1,
							WorldID:          worldID,
							Type:             world.WorldEventTypeFestival,
							Status:           world.WorldEventStatusScheduled,
							Name:             "Location-Specific Festival",
							RequiredDayOfWeek: &dayOfWeek,
							RequiredTimeOfDay: "morning",
							RequiredLocationID: &requiredLocID, // Требует локацию 1, игрок в локации 2
							ScheduledFor:      &scheduledTime,
						},
					}, nil
				}
				repo.getActiveByWorldIDFunc = func(ctx context.Context, worldID uint) ([]world.WorldEvent, error) {
					return []world.WorldEvent{}, nil
				}
			},
			expectedError: false,
			expectedCount: 0,
		},
		{
			name: "activate event with matching location",
			req: CheckWorldEventsRequest{
				WorldID:          1,
				CurrentLocationID: uintPtr(1),
			},
			world: &testWorld,
			setupMocks: func(repo *mockWorldEventRepository, w *world.World) {
				scheduledTime := time.Now().Add(-1 * time.Hour)
				dayOfWeek := w.GetDayOfWeek()
				requiredLocID := uint(1)
				
				repo.getScheduledByWorldIDFunc = func(ctx context.Context, worldID uint) ([]world.WorldEvent, error) {
					return []world.WorldEvent{
						{
							ID:               1,
							WorldID:          worldID,
							Type:             world.WorldEventTypeFestival,
							Status:           world.WorldEventStatusScheduled,
							Name:             "Location-Specific Festival",
							RequiredDayOfWeek: &dayOfWeek,
							RequiredTimeOfDay: "morning",
							RequiredLocationID: &requiredLocID,
							ScheduledFor:      &scheduledTime,
						},
					}, nil
				}
				repo.getActiveByWorldIDFunc = func(ctx context.Context, worldID uint) ([]world.WorldEvent, error) {
					return []world.WorldEvent{}, nil
				}
				repo.saveFunc = func(ctx context.Context, e *world.WorldEvent) error {
					return nil
				}
			},
			expectedError: false,
			expectedCount: 1,
		},
		{
			name: "do not activate already active event",
			req: CheckWorldEventsRequest{
				WorldID:          1,
				CurrentLocationID: nil,
			},
			world: &testWorld,
			setupMocks: func(repo *mockWorldEventRepository, w *world.World) {
				scheduledTime := time.Now().Add(-1 * time.Hour)
				dayOfWeek := w.GetDayOfWeek()
				
				repo.getScheduledByWorldIDFunc = func(ctx context.Context, worldID uint) ([]world.WorldEvent, error) {
					return []world.WorldEvent{
						{
							ID:               1,
							WorldID:          worldID,
							Type:             world.WorldEventTypeFestival,
							Status:           world.WorldEventStatusActive, // Уже активно
							Name:             "Active Festival",
							RequiredDayOfWeek: &dayOfWeek,
							RequiredTimeOfDay: "morning",
							ScheduledFor:      &scheduledTime,
						},
					}, nil
				}
				repo.getActiveByWorldIDFunc = func(ctx context.Context, worldID uint) ([]world.WorldEvent, error) {
					return []world.WorldEvent{}, nil
				}
			},
			expectedError: false,
			expectedCount: 0,
		},
		{
			name: "do not activate event scheduled for future",
			req: CheckWorldEventsRequest{
				WorldID:          1,
				CurrentLocationID: nil,
			},
			world: &testWorld,
			setupMocks: func(repo *mockWorldEventRepository, w *world.World) {
				futureTime := time.Now().Add(1 * time.Hour) // В будущем
				dayOfWeek := w.GetDayOfWeek()
				
				repo.getScheduledByWorldIDFunc = func(ctx context.Context, worldID uint) ([]world.WorldEvent, error) {
					return []world.WorldEvent{
						{
							ID:               1,
							WorldID:          worldID,
							Type:             world.WorldEventTypeFestival,
							Status:           world.WorldEventStatusScheduled,
							Name:             "Future Festival",
							RequiredDayOfWeek: &dayOfWeek,
							RequiredTimeOfDay: "morning",
							ScheduledFor:      &futureTime,
						},
					}, nil
				}
				repo.getActiveByWorldIDFunc = func(ctx context.Context, worldID uint) ([]world.WorldEvent, error) {
					return []world.WorldEvent{}, nil
				}
			},
			expectedError: false,
			expectedCount: 0,
		},
		{
			name: "return existing active events",
			req: CheckWorldEventsRequest{
				WorldID:          1,
				CurrentLocationID: nil,
			},
			world: &testWorld,
			setupMocks: func(repo *mockWorldEventRepository, w *world.World) {
				repo.getScheduledByWorldIDFunc = func(ctx context.Context, worldID uint) ([]world.WorldEvent, error) {
					return []world.WorldEvent{}, nil
				}
				repo.getActiveByWorldIDFunc = func(ctx context.Context, worldID uint) ([]world.WorldEvent, error) {
					return []world.WorldEvent{
						{
							ID:     1,
							Status: world.WorldEventStatusActive,
							Name:   "Already Active Event",
						},
						{
							ID:     2,
							Status: world.WorldEventStatusActive,
							Name:   "Another Active Event",
						},
					}, nil
				}
			},
			expectedError:  false,
			expectedCount:  0,
			expectedActive: 2,
		},
		{
			name: "error getting scheduled events",
			req: CheckWorldEventsRequest{
				WorldID:          1,
				CurrentLocationID: nil,
			},
			world: &testWorld,
			setupMocks: func(repo *mockWorldEventRepository, w *world.World) {
				repo.getScheduledByWorldIDFunc = func(ctx context.Context, worldID uint) ([]world.WorldEvent, error) {
					return nil, errors.New("database error")
				}
			},
			expectedError: true,
		},
		{
			name: "error getting active events",
			req: CheckWorldEventsRequest{
				WorldID:          1,
				CurrentLocationID: nil,
			},
			world: &testWorld,
			setupMocks: func(repo *mockWorldEventRepository, w *world.World) {
				repo.getScheduledByWorldIDFunc = func(ctx context.Context, worldID uint) ([]world.WorldEvent, error) {
					return []world.WorldEvent{}, nil
				}
				repo.getActiveByWorldIDFunc = func(ctx context.Context, worldID uint) ([]world.WorldEvent, error) {
					return nil, errors.New("database error")
				}
			},
			expectedError: true,
		},
		{
			name: "continue on save error",
			req: CheckWorldEventsRequest{
				WorldID:          1,
				CurrentLocationID: nil,
			},
			world: &testWorld,
			setupMocks: func(repo *mockWorldEventRepository, w *world.World) {
				scheduledTime := time.Now().Add(-1 * time.Hour)
				dayOfWeek := w.GetDayOfWeek()
				
				repo.getScheduledByWorldIDFunc = func(ctx context.Context, worldID uint) ([]world.WorldEvent, error) {
					return []world.WorldEvent{
						{
							ID:               1,
							WorldID:          worldID,
							Type:             world.WorldEventTypeFestival,
							Status:           world.WorldEventStatusScheduled,
							Name:             "Event 1",
							RequiredDayOfWeek: &dayOfWeek,
							RequiredTimeOfDay: "morning",
							ScheduledFor:      &scheduledTime,
						},
						{
							ID:               2,
							WorldID:          worldID,
							Type:             world.WorldEventTypeFestival,
							Status:           world.WorldEventStatusScheduled,
							Name:             "Event 2",
							RequiredDayOfWeek: &dayOfWeek,
							RequiredTimeOfDay: "morning",
							ScheduledFor:      &scheduledTime,
						},
					}, nil
				}
				repo.getActiveByWorldIDFunc = func(ctx context.Context, worldID uint) ([]world.WorldEvent, error) {
					return []world.WorldEvent{}, nil
				}
				saveCallCount := 0
				repo.saveFunc = func(ctx context.Context, e *world.WorldEvent) error {
					saveCallCount++
					if saveCallCount == 1 {
						return errors.New("save error")
					}
					return nil
				}
			},
			expectedError: false,
			expectedCount: 1, // Только одно событие должно быть активировано (второе из-за ошибки сохранения)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockWorldEventRepository{}
			if tt.setupMocks != nil {
				tt.setupMocks(repo, tt.world)
			}

			uc := NewCheckWorldEventsUseCase(repo)
			resp, err := uc.Execute(context.Background(), tt.req, tt.world)

			if tt.expectedError {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if resp == nil {
					t.Fatal("expected response, got nil")
				}
				if len(resp.ActivatedEvents) != tt.expectedCount {
					t.Errorf("expected %d activated events, got %d", tt.expectedCount, len(resp.ActivatedEvents))
				}
				if tt.expectedActive > 0 && len(resp.ActiveEvents) != tt.expectedActive {
					t.Errorf("expected %d active events, got %d", tt.expectedActive, len(resp.ActiveEvents))
				}
			}
		})
	}
}

func TestGetActiveEventsDescription(t *testing.T) {
	uc := NewCheckWorldEventsUseCase(nil)

	tests := []struct {
		name     string
		events   []world.WorldEvent
		expected string
	}{
		{
			name:     "empty events",
			events:   []world.WorldEvent{},
			expected: "",
		},
		{
			name: "single event",
			events: []world.WorldEvent{
				{
					Name:        "Festival",
					Type:        world.WorldEventTypeFestival,
					Description: "A great festival",
				},
			},
			expected: "\n\nАктивные события в мире:\n- Festival (festival): A great festival\n",
		},
		{
			name: "multiple events",
			events: []world.WorldEvent{
				{
					Name:        "Festival",
					Type:        world.WorldEventTypeFestival,
					Description: "A great festival",
				},
				{
					Name:        "Market Day",
					Type:        world.WorldEventTypeMarket,
					Description: "Town market is open",
				},
			},
			expected: "\n\nАктивные события в мире:\n- Festival (festival): A great festival\n- Market Day (market): Town market is open\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := uc.GetActiveEventsDescription(tt.events)
			if result != tt.expected {
				t.Errorf("expected:\n%q\ngot:\n%q", tt.expected, result)
			}
		})
	}
}

// Вспомогательная функция для создания указателя на uint
func uintPtr(u uint) *uint {
	return &u
}