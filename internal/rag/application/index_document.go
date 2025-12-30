package application

import (
    "context"

    "dungeons-and-dragons-ai/internal/rag/domain"
)


type IndexDocument struct {
    embedder domain.Embedder
    store    domain.VectorStore
}

func NewIndexDocument(e domain.Embedder, s domain.VectorStore) *IndexDocument {
    return &IndexDocument{embedder: e, store: s}
}

func (uc *IndexDocument) Execute(
    ctx context.Context,
    doc domain.Document,
) error {

    embedding, err := uc.embedder.Embed(ctx, doc.Text)
    if err != nil {
        return err
    }

    return uc.store.Upsert(ctx, doc, embedding)
}
