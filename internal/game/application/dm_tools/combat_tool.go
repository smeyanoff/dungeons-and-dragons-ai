package dm_tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"dungeons-and-dragons-ai/internal/game/domain/combat"
	"dungeons-and-dragons-ai/internal/game/domain/player"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/pkg/logger"
)

// CombatRepository интерфейс для работы с боями
type CombatRepository interface {
	GetActiveBySessionID(ctx context.Context, sessionID uint) (*combat.Combat, error)
	Save(ctx context.Context, c *combat.Combat) error
}

// CheckCombatStatusTool позволяет DM проверить статус активного боя
type CheckCombatStatusTool struct {
	combatRepo CombatRepository
	sessionID  uint
}

// NewCheckCombatStatusTool создает новый инструмент для проверки статуса боя
func NewCheckCombatStatusTool(combatRepo CombatRepository, sessionID uint) *CheckCombatStatusTool {
	return &CheckCombatStatusTool{
		combatRepo: combatRepo,
		sessionID:  sessionID,
	}
}

func (t *CheckCombatStatusTool) Name() string {
	return "check_combat_status"
}

func (t *CheckCombatStatusTool) Description() string {
	return "Проверить статус активного боя. Возвращает информацию о текущем бое, если он активен: состояние, текущий ход, участники боя."
}

func (t *CheckCombatStatusTool) Parameters() json.RawMessage {
	// Этот инструмент не требует параметров
	return BuildJSONSchema(nil, nil)
}

func (t *CheckCombatStatusTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	logger.Info("CheckCombatStatusTool: executing",
		logger.Uint("session_id", t.sessionID),
	)
	
	activeCombat, err := t.combatRepo.GetActiveBySessionID(ctx, t.sessionID)
	if err != nil {
		logger.Error("CheckCombatStatusTool: failed to get combat",
			logger.Uint("session_id", t.sessionID),
			logger.ErrorField(err),
		)
		return nil, fmt.Errorf("failed to check combat: %w", err)
	}
	
	if activeCombat == nil {
		logger.Info("CheckCombatStatusTool: no active combat",
			logger.Uint("session_id", t.sessionID),
		)
		return map[string]interface{}{
			"active": false,
		}, nil
	}
	
	// Собираем информацию об участниках
	participants := make([]map[string]interface{}, 0, len(activeCombat.Participants))
	playerCount := 0
	enemyCount := 0
	for _, p := range activeCombat.Participants {
		participant := map[string]interface{}{
			"is_player": p.IsPlayer,
		}
		
		if p.IsPlayer {
			playerCount++
			if p.CharacterID != nil {
				participant["character_id"] = *p.CharacterID
			}
		} else {
			enemyCount++
			participant["monster_name"] = p.MonsterName
			participant["hp"] = p.MonsterHP
			participant["max_hp"] = p.MonsterMaxHP
			participant["ac"] = p.MonsterAC
			participant["attack_bonus"] = p.MonsterAttackBonus
		}
		
		participants = append(participants, participant)
	}
	
	result := map[string]interface{}{
		"active":        true,
		"state":         string(activeCombat.State),
		"current_turn":  activeCombat.CurrentTurn,
		"participants":  participants,
		"turn_count":    len(participants),
	}
	
	logger.Info("CheckCombatStatusTool: completed successfully",
		logger.Uint("session_id", t.sessionID),
		logger.String("combat_state", string(activeCombat.State)),
		logger.Int("current_turn", activeCombat.CurrentTurn),
		logger.Int("total_participants", len(participants)),
		logger.Int("players", playerCount),
		logger.Int("enemies", enemyCount),
	)
	
	return result, nil
}

// PerformCombatAttackTool позволяет DM выполнить атаку во время активного боя
type PerformCombatAttackTool struct {
	combatRepo  CombatRepository
	sessionRepo GameSessionRepository
	sessionID   uint
}

// GameSessionRepository интерфейс для работы с сессиями
type GameSessionRepository interface {
	GetByChatID(ctx context.Context, chatID int64) (*session.GameSession, error)
}

// NewPerformCombatAttackTool создает новый инструмент для выполнения атаки в бою
func NewPerformCombatAttackTool(
	combatRepo CombatRepository,
	sessionRepo GameSessionRepository,
	sessionID uint,
) *PerformCombatAttackTool {
	return &PerformCombatAttackTool{
		combatRepo:  combatRepo,
		sessionRepo: sessionRepo,
		sessionID:   sessionID,
	}
}

func (t *PerformCombatAttackTool) Name() string {
	return "perform_combat_attack"
}

func (t *PerformCombatAttackTool) Description() string {
	return `Выполнить атаку во время активного боя. Используй этот инструмент, когда игрок описывает атакующее действие во время боя (например, "атакую", "бью мечом", "броситься вперед" и т.д.).
Инструмент автоматически:
- Определит атакующего и цель
- Выполнит бросок кубика для атаки (d20 + бонус атаки)
- Проверит попадание против AC цели
- Вычислит урон при попадании
- Применит урон к цели
- Вернет подробный результат атаки

Используй этот инструмент ТОЛЬКО во время активного боя. Если бой не активен, просто опиши действие игрока в ответе.`
}

func (t *PerformCombatAttackTool) Parameters() json.RawMessage {
	// Этот инструмент не требует параметров - он автоматически определяет атакующего и цель
	return BuildJSONSchema(nil, nil)
}

