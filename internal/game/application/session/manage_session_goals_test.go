package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"dungeons-and-dragons-ai/internal/game/domain/session"
)

// Mock Session Repository
type mockSessionRepo struct {
	getByChatIDFunc func(ctx context.Context, chatID int64) (*session.GameSession, error)
	saveFunc        func(ctx context.Context, gs *session.GameSession) error
}

func (m *mockSessionRepo) GetByChatID(ctx context.Context, chatID int64) (*session.GameSession, error) {
	if m.getByChatIDFunc != nil {
		return m.getByChatIDFunc(ctx, chatID)
	}
	return &session.GameSession{}, nil
}

func (m *mockSessionRepo) Save(ctx context.Context, gs *session.GameSession) error {
	if m.saveFunc != nil {
		return m.saveFunc(ctx, gs)
	}
	return nil
}

func TestManageSessionGoalsUseCase_UpdateGoalProgress(t *testing.T) {
	tests := []struct {
		name           string
		setupMock      func() *mockSessionRepo
		request        UpdateGoalProgressRequest
		expectedError  bool
		expectedSaved  bool
		expectedStatus session.SessionGoalStatus
	}{
		{
			name: "success - update exploration goal",
			setupMock: func() *mockSessionRepo {
				gs := &session.GameSession{
					State: session.StateActive,
					SessionGoals: []session.SessionGoal{
						{
							Type:         session.GoalTypeExploration,
							Status:       session.GoalStatusActive,
							CurrentValue: 1,
							TargetValue:  3,
						},
					},
				}
				return &mockSessionRepo{
					getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
						return gs, nil
					},
					saveFunc: func(ctx context.Context, gs *session.GameSession) error {
						return nil
					},
				}
			},
			request: UpdateGoalProgressRequest{
				ChatID:      12345,
				GoalType:    session.GoalTypeExploration,
				IncrementBy: 2,
			},
			expectedError:  false,
			expectedSaved:  true,
			expectedStatus: session.GoalStatusCompleted,
		},
		{
			name: "success - complete exploration goal",
			setupMock: func() *mockSessionRepo {
				gs := &session.GameSession{
					State: session.StateActive,
					SessionGoals: []session.SessionGoal{
						{
							Type:         session.GoalTypeExploration,
							Status:       session.GoalStatusActive,
							CurrentValue: 2,
							TargetValue:  3,
						},
					},
				}
				return &mockSessionRepo{
					getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
						return gs, nil
					},
					saveFunc: func(ctx context.Context, gs *session.GameSession) error {
						return nil
					},
				}
			},
			request: UpdateGoalProgressRequest{
				ChatID:      12345,
				GoalType:    session.GoalTypeExploration,
				IncrementBy: 2,
			},
			expectedError:  false,
			expectedSaved:  true,
			expectedStatus: session.GoalStatusCompleted,
		},
		{
			name: "error - session not found",
			setupMock: func() *mockSessionRepo {
				return &mockSessionRepo{
					getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
						return nil, errors.New("session not found")
					},
				}
			},
			request: UpdateGoalProgressRequest{
				ChatID:      12345,
				GoalType:    session.GoalTypeExploration,
				IncrementBy: 1,
			},
			expectedError: true,
			expectedSaved: false,
		},
		{
			name: "no update - session not active",
			setupMock: func() *mockSessionRepo {
				gs := &session.GameSession{
					State: session.StateDone,
					SessionGoals: []session.SessionGoal{
						{
							Type:         session.GoalTypeExploration,
							Status:       session.GoalStatusActive,
							CurrentValue: 1,
							TargetValue:  3,
						},
					},
				}
				return &mockSessionRepo{
					getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
						return gs, nil
					},
					saveFunc: func(ctx context.Context, gs *session.GameSession) error {
						return nil
					},
				}
			},
			request: UpdateGoalProgressRequest{
				ChatID:      12345,
				GoalType:    session.GoalTypeExploration,
				IncrementBy: 1,
			},
			expectedError: false,
			expectedSaved: false,
		},
		{
			name: "no update - goal already completed",
			setupMock: func() *mockSessionRepo {
				gs := &session.GameSession{
					State: session.StateActive,
					SessionGoals: []session.SessionGoal{
						{
							Type:         session.GoalTypeExploration,
							Status:       session.GoalStatusCompleted,
							CurrentValue: 3,
							TargetValue:  3,
						},
					},
				}
				return &mockSessionRepo{
					getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
						return gs, nil
					},
					saveFunc: func(ctx context.Context, gs *session.GameSession) error {
						return nil
					},
				}
			},
			request: UpdateGoalProgressRequest{
				ChatID:      12345,
				GoalType:    session.GoalTypeExploration,
				IncrementBy: 1,
			},
			expectedError: false,
			expectedSaved: false,
		},
		{
			name: "error - save failed",
			setupMock: func() *mockSessionRepo {
				gs := &session.GameSession{
					State: session.StateActive,
					SessionGoals: []session.SessionGoal{
						{
							Type:         session.GoalTypeExploration,
							Status:       session.GoalStatusActive,
							CurrentValue: 1,
							TargetValue:  3,
						},
					},
				}
				return &mockSessionRepo{
					getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
						return gs, nil
					},
					saveFunc: func(ctx context.Context, gs *session.GameSession) error {
						return errors.New("save failed")
					},
				}
			},
			request: UpdateGoalProgressRequest{
				ChatID:      12345,
				GoalType:    session.GoalTypeExploration,
				IncrementBy: 1,
			},
			expectedError: true,
			expectedSaved: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := tt.setupMock()
			uc := NewManageSessionGoalsUseCase(mockRepo)

			saved := false
			if mockRepo.saveFunc != nil {
				originalSave := mockRepo.saveFunc
				mockRepo.saveFunc = func(ctx context.Context, gs *session.GameSession) error {
					saved = true
					return originalSave(ctx, gs)
				}
			}

			err := uc.UpdateGoalProgress(context.Background(), tt.request)

			if tt.expectedError && err == nil {
				t.Error("expected error, but got none")
			}
			if !tt.expectedError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if saved != tt.expectedSaved {
				t.Errorf("expected saved=%v, got saved=%v", tt.expectedSaved, saved)
			}

			if !tt.expectedError && tt.expectedStatus != "" && saved {
				// Check if the goal status was updated correctly
				gs, _ := mockRepo.GetByChatID(context.Background(), tt.request.ChatID)
				for _, goal := range gs.SessionGoals {
					if goal.Type == tt.request.GoalType {
						if goal.Status != tt.expectedStatus {
							t.Errorf("expected goal status %v, got %v", tt.expectedStatus, goal.Status)
						}
						break
					}
				}
			}
		})
	}
}

