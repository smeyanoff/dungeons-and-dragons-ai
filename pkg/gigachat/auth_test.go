package gigachat

import "testing"

func TestNormalizeExpiresAt(t *testing.T) {
	now := int64(1_700_000_000)

	t.Run("seconds", func(t *testing.T) {
		expiresAt := now + 3600
		normalized := normalizeExpiresAt(expiresAt, now)
		if normalized != expiresAt {
			t.Fatalf("expected %d, got %d", expiresAt, normalized)
		}
	})

	t.Run("milliseconds", func(t *testing.T) {
		expiresAtMs := (now + 3600) * 1000
		normalized := normalizeExpiresAt(expiresAtMs, now)
		expected := expiresAtMs / 1000
		if normalized != expected {
			t.Fatalf("expected %d, got %d", expected, normalized)
		}
	})
}
