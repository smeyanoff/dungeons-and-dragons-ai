package location_event

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"dungeons-and-dragons-ai/internal/game/domain/world"
)

// Инициализация генератора случайных чисел с seed на основе времени
// Это обеспечивает разнообразие событий между запусками
var rng *rand.Rand

func init() {
	rng = rand.New(rand.NewSource(time.Now().UnixNano()))
}

// LocationEventGenerator генерирует автоматические события для локаций
type LocationEventGenerator struct {
	eventRepo LocationEventRepository
}

// LocationEventRepository интерфейс для работы с событиями локаций
type LocationEventRepository interface {
	GetByLocationID(ctx context.Context, locationID uint) ([]world.WorldEvent, error)
	Save(ctx context.Context, e *world.WorldEvent) error
}

// GenerateLocationEventRequest запрос на генерацию события для локации
type GenerateLocationEventRequest struct {
	WorldID      uint
	LocationID   uint
	LocationName string
	IsFirstVisit bool
}

// LocationEventType тип события в локации
type LocationEventType string

const (
	EventTypeNPC       LocationEventType = "npc"       // Встреча с NPC
	EventTypeItem      LocationEventType = "item"      // Находка предмета
	EventTypeTrap      LocationEventType = "trap"      // Ловушка
	EventTypePuzzle    LocationEventType = "puzzle"    // Загадка
	EventTypeEncounter LocationEventType = "encounter" // Случайная встреча/бой
)

const (
	locationEventStatusPending  = "pending"
	locationEventStatusResolved = "resolved"
	locationEventStatusExpired  = "expired"
)

const (
	locationEventCooldown        = 20 * time.Minute
	locationEventPendingTTL      = 30 * time.Minute
	locationEventWindow          = 24 * time.Hour
	maxEventsPerLocationPerWindow = 3
)

// GenerateLocationEventResponse ответ с сгенерированным событием
type GenerateLocationEventResponse struct {
	Event       *world.WorldEvent
	Description string
}

func NewLocationEventGenerator(eventRepo LocationEventRepository) *LocationEventGenerator {
	return &LocationEventGenerator{
		eventRepo: eventRepo,
	}
}

// Execute генерирует случайное событие для локации при первом посещении
func (g *LocationEventGenerator) Execute(
	ctx context.Context,
	req GenerateLocationEventRequest,
) (*GenerateLocationEventResponse, error) {
	// Генерируем события только при первом посещении (страхуем интеграции).
	if !req.IsFirstVisit {
		return nil, nil
	}

	// Проверяем, было ли уже сгенерировано событие для этой локации
	existingEvents, err := g.eventRepo.GetByLocationID(ctx, req.LocationID)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing events: %w", err)
	}

	now := time.Now()
	if len(existingEvents) > 0 {
		recentCount := 0
		for i := range existingEvents {
			ev := existingEvents[i]
			if now.Sub(ev.CreatedAt) <= locationEventWindow {
				recentCount++
			}
		}
		if recentCount >= maxEventsPerLocationPerWindow {
			return nil, nil
		}

		hasActivePending := false
		var latestEvent *world.WorldEvent
		for i := range existingEvents {
			ev := &existingEvents[i]
			if latestEvent == nil || ev.CreatedAt.After(latestEvent.CreatedAt) {
				latestEvent = ev
			}
			meta, ok := parseLocationEventMetadata(ev)
			if !ok || meta.Status == "" {
				meta.Status = locationEventStatusPending
			}
			if meta.Status == locationEventStatusPending {
				if now.Sub(ev.CreatedAt) <= locationEventPendingTTL {
					hasActivePending = true
				} else {
					if err := g.expireLocationEvent(ctx, ev); err != nil {
						return nil, fmt.Errorf("failed to expire location event: %w", err)
					}
				}
			}
		}

		if hasActivePending {
			return nil, nil
		}
		if latestEvent != nil && now.Sub(latestEvent.CreatedAt) < locationEventCooldown {
			return nil, nil
		}
	}

	// Генерируем случайный тип события (веса для разнообразия)
	eventType := g.rollEventType()

	// Создаем событие на основе типа
	event, description := g.createLocationEvent(req, eventType)

	// Сохраняем событие
	if err := g.eventRepo.Save(ctx, event); err != nil {
		return nil, fmt.Errorf("failed to save location event: %w", err)
	}

	return &GenerateLocationEventResponse{
		Event:       event,
		Description: description,
	}, nil
}

// rollEventType определяет тип события на основе весов
// Веса: NPC (30%), Item (25%), Encounter (20%), Trap (15%), Puzzle (10%)
func (g *LocationEventGenerator) rollEventType() LocationEventType {
	r := rng.Intn(100)
	if r < 30 {
		return EventTypeNPC
	} else if r < 55 {
		return EventTypeItem
	} else if r < 75 {
		return EventTypeEncounter
	} else if r < 90 {
		return EventTypeTrap
	}
	return EventTypePuzzle
}

