package worldmap

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"dungeons-and-dragons-ai/internal/game/domain/event"
	"dungeons-and-dragons-ai/internal/game/domain/inventory"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/game/domain/world"
	"dungeons-and-dragons-ai/internal/llm/domain"
	ragdomain "dungeons-and-dragons-ai/internal/rag/domain"

	"github.com/google/uuid"
)

type MoveToLocationUseCase struct {
	sessionRepo    session.Repository
	llm            domain.LLM // Добавлено для генерации сценариев
	worldEventRepo WorldEventRepository
	eventRepo      StoryEventRepository
	indexDocUC     RAGIndexer
	inventoryRepo  InventoryRepository
}

type WorldEventRepository interface {
	GetByLocationID(ctx context.Context, locationID uint) ([]world.WorldEvent, error)
	Save(ctx context.Context, e *world.WorldEvent) error
}

type StoryEventRepository interface {
	Save(ctx context.Context, e *event.StoryEvent) error
}

type RAGIndexer interface {
	Execute(ctx context.Context, doc ragdomain.Document) error
}

// InventoryRepository — доступ к инвентарю персонажа, нужен для гейта по квестовым
// предметам (LocationConnection.RequiredItemName). Если не задан (nil), гейт отключается —
// перемещение работает как раньше, без проверки предметов.
type InventoryRepository interface {
	GetByCharacterID(ctx context.Context, characterID uint) (*inventory.Inventory, error)
}

func NewMoveToLocationUseCase(
	llm domain.LLM,
	sessionRepo session.Repository,
	worldEventRepo WorldEventRepository,
	eventRepo StoryEventRepository,
	indexDocUC RAGIndexer,
	inventoryRepo InventoryRepository,
) *MoveToLocationUseCase {
	return &MoveToLocationUseCase{
		llm:            llm,
		sessionRepo:    sessionRepo,
		worldEventRepo: worldEventRepo,
		eventRepo:      eventRepo,
		indexDocUC:     indexDocUC,
		inventoryRepo:  inventoryRepo,
	}
}

type MoveToLocationRequest struct {
	ChatID int64

	// Один из вариантов должен быть задан:
	ToLocationID *uint
	Direction    string
}

type MoveToLocationResponse struct {
	From *world.Location
	To   *world.Location

	Message string
}

