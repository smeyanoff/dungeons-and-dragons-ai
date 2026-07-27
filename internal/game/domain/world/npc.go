package world

type NPC struct {
	ID          uint `gorm:"primaryKey"`
	LocationID  uint `gorm:"index"`
	Name        string
	Role        string
	Personality string
	// Attitude — отношение NPC к игроку (hostile/wary/neutral/friendly), заданное при создании
	// и передаваемое в контекст промпта DM, чтобы поведение NPC соответствовало характеру.
	Attitude string
}