func (t *PerformCombatAttackTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	logger.Info("PerformCombatAttackTool: executing",
		logger.Uint("session_id", t.sessionID),
	)

	// Получаем активный бой
	activeCombat, err := t.combatRepo.GetActiveBySessionID(ctx, t.sessionID)
	if err != nil {
		logger.Error("PerformCombatAttackTool: failed to get combat",
			logger.Uint("session_id", t.sessionID),
			logger.ErrorField(err),
		)
		return nil, fmt.Errorf("failed to get combat: %w", err)
	}

	if activeCombat == nil || !activeCombat.IsActive() {
		logger.Info("PerformCombatAttackTool: no active combat",
			logger.Uint("session_id", t.sessionID),
		)
		return map[string]interface{}{
			"error": "Нет активного боя. Используй этот инструмент только во время активного боя.",
		}, nil
	}

	// Находим игрока в бою
	var playerParticipant *combat.CombatParticipant
	for i := range activeCombat.Participants {
		if activeCombat.Participants[i].IsPlayer && activeCombat.Participants[i].IsAlive() {
			playerParticipant = &activeCombat.Participants[i]
			break
		}
	}

	if playerParticipant == nil {
		return map[string]interface{}{
			"error": "Персонаж игрока не участвует в бою или мертв.",
		}, nil
	}

	// Находим цель (первого живого врага)
	var targetParticipant *combat.CombatParticipant
	for i := range activeCombat.Participants {
		if !activeCombat.Participants[i].IsPlayer && activeCombat.Participants[i].IsAlive() {
			targetParticipant = &activeCombat.Participants[i]
			break
		}
	}

	if targetParticipant == nil {
		// Все враги мертвы - завершаем бой
		activeCombat.State = combat.CombatStateFinished
		if err := t.combatRepo.Save(ctx, activeCombat); err != nil {
			logger.Error("PerformCombatAttackTool: failed to save combat",
				logger.ErrorField(err),
			)
		}
		return map[string]interface{}{
			"message": "🎉 Все враги побеждены! Бой окончен.",
			"combat_finished": true,
		}, nil
	}

	// Определяем выражение урона по классу персонажа
	damageExpression := getDamageByClass(string(playerParticipant.Character.Class))

	// Выполняем атаку
	result, err := combat.PerformAttack(playerParticipant, targetParticipant, damageExpression)
	if err != nil {
		logger.Error("PerformCombatAttackTool: failed to perform attack",
			logger.ErrorField(err),
		)
		return nil, fmt.Errorf("failed to perform attack: %w", err)
	}

	// Сохраняем состояние боя
	if err := t.combatRepo.Save(ctx, activeCombat); err != nil {
		logger.Error("PerformCombatAttackTool: failed to save combat",
			logger.ErrorField(err),
		)
		// Не возвращаем ошибку - результат атаки уже сформирован
	}

	// Формируем результат
	attackResult := map[string]interface{}{
		"hit":           result.Hit,
		"critical_hit":  result.CriticalHit,
		"attacker_name": result.AttackerName,
		"target_name":   result.TargetName,
		"attack_roll":   result.AttackRoll,
		"ac":            result.AC,
		"damage":        0,
		"target_hp":     targetParticipant.GetHP(),
		"target_max_hp": targetParticipant.GetMaxHP(),
	}

	if result.Hit {
		attackResult["damage"] = result.Damage
	}

	// Проверяем, не закончился ли бой
	if activeCombat.CheckCombatEnd() {
		activeCombat.State = combat.CombatStateFinished
		if err := t.combatRepo.Save(ctx, activeCombat); err != nil {
			logger.Error("PerformCombatAttackTool: failed to save combat",
				logger.ErrorField(err),
			)
		}
		attackResult["combat_finished"] = true
		
		// Проверяем, кто победил
		alivePlayers := 0
		for _, p := range activeCombat.Participants {
			if p.IsPlayer && p.IsAlive() {
				alivePlayers++
			}
		}
		
		if alivePlayers > 0 {
			attackResult["victory"] = true
			attackResult["message"] = "🎉 Победа! Все враги повержены!"
		} else {
			attackResult["victory"] = false
			attackResult["message"] = "💀 Поражение! Все игроки повержены!"
		}
	}

	logger.Info("PerformCombatAttackTool: attack completed",
		logger.Uint("session_id", t.sessionID),
		logger.Bool("hit", result.Hit),
		logger.Bool("critical", result.CriticalHit),
		logger.Int("damage", result.Damage),
	)

	return attackResult, nil
}

// getDamageByClass возвращает выражение урона по классу
func getDamageByClass(class string) string {
	damageMap := map[string]string{
		"fighter": "1d8", // Меч
		"wizard":  "1d6", // Магический урон
		"rogue":   "1d6", // Кинжал
		"cleric":  "1d8", // Булава
		"ranger":  "1d8", // Лук
	}

	if dmg, ok := damageMap[class]; ok {
		return dmg
	}
	return "1d6" // По умолчанию
}

// ApplyDamageTool позволяет DM нанести урон игроку или монстру во время боя
type ApplyDamageTool struct {
	combatRepo   CombatRepository
	sessionRepo  GameSessionRepository
	playerRepo   PlayerRepository
	sessionID    uint
	chatID       int64
}

// PlayerRepository интерфейс для работы с игроками
type PlayerRepository interface {
	GetByTgUserIDAndSessionID(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error)
	Save(ctx context.Context, p *player.Player) error
}

// syncPlayerHPFromCombat синхронизирует HP персонажа из боевого участника в БД
// Используется для обеспечения консистентности между боем и состоянием персонажа
func syncPlayerHPFromCombat(
	ctx context.Context,
	participant *combat.CombatParticipant,
	sessionRepo GameSessionRepository,
	playerRepo PlayerRepository,
	sessionID uint,
	chatID int64,
) error {
	if !participant.IsPlayer || participant.Character == nil {
		return nil // Не игрок или персонаж не загружен
	}

	// Получаем сессию для поиска игрока
	gs, err := sessionRepo.GetByChatID(ctx, chatID)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}
	if gs == nil {
		return fmt.Errorf("session not found")
	}

	// Ищем игрока по CharacterID из боевого участника
	var targetPlayer *player.Player
	if participant.CharacterID != nil {
		// Пытаемся найти игрока по CharacterID
		for i := range gs.Players {
			if gs.Players[i].CharacterID == *participant.CharacterID {
				targetPlayer = &gs.Players[i]
				break
			}
		}
	}

	// Если не нашли по CharacterID, используем первого игрока (для обратной совместимости)
	if targetPlayer == nil {
		targetPlayer = gs.GetFirstPlayer()
	}

	if targetPlayer == nil {
		return fmt.Errorf("player not found in session")
	}

	// Обновляем HP и статус персонажа из боевого участника
	targetPlayer.Character.HP = participant.Character.HP
	targetPlayer.Character.Status = participant.Character.Status
	targetPlayer.Character.MaxHP = participant.Character.MaxHP // На случай, если MaxHP изменился

	// Сохраняем изменения
	if err := playerRepo.Save(ctx, targetPlayer); err != nil {
		return fmt.Errorf("failed to save player: %w", err)
	}

	logger.Info("Player HP synced from combat",
		logger.Uint("session_id", sessionID),
		logger.Uint("character_id", targetPlayer.CharacterID),
		logger.Int("hp", targetPlayer.Character.HP),
		logger.String("status", string(targetPlayer.Character.Status)),
	)

	return nil
}

