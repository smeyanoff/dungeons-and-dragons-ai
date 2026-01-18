package world

type Location struct {
	ID          uint `gorm:"primaryKey"`
	WorldID     uint `gorm:"index"`
	Name        string
	Description string

	NPCs        []NPC
	Monsters    []Monster
	Connections []LocationConnection `gorm:"foreignKey:FromLocationID"`
}

// LocationConnection представляет связь между двумя локациями
type LocationConnection struct {
	ID             uint   `gorm:"primaryKey"`
	FromLocationID uint   `gorm:"index"`
	ToLocationID   uint   `gorm:"index"`
	Direction      string // "north", "south", "east", "west", "up", "down", "portal", etc.
	Description    string // Описание пути (например, "узкая тропа", "магический портал")
}
