package worldmap

import (
	"context"
	"fmt"
	"strings"

	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/game/domain/world"
)

type MoveToLocationUseCase struct {
	sessionRepo session.Repository
}

func NewMoveToLocationUseCase(sessionRepo session.Repository) *MoveToLocationUseCase {
	return &MoveToLocationUseCase{sessionRepo: sessionRepo}
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

