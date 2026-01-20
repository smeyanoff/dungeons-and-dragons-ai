package world

import (
	"time"

	"gorm.io/datatypes"
)

// WorldEventType тип мирового события
type WorldEventType string

const (
	WorldEventTypeRandomEncounter WorldEventType = "random_encounter" // Случайная встреча
	WorldEventTypeFestival        WorldEventType = "festival"         // Праздник
	WorldEventTypeSeasonal        WorldEventType = "seasonal"         // Сезонное событие
	WorldEventTypeNatural         WorldEventType = "natural"          // Природное явление (метеоритный дождь, северное сияние и т.д.)
	WorldEventTypeMarket          WorldEventType = "market"           // Рынок/ярмарка
	WorldEventTypeQuest           WorldEventType = "quest"            // Специальный квест
	// События локаций (автоматическая генерация)
	WorldEventTypeLocationNPC      WorldEventType = "location_npc"      // Встреча с NPC в локации
	WorldEventTypeLocationItem     WorldEventType = "location_item"     // Находка предмета в локации
	WorldEventTypeLocationTrap     WorldEventType = "location_trap"     // Ловушка в локации
	WorldEventTypeLocationPuzzle   WorldEventType = "location_puzzle"   // Загадка в локации
	WorldEventTypeLocationEncounter WorldEventType = "location_encounter" // Случайная встреча в локации
)

// WorldEventStatus статус мирового события
type WorldEventStatus string

const (
	WorldEventStatusScheduled WorldEventStatus = "scheduled" // Запланировано
	WorldEventStatusActive    WorldEventStatus = "active"    // Активно
	WorldEventStatusCompleted WorldEventStatus = "completed" // Завершено
	WorldEventStatusCancelled WorldEventStatus = "cancelled" // Отменено
)

// LocationEventMetadata описывает структурированную полезную нагрузку событий локации.
type LocationEventMetadata struct {
	Hook            string   `json:"hook"`
	Options         []string `json:"options,omitempty"`
	SuggestedChecks []string `json:"suggested_checks,omitempty"`
	Stakes          string   `json:"stakes,omitempty"`
	Status          string   `json:"status,omitempty"`
}

// WorldEvent представляет динамическое событие в игровом мире
type WorldEvent struct {
	ID          uint             `gorm:"primaryKey"`
	WorldID     uint             `gorm:"index"`
	Type        WorldEventType   `gorm:"type:varchar(32);not null"`
	Status      WorldEventStatus `gorm:"type:varchar(16);not null"`
	Name        string           `gorm:"type:varchar(255);not null"`
	Description string           `gorm:"type:text"`
	Metadata    datatypes.JSON   `gorm:"type:jsonb"`
	
	// Условия активации
	RequiredDayOfWeek *int    // День недели (0-6, где 0 = воскресенье) или nil если не важно
	RequiredTimeOfDay string  // Время суток (morning, noon, evening, night и т.д.) или "" если не важно
	RequiredWeather   string  // Погода (clear, rainy, stormy и т.д.) или "" если не важно
	RequiredLocationID *uint  // ID локации или nil если не важно
	
	// Время
	ScheduledFor *time.Time // Запланированное время активации
	ActivatedAt  *time.Time // Время активации
	CompletedAt  *time.Time // Время завершения
	
	// Повторяемость
	IsRecurring bool       // Повторяющееся ли событие
	RecurrenceInterval *int // Интервал повторения в днях (nil если не повторяющееся)
	
	CreatedAt time.Time
	UpdatedAt time.Time
}

// CanActivate проверяет, может ли событие быть активировано в данный момент
func (e *WorldEvent) CanActivate(
	dayOfWeek int,
	timeOfDay string,
	weather string,
	currentLocationID *uint,
) bool {
	// Проверяем статус
	if e.Status != WorldEventStatusScheduled {
		return false
	}
	
	// Проверяем запланированное время
	if e.ScheduledFor != nil && time.Now().Before(*e.ScheduledFor) {
		return false
	}
	
	// Проверяем день недели
	if e.RequiredDayOfWeek != nil && *e.RequiredDayOfWeek != dayOfWeek {
		return false
	}
	
	// Проверяем время суток
	if e.RequiredTimeOfDay != "" && e.RequiredTimeOfDay != timeOfDay {
		return false
	}
	
	// Проверяем погоду
	if e.RequiredWeather != "" && e.RequiredWeather != weather {
		return false
	}
	
	// Проверяем локацию
	if e.RequiredLocationID != nil {
		if currentLocationID == nil || *e.RequiredLocationID != *currentLocationID {
			return false
		}
	}
	
	return true
}

// Activate активирует событие
func (e *WorldEvent) Activate() {
	e.Status = WorldEventStatusActive
	now := time.Now()
	e.ActivatedAt = &now
}

// Complete завершает событие
func (e *WorldEvent) Complete() {
	e.Status = WorldEventStatusCompleted
	now := time.Now()
	e.CompletedAt = &now
}

// Cancel отменяет событие
func (e *WorldEvent) Cancel() {
	e.Status = WorldEventStatusCancelled
}

// Reschedule перепланирует повторяющееся событие
func (e *WorldEvent) Reschedule(days int) {
	if !e.IsRecurring || e.RecurrenceInterval == nil {
		return
	}
	
	// Устанавливаем следующее запланированное время
	nextTime := time.Now().AddDate(0, 0, days)
	e.ScheduledFor = &nextTime
	e.Status = WorldEventStatusScheduled
	e.ActivatedAt = nil
	e.CompletedAt = nil
}
