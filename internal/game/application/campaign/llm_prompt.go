package campaign

import (
	"fmt"
	"strings"
)

// GenerateMainQuestPrompt создает промпт для генерации главного квеста
func GenerateMainQuestPrompt(worldTheme string) string {
	return fmt.Sprintf(`Ты — Dungeon Master в D&D. Создай главный квест для кампании.

Тематика мира: %s

ВАЖНО: Ответь ТОЛЬКО валидным JSON, без дополнительного текста, без markdown блоков кода.

Формат ответа:

{
  "title": "название квеста",
  "description": "подробное описание квеста (2-3 предложения)",
  "items": [
    { "name": "название предмета", "purpose": "назначение предмета" }
  ]
}

Создай 2-3 предмета, связанных с квестом.`, worldTheme)
}

// GenerateMainQuestPromptStrict создает более строгий промпт для retry
func GenerateMainQuestPromptStrict(worldTheme string) string {
	return fmt.Sprintf(`Ты — Dungeon Master в D&D. ВАЖНО: Ответь ТОЛЬКО валидным JSON, без дополнительного текста, без markdown блоков.

Создай главный квест для кампании.

Тематика мира: %s

Ответ ТОЛЬКО в JSON следующего формата (без markdown, без комментариев, без дополнительного текста):

{
  "title": "название квеста",
  "description": "подробное описание квеста (2-3 предложения)",
  "items": [
    { "name": "название предмета", "purpose": "назначение предмета" }
  ]
}

Создай 2-3 предмета, связанных с квестом.`, worldTheme)
}

// GenerateLocationsPrompt создает промпт для генерации списка локаций
func GenerateLocationsPrompt(worldTheme, mainQuestTitle string) string {
	return fmt.Sprintf(`Ты — Dungeon Master в D&D. Создай список ключевых локаций для кампании.

Тематика мира: %s
Главный квест: %s

Создай 3-5 локаций, которые важны для главного квеста.

ВАЖНО: 
- Описание каждой локации должно быть КРАТКИМ (максимум 1 предложение, до 100 символов)
- Ответь ТОЛЬКО валидным JSON без markdown блоков
- Убедись, что JSON полностью закрыт

Формат ответа:

{
  "locations": [
    {
      "name": "название локации",
      "description": "краткое описание (до 100 символов)"
    }
  ]
}`, worldTheme, mainQuestTitle)
}

// GenerateLocationsPromptStrict создает более строгий промпт для retry
func GenerateLocationsPromptStrict(worldTheme, mainQuestTitle string) string {
	return fmt.Sprintf(`Ты — Dungeon Master в D&D. ВАЖНО: Ответь ТОЛЬКО валидным JSON, без дополнительного текста, без markdown блоков.

Создай список ключевых локаций для кампании.

Тематика мира: %s
Главный квест: %s

Создай РОВНО 3 локации. Описание каждой локации - МАКСИМУМ 80 символов (одно короткое предложение).

Ответ ТОЛЬКО в JSON следующего формата (без markdown, без комментариев):

{
  "locations": [
    {
      "name": "название",
      "description": "короткое описание"
    }
  ]
}`, worldTheme, mainQuestTitle)
}

// GenerateLocationPredefinedChecksPrompt создает промпт для генерации предопределенных проверок для локации
func GenerateLocationPredefinedChecksPrompt(locationName, locationDescription string) string {
	return fmt.Sprintf(`Ты — Dungeon Master в D&D. Создай предопределенные проверки навыков для локации.

Локация: %s
Описание: %s

Создай 1-3 предопределенные проверки (проверки характеристик, которые игрок может выполнить в этой локации).

ВАЖНО:
- Ответь ТОЛЬКО валидным JSON без markdown блоков
- Убедись, что JSON полностью закрыт

Формат ответа:

{
  "predefined_checks": [
    {
      "ability": "wisdom|dexterity|strength|constitution|intelligence|charisma",
      "dc": 12,
      "description": "Проверка Восприятия (DC 12) - заметить скрытую дверь",
      "location_hint": "на стене в восточной части зала"
    }
  ]
}

Примеры предопределенных проверок:
- Проверка Восприятия (Wisdom) DC 12 - заметить скрытую дверь
- Проверка Ловкости (Dexterity) DC 15 - пройти по узкому мосту
- Проверка Силы (Strength) DC 14 - поднять камень, блокирующий проход
- Проверка Интеллекта (Intelligence) DC 13 - разгадать древнюю загадку на стене`, locationName, locationDescription)
}

