package application

import (
	"context"
	"fmt"
	"log"
	"sync/atomic"
	"time"

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
	return uc.executeWithRetry(ctx, doc, 3)
}

// executeWithRetry выполняет индексацию с retry логикой
func (uc *IndexDocument) executeWithRetry(
	ctx context.Context,
	doc domain.Document,
	maxRetries int,
) error {
	const initialBackoff = 100 * time.Millisecond
	const maxBackoff = 2 * time.Second

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff с jitter
			backoff := initialBackoff * time.Duration(1<<uint(attempt-1))
			if backoff > maxBackoff {
				backoff = maxBackoff
			}

			log.Printf("[RAG Index] Retry attempt %d/%d for document (session_id: %d, source: %s) after %v",
				attempt, maxRetries+1, doc.SessionID, doc.Source, backoff)

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}

		err := uc.executeSingleAttempt(ctx, doc)
		if err == nil {
			// Успешная индексация
			log.Printf("[RAG Index] Successfully indexed document (session_id: %d, source: %s, text_length: %d)",
				doc.SessionID, doc.Source, len(doc.Text))
			return nil
		}

		lastErr = err
		atomic.AddInt64(&uc.indexErrorCount, 1)

		// Логируем ошибку с деталями
		log.Printf("[RAG Index] Indexing failed (attempt %d/%d): %v (session_id: %d, source: %s)",
			attempt+1, maxRetries+1, err, doc.SessionID, doc.Source)

		// Если это не последняя попытка, продолжаем
		if attempt < maxRetries {
			continue
		}
	}

	// Все попытки исчерпаны - логируем критическую ошибку
	log.Printf("[RAG Index] CRITICAL: Failed to index document after %d attempts: %v (session_id: %d, source: %s, text_preview: %s...)",
		maxRetries+1, lastErr, doc.SessionID, doc.Source, doc.Text[:min(100, len(doc.Text))])

	// Graceful fallback: возвращаем nil вместо ошибки, чтобы не прерывать основной поток
	// RAG индексация не критична для игрового процесса
	log.Printf("[RAG Index] Using graceful fallback - continuing without RAG indexing")
	return nil
}

// executeSingleAttempt выполняет одну попытку индексации
func (uc *IndexDocument) executeSingleAttempt(
	ctx context.Context,
	doc domain.Document,
) error {
	// Получаем эмбеддинги
	embedding, err := uc.embedder.Embed(ctx, doc.Text)
	if err != nil {
		atomic.AddInt64(&uc.embedErrorCount, 1)
		return fmt.Errorf("embedding failed: %w", err)
	}

	// Сохраняем в векторное хранилище
	err = uc.store.Upsert(ctx, doc, embedding)
	if err != nil {
		return fmt.Errorf("vector store upsert failed: %w", err)
	}

	atomic.AddInt64(&uc.indexCount, 1)
	return nil
}

// min возвращает минимальное из двух чисел
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
