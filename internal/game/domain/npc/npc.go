package npc

func New(name, role string) *NPC {
	return &NPC{
		Name: name,
		Role: role,
	}
}

type NPC struct {
	Name string
	Role string
}
