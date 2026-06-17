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

// createLocationEvent создает событие локации на основе типа с вариативными ветками развития
func (g *LocationEventGenerator) createLocationEvent(
	req GenerateLocationEventRequest,
	eventType LocationEventType,
) (*world.WorldEvent, string) {
	now := time.Now()
	locationID := req.LocationID

	var worldEventType world.WorldEventType
	var name string
	var description string
	var branches []world.LocationEventBranch

	switch eventType {
	case EventTypeNPC:
		worldEventType = world.WorldEventTypeLocationNPC
		name = fmt.Sprintf("Встреча в %s", req.LocationName)
		description = fmt.Sprintf("В локации %s вас встречает таинственный странник. Что вы предпримете?", req.LocationName)

		branches = []world.LocationEventBranch{
			{
				ID:             "talk_friendly",
				Name:           "Дружелюбно поговорить",
				Description:    "Поприветствовать странника и завязать разговор",
				RequiredAction: "поговорить дружелюбно",
				SuccessRate:    70,
				Consequences:   "Можете получить полезную информацию или временного союзника",
				Reward:         "информация о локации или квест",
			},
			{
				ID:             "recruit_companion",
				Name:           "Предложить присоединиться",
				Description:    "Предложить страннику присоединиться к вашему отряду",
				RequiredAction: "предложить присоединиться",
				SuccessRate:    45,
				Consequences:   "Странник может согласиться присоединиться к вашему отряду как компаньон",
				Reward:         "новый компаньон в отряде",
			},
			{
				ID:             "ask_about_location",
				Name:           "Спросить о локации",
				Description:    "Поинтересоваться информацией о текущем месте",
				RequiredAction: "спросить о локации",
				SuccessRate:    60,
				Consequences:   "Получите карту или подсказку",
				Reward:         "карта локации",
			},
			{
				ID:             "ignore_suspicious",
				Name:           "Заподозрить неладное",
				Description:    "Не доверять незнакомцу и держаться настороже",
				RequiredAction: "игнорировать с подозрением",
				SuccessRate:    80,
				Consequences:   "Избегнете возможной ловушки, но упустите机会",
				Reward:         "",
			},
			{
				ID:             "intimidate",
				Name:           "Запугать странника",
				Description:    "Показать силу и потребовать информацию",
				RequiredAction: "запугать",
				SuccessRate:    40,
				Consequences:   "Можете получить информацию силой, но рискуете конфликтом",
				Reward:         "ценная информация",
			},
		}

	case EventTypeItem:
		worldEventType = world.WorldEventTypeLocationItem
		name = fmt.Sprintf("Таинственная находка в %s", req.LocationName)
		description = fmt.Sprintf("В локации %s вы замечаете подозрительный сундук. Что вы сделаете?", req.LocationName)

		branches = []world.LocationEventBranch{
			{
				ID:             "open_carefully",
				Name:           "Аккуратно открыть",
				Description:    "Тщательно осмотреть сундук перед открытием",
				RequiredAction: "осмотреть и открыть аккуратно",
				SuccessRate:    75,
				Consequences:   "Безопасно получите содержимое",
				Reward:         "предмет из сундука",
			},
			{
				ID:             "force_open",
				Name:           "Взломать силой",
				Description:    "Просто сломать замок и открыть",
				RequiredAction: "взломать силой",
				SuccessRate:    50,
				Consequences:   "Можете повредить содержимое или активировать ловушку",
				Reward:         "предмет (возможно поврежденный)",
			},
			{
				ID:             "ignore_caution",
				Name:           "Осторожно проигнорировать",
				Description:    "Не трогать подозрительный сундук",
				RequiredAction: "оставить в покое",
				SuccessRate:    100,
				Consequences:   "Избегнете опасности, но упустите возможную награду",
				Reward:         "",
			},
		}

	case EventTypeTrap:
		worldEventType = world.WorldEventTypeLocationTrap
		name = fmt.Sprintf("Подозрительные признаки в %s", req.LocationName)
		description = fmt.Sprintf("В локации %s вы замечаете странные следы на полу. Кажется, здесь ловушка!", req.LocationName)

		branches = []world.LocationEventBranch{
			{
				ID:             "disarm_expert",
				Name:           "Обезвредить как эксперт",
				Description:    "Тщательно изучить и обезвредить механизм",
				RequiredAction: "обезвредить ловушку",
				SuccessRate:    60,
				Consequences:   "Безопасно пройдете дальше",
				Reward:         "безопасный проход",
			},
			{
				ID:             "trigger_intentionally",
				Name:           "Активировать намеренно",
				Description:    "Специально активировать ловушку в безопасном месте",
				RequiredAction: "активировать контролируемо",
				SuccessRate:    45,
				Consequences:   "Избегнете засады, но получите урон",
				Reward:         "расчистка пути",
			},
			{
				ID:             "avoid_completely",
				Name:           "Полностью обойти",
				Description:    "Найти безопасный обходной путь",
				RequiredAction: "обойти стороной",
				SuccessRate:    70,
				Consequences:   "Потеряете время, но останетесь невредимы",
				Reward:         "безопасный обход",
			},
			{
				ID:             "rush_through",
				Name:           "Броситься напролом",
				Description:    "Быстро пробежать через опасную зону",
				RequiredAction: "рискованно пробежать",
				SuccessRate:    30,
				Consequences:   "Высокий риск получить серьезный урон",
				Reward:         "быстрый проход или провал",
			},
		}

	case EventTypePuzzle:
		worldEventType = world.WorldEventTypeLocationPuzzle
		name = fmt.Sprintf("Загадка в %s", req.LocationName)
		description = fmt.Sprintf("В локации %s перед вами древняя загадка, высеченная на камне. Что вы предпримете?", req.LocationName)

		branches = []world.LocationEventBranch{
			{
				ID:             "solve_intellectually",
				Name:           "Решить умом",
				Description:    "Внимательно изучить и логически решить",
				RequiredAction: "решить загадку умом",
				SuccessRate:    65,
				Consequences:   "Получите награду за сообразительность",
				Reward:         "сокровище или знание",
			},
			{
				ID:             "use_magic",
				Name:           "Использовать магию",
				Description:    "Применить заклинание для решения",
				RequiredAction: "использовать магию",
				SuccessRate:    55,
				Consequences:   "Магическое решение, но потратите ресурсы",
				Reward:         "магическая награда",
			},
			{
				ID:             "brute_force",
				Name:           "Силой сломать",
				Description:    "Просто разрушить механизм загадки",
				RequiredAction: "разрушить силой",
				SuccessRate:    35,
				Consequences:   "Можете повредить механизм или получить урон",
				Reward:         "доступ к следующей комнате",
			},
			{
				ID:             "seek_hint",
				Name:           "Поискать подсказки",
				Description:    "Найти скрытые подсказки вокруг",
				RequiredAction: "искать подсказки",
				SuccessRate:    75,
				Consequences:   "Получите помощь, но потратите время",
				Reward:         "подсказки для решения",
			},
		}

	case EventTypeEncounter:
		worldEventType = world.WorldEventTypeLocationEncounter
		name = fmt.Sprintf("Неожиданная встреча в %s", req.LocationName)
		description = fmt.Sprintf("В локации %s из-за угла выходит группа подозрительных личностей. Ваши действия?", req.LocationName)

		branches = []world.LocationEventBranch{
			{
				ID:             "negotiate_peacefully",
				Name:           "Мирные переговоры",
				Description:    "Попытаться договориться словами",
				RequiredAction: "договориться мирно",
				SuccessRate:    60,
				Consequences:   "Можете получить союзников или информацию",
				Reward:         "союзники или информация",
			},
			{
				ID:             "intimidate_group",
				Name:           "Показать силу",
				Description:    "Запугать группу своей мощью",
				RequiredAction: "запудать силой",
				SuccessRate:    50,
				Consequences:   "Получите уважение или спровоцируете конфликт",
				Reward:         "уважение или конфликт",
			},
			{
				ID:             "stealth_avoid",
				Name:           "Тихо скрыться",
				Description:    "Незаметно уйти от встречи",
				RequiredAction: "скрыться незаметно",
				SuccessRate:    65,
				Consequences:   "Избегнете проблем, но упустите возможности",
				Reward:         "безопасное отступление",
			},
			{
				ID:             "prepare_combat",
				Name:           "Приготовиться к бою",
				Description:    "Показать готовность к сражению",
				RequiredAction: "приготовиться к бою",
				SuccessRate:    70,
				Consequences:   "Можете предотвратить бой или спровоцировать его",
				Reward:         "предотвращение боя или победа",
			},
		}
	}

	event := &world.WorldEvent{
		WorldID:            req.WorldID,
		Type:               worldEventType,
		Status:             world.WorldEventStatusActive,
		Name:               name,
		Description:        description,
		Metadata:           buildLocationEventMetadataWithBranches(description, branches, string(eventType), locationEventStatusPending),
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

// buildLocationEventMetadataWithBranches создает метаданные события с ветками развития
func buildLocationEventMetadataWithBranches(hook string, branches []world.LocationEventBranch, eventType string, status string) []byte {
	// Создаем совместимые options для обратной совместимости
	options := make([]string, len(branches))
	for i, branch := range branches {
		options[i] = branch.Name
	}

	meta := world.LocationEventMetadata{
		Hook:            hook,
		Options:         options,
		Status:          status,
		Branches:        branches,
		EventType:       eventType,
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
