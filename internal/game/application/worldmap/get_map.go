package worldmap

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/game/domain/world"
)

// truncateRunes безопасно усекает строку до указанного количества рун с добавлением суффикса
func truncateRunes(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	if maxRunes <= 0 || s == "" {
		return ""
	}

	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}

	// Берем первые maxRunes рун и добавляем суффикс
	return string(runes[:maxRunes]) + "..."
}

type GetMapUseCase struct {
	sessionRepo session.Repository
	cacheMu     sync.RWMutex
	cache       map[uint]cachedMap
}

type cachedMap struct {
	signature string
	text      string
}

func NewGetMapUseCase(sessionRepo session.Repository) *GetMapUseCase {
	return &GetMapUseCase{
		sessionRepo: sessionRepo,
		cache:       make(map[uint]cachedMap),
	}
}

func (uc *GetMapUseCase) Execute(
	ctx context.Context,
	chatID int64,
) (string, error) {
	// Получаем сессию
	gs, err := uc.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("failed to get session: %w", err)
	}

	if gs == nil {
		return "Игра не начата. Используйте /newgame для начала новой игры.", nil
	}

	signature := uc.buildMapSignature(&gs.World, gs.CurrentLocationID)
	if cached, ok := uc.getCachedMap(gs.World.ID, signature); ok {
		return cached, nil
	}

	// Формируем карту мира
	mapText := uc.generateMap(&gs.World, gs.CurrentLocationID)
	uc.setCachedMap(gs.World.ID, signature, mapText)
	return mapText, nil
}

// generateMap создает ASCII визуализацию карты мира на основе связей между локациями
func (uc *GetMapUseCase) generateMap(w *world.World, currentLocationID *uint) string {
	if len(w.Locations) == 0 {
		return "🗺️ Карта мира пуста. Локации еще не открыты."
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("🗺️ Карта мира: %s\n", w.Name))
	parts = append(parts, "")

	knownLocations := uc.buildKnownLocations(w, currentLocationID)

	// Создаем карту связей для быстрого доступа
	locationMap := make(map[uint]*world.Location)
	for i := range w.Locations {
		locationMap[w.Locations[i].ID] = &w.Locations[i]
	}

	// Определяем текущую локацию (если не задана — считаем первой локацией мира)
	var currentLoc *world.Location
	if currentLocationID != nil {
		currentLoc = locationMap[*currentLocationID]
	}
	if currentLoc == nil {
		currentLoc = &w.Locations[0]
	}
	// Текущая локация всегда "известна" игроку, даже если CurrentLocationID не установлен.
	if currentLoc != nil {
		knownLocations[currentLoc.ID] = true
	}

	// Легенда карты
	parts = append(parts, "📖 Легенда:")
	parts = append(parts, "📍▶️ - Ваше текущее положение")
	parts = append(parts, "✅ - Посещенная локация")
	parts = append(parts, "❓ - Неизвестная локация")
	parts = append(parts, "⬆️⬇️➡️⬅️ - Направления движения")
	parts = append(parts, "🌀 - Портал или магический переход")
	parts = append(parts, "🛤️ - Дорога или путь")
	parts = append(parts, "")

	// Детальная информация о текущей локации
	parts = append(parts, "🏠 ТЕКУЩАЯ ЛОКАЦИЯ")
	parts = append(parts, fmt.Sprintf("📌 Название: %s", currentLoc.Name))
	if currentLoc.Description != "" {
		// Ограничиваем длину описания для читаемости
		desc := truncateRunes(currentLoc.Description, 100)
		parts = append(parts, fmt.Sprintf("📝 Описание: %s", desc))
	}

	// NPCs и монстры текущей локации (короткая сводка)
	if len(currentLoc.NPCs) > 0 || len(currentLoc.Monsters) > 0 {
		parts = append(parts, "")
		parts = append(parts, "Обнаружено:")
		for _, npc := range currentLoc.NPCs {
			line := fmt.Sprintf("👤 %s", npc.Name)
			if strings.TrimSpace(npc.Role) != "" {
				line += fmt.Sprintf(" (%s)", npc.Role)
			}
			parts = append(parts, line)
		}
		for _, m := range currentLoc.Monsters {
			parts = append(parts, fmt.Sprintf("👹 %s", m.Name))
		}
	}

	parts = append(parts, "")

	// Доступные пути из текущей локации
	if len(currentLoc.Connections) > 0 {
		parts = append(parts, "🚪 ДОСТУПНЫЕ ПУТИ")
		for _, conn := range currentLoc.Connections {
			sym := uc.getDirectionSymbol(conn.Direction)
			toLocName := "Неизвестная локация"
			status := "неизвестна"
			marker := "❓ "

			if toLoc, exists := locationMap[conn.ToLocationID]; exists && toLoc != nil {
				// Название локации мы знаем из карты мира, но отмечаем маркером visited/unknown.
				toLocName = toLoc.Name
				if knownLocations[toLoc.ID] {
					status = "известна"
					marker = "✅ "
				}
			} else if conn.ToLocationID != 0 {
				// Если ссылка битая — все равно показываем её ID (нужно для диагностики/тестов).
				toLocName = fmt.Sprintf("Локация #%d", conn.ToLocationID)
			}

			pathDesc := fmt.Sprintf("   %s %s → %s%s (%s)", sym, strings.ToLower(conn.Direction), marker, toLocName, status)
			if conn.Description != "" {
				pathDesc += fmt.Sprintf("\n     └─ %s", conn.Description)
			}
			parts = append(parts, pathDesc)
		}
	} else {
		parts = append(parts, "🚪 ДОСТУПНЫЕ ПУТИ")
		parts = append(parts, "   Нет известных путей из этой локации.")
	}
	parts = append(parts, "")

	// Добавляем сводную информацию о мире
	parts = append(parts, "---")
	parts = append(parts, "📊 СВОДКА МИРА")

	// Прогресс исследования
	exploredLocations := len(knownLocations)

	parts = append(parts, fmt.Sprintf("🏛️ Всего локаций: %d", len(w.Locations)))
	exploredPercent := 0.0
	if len(w.Locations) > 0 {
		exploredPercent = float64(exploredLocations) / float64(len(w.Locations)) * 100
	}
	parts = append(parts, fmt.Sprintf("🔍 Исследовано: %d (%.1f%%)", exploredLocations, exploredPercent))
	parts = append(parts, fmt.Sprintf("🛤️ Путей: %d", totalConnectionsCount(w)))

	// Добавляем информацию о времени суток и погоде
	if w.TimeOfDay != "" {
		parts = append(parts, fmt.Sprintf("🕐 Время суток: %s", uc.translateTimeOfDay(w.TimeOfDay)))
	}
	if w.Weather != "" {
		parts = append(parts, fmt.Sprintf("🌤️ Погода: %s", uc.translateWeather(w.Weather)))
	}

	// Добавляем подсказки по навигации
	parts = append(parts, "")
	parts = append(parts, "💡 Советы по навигации:")
	parts = append(parts, "• Используйте кнопки под картой для быстрого перемещения")
	parts = append(parts, "• Исследуйте локации, чтобы открывать новые пути")
	parts = append(parts, "• Некоторые пути могут требовать специальных предметов или умений")

	return strings.Join(parts, "\n")
}