func TestManageSessionGoalsUseCase_CheckSessionExpiredGoals(t *testing.T) {
	tests := []struct {
		name          string
		setupMock     func() *mockSessionRepo
		chatID        int64
		expectedError bool
		expectedSaved bool
	}{
		{
			name: "success - expire goals",
			setupMock: func() *mockSessionRepo {
				pastTime := time.Now().Add(-time.Hour)
				gs := &session.GameSession{
					State: session.StateActive,
					SessionGoals: []session.SessionGoal{
						{
							Type:        session.GoalTypeExploration,
							Status:      session.GoalStatusActive,
							TimeLimit:   &pastTime,
							Description: "Explore locations",
						},
					},
				}
				return &mockSessionRepo{
					getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
						return gs, nil
					},
					saveFunc: func(ctx context.Context, gs *session.GameSession) error {
						return nil
					},
				}
			},
			chatID:        12345,
			expectedError: false,
			expectedSaved: true,
		},
		{
			name: "success - no expired goals",
			setupMock: func() *mockSessionRepo {
				futureTime := time.Now().Add(time.Hour)
				gs := &session.GameSession{
					State: session.StateActive,
					SessionGoals: []session.SessionGoal{
						{
							Type:      session.GoalTypeExploration,
							Status:    session.GoalStatusActive,
							TimeLimit: &futureTime,
						},
					},
				}
				return &mockSessionRepo{
					getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
						return gs, nil
					},
					saveFunc: func(ctx context.Context, gs *session.GameSession) error {
						return nil
					},
				}
			},
			chatID:        12345,
			expectedError: false,
			expectedSaved: false,
		},
		{
			name: "no action - session not active",
			setupMock: func() *mockSessionRepo {
				pastTime := time.Now().Add(-time.Hour)
				gs := &session.GameSession{
					State: session.StateDone,
					SessionGoals: []session.SessionGoal{
						{
							Type:      session.GoalTypeExploration,
							Status:    session.GoalStatusActive,
							TimeLimit: &pastTime,
						},
					},
				}
				return &mockSessionRepo{
					getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
						return gs, nil
					},
					saveFunc: func(ctx context.Context, gs *session.GameSession) error {
						return nil
					},
				}
			},
			chatID:        12345,
			expectedError: false,
			expectedSaved: false,
		},
		{
			name: "error - session not found",
			setupMock: func() *mockSessionRepo {
				return &mockSessionRepo{
					getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
						return nil, errors.New("session not found")
					},
				}
			},
			chatID:        12345,
			expectedError: true,
			expectedSaved: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := tt.setupMock()
			uc := NewManageSessionGoalsUseCase(mockRepo)

			saved := false
			if mockRepo.saveFunc != nil {
				originalSave := mockRepo.saveFunc
				mockRepo.saveFunc = func(ctx context.Context, gs *session.GameSession) error {
					saved = true
					return originalSave(ctx, gs)
				}
			}

			err := uc.CheckSessionExpiredGoals(context.Background(), tt.chatID)

			if tt.expectedError && err == nil {
				t.Error("expected error, but got none")
			}
			if !tt.expectedError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if saved != tt.expectedSaved {
				t.Errorf("expected saved=%v, got saved=%v", tt.expectedSaved, saved)
			}
		})
	}
}

