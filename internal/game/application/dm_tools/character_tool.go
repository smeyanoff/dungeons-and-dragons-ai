package dm_tools

import (
	"context"
	"encoding/json"
	"fmt"

	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/pkg/logger"
)

// SessionRepository интерфейс для работы с сессиями
type SessionRepository interface {
	GetByChatID(ctx context.Context, chatID int64) (*session.GameSession, error)
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
		"name":         char.Name,
		"race":         string(char.Race),
		"class":        string(char.Class),
		"level":        char.Level,
		"hp":           char.HP,
		"max_hp":       char.MaxHP,
		"experience":   char.Experience,
		"exp_to_next":  expToNext,
		"status":       string(char.Status),
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

// RequestAbilityCheckTool позволяет DM запросить проверку характеристики у игрока
type RequestAbilityCheckTool struct {
	sessionRepo SessionRepository
	chatID      int64
}

// NewRequestAbilityCheckTool создает новый инструмент для запроса проверки характеристики
func NewRequestAbilityCheckTool(sessionRepo SessionRepository, chatID int64) *RequestAbilityCheckTool {
	return &RequestAbilityCheckTool{
		sessionRepo: sessionRepo,
		chatID:      chatID,
	}
}

func (t *RequestAbilityCheckTool) Name() string {
	return "request_ability_check"
}

func (t *RequestAbilityCheckTool) Description() string {
	return `Запросить проверку характеристики у игрока. Возвращает информацию о характеристике и модификаторе игрока для данной проверки.

Параметры:
- ability: характеристика для проверки ("strength", "dexterity", "constitution", "intelligence", "wisdom", "charisma")
- dc (опционально): сложность проверки (Difficulty Class). Если указана, tool вернет информацию о шансе успеха.

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
			Description: "Сложность проверки (Difficulty Class, опционально)",
			Required:    false,
		},
	}

	return BuildJSONSchema(properties, []string{"ability"})
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

	dc := 0
	if dcVal, ok := args["dc"].(float64); ok {
		dc = int(dcVal)
	} else if dcVal, ok := args["dc"].(int); ok {
		dc = dcVal
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

	// Формируем результат
	result := map[string]interface{}{
		"ability":      abilityStr,
		"ability_name": abilityName,
		"ability_value": abilityValue,
		"modifier":     modifier,
		"character_name": char.Name,
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
		"ability":             abilityStr,
		"ability_name":        abilityName,
		"ability_value":       abilityValue,
		"modifier":            modifier,
		"saving_throw_modifier": savingThrowModifier,
		"proficiency_bonus":   proficiencyBonus,
		"character_name":      char.Name,
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
		"ability":       abilityStr,
		"ability_name":  abilityName,
		"dc":            dc,
		"roll_result":   rollResult,
		"success":       success,
		"critical_success": criticalSuccess,
		"critical_failure": criticalFailure,
		"character_name": characterName,
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
		"character_name": char.Name,
		"character_class": string(char.Class),
		"character_level": char.Level,
		"filter_type":    filterType,
		"abilities":      filteredAbilities,
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
