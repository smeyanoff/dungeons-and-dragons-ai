package achievement

import (
	"context"
	"errors"
	"testing"
	"time"

	"dungeons-and-dragons-ai/internal/game/domain/achievement"
	"dungeons-and-dragons-ai/internal/game/domain/player"
)

// Mock Achievement Repository
type mockCheckAchievementsRepo struct {
	getAllFunc                     func(ctx context.Context) ([]*achievement.Achievement, error)
	getByCodeFunc                  func(ctx context.Context, code string) (*achievement.Achievement, error)
	getPlayerAchievementsFunc      func(ctx context.Context, playerID uint) ([]*achievement.PlayerAchievement, error)
	getPlayerAchievementByCodeFunc func(ctx context.Context, playerID uint, code string) (*achievement.PlayerAchievement, error)
	getAchievementProgressFunc     func(ctx context.Context, playerID uint, achievementID uint) (*achievement.AchievementProgress, error)
	saveAchievementProgressFunc    func(ctx context.Context, progress *achievement.AchievementProgress) error
	savePlayerAchievementFunc      func(ctx context.Context, playerAchievement *achievement.PlayerAchievement) error
}

func (m *mockCheckAchievementsRepo) GetAll(ctx context.Context) ([]*achievement.Achievement, error) {
	if m.getAllFunc != nil {
		return m.getAllFunc(ctx)
	}
	return []*achievement.Achievement{}, nil
}

func (m *mockCheckAchievementsRepo) GetByCode(ctx context.Context, code string) (*achievement.Achievement, error) {
	if m.getByCodeFunc != nil {
		return m.getByCodeFunc(ctx, code)
	}
	return nil, nil
}

func (m *mockCheckAchievementsRepo) GetPlayerAchievements(ctx context.Context, playerID uint) ([]*achievement.PlayerAchievement, error) {
	if m.getPlayerAchievementsFunc != nil {
		return m.getPlayerAchievementsFunc(ctx, playerID)
	}
	return []*achievement.PlayerAchievement{}, nil
}

func (m *mockCheckAchievementsRepo) GetPlayerAchievementByCode(ctx context.Context, playerID uint, code string) (*achievement.PlayerAchievement, error) {
	if m.getPlayerAchievementByCodeFunc != nil {
		return m.getPlayerAchievementByCodeFunc(ctx, playerID, code)
	}
	return nil, nil
}

func (m *mockCheckAchievementsRepo) GetAchievementProgress(ctx context.Context, playerID uint, achievementID uint) (*achievement.AchievementProgress, error) {
	if m.getAchievementProgressFunc != nil {
		return m.getAchievementProgressFunc(ctx, playerID, achievementID)
	}
	return nil, nil
}

func (m *mockCheckAchievementsRepo) SaveAchievementProgress(ctx context.Context, progress *achievement.AchievementProgress) error {
	if m.saveAchievementProgressFunc != nil {
		return m.saveAchievementProgressFunc(ctx, progress)
	}
	return nil
}

func (m *mockCheckAchievementsRepo) SavePlayerAchievement(ctx context.Context, playerAchievement *achievement.PlayerAchievement) error {
	if m.savePlayerAchievementFunc != nil {
		return m.savePlayerAchievementFunc(ctx, playerAchievement)
	}
	return nil
}

// Mock Player Repository (not used in CheckAchievementsUseCase, but needed for constructor)
type mockCheckPlayerRepo struct{}

func (m *mockCheckPlayerRepo) GetByID(ctx context.Context, id uint) (*player.Player, error) {
	return nil, nil
}

func (m *mockCheckPlayerRepo) GetByTgUserIDAndSessionID(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
	return nil, nil
}

func (m *mockCheckPlayerRepo) Save(ctx context.Context, player *player.Player) error {
	return nil
}

func (m *mockCheckPlayerRepo) GetBySessionID(ctx context.Context, sessionID uint) ([]*player.Player, error) {
	return nil, nil
}

