package npc

func New(name, role, attitude string) *NPC {
	return &NPC{
		Name:     name,
		Role:     role,
		Attitude: attitude,
	}
}

type NPC struct {
	Name string
	Role string
	// Attitude — отношение NPC к игроку, заданное при создании (hostile/wary/neutral/friendly).
	// Используется при построении контекста DM, чтобы NPC вели себя согласно характеру
	// (например, злодеи по природе враждебны игроку, а не нейтральны по умолчанию).
	Attitude string
}
