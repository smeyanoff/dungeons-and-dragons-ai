package world

import "encoding/json"

// PredefinedCheck представляет предопределенную проверку в локации
type PredefinedCheck struct {
	Ability      string `json:"ability"`       // Характеристика: "strength", "dexterity", "constitution", "intelligence", "wisdom", "charisma"
	DC           int    `json:"dc"`            // Сложность проверки (Difficulty Class)
	Description  string `json:"description"`   // Описание проверки для игрока
	LocationHint string `json:"location_hint"` // Подсказка о том, где в локации находится проверка
}

type Location struct {
	ID          uint `gorm:"primaryKey"`
	WorldID     uint `gorm:"index"`
	Name        string
	Description string

	// PredefinedChecks хранятся как JSON в БД
	PredefinedChecksJSON json.RawMessage `gorm:"type:jsonb" json:"-"` // JSON представление предопределенных проверок для БД

	NPCs        []NPC
	Monsters    []Monster
	Connections []LocationConnection `gorm:"foreignKey:FromLocationID"`
}

// PredefinedChecks возвращает список предопределенных проверок (десериализация из JSON)
func (l *Location) PredefinedChecks() []PredefinedCheck {
	if len(l.PredefinedChecksJSON) == 0 {
		return []PredefinedCheck{}
	}
	var checks []PredefinedCheck
	if err := json.Unmarshal(l.PredefinedChecksJSON, &checks); err != nil {
		return []PredefinedCheck{}
	}
	return checks
}

// SetPredefinedChecks устанавливает предопределенные проверки (сериализация в JSON)
func (l *Location) SetPredefinedChecks(checks []PredefinedCheck) error {
	if len(checks) == 0 {
		l.PredefinedChecksJSON = nil
		return nil
	}
	data, err := json.Marshal(checks)
	if err != nil {
		return err
	}
	l.PredefinedChecksJSON = json.RawMessage(data)
	return nil
}

// LocationConnection представляет связь между двумя локациями
type LocationConnection struct {
	ID             uint   `gorm:"primaryKey"`
	FromLocationID uint   `gorm:"index"`
	ToLocationID   uint   `gorm:"index"`
	Direction      string // "north", "south", "east", "west", "up", "down", "portal", etc.
	Description    string // Описание пути (например, "узкая тропа", "магический портал")
}
