package integration

import (
	"context"
	"encoding/json"
	"testing"

	locationeventapp "dungeons-and-dragons-ai/internal/game/application/location_event"
	questdomain "dungeons-and-dragons-ai/internal/game/domain/quest"
	worlddomain "dungeons-and-dragons-ai/internal/game/domain/world"
	"dungeons-and-dragons-ai/internal/game/infrastructure/persistence"
)

func TestLocationEvent_PayloadMetadata_Structure(t *testing.T) {
	cfg := setupInfraOnlyIntegrationTest(t)
	if cfg == nil {
		return
	}
	defer cleanupTest(t, &testConfig{db: cfg.db})

	ctx := cfg.ctx

	q := &questdomain.Quest{Title: "Test Quest (LocationEvent)", Description: "Test quest for location event payload"}
	w := worlddomain.New("Test World (LocationEvent)")
	w.Description = "Deterministic test world for location event payload checks"
	w.SetMainQuest(q)
	w.Locations = []worlddomain.Location{
		{Name: "Start", Description: "Start location"},
	}
	if err := cfg.worldRepo.Save(ctx, w); err != nil {
		t.Fatalf("Не удалось сохранить тестовый мир: %v", err)
	}
	if len(w.Locations) == 0 || w.Locations[0].ID == 0 {
		t.Fatalf("Ожидали сохраненную локацию с ID, получили: %+v", w.Locations)
	}

	loc := w.Locations[0]
	// LocationEventGenerator работает поверх репозитория world events.
	worldEventRepo := persistence.NewWorldEventRepository(cfg.db)
	generator := locationeventapp.NewLocationEventGenerator(worldEventRepo)

	resp, err := generator.Execute(ctx, locationeventapp.GenerateLocationEventRequest{
		WorldID:      w.ID,
		LocationID:   loc.ID,
		LocationName: loc.Name,
		IsFirstVisit: true,
	})
	if err != nil {
		t.Fatalf("generator.Execute: %v", err)
	}
	if resp == nil || resp.Event == nil {
		t.Fatalf("Ожидали сгенерированное событие, получили nil")
	}

	// Проверяем, что метадата — валидный JSON и содержит ключевые поля.
	var meta worlddomain.LocationEventMetadata
	if err := json.Unmarshal(resp.Event.Metadata, &meta); err != nil {
		t.Fatalf("metadata is not valid JSON: %v (raw=%s)", err, string(resp.Event.Metadata))
	}
	if meta.Hook == "" {
		t.Fatalf("metadata.hook пуст")
	}
	if len(meta.Options) < 2 {
		t.Fatalf("metadata.options слишком мало (%d): %#v", len(meta.Options), meta.Options)
	}
	if len(meta.SuggestedChecks) < 1 {
		t.Fatalf("metadata.suggested_checks пуст: %#v", meta.SuggestedChecks)
	}
	if meta.Stakes == "" {
		t.Fatalf("metadata.stakes пуст")
	}
	if meta.Status == "" {
		t.Fatalf("metadata.status пуст")
	}
}

// Ensure context is referenced (test uses cfg.ctx in generator calls, but keep compile-time clarity).
var _ = context.Background
