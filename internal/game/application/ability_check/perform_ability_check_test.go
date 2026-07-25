package ability_check

import (
	"context"
	"testing"
	"time"

	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/event"
	"dungeons-and-dragons-ai/internal/game/domain/player"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/rag/application"
	ragdomain "dungeons-and-dragons-ai/internal/rag/domain"
)

type mockSessionRepo struct {
	session      *session.GameSession
	getErr       error
	saveErr      error
	savedSession *session.GameSession
}

func (m *mockSessionRepo) GetByChatID(ctx context.Context, chatID int64) (*session.GameSession, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.session, nil
}

func (m *mockSessionRepo) Save(ctx context.Context, gs *session.GameSession) error {
	m.savedSession = gs
	return m.saveErr
}

type mockEventRepo struct {
	savedEvents []*event.StoryEvent
	saveErr     error
}

func (m *mockEventRepo) Save(ctx context.Context, e *event.StoryEvent) error {
	m.savedEvents = append(m.savedEvents, e)
	return m.saveErr
}

type mockEmbedder struct {
	lastText  string
	embedding []float32
	err       error
}

func (m *mockEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	m.lastText = text
	if m.embedding == nil {
		m.embedding = []float32{0.1, 0.2, 0.3}
	}
	return m.embedding, m.err
}

type mockVectorStore struct {
	upsertedDocs []ragdomain.Document
	embeddings   [][]float32
	err          error
}

func (m *mockVectorStore) EnsureCollection(ctx context.Context) error {
	return nil
}

func (m *mockVectorStore) Upsert(ctx context.Context, doc ragdomain.Document, embedding []float32) error {
	m.upsertedDocs = append(m.upsertedDocs, doc)
	m.embeddings = append(m.embeddings, embedding)
	return m.err
}

func (m *mockVectorStore) Search(ctx context.Context, sessionID uint, locationID *uint, embedding []float32, limit int) ([]ragdomain.Document, error) {
	return nil, nil
}

func (m *mockVectorStore) Delete(ctx context.Context, sessionID uint) error {
	return nil
}

func createSessionWithPendingCheck(t *testing.T, ability string, dc int) *session.GameSession {
	t.Helper()
	char, _ := character.NewCharacter("Test Hero", character.ClassFighter, character.RaceHuman, character.Stats{
		Strength: 10,
	})
	gs := &session.GameSession{
		ChatID: 1,
		State:  session.StateActive,
		Players: []player.Player{
			{Character: *char},
		},
	}
	gs.Model.ID = 1
	gs.SetPendingAbilityCheck("check-1", ability, dc)
	return gs
}

