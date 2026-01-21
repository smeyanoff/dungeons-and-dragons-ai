package application

import (
	"context"
	"sync/atomic"

	"dungeons-and-dragons-ai/internal/rag/domain"
)

type IndexDocument struct {
	embedder         domain.Embedder
	store            domain.VectorStore
	indexCount       int64 // Counter for successful indexing
	indexErrorCount  int64 // Counter for indexing errors
	embedErrorCount  int64 // Counter for embedding errors
}

func NewIndexDocument(e domain.Embedder, s domain.VectorStore) *IndexDocument {
	return &IndexDocument{
		embedder: e,
		store:    s,
	}
}

// GetMetrics возвращает метрики индексации
func (uc *IndexDocument) GetMetrics() map[string]int64 {
	return map[string]int64{
		"index_success":     atomic.LoadInt64(&uc.indexCount),
		"index_errors":      atomic.LoadInt64(&uc.indexErrorCount),
		"embedding_errors":  atomic.LoadInt64(&uc.embedErrorCount),
	}
}

func (uc *IndexDocument) Execute(
	ctx context.Context,
	doc domain.Document,
) error {

	embedding, err := uc.embedder.Embed(ctx, doc.Text)
	if err != nil {
		atomic.AddInt64(&uc.embedErrorCount, 1)
		return err
	}

	err = uc.store.Upsert(ctx, doc, embedding)
	if err != nil {
		atomic.AddInt64(&uc.indexErrorCount, 1)
		return err
	}

	atomic.AddInt64(&uc.indexCount, 1)
	return nil
}
