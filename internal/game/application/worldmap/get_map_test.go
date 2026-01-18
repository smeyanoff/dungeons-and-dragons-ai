package worldmap

import (
	"context"
	"errors"
	"strings"
	"testing"

	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/game/domain/world"
)

// mockSessionRepository мок для репозитория сессий
type mockSessionRepository struct {
	getByChatIDFunc func(ctx context.Context, chatID int64) (*session.GameSession, error)
}

func (m *mockSessionRepository) GetByChatID(ctx context.Context, chatID int64) (*session.GameSession, error) {
	if m.getByChatIDFunc != nil {
		return m.getByChatIDFunc(ctx, chatID)
	}
	return nil, nil
}

func (m *mockSessionRepository) Save(ctx context.Context, s *session.GameSession) error {
	return nil
}

func (m *mockSessionRepository) Delete(ctx context.Context, chatID int64) error {
	return nil
}

func TestGetMapUseCase_Execute(t *testing.T) {
	tests := []struct {
		name           string
		chatID         int64
		setupMocks     func(*mockSessionRepository)
		expectedError  bool
		expectedInMap  []string // Строки, которые должны быть в карте
		expectedNotInMap []string // Строки, которых не должно быть в карте
	}{
		{
			name:   "no session",
			chatID: 12345,
			setupMocks: func(repo *mockSessionRepository) {
				repo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return nil, nil
				}
			},
			expectedError: false,
			expectedInMap: []string{"Игра не начата"},
		},
		{
			name:   "empty world",
			chatID: 12345,
			setupMocks: func(repo *mockSessionRepository) {
				repo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return &session.GameSession{
						ChatID: chatID,
						World:  world.World{Name: "Empty World", Locations: []world.Location{}},
					}, nil
				}
			},
			expectedError: false,
			expectedInMap: []string{"Карта мира пуста"},
		},
		{
			name:   "single location",
			chatID: 12345,
			setupMocks: func(repo *mockSessionRepository) {
				repo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return &session.GameSession{
						ChatID: chatID,
						World: world.World{
							ID:   1,
							Name: "Test World",
							Locations: []world.Location{
								{
									ID:          1,
									WorldID:     1,
									Name:        "Town Square",
									Description: "The center of the town",
								},
							},
						},
					}, nil
				}
			},
			expectedError: false,
			expectedInMap: []string{"Test World", "Town Square", "The center of the town"},
		},
		{
			name:   "location with connections",
			chatID: 12345,
			setupMocks: func(repo *mockSessionRepository) {
				repo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return &session.GameSession{
						ChatID: chatID,
						World: world.World{
							ID:   1,
							Name: "Test World",
							Locations: []world.Location{
								{
									ID:          1,
									WorldID:     1,
									Name:        "Town Square",
									Description: "The center",
									Connections: []world.LocationConnection{
										{
											ID:             1,
											FromLocationID: 1,
											ToLocationID:   2,
											Direction:      "north",
											Description:    "Main street",
										},
									},
								},
								{
									ID:          2,
									WorldID:     1,
									Name:        "Market",
									Description: "The market",
								},
							},
						},
					}, nil
				}
			},
			expectedError: false,
			expectedInMap: []string{"Town Square", "Main street", "Market", "⬆️"},
		},
		{
			name:   "location with NPCs and monsters",
			chatID: 12345,
			setupMocks: func(repo *mockSessionRepository) {
				repo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return &session.GameSession{
						ChatID: chatID,
						World: world.World{
							ID:   1,
							Name: "Test World",
							Locations: []world.Location{
								{
									ID:          1,
									WorldID:     1,
									Name:        "Tavern",
									Description: "A cozy tavern",
									NPCs: []world.NPC{
										{
											ID:   1,
											Name: "Innkeeper",
											Role: "Merchant",
										},
									},
									Monsters: []world.Monster{
										{
											ID:   1,
											Name: "Goblin",
										},
									},
								},
							},
						},
					}, nil
				}
			},
			expectedError: false,
			expectedInMap: []string{"Tavern", "👤 Innkeeper", "Merchant", "👹 Goblin", "Обнаружено:"},
		},
		{
			name:   "world with time and weather",
			chatID: 12345,
			setupMocks: func(repo *mockSessionRepository) {
				repo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return &session.GameSession{
						ChatID: chatID,
						World: world.World{
							ID:       1,
							Name:     "Test World",
							TimeOfDay: "evening",
							Weather:   "rainy",
							Locations: []world.Location{
								{
									ID:      1,
									WorldID: 1,
									Name:    "Location",
								},
							},
						},
					}, nil
				}
			},
			expectedError: false,
			expectedInMap: []string{"Вечер", "Дождь", "Время суток:", "Погода:"},
		},
		{
			name:   "long description truncated",
			chatID: 12345,
			setupMocks: func(repo *mockSessionRepository) {
				repo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					longDesc := strings.Repeat("a", 150)
					return &session.GameSession{
						ChatID: chatID,
						World: world.World{
							ID:   1,
							Name: "Test World",
							Locations: []world.Location{
								{
									ID:          1,
									WorldID:     1,
									Name:        "Location",
									Description: longDesc,
								},
							},
						},
					}, nil
				}
			},
			expectedError: false,
			expectedInMap: []string{"..."},
			expectedNotInMap: []string{strings.Repeat("a", 150)}, // Полное описание не должно быть
		},
		{
			name:   "connection to non-existent location",
			chatID: 12345,
			setupMocks: func(repo *mockSessionRepository) {
				repo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return &session.GameSession{
						ChatID: chatID,
						World: world.World{
							ID:   1,
							Name: "Test World",
							Locations: []world.Location{
								{
									ID:          1,
									WorldID:     1,
									Name:        "Location 1",
									Connections: []world.LocationConnection{
										{
											ID:             1,
											FromLocationID: 1,
											ToLocationID:   999, // Несуществующая локация
											Direction:      "north",
										},
									},
								},
							},
						},
					}, nil
				}
			},
			expectedError: false,
			expectedInMap: []string{"Локация #999"},
		},
		{
			name:   "different direction symbols",
			chatID: 12345,
			setupMocks: func(repo *mockSessionRepository) {
				repo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return &session.GameSession{
						ChatID: chatID,
						World: world.World{
							ID:   1,
							Name: "Test World",
							Locations: []world.Location{
								{
									ID:          1,
									WorldID:     1,
									Name:        "Center",
									Connections: []world.LocationConnection{
										{ID: 1, FromLocationID: 1, ToLocationID: 2, Direction: "north"},
										{ID: 2, FromLocationID: 1, ToLocationID: 3, Direction: "south"},
										{ID: 3, FromLocationID: 1, ToLocationID: 4, Direction: "east"},
										{ID: 4, FromLocationID: 1, ToLocationID: 5, Direction: "west"},
										{ID: 5, FromLocationID: 1, ToLocationID: 6, Direction: "up"},
										{ID: 6, FromLocationID: 1, ToLocationID: 7, Direction: "down"},
										{ID: 7, FromLocationID: 1, ToLocationID: 8, Direction: "portal"},
										{ID: 8, FromLocationID: 1, ToLocationID: 9, Direction: "path"},
									},
								},
							},
						},
					}, nil
				}
			},
			expectedError: false,
			expectedInMap: []string{"⬆️", "⬇️", "➡️", "⬅️"},
		},
		{
			name:   "session repo error",
			chatID: 12345,
			setupMocks: func(repo *mockSessionRepository) {
				repo.getByChatIDFunc = func(ctx context.Context, chatID int64) (*session.GameSession, error) {
					return nil, errors.New("database error")
				}
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockSessionRepository{}
			if tt.setupMocks != nil {
				tt.setupMocks(repo)
			}

			uc := NewGetMapUseCase(repo)
			result, err := uc.Execute(context.Background(), tt.chatID)

			if tt.expectedError {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if result == "" {
					t.Error("expected non-empty map result")
				}

				// Проверяем наличие ожидаемых строк
				for _, expected := range tt.expectedInMap {
					if !strings.Contains(result, expected) {
						t.Errorf("expected map to contain %q, got:\n%s", expected, result)
					}
				}

				// Проверяем отсутствие неожидаемых строк
				for _, notExpected := range tt.expectedNotInMap {
					if strings.Contains(result, notExpected) {
						t.Errorf("expected map not to contain %q", notExpected)
					}
				}
			}
		})
	}
}

