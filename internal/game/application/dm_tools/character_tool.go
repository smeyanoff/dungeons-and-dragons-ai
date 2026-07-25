package dm_tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/event"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/pkg/logger"

	"github.com/google/uuid"
)

// SessionRepository интерфейс для работы с сессиями
type SessionRepository interface {
	GetByChatID(ctx context.Context, chatID int64) (*session.GameSession, error)
	Save(ctx context.Context, gs *session.GameSession) error
}

// GetCharacterStatsTool позволяет DM получить характеристики персонажа
type GetCharacterStatsTool struct {
	sessionRepo SessionRepository
	chatID      int64
}

// NewGetCharacterStatsTool создает новый инструмент для получения характеристик персонажа
func NewGetCharacterStatsTool(sessionRepo SessionRepository, chatID int64) *GetCharacterStatsTool {
	return &GetCharacterStatsTool{
		sessionRepo: sessionRepo,
		chatID:      chatID,
	}
}

func (t *GetCharacterStatsTool) Name() string {
	return "get_character_stats"
}

func (t *GetCharacterStatsTool) Description() string {
	return "Получить характеристики персонажа. Возвращает имя, расу, класс, уровень, HP, опыт и все характеристики (STR, DEX, CON, INT, WIS, CHA)."
}

func (t *GetCharacterStatsTool) Parameters() json.RawMessage {
	// Этот инструмент не требует параметров
	return BuildJSONSchema(nil, nil)
}

