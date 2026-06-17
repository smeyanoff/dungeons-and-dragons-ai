package player

import (
	"dungeons-and-dragons-ai/internal/game/domain/character"
	"encoding/json"
)

// NarrativeStyle определяет стиль повествования
type NarrativeStyle string

const (
	NarrativeStyleBalanced    NarrativeStyle = "balanced"     // Сбалансированный стиль
	NarrativeStyleDark        NarrativeStyle = "dark"         // Темный, мрачный стиль
	NarrativeStyleLight       NarrativeStyle = "light"        // Светлый, позитивный стиль
	NarrativeStyleDetailed    NarrativeStyle = "detailed"     // Детализированный
	NarrativeStyleMinimalist  NarrativeStyle = "minimalist"   // Минималистичный
)

// DetailLevel определяет уровень детализации описаний
type DetailLevel string

const (
	DetailLevelLow    DetailLevel = "low"    // Минимальные описания
	DetailLevelMedium DetailLevel = "medium" // Средний уровень
	DetailLevelHigh   DetailLevel = "high"   // Максимальная детализация
)

// UserPreferences настройки персонализации пользователя
type UserPreferences struct {
	NarrativeStyle NarrativeStyle `json:"narrative_style"` // Стиль повествования
	DetailLevel    DetailLevel    `json:"detail_level"`    // Уровень детализации
	Language       string         `json:"language"`        // Язык (ru, en)
	ShowStats      bool           `json:"show_stats"`      // Показывать ли статистику после действий
}

type Player struct {
	ID       uint  `gorm:"primaryKey"`
	TgUserID int64 `gorm:"uniqueIndex"`
	Name     string

	GameSessionID uint
	Character     character.Character
	CharacterID   uint

	// Настройки персонализации
	PreferencesJSON json.RawMessage `gorm:"type:jsonb" json:"-"`
	Preferences     UserPreferences `gorm:"-"` // В памяти для удобства
}

// GetPreferences возвращает настройки пользователя (десериализация из JSON)
func (p *Player) GetPreferences() UserPreferences {
	if len(p.PreferencesJSON) == 0 {
		// Возвращаем настройки по умолчанию
		return UserPreferences{
			NarrativeStyle: NarrativeStyleBalanced,
			DetailLevel:    DetailLevelMedium,
			Language:       "ru",
			ShowStats:      true,
		}
	}

	if p.Preferences.NarrativeStyle != "" {
		// Уже десериализовано
		return p.Preferences
	}

	var prefs UserPreferences
	if err := json.Unmarshal(p.PreferencesJSON, &prefs); err != nil {
		// В случае ошибки возвращаем значения по умолчанию
		return UserPreferences{
			NarrativeStyle: NarrativeStyleBalanced,
			DetailLevel:    DetailLevelMedium,
			Language:       "ru",
			ShowStats:      true,
		}
	}

	p.Preferences = prefs
	return prefs
}

// SetPreferences устанавливает настройки пользователя (сериализация в JSON)
func (p *Player) SetPreferences(prefs UserPreferences) error {
	p.Preferences = prefs
	data, err := json.Marshal(prefs)
	if err != nil {
		return err
	}
	p.PreferencesJSON = json.RawMessage(data)
	return nil
}