// GenerateLocationPredefinedChecksPromptStrict создает более строгий промпт для retry
func GenerateLocationPredefinedChecksPromptStrict(locationName, locationDescription string) string {
	return fmt.Sprintf(`Ты — Dungeon Master в D&D. ВАЖНО: Ответь ТОЛЬКО валидным JSON без markdown и комментариев.

Создай 1-2 предопределенные проверки навыков для локации.
Локация: %s
Описание: %s

Ответ ТОЛЬКО в JSON:
{
  "predefined_checks": [
    {
      "ability": "wisdom|dexterity|strength|constitution|intelligence|charisma",
      "dc": 12,
      "description": "короткое описание проверки",
      "location_hint": "конкретное место в локации"
    }
  ]
}

Требования:
- ability и dc обязательны
- dc в диапазоне 8-20
- description короткая (до 120 символов)
- location_hint обязателен`, locationName, locationDescription)
}

// GenerateLocationNPCsPrompt создает промпт для генерации NPC локации
func GenerateLocationNPCsPrompt(locationName, locationDescription string) string {
	return fmt.Sprintf(`Ты — Dungeon Master в D&D. Создай NPC для локации.

Локация: %s
Описание: %s

Создай 1-3 NPC для этой локации. Каждый NPC должен иметь уникальную роль.

Ответь ТОЛЬКО валидным JSON без markdown блоков:

{
  "npcs": [
    { "name": "имя NPC", "role": "роль NPC (например: торговец, страж, маг)" }
  ]
}`, locationName, locationDescription)
}

// GenerateLocationNPCsPromptStrict создает более строгий промпт для retry
func GenerateLocationNPCsPromptStrict(locationName, locationDescription string) string {
	return fmt.Sprintf(`Ты — Dungeon Master в D&D. ВАЖНО: Ответь ТОЛЬКО валидным JSON без markdown и комментариев.

Создай 1-3 NPC для локации.
Локация: %s
Описание: %s

Ответ ТОЛЬКО в JSON:
{
  "npcs": [
    { "name": "имя NPC", "role": "роль NPC" }
  ]
}

Требования:
- Имя и роль обязательны
- Никакого дополнительного текста вне JSON`, locationName, locationDescription)
}

// GenerateConnectionsPrompt создает промпт для генерации связей между локациями
func GenerateConnectionsPrompt(locations []LocationDTO) string {
	var locationNames []string
	for _, loc := range locations {
		locationNames = append(locationNames, loc.Name)
	}

	return fmt.Sprintf(`Ты — Dungeon Master в D&D. Создай связи между локациями.

Локации:
%s

Создай логичные связи между локациями. Каждая локация должна иметь 1-3 связи с другими локациями.
Связи должны соответствовать географии мира.

Ответь ТОЛЬКО валидным JSON без markdown блоков:

{
  "connections": {
    "название_локации_1": [
      {
        "to_location": "название_другой_локации",
        "direction": "north|south|east|west|up|down|portal|path",
        "description": "описание пути"
      }
    ]
  }
}

Используй только названия локаций из списка выше.`, strings.Join(locationNames, "\n- "))
}

// GenerateConnectionsPromptStrict создает более строгий промпт для retry
func GenerateConnectionsPromptStrict(locations []LocationDTO) string {
	var locationNames []string
	for _, loc := range locations {
		locationNames = append(locationNames, loc.Name)
	}

	return fmt.Sprintf(`Ты — Dungeon Master в D&D. ВАЖНО: Ответь ТОЛЬКО валидным JSON без markdown и комментариев.

Создай связи между локациями (каждая локация 1-3 связи).
Локации:
%s

Ответ ТОЛЬКО в JSON:
{
  "connections": {
    "название_локации_1": [
      {
        "to_location": "название_другой_локации",
        "direction": "north|south|east|west|up|down|portal|path",
        "description": "короткое описание пути"
      }
    ]
  }
}

Требования:
- Используй только названия из списка
- direction строго из перечисления
- description до 80 символов`, strings.Join(locationNames, "\n- "))
}