// NewApplyDamageTool создает новый инструмент для нанесения урона
func NewApplyDamageTool(
	combatRepo CombatRepository,
	sessionRepo GameSessionRepository,
	playerRepo PlayerRepository,
	sessionID uint,
	chatID int64,
) *ApplyDamageTool {
	return &ApplyDamageTool{
		combatRepo:  combatRepo,
		sessionRepo: sessionRepo,
		playerRepo:  playerRepo,
		sessionID:   sessionID,
		chatID:      chatID,
	}
}

func (t *ApplyDamageTool) Name() string {
	return "apply_damage"
}

func (t *ApplyDamageTool) Description() string {
	return `Нанести урон игроку или монстру во время активного боя. Используй этот инструмент, когда описываешь атаку врага по игроку или когда нужно нанести урон монстру по какой-то причине.

Параметры:
- target_type: "player" для игрока, "monster" для монстра
- target_name: имя цели (для монстра - имя монстра, для игрока - можно использовать "player")
- damage_amount: количество урона (положительное число)

Инструмент автоматически:
- Найдет цель (игрока или монстра) в активном бою
- Применит урон к цели
- Обновит HP в базе данных
- Вернет текущее состояние цели

Используй этот инструмент ТОЛЬКО во время активного боя. Если бой не активен, просто опиши действие в ответе.`
}

func (t *ApplyDamageTool) Parameters() json.RawMessage {
	properties := JSONSchemaProperties{
		"target_type": {
			Type:        "string",
			Description: "Тип цели: 'player' для игрока, 'monster' для монстра",
			Required:    true,
			Enum:        []interface{}{"player", "monster"},
		},
		"target_name": {
			Type:        "string",
			Description: "Имя цели (для монстра - имя монстра из боя, для игрока - можно использовать 'player')",
			Required:    true,
		},
		"damage_amount": {
			Type:        "integer",
			Description: "Количество урона (положительное число)",
			Required:    true,
		},
	}

	return BuildJSONSchema(properties, []string{"target_type", "target_name", "damage_amount"})
}

func (t *ApplyDamageTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	logger.Info("ApplyDamageTool: executing",
		logger.Uint("session_id", t.sessionID),
	)

	// Получаем параметры
	targetType, ok := args["target_type"].(string)
	if !ok {
		return nil, fmt.Errorf("target_type is required and must be a string")
	}

	targetName, ok := args["target_name"].(string)
	if !ok {
		return nil, fmt.Errorf("target_name is required and must be a string")
	}

	damageAmount, ok := args["damage_amount"].(float64)
	if !ok {
		// Попробуем integer
		damageInt, ok := args["damage_amount"].(int)
		if !ok {
			return nil, fmt.Errorf("damage_amount is required and must be a number")
		}
		damageAmount = float64(damageInt)
	}

	damage := int(damageAmount)
	if damage <= 0 {
		return map[string]interface{}{
			"error": "damage_amount must be positive",
		}, nil
	}

	// Получаем активный бой
	activeCombat, err := t.combatRepo.GetActiveBySessionID(ctx, t.sessionID)
	if err != nil {
		logger.Error("ApplyDamageTool: failed to get combat",
			logger.ErrorField(err),
		)
		return nil, fmt.Errorf("failed to get combat: %w", err)
	}

	if activeCombat == nil || !activeCombat.IsActive() {
		return map[string]interface{}{
			"error": "Нет активного боя. Используй этот инструмент только во время активного боя.",
		}, nil
	}

	// Находим цель
	var targetParticipant *combat.CombatParticipant
	var targetFound bool

	if targetType == "player" {
		// Ищем игрока
		for i := range activeCombat.Participants {
			if activeCombat.Participants[i].IsPlayer && activeCombat.Participants[i].IsAlive() {
				targetParticipant = &activeCombat.Participants[i]
				targetFound = true
				break
			}
		}
	} else if targetType == "monster" {
		// Ищем монстра по имени
		for i := range activeCombat.Participants {
			if !activeCombat.Participants[i].IsPlayer &&
				activeCombat.Participants[i].IsAlive() &&
				activeCombat.Participants[i].MonsterName == targetName {
				targetParticipant = &activeCombat.Participants[i]
				targetFound = true
				break
			}
		}
		// Если не нашли по имени, используем первого живого врага
		if !targetFound {
			for i := range activeCombat.Participants {
				if !activeCombat.Participants[i].IsPlayer && activeCombat.Participants[i].IsAlive() {
					targetParticipant = &activeCombat.Participants[i]
					targetFound = true
					break
				}
			}
		}
	} else {
		return map[string]interface{}{
			"error": fmt.Sprintf("invalid target_type: %s (must be 'player' or 'monster')", targetType),
		}, nil
	}

	if !targetFound || targetParticipant == nil {
		return map[string]interface{}{
			"error": fmt.Sprintf("Цель не найдена: %s (%s)", targetName, targetType),
		}, nil
	}

	targetDisplayName := targetParticipant.GetName()
	oldHP := targetParticipant.GetHP()

	// Применяем урон
	if err := targetParticipant.ApplyDamage(damage); err != nil {
		logger.Error("ApplyDamageTool: failed to apply damage",
			logger.ErrorField(err),
		)
		return nil, fmt.Errorf("failed to apply damage: %w", err)
	}

	newHP := targetParticipant.GetHP()

	// Сохраняем изменения в бою
	if err := t.combatRepo.Save(ctx, activeCombat); err != nil {
		logger.Error("ApplyDamageTool: failed to save combat",
			logger.ErrorField(err),
		)
		// Не возвращаем ошибку - урон уже применен
	}

	// Если цель - игрок, обновляем персонажа в БД
	if targetParticipant.IsPlayer && t.playerRepo != nil {
		if err := syncPlayerHPFromCombat(ctx, targetParticipant, t.sessionRepo, t.playerRepo, t.sessionID, t.chatID); err != nil {
			logger.Error("ApplyDamageTool: failed to sync player HP",
				logger.ErrorField(err),
				logger.Uint("session_id", t.sessionID),
			)
			// Не возвращаем ошибку - урон уже применен в бою
		}
	}

	// Проверяем, не закончился ли бой
	combatFinished := false
	victory := false
	if activeCombat.CheckCombatEnd() {
		activeCombat.State = combat.CombatStateFinished
		if err := t.combatRepo.Save(ctx, activeCombat); err != nil {
			logger.Error("ApplyDamageTool: failed to save combat",
				logger.ErrorField(err),
			)
		}
		combatFinished = true
		
		// Проверяем, кто победил
		alivePlayers := 0
		for _, p := range activeCombat.Participants {
			if p.IsPlayer && p.IsAlive() {
				alivePlayers++
			}
		}
		victory = alivePlayers > 0
	}

	// Формируем результат
	result := map[string]interface{}{
		"target_name":   targetDisplayName,
		"damage":        damage,
		"old_hp":        oldHP,
		"new_hp":        newHP,
		"max_hp":        targetParticipant.GetMaxHP(),
		"is_dead":       !targetParticipant.IsAlive(),
		"message":       fmt.Sprintf("💥 %s получил(а) %d урона. HP: %d/%d", targetDisplayName, damage, newHP, targetParticipant.GetMaxHP()),
	}

	if combatFinished {
		result["combat_finished"] = true
		result["victory"] = victory
		if victory {
			result["message"] = fmt.Sprintf("%s\n\n🎉 Победа! Все враги повержены!", result["message"])
		} else {
			result["message"] = fmt.Sprintf("%s\n\n💀 Поражение! Все игроки повержены!", result["message"])
		}
	}

	logger.Info("ApplyDamageTool: damage applied",
		logger.Uint("session_id", t.sessionID),
		logger.String("target", targetDisplayName),
		logger.Int("damage", damage),
		logger.Int("old_hp", oldHP),
		logger.Int("new_hp", newHP),
		logger.Bool("is_dead", !targetParticipant.IsAlive()),
	)

	return result, nil
}

