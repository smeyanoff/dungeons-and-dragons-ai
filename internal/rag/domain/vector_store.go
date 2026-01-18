package domain

import "context"

type VectorStore interface {
	EnsureCollection(ctx context.Context) error
	Upsert(
		ctx context.Context,
		doc Document,
		embedding []float32) error
	Search(
		ctx context.Context,
		sessionID uint,
		embedding []float32,
		limit int,
	) ([]Document, error)
}
