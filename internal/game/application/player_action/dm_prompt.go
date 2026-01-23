package player_action

import (
	"fmt"
	"strings"

	"dungeons-and-dragons-ai/internal/game/domain/player"
)

// buildPersonalizedStyleInstructions генерирует инструкции по стилю на основе предпочтений
func buildPersonalizedStyleInstructions(preferences player.UserPreferences) (string, string) {
	var lengthInstruction, styleInstruction string

	// Инструкции по длине описаний
	switch preferences.DetailLevel {
	case player.DetailLevelLow:
		lengthInstruction = "Краткие описания. 1-2 предложения."
	case player.DetailLevelHigh:
		lengthInstruction = "Детальные описания. 4-6 предложений с богатством деталей."
	default: // DetailLevelMedium
		lengthInstruction = "Средний уровень детализации. 2-3 предложения."
	}

	// Инструкции по стилю повествования
	switch preferences.NarrativeStyle {
	case player.NarrativeStyleDark:
		styleInstruction = "Темный, мрачный тон. Акцент на опасности, напряжении, мрачных аспектах мира. Избегать чрезмерной позитивности."
	case player.NarrativeStyleLight:
		styleInstruction = "Светлый, позитивный тон. Акцент на надежде, дружбе, прекрасных моментах. Избегать чрезмерного негатива."
	case player.NarrativeStyleDetailed:
		styleInstruction = "Максимальная детализация окружения, персонажей и событий. Богатые описания сенсорных ощущений."
	case player.NarrativeStyleMinimalist:
		styleInstruction = "Минималистичный стиль. Только самая важная информация, без лишних деталей."
	default: // NarrativeStyleBalanced
		styleInstruction = "Сбалансированный стиль. Реалистичные описания с элементами приключений."
	}

	return lengthInstruction, styleInstruction
}

// BuildDMPrompt создает строгий и лаконичный промпт для Dungeon Master
// с учетом контекста игры, действия игрока и настроек персонализации
func BuildDMPrompt(gameContext, playerMessage string, preferences player.UserPreferences) string {
	// Проверяем, есть ли в контексте информация о активном бое
	hasActiveCombat := strings.Contains(gameContext, "--- Текущий бой ---") ||
		strings.Contains(gameContext, "⚔️ КРИТИЧЕСКИ ВАЖНО: Статус боя в ответах")

	combatInstruction := ""
	if hasActiveCombat {
		combatInstruction = "\n\n⚔️ БОЙ АКТИВЕН. Начинай ответ с '⚔️ [В БОЮ]'."
	}

	// Получаем персонализированные инструкции стиля
	lengthInstruction, styleInstruction := buildPersonalizedStyleInstructions(preferences)

	return fmt.Sprintf(`Ты — Dungeon Master в D&D 5e.

КОНТЕКСТ:
%s

ДЕЙСТВИЕ ИГРОКА: "%s"%s

ПРАВИЛА:

🎨 СТИЛЬ: %s, %s

⚠️ ПРОВЕРКИ: НЕ проси броски кубиков — система сама обработает. Результаты проверок уже в истории. Используй их для описания последствий.

🖼️ ИЗОБРАЖЕНИЯ: Только для ключевых моментов (боссы, важные локации). Максимум 2-3 за сессию.

🛠️ ИНСТРУМЕНТЫ: Используй только когда нужно. Для боя: perform_combat_attack. Для случайностей: roll_dice.

📚 ИСТОРИЯ: Используй RAG для согласованности сюжета. Результаты проверок в истории - используй их!

🚫 ЗАПРЕЩЕНО: Просить /roll, предупреждать о ловушках заранее, игнорировать результаты инструментов.

🌿 МИНИ-ИВЕНТЫ: Иногда добавляй атмосферные описания (1-2 предложения).

✅ ОБЯЗАТЕЛЬНО: Используй результаты проверок из истории. После провала/успеха — опиши последствия и продолжи повествование.`, gameContext, playerMessage, combatInstruction, lengthInstruction, styleInstruction)
}
