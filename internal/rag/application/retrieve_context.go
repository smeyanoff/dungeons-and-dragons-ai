package application

import (
	"context"
	"sync/atomic"

	"dungeons-and-dragons-ai/internal/rag/domain"
)

type RetrieveContext struct {
	embedder          domain.Embedder
	store             domain.VectorStore
	retrieveCount     int64 // Counter for successful retrievals
	retrieveErrorCount int64 // Counter for retrieval errors
	embedErrorCount   int64 // Counter for embedding errors
}

func NewRetrieveContext(e domain.Embedder, s domain.VectorStore) *RetrieveContext {
	return &RetrieveContext{
		embedder: e,
		store:    s,
	}
}

// GetMetrics возвращает метрики поиска
func (uc *RetrieveContext) GetMetrics() map[string]int64 {
	return map[string]int64{
		"retrieve_success":   atomic.LoadInt64(&uc.retrieveCount),
		"retrieve_errors":    atomic.LoadInt64(&uc.retrieveErrorCount),
		"embedding_errors":   atomic.LoadInt64(&uc.embedErrorCount),
	}
}

func (uc *RetrieveContext) Execute(
	ctx context.Context,
	sessionID uint,
	query string,
	limit int,
) ([]domain.Document, error) {

	embedding, err := uc.embedder.Embed(ctx, query)
	if err != nil {
		atomic.AddInt64(&uc.embedErrorCount, 1)
		return nil, err
	}

	docs, err := uc.store.Search(ctx, sessionID, embedding, limit)
	if err != nil {
		atomic.AddInt64(&uc.retrieveErrorCount, 1)
		return nil, err
	}

	atomic.AddInt64(&uc.retrieveCount, 1)
	return docs, nil
}