func (uc *MoveToLocationUseCase) Execute(ctx context.Context, req MoveToLocationRequest) (*MoveToLocationResponse, error) {
	gs, err := uc.sessionRepo.GetByChatID(ctx, req.ChatID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}
	if gs == nil {
		return nil, fmt.Errorf("game session not found")
	}
	if !gs.IsActive() {
		return nil, fmt.Errorf("game session is not active")
	}
	if len(gs.World.Locations) == 0 {
		return nil, fmt.Errorf("world has no locations")
	}

	locationMap := make(map[uint]*world.Location, len(gs.World.Locations))
	for i := range gs.World.Locations {
		loc := &gs.World.Locations[i]
		locationMap[loc.ID] = loc
	}

	// Определяем текущую локацию (если не задана — инициализируем первой)
	var current *world.Location
	if gs.CurrentLocationID != nil {
		current = locationMap[*gs.CurrentLocationID]
	}
	if current == nil {
		firstID := gs.World.Locations[0].ID
		gs.CurrentLocationID = &firstID
		current = locationMap[firstID]
		_ = uc.sessionRepo.Save(ctx, gs)
	}
	if current != nil {
		current.Visited = true
	}

	var targetID uint
	if req.ToLocationID != nil && *req.ToLocationID != 0 {
		targetID = *req.ToLocationID
	} else if strings.TrimSpace(req.Direction) != "" {
		dir := normalizeDirection(req.Direction)
		for _, conn := range current.Connections {
			if normalizeDirection(conn.Direction) == dir {
				targetID = conn.ToLocationID
				break
			}
		}
	} else {
		return nil, fmt.Errorf("either ToLocationID or Direction must be provided")
	}

	to := locationMap[targetID]
	if to == nil {
		return nil, fmt.Errorf("target location not found")
	}

	// Проверяем достижимость (есть ли связь из текущей локации) и находим соединение
	var travelConnection *world.LocationConnection
	reachable := false
	for _, conn := range current.Connections {
		if conn.ToLocationID == targetID {
			reachable = true
			travelConnection = &conn
			break
		}
	}
	if !reachable {
		return nil, fmt.Errorf("target location is not reachable from current location")
	}

	// Гейт по квестовому предмету: если связь требует конкретный предмет, у партии
	// должен быть хотя бы один персонаж, у которого этот предмет есть в инвентаре.
	if travelConnection.RequiredItemName != "" && !uc.partyHasItem(ctx, gs, travelConnection.RequiredItemName) {
		return nil, fmt.Errorf("путь в «%s» заблокирован: нужен предмет %q", to.Name, travelConnection.RequiredItemName)
	}

	// Рассчитываем время перемещения и продвигаем время в мире
	travelHours := gs.World.CalculateTravelTimeHours(*travelConnection)
	if travelHours > 0 {
		oldTimeOfDay := gs.World.TimeOfDay
		oldDay := gs.World.Day

		// Продвигаем время
		gs.World.AdvanceTimeByHours(travelHours)

		// Проверяем, изменилась ли погода
		weatherChanged := gs.World.MaybeChangeWeather()
		newTimeOfDay := gs.World.TimeOfDay
		newDay := gs.World.Day

		// Сохраняем изменения мира
		if err := uc.sessionRepo.Save(ctx, gs); err != nil {
			return nil, fmt.Errorf("failed to save world time changes: %w", err)
		}

		// Создаем сообщение о времени
		timeMsg := uc.buildTimeProgressionMessage(oldTimeOfDay, newTimeOfDay, oldDay, newDay, travelHours, weatherChanged, gs.World.Weather)
		_ = timeMsg // Пока сохраняем для использования в сообщении
	}

	// Фиксируем активные события локации как "игнор" при уходе
	if err := uc.resolveLocationEventsOnLeave(ctx, gs, current); err != nil {
		return nil, err
	}

	// Генерируем сценарий для новой локации при первом посещении
	if err := uc.generateScenarioForLocationIfNeeded(ctx, to); err != nil {
		// Логируем ошибку, но не прерываем перемещение
		// TODO: добавить логирование когда будет доступен logger
	}

	// Обновляем текущую локацию
	gs.ClearPendingAbilityCheck()
	gs.ClearSceneAbilityCheckHistory()
	to.Visited = true
	gs.CurrentLocationID = &targetID
	if err := uc.sessionRepo.Save(ctx, gs); err != nil {
		return nil, fmt.Errorf("failed to save session: %w", err)
	}

	// Создаем сообщение о перемещении
	msg := uc.buildMovementMessage(current, to, travelHours, &gs.World)
	msg += uc.maybeGenerateRandomEncounter(ctx, gs, current, to, travelHours)
	return &MoveToLocationResponse{
		From:    current,
		To:      to,
		Message: msg,
	}, nil
}

// partyHasItem проверяет, есть ли у хотя бы одного персонажа партии предмет с именем
// itemName (сравнение case-insensitive, по подстроке — как в validate_item_usage).
// Если inventoryRepo не задан, гейт считается отключённым (true), чтобы не ломать
// существующие вызовы/тесты, где инвентарь не подключён.
func (uc *MoveToLocationUseCase) partyHasItem(ctx context.Context, gs *session.GameSession, itemName string) bool {
	if uc.inventoryRepo == nil {
		return true
	}
	needle := strings.ToLower(strings.TrimSpace(itemName))
	if needle == "" {
		return true
	}
	for i := range gs.Players {
		charID := gs.Players[i].Character.ID
		if charID == 0 {
			continue
		}
		inv, err := uc.inventoryRepo.GetByCharacterID(ctx, charID)
		if err != nil || inv == nil {
			continue
		}
		for _, it := range inv.Items {
			if strings.Contains(strings.ToLower(it.Name), needle) {
				return true
			}
		}
	}
	return false
}

