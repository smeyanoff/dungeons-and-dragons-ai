package subscription

import (
	"testing"

	"dungeons-and-dragons-ai/internal/game/domain/session"
)

func TestGetOptionalActivityUsage_NoData(t *testing.T) {
	gs := &session.GameSession{}
	if got := GetOptionalActivityUsage(gs, "image_generation"); got != 0 {
		t.Errorf("expected 0 for empty session, got %d", got)
	}
}

func TestIncrementOptionalActivityUsage(t *testing.T) {
	gs := &session.GameSession{}

	if got := IncrementOptionalActivityUsage(gs, "image_generation"); got != 1 {
		t.Errorf("expected 1 after first increment, got %d", got)
	}
	if got := IncrementOptionalActivityUsage(gs, "image_generation"); got != 2 {
		t.Errorf("expected 2 after second increment, got %d", got)
	}
	if got := GetOptionalActivityUsage(gs, "image_generation"); got != 2 {
		t.Errorf("expected GetOptionalActivityUsage to reflect 2, got %d", got)
	}
}

func TestIncrementOptionalActivityUsage_IndependentKeys(t *testing.T) {
	gs := &session.GameSession{}

	IncrementOptionalActivityUsage(gs, "image_generation")
	IncrementOptionalActivityUsage(gs, "image_generation")
	IncrementOptionalActivityUsage(gs, "some_other_activity")

	if got := GetOptionalActivityUsage(gs, "image_generation"); got != 2 {
		t.Errorf("expected image_generation=2, got %d", got)
	}
	if got := GetOptionalActivityUsage(gs, "some_other_activity"); got != 1 {
		t.Errorf("expected some_other_activity=1, got %d", got)
	}
}

func TestGetOptionalActivityUsage_CorruptedJSON(t *testing.T) {
	gs := &session.GameSession{OptionalActivityUsage: []byte("not json")}
	if got := GetOptionalActivityUsage(gs, "image_generation"); got != 0 {
		t.Errorf("expected 0 for corrupted JSON, got %d", got)
	}
}