// createLocationEvent создает событие локации на основе типа
func (g *LocationEventGenerator) createLocationEvent(
	req GenerateLocationEventRequest,
	eventType LocationEventType,
) (*world.WorldEvent, string) {
	now := time.Now()
	locationID := req.LocationID

	var worldEventType world.WorldEventType
	var name string
	var description string
	var options []string
	var checks []string
	var stakes string

	switch eventType {
	case EventTypeNPC:
		worldEventType = world.WorldEventTypeLocationNPC
		name = fmt.Sprintf("Встреча в %s", req.LocationName)
		description = fmt.Sprintf("В локации %s вас встречает интересный персонаж, готовый пообщаться.", req.LocationName)
		options = []string{"Поговорить", "Спросить о локации", "Игнорировать"}
		checks = []string{"charisma"}
		stakes = "Можно получить информацию или помощь, но есть риск навлечь внимание."
	case EventTypeItem:
		worldEventType = world.WorldEventTypeLocationItem
		name = fmt.Sprintf("Находка в %s", req.LocationName)
		description = fmt.Sprintf("В локации %s вы находите заинтересовавший вас предмет.", req.LocationName)
		options = []string{"Осмотреть", "Взять", "Оставить"}
		checks = []string{"wisdom"}
		stakes = "Можно получить полезный предмет или попасть в неприятность."
	case EventTypeTrap:
		worldEventType = world.WorldEventTypeLocationTrap
		name = fmt.Sprintf("Ловушка в %s", req.LocationName)
		description = fmt.Sprintf("Осторожно! В локации %s вас поджидает опасная ловушка.", req.LocationName)
		options = []string{"Попытаться обезвредить", "Обойти", "Осмотреть механизмы"}
		checks = []string{"dexterity"}
		stakes = "Ошибка может привести к урону или потере времени."
	case EventTypePuzzle:
		worldEventType = world.WorldEventTypeLocationPuzzle
		name = fmt.Sprintf("Загадка в %s", req.LocationName)
		description = fmt.Sprintf("В локации %s вы обнаруживаете загадочную загадку, требующую решения.", req.LocationName)
		options = []string{"Попытаться решить", "Изучить детали", "Отступить"}
		checks = []string{"intelligence"}
		stakes = "Успех может открыть доступ к награде или проходу."
	case EventTypeEncounter:
		worldEventType = world.WorldEventTypeLocationEncounter
		name = fmt.Sprintf("Встреча в %s", req.LocationName)
		description = fmt.Sprintf("В локации %s вы столкнулись с неожиданной встречей, возможно опасной.", req.LocationName)
		options = []string{"Подготовиться к бою", "Попытаться договориться", "Спрятаться"}
		checks = []string{"dexterity", "charisma"}
		stakes = "Исход встречи может изменить ситуацию в локации."
	}

	event := &world.WorldEvent{
		WorldID:            req.WorldID,
		Type:               worldEventType,
		Status:             world.WorldEventStatusActive,
		Name:               name,
		Description:        description,
		Metadata:           buildLocationEventMetadata(description, options, checks, stakes, locationEventStatusPending),
		RequiredLocationID: &locationID,
		ActivatedAt:        &now,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	return event, description
}

func buildLocationEventMetadata(hook string, options []string, checks []string, stakes string, status string) []byte {
	meta := world.LocationEventMetadata{
		Hook:            hook,
		Options:         options,
		SuggestedChecks: checks,
		Stakes:          stakes,
		Status:          status,
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		return nil
	}
	return raw
}

func parseLocationEventMetadata(event *world.WorldEvent) (world.LocationEventMetadata, bool) {
	if event == nil || len(event.Metadata) == 0 {
		return world.LocationEventMetadata{}, false
	}
	var meta world.LocationEventMetadata
	if err := json.Unmarshal(event.Metadata, &meta); err != nil {
		return world.LocationEventMetadata{}, false
	}
	return meta, true
}

func (g *LocationEventGenerator) expireLocationEvent(ctx context.Context, ev *world.WorldEvent) error {
	meta, ok := parseLocationEventMetadata(ev)
	if !ok {
		meta = world.LocationEventMetadata{}
	}
	meta.Status = locationEventStatusExpired
	updated := buildLocationEventMetadata(meta.Hook, meta.Options, meta.SuggestedChecks, meta.Stakes, meta.Status)
	ev.Metadata = updated
	ev.Status = world.WorldEventStatusCancelled
	now := time.Now()
	ev.UpdatedAt = now
	return g.eventRepo.Save(ctx, ev)
}
