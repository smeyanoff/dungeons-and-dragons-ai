package domain

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
}