func (uc *MoveToLocationUseCase) resolveLocationEventsOnLeave(
	ctx context.Context,
	gs *session.GameSession,
	current *world.Location,
) error {
	if uc.worldEventRepo == nil || current == nil {
		return nil
	}

	events, err := uc.worldEventRepo.GetByLocationID(ctx, current.ID)
	if err != nil {
		return fmt.Errorf("failed to get location events: %w", err)
	}

	for i := range events {
		ev := events[i]
		if !isLocationEventType(ev.Type) || ev.Status != world.WorldEventStatusActive {
			continue
		}

		ev.Cancel()
		now := time.Now()
		ev.CompletedAt = &now
		ev.UpdatedAt = now
		ev.Metadata = updateLocationEventMetadataStatus(ev.Metadata, "ignored")

		if err := uc.worldEventRepo.Save(ctx, &ev); err != nil {
			return fmt.Errorf("failed to save location event: %w", err)
		}

		content := buildLocationEventOutcomeStory(&ev, "Событие локации проигнорировано — игрок покинул локацию.")
		leftLocationID := current.ID
		if uc.eventRepo != nil {
			eventItem := &event.StoryEvent{
				GameSessionID: gs.ID,
				LocationID:    &leftLocationID,
				AuthorType:    event.AuthorTypeDM,
				Content:       content,
				CreatedAt:     time.Now(),
			}
			if err := uc.eventRepo.Save(ctx, eventItem); err != nil {
				return fmt.Errorf("failed to save location outcome story event: %w", err)
			}
		}

		if uc.indexDocUC != nil {
			doc := ragdomain.Document{
				ID:         uuid.New().String(),
				Source:     ragdomain.SourceEvent,
				SessionID:  gs.ID,
				LocationID: &leftLocationID,
				Text:       content,
				Timestamp:  time.Now(),
			}
			if err := uc.indexDocUC.Execute(ctx, doc); err != nil {
				return fmt.Errorf("failed to index location outcome in RAG: %w", err)
			}
		}
	}

	return nil
}

func isLocationEventType(t world.WorldEventType) bool {
	switch t {
	case world.WorldEventTypeLocationNPC,
		world.WorldEventTypeLocationItem,
		world.WorldEventTypeLocationTrap,
		world.WorldEventTypeLocationPuzzle,
		world.WorldEventTypeLocationEncounter,
		world.WorldEventTypeRandomEncounter:
		return true
	default:
		return false
	}
}

func updateLocationEventMetadataStatus(meta []byte, status string) []byte {
	if len(meta) == 0 {
		return meta
	}
	var payload world.LocationEventMetadata
	if err := json.Unmarshal(meta, &payload); err != nil {
		return meta
	}
	payload.Status = status
	updated, err := json.Marshal(payload)
	if err != nil {
		return meta
	}
	return updated
}

func buildLocationEventOutcomeStory(ev *world.WorldEvent, outcome string) string {
	if ev == nil {
		return outcome
	}

	parts := []string{
		fmt.Sprintf("Событие локации: %s", ev.Name),
		outcome,
	}
	if ev.Description != "" {
		parts = append(parts, fmt.Sprintf("Описание: %s", ev.Description))
	}

	return strings.Join(parts, "\n")
}

func normalizeDirection(dir string) string {
	d := strings.ToLower(strings.TrimSpace(dir))
	switch d {
	case "n":
		return "north"
	case "s":
		return "south"
	case "e":
		return "east"
	case "w":
		return "west"
	case "u":
		return "up"
	case "d":
		return "down"
	}
	return d
}

