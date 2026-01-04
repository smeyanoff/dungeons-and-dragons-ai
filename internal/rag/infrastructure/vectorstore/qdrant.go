package vectorstore

import (
	"context"

	"dungeons-and-dragons-ai/internal/rag/domain"
	"github.com/qdrant/go-client/qdrant"
)

type QdrantStore struct {
	client *qdrant.Client
}

func NewQdrantStore(client *qdrant.Client) *QdrantStore {
	return &QdrantStore{client: client}
}

func (s *QdrantStore) Upsert(
	ctx context.Context,
	doc domain.Document,
	embedding []float32,
) error {

	_, err := s.client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: "dnd",
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
				Payload: map[string]*qdrant.Value{
					"session_id": {Kind: &qdrant.Value_IntegerValue{IntegerValue: int64(doc.SessionID)}},
					"source":     {Kind: &qdrant.Value_StringValue{StringValue: string(doc.Source)}},
					"text":       {Kind: &qdrant.Value_StringValue{StringValue: doc.Text}},
				},
			},
		},
	})

	return err
}

func (s *QdrantStore) Search(
	ctx context.Context,
	sessionID uint,
	embedding []float32,
	limit int,
) ([]domain.Document, error) {

	resp, err := s.client.Search(ctx, &qdrant.SearchPoints{
		CollectionName: "dnd",
		Vector:         embedding,
		Limit:          uint64(limit),
		Filter: &qdrant.Filter{
			Must: []*qdrant.Condition{
				{
					ConditionOneOf: &qdrant.Condition_Field{
						Field: &qdrant.FieldCondition{
							Key: "session_id",
							Match: &qdrant.Match{
								MatchValue: &qdrant.Match_Integer{
									Integer: int64(sessionID),
								},
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	docs := make([]domain.Document, 0, len(resp))
	for _, p := range resp {
		payload := p.Payload
		docs = append(docs, domain.Document{
			ID:     p.Id.GetUuid(),
			Source: domain.SourceType(payload["source"].GetStringValue()),
			Text:   payload["text"].GetStringValue(),
		})
	}

	return docs, nil
}
