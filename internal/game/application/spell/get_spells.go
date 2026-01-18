package spell

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/game/domain/spell"
	"dungeons-and-dragons-ai/internal/game/infrastructure/persistence"
)

type GetSpellsUseCase struct {
	spellRepo   *persistence.SpellRepository
	sessionRepo session.Repository
}

func NewGetSpellsUseCase(
	spellRepo *persistence.SpellRepository,
	sessionRepo session.Repository,
) *GetSpellsUseCase {
	return &GetSpellsUseCase{
		spellRepo:   spellRepo,
		sessionRepo: sessionRepo,
	}
}

type GetSpellsRequest struct {
	ChatID  int64
	TgUserID int64
}

func (uc *GetSpellsUseCase) Execute(ctx context.Context, req GetSpellsRequest) (string, error) {
	// Получаем сессию
	gs, err := uc.sessionRepo.GetByChatID(ctx, req.ChatID)
	if err != nil {
		return "", fmt.Errorf("failed to get session: %w", err)
	}

	if gs == nil {
		return "Игра не начата. Используйте /newgame для начала новой игры.", nil
	}

	// Ищем игрока по TgUserID
	player := gs.FindPlayerByTgUserID(req.TgUserID)
	if player == nil {
		player = gs.GetFirstPlayer()
		if player == nil {
			return "Персонаж не создан. Используйте /createcharacter для создания персонажа.", nil
		}
	}

	// Проверяем, является ли персонаж заклинателем
	if !player.Character.IsSpellcaster() {
		return fmt.Sprintf("Ваш класс (%s) не использует заклинания. Заклинания доступны для классов: Wizard, Cleric, Ranger.", 
			player.Character.Class), nil
	}

	// Получаем известные заклинания персонажа
	characterSpells, err := uc.spellRepo.GetCharacterSpells(ctx, player.CharacterID)
	if err != nil {
		return "", fmt.Errorf("failed to get character spells: %w", err)
	}

	// Получаем все заклинания, доступные для класса
	allSpells, err := uc.spellRepo.GetByClass(ctx, character.Class(player.Character.Class))
	if err != nil {
		return "", fmt.Errorf("failed to get spells for class: %w", err)
	}

	// Создаем карту известных заклинаний для быстрого поиска
	knownSpellsMap := make(map[uint]bool)
	for _, cs := range characterSpells {
		knownSpellsMap[cs.SpellID] = true
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("📜 Заклинания персонажа %s (%s)\n\n", 
		player.Character.Name, player.Character.Class))

	// Группируем заклинания по уровню
	spellsByLevel := make(map[int][]*spell.Spell)
	for _, s := range allSpells {
		spellsByLevel[s.Level] = append(spellsByLevel[s.Level], s)
	}

	// Сортируем уровни
	var levels []int
	for level := range spellsByLevel {
		levels = append(levels, level)
	}
	sort.Ints(levels)

	// Выводим информацию о слотах заклинаний
	result.WriteString("💫 Слоты заклинаний:\n")
	hasAnySlots := false
	for i := 1; i <= 9; i++ {
		maxSlots := player.Character.SpellSlots.GetSlotsByLevel(i)
		usedSlots := player.Character.SpellSlots.GetUsedSlotsByLevel(i)
		if maxSlots > 0 {
			hasAnySlots = true
			slotEmoji := "✅"
			if usedSlots >= maxSlots {
				slotEmoji = "❌"
			}
			result.WriteString(fmt.Sprintf("  %s Уровень %d: %d/%d\n", 
				slotEmoji, i, maxSlots-usedSlots, maxSlots))
		}
	}
	if !hasAnySlots {
		result.WriteString("  У вас пока нет слотов заклинаний.\n")
	}
	result.WriteString("\n")

	// Выводим заклинания по уровням
	for _, level := range levels {
		levelSpells := spellsByLevel[level]
		sort.Slice(levelSpells, func(i, j int) bool {
			return levelSpells[i].Name < levelSpells[j].Name
		})

		levelName := "Заговоры"
		if level > 0 {
			levelName = fmt.Sprintf("Уровень %d", level)
		}

		result.WriteString(fmt.Sprintf("🔮 %s:\n", levelName))
		for _, s := range levelSpells {
			icon := s.Icon
			if icon == "" {
				icon = "✨"
			}

			// Проверяем, известно ли заклинание
			known := knownSpellsMap[s.ID]
			status := "❌ Не изучено"
			if known {
				status = "✅ Изучено"
			}

			result.WriteString(fmt.Sprintf("  %s %s - %s\n", icon, s.Name, status))
			
			// Для известных заклинаний показываем краткую информацию
			if known {
				result.WriteString(fmt.Sprintf("     %s\n", s.Description))
				if s.Damage != "" {
					result.WriteString(fmt.Sprintf("     Урон: %s\n", s.Damage))
				}
				if s.Healing != "" {
					result.WriteString(fmt.Sprintf("     Лечение: %s\n", s.Healing))
				}
				result.WriteString(fmt.Sprintf("     Время каста: %s, Дистанция: %s\n", 
					s.CastingTime, s.Range))
			}
			result.WriteString("\n")
		}
	}

	if len(allSpells) == 0 {
		return fmt.Sprintf("Для вашего класса (%s) пока нет доступных заклинаний.", 
			player.Character.Class), nil
	}

	return result.String(), nil
}