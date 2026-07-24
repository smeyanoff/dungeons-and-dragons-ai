package dm_tools

import (
	"context"
	"errors"
	"testing"

	"dungeons-and-dragons-ai/internal/game/domain/world"
)

// mockCampaignFactRepository мок для CampaignFactRepository
type mockCampaignFactRepository struct {
	saveFunc func(ctx context.Context, fact *world.CampaignFact) error
	saved    []*world.CampaignFact
}

func (m *mockCampaignFactRepository) Save(ctx context.Context, fact *world.CampaignFact) error {
	if m.saveFunc != nil {
		return m.saveFunc(ctx, fact)
	}
	m.saved = append(m.saved, fact)
	return nil
}

func TestSaveCampaignFactTool_Name(t *testing.T) {
	tool := NewSaveCampaignFactTool(nil, 1)
	if tool.Name() != "save_campaign_fact" {
		t.Errorf("expected name 'save_campaign_fact', got '%s'", tool.Name())
	}
}

func TestSaveCampaignFactTool_Description(t *testing.T) {
	tool := NewSaveCampaignFactTool(nil, 1)
	if tool.Description() == "" {
		t.Error("expected non-empty description")
	}
}

func TestSaveCampaignFactTool_Parameters(t *testing.T) {
	tool := NewSaveCampaignFactTool(nil, 1)
	params := tool.Parameters()
	if len(params) == 0 {
		t.Error("expected non-empty parameters")
	}
}

func TestSaveCampaignFactTool_Execute(t *testing.T) {
	tests := []struct {
		name          string
		worldID       uint
		args          map[string]interface{}
		setupMock     func(*mockCampaignFactRepository)
		expectedError bool
		validate      func(*testing.T, *mockCampaignFactRepository, interface{})
	}{
		{
			name:    "valid reputation fact",
			worldID: 42,
			args: map[string]interface{}{
				"category": "reputation",
				"text":     "Игрок прославился как убийца дракона.",
			},
			validate: func(t *testing.T, repo *mockCampaignFactRepository, result interface{}) {
				if len(repo.saved) != 1 {
					t.Fatalf("expected 1 saved fact, got %d", len(repo.saved))
				}
				fact := repo.saved[0]
				if fact.WorldID != 42 {
					t.Errorf("expected WorldID 42, got %d", fact.WorldID)
				}
				if fact.Category != world.FactCategoryReputation {
					t.Errorf("expected category reputation, got %s", fact.Category)
				}
				if fact.Text != "Игрок прославился как убийца дракона." {
					t.Errorf("unexpected text: %s", fact.Text)
				}
			},
		},
		{
			name:    "unknown category falls back to decision",
			worldID: 1,
			args: map[string]interface{}{
				"category": "nonsense",
				"text":     "Что-то важное произошло.",
			},
			validate: func(t *testing.T, repo *mockCampaignFactRepository, result interface{}) {
				if len(repo.saved) != 1 {
					t.Fatalf("expected 1 saved fact, got %d", len(repo.saved))
				}
				if repo.saved[0].Category != world.FactCategoryDecision {
					t.Errorf("expected fallback category decision, got %s", repo.saved[0].Category)
				}
			},
		},
		{
			name:    "empty text returns error",
			worldID: 1,
			args: map[string]interface{}{
				"category": "quest",
				"text":     "",
			},
			expectedError: true,
		},
		{
			name:    "repository error is returned in result, not as Go error",
			worldID: 1,
			args: map[string]interface{}{
				"category": "quest",
				"text":     "Найден первый фрагмент карты.",
			},
			setupMock: func(m *mockCampaignFactRepository) {
				m.saveFunc = func(ctx context.Context, fact *world.CampaignFact) error {
					return errors.New("db error")
				}
			},
			validate: func(t *testing.T, repo *mockCampaignFactRepository, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map result, got %T", result)
				}
				if success, _ := resultMap["success"].(bool); success {
					t.Error("expected success=false when repository fails")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockCampaignFactRepository{}
			if tt.setupMock != nil {
				tt.setupMock(repo)
			}
			tool := NewSaveCampaignFactTool(repo, tt.worldID)

			result, err := tool.Execute(context.Background(), tt.args)

			if tt.expectedError && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.expectedError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.validate != nil {
				tt.validate(t, repo, result)
			}
		})
	}
}
