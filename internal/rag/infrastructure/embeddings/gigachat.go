package embeddings

import (
	"context"
	"errors"

	"dungeons-and-dragons-ai/internal/rag/domain"
	"dungeons-and-dragons-ai/pkg/gigachat"
)

type GigachatEmbedder struct {
	client *gigachat.Client
}

func NewGigachatEmbedder(client *gigachat.Client) domain.Embedder {
	return &GigachatEmbedder{
		client: client,
	}
}

func (e *GigachatEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	if e.client == nil {
		return nil, errors.New("Gigachat client is not initialized")
	}
	return e.client.Embed(ctx, text)
}
