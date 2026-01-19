package quest

import (
	"testing"
	"time"
)

func TestNewDailyQuest(t *testing.T) {
	tests := []struct {
		name        string
		questType   DailyQuestType
		title       string
		description string
		targetValue int
		expReward   int
		goldReward  int
		wantType    DailyQuestType
		wantTitle   string
	}{
		{
			name:        "complete quest type",
			questType:   DailyQuestTypeCompleteQuest,
			title:       "Завершить квест",
			description: "Завершите любой активный квест",
			targetValue: 1,
			expReward:   50,
			goldReward:  10,
			wantType:    DailyQuestTypeCompleteQuest,
			wantTitle:   "Завершить квест",
		},
		{
			name:        "win combat type",
			questType:   DailyQuestTypeWinCombat,
			title:       "Победить в бою",
			description: "Одолейте врагов в бою",
			targetValue: 1,
			expReward:   75,
			goldReward:  15,
			wantType:    DailyQuestTypeWinCombat,
			wantTitle:   "Победить в бою",
		},
		{
			name:        "explore location type",
			questType:   DailyQuestTypeExploreLocation,
			title:       "Исследовать локацию",
			description: "Посетите новую локацию в мире",
			targetValue: 1,
			expReward:   25,
			goldReward:  5,
			wantType:    DailyQuestTypeExploreLocation,
			wantTitle:   "Исследовать локацию",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewDailyQuest(tt.questType, tt.title, tt.description, tt.targetValue, tt.expReward, tt.goldReward)

			if got.Type != tt.wantType {
				t.Errorf("NewDailyQuest() Type = %v, want %v", got.Type, tt.wantType)
			}
			if got.Title != tt.wantTitle {
				t.Errorf("NewDailyQuest() Title = %v, want %v", got.Title, tt.wantTitle)
			}
			if got.Description != tt.description {
				t.Errorf("NewDailyQuest() Description = %v, want %v", got.Description, tt.description)
			}
			if got.TargetValue != tt.targetValue {
				t.Errorf("NewDailyQuest() TargetValue = %v, want %v", got.TargetValue, tt.targetValue)
			}
			if got.ExperienceReward != tt.expReward {
				t.Errorf("NewDailyQuest() ExperienceReward = %v, want %v", got.ExperienceReward, tt.expReward)
			}
			if got.GoldReward != tt.goldReward {
				t.Errorf("NewDailyQuest() GoldReward = %v, want %v", got.GoldReward, tt.goldReward)
			}
			if got.CreatedAt.IsZero() {
				t.Error("NewDailyQuest() CreatedAt should be set")
			}
		})
	}
}