func TestPerformAbilityCheckUseCase_Execute_Errors(t *testing.T) {
	t.Run("session not found", func(t *testing.T) {
		uc := NewPerformAbilityCheckUseCase(&mockSessionRepo{}, &mockEventRepo{}, nil)
		_, err := uc.Execute(context.Background(), 1)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("no pending ability check", func(t *testing.T) {
		gs := &session.GameSession{ChatID: 1, State: session.StateActive}
		uc := NewPerformAbilityCheckUseCase(&mockSessionRepo{session: gs}, &mockEventRepo{}, nil)
		_, err := uc.Execute(context.Background(), 1)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("player not found", func(t *testing.T) {
		gs := &session.GameSession{ChatID: 1, State: session.StateActive}
		gs.SetPendingAbilityCheck("check-1", "strength", 10)
		uc := NewPerformAbilityCheckUseCase(&mockSessionRepo{session: gs}, &mockEventRepo{}, nil)
		_, err := uc.Execute(context.Background(), 1)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestPerformAbilityCheckUseCase_Execute_Success(t *testing.T) {
	gs := createSessionWithPendingCheck(t, "strength", 10)
	sessionRepo := &mockSessionRepo{session: gs}
	eventRepo := &mockEventRepo{}
	embedder := &mockEmbedder{}
	store := &mockVectorStore{}
	indexUC := application.NewIndexDocument(embedder, store)

	uc := NewPerformAbilityCheckUseCase(sessionRepo, eventRepo, indexUC)
	result, err := uc.Execute(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Ability != "strength" {
		t.Fatalf("expected ability=strength, got %s", result.Ability)
	}
	if result.BaseDC != 10 {
		t.Fatalf("expected base dc=10, got %d", result.BaseDC)
	}
	if result.DC < 8 || result.DC > 20 {
		t.Fatalf("expected adaptive dc in [8..20], got %d", result.DC)
	}
	if result.BaseRoll < 1 || result.BaseRoll > 20 {
		t.Fatalf("expected base roll in [1..20], got %d", result.BaseRoll)
	}
	if result.Total < result.BaseRoll-5 || result.Total > result.BaseRoll+5 {
		t.Fatalf("expected total close to base roll, got %d (base: %d)", result.Total, result.BaseRoll)
	}
	if result.Message == "" {
		t.Fatal("expected non-empty message")
	}

	if gs.HasPendingAbilityCheck() {
		t.Fatal("expected pending ability check to be cleared")
	}
	if sessionRepo.savedSession == nil {
		t.Fatal("expected session to be saved after clearing pending check")
	}

	if len(eventRepo.savedEvents) != 1 {
		t.Fatalf("expected 1 event saved, got %d", len(eventRepo.savedEvents))
	}
	if eventRepo.savedEvents[0].Content != result.Message {
		t.Fatalf("expected event content to match result message")
	}

	if len(store.upsertedDocs) != 1 {
		t.Fatalf("expected 1 document indexed, got %d", len(store.upsertedDocs))
	}
	if store.upsertedDocs[0].Text != result.Message {
		t.Fatalf("expected indexed document text to match result message")
	}

	if embedder.lastText != result.Message {
		t.Fatalf("expected embedder to be called with result message")
	}
}

func TestResolveAbility_Default(t *testing.T) {
	name, value := resolveAbility(character.Stats{}, "unknown")
	if name == "" {
		t.Fatal("expected non-empty name for unknown ability")
	}
	if value != 10 {
		t.Fatalf("expected default value 10, got %d", value)
	}
}

func TestPerformAbilityCheckUseCase_Execute_IgnoresEventSaveErrors(t *testing.T) {
	gs := createSessionWithPendingCheck(t, "strength", 1)
	sessionRepo := &mockSessionRepo{session: gs}
	eventRepo := &mockEventRepo{saveErr: context.Canceled}

	uc := NewPerformAbilityCheckUseCase(sessionRepo, eventRepo, nil)
	_, err := uc.Execute(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error despite event save error: %v", err)
	}
	if sessionRepo.savedSession == nil {
		t.Fatal("expected session to be saved even if event save fails")
	}
}

func TestPerformAbilityCheckUseCase_Execute_UsesAbilityStats(t *testing.T) {
	char, _ := character.NewCharacter("Test Hero", character.ClassFighter, character.RaceHuman, character.Stats{
		Dexterity: 18,
	})
	gs := &session.GameSession{
		ChatID: 1,
		State:  session.StateActive,
		Players: []player.Player{
			{Character: *char},
		},
	}
	gs.Model.ID = 1
	gs.SetPendingAbilityCheck("check-1", "dexterity", 1)

	uc := NewPerformAbilityCheckUseCase(&mockSessionRepo{session: gs}, &mockEventRepo{}, nil)
	result, err := uc.Execute(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Modifier != 4 {
		t.Fatalf("expected dexterity modifier=4, got %d", result.Modifier)
	}
}

func TestPerformAbilityCheckUseCase_Execute_SetsOutcomeText(t *testing.T) {
	gs := createSessionWithPendingCheck(t, "strength", 20)
	gs.PendingAbilityCheckRequestedAt = func() *time.Time {
		now := time.Now().Add(-5 * time.Minute)
		return &now
	}()

	uc := NewPerformAbilityCheckUseCase(&mockSessionRepo{session: gs}, &mockEventRepo{}, nil)
	result, err := uc.Execute(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Message == "" {
		t.Fatal("expected message to be set")
	}
}
