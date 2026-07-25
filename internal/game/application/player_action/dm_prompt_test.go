package player_action

import (
	"strings"
	"testing"

	"dungeons-and-dragons-ai/internal/game/domain/player"
)

func TestBuildPersonalizedStyleInstructions(t *testing.T) {
	tests := []struct {
		name              string
		preferences       player.UserPreferences
		expectedLength    string
		expectedStyle     string
	}{
		{
			name: "low detail, dark style",
			preferences: player.UserPreferences{
				DetailLevel:     player.DetailLevelLow,
				NarrativeStyle:  player.NarrativeStyleDark,
			},
			expectedLength: "Краткие описания. 1-2 предложения.",
			expectedStyle:  "Темный, мрачный тон. Акцент на опасности, напряжении, мрачных аспектах мира. Избегать чрезмерной позитивности.",
		},
		{
			name: "high detail, light style",
			preferences: player.UserPreferences{
				DetailLevel:     player.DetailLevelHigh,
				NarrativeStyle:  player.NarrativeStyleLight,
			},
			expectedLength: "Детальные описания. 4-6 предложений с богатством деталей.",
			expectedStyle:  "Светлый, позитивный тон. Акцент на надежде, дружбе, прекрасных моментах. Избегать чрезмерного негатива.",
		},
		{
			name: "medium detail, detailed style",
			preferences: player.UserPreferences{
				DetailLevel:     player.DetailLevelMedium,
				NarrativeStyle:  player.NarrativeStyleDetailed,
			},
			expectedLength: "Средний уровень детализации. 2-3 предложения.",
			expectedStyle:  "Максимальная детализация окружения, персонажей и событий. Богатые описания сенсорных ощущений.",
		},
		{
			name: "medium detail, minimalist style",
			preferences: player.UserPreferences{
				DetailLevel:     player.DetailLevelMedium,
				NarrativeStyle:  player.NarrativeStyleMinimalist,
			},
			expectedLength: "Средний уровень детализации. 2-3 предложения.",
			expectedStyle:  "Минималистичный стиль. Только самая важная информация, без лишних деталей.",
		},
		{
			name: "medium detail, balanced style (default)",
			preferences: player.UserPreferences{
				DetailLevel:     player.DetailLevelMedium,
				NarrativeStyle:  player.NarrativeStyleBalanced,
			},
			expectedLength: "Средний уровень детализации. 2-3 предложения.",
			expectedStyle:  "Сбалансированный стиль. Реалистичные описания с элементами приключений.",
		},
		{
			name: "default preferences (empty)",
			preferences: player.UserPreferences{},
			expectedLength: "Средний уровень детализации. 2-3 предложения.",
			expectedStyle:  "Сбалансированный стиль. Реалистичные описания с элементами приключений.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lengthInstruction, styleInstruction := buildPersonalizedStyleInstructions(tt.preferences)

			if lengthInstruction != tt.expectedLength {
				t.Errorf("buildPersonalizedStyleInstructions() length = %q, want %q", lengthInstruction, tt.expectedLength)
			}

			if styleInstruction != tt.expectedStyle {
				t.Errorf("buildPersonalizedStyleInstructions() style = %q, want %q", styleInstruction, tt.expectedStyle)
			}
		})
	}
}

func TestBuildDMPrompt_NoCombat(t *testing.T) {
	gameContext := "Игрок находится в темном лесу. Вокруг деревья и тишина."
	playerMessage := "Я осматриваюсь вокруг"
	preferences := player.UserPreferences{
		DetailLevel:    player.DetailLevelMedium,
		NarrativeStyle: player.NarrativeStyleBalanced,
	}

	prompt := BuildDMPrompt(gameContext, playerMessage, preferences)

	// Check basic structure
	if !strings.Contains(prompt, "Ты — Dungeon Master в D&D 5e") {
		t.Error("BuildDMPrompt() should contain DM introduction")
	}

	if !strings.Contains(prompt, gameContext) {
		t.Error("BuildDMPrompt() should contain game context")
	}

	if !strings.Contains(prompt, playerMessage) {
		t.Error("BuildDMPrompt() should contain player message")
	}

	// Check personalization
	if !strings.Contains(prompt, "Средний уровень детализации") {
		t.Error("BuildDMPrompt() should contain detail level instruction")
	}

	if !strings.Contains(prompt, "Сбалансированный стиль") {
		t.Error("BuildDMPrompt() should contain narrative style instruction")
	}

	// Should not contain combat instruction
	if strings.Contains(prompt, "⚔️ БОЙ АКТИВЕН") {
		t.Error("BuildDMPrompt() should not contain combat instruction when no combat")
	}
}