func (uc *GetMapUseCase) getCachedMap(worldID uint, signature string) (string, bool) {
	if worldID == 0 {
		return "", false
	}
	uc.cacheMu.RLock()
	defer uc.cacheMu.RUnlock()
	cached, ok := uc.cache[worldID]
	if !ok || cached.signature != signature {
		return "", false
	}
	return cached.text, true
}

func (uc *GetMapUseCase) setCachedMap(worldID uint, signature, text string) {
	if worldID == 0 || signature == "" || text == "" {
		return
	}
	uc.cacheMu.Lock()
	defer uc.cacheMu.Unlock()
	uc.cache[worldID] = cachedMap{signature: signature, text: text}
}

func (uc *GetMapUseCase) buildMapSignature(w *world.World, currentLocationID *uint) string {
	if w == nil {
		return ""
	}
	totalConnections := 0
	totalNPCs := 0
	totalMonsters := 0
	knownLocations := 0
	for _, loc := range w.Locations {
		totalConnections += len(loc.Connections)
		totalNPCs += len(loc.NPCs)
		totalMonsters += len(loc.Monsters)
		if loc.Visited || len(loc.ScenarioJSON) > 0 {
			knownLocations++
		}
	}
	currentID := uint(0)
	if currentLocationID != nil {
		currentID = *currentLocationID
	}
	return fmt.Sprintf("%d:%d:%d:%d:%d:%d:%s:%s:%d:%d:%d",
		w.ID,
		currentID,
		len(w.Locations),
		totalConnections,
		totalNPCs+totalMonsters,
		knownLocations,
		w.TimeOfDay,
		w.Weather,
		w.Day,
		w.Week,
		w.Month,
	)
}

