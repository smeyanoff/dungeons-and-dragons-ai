package application

import (
	"context"
	"errors"
	"testing"

	"dungeons-and-dragons-ai/internal/rag/domain"
)

// Mock Embedder
type mockRetrieveEmbedder struct {
	embedFunc func(ctx context.Context, text string) ([]float32, error)
}

func (m *mockRetrieveEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	if m.embedFunc != nil {
		return m.embedFunc(ctx, text)
	}
	return []float32{0.1, 0.2, 0.3}, nil
}

// Mock VectorStore
type mockRetrieveVectorStore struct {
	ensureCollectionFunc func(ctx context.Context) error
	upsertFunc           func(ctx context.Context, doc domain.Document, embedding []float32) error
	searchFunc           func(ctx context.Context, sessionID uint, locationID *uint, embedding []float32, limit int) ([]domain.Document, error)
}

func (m *mockRetrieveVectorStore) EnsureCollection(ctx context.Context) error {
	if m.ensureCollectionFunc != nil {
		return m.ensureCollectionFunc(ctx)
	}
	return nil
}

func (m *mockRetrieveVectorStore) Upsert(ctx context.Context, doc domain.Document, embedding []float32) error {
	if m.upsertFunc != nil {
		return m.upsertFunc(ctx, doc, embedding)
	}
	return nil
}

func (m *mockRetrieveVectorStore) Search(ctx context.Context, sessionID uint, locationID *uint, embedding []float32, limit int) ([]domain.Document, error) {
	if m.searchFunc != nil {
		return m.searchFunc(ctx, sessionID, locationID, embedding, limit)
	}
	return nil, nil
}

func TestRetrieveContext_Execute(t *testing.T) {
	tests := []struct {
		name          string
		sessionID     uint
		query         string
		limit         int
		setupMocks    func(*mockRetrieveEmbedder, *mockRetrieveVectorStore)
		expectedError bool
		validate      func(*testing.T, []domain.Document)
	}{
		{
			name:      "successful retrieval",
			sessionID: 1,
			query:     "What happened in the game?",
			limit:     5,
			setupMocks: func(embedder *mockRetrieveEmbedder, store *mockRetrieveVectorStore) {
				embedder.embedFunc = func(ctx context.Context, text string) ([]float32, error) {
					if text != "What happened in the game?" {
						t.Errorf("unexpected query: %s", text)
					}
					return []float32{0.1, 0.2, 0.3}, nil
				}
				store.searchFunc = func(ctx context.Context, sessionID uint, locationID *uint, embedding []float32, limit int) ([]domain.Document, error) {
					if sessionID != 1 {
						t.Errorf("unexpected sessionID: %d", sessionID)
					}
					if limit != 5 {
						t.Errorf("unexpected limit: %d", limit)
					}
					return []domain.Document{
						{
							ID:        "doc1",
							SessionID: 1,
							Text:      "Player went north",
							Source:    domain.SourceEvent,
						},
						{
							ID:        "doc2",
							SessionID: 1,
							Text:      "DM: You see a castle",
							Source:    domain.SourceEvent,
						},
					}, nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, docs []domain.Document) {
				if len(docs) != 2 {
					t.Errorf("expected 2 documents, got %d", len(docs))
				}
				if docs[0].ID != "doc1" {
					t.Errorf("unexpected first doc ID: %s", docs[0].ID)
				}
			},
		},
		{
			name:      "empty results",
			sessionID: 1,
			query:     "test query",
			limit:     5,
			setupMocks: func(embedder *mockRetrieveEmbedder, store *mockRetrieveVectorStore) {
				embedder.embedFunc = func(ctx context.Context, text string) ([]float32, error) {
					return []float32{0.1, 0.2, 0.3}, nil
				}
				store.searchFunc = func(ctx context.Context, sessionID uint, locationID *uint, embedding []float32, limit int) ([]domain.Document, error) {
					return []domain.Document{}, nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, docs []domain.Document) {
				if len(docs) != 0 {
					t.Errorf("expected 0 documents, got %d", len(docs))
				}
			},
		},
		{
			name:      "embedder error",
			sessionID: 1,
			query:     "test query",
			limit:     5,
			setupMocks: func(embedder *mockRetrieveEmbedder, store *mockRetrieveVectorStore) {
				embedder.embedFunc = func(ctx context.Context, text string) ([]float32, error) {
					return nil, errors.New("embedder error")
				}
			},
			expectedError: true,
		},
		{
			name:      "vector store error",
			sessionID: 1,
			query:     "test query",
			limit:     5,
			setupMocks: func(embedder *mockRetrieveEmbedder, store *mockRetrieveVectorStore) {
				embedder.embedFunc = func(ctx context.Context, text string) ([]float32, error) {
					return []float32{0.1, 0.2, 0.3}, nil
				}
				store.searchFunc = func(ctx context.Context, sessionID uint, locationID *uint, embedding []float32, limit int) ([]domain.Document, error) {
					return nil, errors.New("vector store error")
				}
			},
			expectedError: true,
		},
		{
			name:      "respects limit",
			sessionID: 1,
			query:     "test query",
			limit:     2,
			setupMocks: func(embedder *mockRetrieveEmbedder, store *mockRetrieveVectorStore) {
				embedder.embedFunc = func(ctx context.Context, text string) ([]float32, error) {
					return []float32{0.1, 0.2, 0.3}, nil
				}
				store.searchFunc = func(ctx context.Context, sessionID uint, locationID *uint, embedding []float32, limit int) ([]domain.Document, error) {
					if limit != 2 {
						t.Errorf("expected limit 2, got %d", limit)
					}
					return []domain.Document{
						{ID: "doc1", SessionID: 1, Text: "Doc 1"},
						{ID: "doc2", SessionID: 1, Text: "Doc 2"},
					}, nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, docs []domain.Document) {
				if len(docs) > 2 {
					t.Errorf("expected at most 2 documents, got %d", len(docs))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			embedder := &mockRetrieveEmbedder{}
			store := &mockRetrieveVectorStore{}

			if tt.setupMocks != nil {
				tt.setupMocks(embedder, store)
			}

			uc := NewRetrieveContext(embedder, store)

			result, err := uc.Execute(context.Background(), tt.sessionID, nil, tt.query, tt.limit)

			if tt.expectedError {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if tt.validate != nil {
					tt.validate(t, result)
				}
			}
		})
	}
}
