package embeddings

import (
	"dungeons-and-dragons-ai/pkg/gigachat"

	"dungeons-and-dragons-ai/internal/rag/domain"
)

type GigachatEmbedder struct {
	client *gigachat.Client
}

func NewGigachatEmbedder(client *gigachat.Client) *GigachatEmbedder {
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