// GetBattlefieldStatusTool позволяет DM получить визуализацию поля боя
type GetBattlefieldStatusTool struct {
	combatRepo CombatRepository
	sessionID  uint
}

// NewGetBattlefieldStatusTool создает новый инструмент для визуализации поля боя
func NewGetBattlefieldStatusTool(combatRepo CombatRepository, sessionID uint) *GetBattlefieldStatusTool {
	return &GetBattlefieldStatusTool{
		combatRepo: combatRepo,
		sessionID:  sessionID,
	}
}

func (t *GetBattlefieldStatusTool) Name() string {
	return "get_battlefield_status"
}

func (t *GetBattlefieldStatusTool) Description() string {
	return `Получить визуализацию поля боя с информацией о всех участниках. Возвращает ASCII-таблицу с информацией о каждом участнике (имя, HP, AC, инициатива, текущий ход, статус).

Параметры:
- format (опционально): формат визуализации - "table" (по умолчанию), "compact" (компактный), "detailed" (детальный)

Используй этот инструмент во время активного боя для отображения состояния поля боя в ответе игроку или для оценки ситуации.`
}

func (t *GetBattlefieldStatusTool) Parameters() json.RawMessage {
	properties := JSONSchemaProperties{
		"format": {
			Type:        "string",
			Description: "Формат визуализации: 'table' (по умолчанию), 'compact', 'detailed'",
			Required:    false,
			Enum:        []interface{}{"table", "compact", "detailed"},
		},
	}

	return BuildJSONSchema(properties, nil)
}

func (t *GetBattlefieldStatusTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	logger.Info("GetBattlefieldStatusTool: executing",
		logger.Uint("session_id", t.sessionID),
	)

	// Получаем формат (по умолчанию "table")
	format := "table"
	if formatStr, ok := args["format"].(string); ok && formatStr != "" {
		format = formatStr
	}

	// Получаем активный бой
	activeCombat, err := t.combatRepo.GetActiveBySessionID(ctx, t.sessionID)
	if err != nil {
		logger.Error("GetBattlefieldStatusTool: failed to get combat",
			logger.Uint("session_id", t.sessionID),
			logger.ErrorField(err),
		)
		return nil, fmt.Errorf("failed to get combat: %w", err)
	}

	if activeCombat == nil || !activeCombat.IsActive() {
		logger.Info("GetBattlefieldStatusTool: no active combat",
			logger.Uint("session_id", t.sessionID),
		)
		return map[string]interface{}{
			"active":  false,
			"message": "Нет активного боя.",
		}, nil
	}

	// Формируем визуализацию в зависимости от формата
	var battlefieldView string
	switch format {
	case "compact":
		battlefieldView = t.formatCompact(activeCombat)
	case "detailed":
		battlefieldView = t.formatDetailed(activeCombat)
	default: // "table"
		battlefieldView = t.formatTable(activeCombat)
	}

	result := map[string]interface{}{
		"active":       true,
		"format":       format,
		"battlefield":  battlefieldView,
		"current_turn": activeCombat.CurrentTurn,
		"participants": len(activeCombat.Participants),
		"turn_count":   len(activeCombat.Participants),
	}

	logger.Info("GetBattlefieldStatusTool: completed successfully",
		logger.Uint("session_id", t.sessionID),
		logger.String("format", format),
		logger.Int("participants", len(activeCombat.Participants)),
	)

	return result, nil
}

// formatTable форматирует поле боя в виде таблицы
func (t *GetBattlefieldStatusTool) formatTable(combat *combat.Combat) string {
	var lines []string
	lines = append(lines, "⚔️ ПОЛЕ БОЯ")
	lines = append(lines, strings.Repeat("=", 60))
	lines = append(lines, "")

	// Заголовок таблицы
	lines = append(lines, fmt.Sprintf("%-4s %-20s %-8s %-8s %-6s %-10s",
		"#", "Имя", "HP", "AC", "Иниц.", "Статус"))
	lines = append(lines, strings.Repeat("-", 60))

	// Участники
	for i, participant := range combat.Participants {
		var name string
		var hp, maxHP int
		var ac int
		var status string

		if participant.IsPlayer {
			name = participant.GetName()
			hp = participant.GetHP()
			maxHP = participant.GetMaxHP()
			ac = participant.GetAC()
		} else {
			name = participant.MonsterName
			hp = participant.GetHP()
			maxHP = participant.GetMaxHP()
			ac = participant.MonsterAC
		}

		// Определяем статус
		if !participant.IsAlive() {
			status = "💀 Мертв"
		} else if i == combat.CurrentTurn {
			status = "⚡ ХОД"
		} else {
			status = "⏳ Ожидает"
		}

		// Обрезаем имя если слишком длинное
		if len(name) > 20 {
			name = name[:17] + "..."
		}

		lines = append(lines, fmt.Sprintf("%-4d %-20s %-8s %-8d %-6d %-10s",
			i+1, name, fmt.Sprintf("%d/%d", hp, maxHP), ac, participant.Initiative, status))
	}

	lines = append(lines, strings.Repeat("-", 60))
	currentParticipant := combat.GetCurrentParticipant()
	if currentParticipant != nil {
		lines = append(lines, fmt.Sprintf("Текущий ход: #%d (%s)",
			combat.CurrentTurn+1, currentParticipant.GetName()))
	}

	return strings.Join(lines, "\n")
}

