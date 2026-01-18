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
	Text      string
	Timestamp time.Time // Временная метка для сортировки по времени
}