// generateScenarioForLocationIfNeeded генерирует сценарий для локации при первом посещении
func (uc *MoveToLocationUseCase) generateScenarioForLocationIfNeeded(ctx context.Context, location *world.Location) error {
	// Проверяем, есть ли уже сценарий
	existingScenario := location.GetScenario()
	if existingScenario != nil {
		// Сценарий уже существует
		return nil
	}

	// Сначала пытаемся сгенерировать сценарий с помощью LLM
	scenario, err := uc.generateLLMLocationScenario(ctx, *location)
	if err != nil {
		// Если LLM недоступен или генерация не удалась, используем заранее подготовленный сценарий
		scenario = uc.generateSimpleLocationScenario(*location)
	}

	// Сохраняем сценарий в локации
	if err := location.SetScenario(scenario); err != nil {
		return fmt.Errorf("failed to set scenario: %w", err)
	}

	return nil
}

// generateLLMLocationScenario генерирует мини-сценарий для локации с помощью LLM
func (uc *MoveToLocationUseCase) generateLLMLocationScenario(ctx context.Context, location world.Location) (*world.LocationScenario, error) {
	if uc.llm == nil {
		return nil, fmt.Errorf("LLM not available")
	}

	// Создаем мини-промпт для генерации сценария
	prompt := fmt.Sprintf(`Создай краткий сценарий для локации в D&D 5e.

Локация: %s
Описание: %s

Требования к сценарию:
- Краткое название (2-4 слова)
- Краткое описание ситуации (1-2 предложения)
- Конкретная цель для игрока (что нужно сделать)
- 2-3 ключевых события
- Возможные исходы (успех/неудача)
- Небольшая награда

Формат ответа (ТОЛЬКО JSON, без markdown):
{
  "title": "Название сценария",
  "description": "Краткое описание",
  "objective": "Цель игрока",
  "key_events": ["Событие 1", "Событие 2"],
  "possible_outcomes": ["Исход 1", "Исход 2"],
  "reward": "Награда"
}`, location.Name, location.Description)

	llmCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	raw, err := uc.llm.GenerateWithMaxTokens(llmCtx, prompt, 800)
	if err != nil {
		log.Printf("[MoveToLocation] Failed to generate LLM scenario for %s: %v", location.Name, err)
		return nil, err
	}

	// Парсим JSON ответ
	var scenario world.LocationScenario
	cleaned := strings.TrimSpace(strings.Trim(raw, "```json"))
	if err := json.Unmarshal([]byte(cleaned), &scenario); err != nil {
		log.Printf("[MoveToLocation] Failed to parse LLM scenario JSON for %s: %v", location.Name, err)
		return nil, err
	}

	// Заполняем обязательные поля
	scenario.ID = fmt.Sprintf("llm_scenario_%d_%d", location.ID, time.Now().Unix())
	scenario.Status = "not_started"
	scenario.CreatedAt = time.Now().Format(time.RFC3339)

	log.Printf("[MoveToLocation] Generated LLM scenario for location %s: %s", location.Name, scenario.Title)
	return &scenario, nil
}

