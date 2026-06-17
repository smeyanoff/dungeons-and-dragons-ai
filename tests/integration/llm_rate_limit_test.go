package integration

import (
	"context"
	"os"
	"strconv"
	"time"

	llmdomain "dungeons-and-dragons-ai/internal/llm/domain"
	llminfra "dungeons-and-dragons-ai/internal/llm/infrastructure"
	llmtools "dungeons-and-dragons-ai/internal/llm/domain/tools"
)

// rateLimitedLLM is a test-only wrapper that enforces a minimum delay between LLM calls.
// This protects the provider from accidental bursts when running the full integration suite.
type rateLimitedLLM struct {
	inner   llmdomain.LLM
	limiter *llminfra.SimpleLLMRateLimiter
}

func wrapLLMWithTestRateLimit(inner llmdomain.LLM) llmdomain.LLM {
	// Default is intentionally conservative for integration suites.
	delay := 2500 * time.Millisecond
	if v := os.Getenv("LLM_TEST_MIN_DELAY_MS"); v != "" {
		if ms, err := strconv.Atoi(v); err == nil && ms >= 0 {
			delay = time.Duration(ms) * time.Millisecond
		}
	}
	return &rateLimitedLLM{
		inner:   inner,
		limiter: llminfra.NewSimpleLLMRateLimiter(delay),
	}
}

func (l *rateLimitedLLM) wait(ctx context.Context) error {
	if l == nil || l.limiter == nil {
		return nil
	}
	return l.limiter.Wait(ctx)
}

func (l *rateLimitedLLM) Generate(ctx context.Context, prompt string) (string, error) {
	if err := l.wait(ctx); err != nil {
		return "", err
	}
	return l.inner.Generate(ctx, prompt)
}

func (l *rateLimitedLLM) GenerateWithMaxTokens(ctx context.Context, prompt string, maxTokens int) (string, error) {
	if err := l.wait(ctx); err != nil {
		return "", err
	}
	return l.inner.GenerateWithMaxTokens(ctx, prompt, maxTokens)
}

func (l *rateLimitedLLM) GenerateWithTools(ctx context.Context, prompt string, tools []llmtools.Tool) (*llmdomain.LLMResponseWithTools, error) {
	if err := l.wait(ctx); err != nil {
		return nil, err
	}
	return l.inner.GenerateWithTools(ctx, prompt, tools)
}
