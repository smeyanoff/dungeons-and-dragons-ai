package character

type Stats struct {
	ID          uint `gorm:"primaryKey"`
	CharacterID uint `gorm:"uniqueIndex"`

	Strength     int
	Dexterity    int
	Constitution int
	Intelligence int
	Wisdom       int
	Charisma     int
}