// generateSimpleLocationScenario создает простой сценарий для локации
func (uc *MoveToLocationUseCase) generateSimpleLocationScenario(location world.Location) *world.LocationScenario {
	name := location.Name
	desc := location.Description

	// Определяем тип сценария на основе названия и описания
	var title, description, objective, reward string
	var keyEvents, possibleOutcomes []string

	nameLower := strings.ToLower(name)
	descLower := strings.ToLower(desc)

	if strings.Contains(nameLower, "замок") || strings.Contains(nameLower, "крепост") || strings.Contains(descLower, "замок") {
		title = "Тайны древнего замка"
		description = "В этом древнем замке скрыты секреты давно ушедшей эпохи. Стены хранят память о славных победах и трагических поражениях."
		objective = "Раскрыть главный секрет замка и найти скрытый артефакт"
		keyEvents = []string{
			"Исследовать главный зал и найти древние записи",
			"Встретить стража замка и пройти испытание",
			"Решить загадку потайной комнаты",
			"Противостоять защитному голему",
		}
		possibleOutcomes = []string{
			"Получить древний артефакт и благословение замка",
			"Быть изгнанным стражами и потерять время",
			"Найти лишь часть сокровищ замка",
		}
		reward = "Древний артефакт с магическими свойствами и золотые монеты"
	} else if strings.Contains(nameLower, "пещер") || strings.Contains(nameLower, "гробниц") || strings.Contains(nameLower, "подзем") {
		title = "Сокровища подземного мира"
		description = "В глубинах этой локации хранятся богатства и опасности подземного мира. Темные коридоры полны тайн и опасностей."
		objective = "Найти и добыть ценный артефакт, охраняемый подземными стражами"
		keyEvents = []string{
			"Преодолеть ловушки у входа",
			"Исследовать основные коридоры и найти подсказки",
			"Решить механизм защиты сокровищницы",
			"Противостоять стражу подземелья",
		}
		possibleOutcomes = []string{
			"Получить ценный артефакт и богатства подземелья",
			"Погибнуть в ловушках или от стражей",
			"Вернуться с пустыми руками после тяжелой борьбы",
		}
		reward = "Ценный артефакт и коллекция драгоценных камней"
	} else if strings.Contains(nameLower, "лес") || strings.Contains(descLower, "лес") {
		title = "Тайны древнего леса"
		description = "Этот лес полон жизни и магии природы. Деревья здесь помнят древние времена, а тропы ведут к священным местам."
		objective = "Найти и защитить священное место леса от угрозы"
		keyEvents = []string{
			"Найти следы древних ритуалов и священных символов",
			"Встретить хранителя леса и получить его благословение",
			"Решить природную загадку древнего дуба",
			"Защитить священное место от вторгшихся существ",
		}
		possibleOutcomes = []string{
			"Получить благословение природы и магические растения",
			"Быть изгнанным духами леса",
			"Найти лишь частичные знания о тайнах леса",
		}
		reward = "Благословение природы и редкие магические ингредиенты"
	} else if strings.Contains(nameLower, "город") || strings.Contains(nameLower, "деревн") || strings.Contains(descLower, "город") {
		title = "Тайны населенного пункта"
		description = "В этом населенном пункте кипит жизнь, но под спокойной поверхностью скрываются интриги и секреты."
		objective = "Расследовать подозрительную активность и помочь жителям"
		keyEvents = []string{
			"Поговорить с местными жителями и собрать информацию",
			"Найти следы подозрительной активности",
			"Исследовать заброшенные районы",
			"Противостоять источнику проблем",
		}
		possibleOutcomes = []string{
			"Разрешить проблемы города и получить награду от жителей",
			"Не справиться с задачей и быть вынужденным уйти",
			"Найти частичные решения и помочь некоторым жителям",
		}
		reward = "Благодарность жителей и полезные предметы"
	} else {
		// Общий сценарий для остальных локаций
		title = fmt.Sprintf("Тайна локации '%s'", name)
		description = fmt.Sprintf("Эта локация хранит свои секреты и ждет достойного исследователя. %s", desc)
		objective = "Исследовать локацию и раскрыть её главную тайну"
		keyEvents = []string{
			"Осмотреть окрестности и найти ключевые объекты",
			"Найти подсказки о предназначении локации",
			"Встретить стражей или обитателей локации",
			"Решить основную загадку или проблему",
		}
		possibleOutcomes = []string{
			"Раскрыть тайну и получить награду",
			"Не справиться с вызовами локации",
			"Найти частичные ответы и подсказки",
		}
		reward = "Ценный предмет или важная информация"
	}

	return &world.LocationScenario{
		ID:               fmt.Sprintf("scenario_%d_%d", location.ID, time.Now().Unix()),
		Title:            title,
		Description:      description,
		Objective:        objective,
		KeyEvents:        keyEvents,
		PossibleOutcomes: possibleOutcomes,
		Reward:           reward,
		Status:           "not_started",
		CreatedAt:        time.Now().Format(time.RFC3339),
	}
}

