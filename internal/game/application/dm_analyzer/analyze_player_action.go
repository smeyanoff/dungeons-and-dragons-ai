package dm_analyzer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"dungeons-and-dragons-ai/internal/game/domain/event"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/llm/domain"
)

// PlayerActionAnalysis содержит анализ действия игрока и необходимость проверок
type PlayerActionAnalysis struct {
	// Нужна ли проверка навыка
	NeedsAbilityCheck bool `json:"needs_ability_check"`

	// Если нужна проверка, детали:
	AbilityCheck *AbilityCheckDetails `json:"ability_check,omitempty"`

	// Нужно ли использовать предопределенную проверку из локации
	NeedsPredefinedCheck bool `json:"needs_predefined_check"`

	// Если нужна предопределенная проверка, детали:
	PredefinedCheck *PredefinedCheckDetails `json:"predefined_check,omitempty"`

	// Нужно ли использовать roll_dice для случайного события
	NeedsRandomRoll bool `json:"needs_random_roll"`

	// Если нужен случайный бросок, детали:
	RandomRoll *RandomRollDetails `json:"random_roll,omitempty"`

	// Действие не требует проверок - просто описать результат
	SimpleAction bool `json:"simple_action"`

	// Рекомендация для DM
	Recommendation string `json:"recommendation,omitempty"`
}

// AbilityCheckDetails детали проверки навыка
type AbilityCheckDetails struct {
	Ability string `json:"ability"` // "strength", "dexterity", "intelligence", "wisdom", "charisma", "constitution"
	DC      int    `json:"dc"`      // Difficulty Class (10-20)
	Reason  string `json:"reason"`  // Причина проверки
	Stakes  string `json:"stakes"`  // Что на кону при успехе/провале (кратко)
}

// PredefinedCheckDetails детали предопределенной проверки
type PredefinedCheckDetails struct {
	LocationName string `json:"location_name"` // Название локации
	CheckIndex   int    `json:"check_index"`   // Индекс проверки в списке предопределенных проверок локации
	Reason       string `json:"reason"`        // Причина использования предопределенной проверки
}

// RandomRollDetails детали случайного броска
type RandomRollDetails struct {
	DiceExpression string `json:"dice_expression"` // "d20", "2d6+3", etc.
	Reason         string `json:"reason"`          // Причина броска
}

// EventRepository интерфейс для работы с событиями
type EventRepository interface {
	GetBySessionID(ctx context.Context, sessionID uint, limit int) ([]event.StoryEvent, error)
}

type AnalyzePlayerActionUseCase struct {
	llm       domain.LLM
	eventRepo EventRepository
}

func NewAnalyzePlayerActionUseCase(
	llm domain.LLM,
	eventRepo EventRepository,
) *AnalyzePlayerActionUseCase {
	return &AnalyzePlayerActionUseCase{
		llm:       llm,
		eventRepo: eventRepo,
	}
}

// Execute анализирует действие игрока и определяет необходимость проверок
func (uc *AnalyzePlayerActionUseCase) Execute(
	ctx context.Context,
	gs *session.GameSession,
	playerMessage string,
	gameContext string,
) (*PlayerActionAnalysis, error) {
	// Получаем последние события для контекста
	var recentEvents []event.StoryEvent
	if uc.eventRepo != nil {
		events, err := uc.eventRepo.GetBySessionID(ctx, gs.ID, 10)
		if err == nil {
			recentEvents = events
		}
	}

	// Формируем промпт для анализа
	prompt := buildPlayerActionAnalysisPrompt(playerMessage, gameContext, recentEvents)

	// Вызываем LLM для анализа
	llmCtx, llmCancel := context.WithTimeout(ctx, 15*time.Second)
	defer llmCancel()

	raw, err := uc.llm.Generate(llmCtx, prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM error: %w", err)
	}

	// Очищаем ответ от markdown блоков
	cleaned := cleanJSONResponse(raw)

	// Пытаемся распарсить JSON
	var analysis PlayerActionAnalysis
	if err := json.Unmarshal([]byte(cleaned), &analysis); err != nil {
		// Если не удалось распарсить, возвращаем простой анализ
		return &PlayerActionAnalysis{
			SimpleAction:   true,
			Recommendation: "Просто опиши результат действия естественным языком БЕЗ проверки.",
		}, nil
	}

	return &analysis, nil
}