// formatCompact форматирует поле боя в компактном виде
func (t *GetBattlefieldStatusTool) formatCompact(combat *combat.Combat) string {
	var lines []string
	lines = append(lines, "⚔️ ПОЛЕ БОЯ (компактный вид)")
	lines = append(lines, "")

	for i, participant := range combat.Participants {
		var prefix string
		if i == combat.CurrentTurn {
			prefix = "⚡"
		} else if !participant.IsAlive() {
			prefix = "💀"
		} else {
			prefix = "○"
		}

		name := participant.GetName()
		hp := participant.GetHP()
		maxHP := participant.GetMaxHP()

		lines = append(lines, fmt.Sprintf("%s %s: HP %d/%d", prefix, name, hp, maxHP))
	}

	return strings.Join(lines, "\n")
}

// formatDetailed форматирует поле боя в детальном виде
func (t *GetBattlefieldStatusTool) formatDetailed(combat *combat.Combat) string {
	var lines []string
	lines = append(lines, "⚔️ ДЕТАЛЬНАЯ ИНФОРМАЦИЯ О ПОЛЕ БОЯ")
	lines = append(lines, strings.Repeat("=", 60))
	lines = append(lines, "")

	for i, participant := range combat.Participants {
		var name string
		var hp, maxHP int
		var ac int
		var attackBonus int

		if participant.IsPlayer {
			name = participant.GetName()
			hp = participant.GetHP()
			maxHP = participant.GetMaxHP()
			ac = participant.GetAC()
			attackBonus = participant.GetAttackBonus()
		} else {
			name = participant.MonsterName
			hp = participant.GetHP()
			maxHP = participant.GetMaxHP()
			ac = participant.MonsterAC
			attackBonus = participant.MonsterAttackBonus
		}

		var turnIndicator string
		if i == combat.CurrentTurn {
			turnIndicator = " ⚡ ТЕКУЩИЙ ХОД"
		}

		var status string
		if !participant.IsAlive() {
			status = "💀 Мертв"
		} else {
			status = "✅ Жив"
		}

		var role string
		if participant.IsPlayer {
			role = "Игрок"
		} else {
			role = "Враг"
		}

		lines = append(lines, fmt.Sprintf("Участник #%d%s", i+1, turnIndicator))
		lines = append(lines, fmt.Sprintf("  Имя: %s (%s)", name, role))
		lines = append(lines, fmt.Sprintf("  Статус: %s", status))
		lines = append(lines, fmt.Sprintf("  HP: %d/%d", hp, maxHP))
		lines = append(lines, fmt.Sprintf("  AC: %d", ac))
		lines = append(lines, fmt.Sprintf("  Бонус атаки: %+d", attackBonus))
		lines = append(lines, fmt.Sprintf("  Инициатива: %d", participant.Initiative))
		lines = append(lines, "")
	}

	lines = append(lines, strings.Repeat("=", 60))
	currentParticipant := combat.GetCurrentParticipant()
	if currentParticipant != nil {
		lines = append(lines, fmt.Sprintf("Текущий ход: #%d (%s)",
			combat.CurrentTurn+1, currentParticipant.GetName()))
	}

	return strings.Join(lines, "\n")
}

// PerformEnemyAttackTool позволяет DM выполнить атаку противника во время боя
type PerformEnemyAttackTool struct {
	combatRepo   CombatRepository
	sessionRepo  GameSessionRepository
	playerRepo   PlayerRepository
	sessionID    uint
	chatID       int64
}

// NewPerformEnemyAttackTool создает новый инструмент для выполнения атаки противника
func NewPerformEnemyAttackTool(
	combatRepo CombatRepository,
	sessionRepo GameSessionRepository,
	playerRepo PlayerRepository,
	sessionID uint,
	chatID int64,
) *PerformEnemyAttackTool {
	return &PerformEnemyAttackTool{
		combatRepo:  combatRepo,
		sessionRepo: sessionRepo,
		playerRepo:  playerRepo,
		sessionID:   sessionID,
		chatID:      chatID,
	}
}

func (t *PerformEnemyAttackTool) Name() string {
	return "perform_enemy_attack"
}

func (t *PerformEnemyAttackTool) Description() string {
	return `Выполнить атаку противника (врага) по игроку во время активного боя. Используй этот инструмент, когда описываешь атаку врага по игроку.

Параметры:
- enemy_id (опционально): ID врага в бою (можно использовать имя или порядковый номер). Если не указан, используется первый живой враг.
- target_id (опционально): ID цели (обычно "player" для игрока). Если не указан, используется игрок.
- damage_expression (опционально): выражение урона в формате D&D (например, "1d6", "2d4+2"). Если не указано, используется стандартный урон врага.

Инструмент автоматически:
- Найдет врага и игрока в активном бою
- Выполнит бросок кубика для атаки (d20 + бонус атаки врага)
- Проверит попадание против AC игрока
- Вычислит урон при попадании
- Применит урон к игроку
- Вернет подробный результат атаки

Используй этот инструмент ТОЛЬКО во время активного боя для атак врагов по игроку.`
}

func (t *PerformEnemyAttackTool) Parameters() json.RawMessage {
	properties := JSONSchemaProperties{
		"enemy_id": {
			Type:        "string",
			Description: "ID или имя врага в бою (опционально, по умолчанию первый живой враг)",
			Required:    false,
		},
		"target_id": {
			Type:        "string",
			Description: "ID цели (обычно 'player' для игрока, опционально)",
			Required:    false,
		},
		"damage_expression": {
			Type:        "string",
			Description: "Выражение урона в формате D&D (например, '1d6', '2d4+2', опционально)",
			Required:    false,
		},
	}

	return BuildJSONSchema(properties, nil)
}