func (t *GetCharacterStatsTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	logger.Info("GetCharacterStatsTool: executing",
		logger.Int64("chat_id", t.chatID),
	)

	gs, err := t.sessionRepo.GetByChatID(ctx, t.chatID)
	if err != nil {
		logger.Error("GetCharacterStatsTool: failed to get session",
			logger.Int64("chat_id", t.chatID),
			logger.ErrorField(err),
		)
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	if gs == nil {
		logger.Warn("GetCharacterStatsTool: session not found",
			logger.Int64("chat_id", t.chatID),
		)
		return nil, fmt.Errorf("session not found")
	}

	player := gs.GetFirstPlayer()
	if player == nil {
		logger.Warn("GetCharacterStatsTool: player not found",
			logger.Int64("chat_id", t.chatID),
			logger.Uint("session_id", gs.ID),
		)
		return nil, fmt.Errorf("player not found")
	}

	char := player.Character
	expToNext := char.GetExperienceToNextLevel()

	result := map[string]interface{}{
		"name":        char.Name,
		"race":        string(char.Race),
		"class":       string(char.Class),
		"level":       char.Level,
		"hp":          char.HP,
		"max_hp":      char.MaxHP,
		"experience":  char.Experience,
		"exp_to_next": expToNext,
		"status":      string(char.Status),
		"stats": map[string]interface{}{
			"strength":     char.Stats.Strength,
			"dexterity":    char.Stats.Dexterity,
			"constitution": char.Stats.Constitution,
			"intelligence": char.Stats.Intelligence,
			"wisdom":       char.Stats.Wisdom,
			"charisma":     char.Stats.Charisma,
		},
	}

	logger.Info("GetCharacterStatsTool: completed successfully",
		logger.Int64("chat_id", t.chatID),
		logger.String("character_name", char.Name),
		logger.Int("level", char.Level),
		logger.Int("hp", char.HP),
		logger.Int("max_hp", char.MaxHP),
	)

	return result, nil
}

// EventRepository интерфейс для работы с событиями игры
type EventRepository interface {
	GetBySessionID(ctx context.Context, sessionID uint, limit int) ([]event.StoryEvent, error)
	Save(ctx context.Context, e *event.StoryEvent) error
}

// RequestAbilityCheckTool позволяет DM запросить проверку характеристики у игрока
type RequestAbilityCheckTool struct {
	sessionRepo SessionRepository
	eventRepo   EventRepository
	chatID      int64
}

// NewRequestAbilityCheckTool создает новый инструмент для запроса проверки характеристики
func NewRequestAbilityCheckTool(sessionRepo SessionRepository, eventRepo EventRepository, chatID int64) *RequestAbilityCheckTool {
	return &RequestAbilityCheckTool{
		sessionRepo: sessionRepo,
		eventRepo:   eventRepo,
		chatID:      chatID,
	}
}

func (t *RequestAbilityCheckTool) Name() string {
	return "request_ability_check"
}

func (t *RequestAbilityCheckTool) Description() string {
	return `Запросить проверку характеристики у игрока. Возвращает информацию о характеристике и модификаторе игрока для данной проверки.

⚠️ КРИТИЧЕСКИ ВАЖНО: 
- Перед вызовом инструмента обязательно напиши в ответе 1–2 предложения, объясняющие игроку «почему» нужна проверка и «что на кону» (успех/провал).
- Параметр 'dc' (Difficulty Class) ОБЯЗАТЕЛЕН для правильной оценки результата проверки. БЕЗ DC невозможно определить успех/провал проверки.
- После вызова этого инструмента ОБЯЗАТЕЛЬНО попроси игрока бросить кубик d20 (командой /roll).
- После броска игрока ОБЯЗАТЕЛЬНО используй инструмент 'evaluate_check' для определения успеха/провала.
- Обязательно укажи 'reason' (почему нужна проверка) и 'stakes' (что на кону). Без этого проверка будет отклонена.

Параметры:
- ability: характеристика для проверки ("strength", "dexterity", "constitution", "intelligence", "wisdom", "charisma")
- dc (ОБЯЗАТЕЛЬНО): сложность проверки (Difficulty Class). Должна быть указана для правильной оценки результата.
- reason (ОБЯЗАТЕЛЬНО): коротко, зачем нужна проверка (что игрок делает).
- stakes (ОБЯЗАТЕЛЬНО): ставки/риск результата (что произойдет при провале/успехе).

Используй этот инструмент, когда описываешь ситуацию, требующую проверки характеристики (например, "проверка силы для открытия двери", "проверка ловкости для уклонения").`
}

func (t *RequestAbilityCheckTool) Parameters() json.RawMessage {
	properties := JSONSchemaProperties{
		"ability": {
			Type:        "string",
			Description: "Характеристика для проверки: 'strength', 'dexterity', 'constitution', 'intelligence', 'wisdom', 'charisma'",
			Required:    true,
			Enum:        []interface{}{"strength", "dexterity", "constitution", "intelligence", "wisdom", "charisma"},
		},
		"dc": {
			Type:        "integer",
			Description: "Сложность проверки (Difficulty Class). ОБЯЗАТЕЛЬНО укажи DC для правильной оценки результата (10-легко, 13-средне, 16-сложно, 19-очень сложно)",
			Required:    true,
		},
		"reason": {
			Type:        "string",
			Description: "Почему нужна проверка (что именно делает игрок).",
			Required:    true,
		},
		"stakes": {
			Type:        "string",
			Description: "Ставки проверки: что произойдет при успехе/провале.",
			Required:    true,
		},
	}

	return BuildJSONSchema(properties, []string{"ability", "dc", "reason", "stakes"})
}

func (t *RequestAbilityCheckTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	logger.Info("RequestAbilityCheckTool: executing",
		logger.Int64("chat_id", t.chatID),
	)

	// Получаем параметры
	abilityStr, ok := args["ability"].(string)
	if !ok {
		return nil, fmt.Errorf("ability is required and must be a string")
	}

	// DC теперь обязательный параметр
	dc := 0
	if dcVal, ok := args["dc"].(float64); ok {
		dc = int(dcVal)
	} else if dcVal, ok := args["dc"].(int); ok {
		dc = dcVal
	} else {
		return nil, fmt.Errorf("dc is required and must be an integer (Difficulty Class for the ability check)")
	}

	if dc <= 0 || dc > 30 {
		return nil, fmt.Errorf("dc must be between 1 and 30 (typical range: 10-20)")
	}

	reason, ok := args["reason"].(string)
	if !ok || strings.TrimSpace(reason) == "" {
		logger.Warn("RequestAbilityCheckTool: missing reason",
			logger.Int64("chat_id", t.chatID),
		)
		return map[string]interface{}{
			"rejected": true,
			"reason":   "missing_reason",
			"warning":  "Проверка отклонена: обязательно укажи причину (reason) и ставки (stakes). Опиши сцену без броска или переформулируй запрос.",
		}, nil
	}

	stakes, ok := args["stakes"].(string)
	if !ok || strings.TrimSpace(stakes) == "" {
		logger.Warn("RequestAbilityCheckTool: missing stakes",
			logger.Int64("chat_id", t.chatID),
		)
		return map[string]interface{}{
			"rejected": true,
			"reason":   "missing_stakes",
			"warning":  "Проверка отклонена: обязательно укажи причину (reason) и ставки (stakes). Опиши сцену без броска или переформулируй запрос.",
		}, nil
	}

	// Получаем сессию
	gs, err := t.sessionRepo.GetByChatID(ctx, t.chatID)
	if err != nil {
		logger.Error("RequestAbilityCheckTool: failed to get session",
			logger.Int64("chat_id", t.chatID),
			logger.ErrorField(err),
		)
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	if gs == nil {
		logger.Warn("RequestAbilityCheckTool: session not found",
			logger.Int64("chat_id", t.chatID),
		)
		return nil, fmt.Errorf("session not found")
	}

	if gs.HasPendingAbilityCheck() {
		return map[string]interface{}{
			"ability":         abilityStr,
			"ability_name":    abilityStr,
			"ability_value":   0,
			"modifier":        0,
			"character_name":  "Персонаж",
			"already_pending": true,
			"warning":         "У игрока уже есть активная проверка. Дождись результата или опиши исход без новой проверки.",
		}, nil
	}

	player := gs.GetFirstPlayer()
	if player == nil {
		logger.Warn("RequestAbilityCheckTool: player not found",
			logger.Int64("chat_id", t.chatID),
			logger.Uint("session_id", gs.ID),
		)
		return nil, fmt.Errorf("player not found")
	}

	char := player.Character

	// Получаем значение характеристики и модификатор
	var abilityValue int
	var abilityName string

	switch abilityStr {
	case "strength":
		abilityValue = char.Stats.Strength
		abilityName = "Сила (STR)"
	case "dexterity":
		abilityValue = char.Stats.Dexterity
		abilityName = "Ловкость (DEX)"
	case "constitution":
		abilityValue = char.Stats.Constitution
		abilityName = "Телосложение (CON)"
	case "intelligence":
		abilityValue = char.Stats.Intelligence
		abilityName = "Интеллект (INT)"
	case "wisdom":
		abilityValue = char.Stats.Wisdom
		abilityName = "Мудрость (WIS)"
	case "charisma":
		abilityValue = char.Stats.Charisma
		abilityName = "Харизма (CHA)"
	default:
		return nil, fmt.Errorf("invalid ability: %s", abilityStr)
	}

	// Вычисляем модификатор
	modifier := calculateModifier(abilityValue)

	// P2: не повторяем одну и ту же проверку в рамках текущей сцены (локации).
	// Если проверка уже была выполнена, DM должен описать исход или эскалировать ставки/изменить подход.
	if gs.IsAbilityCheckRepeatedInScene(abilityStr) {
		return map[string]interface{}{
			"ability":         abilityStr,
			"ability_name":    abilityName,
			"ability_value":   abilityValue,
			"modifier":        modifier,
			"character_name":  char.Name,
			"repeat_in_scene": true,
			"warning":         "Повторная проверка этой характеристики в текущей сцене не требуется. Опиши последствия/исход или предложи другой подход, вместо нового броска.",
		}, nil
	}

	// Проверяем, не была ли уже выполнена проверка этой характеристики
	// В D&D проверка навыка обычно выполняется только один раз - если провалена, нужно попробовать другой подход
	if t.eventRepo != nil {
		recentEvents, err := t.eventRepo.GetBySessionID(ctx, gs.ID, 50) // Проверяем последние 50 событий для поиска предыдущих проверок
		if err == nil && len(recentEvents) > 0 {
			recentChecks := 0
			// Ищем предыдущие проверки той же характеристики
			// Проверяем события на наличие результатов evaluate_check
			for i := len(recentEvents) - 1; i >= 0; i-- {
				evt := recentEvents[i]

				// Сцена в этой модели = локация (см. GameSession.IsAbilityCheckRepeatedInScene
				// и ClearSceneAbilityCheckHistory, которая сбрасывается при перемещении).
				// Событие из другой локации относится к уже покинутой сцене и не должно
				// блокировать новую попытку той же проверки в текущей сцене.
				if evt.LocationID != nil && gs.CurrentLocationID != nil && *evt.LocationID != *gs.CurrentLocationID {
					continue
				}

				// Ищем в содержимом события упоминания о проверке этой характеристики
				// Проверяем, есть ли в событии результат evaluate_check для этой характеристики
				content := strings.ToLower(evt.Content)
				// Проверяем, является ли это сообщением от DM (не от игрока)
				if evt.AuthorType == event.AuthorTypeDM {
					// Ищем упоминания характеристики в контексте выполненной проверки навыка
					// Используем более строгие паттерны, чтобы избежать ложных срабатываний
					hasAbility := false

					// Проверяем точные совпадения с названиями характеристик в контексте проверки
					if strings.Contains(strings.ToLower(content), "проверка "+strings.ToLower(abilityName)) ||
						strings.Contains(strings.ToLower(content), abilityStr+" ") ||
						strings.Contains(strings.ToLower(content), "проверки "+strings.ToLower(abilityName)) {
						hasAbility = true
					} else {
						// Альтернативные названия только для точных совпадений в контексте проверки
						checkContext := strings.Contains(content, "проверка") || strings.Contains(content, "проверки") ||
							strings.Contains(content, "d20") || strings.Contains(content, "бросок")
						if checkContext {
							// Используем только основные названия характеристик, без синонимов
							hasAbility = strings.Contains(content, strings.ToLower(abilityName))
						}
					}

					// Проверяем, есть ли в событии результат проверки (успех или провал)
					// Ищем более специфические паттерны результатов проверок
					hasResult := strings.Contains(content, "успех") || strings.Contains(content, "провал") ||
						strings.Contains(content, "success") || strings.Contains(content, "failure") ||
						strings.Contains(content, "✅") || strings.Contains(content, "❌") ||
						strings.Contains(content, "= d20") || // результат броска
						(strings.Contains(content, "бросок") && strings.Contains(content, "результат"))

					if hasAbility && hasResult {
						// Найдена предыдущая проверка той же характеристики
						result := map[string]interface{}{
							"ability":         abilityStr,
							"ability_name":    abilityName,
							"ability_value":   abilityValue,
							"modifier":        modifier,
							"character_name":  char.Name,
							"already_checked": true,
							"warning":         fmt.Sprintf("Проверка %s уже была выполнена. В D&D проверка навыка выполняется только один раз - если она провалена, нужно попробовать другой подход (например, использовать другой навык, найти дополнительную информацию, попросить помощи у NPC). Сообщи игроку, что он уже пытался выполнить эту проверку, и предложи попробовать другой подход.", abilityName),
						}
						if dc > 0 {
							result["dc"] = dc
						}
						return result, nil
					}

					// Базовый cooldown: если похожая проверка была очень недавно, не запрашиваем повтор
					if hasAbility && time.Since(evt.CreatedAt) < 2*time.Minute {
						return map[string]interface{}{
							"ability":        abilityStr,
							"ability_name":   abilityName,
							"ability_value":  abilityValue,
							"modifier":       modifier,
							"character_name": char.Name,
							"cooldown":       true,
							"warning":        "Эта проверка выполнялась совсем недавно. Опиши исход сцены или предложи другой подход, вместо повторной проверки.",
						}, nil
					}

					if isAbilityCheckEvent(content, evt.CreatedAt) {
						recentChecks++
					}
				}
			}

			if recentChecks >= abilityCheckBudgetMax {
				logger.Info("RequestAbilityCheckTool: budget exceeded",
					logger.Int64("chat_id", t.chatID),
					logger.Int("recent_checks", recentChecks),
				)
				return map[string]interface{}{
					"ability":          abilityStr,
					"ability_name":     abilityName,
					"ability_value":    abilityValue,
					"modifier":         modifier,
					"character_name":   char.Name,
					"budget_exceeded":  true,
					"warning":          "Слишком много проверок за короткое время. Опиши исход сцены без нового броска или эскалируй ставки.",
					"budget_window":    abilityCheckBudgetWindow.String(),
					"budget_max_count": abilityCheckBudgetMax,
				}, nil
			}
		}
	}

	if isTrivialCheck(dc, stakes) {
		message := fmt.Sprintf("✅ Проверка не требуется: %s. Низкие ставки (%s) — действие выполняется автоматически.",
			strings.TrimSpace(reason), strings.TrimSpace(stakes))
		if t.eventRepo != nil {
			_ = t.eventRepo.Save(ctx, &event.StoryEvent{
				GameSessionID: gs.ID,
				LocationID:    gs.CurrentLocationID,
				AuthorType:    event.AuthorTypeDM,
				Content:       message,
				CreatedAt:     time.Now(),
			})
		}
		return map[string]interface{}{
			"ability":        abilityStr,
			"ability_name":   abilityName,
			"ability_value":  abilityValue,
			"modifier":       modifier,
			"character_name": char.Name,
			"auto_resolved":  true,
			"outcome":        "success",
			"message":        message,
		}, nil
	}

	// Сохраняем ожидаемую проверку в сессии с контекстом
	checkID := uuid.New().String()
	gs.SetPendingAbilityCheckWithContext(checkID, abilityStr, dc, reason, stakes)
	if err := t.sessionRepo.Save(ctx, gs); err != nil {
		logger.Error("RequestAbilityCheckTool: failed to save pending check",
			logger.Int64("chat_id", t.chatID),
			logger.ErrorField(err),
		)
		return nil, fmt.Errorf("failed to save pending ability check: %w", err)
	}

	// Формируем результат
	result := map[string]interface{}{
		"check_id":       checkID,
		"ability":        abilityStr,
		"ability_name":   abilityName,
		"ability_value":  abilityValue,
		"modifier":       modifier,
		"character_name": char.Name,
		"reason":         strings.TrimSpace(reason),
		"stakes":         strings.TrimSpace(stakes),
	}

	// Если указана DC, вычисляем вероятность успеха
	if dc > 0 {
		// Минимальный бросок для успеха = DC - modifier
		minRollToSucceed := dc - modifier
		if minRollToSucceed < 1 {
			minRollToSucceed = 1
		}
		if minRollToSucceed > 20 {
			minRollToSucceed = 21 // Невозможно успешно (даже с нат. 20)
		}

		// Вероятность успеха
		successChance := 0.0
		if minRollToSucceed <= 1 {
			successChance = 95.0 // 95% (19 из 20, не считая нат. 1 как автоматический промах)
		} else if minRollToSucceed <= 20 {
			successChance = float64(21-minRollToSucceed) / 20.0 * 100.0
		} else {
			successChance = 5.0 // 5% (только натуральная 20)
		}

		result["dc"] = dc
		result["min_roll_to_succeed"] = minRollToSucceed
		result["success_chance_percent"] = fmt.Sprintf("%.1f%%", successChance)
		result["description"] = fmt.Sprintf("Для успешной проверки %s (DC %d) %s нужно бросить не менее %d на d20 (с учетом модификатора %+d). Вероятность успеха: %.1f%%",
			abilityName, dc, char.Name, minRollToSucceed, modifier, successChance)
	} else {
		result["description"] = fmt.Sprintf("%s имеет %s %d (модификатор %+d)",
			char.Name, abilityName, abilityValue, modifier)
	}

	logger.Info("RequestAbilityCheckTool: completed successfully",
		logger.Int64("chat_id", t.chatID),
		logger.String("ability", abilityStr),
		logger.Int("ability_value", abilityValue),
		logger.Int("modifier", modifier),
		logger.Int("dc", dc),
	)

	return result, nil
}

const (
	abilityCheckBudgetWindow = 5 * time.Minute
	abilityCheckBudgetMax    = 3
)

func isAbilityCheckEvent(content string, createdAt time.Time) bool {
	if time.Since(createdAt) > abilityCheckBudgetWindow {
		return false
	}
	if strings.Contains(content, "🎲") && strings.Contains(content, "проверка") {
		return true
	}
	if strings.Contains(content, "проверка") && (strings.Contains(content, "dc") || strings.Contains(content, "d20")) {
		return true
	}
	return false
}

func isTrivialCheck(dc int, stakes string) bool {
	if dc > 0 && dc <= 8 {
		return true
	}
	stakesLower := strings.ToLower(strings.TrimSpace(stakes))
	if stakesLower == "" {
		return false
	}
	trivialMarkers := []string{
		"низк", "мелк", "незнач", "без риска", "безопасн", "рутин",
		"trivial", "low", "minor", "no risk",
	}
	for _, marker := range trivialMarkers {
		if strings.Contains(stakesLower, marker) {
			return true
		}
	}
	return false
}

// RequestSavingThrowTool позволяет DM запросить спасбросок у игрока
type RequestSavingThrowTool struct {
	sessionRepo SessionRepository
	chatID      int64
}

// NewRequestSavingThrowTool создает новый инструмент для запроса спасброска
func NewRequestSavingThrowTool(sessionRepo SessionRepository, chatID int64) *RequestSavingThrowTool {
	return &RequestSavingThrowTool{
		sessionRepo: sessionRepo,
		chatID:      chatID,
	}
}

func (t *RequestSavingThrowTool) Name() string {
	return "request_saving_throw"
}

func (t *RequestSavingThrowTool) Description() string {
	return `Запросить спасбросок у игрока. Возвращает информацию о характеристике и модификаторе игрока для спасброска.

Параметры:
- ability: характеристика для спасброска ("strength", "dexterity", "constitution", "intelligence", "wisdom", "charisma")
- dc (опционально): сложность спасброска (Difficulty Class). Если указана, tool вернет информацию о шансе успеха.

Используй этот инструмент, когда описываешь ситуацию, требующую спасброска (например, "спасбросок телосложения от яда", "спасбросок ловкости от ловушки").`
}

func (t *RequestSavingThrowTool) Parameters() json.RawMessage {
	properties := JSONSchemaProperties{
		"ability": {
			Type:        "string",
			Description: "Характеристика для спасброска: 'strength', 'dexterity', 'constitution', 'intelligence', 'wisdom', 'charisma'",
			Required:    true,
			Enum:        []interface{}{"strength", "dexterity", "constitution", "intelligence", "wisdom", "charisma"},
		},
		"dc": {
			Type:        "integer",
			Description: "Сложность спасброска (Difficulty Class, опционально)",
			Required:    false,
		},
	}

	return BuildJSONSchema(properties, []string{"ability"})
}

func (t *RequestSavingThrowTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	logger.Info("RequestSavingThrowTool: executing",
		logger.Int64("chat_id", t.chatID),
	)

	// Получаем параметры
	abilityStr, ok := args["ability"].(string)
	if !ok {
		return nil, fmt.Errorf("ability is required and must be a string")
	}

	dc := 0
	if dcVal, ok := args["dc"].(float64); ok {
		dc = int(dcVal)
	} else if dcVal, ok := args["dc"].(int); ok {
		dc = dcVal
	}

	// Получаем сессию
	gs, err := t.sessionRepo.GetByChatID(ctx, t.chatID)
	if err != nil {
		logger.Error("RequestSavingThrowTool: failed to get session",
			logger.Int64("chat_id", t.chatID),
			logger.ErrorField(err),
		)
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	if gs == nil {
		logger.Warn("RequestSavingThrowTool: session not found",
			logger.Int64("chat_id", t.chatID),
		)
		return nil, fmt.Errorf("session not found")
	}

	player := gs.GetFirstPlayer()
	if player == nil {
		logger.Warn("RequestSavingThrowTool: player not found",
			logger.Int64("chat_id", t.chatID),
			logger.Uint("session_id", gs.ID),
		)
		return nil, fmt.Errorf("player not found")
	}

	char := player.Character

	// Получаем значение характеристики и модификатор
	var abilityValue int
	var abilityName string

	switch abilityStr {
	case "strength":
		abilityValue = char.Stats.Strength
		abilityName = "Сила (STR)"
	case "dexterity":
		abilityValue = char.Stats.Dexterity
		abilityName = "Ловкость (DEX)"
	case "constitution":
		abilityValue = char.Stats.Constitution
		abilityName = "Телосложение (CON)"
	case "intelligence":
		abilityValue = char.Stats.Intelligence
		abilityName = "Интеллект (INT)"
	case "wisdom":
		abilityValue = char.Stats.Wisdom
		abilityName = "Мудрость (WIS)"
	case "charisma":
		abilityValue = char.Stats.Charisma
		abilityName = "Харизма (CHA)"
	default:
		return nil, fmt.Errorf("invalid ability: %s", abilityStr)
	}

	// Вычисляем модификатор (для спасбросков добавляется бонус мастерства на высоких уровнях)
	modifier := calculateModifier(abilityValue)
	// Добавляем бонус мастерства для спасбросков (на уровнях выше 1)
	proficiencyBonus := (char.Level-1)/4 + 2
	savingThrowModifier := modifier + proficiencyBonus

	// Формируем результат
	result := map[string]interface{}{
		"ability":               abilityStr,
		"ability_name":          abilityName,
		"ability_value":         abilityValue,
		"modifier":              modifier,
		"saving_throw_modifier": savingThrowModifier,
		"proficiency_bonus":     proficiencyBonus,
		"character_name":        char.Name,
	}

	// Если указана DC, вычисляем вероятность успеха
	if dc > 0 {
		// Минимальный бросок для успеха = DC - savingThrowModifier
		minRollToSucceed := dc - savingThrowModifier
		if minRollToSucceed < 1 {
			minRollToSucceed = 1
		}
		if minRollToSucceed > 20 {
			minRollToSucceed = 21 // Невозможно успешно (даже с нат. 20)
		}

		// Вероятность успеха
		successChance := 0.0
		if minRollToSucceed <= 1 {
			successChance = 95.0 // 95% (19 из 20, не считая нат. 1 как автоматический промах)
		} else if minRollToSucceed <= 20 {
			successChance = float64(21-minRollToSucceed) / 20.0 * 100.0
		} else {
			successChance = 5.0 // 5% (только натуральная 20)
		}

		result["dc"] = dc
		result["min_roll_to_succeed"] = minRollToSucceed
		result["success_chance_percent"] = fmt.Sprintf("%.1f%%", successChance)
		result["description"] = fmt.Sprintf("Для успешного спасброска %s (DC %d) %s нужно бросить не менее %d на d20 (с учетом модификатора %+d). Вероятность успеха: %.1f%%",
			abilityName, dc, char.Name, minRollToSucceed, savingThrowModifier, successChance)
	} else {
		result["description"] = fmt.Sprintf("%s имеет %s %d (модификатор спасброска %+d, включая бонус мастерства %+d)",
			char.Name, abilityName, abilityValue, savingThrowModifier, proficiencyBonus)
	}

	logger.Info("RequestSavingThrowTool: completed successfully",
		logger.Int64("chat_id", t.chatID),
		logger.String("ability", abilityStr),
		logger.Int("ability_value", abilityValue),
		logger.Int("saving_throw_modifier", savingThrowModifier),
		logger.Int("dc", dc),
	)

	return result, nil
}

// EvaluateCheckTool позволяет DM оценить результат проверки
type EvaluateCheckTool struct {
	sessionRepo SessionRepository
	chatID      int64
}

// NewEvaluateCheckTool создает новый инструмент для оценки результата проверки
func NewEvaluateCheckTool(sessionRepo SessionRepository, chatID int64) *EvaluateCheckTool {
	return &EvaluateCheckTool{
		sessionRepo: sessionRepo,
		chatID:      chatID,
	}
}

func (t *EvaluateCheckTool) Name() string {
	return "evaluate_check"
}

func (t *EvaluateCheckTool) Description() string {
	return `Оценить результат проверки характеристики или спасброска. Сравнивает результат броска с DC (сложностью проверки) и определяет успех или провал.

Параметры:
- ability: характеристика для проверки ("strength", "dexterity", "constitution", "intelligence", "wisdom", "charisma")
- dc: сложность проверки (Difficulty Class)
- roll_result: результат броска кубика (d20 + модификатор)

Используй этот инструмент для автоматической оценки результата проверки, когда игрок выполнил бросок.`
}

func (t *EvaluateCheckTool) Parameters() json.RawMessage {
	properties := JSONSchemaProperties{
		"ability": {
			Type:        "string",
			Description: "Характеристика для проверки: 'strength', 'dexterity', 'constitution', 'intelligence', 'wisdom', 'charisma'",
			Required:    true,
			Enum:        []interface{}{"strength", "dexterity", "constitution", "intelligence", "wisdom", "charisma"},
		},
		"dc": {
			Type:        "integer",
			Description: "Сложность проверки (Difficulty Class)",
			Required:    true,
		},
		"roll_result": {
			Type:        "integer",
			Description: "Результат броска кубика (d20 + модификатор)",
			Required:    true,
		},
	}

	return BuildJSONSchema(properties, []string{"ability", "dc", "roll_result"})
}

func (t *EvaluateCheckTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	logger.Info("EvaluateCheckTool: executing",
		logger.Int64("chat_id", t.chatID),
	)

	// Получаем параметры
	abilityStr, ok := args["ability"].(string)
	if !ok {
		return nil, fmt.Errorf("ability is required and must be a string")
	}

	dc := 0
	if dcVal, ok := args["dc"].(float64); ok {
		dc = int(dcVal)
	} else if dcVal, ok := args["dc"].(int); ok {
		dc = dcVal
	} else {
		return nil, fmt.Errorf("dc is required and must be a number")
	}

	rollResult := 0
	if rollVal, ok := args["roll_result"].(float64); ok {
		rollResult = int(rollVal)
	} else if rollVal, ok := args["roll_result"].(int); ok {
		rollResult = rollVal
	} else {
		return nil, fmt.Errorf("roll_result is required and must be a number")
	}

	// Получаем сессию для информации о персонаже
	gs, err := t.sessionRepo.GetByChatID(ctx, t.chatID)
	if err != nil {
		logger.Error("EvaluateCheckTool: failed to get session",
			logger.Int64("chat_id", t.chatID),
			logger.ErrorField(err),
		)
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	characterName := "Персонаж"
	if gs != nil {
		player := gs.GetFirstPlayer()
		if player != nil {
			characterName = player.Character.Name
		}
	}

	// Определяем название характеристики
	var abilityName string
	switch abilityStr {
	case "strength":
		abilityName = "Сила (STR)"
	case "dexterity":
		abilityName = "Ловкость (DEX)"
	case "constitution":
		abilityName = "Телосложение (CON)"
	case "intelligence":
		abilityName = "Интеллект (INT)"
	case "wisdom":
		abilityName = "Мудрость (WIS)"
	case "charisma":
		abilityName = "Харизма (CHA)"
	default:
		return nil, fmt.Errorf("invalid ability: %s", abilityStr)
	}

	// Определяем успех или провал
	success := rollResult >= dc
	criticalSuccess := rollResult >= dc+10 // Превышение DC на 10+ считается критическим успехом
	criticalFailure := rollResult <= dc-10 // Недостижение DC на 10+ считается критическим провалом

	result := map[string]interface{}{
		"ability":          abilityStr,
		"ability_name":     abilityName,
		"dc":               dc,
		"roll_result":      rollResult,
		"success":          success,
		"critical_success": criticalSuccess,
		"critical_failure": criticalFailure,
		"character_name":   characterName,
	}

	if success {
		if criticalSuccess {
			result["message"] = fmt.Sprintf("✅ Критический успех! %s успешно прошел проверку %s (бросок: %d против DC %d)", characterName, abilityName, rollResult, dc)
		} else {
			result["message"] = fmt.Sprintf("✅ Успех! %s успешно прошел проверку %s (бросок: %d против DC %d)", characterName, abilityName, rollResult, dc)
		}
	} else {
		if criticalFailure {
			result["message"] = fmt.Sprintf("❌ Критический провал! %s провалил проверку %s (бросок: %d против DC %d)", characterName, abilityName, rollResult, dc)
		} else {
			result["message"] = fmt.Sprintf("❌ Провал! %s провалил проверку %s (бросок: %d против DC %d)", characterName, abilityName, rollResult, dc)
		}
	}

	logger.Info("EvaluateCheckTool: completed successfully",
		logger.Int64("chat_id", t.chatID),
		logger.String("ability", abilityStr),
		logger.Int("dc", dc),
		logger.Int("roll_result", rollResult),
		logger.Bool("success", success),
	)

	return result, nil
}

// calculateModifier вычисляет модификатор характеристики (по правилам D&D)
func calculateModifier(stat int) int {
	return (stat - 10) / 2
}

// GetCharacterAbilitiesTool позволяет DM получить список доступных способностей персонажа
type GetCharacterAbilitiesTool struct {
	sessionRepo SessionRepository
	chatID      int64
}

// NewGetCharacterAbilitiesTool создает новый инструмент для получения способностей персонажа
func NewGetCharacterAbilitiesTool(sessionRepo SessionRepository, chatID int64) *GetCharacterAbilitiesTool {
	return &GetCharacterAbilitiesTool{
		sessionRepo: sessionRepo,
		chatID:      chatID,
	}
}

func (t *GetCharacterAbilitiesTool) Name() string {
	return "get_character_abilities"
}

func (t *GetCharacterAbilitiesTool) Description() string {
	return `Получить список доступных способностей персонажа. Возвращает перки (Feats), заклинания (Spells) и классовые способности.

Параметры:
- filter_type (опционально): фильтр по типу способности - "all" (все), "spells" (только заклинания), "feats" (только перки), "class" (только классовые способности)

Используй этот инструмент для получения информации о доступных способностях персонажа (перки, заклинания, классовые способности).`
}

func (t *GetCharacterAbilitiesTool) Parameters() json.RawMessage {
	properties := JSONSchemaProperties{
		"filter_type": {
			Type:        "string",
			Description: "Фильтр по типу: 'all' (все), 'spells' (только заклинания), 'feats' (только перки), 'class' (только классовые)",
			Required:    false,
			Enum:        []interface{}{"all", "spells", "feats", "class"},
		},
	}

	return BuildJSONSchema(properties, nil)
}

func (t *GetCharacterAbilitiesTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	logger.Info("GetCharacterAbilitiesTool: executing",
		logger.Int64("chat_id", t.chatID),
	)

	// Получаем параметры
	filterType := "all"
	if filterTypeStr, ok := args["filter_type"].(string); ok && filterTypeStr != "" {
		filterType = filterTypeStr
	}

	// Получаем сессию
	gs, err := t.sessionRepo.GetByChatID(ctx, t.chatID)
	if err != nil {
		logger.Error("GetCharacterAbilitiesTool: failed to get session",
			logger.Int64("chat_id", t.chatID),
			logger.ErrorField(err),
		)
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	if gs == nil {
		logger.Warn("GetCharacterAbilitiesTool: session not found",
			logger.Int64("chat_id", t.chatID),
		)
		return nil, fmt.Errorf("session not found")
	}

	player := gs.GetFirstPlayer()
	if player == nil {
		logger.Warn("GetCharacterAbilitiesTool: player not found",
			logger.Int64("chat_id", t.chatID),
			logger.Uint("session_id", gs.ID),
		)
		return nil, fmt.Errorf("player not found")
	}

	char := player.Character

	// Получаем способности персонажа по классу и уровню
	allAbilities := getCharacterAbilities(char.Class, char.Level)

	// Фильтруем по типу
	var filteredAbilities []interface{}
	for _, ability := range allAbilities {
		// Применяем фильтр
		if filterType != "all" {
			if filterType == "spells" && ability.Type != "spell" {
				continue
			}
			if filterType == "feats" && ability.Type != "feat" {
				continue
			}
			if filterType == "class" && ability.Type != "class" {
				continue
			}
		}

		abilityMap := map[string]interface{}{
			"name":        ability.Name,
			"description": ability.Description,
			"type":        string(ability.Type),
			"use_type":    string(ability.UseType),
		}

		// Добавляем специфичные для заклинаний поля
		if ability.Type == "spell" {
			abilityMap["spell_level"] = ability.SpellLevel
			abilityMap["spell_school"] = ability.SpellSchool
		}

		// Добавляем информацию об использовании
		if ability.UseType == "active" && ability.UsesPerDay > 0 {
			abilityMap["uses_per_day"] = ability.UsesPerDay
			abilityMap["uses_remaining"] = ability.UsesRemaining
		}

		filteredAbilities = append(filteredAbilities, abilityMap)
	}

	result := map[string]interface{}{
		"character_name":  char.Name,
		"character_class": string(char.Class),
		"character_level": char.Level,
		"filter_type":     filterType,
		"abilities":       filteredAbilities,
		"total_abilities": len(filteredAbilities),
	}

	logger.Info("GetCharacterAbilitiesTool: completed successfully",
		logger.Int64("chat_id", t.chatID),
		logger.String("character_name", char.Name),
		logger.String("character_class", string(char.Class)),
		logger.Int("character_level", char.Level),
		logger.Int("total_abilities", len(filteredAbilities)),
		logger.String("filter_type", filterType),
	)

	return result, nil
}

// getCharacterAbilities возвращает список способностей персонажа (упрощенная версия)
// Использует пакет character для получения способностей
func getCharacterAbilities(class character.Class, level int) []character.Ability {
	return character.GetCharacterAbilities(class, level)
}
