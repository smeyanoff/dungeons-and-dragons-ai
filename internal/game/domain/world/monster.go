package world

type Monster struct {
	ID         uint `gorm:"primaryKey"`
	LocationID uint `gorm:"index"`
	Name       string
	HP         int
	Attack     int
}
