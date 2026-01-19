package dm_tools

import (
	"context"
	"encoding/json"
	"fmt"

	"dungeons-and-dragons-ai/internal/game/application/spell"
	"dungeons-and-dragons-ai/pkg/logger"
)

// UseSpellTool позволяет DM использовать заклинания персонажем
type UseSpellTool struct {
	useSpellUC *spell.UseSpellUseCase
	sessionRepo GameSessionRepository
	sessionID   uint
	chatID      int64
}

// NewUseSpellTool создает новый инструмент для использования заклинаний
func NewUseSpellTool(
	useSpellUC *spell.UseSpellUseCase,
	sessionRepo GameSessionRepository,
	sessionID uint,
	chatID int64,
) *UseSpellTool {
	return &UseSpellTool{
		useSpellUC:  useSpellUC,
		sessionRepo: sessionRepo,
		sessionID:   sessionID,
		chatID:      chatID,
	}
}

func (t *UseSpellTool) Name() string {
	return "use_spell"
}

func (t *UseSpellTool) Description() string {
	return `Использовать заклинание персонажем во время боя или вне боя. 

⚠️ КРИТИЧЕСКИ ВАЖНО: ВСЕГДА используй этот инструмент, когда игрок описывает использование заклинания (например, "кастую огненный шар", "использую лечение ран", "бросаю магическую стрелу"). НИКОГДА не описывай использование заклинания без вызова этого инструмента - система не знает, изучено ли заклинание персонажем, и это приведет к ошибке.

Инструмент автоматически:
- Проверит, известно ли заклинание персонажу (если не изучено - вернет ошибку)
- Проверит доступность слотов заклинаний (если это не заговор)
- Использует слот заклинания (если требуется)
- Вычислит урон/лечение заклинания
- Применит эффекты заклинания в бою или вне боя

Параметры:
- spell_name (обязательно): Имя заклинания для использования (например, "Огненный снаряд", "Лечение ран", "Магическая стрела")
- target (опционально): Цель заклинания (например, "player", "enemy", имя монстра) - используется для боевых заклинаний

Примеры:
- use_spell(spell_name="Огненный снаряд", target="goblin") - использовать боевое заклинание по врагу
- use_spell(spell_name="Лечение ран") - использовать заклинание лечения на себе/союзнике
- use_spell(spell_name="Магическая стрела", target="enemy") - использовать боевое заклинание по первому врагу

Если инструмент вернет ошибку "Вы еще не выучили заклинание", опиши это игроку и предложи использовать команду /spells для просмотра доступных заклинаний.`
}

func (t *UseSpellTool) Parameters() json.RawMessage {
	properties := JSONSchemaProperties{
		"spell_name": {
			Type:        "string",
			Description: "Имя заклинания для использования (обязательно)",
			Required:    true,
		},
		"target": {
			Type:        "string",
			Description: "Цель заклинания (опционально, для боевых заклинаний: 'player', имя врага)",
			Required:    false,
		},
	}

	return BuildJSONSchema(properties, []string{"spell_name"})
}

func (t *UseSpellTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	logger.Info("UseSpellTool: executing",
		logger.Uint("session_id", t.sessionID),
		logger.Int64("chat_id", t.chatID),
	)

	// Получаем сессию для определения TgUserID
	gs, err := t.sessionRepo.GetByChatID(ctx, t.chatID)
	if err != nil {
		logger.Error("UseSpellTool: failed to get session",
			logger.ErrorField(err),
		)
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	if gs == nil {
		return map[string]interface{}{
			"error": "Игра не начата. Используй этот инструмент только во время активной игры.",
		}, nil
	}

	// Получаем параметры
	spellName, ok := args["spell_name"].(string)
	if !ok || spellName == "" {
		return nil, fmt.Errorf("spell_name is required and must be a string")
	}

	target := ""
	if targetVal, ok := args["target"].(string); ok {
		target = targetVal
	}

	// Определяем TgUserID из сессии (используем первого игрока или chatID)
	tgUserID := t.chatID // Для приватных чатов chatID == userID
	if gs.GetFirstPlayer() != nil {
		tgUserID = gs.GetFirstPlayer().TgUserID
	}

	// Выполняем использование заклинания
	req := spell.UseSpellRequest{
		ChatID:    t.chatID,
		TgUserID:  tgUserID,
		SpellName: spellName,
		Target:    target,
	}

	resp, err := t.useSpellUC.Execute(ctx, req)
	if err != nil {
		logger.Error("UseSpellTool: failed to use spell",
			logger.ErrorField(err),
			logger.String("spell_name", spellName),
		)
		return map[string]interface{}{
			"error":   err.Error(),
			"success": false,
		}, nil
	}

	if !resp.Success {
		return map[string]interface{}{
			"error":   resp.Message,
			"success": false,
		}, nil
	}

	logger.Info("UseSpellTool: spell used successfully",
		logger.String("spell_name", spellName),
		logger.Int("damage", resp.DamageDealt),
		logger.Int("healing", resp.HealingDone),
		logger.Bool("slot_used", resp.SlotUsed),
	)

	// Формируем результат для DM
	result := map[string]interface{}{
		"success":       true,
		"spell_name":    resp.SpellUsed.Name,
		"spell_level":   resp.SpellLevel,
		"message":       resp.Message,
		"damage_dealt":  resp.DamageDealt,
		"healing_done":  resp.HealingDone,
		"slot_used":     resp.SlotUsed,
	}

	// Добавляем детальную информацию о заклинании
	if resp.SpellUsed != nil {
		result["spell_description"] = resp.SpellUsed.Description
		result["spell_damage"] = resp.SpellUsed.Damage
		result["spell_healing"] = resp.SpellUsed.Healing
		result["spell_effect"] = resp.SpellUsed.Effect
		result["casting_time"] = string(resp.SpellUsed.CastingTime)
		result["range"] = string(resp.SpellUsed.Range)
	}

	return result, nil
}
