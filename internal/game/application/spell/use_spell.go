package spell

import (
	"context"
	"fmt"
	"strings"

	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/combat"
	"dungeons-and-dragons-ai/internal/game/domain/dice"
	"dungeons-and-dragons-ai/internal/game/domain/player"
	"dungeons-and-dragons-ai/internal/game/domain/spell"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/game/infrastructure/persistence"
	"dungeons-and-dragons-ai/pkg/logger"
)

type UseSpellUseCase struct {
	spellRepo   *persistence.SpellRepository
	sessionRepo session.Repository
	playerRepo  PlayerRepository
	combatRepo  CombatRepository
}

type PlayerRepository interface {
	GetByTgUserIDAndSessionID(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error)
	Save(ctx context.Context, player *player.Player) error
}

type CombatRepository interface {
	GetActiveBySessionID(ctx context.Context, sessionID uint) (*combat.Combat, error)
	Save(ctx context.Context, c *combat.Combat) error
}

func NewUseSpellUseCase(
	spellRepo *persistence.SpellRepository,
	sessionRepo session.Repository,
	playerRepo PlayerRepository,
	combatRepo CombatRepository,
) *UseSpellUseCase {
	return &UseSpellUseCase{
		spellRepo:   spellRepo,
		sessionRepo: sessionRepo,
		playerRepo:  playerRepo,
		combatRepo:  combatRepo,
	}
}

type UseSpellRequest struct {
	ChatID   int64
	TgUserID int64
	SpellName string // Имя заклинания для использования
	Target   string  // Цель заклинания (опционально, для боевых заклинаний)
}

type UseSpellResponse struct {
	Success      bool
	Message      string
	SpellUsed    *spell.Spell
	DamageDealt  int  // Урон, нанесенный заклинанием
	HealingDone  int  // Лечение, сделанное заклинанием
	SlotUsed     bool // Был ли использован слот заклинания
	SpellLevel   int  // Уровень использованного заклинания
}

