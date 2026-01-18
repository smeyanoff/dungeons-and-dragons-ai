package session

import (
	"context"

	"dungeons-and-dragons-ai/internal/game/domain/player"
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

	Players []player.Player `gorm:"foreignKey:GameSessionID"`
}

func (s *GameSession) IsActive() bool {
	return s.State == StateActive
}

// End завершает игру, устанавливая статус в StateDone
func (s *GameSession) End() {
	s.State = StateDone
}

// FindPlayerByTgUserID находит игрока по Telegram User ID
// В приватных чатах chatID == userID, поэтому это работает для приватных чатов
// Для групповых чатов нужно передавать userID явно
func (s *GameSession) FindPlayerByTgUserID(tgUserID int64) *player.Player {
	for i := range s.Players {
		if s.Players[i].TgUserID == tgUserID {
			return &s.Players[i]
		}
	}
	return nil
}

// GetFirstPlayer возвращает первого игрока (для обратной совместимости)
// В будущем для мультиплеера лучше использовать FindPlayerByTgUserID
func (s *GameSession) GetFirstPlayer() *player.Player {
	if len(s.Players) == 0 {
		return nil
	}
	return &s.Players[0]
}

type Repository interface {
	GetByChatID(ctx context.Context, chatID int64) (*GameSession, error)
	Save(ctx context.Context, session *GameSession) error
	Delete(ctx context.Context, chatID int64) error
}
