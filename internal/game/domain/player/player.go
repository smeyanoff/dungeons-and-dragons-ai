package player

import (
	"dungeons-and-dragons-ai/internal/game/domain/character"
)

type Player struct {
	ID       uint  `gorm:"primaryKey"`
	TgUserID int64 `gorm:"uniqueIndex"`
	Name     string

	GameSessionID uint
	Character     character.Character
	CharacterID   uint
}