func TestCheckAchievementsUseCase_Execute_NewAchievementUnlocked(t *testing.T) {
	ctx := context.Background()

	// Create test achievement
	testAchievement := &achievement.Achievement{
		ID:                1,
		Code:              "first_quest_completed",
		Title:             "Первый квест",
		Description:       "Завершите свой первый квест",
		Type:              achievement.AchievementTypeQuest,
		Rarity:            achievement.RarityCommon,
		RequirementValue:  1,
		RequirementKey:    "quests_completed",
		ExperienceReward:  50,
		GoldReward:        10,
		Icon:              "🏆",
		IsHidden:          false,
		IsRepeatable:      false,
	}

	achievementRepo := &mockCheckAchievementsRepo{
		getAllFunc: func(ctx context.Context) ([]*achievement.Achievement, error) {
			return []*achievement.Achievement{testAchievement}, nil
		},
		getPlayerAchievementByCodeFunc: func(ctx context.Context, playerID uint, code string) (*achievement.PlayerAchievement, error) {
			return nil, nil // Achievement not earned yet
		},
		getAchievementProgressFunc: func(ctx context.Context, playerID uint, achievementID uint) (*achievement.AchievementProgress, error) {
			return nil, nil // No progress yet
		},
		saveAchievementProgressFunc: func(ctx context.Context, progress *achievement.AchievementProgress) error {
			return nil
		},
		savePlayerAchievementFunc: func(ctx context.Context, playerAchievement *achievement.PlayerAchievement) error {
			return nil
		},
	}

	playerRepo := &mockCheckPlayerRepo{}

	uc := NewCheckAchievementsUseCase(achievementRepo, playerRepo)

	req := CheckAchievementsRequest{
		PlayerID:     1,
		RequirementKey: "quests_completed",
		CurrentValue: 1,
	}

	result, err := uc.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if len(result) != 1 {
		t.Errorf("Execute() returned %d achievements, want 1", len(result))
	}

	if result[0].Achievement.ID != testAchievement.ID {
		t.Errorf("Execute() returned achievement ID %d, want %d", result[0].Achievement.ID, testAchievement.ID)
	}

	if !contains(result[0].Message, "Достижение разблокировано") {
		t.Error("Execute() message should contain 'Достижение разблокировано'")
	}
}

func TestCheckAchievementsUseCase_Execute_AlreadyEarnedNonRepeatable(t *testing.T) {
	ctx := context.Background()

	testAchievement := &achievement.Achievement{
		ID:               1,
		Code:             "first_quest_completed",
		RequirementValue: 1,
		RequirementKey:   "quests_completed",
		IsRepeatable:     false,
	}

	existingAchievement := &achievement.PlayerAchievement{
		ID:            1,
		PlayerID:      1,
		AchievementID: 1,
		Progress:      1,
		EarnedAt:      time.Now(),
		EarnedCount:   1,
	}

	achievementRepo := &mockCheckAchievementsRepo{
		getAllFunc: func(ctx context.Context) ([]*achievement.Achievement, error) {
			return []*achievement.Achievement{testAchievement}, nil
		},
		getPlayerAchievementByCodeFunc: func(ctx context.Context, playerID uint, code string) (*achievement.PlayerAchievement, error) {
			return existingAchievement, nil
		},
	}

	playerRepo := &mockCheckPlayerRepo{}

	uc := NewCheckAchievementsUseCase(achievementRepo, playerRepo)

	req := CheckAchievementsRequest{
		PlayerID:       1,
		RequirementKey: "quests_completed",
		CurrentValue:   2, // Even higher value
	}

	result, err := uc.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if len(result) != 0 {
		t.Errorf("Execute() returned %d achievements, want 0 (already earned non-repeatable)", len(result))
	}
}

func TestCheckAchievementsUseCase_Execute_RepeatableAchievement(t *testing.T) {
	ctx := context.Background()

	testAchievement := &achievement.Achievement{
		ID:               1,
		Code:             "combat_victory",
		RequirementValue: 5,
		RequirementKey:   "combat_wins",
		IsRepeatable:     true,
		ExperienceReward: 25,
		GoldReward:       5,
	}

	existingAchievement := &achievement.PlayerAchievement{
		ID:            1,
		PlayerID:      1,
		AchievementID: 1,
		Progress:      5,
		EarnedAt:      time.Now(),
		EarnedCount:   1,
	}

	achievementRepo := &mockCheckAchievementsRepo{
		getAllFunc: func(ctx context.Context) ([]*achievement.Achievement, error) {
			return []*achievement.Achievement{testAchievement}, nil
		},
		getPlayerAchievementByCodeFunc: func(ctx context.Context, playerID uint, code string) (*achievement.PlayerAchievement, error) {
			return existingAchievement, nil
		},
		getAchievementProgressFunc: func(ctx context.Context, playerID uint, achievementID uint) (*achievement.AchievementProgress, error) {
			return &achievement.AchievementProgress{
				PlayerID:      1,
				AchievementID: 1,
				CurrentValue:  9, // 4 more wins needed for next milestone
			}, nil
		},
		saveAchievementProgressFunc: func(ctx context.Context, progress *achievement.AchievementProgress) error {
			return nil
		},
		savePlayerAchievementFunc: func(ctx context.Context, playerAchievement *achievement.PlayerAchievement) error {
			return nil
		},
	}

	playerRepo := &mockCheckPlayerRepo{}

	uc := NewCheckAchievementsUseCase(achievementRepo, playerRepo)

	req := CheckAchievementsRequest{
		PlayerID:       1,
		RequirementKey: "combat_wins",
		CurrentValue:   1, // Additional win
	}

	result, err := uc.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	// For repeatable achievements that are already earned, no new unlocks are returned
	// The progress is just updated
	if len(result) != 0 {
		t.Errorf("Execute() returned %d achievements, want 0 (repeatable achievement already earned)", len(result))
	}
}

