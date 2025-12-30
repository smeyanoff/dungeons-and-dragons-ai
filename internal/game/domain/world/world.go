package world

import (
	"gorm.io/gorm"
)

type World struct {
	ID          uint `gorm:"primaryKey"`
	Name        string
	Description string

	Locations []Location
}
