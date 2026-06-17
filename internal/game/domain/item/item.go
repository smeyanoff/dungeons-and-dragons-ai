package item

func New(name, purpose string) *Item {
	return &Item{
		Name:    name,
		Purpose: purpose,
	}
}

type Item struct {
	ID      uint `gorm:"primaryKey"`
	Name    string
	Purpose string
}
