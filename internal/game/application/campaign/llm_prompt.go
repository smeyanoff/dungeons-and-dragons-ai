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

ОБЯЗАТЕЛЬНО: первая локация в списке — низкоуровневая, мирная, оживлённая стартовая точка: деревня, таверна у дороги, приграничное поселение, портовый квартал и т.п. Игрок начинает игру именно в ней. Остальные локации — по нарастанию опасности/важности для квеста.

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

ОБЯЗАТЕЛЬНО: первая локация — мирная низкоуровневая стартовая точка (деревня, таверна, приграничное поселение и т.п.). Остальные — по нарастанию.

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

// GenerateLocationPlaybookPrompt создаёт промпт для генерации playbook локации: hook → 2–4 сцены → критический выбор → последствия.
func GenerateLocationPlaybookPrompt(locName, locDesc string, npcNames, questItemNames []string) string {
	npcsStr := "нет"
	if len(npcNames) > 0 {
		npcsStr = strings.Join(npcNames, ", ")
	}
	itemsStr := "нет"
	if len(questItemNames) > 0 {
		itemsStr = strings.Join(questItemNames, ", ")
	}
	return fmt.Sprintf(`Ты — Dungeon Master в D&D. Создай короткий playbook для локации: зацепка (hook) → 2–4 сцены (key_events) → критический выбор → последствия (possible_outcomes). Свяжи сценарий с NPC и квестовыми предметами, если они указаны.

Локация: %s
Описание: %s
NPC в локации: %s
Квестовые предметы (для связи): %s

Ответь ТОЛЬКО валидным JSON без markdown:
{
  "hook": "1–2 предложения: зацепка, открывающая ситуацию в локации",
  "key_events": ["сцена 1", "сцена 2", "сцена 3 или 4"],
  "critical_choice": "описание критического выбора, который должен принять игрок",
  "possible_outcomes": ["исход 1", "исход 2"],
  "related_npc_names": ["имя NPC из списка или пустой массив"],
  "related_quest_item_names": ["имя предмета из списка или пустой массив"]
}

Требования: key_events — 2–4 элемента; possible_outcomes — 2–3; related_npc_names и related_quest_item_names — подмножества указанных выше списков или [].`, locName, locDesc, npcsStr, itemsStr)
}
