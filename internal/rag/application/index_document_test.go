package application

import (
	"context"
	"errors"
	"testing"

	"dungeons-and-dragons-ai/internal/rag/domain"
)

// Mock Embedder
type mockIndexEmbedder struct {
	embedFunc func(ctx context.Context, text string) ([]float32, error)
}

func (m *mockIndexEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	if m.embedFunc != nil {
		return m.embedFunc(ctx, text)
	}
	return []float32{0.1, 0.2, 0.3}, nil
}

// Mock VectorStore
type mockIndexVectorStore struct {
	ensureCollectionFunc func(ctx context.Context) error
	upsertFunc           func(ctx context.Context, doc domain.Document, embedding []float32) error
	searchFunc           func(ctx context.Context, sessionID uint, locationID *uint, embedding []float32, limit int) ([]domain.Document, error)
}

func (m *mockIndexVectorStore) EnsureCollection(ctx context.Context) error {
	if m.ensureCollectionFunc != nil {
		return m.ensureCollectionFunc(ctx)
	}
	return nil
}

func (m *mockIndexVectorStore) Upsert(ctx context.Context, doc domain.Document, embedding []float32) error {
	if m.upsertFunc != nil {
		return m.upsertFunc(ctx, doc, embedding)
	}
	return nil
}

func (m *mockIndexVectorStore) Search(ctx context.Context, sessionID uint, locationID *uint, embedding []float32, limit int) ([]domain.Document, error) {
	if m.searchFunc != nil {
		return m.searchFunc(ctx, sessionID, locationID, embedding, limit)
	}
	return nil, nil
}

func TestIndexDocument_Execute(t *testing.T) {
	tests := []struct {
		name          string
		doc           domain.Document
		setupMocks    func(*mockIndexEmbedder, *mockIndexVectorStore)
		expectedError bool
	}{
		{
			name: "successful indexing",
			doc: domain.Document{
				ID:        "doc1",
				SessionID: 1,
				Text:      "Test document text",
				Source:    domain.SourceEvent,
			},
			setupMocks: func(embedder *mockIndexEmbedder, store *mockIndexVectorStore) {
				embedder.embedFunc = func(ctx context.Context, text string) ([]float32, error) {
					if text != "Test document text" {
						t.Errorf("unexpected text: %s", text)
					}
					return []float32{0.1, 0.2, 0.3}, nil
				}
				store.upsertFunc = func(ctx context.Context, doc domain.Document, embedding []float32) error {
					if doc.ID != "doc1" {
						t.Errorf("unexpected doc ID: %s", doc.ID)
					}
					if len(embedding) == 0 {
						t.Error("expected non-empty embedding")
					}
					return nil
				}
			},
			expectedError: false,
		},
		{
			name: "embedder error",
			doc: domain.Document{
				ID:        "doc1",
				SessionID: 1,
				Text:      "Test document text",
			},
			setupMocks: func(embedder *mockIndexEmbedder, store *mockIndexVectorStore) {
				embedder.embedFunc = func(ctx context.Context, text string) ([]float32, error) {
					return nil, errors.New("embedder error")
				}
			},
			expectedError: false,
		},
		{
			name: "vector store error",
			doc: domain.Document{
				ID:        "doc1",
				SessionID: 1,
				Text:      "Test document text",
			},
			setupMocks: func(embedder *mockIndexEmbedder, store *mockIndexVectorStore) {
				embedder.embedFunc = func(ctx context.Context, text string) ([]float32, error) {
					return []float32{0.1, 0.2, 0.3}, nil
				}
				store.upsertFunc = func(ctx context.Context, doc domain.Document, embedding []float32) error {
					return errors.New("vector store error")
				}
			},
			expectedError: false,
		},
		{
			name: "empty text",
			doc: domain.Document{
				ID:        "doc1",
				SessionID: 1,
				Text:      "",
			},
			setupMocks: func(embedder *mockIndexEmbedder, store *mockIndexVectorStore) {
				embedder.embedFunc = func(ctx context.Context, text string) ([]float32, error) {
					return []float32{0.1, 0.2, 0.3}, nil
				}
			},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			embedder := &mockIndexEmbedder{}
			store := &mockIndexVectorStore{}

			if tt.setupMocks != nil {
				tt.setupMocks(embedder, store)
			}

			uc := NewIndexDocument(embedder, store)

			err := uc.Execute(context.Background(), tt.doc)

			if tt.expectedError {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}
