package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"dungeons-and-dragons-ai/internal/game/domain/llm_log"
	"dungeons-and-dragons-ai/internal/game/infrastructure/persistence"
	"dungeons-and-dragons-ai/internal/monitoring"
	"dungeons-and-dragons-ai/pkg/logger"
)

// Simple mock repository for testing
type mockLLMLogRepo struct{}

func (m *mockLLMLogRepo) GetRecent(ctx context.Context, limit int) ([]*llm_log.LLMLog, error) {
	return []*llm_log.LLMLog{}, nil
}
func (m *mockLLMLogRepo) GetByID(ctx context.Context, id uint) (*llm_log.LLMLog, error) {
	return nil, nil
}
func (m *mockLLMLogRepo) GetByChatID(ctx context.Context, chatID int64, limit int) ([]*llm_log.LLMLog, error) {
	return []*llm_log.LLMLog{}, nil
}
func (m *mockLLMLogRepo) GetByTgUserID(ctx context.Context, tgUserID int64, limit int) ([]*llm_log.LLMLog, error) {
	return []*llm_log.LLMLog{}, nil
}
func (m *mockLLMLogRepo) GetByDateRange(ctx context.Context, from, to time.Time, limit int) ([]*llm_log.LLMLog, error) {
	return []*llm_log.LLMLog{}, nil
}
func (m *mockLLMLogRepo) GetWithErrors(ctx context.Context, limit int) ([]*llm_log.LLMLog, error) {
	return []*llm_log.LLMLog{}, nil
}
func (m *mockLLMLogRepo) GetStats(ctx context.Context, from, to time.Time) (*persistence.LLMStats, error) {
	return &persistence.LLMStats{
		TotalRequests:     0,
		TotalErrors:       0,
		AverageDurationMs: 0,
		TotalTokens:       0,
		TotalToolCalls:    0,
		TotalProblems:     0,
	}, nil
}
func (m *mockLLMLogRepo) GetByFilters(ctx context.Context, filters persistence.LLMLogFilters, limit int) ([]*llm_log.LLMLog, error) {
	return []*llm_log.LLMLog{}, nil
}
func (m *mockLLMLogRepo) GetBranches(ctx context.Context, filters persistence.LLMLogFilters, limit int) ([]*persistence.LLMLogBranch, error) {
	return []*persistence.LLMLogBranch{}, nil
}

func main() {
	// Initialize logger
	if err := logger.InitFromEnv(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Info("Starting monitoring test server")

	// Create mock repository
	repo := &mockLLMLogRepo{}

	// Start monitoring server
	server := monitoring.NewServer(":8081", repo)

	logger.Info("Monitoring server starting on :8081")
	log.Fatal(server.Start())
}