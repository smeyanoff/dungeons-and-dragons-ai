package character

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"time"

	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/inventory"
	"dungeons-and-dragons-ai/internal/game/domain/player"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/game/domain/spell"
)

type CreateCharacterUseCase struct {
	sessionRepo   session.Repository
	playerRepo    PlayerRepository
	inventoryRepo InventoryRepository
	spellRepo     SpellRepository
}

type PlayerRepository interface {
	GetByTgUserIDAndSessionID(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error)
	Save(ctx context.Context, p *player.Player) error
}

type InventoryRepository interface {
	Save(ctx context.Context, inv *inventory.Inventory) error
}

// SpellRepository — часть API persistence.SpellRepository, нужная для наполнения
// книги заклинаний персонажа при создании.
type SpellRepository interface {
	GetByClass(ctx context.Context, class character.Class) ([]*spell.Spell, error)
	SaveCharacterSpell(ctx context.Context, cs *spell.CharacterSpell) error
}

func NewCreateCharacterUseCase(
	sessionRepo session.Repository,
	playerRepo PlayerRepository,
	inventoryRepo InventoryRepository,
	spellRepo SpellRepository,
) *CreateCharacterUseCase {
	return &CreateCharacterUseCase{
		sessionRepo:   sessionRepo,
		playerRepo:    playerRepo,
		inventoryRepo: inventoryRepo,
		spellRepo:     spellRepo,
	}
}

type CreateCharacterRequest struct {
	ChatID int64
	Name   string
	Race   character.Race
	Class  character.Class
	Stats  *character.Stats
}

