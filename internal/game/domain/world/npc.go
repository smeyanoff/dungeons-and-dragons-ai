package world

type NPC struct {
	ID          uint `gorm:"primaryKey"`
	LocationID  uint `gorm:"index"`
	Name        string
	Role        string
	Personality string
}
