package campaign

type CampaignGenerationDTO struct {
	MainQuest QuestDTO      `json:"main_quest"`
	Locations []LocationDTO `json:"locations"`
}

type QuestDTO struct {
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Items       []ItemDTO `json:"items"`
}

type LocationDTO struct {
	Name             string               `json:"name"`
	Description      string               `json:"description"`
	NPCs             []NPCDTO             `json:"npcs"`
	Connections      []ConnectionDTO      `json:"connections,omitempty"`
	PredefinedChecks []PredefinedCheckDTO `json:"predefined_checks,omitempty"`
}

type PredefinedCheckDTO struct {
	Ability      string `json:"ability"`       // Характеристика: "strength", "dexterity", "constitution", "intelligence", "wisdom", "charisma"
	DC           int    `json:"dc"`            // Сложность проверки (Difficulty Class)
	Description  string `json:"description"`   // Описание проверки для игрока
	LocationHint string `json:"location_hint"` // Подсказка о том, где в локации находится проверка
}

type ConnectionDTO struct {
	ToLocation  string `json:"to_location"` // Имя целевой локации
	Direction   string `json:"direction"`   // "north", "south", "east", "west", "up", "down", "portal", etc.
	Description string `json:"description"` // Описание пути
}

type NPCDTO struct {
	Name     string `json:"name"`
	Role     string `json:"role"`
	Relation string `json:"relation,omitempty"` // Родство/принадлежность к другому NPC или фракции (если применимо)
}

type ItemDTO struct {
	Name    string `json:"name"`
	Purpose string `json:"purpose"`
}
