package integration

import (
	"context"
	"fmt"

	dm_tools "dungeons-and-dragons-ai/internal/game/application/dm_tools"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/llm/domain"
	ragdomain "dungeons-and-dragons-ai/internal/rag/domain"
)

// NOTE: This file contains small, shared test stubs used by multiple integration tests.

// noopDMLLM is a deterministic DM-LLM stub.
// It never calls the network and returns a minimal stable response.
type noopDMLLM struct{}

func (noopDMLLM) Generate(ctx context.Context, prompt string) (string, error) {
	_ = ctx
	_ = prompt
	return "Ок.", nil
}

func (n noopDMLLM) GenerateWithMaxTokens(ctx context.Context, prompt string, maxTokens int) (string, error) {
	_ = maxTokens
	return n.Generate(ctx, prompt)
}

func (noopDMLLM) GenerateWithTools(ctx context.Context, prompt string, tools []dm_tools.Tool) (*domain.LLMResponseWithTools, error) {
	_ = ctx
	_ = prompt
	_ = tools
	return &domain.LLMResponseWithTools{Content: "Ок.", Finished: true}, nil
}

// analysisLLM is a deterministic LLM stub for AnalyzePlayerActionUseCase (returns a JSON analysis blob).
type analysisLLM struct {
	json string
}

func (l analysisLLM) Generate(ctx context.Context, prompt string) (string, error) {
	_ = ctx
	_ = prompt
	return l.json, nil
}

func (l analysisLLM) GenerateWithMaxTokens(ctx context.Context, prompt string, maxTokens int) (string, error) {
	_ = maxTokens
	return l.Generate(ctx, prompt)
}

func (analysisLLM) GenerateWithTools(ctx context.Context, prompt string, tools []dm_tools.Tool) (*domain.LLMResponseWithTools, error) {
	_ = ctx
	_ = prompt
	_ = tools
	return &domain.LLMResponseWithTools{Content: "{}", Finished: true}, nil
}

// --- No-op RAG dependencies to avoid network calls in tests ---

type noopEmbedder struct{}

func (noopEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	_ = ctx
	_ = text
	// Minimal non-empty embedding to exercise the pipeline.
	return []float32{0.0, 0.0, 0.0}, nil
}

type noopVectorStore struct{}

func (noopVectorStore) EnsureCollection(ctx context.Context) error {
	_ = ctx
	return nil
}

func (noopVectorStore) Upsert(ctx context.Context, doc ragdomain.Document, embedding []float32) error {
	_ = ctx
	_ = doc
	_ = embedding
	return nil
}

func (noopVectorStore) Search(ctx context.Context, sessionID uint, embedding []float32, limit int) ([]ragdomain.Document, error) {
	_ = ctx
	_ = sessionID
	_ = embedding
	_ = limit
	return nil, nil
}

// staticContextBuilder is a minimal ContextBuilder for tests.
type staticContextBuilder struct{}

func (staticContextBuilder) BuildContext(ctx context.Context, gs *session.GameSession, playerMessage string) (string, error) {
	_ = ctx
	if gs == nil {
		return "", fmt.Errorf("gs is nil")
	}
	return fmt.Sprintf("WORLD: %s\nPLAYER: %s", gs.World.Name, playerMessage), nil
}
