package world

type Item struct {
	ID         uint `gorm:"primaryKey"`
	LocationID uint `gorm:"index"`
	Name       string
	Type       string
	Effect     string
}