func TestManageSessionGoalsUseCase_GetSessionGoals(t *testing.T) {
	tests := []struct {
		name           string
		setupMock      func() *mockSessionRepo
		chatID         int64
		expectedError  bool
		expectedGoals  int
	}{
		{
			name: "success - get goals",
			setupMock: func() *mockSessionRepo {
				now := time.Now()
				futureTime := now.Add(time.Hour)
				gs := &session.GameSession{
					State: session.StateActive,
					SessionGoals: []session.SessionGoal{
						{
							Type:        session.GoalTypeExploration,
							Description: "Explore 3 locations",
							Status:      session.GoalStatusActive,
							CurrentValue: 1,
							TargetValue:  3,
							TimeLimit:    &futureTime,
						},
						{
							Type:        session.GoalTypeCombat,
							Description: "Win 2 battles",
							Status:      session.GoalStatusCompleted,
							CurrentValue: 2,
							TargetValue:  2,
							TimeLimit:    &futureTime,
						},
					},
				}
				return &mockSessionRepo{
					getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
						return gs, nil
					},
				}
			},
			chatID:        12345,
			expectedError: false,
			expectedGoals: 2,
		},
		{
			name: "success - empty goals",
			setupMock: func() *mockSessionRepo {
				gs := &session.GameSession{
					State:        session.StateActive,
					SessionGoals: []session.SessionGoal{},
				}
				return &mockSessionRepo{
					getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
						return gs, nil
					},
				}
			},
			chatID:        12345,
			expectedError: false,
			expectedGoals: 0,
		},
		{
			name: "error - session not found",
			setupMock: func() *mockSessionRepo {
				return &mockSessionRepo{
					getByChatIDFunc: func(ctx context.Context, chatID int64) (*session.GameSession, error) {
						return nil, errors.New("session not found")
					},
				}
			},
			chatID:        12345,
			expectedError: true,
			expectedGoals: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := tt.setupMock()
			uc := NewManageSessionGoalsUseCase(mockRepo)

			response, err := uc.GetSessionGoals(context.Background(), tt.chatID)

			if tt.expectedError && err == nil {
				t.Error("expected error, but got none")
			}
			if !tt.expectedError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if !tt.expectedError {
				if response == nil {
					t.Error("expected response, got nil")
					return
				}
				if len(response.Goals) != tt.expectedGoals {
					t.Errorf("expected %d goals, got %d", tt.expectedGoals, len(response.Goals))
				}
			}
		})
	}
}