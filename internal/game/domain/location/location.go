package location

import "dungeons-and-dragons-ai/internal/game/domain/npc"

func New(name, description string) *Location {
	return &Location{
		Name:        name,
		Description: description,
		NPCs:        []npc.NPC{},
	}
}

func (l *Location) AddNPC(n *npc.NPC) {
	l.NPCs = append(l.NPCs, *n)
}

type Location struct {
	Name        string
	Description string
	NPCs        []npc.NPC
}