func TestCheckAchievementsUseCase_Execute_ProgressUpdateOnly(t *testing.T) {
	ctx := context.Background()

	testAchievement := &achievement.Achievement{
		ID:               1,
		Code:             "explorer",
		RequirementValue: 10,
		RequirementKey:   "locations_visited",
		IsRepeatable:     false,
	}

	achievementRepo := &mockCheckAchievementsRepo{
		getAllFunc: func(ctx context.Context) ([]*achievement.Achievement, error) {
			return []*achievement.Achievement{testAchievement}, nil
		},
		getPlayerAchievementByCodeFunc: func(ctx context.Context, playerID uint, code string) (*achievement.PlayerAchievement, error) {
			return nil, nil // Not earned yet
		},
		getAchievementProgressFunc: func(ctx context.Context, playerID uint, achievementID uint) (*achievement.AchievementProgress, error) {
			return &achievement.AchievementProgress{
				PlayerID:     1,
				AchievementID: 1,
				CurrentValue: 7, // 7 out of 10
			}, nil
		},
		saveAchievementProgressFunc: func(ctx context.Context, progress *achievement.AchievementProgress) error {
			return nil
		},
	}

	playerRepo := &mockCheckPlayerRepo{}

	uc := NewCheckAchievementsUseCase(achievementRepo, playerRepo)

	req := CheckAchievementsRequest{
		PlayerID:       1,
		RequirementKey: "locations_visited",
		CurrentValue:   2, // Visit 2 more locations
	}

	result, err := uc.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if len(result) != 0 {
		t.Errorf("Execute() returned %d achievements, want 0 (only progress update)", len(result))
	}
}

func TestCheckAchievementsUseCase_Execute_RepositoryError(t *testing.T) {
	ctx := context.Background()

	achievementRepo := &mockCheckAchievementsRepo{
		getAllFunc: func(ctx context.Context) ([]*achievement.Achievement, error) {
			return nil, errors.New("database error")
		},
	}

	playerRepo := &mockCheckPlayerRepo{}

	uc := NewCheckAchievementsUseCase(achievementRepo, playerRepo)

	req := CheckAchievementsRequest{
		PlayerID:       1,
		RequirementKey: "quests_completed",
		CurrentValue:   1,
	}

	_, err := uc.Execute(ctx, req)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}

	if !contains(err.Error(), "failed to get achievements") {
		t.Errorf("Execute() error = %v, want 'failed to get achievements'", err)
	}
}

func TestCheckAchievementsUseCase_Execute_NoMatchingAchievements(t *testing.T) {
	ctx := context.Background()

	testAchievement := &achievement.Achievement{
		ID:             1,
		RequirementKey: "combat_wins",
	}

	achievementRepo := &mockCheckAchievementsRepo{
		getAllFunc: func(ctx context.Context) ([]*achievement.Achievement, error) {
			return []*achievement.Achievement{testAchievement}, nil
		},
	}

	playerRepo := &mockCheckPlayerRepo{}

	uc := NewCheckAchievementsUseCase(achievementRepo, playerRepo)

	req := CheckAchievementsRequest{
		PlayerID:       1,
		RequirementKey: "quests_completed", // Different key
		CurrentValue:   1,
	}

	result, err := uc.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if len(result) != 0 {
		t.Errorf("Execute() returned %d achievements, want 0 (no matching requirements)", len(result))
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsAt(s, substr)))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}