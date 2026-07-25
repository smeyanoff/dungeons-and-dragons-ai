package vectorstore

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"dungeons-and-dragons-ai/internal/rag/domain"
	"github.com/qdrant/go-client/qdrant"
)

const (
	collectionName = "dnd"
	// defaultEmbeddingDimension используется, если размерность не передана явно
	// (обратная совместимость с NewQdrantStore без второго аргумента).
	defaultEmbeddingDimension = 1024
)

type QdrantStore struct {
	Client             *qdrant.Client
	embeddingDimension uint64
}

// NewQdrantStore создает хранилище векторов. dimension — размерность векторов
// текущей модели эмбеддингов (см. gigachat.EmbeddingDimension); 0 означает
// "использовать значение по умолчанию" (1024, старая GigaChat-Embeddings).
func NewQdrantStore(client *qdrant.Client, dimension int) domain.VectorStore {
	dim := uint64(defaultEmbeddingDimension)
	if dimension > 0 {
		dim = uint64(dimension)
	}
	return &QdrantStore{Client: client, embeddingDimension: dim}
}

// EnsureCollection реализует domain.VectorStore
func (s *QdrantStore) EnsureCollection(ctx context.Context) error {
	return s.ensureCollection(ctx)
}

func (s *QdrantStore) ensureCollection(ctx context.Context) error {
	// Проверяем существование коллекции
	collections, err := s.Client.ListCollections(ctx)
	if err != nil {
		return fmt.Errorf("failed to get collections: %w", err)
	}

	// Проверяем, существует ли коллекция
	collectionExists := false
	for _, name := range collections {
		if name == collectionName {
			collectionExists = true
			break
		}
	}

	if collectionExists {
		// Коллекция существует — проверяем, что её размерность совпадает с текущей
		// моделью эмбеддингов. Несовпадение (например, после смены
		// GIGACHAT_EMBEDDINGS_MODEL на модель с другой размерностью вектора)
		// раньше не обнаруживалось здесь и всплывало только при первом поиске
		// как runtime-ошибка Qdrant ("Vector dimension error"), из-за чего RAG-контекст
		// молча переставал работать. Пересоздаем коллекцию с правильной размерностью:
		// это безопасно, т.к. RAG-индекс — производный кэш для семантического поиска,
		// а не источник истины (сама история хода игры хранится в Postgres).
		collectionInfo, err := s.Client.GetCollectionInfo(ctx, collectionName)
		if err != nil {
			return fmt.Errorf("failed to get collection info: %w", err)
		}

		existingDim := uint64(0)
		if params := collectionInfo.GetConfig().GetParams(); params != nil {
			if vp := params.GetVectorsConfig().GetParams(); vp != nil {
				existingDim = vp.GetSize()
			}
		}

		if existingDim == 0 || existingDim == s.embeddingDimension {
			return nil
		}

		if err := s.Client.DeleteCollection(ctx, collectionName); err != nil {
			return fmt.Errorf("failed to delete collection with stale vector dimension (existing: %d, expected: %d): %w", existingDim, s.embeddingDimension, err)
		}
		collectionExists = false
	}

	if collectionExists {
		return nil
	}

	// Коллекция не существует (или пересоздается из-за несовпадения размерности) — создаем её
	err = s.Client.CreateCollection(ctx, &qdrant.CreateCollection{
		CollectionName: collectionName,
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     s.embeddingDimension,
			Distance: qdrant.Distance_Cosine,
		}),
	})
	if err != nil {
		return fmt.Errorf("failed to create collection: %w", err)
	}

	return nil
}

