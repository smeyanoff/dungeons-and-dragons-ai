package application

import (
	"context"

	"dungeons-and-dragons-ai/internal/rag/domain"
)

type RetrieveContext struct {
	embedder domain.Embedder
	store    domain.VectorStore
}

func NewRetrieveContext(e domain.Embedder, s domain.VectorStore) *RetrieveContext {
	return &RetrieveContext{embedder: e, store: s}
}

func (uc *RetrieveContext) Execute(
	ctx context.Context,
	sessionID uint,
	query string,
	limit int,
) ([]domain.Document, error) {

	embedding, err := uc.embedder.Embed(ctx, query)
	if err != nil {
		return nil, err
	}

	return uc.store.Search(ctx, sessionID, embedding, limit)
}
