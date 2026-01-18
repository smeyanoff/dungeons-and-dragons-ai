package character

import (
	"context"
	"fmt"

	"dungeons-and-dragons-ai/internal/game/domain/player"
	"dungeons-and-dragons-ai/internal/game/domain/session"
)

type AddExperienceUseCase struct {
	playerRepo  PlayerRepository
	sessionRepo session.Repository
}

func NewAddExperienceUseCase(playerRepo PlayerRepository, sessionRepo session.Repository) *AddExperienceUseCase {
	return &AddExperienceUseCase{
		playerRepo:  playerRepo,
		sessionRepo: sessionRepo,
	}
}

type AddExperienceRequest struct {
	ChatID int64
	Amount int
	Reason string // Причина начисления опыта (например, "quest_completed", "combat_victory")
}

func (uc *AddExperienceUseCase) Execute(
	ctx context.Context,
	req AddExperienceRequest,
) (*player.Player, bool, error) {
	// Получаем сессию
	gs, err := uc.sessionRepo.GetByChatID(ctx, req.ChatID)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get session: %w", err)
	}

	if gs == nil {
		return nil, false, fmt.Errorf("game session not found")
	}

	// Получаем игрока
	p, err := uc.playerRepo.GetByTgUserIDAndSessionID(ctx, req.ChatID, gs.Model.ID)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get player: %w", err)
	}

	if p == nil {
		return nil, false, fmt.Errorf("player not found, create character first")
	}

	// Начисляем опыт
	leveledUp, err := p.Character.AddExperience(req.Amount)
	if err != nil {
		return nil, false, fmt.Errorf("failed to add experience: %w", err)
	}

	// Сохраняем изменения
	if err := uc.playerRepo.Save(ctx, p); err != nil {
		return nil, false, fmt.Errorf("failed to save player: %w", err)
	}

	return p, leveledUp, nil
}