// getDirectionSymbol возвращает символ для направления
func (uc *GetMapUseCase) getDirectionSymbol(direction string) string {
	switch strings.ToLower(direction) {
	case "north", "n":
		return "⬆️"
	case "south", "s":
		return "⬇️"
	case "east", "e":
		return "➡️"
	case "west", "w":
		return "⬅️"
	case "up", "u":
		return "⬆️⬆️"
	case "down", "d":
		return "⬇️⬇️"
	case "portal":
		return "🌀"
	case "path", "road":
		return "🛤️"
	default:
		return "→"
	}
}

// translateTimeOfDay переводит время суток на русский
func (uc *GetMapUseCase) translateTimeOfDay(timeOfDay string) string {
	translations := map[string]string{
		"morning":   "Утро",
		"noon":      "Полдень",
		"afternoon": "День",
		"evening":   "Вечер",
		"night":     "Ночь",
		"midnight":  "Полночь",
	}

	if translated, exists := translations[strings.ToLower(timeOfDay)]; exists {
		return translated
	}
	return timeOfDay
}

// translateWeather переводит погоду на русский
func (uc *GetMapUseCase) translateWeather(weather string) string {
	translations := map[string]string{
		"clear":  "Ясно",
		"cloudy": "Облачно",
		"rainy":  "Дождь",
		"stormy": "Гроза",
		"foggy":  "Туман",
		"snowy":  "Снег",
	}

	if translated, exists := translations[strings.ToLower(weather)]; exists {
		return translated
	}
	return weather
}

func (uc *GetMapUseCase) buildKnownLocations(w *world.World, currentLocationID *uint) map[uint]bool {
	known := make(map[uint]bool)
	if w == nil {
		return known
	}
	for i := range w.Locations {
		loc := &w.Locations[i]
		if loc.Visited || len(loc.ScenarioJSON) > 0 {
			known[loc.ID] = true
		}
	}
	if currentLocationID != nil && *currentLocationID != 0 {
		known[*currentLocationID] = true
	}
	return known
}

func totalConnectionsCount(w *world.World) int {
	if w == nil {
		return 0
	}
	total := 0
	for _, loc := range w.Locations {
		total += len(loc.Connections)
	}
	return total
}

// generateVisualMap создает визуальную ASCII-карту связей между локациями
func (uc *GetMapUseCase) generateVisualMap(w *world.World, currentLocationID *uint, knownLocations map[uint]bool) string {
	if len(w.Locations) <= 1 {
		return ""
	}

	// Создаем карту связей для быстрого доступа
	locationMap := make(map[uint]*world.Location)
	for i := range w.Locations {
		locationMap[w.Locations[i].ID] = &w.Locations[i]
	}

	// Определяем текущую локацию
	var currentLoc *world.Location
	if currentLocationID != nil {
		currentLoc = locationMap[*currentLocationID]
	}
	if currentLoc == nil {
		currentLoc = &w.Locations[0]
	}

	// Строим иерархическую структуру локаций
	levels := uc.buildLocationHierarchy(locationMap, currentLoc.ID)

	if len(levels) == 0 || len(levels) == 1 {
		return ""
	}

	var result []string
	result = append(result, "```")
	result = append(result, "Визуальная карта связей:")
	result = append(result, "")

	// Отрисовываем уровни с улучшенным форматированием
	maxLevelSize := 0
	for _, locations := range levels {
		if len(locations) > maxLevelSize {
			maxLevelSize = len(locations)
		}
	}

	for level, locations := range levels {
		if level > 0 {
			result = append(result, "")
		}

		var levelLine []string
		for _, locID := range locations {
			if loc, exists := locationMap[locID]; exists {
				var symbol string
				if currentLoc != nil && loc.ID == currentLoc.ID {
					symbol = "🏠" // Дом для текущей локации
				} else if knownLocations[loc.ID] {
					symbol = "📍" // Известная локация
				} else {
					symbol = "❓" // Неизвестная локация
				}

				// Сокращаем длинные названия
				name := "Неизв."
				if knownLocations[loc.ID] || (currentLoc != nil && loc.ID == currentLoc.ID) {
					name = truncateRunes(loc.Name, 12)
				}

				levelLine = append(levelLine, fmt.Sprintf("%s%s", symbol, name))
			}
		}

		// Выравниваем строку по центру
		lineStr := strings.Join(levelLine, " ── ")
		result = append(result, lineStr)

		// Добавляем связи для следующего уровня
		if level < len(levels)-1 {
			nextLevel := levels[level+1]
			var connectionParts []string

			for i, fromLocID := range locations {
				fromLoc := locationMap[fromLocID]
				hasConnection := false

				// Проверяем связи с следующим уровнем
				for _, conn := range fromLoc.Connections {
					for _, toLocID := range nextLevel {
						if conn.ToLocationID == toLocID {
							symbol := uc.getDirectionSymbol(conn.Direction)
							connectionParts = append(connectionParts, fmt.Sprintf(" %s ", symbol))
							hasConnection = true
							break
						}
					}
					if hasConnection {
						break
					}
				}

				if !hasConnection {
					connectionParts = append(connectionParts, "   ")
				}

				// Добавляем разделители между локациями
				if i < len(locations)-1 {
					connectionParts = append(connectionParts, "───")
				}
			}

			if len(connectionParts) > 0 {
				result = append(result, strings.Join(connectionParts, ""))
			}
		}
	}

	result = append(result, "```")
	return strings.Join(result, "\n")
}

