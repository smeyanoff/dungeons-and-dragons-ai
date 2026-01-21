package worldmap

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/game/domain/world"
)

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

	// Короткий блок “где ты сейчас” + доступные выходы
	parts = append(parts, fmt.Sprintf("📌 Сейчас вы здесь: %s", currentLoc.Name))
	if len(currentLoc.Connections) > 0 {
		var exits []string
		for _, conn := range currentLoc.Connections {
			sym := uc.getDirectionSymbol(conn.Direction)
			exits = append(exits, fmt.Sprintf("%s %s", sym, strings.ToLower(conn.Direction)))
		}
		parts = append(parts, "Выходы: "+strings.Join(exits, ", "))
	} else {
		parts = append(parts, "Выходы: нет известных путей.")
	}
	parts = append(parts, "")

	// Формируем список локаций с их связями
	for i := range w.Locations {
		loc := &w.Locations[i]
		isCurrent := currentLoc != nil && loc.ID == currentLoc.ID
		prefix := "📍"
		if isCurrent {
			prefix = "📍▶️"
		}
		parts = append(parts, fmt.Sprintf("%s %s", prefix, loc.Name))

		if loc.Description != "" {
			// Ограничиваем длину описания для компактности
			desc := loc.Description
			if len(desc) > 100 {
				desc = desc[:97] + "..."
			}
			parts = append(parts, fmt.Sprintf("   %s", desc))
		}

		// Добавляем информацию о связях
		if len(loc.Connections) > 0 {
			var connectionLines []string
			for _, conn := range loc.Connections {
				var toLocationName string
				if toLoc, exists := locationMap[conn.ToLocationID]; exists {
					toLocationName = toLoc.Name
				} else {
					toLocationName = fmt.Sprintf("Локация #%d", conn.ToLocationID)
				}

				// Формируем направление
				directionSymbol := uc.getDirectionSymbol(conn.Direction)
				connectionInfo := fmt.Sprintf("%s %s", directionSymbol, toLocationName)

				if conn.Description != "" {
					desc := conn.Description
					if len(desc) > 50 {
						desc = desc[:47] + "..."
					}
					connectionInfo += fmt.Sprintf(" (%s)", desc)
				}

				connectionLines = append(connectionLines, "   └─ "+connectionInfo)
			}
			parts = append(parts, strings.Join(connectionLines, "\n"))
		}

		// Добавляем информацию о NPC и монстрах
		if len(loc.NPCs) > 0 || len(loc.Monsters) > 0 {
			var entities []string
			for _, npc := range loc.NPCs {
				entities = append(entities, fmt.Sprintf("👤 %s (%s)", npc.Name, npc.Role))
			}
			for _, monster := range loc.Monsters {
				entities = append(entities, fmt.Sprintf("👹 %s", monster.Name))
			}
			if len(entities) > 0 {
				parts = append(parts, "   Обнаружено: "+strings.Join(entities, ", "))
			}
		}

		parts = append(parts, "")
	}

	// Добавляем информацию о времени суток и погоде
	if w.TimeOfDay != "" || w.Weather != "" {
		parts = append(parts, "---")
		if w.TimeOfDay != "" {
			parts = append(parts, fmt.Sprintf("🕐 Время суток: %s", uc.translateTimeOfDay(w.TimeOfDay)))
		}
		if w.Weather != "" {
			parts = append(parts, fmt.Sprintf("🌤️ Погода: %s", uc.translateWeather(w.Weather)))
		}
	}

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
	for _, loc := range w.Locations {
		totalConnections += len(loc.Connections)
		totalNPCs += len(loc.NPCs)
		totalMonsters += len(loc.Monsters)
	}
	currentID := uint(0)
	if currentLocationID != nil {
		currentID = *currentLocationID
	}
	return fmt.Sprintf("%d:%d:%d:%d:%d:%s:%s:%d:%d:%d",
		w.ID,
		currentID,
		len(w.Locations),
		totalConnections,
		totalNPCs+totalMonsters,
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