func TestDailyQuestProgress_IsCompleted(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name        string
		progress    *DailyQuestProgress
		want        bool
	}{
		{
			name: "not completed - current less than target",
			progress: &DailyQuestProgress{
				CurrentValue: 0,
				TargetValue:  1,
				Completed:    false,
			},
			want: false,
		},
		{
			name: "completed - current equals target",
			progress: &DailyQuestProgress{
				CurrentValue: 1,
				TargetValue:  1,
				Completed:    false,
			},
			want: true,
		},
		{
			name: "completed - current greater than target",
			progress: &DailyQuestProgress{
				CurrentValue: 2,
				TargetValue:  1,
				Completed:    false,
			},
			want: true,
		},
		{
			name: "completed - marked as completed",
			progress: &DailyQuestProgress{
				CurrentValue: 0,
				TargetValue:  1,
				Completed:    true,
				CompletedAt:  &now,
			},
			want: true,
		},
		{
			name: "completed - marked as completed even if current less than target",
			progress: &DailyQuestProgress{
				CurrentValue: 0,
				TargetValue:  5,
				Completed:    true,
				CompletedAt:  &now,
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.progress.IsCompleted(); got != tt.want {
				t.Errorf("DailyQuestProgress.IsCompleted() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDailyQuestProgress_IncrementProgress(t *testing.T) {
	tests := []struct {
		name           string
		initialValue   int
		targetValue    int
		increment      int
		wantValue      int
		wantCompleted  bool
	}{
		{
			name:          "increment by 1",
			initialValue:  0,
			targetValue:   3,
			increment:     1,
			wantValue:     1,
			wantCompleted: false,
		},
		{
			name:          "increment by multiple",
			initialValue:  1,
			targetValue:   3,
			increment:     2,
			wantValue:     3,
			wantCompleted: true,
		},
		{
			name:          "increment completes quest",
			initialValue:  0,
			targetValue:   1,
			increment:     1,
			wantValue:     1,
			wantCompleted: true,
		},
		{
			name:          "increment exceeds target",
			initialValue:  0,
			targetValue:   1,
			increment:     5,
			wantValue:     5,
			wantCompleted: true,
		},
		{
			name:          "no increment if already completed",
			initialValue:  1,
			targetValue:   1,
			increment:     1,
			wantValue:     1,
			wantCompleted: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			progress := &DailyQuestProgress{
				CurrentValue: tt.initialValue,
				TargetValue:  tt.targetValue,
				Completed:    tt.initialValue >= tt.targetValue,
			}

			progress.IncrementProgress(tt.increment)

			if progress.CurrentValue != tt.wantValue {
				t.Errorf("DailyQuestProgress.IncrementProgress() CurrentValue = %v, want %v", progress.CurrentValue, tt.wantValue)
			}
			if progress.IsCompleted() != tt.wantCompleted {
				t.Errorf("DailyQuestProgress.IncrementProgress() IsCompleted = %v, want %v", progress.IsCompleted(), tt.wantCompleted)
			}
		})
	}
}

func TestDailyQuestProgress_Complete(t *testing.T) {
	tests := []struct {
		name        string
		progress    *DailyQuestProgress
		wantCompleted bool
		wantCompletedAtSet bool
	}{
		{
			name: "complete uncompleted quest",
			progress: &DailyQuestProgress{
				CurrentValue: 0,
				TargetValue:  1,
				Completed:    false,
				CompletedAt: nil,
			},
			wantCompleted: true,
			wantCompletedAtSet: true,
		},
		{
			name: "complete already completed quest",
			progress: &DailyQuestProgress{
				CurrentValue: 1,
				TargetValue:  1,
				Completed:    true,
				CompletedAt: func() *time.Time { t := time.Now(); return &t }(),
			},
			wantCompleted: true,
			wantCompletedAtSet: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			beforeTime := time.Now()
			tt.progress.Complete()
			afterTime := time.Now().Add(10 * time.Millisecond) // Добавляем небольшой запас

			if tt.progress.Completed != tt.wantCompleted {
				t.Errorf("DailyQuestProgress.Complete() Completed = %v, want %v", tt.progress.Completed, tt.wantCompleted)
			}
			if tt.wantCompletedAtSet {
				if tt.progress.CompletedAt == nil {
					t.Error("DailyQuestProgress.Complete() CompletedAt should be set")
				} else {
					if tt.progress.CompletedAt.Before(beforeTime.Add(-10*time.Millisecond)) || tt.progress.CompletedAt.After(afterTime) {
						t.Errorf("DailyQuestProgress.Complete() CompletedAt = %v, should be between %v and %v", 
							tt.progress.CompletedAt, beforeTime, afterTime)
					}
				}
			}
		})
	}
}

func TestGetDailyQuestTypes(t *testing.T) {
	got := GetDailyQuestTypes()
	
	expectedTypes := []DailyQuestType{
		DailyQuestTypeCompleteQuest,
		DailyQuestTypeWinCombat,
		DailyQuestTypeExploreLocation,
	}

	if len(got) != len(expectedTypes) {
		t.Errorf("GetDailyQuestTypes() returned %d types, want %d", len(got), len(expectedTypes))
	}

	typeMap := make(map[DailyQuestType]bool)
	for _, t := range got {
		typeMap[t] = true
	}

	for _, expectedType := range expectedTypes {
		if !typeMap[expectedType] {
			t.Errorf("GetDailyQuestTypes() missing type %v", expectedType)
		}
	}
}