func (t *PerformEnemyAttackTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	logger.Info("PerformEnemyAttackTool: executing",
		logger.Uint("session_id", t.sessionID),
	)

	// Получаем активный бой
	activeCombat, err := t.combatRepo.GetActiveBySessionID(ctx, t.sessionID)
	if err != nil {
		logger.Error("PerformEnemyAttackTool: failed to get combat",
			logger.Uint("session_id", t.sessionID),
			logger.ErrorField(err),
		)
		return nil, fmt.Errorf("failed to get combat: %w", err)
	}

	if activeCombat == nil || !activeCombat.IsActive() {
		logger.Info("PerformEnemyAttackTool: no active combat",
			logger.Uint("session_id", t.sessionID),
		)
		return map[string]interface{}{
			"error": "Нет активного боя. Используй этот инструмент только во время активного боя.",
		}, nil
	}

	// Получаем параметры (все опциональны)
	enemyID := ""
	if enemyIDStr, ok := args["enemy_id"].(string); ok && enemyIDStr != "" {
		enemyID = enemyIDStr
	}

	targetID := "player"
	if targetIDStr, ok := args["target_id"].(string); ok && targetIDStr != "" {
		targetID = targetIDStr
	}

	damageExpression := ""
	if damageExpr, ok := args["damage_expression"].(string); ok && damageExpr != "" {
		damageExpression = damageExpr
	}

	// Находим врага (атакующего)
	var attackerParticipant *combat.CombatParticipant
	var attackerFound bool

	if enemyID != "" {
		// Ищем врага по ID или имени
		for i := range activeCombat.Participants {
			if !activeCombat.Participants[i].IsPlayer &&
				activeCombat.Participants[i].IsAlive() {
				// Проверяем по имени или индексу
				if activeCombat.Participants[i].MonsterName == enemyID ||
					fmt.Sprintf("%d", i+1) == enemyID {
					attackerParticipant = &activeCombat.Participants[i]
					attackerFound = true
					break
				}
			}
		}
	} else {
		// Используем первого живого врага
		for i := range activeCombat.Participants {
			if !activeCombat.Participants[i].IsPlayer &&
				activeCombat.Participants[i].IsAlive() {
				attackerParticipant = &activeCombat.Participants[i]
				attackerFound = true
				break
			}
		}
	}

	if !attackerFound || attackerParticipant == nil {
		return map[string]interface{}{
			"error": "Враг не найден или все враги мертвы.",
		}, nil
	}

	// Находим цель (игрока)
	var targetParticipant *combat.CombatParticipant
	var targetFound bool

	if targetID == "player" {
		// Ищем игрока
		for i := range activeCombat.Participants {
			if activeCombat.Participants[i].IsPlayer && activeCombat.Participants[i].IsAlive() {
				targetParticipant = &activeCombat.Participants[i]
				targetFound = true
				break
			}
		}
	} else {
		// Ищем цель по ID или имени
		for i := range activeCombat.Participants {
			if activeCombat.Participants[i].IsAlive() {
				name := activeCombat.Participants[i].GetName()
				if name == targetID || fmt.Sprintf("%d", i+1) == targetID {
					targetParticipant = &activeCombat.Participants[i]
					targetFound = true
					break
				}
			}
		}
	}

	if !targetFound || targetParticipant == nil {
		return map[string]interface{}{
			"error": fmt.Sprintf("Цель не найдена: %s", targetID),
		}, nil
	}

	// Определяем выражение урона
	if damageExpression == "" {
		// Используем стандартный урон врага (1d6 для большинства монстров)
		damageExpression = "1d6"
	}

	// Выполняем атаку
	result, err := combat.PerformAttack(attackerParticipant, targetParticipant, damageExpression)
	if err != nil {
		logger.Error("PerformEnemyAttackTool: failed to perform attack",
			logger.ErrorField(err),
		)
		return nil, fmt.Errorf("failed to perform attack: %w", err)
	}

	// Сохраняем состояние боя
	if err := t.combatRepo.Save(ctx, activeCombat); err != nil {
		logger.Error("PerformEnemyAttackTool: failed to save combat",
			logger.ErrorField(err),
		)
		// Не возвращаем ошибку - результат атаки уже сформирован
	}

	// Если цель - игрок, обновляем персонажа в БД
	if targetParticipant.IsPlayer && t.playerRepo != nil {
		if err := syncPlayerHPFromCombat(ctx, targetParticipant, t.sessionRepo, t.playerRepo, t.sessionID, t.chatID); err != nil {
			logger.Error("PerformEnemyAttackTool: failed to sync player HP",
				logger.ErrorField(err),
				logger.Uint("session_id", t.sessionID),
			)
			// Не возвращаем ошибку - урон уже применен в бою
		}
	}

	// Переходим к следующему ходу после атаки врага
	activeCombat.NextTurn()

	// Проверяем, не закончился ли бой
	combatFinished := false
	victory := false
	if activeCombat.CheckCombatEnd() {
		activeCombat.State = combat.CombatStateFinished
		combatFinished = true

		// Проверяем, кто победил
		alivePlayers := 0
		for _, p := range activeCombat.Participants {
			if p.IsPlayer && p.IsAlive() {
				alivePlayers++
			}
		}
		victory = alivePlayers > 0
	}

	// Сохраняем состояние боя после перехода хода
	if err := t.combatRepo.Save(ctx, activeCombat); err != nil {
		logger.Error("PerformEnemyAttackTool: failed to save combat after next turn",
			logger.ErrorField(err),
		)
		// Не возвращаем ошибку - результат атаки уже сформирован
	}

	// Проверяем, чей следующий ход
	nextParticipant := activeCombat.GetCurrentParticipant()
	nextTurnInfo := ""
	if nextParticipant != nil && !combatFinished {
		if nextParticipant.IsPlayer {
			// Следующий ход игрока - явно сообщаем об этом
			nextTurnInfo = fmt.Sprintf("🎯 Теперь твой ход, %s! Что ты делаешь?", nextParticipant.GetName())
		} else {
			// Следующий ход другого врага (не должно происходить, но на всякий случай)
			nextTurnInfo = fmt.Sprintf("🎯 Следующий ход: %s", nextParticipant.GetName())
		}
	}

	// Формируем результат
	attackResult := map[string]interface{}{
		"hit":           result.Hit,
		"critical_hit":  result.CriticalHit,
		"attacker_name": result.AttackerName,
		"target_name":   result.TargetName,
		"attack_roll":   result.AttackRoll,
		"ac":            result.AC,
		"damage":        0,
		"target_hp":     targetParticipant.GetHP(),
		"target_max_hp": targetParticipant.GetMaxHP(),
	}

	if result.Hit {
		attackResult["damage"] = result.Damage
	}

	if nextTurnInfo != "" {
		attackResult["next_turn"] = nextTurnInfo
	}

	if combatFinished {
		attackResult["combat_finished"] = true
		attackResult["victory"] = victory
		if victory {
			attackResult["message"] = "🎉 Победа! Все враги повержены!"
		} else {
			attackResult["message"] = "💀 Поражение! Все игроки повержены!"
		}
	}

	logger.Info("PerformEnemyAttackTool: attack completed",
		logger.Uint("session_id", t.sessionID),
		logger.Bool("hit", result.Hit),
		logger.Bool("critical", result.CriticalHit),
		logger.Int("damage", result.Damage),
	)

	return attackResult, nil
}

