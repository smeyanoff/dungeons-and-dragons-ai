package gigachat

import (
	"context"
	"testing"
	"time"
)

func TestRateLimiter_Basic(t *testing.T) {
	// Test that rate limiter is properly initialized
	cfg := Config{
		RPSLimit:  10.0,
		RateBurst: 3,
	}

	client := NewClient(cfg)

	if client.rateLimiter == nil {
		t.Fatal("Rate limiter should be initialized")
	}

	// Test that we can get metrics
	metrics := client.GetMetrics()
	if _, exists := metrics["rate_limiter_tokens"]; !exists {
		t.Error("Rate limiter tokens should be in metrics")
	}
}

func TestRateLimiter_Config(t *testing.T) {
	tests := []struct {
		name       string
		rpsLimit   float64
		rateBurst  int
		expectInit bool
	}{
		{"default", 0, 0, true},
		{"custom", 5.0, 2, true},
		{"zero_rps", 0, 5, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				RPSLimit:  tt.rpsLimit,
				RateBurst: tt.rateBurst,
			}

			client := NewClient(cfg)

			if tt.expectInit && client.rateLimiter == nil {
				t.Fatal("Rate limiter should be initialized")
			}
		})
	}
}

func TestRateLimiter_Wait(t *testing.T) {
	cfg := Config{
		RPSLimit:  100.0, // High limit for fast testing
		RateBurst: 10,
	}

	client := NewClient(cfg)

	ctx := context.Background()
	start := time.Now()

	// Make several requests quickly
	for i := 0; i < 5; i++ {
		err := client.rateLimiter.Wait(ctx)
		if err != nil {
			t.Fatalf("Rate limiter wait failed: %v", err)
		}
	}

	elapsed := time.Since(start)
	// Should complete quickly with high limit
	if elapsed > 100*time.Millisecond {
		t.Errorf("Rate limiter too slow: %v", elapsed)
	}
}