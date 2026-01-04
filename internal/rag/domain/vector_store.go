package domain

type VectorStore interface {
	Upsert(ctx context.Context, doc domain.Document, embedding []float32) error
}