// Execute использует заклинание персонажем
func (uc *UseSpellUseCase) Execute(ctx context.Context, req UseSpellRequest) (*UseSpellResponse, error) {
	// Получаем сессию
	gs, err := uc.sessionRepo.GetByChatID(ctx, req.ChatID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	if gs == nil {
		return &UseSpellResponse{
			Success: false,
			Message: "Игра не начата. Используйте /newgame для начала новой игры.",
		}, nil
	}

	// Ищем игрока
	player := gs.FindPlayerByTgUserID(req.TgUserID)
	if player == nil {
		player = gs.GetFirstPlayer()
		if player == nil {
			return &UseSpellResponse{
				Success: false,
				Message: "Персонаж не создан. Используйте /createcharacter для создания персонажа.",
			}, nil
		}
	}

	// Проверяем, является ли персонаж заклинателем
	if !player.Character.IsSpellcaster() {
		return &UseSpellResponse{
			Success: false,
			Message: fmt.Sprintf("Ваш класс (%s) не использует заклинания. Заклинания доступны для классов: Wizard, Cleric, Ranger.",
				player.Character.Class),
		}, nil
	}

	// Ищем заклинание по имени (поиск нечувствителен к регистру)
	spellNameLower := strings.ToLower(strings.TrimSpace(req.SpellName))
	allSpells, err := uc.spellRepo.GetByClass(ctx, character.Class(player.Character.Class))
	if err != nil {
		return nil, fmt.Errorf("failed to get spells: %w", err)
	}

	var foundSpell *spell.Spell
	for _, s := range allSpells {
		if strings.ToLower(s.Name) == spellNameLower {
			foundSpell = s
			break
		}
	}

	if foundSpell == nil {
		return &UseSpellResponse{
			Success: false,
			Message: fmt.Sprintf("Заклинание '%s' не найдено или недоступно для вашего класса.", req.SpellName),
		}, nil
	}

	// Проверяем, известно ли заклинание персонажу
	characterSpells, err := uc.spellRepo.GetCharacterSpells(ctx, player.CharacterID)
	if err != nil {
		return nil, fmt.Errorf("failed to get character spells: %w", err)
	}

	known := false
	for _, cs := range characterSpells {
		if cs.SpellID == foundSpell.ID {
			known = true
			break
		}
	}

	if !known {
		return &UseSpellResponse{
			Success: false,
			Message: fmt.Sprintf("Вы еще не выучили заклинание '%s'. Используйте /spells для просмотра доступных заклинаний.",
				foundSpell.Name),
		}, nil
	}

	// Проверяем доступность слотов (если это не заговор)
	var slotUsed bool
	spellLevel := foundSpell.Level
	if !foundSpell.IsCantrip() {
		// Для заклинаний нужен слот соответствующего уровня
		maxSlots := player.Character.SpellSlots.GetSlotsByLevel(spellLevel)
		usedSlots := player.Character.SpellSlots.GetUsedSlotsByLevel(spellLevel)

		if usedSlots >= maxSlots {
			return &UseSpellResponse{
				Success: false,
				Message: fmt.Sprintf("У вас нет доступных слотов заклинаний %d уровня (%d/%d использовано). Используйте длинный отдых для восстановления слотов.",
					spellLevel, usedSlots, maxSlots),
			}, nil
		}

		// Используем слот заклинания
		if !player.Character.SpellSlots.UseSpellSlot(spellLevel) {
			return &UseSpellResponse{
				Success: false,
				Message: fmt.Sprintf("Не удалось использовать слот заклинания %d уровня.", spellLevel),
			}, nil
		}
		slotUsed = true
	}

	// Вычисляем эффекты заклинания (урон/лечение)
	var damageDealt, healingDone int
	if foundSpell.Damage != "" {
		damageResult, err := dice.RollDamage(foundSpell.Damage)
		if err != nil {
			return nil, fmt.Errorf("failed to roll damage for spell: %w", err)
		}
		damageDealt = damageResult.Total
	}

	if foundSpell.Healing != "" {
		healingResult, err := dice.RollDamage(foundSpell.Healing)
		if err != nil {
			return nil, fmt.Errorf("failed to roll healing for spell: %w", err)
		}
		healingDone = healingResult.Total
	}

	// Проверяем, активен ли бой
	activeCombat, err := uc.combatRepo.GetActiveBySessionID(ctx, gs.ID)
	if err != nil {
		logger.Warn("UseSpellUseCase: failed to get combat (non-critical)",
			logger.ErrorField(err),
			logger.Uint("session_id", gs.ID),
		)
		activeCombat = nil
	}

	var combatTargetParticipant *combat.CombatParticipant
	var playerParticipant *combat.CombatParticipant
	inCombat := activeCombat != nil && activeCombat.IsActive()

	if inCombat {
		// Находим игрока в бою
		for i := range activeCombat.Participants {
			if activeCombat.Participants[i].IsPlayer && activeCombat.Participants[i].IsAlive() {
				// Проверяем, что это правильный игрок
				if activeCombat.Participants[i].CharacterID != nil && *activeCombat.Participants[i].CharacterID == player.CharacterID {
					playerParticipant = &activeCombat.Participants[i]
					break
				}
			}
		}

		// Если не нашли по CharacterID, используем первого игрока
		if playerParticipant == nil {
			for i := range activeCombat.Participants {
				if activeCombat.Participants[i].IsPlayer && activeCombat.Participants[i].IsAlive() {
					playerParticipant = &activeCombat.Participants[i]
					break
				}
			}
		}

		// Находим цель в бою (если указана)
		if req.Target != "" && req.Target != "player" {
			// Ищем врага по имени
			targetLower := strings.ToLower(strings.TrimSpace(req.Target))
			for i := range activeCombat.Participants {
				if !activeCombat.Participants[i].IsPlayer &&
					activeCombat.Participants[i].IsAlive() &&
					strings.ToLower(activeCombat.Participants[i].MonsterName) == targetLower {
					combatTargetParticipant = &activeCombat.Participants[i]
					break
				}
			}
			// Если не нашли по имени, используем первого живого врага
			if combatTargetParticipant == nil {
				for i := range activeCombat.Participants {
					if !activeCombat.Participants[i].IsPlayer && activeCombat.Participants[i].IsAlive() {
						combatTargetParticipant = &activeCombat.Participants[i]
						break
					}
				}
			}
		} else if req.Target == "player" || req.Target == "" {
			// Цель - игрок (для лечения)
			combatTargetParticipant = playerParticipant
		}
	}

	// Применяем урон/лечение
	if inCombat && combatTargetParticipant != nil {
		// Применяем эффекты в бою
		if damageDealt > 0 {
			// Применяем урон к цели в бою
			if err := combatTargetParticipant.ApplyDamage(damageDealt); err != nil {
				logger.Error("UseSpellUseCase: failed to apply damage in combat",
					logger.ErrorField(err),
				)
				// Не возвращаем ошибку - продолжаем выполнение
			} else {
				logger.Info("UseSpellUseCase: damage applied in combat",
					logger.Int("damage", damageDealt),
					logger.String("target", combatTargetParticipant.GetName()),
				)
			}
		}

		if healingDone > 0 && combatTargetParticipant.IsPlayer && combatTargetParticipant.Character != nil {
			// Применяем лечение к игроку в бою
			if err := combatTargetParticipant.Character.Heal(healingDone); err != nil {
				logger.Error("UseSpellUseCase: failed to apply healing in combat",
					logger.ErrorField(err),
				)
			} else {
				logger.Info("UseSpellUseCase: healing applied in combat",
					logger.Int("healing", healingDone),
				)
			}
		}

		// Сохраняем состояние боя
		if err := uc.combatRepo.Save(ctx, activeCombat); err != nil {
			logger.Error("UseSpellUseCase: failed to save combat",
				logger.ErrorField(err),
			)
			// Не возвращаем ошибку - эффекты уже применены
		}

		// Переходим к следующему ходу после использования заклинания
		activeCombat.NextTurn()

		// Синхронизируем HP игрока с БД (если цель - игрок)
		if combatTargetParticipant.IsPlayer {
			// Обновляем HP персонажа из боевого участника
			player.Character.HP = combatTargetParticipant.Character.HP
			player.Character.Status = combatTargetParticipant.Character.Status
		}

		// Сохраняем состояние боя после перехода хода
		if err := uc.combatRepo.Save(ctx, activeCombat); err != nil {
			logger.Error("UseSpellUseCase: failed to save combat after turn transition",
				logger.ErrorField(err),
			)
		}
	} else {
		// Вне боя - применяем лечение к персонажу напрямую
		if healingDone > 0 {
			if err := player.Character.Heal(healingDone); err != nil {
				return nil, fmt.Errorf("failed to apply healing: %w", err)
			}
		}
		// Урон вне боя не применяется (заклинание должно быть использовано в бою для нанесения урона врагам)
	}

	// Сохраняем изменения в персонаже (HP, использованные слоты)
	// Всегда сохраняем, так как HP могло измениться от лечения или синхронизации с боем
	if err := uc.playerRepo.Save(ctx, player); err != nil {
		return nil, fmt.Errorf("failed to save player: %w", err)
	}

	// Формируем сообщение о результате использования заклинания
	var messageParts []string
	messageParts = append(messageParts, fmt.Sprintf("✨ Вы использовали заклинание '%s'!", foundSpell.Name))

	if foundSpell.IsCantrip() {
		messageParts = append(messageParts, "(Заговор - не расходует слот)")
	} else {
		remainingSlots := player.Character.SpellSlots.GetSlotsByLevel(spellLevel) - player.Character.SpellSlots.GetUsedSlotsByLevel(spellLevel)
		messageParts = append(messageParts, fmt.Sprintf("Использован слот %d уровня (%d осталось)", spellLevel, remainingSlots))
	}

	if damageDealt > 0 {
		if inCombat && combatTargetParticipant != nil {
			messageParts = append(messageParts, fmt.Sprintf("💥 Урон: %d (нанесен %s)", damageDealt, combatTargetParticipant.GetName()))
			if !combatTargetParticipant.IsAlive() {
				messageParts = append(messageParts, fmt.Sprintf("💀 %s повержен!", combatTargetParticipant.GetName()))
			}
		} else {
			messageParts = append(messageParts, fmt.Sprintf("💥 Урон: %d", damageDealt))
		}
	}

	if healingDone > 0 {
		if inCombat && combatTargetParticipant != nil && combatTargetParticipant.IsPlayer {
			messageParts = append(messageParts, fmt.Sprintf("💚 Лечение: %d (HP: %d/%d)", healingDone, combatTargetParticipant.GetHP(), combatTargetParticipant.GetMaxHP()))
		} else {
			messageParts = append(messageParts, fmt.Sprintf("💚 Лечение: %d (HP: %d/%d)", healingDone, player.Character.HP, player.Character.MaxHP))
		}
	}

	if inCombat {
		messageParts = append(messageParts, "\n⚔️ Ход перешел к следующему участнику боя.")
	}

	if foundSpell.Effect != "" {
		messageParts = append(messageParts, fmt.Sprintf("\n%s", foundSpell.Effect))
	}

	return &UseSpellResponse{
		Success:     true,
		Message:     strings.Join(messageParts, "\n"),
		SpellUsed:   foundSpell,
		DamageDealt: damageDealt,
		HealingDone: healingDone,
		SlotUsed:    slotUsed,
		SpellLevel:  spellLevel,
	}, nil
}