func (s *QdrantStore) Upsert(
	ctx context.Context,
	doc domain.Document,
	embedding []float32,
) error {

	payload := map[string]*qdrant.Value{
		"session_id": {Kind: &qdrant.Value_IntegerValue{IntegerValue: safeUintToInt64(doc.SessionID)}},
		"source":     {Kind: &qdrant.Value_StringValue{StringValue: string(doc.Source)}},
		"text":       {Kind: &qdrant.Value_StringValue{StringValue: doc.Text}},
		"timestamp":  {Kind: &qdrant.Value_IntegerValue{IntegerValue: doc.Timestamp.Unix()}},
	}
	if doc.LocationID != nil {
		payload["location_id"] = &qdrant.Value{Kind: &qdrant.Value_IntegerValue{IntegerValue: safeUintToInt64(*doc.LocationID)}}
	}

	_, err := s.Client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: collectionName,
		Points: []*qdrant.PointStruct{
			{
				Id: &qdrant.PointId{
					PointIdOptions: &qdrant.PointId_Uuid{
						Uuid: doc.ID,
					},
				},
				Vectors: &qdrant.Vectors{
					VectorsOptions: &qdrant.Vectors_Vector{
						Vector: &qdrant.Vector{
							Data: embedding,
						},
					},
				},
				Payload: payload,
			},
		},
	})

	return err
}

func (s *QdrantStore) Search(
	ctx context.Context,
	sessionID uint,
	locationID *uint,
	embedding []float32,
	limit int,
) ([]domain.Document, error) {

	// Валидация limit для предотвращения overflow
	if limit < 0 {
		return nil, fmt.Errorf("limit must be non-negative, got %d", limit)
	}
	if limit > math.MaxInt {
		return nil, fmt.Errorf("limit exceeds maximum allowed value %d, got %d", math.MaxInt, limit)
	}
	limitUint64 := uint64(limit)

	conditions := []*qdrant.Condition{
		{
			ConditionOneOf: &qdrant.Condition_Field{
				Field: &qdrant.FieldCondition{
					Key: "session_id",
					Match: &qdrant.Match{
						MatchValue: &qdrant.Match_Integer{
							Integer: safeUintToInt64(sessionID),
						},
					},
				},
			},
		},
	}
	if locationID != nil {
		conditions = append(conditions, &qdrant.Condition{
			ConditionOneOf: &qdrant.Condition_Field{
				Field: &qdrant.FieldCondition{
					Key: "location_id",
					Match: &qdrant.Match{
						MatchValue: &qdrant.Match_Integer{
							Integer: safeUintToInt64(*locationID),
						},
					},
				},
			},
		})
	}

	resp, err := s.Client.GetPointsClient().Search(ctx, &qdrant.SearchPoints{
		CollectionName: collectionName,
		Vector:         embedding,
		Limit:          limitUint64,
		Filter: &qdrant.Filter{
			Must: conditions,
		},
		// Без этого Qdrant по умолчанию не возвращает payload вместе с точками —
		// найденные документы приходят с пустым текстом (found_docs>0, docs_added=0).
		WithPayload: qdrant.NewWithPayload(true),
	})
	if err != nil {
		return nil, err
	}

	docs := make([]domain.Document, 0, len(resp.Result))
	for _, p := range resp.Result {
		payload := p.Payload
		doc := domain.Document{
			ID:        p.Id.GetUuid(),
			SessionID: sessionID,
			Source:    domain.SourceType(payload["source"].GetStringValue()),
			Text:      payload["text"].GetStringValue(),
		}
		if locValue, ok := payload["location_id"]; ok {
			if locID := locValue.GetIntegerValue(); locID > 0 {
				loc := uint(locID)
				doc.LocationID = &loc
			}
		}
		// Восстанавливаем timestamp, если он есть
		if tsValue, ok := payload["timestamp"]; ok {
			if ts := tsValue.GetIntegerValue(); ts > 0 {
				doc.Timestamp = time.Unix(ts, 0)
			}
		}
		// Если timestamp не найден, устанавливаем нулевое время (для обратной совместимости)
		if doc.Timestamp.IsZero() {
			doc.Timestamp = time.Now()
		}
		docs = append(docs, doc)
	}

	// Сортируем документы по времени (более новые первыми для приоритета недавних событий)
	// Это улучшает понимание временной последовательности в RAG
	sort.Slice(docs, func(i, j int) bool {
		return docs[i].Timestamp.After(docs[j].Timestamp)
	})

	return docs, nil
}

// safeUintToInt64 безопасно преобразует uint в int64 с проверкой на overflow
func safeUintToInt64(val uint) int64 {
	if val > math.MaxInt64 {
		// Если значение превышает MaxInt64, возвращаем MaxInt64
		// В практике это не должно происходить для sessionID и других ID
		return math.MaxInt64
	}
	return int64(val)
}