func (uc *CreateCharacterUseCase) Execute(
	ctx context.Context,
	req CreateCharacterRequest,
) (*player.Player, error) {
	// Получаем сессию
	gs, err := uc.sessionRepo.GetByChatID(ctx, req.ChatID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	if gs == nil {
		return nil, fmt.Errorf("game session not found, use /newgame first")
	}

	if !gs.IsActive() {
		return nil, fmt.Errorf("game session is not active")
	}

	// Проверяем, есть ли уже персонаж у игрока
	// Примечание: в приватных чатах Telegram ChatID = UserID, поэтому используем req.ChatID как TgUserID
	existingPlayer, err := uc.playerRepo.GetByTgUserIDAndSessionID(ctx, req.ChatID, gs.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing player: %w", err)
	}

	if existingPlayer != nil {
		return nil, fmt.Errorf("character already created for this session")
	}

	// Валидация имени
	if strings.TrimSpace(req.Name) == "" {
		return nil, fmt.Errorf("character name cannot be empty")
	}

	// Генерируем характеристики, если не предоставлены
	stats := req.Stats
	if stats == nil {
		stats = generateStats()
	}

	// Применяем бонусы расы к характеристикам
	applyRaceBonuses(stats, req.Race)

	// Создаем персонажа
	char, err := character.NewCharacter(req.Name, req.Class, req.Race, *stats)
	if err != nil {
		return nil, fmt.Errorf("failed to create character: %w", err)
	}

	// Создаем игрока
	p := &player.Player{
		TgUserID:      req.ChatID,
		Name:          req.Name,
		GameSessionID: gs.ID,
		Character:     *char,
	}

	if err := uc.playerRepo.Save(ctx, p); err != nil {
		return nil, fmt.Errorf("failed to save player: %w", err)
	}

	// Выдаём стартовое снаряжение и немного золота
	startingInventory := buildStartingInventory(p.Character.ID, req.Class)
	if err := uc.inventoryRepo.Save(ctx, startingInventory); err != nil {
		return nil, fmt.Errorf("failed to save starting inventory: %w", err)
	}

	// Наполняем книгу заклинаний персонажа фиксированным стартовым списком
	// (актуально только для классов, заучивающих заклинания заранее — см. UsesSpellbook)
	if char.UsesSpellbook() {
		if err := uc.grantStartingSpells(ctx, p.Character.ID, req.Class); err != nil {
			return nil, fmt.Errorf("failed to grant starting spells: %w", err)
		}
	}

	return p, nil
}

// grantStartingSpells записывает в книгу заклинаний персонажа фиксированный список
// заклинаний, соответствующий стартовым способностям класса (character.GetCharacterAbilities).
// Это единственный источник заклинаний, доступных персонажу с UsesSpellbook() == true —
// скастовать заклинание, отсутствующее в этом списке, будет невозможно (см. UseSpellUseCase).
func (uc *CreateCharacterUseCase) grantStartingSpells(ctx context.Context, characterID uint, class character.Class) error {
	var startingSpellNames []string
	for _, ability := range character.GetCharacterAbilities(class, 1) {
		if ability.Type == character.AbilityTypeSpell && ability.SpellLevel > 0 {
			startingSpellNames = append(startingSpellNames, ability.Name)
		}
	}

	if len(startingSpellNames) == 0 {
		return nil
	}

	classSpells, err := uc.spellRepo.GetByClass(ctx, class)
	if err != nil {
		return fmt.Errorf("failed to load class spells: %w", err)
	}

	for _, name := range startingSpellNames {
		for _, s := range classSpells {
			if s.Name != name {
				continue
			}
			cs := &spell.CharacterSpell{
				CharacterID: characterID,
				SpellID:     s.ID,
				Prepared:    true,
				LearnedAt:   time.Now(),
			}
			if err := uc.spellRepo.SaveCharacterSpell(ctx, cs); err != nil {
				return fmt.Errorf("failed to save character spell %q: %w", name, err)
			}
			break
		}
	}

	return nil
}

// generateStats генерирует характеристики персонажа по стандартным правилам D&D
// Использует метод "4d6 drop lowest" для каждой характеристики
// Использует crypto/rand для криптографически безопасной генерации случайных чисел
func generateStats() *character.Stats {
	// secureRandInt генерирует криптографически безопасное случайное число от 1 до max (включительно)
	secureRandInt := func(max int) int {
		if max <= 0 {
			return 0
		}
		// Генерируем случайное число в диапазоне [0, max)
		bigMax := big.NewInt(int64(max))
		randomBig, err := rand.Int(rand.Reader, bigMax)
		if err != nil {
			// Fallback: если crypto/rand не работает, возвращаем 1 (не должно происходить)
			return 1
		}
		// Преобразуем в int и добавляем 1 для диапазона [1, max]
		return int(randomBig.Int64()) + 1
	}

	rollStat := func() int {
		// Бросаем 4d6 используя криптографически безопасный генератор
		rolls := make([]int, 4)
		for i := range rolls {
			rolls[i] = secureRandInt(6) // d6: от 1 до 6
		}

		// Находим минимальное значение
		min := rolls[0]
		for _, r := range rolls[1:] {
			if r < min {
				min = r
			}
		}

		// Суммируем все, кроме минимального
		sum := 0
		for _, r := range rolls {
			if r != min {
				sum += r
			}
		}

		// В D&D 5e минимальная характеристика обычно 8 (стандартный массив включает 8)
		// Это предотвращает создание персонажей с критически низкими характеристиками
		if sum < 8 {
			sum = 8
		}

		return sum
	}

	return &character.Stats{
		Strength:     rollStat(),
		Dexterity:    rollStat(),
		Constitution: rollStat(),
		Intelligence: rollStat(),
		Wisdom:       rollStat(),
		Charisma:     rollStat(),
	}
}

// applyRaceBonuses применяет бонусы расы к характеристикам персонажа
// Соответствует стандартным правилам D&D 5e
func applyRaceBonuses(stats *character.Stats, race character.Race) {
	switch race {
	case character.RaceDwarf:
		// Дварфы получают +2 к Телосложению
		// В D&D 5e также есть подрасы, но для упрощения используем базовый бонус
		stats.Constitution += 2
		// Максимальная характеристика в D&D 5e = 20
		if stats.Constitution > 20 {
			stats.Constitution = 20
		}
	case character.RaceElf:
		// Эльфы получают +2 к Ловкости
		stats.Dexterity += 2
		if stats.Dexterity > 20 {
			stats.Dexterity = 20
		}
	case character.RaceHuman:
		// Люди получают +1 ко всем характеристикам (стандартный вариант)
		stats.Strength++
		stats.Dexterity++
		stats.Constitution++
		stats.Intelligence++
		stats.Wisdom++
		stats.Charisma++
		// Проверяем максимум для всех
		if stats.Strength > 20 {
			stats.Strength = 20
		}
		if stats.Dexterity > 20 {
			stats.Dexterity = 20
		}
		if stats.Constitution > 20 {
			stats.Constitution = 20
		}
		if stats.Intelligence > 20 {
			stats.Intelligence = 20
		}
		if stats.Wisdom > 20 {
			stats.Wisdom = 20
		}
		if stats.Charisma > 20 {
			stats.Charisma = 20
		}
	case character.RaceOrc:
		// Орки получают +2 к Силе и +1 к Телосложению
		stats.Strength += 2
		stats.Constitution++
		if stats.Strength > 20 {
			stats.Strength = 20
		}
		if stats.Constitution > 20 {
			stats.Constitution = 20
		}
	case character.RaceHalfling:
		// Халфлинги получают +2 к Ловкости
		stats.Dexterity += 2
		if stats.Dexterity > 20 {
			stats.Dexterity = 20
		}
	}
}
