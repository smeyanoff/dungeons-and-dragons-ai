package embeddings

import (
	"dungeons-and-dragons-ai/pkg/gigachat"

	"dungeons-and-dragons-ai/internal/rag/domain"
)


type GigachatEmbedder struct {
	client *gigachat.Client
}
