package domain

import "context"

type VectorStore interface {
	EnsureCollection(ctx context.Context) error
	Upsert(
		ctx context.Context,
		doc Document,
		embedding []float32) error
	// Search ищет документы по embedding в рамках сессии.
	// locationID, если не nil, дополнительно ограничивает поиск документами этой локации
	// (используется для отделения "памяти локации" от общей памяти кампании).
	Search(
		ctx context.Context,
		sessionID uint,
		locationID *uint,
		embedding []float32,
		limit int,
	) ([]Document, error)
}