func TestGetMapUseCase_DirectionSymbols(t *testing.T) {
	uc := NewGetMapUseCase(nil)

	tests := []struct {
		direction string
		expected  string
	}{
		{"north", "⬆️"},
		{"N", "⬆️"},
		{"south", "⬇️"},
		{"S", "⬇️"},
		{"east", "➡️"},
		{"E", "➡️"},
		{"west", "⬅️"},
		{"W", "⬅️"},
		{"up", "⬆️⬆️"},
		{"U", "⬆️⬆️"},
		{"down", "⬇️⬇️"},
		{"D", "⬇️⬇️"},
		{"portal", "🌀"},
		{"path", "🛤️"},
		{"road", "🛤️"},
		{"unknown", "→"},
	}

	for _, tt := range tests {
		t.Run(tt.direction, func(t *testing.T) {
			result := uc.getDirectionSymbol(tt.direction)
			if result != tt.expected {
				t.Errorf("expected %q for direction %q, got %q", tt.expected, tt.direction, result)
			}
		})
	}
}

func TestGetMapUseCase_TranslateTimeOfDay(t *testing.T) {
	uc := NewGetMapUseCase(nil)

	tests := []struct {
		timeOfDay string
		expected  string
	}{
		{"morning", "Утро"},
		{"Morning", "Утро"},
		{"MORNING", "Утро"},
		{"noon", "Полдень"},
		{"afternoon", "День"},
		{"evening", "Вечер"},
		{"night", "Ночь"},
		{"midnight", "Полночь"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.timeOfDay, func(t *testing.T) {
			result := uc.translateTimeOfDay(tt.timeOfDay)
			if result != tt.expected {
				t.Errorf("expected %q for timeOfDay %q, got %q", tt.expected, tt.timeOfDay, result)
			}
		})
	}
}

func TestGetMapUseCase_TranslateWeather(t *testing.T) {
	uc := NewGetMapUseCase(nil)

	tests := []struct {
		weather string
		expected string
	}{
		{"clear", "Ясно"},
		{"Clear", "Ясно"},
		{"CLEAR", "Ясно"},
		{"cloudy", "Облачно"},
		{"rainy", "Дождь"},
		{"stormy", "Гроза"},
		{"foggy", "Туман"},
		{"snowy", "Снег"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.weather, func(t *testing.T) {
			result := uc.translateWeather(tt.weather)
			if result != tt.expected {
				t.Errorf("expected %q for weather %q, got %q", tt.expected, tt.weather, result)
			}
		})
	}
}