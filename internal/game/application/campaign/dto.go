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
	Name        string          `json:"name"`
	Description string          `json:"description"`
	NPCs        []NPCDTO        `json:"npcs"`
	Connections []ConnectionDTO `json:"connections,omitempty"`
}

type ConnectionDTO struct {
	ToLocation  string `json:"to_location"` // Имя целевой локации
	Direction   string `json:"direction"`   // "north", "south", "east", "west", "up", "down", "portal", etc.
	Description string `json:"description"` // Описание пути
}

type NPCDTO struct {
	Name string `json:"name"`
	Role string `json:"role"`
}

type ItemDTO struct {
	Name    string `json:"name"`
	Purpose string `json:"purpose"`
}
