package character

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"

	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/player"
	"dungeons-and-dragons-ai/internal/game/domain/session"
)

type CreateCharacterUseCase struct {
	sessionRepo session.Repository
	playerRepo  PlayerRepository
}

type PlayerRepository interface {
	GetByTgUserIDAndSessionID(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error)
	Save(ctx context.Context, p *player.Player) error
}

func NewCreateCharacterUseCase(
	sessionRepo session.Repository,
	playerRepo PlayerRepository,
) *CreateCharacterUseCase {
	return &CreateCharacterUseCase{
		sessionRepo: sessionRepo,
		playerRepo:  playerRepo,
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

	return p, nil
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
