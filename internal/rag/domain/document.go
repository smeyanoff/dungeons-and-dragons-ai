package domain

import "time"

type SourceType string

const (
	SourceWorld    SourceType = "world"
	SourceLocation SourceType = "location"
	SourceNPC      SourceType = "npc"
	SourceEvent    SourceType = "event"
	SourceRule     SourceType = "rule"
)

type Document struct {
	ID        string
	Source    SourceType
	SessionID uint
	// LocationID — локация, к которой относится документ (nil, если документ не привязан к месту).
	LocationID *uint
	Text       string
	Timestamp  time.Time // Временная метка для сортировки по времени
}
