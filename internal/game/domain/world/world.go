package world

import (
	"context"
	"time"
	
	"dungeons-and-dragons-ai/internal/game/domain/location"
	"dungeons-and-dragons-ai/internal/game/domain/quest"
)

func New(name string) *World {
	now := time.Now()
	return &World{
		Name:        name,
		Description: name,
		Locations:   []Location{},
		Events:      []WorldEvent{},
		TimeOfDay:   "morning", // По умолчанию утро
		Weather:     "clear",   // По умолчанию ясная погода
		Day:         1,         // Начинаем с первого дня
		Week:        1,         // Начинаем с первой недели
		Month:       1,         // Начинаем с первого месяца
		Season:      "spring",  // По умолчанию весна
		StartedAt:   now,       // Время создания мира
	}
}

// SetTimeOfDay устанавливает время суток
func (w *World) SetTimeOfDay(timeOfDay string) {
	w.TimeOfDay = timeOfDay
}

// SetWeather устанавливает погоду
func (w *World) SetWeather(weather string) {
	w.Weather = weather
}

// AdvanceTime продвигает время в мире (может вызываться после определенных действий)
func (w *World) AdvanceTime() {
	// Продвигаем время суток
	switch w.TimeOfDay {
	case "morning":
		w.TimeOfDay = "noon"
	case "noon":
		w.TimeOfDay = "afternoon"
	case "afternoon":
		w.TimeOfDay = "evening"
	case "evening":
		w.TimeOfDay = "night"
		w.AdvanceDay() // Переходим на следующий день при наступлении ночи
	case "night":
		w.TimeOfDay = "midnight"
	case "midnight":
		w.TimeOfDay = "morning"
		w.AdvanceDay()
	}
}

// AdvanceDay продвигает день в календаре
func (w *World) AdvanceDay() {
	w.Day++
	
	// Обновляем неделю (каждые 7 дней)
	if w.Day%7 == 0 {
		w.Week = (w.Day / 7) + 1
	}
	
	// Обновляем месяц (каждые 30 дней)
	if w.Day%30 == 0 {
		w.Month = (w.Day / 30) + 1
	}
	
	// Обновляем сезон (каждые 90 дней, 4 сезона)
	seasonIndex := ((w.Day - 1) / 90) % 4
	seasons := []string{"spring", "summer", "autumn", "winter"}
	w.Season = seasons[seasonIndex]
}

// GetDayOfWeek возвращает день недели (0-6, где 0 = воскресенье)
func (w *World) GetDayOfWeek() int {
	// Используем начальное время + количество дней для расчета дня недели
	startDate := w.StartedAt
	if startDate.IsZero() {
		startDate = time.Now()
	}
	// Добавляем дни к начальной дате и получаем день недели
	targetDate := startDate.AddDate(0, 0, w.Day-1)
	return int(targetDate.Weekday())
}

func (w *World) SetMainQuest(q *quest.Quest) {
	w.MainQuest = q
	if q != nil {
		q.WorldID = w.ID
	}
}

func (w *World) AddLocation(loc *location.Location) {
	worldLoc := Location{
		Name:        loc.Name,
		Description: loc.Description,
		NPCs:        make([]NPC, 0, len(loc.NPCs)),
	}
	// Конвертируем NPCs из location.NPC в world.NPC
	for _, npc := range loc.NPCs {
		worldLoc.NPCs = append(worldLoc.NPCs, NPC{
			Name:        npc.Name,
			Role:        npc.Role,
			Personality: "",
		})
	}
	w.Locations = append(w.Locations, worldLoc)
}

type World struct {
	ID          uint `gorm:"primaryKey"`
	Name        string
	Description string

	MainQuest   *quest.Quest
	MainQuestID *uint
	Locations   []Location
	Events      []WorldEvent `gorm:"foreignKey:WorldID"`

	// Время суток и погода
	TimeOfDay string // "morning", "noon", "afternoon", "evening", "night", "midnight"
	Weather   string // "clear", "cloudy", "rainy", "stormy", "foggy", "snowy"
	
	// Календарь
	Day      int    // День от начала игры (начинается с 1)
	Week     int    // Неделя от начала игры (начинается с 1)
	Month    int    // Месяц от начала игры (начинается с 1)
	Season   string // Сезон: "spring", "summer", "autumn", "winter" (опционально)
	StartedAt time.Time `gorm:"type:timestamp"` // Время начала игры в мире
}

type WorldRepository interface {
	Save(ctx context.Context, w *World) error
}