// GetCombatParticipantStatsTool позволяет DM получить характеристики участника боя
type GetCombatParticipantStatsTool struct {
	combatRepo CombatRepository
	sessionID  uint
}

// NewGetCombatParticipantStatsTool создает новый инструмент для получения характеристик участника боя
func NewGetCombatParticipantStatsTool(combatRepo CombatRepository, sessionID uint) *GetCombatParticipantStatsTool {
	return &GetCombatParticipantStatsTool{
		combatRepo: combatRepo,
		sessionID:  sessionID,
	}
}

func (t *GetCombatParticipantStatsTool) Name() string {
	return "get_combat_participant_stats"
}

func (t *GetCombatParticipantStatsTool) Description() string {
	return `Получить характеристики участника активного боя. Возвращает AC, HP, бонус атаки, инициативу и другую информацию для участника боя.

Параметры:
- participant_id (опционально): ID участника (можно использовать имя, "player" для игрока, или порядковый номер). Если не указан, используется текущий участник хода.

Используй этот инструмент для получения характеристик участника боя перед расчетом атаки или урона.`
}

func (t *GetCombatParticipantStatsTool) Parameters() json.RawMessage {
	properties := JSONSchemaProperties{
		"participant_id": {
			Type:        "string",
			Description: "ID участника (имя, 'player' для игрока, или порядковый номер, опционально)",
			Required:    false,
		},
	}

	return BuildJSONSchema(properties, nil)
}

func (t *GetCombatParticipantStatsTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	logger.Info("GetCombatParticipantStatsTool: executing",
		logger.Uint("session_id", t.sessionID),
	)

	// Получаем активный бой
	activeCombat, err := t.combatRepo.GetActiveBySessionID(ctx, t.sessionID)
	if err != nil {
		logger.Error("GetCombatParticipantStatsTool: failed to get combat",
			logger.Uint("session_id", t.sessionID),
			logger.ErrorField(err),
		)
		return nil, fmt.Errorf("failed to get combat: %w", err)
	}

	if activeCombat == nil || !activeCombat.IsActive() {
		logger.Info("GetCombatParticipantStatsTool: no active combat",
			logger.Uint("session_id", t.sessionID),
		)
		return map[string]interface{}{
			"active":  false,
			"message": "Нет активного боя.",
		}, nil
	}

	// Получаем параметры
	participantID := ""
	if participantIDStr, ok := args["participant_id"].(string); ok && participantIDStr != "" {
		participantID = participantIDStr
	}

	// Находим участника
	var participant *combat.CombatParticipant
	var participantFound bool
	var participantIndex int

	if participantID == "" {
		// Используем текущего участника хода
		participant = activeCombat.GetCurrentParticipant()
		if participant != nil {
			participantFound = true
			participantIndex = activeCombat.CurrentTurn
		}
	} else {
		// Ищем участника по ID или имени
		for i := range activeCombat.Participants {
			p := &activeCombat.Participants[i]
			name := p.GetName()
			
			// Проверяем по имени, ID или порядковому номеру
			if name == participantID ||
				participantID == "player" && p.IsPlayer ||
				fmt.Sprintf("%d", i+1) == participantID {
				participant = p
				participantFound = true
				participantIndex = i
				break
			}
		}
	}

	if !participantFound || participant == nil {
		return map[string]interface{}{
			"error": fmt.Sprintf("Участник не найден: %s", participantID),
		}, nil
	}

	// Собираем характеристики
	var name string
	var hp, maxHP int
	var ac int
	var attackBonus int
	var initiative int
	var isPlayer bool

	if participant.IsPlayer {
		name = participant.GetName()
		hp = participant.GetHP()
		maxHP = participant.GetMaxHP()
		ac = participant.GetAC()
		attackBonus = participant.GetAttackBonus()
		initiative = participant.Initiative
		isPlayer = true
	} else {
		name = participant.MonsterName
		hp = participant.GetHP()
		maxHP = participant.GetMaxHP()
		ac = participant.MonsterAC
		attackBonus = participant.MonsterAttackBonus
		initiative = participant.Initiative
		isPlayer = false
	}

	result := map[string]interface{}{
		"participant_id":    participantIndex + 1,
		"name":              name,
		"is_player":         isPlayer,
		"is_alive":          participant.IsAlive(),
		"hp":                hp,
		"max_hp":            maxHP,
		"ac":                ac,
		"attack_bonus":      attackBonus,
		"initiative":        initiative,
		"is_current_turn":   participantIndex == activeCombat.CurrentTurn,
	}

	logger.Info("GetCombatParticipantStatsTool: completed successfully",
		logger.Uint("session_id", t.sessionID),
		logger.String("participant", name),
		logger.Int("hp", hp),
		logger.Int("ac", ac),
		logger.Int("attack_bonus", attackBonus),
	)

	return result, nil
}

// CompareAttackVsDefenseTool позволяет DM сравнить атаку vs защиты для расчета урона
type CompareAttackVsDefenseTool struct {
	combatRepo CombatRepository
	sessionID  uint
}

// NewCompareAttackVsDefenseTool создает новый инструмент для сравнения атаки vs защиты
func NewCompareAttackVsDefenseTool(combatRepo CombatRepository, sessionID uint) *CompareAttackVsDefenseTool {
	return &CompareAttackVsDefenseTool{
		combatRepo: combatRepo,
		sessionID:  sessionID,
	}
}

func (t *CompareAttackVsDefenseTool) Name() string {
	return "compare_attack_vs_defense"
}

