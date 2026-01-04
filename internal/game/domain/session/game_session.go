package session

import (
	"context"
	"time"

	"dungeons-and-dragons-ai/internal/game/domain/world"
	"gorm.io/gorm"
)

type State string

const (
	StateActive State = "active"
	StateDone   State = "done"
	StateFailed State = "failed"
	StatePaused State = "paused"
)

type GameSession struct {
	gorm.Model
	ChatID int64 `gorm:"uniqueIndex"`
	State  State

	World   world.World
	WorldID uint
}

func (s *GameSession) IsActive() bool {
	return s.State == State.StateActive
}

type Repository interface {
	GetByChatID(ctx context.Context, chatID int64) (*GameSession, error)
	Save(ctx context.Context, session *GameSession) error
}