// buildLocationHierarchy строит иерархическую структуру локаций для визуализации
func (uc *GetMapUseCase) buildLocationHierarchy(locationMap map[uint]*world.Location, startLocationID uint) [][]uint {
	if len(locationMap) == 0 {
		return nil
	}

	// BFS для построения уровней
	visited := make(map[uint]bool)
	var levels [][]uint

	// Первый уровень - стартовая локация
	currentLevel := []uint{startLocationID}
	visited[startLocationID] = true

	for len(currentLevel) > 0 {
		levels = append(levels, currentLevel)

		var nextLevel []uint
		for _, locID := range currentLevel {
			if loc, exists := locationMap[locID]; exists {
				for _, conn := range loc.Connections {
					if !visited[conn.ToLocationID] {
						visited[conn.ToLocationID] = true
						nextLevel = append(nextLevel, conn.ToLocationID)
					}
				}
			}
		}

		// Удаляем дубликаты из nextLevel
		seen := make(map[uint]bool)
		var unique []uint
		for _, id := range nextLevel {
			if !seen[id] {
				seen[id] = true
				unique = append(unique, id)
			}
		}

		currentLevel = unique

		// Ограничиваем глубину для читаемости
		if len(levels) >= 5 {
			break
		}
	}

	return levels
}

// analyzeWorldRegions анализирует связи между локациями и группирует их в регионы
func (uc *GetMapUseCase) analyzeWorldRegions(locationMap map[uint]*world.Location) map[string][]uint {
	regions := make(map[string][]uint)
	visited := make(map[uint]bool)
	regionCounter := 1

	for _, loc := range locationMap {
		if !visited[loc.ID] {
			// Начинаем новый регион
			regionName := fmt.Sprintf("Регион %d", regionCounter)
			regionLocations := uc.findConnectedLocations(loc.ID, locationMap, visited)

			if len(regionLocations) > 0 {
				regions[regionName] = regionLocations
				regionCounter++
			}
		}
	}

	// Если только один регион, называем его основным
	if len(regions) == 1 {
		for name := range regions {
			regions["Основной регион"] = regions[name]
			delete(regions, name)
			break
		}
	}

	return regions
}

// findConnectedLocations находит все локации, связанные с данной (DFS)
func (uc *GetMapUseCase) findConnectedLocations(startID uint, locationMap map[uint]*world.Location, visited map[uint]bool) []uint {
	var locations []uint
	stack := []uint{startID}

	for len(stack) > 0 {
		currentID := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if visited[currentID] {
			continue
		}

		visited[currentID] = true
		locations = append(locations, currentID)

		// Добавляем связанные локации в стек
		if loc, exists := locationMap[currentID]; exists {
			for _, conn := range loc.Connections {
				if !visited[conn.ToLocationID] {
					stack = append(stack, conn.ToLocationID)
				}
			}
		}
	}

	return locations
}

// findReachableLocations находит все локации, доступные из заданной
func (uc *GetMapUseCase) findReachableLocations(startID uint, locationMap map[uint]*world.Location) []uint {
	visited := make(map[uint]bool)
	var reachable []uint
	queue := []uint{startID}

	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]

		if visited[currentID] {
			continue
		}

		visited[currentID] = true
		reachable = append(reachable, currentID)

		// Добавляем связанные локации в очередь
		if loc, exists := locationMap[currentID]; exists {
			for _, conn := range loc.Connections {
				if !visited[conn.ToLocationID] {
					queue = append(queue, conn.ToLocationID)
				}
			}
		}
	}

	return reachable
}