// buildTimeProgressionMessage создает сообщение о прошедшем времени
func (uc *MoveToLocationUseCase) buildTimeProgressionMessage(
	oldTimeOfDay, newTimeOfDay string,
	oldDay, newDay int,
	travelHours int,
	weatherChanged bool,
	newWeather string,
) string {
	var parts []string

	if travelHours == 1 {
		parts = append(parts, fmt.Sprintf("⏰ В пути вы провели %d час", travelHours))
	} else if travelHours < 5 {
		parts = append(parts, fmt.Sprintf("⏰ В пути вы провели %d часа", travelHours))
	} else {
		parts = append(parts, fmt.Sprintf("⏰ В пути вы провели %d часов", travelHours))
	}

	// Информация об изменении времени суток
	if oldTimeOfDay != newTimeOfDay {
		timeNames := map[string]string{
			"morning":   "утро",
			"noon":      "полдень",
			"afternoon": "день",
			"evening":   "вечер",
			"night":     "ночь",
			"midnight":  "полночь",
		}

		if oldName, ok := timeNames[oldTimeOfDay]; ok {
			if newName, ok := timeNames[newTimeOfDay]; ok {
				if oldDay != newDay {
					parts = append(parts, fmt.Sprintf("🌅 Наступил новый день. Время суток изменилось: %s → %s", oldName, newName))
				} else {
					parts = append(parts, fmt.Sprintf("🕐 Время суток изменилось: %s → %s", oldName, newName))
				}
			}
		}
	}

	// Информация об изменении погоды
	if weatherChanged {
		weatherNames := map[string]string{
			"clear":  "ясная",
			"cloudy": "облачная",
			"rainy":  "дождливая",
			"stormy": "штормовая",
			"foggy":  "туманная",
			"snowy":  "снежная",
		}

		if weatherName, ok := weatherNames[newWeather]; ok {
			parts = append(parts, fmt.Sprintf("🌤️ Погода изменилась на %s", weatherName))
		}
	}

	return strings.Join(parts, ". ") + "."
}

// buildMovementMessage создает сообщение о перемещении с информацией о времени
func (uc *MoveToLocationUseCase) buildMovementMessage(
	from, to *world.Location,
	travelHours int,
	currentWorld *world.World,
) string {
	var parts []string

	// Основное сообщение о перемещении
	parts = append(parts, fmt.Sprintf("🧭 Вы переместились: %s → %s", from.Name, to.Name))

	// Информация о времени в пути
	if travelHours > 0 {
		if travelHours == 1 {
			parts = append(parts, fmt.Sprintf("⏱️ Время в пути: %d час", travelHours))
		} else if travelHours < 5 {
			parts = append(parts, fmt.Sprintf("⏱️ Время в пути: %d часа", travelHours))
		} else {
			parts = append(parts, fmt.Sprintf("⏱️ Время в пути: %d часов", travelHours))
		}
	}

	// Текущее время и погода
	timeNames := map[string]string{
		"morning":   "🌅 Утро",
		"noon":      "☀️ Полдень",
		"afternoon": "🌇 День",
		"evening":   "🌆 Вечер",
		"night":     "🌙 Ночь",
		"midnight":  "🕛 Полночь",
	}

	if timeName, ok := timeNames[currentWorld.TimeOfDay]; ok {
		parts = append(parts, fmt.Sprintf("🕐 Текущее время: %s", timeName))
	}

	weatherNames := map[string]string{
		"clear":  "ясно",
		"cloudy": "облачно",
		"rainy":  "дождь",
		"stormy": "шторм",
		"foggy":  "туман",
		"snowy":  "снег",
	}

	if weatherName, ok := weatherNames[currentWorld.Weather]; ok {
		parts = append(parts, fmt.Sprintf("🌤️ Погода: %s", weatherName))
	}

	// Описание новой локации
	parts = append(parts, "")
	parts = append(parts, to.Description)

	return strings.Join(parts, "\n")
}
