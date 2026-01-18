package image

import (
	"context"
	"testing"
	"time"
)

func TestInMemoryRateLimiter_CheckLimit(t *testing.T) {
	tests := []struct {
		name      string
		limit     int
		recordGen int
		expected  bool
	}{
		{
			name:      "no records can generate",
			limit:     5,
			recordGen: 0,
			expected:  true,
		},
		{
			name:      "within limit",
			limit:     5,
			recordGen: 3,
			expected:  true,
		},
		{
			name:      "at limit cannot generate",
			limit:     5,
			recordGen: 5,
			expected:  false,
		},
		{
			name:      "over limit cannot generate",
			limit:     5,
			recordGen: 6,
			expected:  false,
		},
		{
			name:      "limit 1 can generate once",
			limit:     1,
			recordGen: 0,
			expected:  true,
		},
		{
			name:      "limit 1 cannot generate twice",
			limit:     1,
			recordGen: 1,
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limiter := NewInMemoryRateLimiter(tt.limit)
			userID := int64(12345)

			// Record generations
			for i := 0; i < tt.recordGen; i++ {
				err := limiter.RecordGeneration(context.Background(), userID)
				if err != nil {
					t.Fatalf("RecordGeneration failed: %v", err)
				}
			}

			result, err := limiter.CheckLimit(context.Background(), userID)
			if err != nil {
				t.Fatalf("CheckLimit failed: %v", err)
			}

			if result != tt.expected {
				t.Errorf("CheckLimit() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestInMemoryRateLimiter_GetRemainingQuota(t *testing.T) {
	tests := []struct {
		name         string
		limit        int
		recordGen    int
		expectedQuota int
	}{
		{
			name:         "no records full quota",
			limit:        5,
			recordGen:    0,
			expectedQuota: 5,
		},
		{
			name:         "some records partial quota",
			limit:        5,
			recordGen:    2,
			expectedQuota: 3,
		},
		{
			name:         "at limit zero quota",
			limit:        5,
			recordGen:    5,
			expectedQuota: 0,
		},
		{
			name:         "over limit zero quota",
			limit:        5,
			recordGen:    10,
			expectedQuota: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limiter := NewInMemoryRateLimiter(tt.limit)
			userID := int64(12345)

			// Record generations
			for i := 0; i < tt.recordGen; i++ {
				err := limiter.RecordGeneration(context.Background(), userID)
				if err != nil {
					t.Fatalf("RecordGeneration failed: %v", err)
				}
			}

			quota, err := limiter.GetRemainingQuota(context.Background(), userID)
			if err != nil {
				t.Fatalf("GetRemainingQuota failed: %v", err)
			}

			if quota != tt.expectedQuota {
				t.Errorf("GetRemainingQuota() = %d, expected %d", quota, tt.expectedQuota)
			}
		})
	}
}

func TestInMemoryRateLimiter_DifferentUsers(t *testing.T) {
	limiter := NewInMemoryRateLimiter(5)
	user1 := int64(11111)
	user2 := int64(22222)

	// User 1 generates 3 images
	for i := 0; i < 3; i++ {
		err := limiter.RecordGeneration(context.Background(), user1)
		if err != nil {
			t.Fatalf("RecordGeneration failed: %v", err)
		}
	}

	// User 2 should have full quota
	canGenerate2, err := limiter.CheckLimit(context.Background(), user2)
	if err != nil {
		t.Fatalf("CheckLimit failed: %v", err)
	}
	if !canGenerate2 {
		t.Error("User 2 should be able to generate")
	}

	quota2, err := limiter.GetRemainingQuota(context.Background(), user2)
	if err != nil {
		t.Fatalf("GetRemainingQuota failed: %v", err)
	}
	if quota2 != 5 {
		t.Errorf("User 2 quota = %d, expected 5", quota2)
	}

	// User 1 should have remaining quota
	canGenerate1, err := limiter.CheckLimit(context.Background(), user1)
	if err != nil {
		t.Fatalf("CheckLimit failed: %v", err)
	}
	if !canGenerate1 {
		t.Error("User 1 should still be able to generate")
	}

	quota1, err := limiter.GetRemainingQuota(context.Background(), user1)
	if err != nil {
		t.Fatalf("GetRemainingQuota failed: %v", err)
	}
	if quota1 != 2 {
		t.Errorf("User 1 quota = %d, expected 2", quota1)
	}
}

func TestInMemoryRateLimiter_CleanupOldRecords(t *testing.T) {
	limiter := NewInMemoryRateLimiter(5)
	userID := int64(12345)

	// Record generation 8 days ago (should be cleaned up)
	oldTime := time.Now().AddDate(0, 0, -8)
	limiter.records[userID] = []time.Time{oldTime}

	// Record a recent generation
	err := limiter.RecordGeneration(context.Background(), userID)
	if err != nil {
		t.Fatalf("RecordGeneration failed: %v", err)
	}

	// Old record should be cleaned up
	records := limiter.records[userID]
	if len(records) != 1 {
		t.Errorf("Expected 1 record after cleanup, got %d", len(records))
	}

	// Remaining quota should be 4 (5 - 1)
	quota, err := limiter.GetRemainingQuota(context.Background(), userID)
	if err != nil {
		t.Fatalf("GetRemainingQuota failed: %v", err)
	}
	if quota != 4 {
		t.Errorf("Quota after cleanup = %d, expected 4", quota)
	}
}
