package player_action

import (
	"fmt"

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

// BuildDMPrompt создает минималистичный промпт для Dungeon Master
func BuildDMPrompt(gameContext, playerMessage string, preferences player.UserPreferences) string {
	// Получаем персонализированные инструкции стиля
	lengthInstruction, styleInstruction := buildPersonalizedStyleInstructions(preferences)

	return fmt.Sprintf(`Ты — Dungeon Master в D&D 5e.

КОНТЕКСТ И ИСТОРИЯ:
%s

ДЕЙСТВИЕ ИГРОКА: "%s"

СТИЛЬ: %s, %s

ПРАВИЛА:
• Используй результаты проверок из истории
• Добавляй атмосферные описания (1-2 предложения)
• Используй инструменты только при необходимости
• Продолжай историю естественно`, gameContext, playerMessage, lengthInstruction, styleInstruction)
}
