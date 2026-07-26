package integration

import (
	"context"
	"testing"
	"time"

	ragapp "dungeons-and-dragons-ai/internal/rag/application"
	ragvectorstore "dungeons-and-dragons-ai/internal/rag/infrastructure/vectorstore"

	"github.com/google/uuid"
	"github.com/qdrant/go-client/qdrant"

	ragdomain "dungeons-and-dragons-ai/internal/rag/domain"
)

// fakeIsolationEmbedder — детерминированный эмбеддер без сети и без GigaChat
// credentials. Тест проверяет только фильтрацию по session_id в Qdrant, а не
// семантическое ранжирование, поэтому содержимое вектора неважно — важна лишь
// его размерность (должна совпадать с размерностью коллекции).
type fakeIsolationEmbedder struct{ dim int }

func (f fakeIsolationEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	v := make([]float32, f.dim)
	v[0] = 1
	return v, nil
}

// connectTestQdrant подключается к тому же Qdrant, что и остальные интеграционные
// тесты (см. isContainersRunning/getEnv/parsePort в test_support_integration_test.go),
// но без Postgres/GigaChat — эта проверка не требует ни того, ни другого.
func connectTestQdrant(t *testing.T) *qdrant.Client {
	t.Helper()
	if !isContainersRunning(t) {
		t.Skip("Контейнеры не запущены или недоступны. Запустите: make docker-up")
	}

	qdrantHost := getEnv("QDRANT_HOST", "localhost")
	if qdrantHost == "qdrant" {
		qdrantHost = "localhost"
	}
	qdrantGrpcPort := getEnv("QDRANT_GRPC_PORT", "6335")
	client, err := qdrant.NewClient(&qdrant.Config{
		Host:                   qdrantHost,
		Port:                   parsePort(qdrantGrpcPort),
		SkipCompatibilityCheck: true,
	})
	if err != nil {
		t.Fatalf("Не удалось подключиться к Qdrant: %v", err)
	}
	return client
}

// TestRAGSessionIsolation проверяет ключевое инвариант RAG-хранилища: документы
// одной игровой сессии (кампании) не должны находиться при поиске в контексте
// другой сессии, даже если обе хранятся в одной общей коллекции Qdrant "dnd".
func TestRAGSessionIsolation(t *testing.T) {
	client := connectTestQdrant(t)
	ctx := context.Background()

	const dim = 1024
	embedder := fakeIsolationEmbedder{dim: dim}
	store := ragvectorstore.NewQdrantStore(client, dim)
	if err := store.EnsureCollection(ctx); err != nil {
		t.Fatalf("EnsureCollection: %v", err)
	}

	indexUC := ragapp.NewIndexDocument(embedder, store)
	retrieveUC := ragapp.NewRetrieveContext(embedder, store)

	// Уникальные session_id на каждый запуск теста, чтобы не столкнуться с данными
	// других тестов/прогонов в общей коллекции.
	base := uint(time.Now().UnixNano())
	sessionA := base
	sessionB := base + 1

	textA := "Секретное событие кампании A: партия нашла артефакт в пещере драконов"
	textB := "Секретное событие кампании B: партия заключила союз с эльфами леса"

	docA := ragdomain.Document{
		ID:        uuid.New().String(),
		Source:    ragdomain.SourceEvent,
		SessionID: sessionA,
		Text:      textA,
		Timestamp: time.Now(),
	}
	docB := ragdomain.Document{
		ID:        uuid.New().String(),
		Source:    ragdomain.SourceEvent,
		SessionID: sessionB,
		Text:      textB,
		Timestamp: time.Now(),
	}

	if err := indexUC.Execute(ctx, docA); err != nil {
		t.Fatalf("не удалось проиндексировать документ сессии A: %v", err)
	}
	if err := indexUC.Execute(ctx, docB); err != nil {
		t.Fatalf("не удалось проиндексировать документ сессии B: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = store.Delete(cleanupCtx, sessionA)
		_ = store.Delete(cleanupCtx, sessionB)
	})

	// Qdrant — eventually consistent при wait=false по умолчанию в Upsert; даём индексу
	// время устаканиться перед поиском (аналогично другим RAG-интеграционным тестам).
	deadline := time.Now().Add(10 * time.Second)
	for {
		docsA, err := retrieveUC.Execute(ctx, sessionA, nil, textA, 10)
		if err != nil {
			t.Fatalf("Execute(sessionA): %v", err)
		}
		if len(docsA) > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Run("сессия A видит только свой документ", func(t *testing.T) {
		docs, err := retrieveUC.Execute(ctx, sessionA, nil, textA, 10)
		if err != nil {
			t.Fatalf("Execute(sessionA): %v", err)
		}
		if len(docs) != 1 {
			t.Fatalf("ожидался 1 документ в сессии A, получено %d: %+v", len(docs), docs)
		}
		if docs[0].Text != textA {
			t.Fatalf("сессия A получила чужой документ: %q", docs[0].Text)
		}
		if docs[0].SessionID != sessionA {
			t.Fatalf("документ сессии A имеет session_id=%d, ожидалось %d", docs[0].SessionID, sessionA)
		}
	})

	t.Run("сессия B видит только свой документ", func(t *testing.T) {
		docs, err := retrieveUC.Execute(ctx, sessionB, nil, textB, 10)
		if err != nil {
			t.Fatalf("Execute(sessionB): %v", err)
		}
		if len(docs) != 1 {
			t.Fatalf("ожидался 1 документ в сессии B, получено %d: %+v", len(docs), docs)
		}
		if docs[0].Text != textB {
			t.Fatalf("сессия B получила чужой документ: %q", docs[0].Text)
		}
	})

	t.Run("документ сессии B не находится в сессии A по чужому тексту", func(t *testing.T) {
		docs, err := retrieveUC.Execute(ctx, sessionA, nil, textB, 10)
		if err != nil {
			t.Fatalf("Execute(sessionA, query=textB): %v", err)
		}
		for _, d := range docs {
			if d.SessionID != sessionA {
				t.Fatalf("нашли документ чужой сессии %d при поиске в сессии %d", d.SessionID, sessionA)
			}
			if d.Text == textB {
				t.Fatalf("документ сессии B утёк в результаты поиска сессии A")
			}
		}
	})

	t.Run("Delete удаляет данные только своей сессии", func(t *testing.T) {
		if err := store.Delete(ctx, sessionA); err != nil {
			t.Fatalf("Delete(sessionA): %v", err)
		}

		deadline := time.Now().Add(10 * time.Second)
		var docsA []ragdomain.Document
		for {
			var err error
			docsA, err = retrieveUC.Execute(ctx, sessionA, nil, textA, 10)
			if err != nil {
				t.Fatalf("Execute(sessionA) после Delete: %v", err)
			}
			if len(docsA) == 0 || time.Now().After(deadline) {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if len(docsA) != 0 {
			t.Fatalf("после Delete(sessionA) документы сессии A всё ещё находятся: %+v", docsA)
		}

		docsB, err := retrieveUC.Execute(ctx, sessionB, nil, textB, 10)
		if err != nil {
			t.Fatalf("Execute(sessionB) после Delete(sessionA): %v", err)
		}
		if len(docsB) != 1 {
			t.Fatalf("Delete(sessionA) задел чужую сессию B: получено %d документов, ожидался 1", len(docsB))
		}
	})
}
