package spell

import (
	"context"
	"fmt"
	"strings"

	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/dice"
	"dungeons-and-dragons-ai/internal/game/domain/player"
	"dungeons-and-dragons-ai/internal/game/domain/spell"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/game/infrastructure/persistence"
)

type UseSpellUseCase struct {
	spellRepo   *persistence.SpellRepository
	sessionRepo session.Repository
	playerRepo  PlayerRepository
}

type PlayerRepository interface {
	GetByTgUserIDAndSessionID(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error)
	Save(ctx context.Context, player *player.Player) error
}

func NewUseSpellUseCase(
	spellRepo *persistence.SpellRepository,
	sessionRepo session.Repository,
	playerRepo PlayerRepository,
) *UseSpellUseCase {
	return &UseSpellUseCase{
		spellRepo:   spellRepo,
		sessionRepo: sessionRepo,
		playerRepo:  playerRepo,
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

	// Сохраняем изменения в персонаже (использованные слоты)
	if slotUsed {
		// Обновляем игрока через адаптер
		if err := uc.playerRepo.Save(ctx, player); err != nil {
			return nil, fmt.Errorf("failed to save player: %w", err)
		}
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
		messageParts = append(messageParts, fmt.Sprintf("💥 Урон: %d", damageDealt))
	}

	if healingDone > 0 {
		messageParts = append(messageParts, fmt.Sprintf("💚 Лечение: %d", healingDone))
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
