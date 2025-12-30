package world

type Location struct {
	ID          uint `gorm:"primaryKey"`
	WorldID     uint `gorm:"index"`
	Name        string
	Description string

	NPCs     []NPC
	Monsters []Monster
	Items    []Item
}