func TestBuildDMPrompt_WithCombat(t *testing.T) {
	gameContext := `Игрок находится в пещере.
--- Текущий бой ---
Гоблин (10 HP)`
	playerMessage := "Я атакую гоблина"
	preferences := player.UserPreferences{
		DetailLevel:    player.DetailLevelHigh,
		NarrativeStyle: player.NarrativeStyleDark,
	}

	prompt := BuildDMPrompt(gameContext, playerMessage, preferences)

	// Check combat instruction is included
	if !strings.Contains(prompt, "⚔️ БОЙ АКТИВЕН") {
		t.Error("BuildDMPrompt() should contain combat instruction when combat is active")
	}

	if !strings.Contains(prompt, "Начинай ответ с '⚔️ [В БОЮ]'") {
		t.Error("BuildDMPrompt() should contain combat response instruction")
	}

	// Check personalization with high detail and dark style
	if !strings.Contains(prompt, "Детальные описания. 4-6 предложений") {
		t.Error("BuildDMPrompt() should contain high detail instruction")
	}

	if !strings.Contains(prompt, "Темный, мрачный тон") {
		t.Error("BuildDMPrompt() should contain dark style instruction")
	}
}

func TestBuildDMPrompt_WithCombatAlternativeMarker(t *testing.T) {
	gameContext := `Игрок сражается с орком.
⚔️ КРИТИЧЕСКИ ВАЖНО: Статус боя в ответах`
	playerMessage := "Я защищаюсь"
	preferences := player.UserPreferences{
		DetailLevel:    player.DetailLevelLow,
		NarrativeStyle: player.NarrativeStyleLight,
	}

	prompt := BuildDMPrompt(gameContext, playerMessage, preferences)

	// Check combat instruction is included with alternative marker
	if !strings.Contains(prompt, "⚔️ БОЙ АКТИВЕН") {
		t.Error("BuildDMPrompt() should contain combat instruction with alternative combat marker")
	}

	// Check personalization with low detail and light style
	if !strings.Contains(prompt, "Краткие описания. 1-2 предложения") {
		t.Error("BuildDMPrompt() should contain low detail instruction")
	}

	if !strings.Contains(prompt, "Светлый, позитивный тон") {
		t.Error("BuildDMPrompt() should contain light style instruction")
	}
}

func TestBuildDMPrompt_DMInstructionsIncluded(t *testing.T) {
	gameContext := "Игрок исследует замок"
	playerMessage := "Я открываю дверь"
	preferences := player.UserPreferences{
		DetailLevel:     player.DetailLevelMedium,
		NarrativeStyle:  player.NarrativeStyleDetailed,
	}

	prompt := BuildDMPrompt(gameContext, playerMessage, preferences)

	// Check that all important DM instructions are included
	requiredInstructions := []string{
		"Проверки навыков решает СИСТЕМА",
		"ЗАПРЕЩЕНО называть конкретную характеристику/навык проверки",
		"ключевых моментов",
		"Инструменты",
		"АВТОМАТИЧЕСКИ опиши последствия",
		"атмосферные описания",
		"проверки навыков к мини-ивентам",
	}

	for _, instruction := range requiredInstructions {
		if !strings.Contains(prompt, instruction) {
			t.Errorf("BuildDMPrompt() should contain instruction: %q", instruction)
		}
	}

	// Check personalization
	if !strings.Contains(prompt, "Максимальная детализация окружения") {
		t.Error("BuildDMPrompt() should contain detailed style instruction")
	}
}

func TestBuildDMPrompt_EmptyPreferences(t *testing.T) {
	gameContext := "Игрок в таверне"
	playerMessage := "Я разговариваю с барменом"
	preferences := player.UserPreferences{} // Empty preferences

	prompt := BuildDMPrompt(gameContext, playerMessage, preferences)

	// Should use default values (medium detail, balanced style)
	if !strings.Contains(prompt, "Средний уровень детализации") {
		t.Error("BuildDMPrompt() should use default medium detail level")
	}

	if !strings.Contains(prompt, "Сбалансированный стиль") {
		t.Error("BuildDMPrompt() should use default balanced style")
	}
}