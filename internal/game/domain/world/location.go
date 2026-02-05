package world

import "encoding/json"

// PredefinedCheck представляет предопределенную проверку в локации
type PredefinedCheck struct {
	Ability      string `json:"ability"`       // Характеристика: "strength", "dexterity", "constitution", "intelligence", "wisdom", "charisma"
	DC           int    `json:"dc"`            // Сложность проверки (Difficulty Class)
	Description  string `json:"description"`   // Описание проверки для игрока
	LocationHint string `json:"location_hint"` // Подсказка о том, где в локации находится проверка
}

// LocationScenario представляет сценарий для локации (цель, события, возможные исходы)
type LocationScenario struct {
	ID               string   `json:"id"`                // Уникальный ID сценария
	Title            string   `json:"title"`             // Название сценария
	Description      string   `json:"description"`       // Подробное описание
	Objective        string   `json:"objective"`         // Цель, которую нужно достичь
	KeyEvents        []string `json:"key_events"`        // Ключевые события сценария
	PossibleOutcomes []string `json:"possible_outcomes"` // Возможные исходы
	Reward           string   `json:"reward"`            // Награда за успешное завершение
	Status           string   `json:"status"`            // Статус: "not_started", "in_progress", "completed", "failed"
	CreatedAt        string   `json:"created_at"`        // Время создания
}

type Location struct {
	ID          uint `gorm:"primaryKey"`
	WorldID     uint `gorm:"index"`
	Name        string
	Description string

	// PredefinedChecks хранятся как JSON в БД
	PredefinedChecksJSON json.RawMessage `gorm:"type:jsonb" json:"-"` // JSON представление предопределенных проверок для БД

	// Scenario - сгенерированный сценарий для локации (цель, события, исходы)
	ScenarioJSON json.RawMessage `gorm:"type:jsonb" json:"-"` // JSON представление сценария для БД

	// Visited отмечает, что локация была посещена в рамках текущего мира
	Visited bool `gorm:"index"`

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

// GetScenario возвращает сценарий локации (десериализация из JSON)
func (l *Location) GetScenario() *LocationScenario {
	if len(l.ScenarioJSON) == 0 {
		return nil
	}
	var scenario LocationScenario
	if err := json.Unmarshal(l.ScenarioJSON, &scenario); err != nil {
		return nil
	}
	return &scenario
}

// SetScenario устанавливает сценарий локации (сериализация в JSON)
func (l *Location) SetScenario(scenario *LocationScenario) error {
	if scenario == nil {
		l.ScenarioJSON = nil
		return nil
	}
	data, err := json.Marshal(scenario)
	if err != nil {
		return err
	}
	l.ScenarioJSON = json.RawMessage(data)
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
