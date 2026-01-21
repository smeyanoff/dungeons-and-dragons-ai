package worldmap

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"dungeons-and-dragons-ai/internal/game/domain/event"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/game/domain/world"
	ragdomain "dungeons-and-dragons-ai/internal/rag/domain"

	"github.com/google/uuid"
)

type MoveToLocationUseCase struct {
	sessionRepo    session.Repository
	worldEventRepo WorldEventRepository
	eventRepo      StoryEventRepository
	indexDocUC     RAGIndexer
}

type WorldEventRepository interface {
	GetByLocationID(ctx context.Context, locationID uint) ([]world.WorldEvent, error)
	Save(ctx context.Context, e *world.WorldEvent) error
}

type StoryEventRepository interface {
	Save(ctx context.Context, e *event.StoryEvent) error
}

type RAGIndexer interface {
	Execute(ctx context.Context, doc ragdomain.Document) error
}

func NewMoveToLocationUseCase(
	sessionRepo session.Repository,
	worldEventRepo WorldEventRepository,
	eventRepo StoryEventRepository,
	indexDocUC RAGIndexer,
) *MoveToLocationUseCase {
	return &MoveToLocationUseCase{
		sessionRepo:    sessionRepo,
		worldEventRepo: worldEventRepo,
		eventRepo:      eventRepo,
		indexDocUC:     indexDocUC,
	}
}

type MoveToLocationRequest struct {
	ChatID int64

	// Один из вариантов должен быть задан:
	ToLocationID *uint
	Direction    string
}

type MoveToLocationResponse struct {
	From *world.Location
	To   *world.Location

	Message string
}

func (uc *MoveToLocationUseCase) Execute(ctx context.Context, req MoveToLocationRequest) (*MoveToLocationResponse, error) {
	gs, err := uc.sessionRepo.GetByChatID(ctx, req.ChatID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}
	if gs == nil {
		return nil, fmt.Errorf("game session not found")
	}
	if !gs.IsActive() {
		return nil, fmt.Errorf("game session is not active")
	}
	if len(gs.World.Locations) == 0 {
		return nil, fmt.Errorf("world has no locations")
	}

	locationMap := make(map[uint]*world.Location, len(gs.World.Locations))
	for i := range gs.World.Locations {
		loc := &gs.World.Locations[i]
		locationMap[loc.ID] = loc
	}

	// Определяем текущую локацию (если не задана — инициализируем первой)
	var current *world.Location
	if gs.CurrentLocationID != nil {
		current = locationMap[*gs.CurrentLocationID]
	}
	if current == nil {
		firstID := gs.World.Locations[0].ID
		gs.CurrentLocationID = &firstID
		current = locationMap[firstID]
		_ = uc.sessionRepo.Save(ctx, gs)
	}

	var targetID uint
	if req.ToLocationID != nil && *req.ToLocationID != 0 {
		targetID = *req.ToLocationID
	} else if strings.TrimSpace(req.Direction) != "" {
		dir := normalizeDirection(req.Direction)
		for _, conn := range current.Connections {
			if normalizeDirection(conn.Direction) == dir {
				targetID = conn.ToLocationID
				break
			}
		}
	} else {
		return nil, fmt.Errorf("either ToLocationID or Direction must be provided")
	}

	to := locationMap[targetID]
	if to == nil {
		return nil, fmt.Errorf("target location not found")
	}

	// Проверяем достижимость (есть ли связь из текущей локации)
	reachable := false
	for _, conn := range current.Connections {
		if conn.ToLocationID == targetID {
			reachable = true
			break
		}
	}
	if !reachable {
		return nil, fmt.Errorf("target location is not reachable from current location")
	}

	// Фиксируем активные события локации как "игнор" при уходе
	if err := uc.resolveLocationEventsOnLeave(ctx, gs, current); err != nil {
		return nil, err
	}

	// Обновляем текущую локацию
	gs.CurrentLocationID = &targetID
	if err := uc.sessionRepo.Save(ctx, gs); err != nil {
		return nil, fmt.Errorf("failed to save session: %w", err)
	}

	msg := fmt.Sprintf("🧭 Вы переместились: %s → %s\n\n%s", current.Name, to.Name, to.Description)
	return &MoveToLocationResponse{
		From:    current,
		To:      to,
		Message: msg,
	}, nil
}

func (uc *MoveToLocationUseCase) resolveLocationEventsOnLeave(
	ctx context.Context,
	gs *session.GameSession,
	current *world.Location,
) error {
	if uc.worldEventRepo == nil || current == nil {
		return nil
	}

	events, err := uc.worldEventRepo.GetByLocationID(ctx, current.ID)
	if err != nil {
		return fmt.Errorf("failed to get location events: %w", err)
	}

	for i := range events {
		ev := events[i]
		if !isLocationEventType(ev.Type) || ev.Status != world.WorldEventStatusActive {
			continue
		}

		ev.Cancel()
		now := time.Now()
		ev.CompletedAt = &now
		ev.UpdatedAt = now
		ev.Metadata = updateLocationEventMetadataStatus(ev.Metadata, "ignored")

		if err := uc.worldEventRepo.Save(ctx, &ev); err != nil {
			return fmt.Errorf("failed to save location event: %w", err)
		}

		content := buildLocationEventOutcomeStory(&ev, "Событие локации проигнорировано — игрок покинул локацию.")
		if uc.eventRepo != nil {
			eventItem := &event.StoryEvent{
				GameSessionID: gs.ID,
				AuthorType:    event.AuthorTypeDM,
				Content:       content,
				CreatedAt:     time.Now(),
			}
			if err := uc.eventRepo.Save(ctx, eventItem); err != nil {
				return fmt.Errorf("failed to save location outcome story event: %w", err)
			}
		}

		if uc.indexDocUC != nil {
			doc := ragdomain.Document{
				ID:        uuid.New().String(),
				Source:    ragdomain.SourceEvent,
				SessionID: gs.ID,
				Text:      content,
				Timestamp: time.Now(),
			}
			if err := uc.indexDocUC.Execute(ctx, doc); err != nil {
				return fmt.Errorf("failed to index location outcome in RAG: %w", err)
			}
		}
	}

	return nil
}

func isLocationEventType(t world.WorldEventType) bool {
	switch t {
	case world.WorldEventTypeLocationNPC,
		world.WorldEventTypeLocationItem,
		world.WorldEventTypeLocationTrap,
		world.WorldEventTypeLocationPuzzle,
		world.WorldEventTypeLocationEncounter:
		return true
	default:
		return false
	}
}

func updateLocationEventMetadataStatus(meta []byte, status string) []byte {
	if len(meta) == 0 {
		return meta
	}
	var payload world.LocationEventMetadata
	if err := json.Unmarshal(meta, &payload); err != nil {
		return meta
	}
	payload.Status = status
	updated, err := json.Marshal(payload)
	if err != nil {
		return meta
	}
	return updated
}

func buildLocationEventOutcomeStory(ev *world.WorldEvent, outcome string) string {
	if ev == nil {
		return outcome
	}

	parts := []string{
		fmt.Sprintf("Событие локации: %s", ev.Name),
		outcome,
	}
	if ev.Description != "" {
		parts = append(parts, fmt.Sprintf("Описание: %s", ev.Description))
	}

	return strings.Join(parts, "\n")
}

func normalizeDirection(dir string) string {
	d := strings.ToLower(strings.TrimSpace(dir))
	switch d {
	case "n":
		return "north"
	case "s":
		return "south"
	case "e":
		return "east"
	case "w":
		return "west"
	case "u":
		return "up"
	case "d":
		return "down"
	}
	return d
}
