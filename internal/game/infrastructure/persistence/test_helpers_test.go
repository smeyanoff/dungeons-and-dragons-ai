package persistence

import (
	"context"
	"testing"
	"time"
)

func waitForContextDone(t *testing.T, ctx context.Context) {
	t.Helper()
	select {
	case <-ctx.Done():
		return
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected context to be done")
	}
}
