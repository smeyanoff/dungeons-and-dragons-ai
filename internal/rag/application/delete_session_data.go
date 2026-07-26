package application

import (
	"context"
	"fmt"
	"log"

	"dungeons-and-dragons-ai/internal/rag/domain"
)

// DeleteSessionData удаляет всю RAG-память сессии (кампании) из векторного
// хранилища. Вызывается при удалении GameSession, чтобы её документы не
// оставались в общей коллекции Qdrant бессрочно после удаления самой сессии
// из Postgres.
type DeleteSessionData struct {
	store domain.VectorStore
}

func NewDeleteSessionData(s domain.VectorStore) *DeleteSessionData {
	return &DeleteSessionData{store: s}
}

func (uc *DeleteSessionData) Execute(ctx context.Context, sessionID uint) error {
	if err := uc.store.Delete(ctx, sessionID); err != nil {
		log.Printf("[RAG Delete] Failed to delete session data (session_id: %d): %v", sessionID, err)
		return fmt.Errorf("vector store delete failed: %w", err)
	}
	return nil
}