// buildPlayerActionAnalysisPrompt создает промпт для анализа действия игрока
func buildPlayerActionAnalysisPrompt(playerMessage, gameContext string, recentEvents []event.StoryEvent) string {
	var parts []string

	parts = append(parts, "Ты — анализатор действий игрока в D&D. Твоя задача определить, нужна ли проверка навыка для действия игрока.")
	parts = append(parts, "")
	parts = append(parts, "Действие игрока:")
	parts = append(parts, fmt.Sprintf("\"%s\"", playerMessage))
	parts = append(parts, "")
	parts = append(parts, "Контекст игры:")
	parts = append(parts, gameContext)
	parts = append(parts, "")

	if len(recentEvents) > 0 {
		parts = append(parts, "Последние события:")
		for i, evt := range recentEvents {
			if i >= 5 { // Ограничиваем до 5 последних событий
				break
			}
			author := "Игрок"
			if evt.AuthorType == event.AuthorTypeDM {
				author = "DM"
			}
			parts = append(parts, fmt.Sprintf("- %s: %s", author, evt.Content))
		}
		parts = append(parts, "")
	}

	parts = append(parts, "ПРАВИЛА АНАЛИЗА:")
	parts = append(parts, "")
	parts = append(parts, "❌ НЕ нужна проверка для:")
	parts = append(parts, "- Простые вопросы: 'где я?', 'что здесь?', 'куда идти?'")
	parts = append(parts, "- Простое перемещение: 'пойти', 'идти', 'двигаться', 'пойти вглубь', 'пойти дальше'")
	parts = append(parts, "- Расспросы, вопросы NPC, получение информации")
	parts = append(parts, "- Разговоры, беседы с NPC")
	parts = append(parts, "- Описательные действия: 'прислушаться', 'осмотреть', 'почувствовать', 'заметить', 'увидеть', 'услышать'")
	parts = append(parts, "- Абстрактные понятия: 'проверить удачу', 'проверить реакцию', 'проверить решимость'")
	parts = append(parts, "- Простые действия: взять предмет, открыть обычную дверь, пройти по дороге, подняться по лестнице")
	parts = append(parts, "")
	parts = append(parts, "✅ Нужна проверка навыка для:")
	parts = append(parts, "- Открыть замок, взломать запертую дверь → 'dexterity' или 'strength' (DC 12-18)")
	parts = append(parts, "- Заметить ловушку, скрытую дверь, секретный проход → 'intelligence' или 'wisdom' (DC 12-15)")
	parts = append(parts, "- Убедить/обмануть/запугать NPC в ВАЖНОЙ ситуации → 'charisma' (DC 12-18)")
	parts = append(parts, "- Вспомнить важную информацию, расшифровать древний текст → 'intelligence' (DC 12-18)")
	parts = append(parts, "- Скрыться, спрятаться, избежать обнаружения → 'wisdom' (DC 10-15)")
	parts = append(parts, "")
	parts = append(parts, "📍 Предопределенные проверки из локации:")
	parts = append(parts, "- Используй ТОЛЬКО когда игрок находится в указанном месте (см. LocationHint)")
	parts = append(parts, "- НЕ используй при простом перемещении в локацию")
	parts = append(parts, "")
	parts = append(parts, "🎲 Случайные события (roll_dice):")
	parts = append(parts, "- Используй для определения случайных событий (погода, встреча, случайный результат)")
	parts = append(parts, "- НЕ проси игрока бросать кубики для случайных событий")
	parts = append(parts, "")
	parts = append(parts, "Верни JSON в следующем формате:")
	parts = append(parts, `{
  "needs_ability_check": true/false,
  "ability_check": {
    "ability": "wisdom",
    "dc": 12,
    "reason": "Коротко: зачем нужна проверка (что делает игрок)",
    "stakes": "Коротко: что произойдет при успехе/провале"
  },
  "needs_predefined_check": true/false,
  "predefined_check": {
    "location_name": "Забытый храм Аргона",
    "check_index": 0,
    "reason": "Игрок находится в указанном месте"
  },
  "needs_random_roll": true/false,
  "random_roll": {
    "dice_expression": "d20",
    "reason": "Определение случайного события"
  },
  "simple_action": true/false,
  "recommendation": "Рекомендация для DM"
}`)
	parts = append(parts, "")
	parts = append(parts, "⚠️ ВАЖНО:")
	parts = append(parts, "- Если действие простое или описательное, установи 'simple_action': true")
	parts = append(parts, "- Если нужна проверка навыка, установи 'needs_ability_check': true и заполни 'ability_check' (включая stakes)")
	parts = append(parts, "- Если нужна предопределенная проверка, установи 'needs_predefined_check': true и заполни 'predefined_check'")
	parts = append(parts, "- Если нужен случайный бросок, установи 'needs_random_roll': true и заполни 'random_roll'")

	return strings.Join(parts, "\n")
}