func (t *CompareAttackVsDefenseTool) Description() string {
	return `Сравнить атаку vs защиты участников боя для расчета попадания и урона. Используй этот инструмент перед описанием атаки для получения информации о шансе попадания.

Параметры:
- attacker_id (опционально): ID атакующего (можно использовать имя, "player" для игрока, или порядковый номер). Если не указан, используется текущий участник хода.
- target_id (опционально): ID цели (можно использовать имя, "player" для игрока, или порядковый номер). Если не указан, используется первый противник.

Инструмент возвращает:
- Характеристики атакующего (AC, бонус атаки)
- Характеристики цели (AC)
- Сравнение: минимальный бросок кубика для попадания
- Вероятность попадания (примерная)

Используй этот инструмент для оценки шанса попадания перед описанием атаки.`
}

func (t *CompareAttackVsDefenseTool) Parameters() json.RawMessage {
	properties := JSONSchemaProperties{
		"attacker_id": {
			Type:        "string",
			Description: "ID атакующего (имя, 'player' для игрока, или порядковый номер, опционально)",
			Required:    false,
		},
		"target_id": {
			Type:        "string",
			Description: "ID цели (имя, 'player' для игрока, или порядковый номер, опционально)",
			Required:    false,
		},
	}

	return BuildJSONSchema(properties, nil)
}

func (t *CompareAttackVsDefenseTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	logger.Info("CompareAttackVsDefenseTool: executing",
		logger.Uint("session_id", t.sessionID),
	)

	// Получаем активный бой
	activeCombat, err := t.combatRepo.GetActiveBySessionID(ctx, t.sessionID)
	if err != nil {
		logger.Error("CompareAttackVsDefenseTool: failed to get combat",
			logger.Uint("session_id", t.sessionID),
			logger.ErrorField(err),
		)
		return nil, fmt.Errorf("failed to get combat: %w", err)
	}

	if activeCombat == nil || !activeCombat.IsActive() {
		logger.Info("CompareAttackVsDefenseTool: no active combat",
			logger.Uint("session_id", t.sessionID),
		)
		return map[string]interface{}{
			"error": "Нет активного боя.",
		}, nil
	}

	// Получаем параметры
	attackerID := ""
	if attackerIDStr, ok := args["attacker_id"].(string); ok && attackerIDStr != "" {
		attackerID = attackerIDStr
	}

	targetID := ""
	if targetIDStr, ok := args["target_id"].(string); ok && targetIDStr != "" {
		targetID = targetIDStr
	}

	// Находим атакующего
	var attacker *combat.CombatParticipant
	var attackerFound bool

	if attackerID == "" {
		// Используем текущего участника хода
		attacker = activeCombat.GetCurrentParticipant()
		attackerFound = attacker != nil
	} else {
		// Ищем атакующего по ID или имени
		for i := range activeCombat.Participants {
			p := &activeCombat.Participants[i]
			name := p.GetName()
			if name == attackerID ||
				attackerID == "player" && p.IsPlayer ||
				fmt.Sprintf("%d", i+1) == attackerID {
				attacker = p
				attackerFound = true
				break
			}
		}
	}

	if !attackerFound || attacker == nil {
		return map[string]interface{}{
			"error": fmt.Sprintf("Атакующий не найден: %s", attackerID),
		}, nil
	}

	// Находим цель
	var target *combat.CombatParticipant
	var targetFound bool

	if targetID == "" {
		// Используем первого противника
		for i := range activeCombat.Participants {
			p := &activeCombat.Participants[i]
			if p.IsAlive() && p.IsPlayer != attacker.IsPlayer {
				target = p
				targetFound = true
				break
			}
		}
	} else {
		// Ищем цель по ID или имени
		for i := range activeCombat.Participants {
			p := &activeCombat.Participants[i]
			name := p.GetName()
			if name == targetID ||
				targetID == "player" && p.IsPlayer ||
				fmt.Sprintf("%d", i+1) == targetID {
				target = p
				targetFound = true
				break
			}
		}
	}

	if !targetFound || target == nil {
		return map[string]interface{}{
			"error": fmt.Sprintf("Цель не найдена: %s", targetID),
		}, nil
	}

	// Получаем характеристики
	attackerName := attacker.GetName()
	attackerAttackBonus := attacker.GetAttackBonus()
	targetName := target.GetName()
	targetAC := target.GetAC()

	// Вычисляем минимальный бросок для попадания
	// Атака попадает, если d20 + attackBonus >= targetAC
	// Значит минимальный бросок = targetAC - attackBonus
	minRollToHit := targetAC - attackerAttackBonus
	// Минимальный бросок не может быть меньше 1 (натуральная 1 всегда промах, но формально считаем)
	if minRollToHit < 1 {
		minRollToHit = 1
	}
	if minRollToHit > 20 {
		// Невозможно попасть даже с критическим ударом (натуральная 20)
		minRollToHit = 21
	}

	// Вычисляем вероятность попадания (примерная)
	// Если minRollToHit <= 1, попадание на 19 из 20 (95%)
	// Если minRollToHit == 2, попадание на 18 из 20 (90%)
	// И так далее
	hitChance := 0.0
	if minRollToHit <= 1 {
		hitChance = 95.0 // 95% (19 из 20, не считая нат. 1 как автоматический промах)
	} else if minRollToHit <= 20 {
		// Вероятность попадания = (21 - minRollToHit) / 20 * 100
		hitChance = float64(21-minRollToHit) / 20.0 * 100.0
	} else {
		hitChance = 5.0 // 5% (только натуральная 20)
	}

	result := map[string]interface{}{
		"attacker_name":      attackerName,
		"attacker_ac":        attacker.GetAC(),
		"attacker_attack_bonus": attackerAttackBonus,
		"target_name":        targetName,
		"target_ac":          targetAC,
		"target_hp":          target.GetHP(),
		"target_max_hp":      target.GetMaxHP(),
		"min_roll_to_hit":    minRollToHit,
		"hit_chance_percent": fmt.Sprintf("%.1f%%", hitChance),
		"comparison":         fmt.Sprintf("Для попадания %s по %s нужно бросить не менее %d на d20 (с учетом бонуса атаки %+d против AC %d)", attackerName, targetName, minRollToHit, attackerAttackBonus, targetAC),
	}

	logger.Info("CompareAttackVsDefenseTool: completed successfully",
		logger.Uint("session_id", t.sessionID),
		logger.String("attacker", attackerName),
		logger.String("target", targetName),
		logger.Int("min_roll", minRollToHit),
		logger.Float64("hit_chance", hitChance),
	)

	return result, nil
}
