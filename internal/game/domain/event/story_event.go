package event

import "time"

type AuthorType string

const (
	AuthorTypePlayer AuthorType = "player"
	AuthorTypeDM     AuthorType = "dm"
	AuthorTypeNPC    AuthorType = "npc"
)

type StoryEvent struct {
	ID            uint       `gorm:"primaryKey"`
	GameSessionID uint       `gorm:"index"`
	// LocationID — локация, в которой произошло событие (nil для старых записей и событий без привязки к месту).
	LocationID *uint      `gorm:"index"`
	AuthorType AuthorType `gorm:"type:varchar(16);not null"`
	AuthorName string
	Content    string `gorm:"type:text"`
	CreatedAt  time.Time
}
