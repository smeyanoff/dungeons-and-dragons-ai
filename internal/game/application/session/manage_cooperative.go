package session

import (
	"context"
	"fmt"

	"dungeons-and-dragons-ai/internal/game/domain/player"
	"dungeons-and-dragons-ai/pkg/logger"
)

// ManageCooperativeUseCase управляет cooperative режимом игры
type ManageCooperativeUseCase struct {
	sessionRepo Repository
	playerRepo  PlayerRepository
}

// PlayerRepository интерфейс для работы с игроками
type PlayerRepository interface {
	GetByTgUserID(ctx context.Context, tgUserID int64) (*player.Player, error)
	Save(ctx context.Context, p *player.Player) error
}

func NewManageCooperativeUseCase(sessionRepo Repository, playerRepo PlayerRepository) *ManageCooperativeUseCase {
	return &ManageCooperativeUseCase{
		sessionRepo: sessionRepo,
		playerRepo:  playerRepo,
	}
}

// EnableCooperativeRequest запрос на включение cooperative режима
type EnableCooperativeRequest struct {
	ChatID     int64
	MaxPlayers int
}

// EnableCooperativeMode включает cooperative режим для сессии
func (uc *ManageCooperativeUseCase) EnableCooperativeMode(ctx context.Context, req EnableCooperativeRequest) error {
	if uc.sessionRepo == nil {
		return fmt.Errorf("session repository is not initialized")
	}
	gs, err := uc.sessionRepo.GetByChatID(ctx, req.ChatID)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}

	if gs == nil {
		return fmt.Errorf("session not found")
	}

	if !gs.IsActive() {
		return fmt.Errorf("session is not active")
	}

	if len(gs.Players) > req.MaxPlayers {
		return fmt.Errorf("too many players in session (%d), max allowed: %d", len(gs.Players), req.MaxPlayers)
	}

	gs.EnableCooperativeMode(req.MaxPlayers)

	if err := uc.sessionRepo.Save(ctx, gs); err != nil {
		return fmt.Errorf("failed to save session: %w", err)
	}

	logger.Info("Cooperative mode enabled",
		logger.Int64("chat_id", req.ChatID),
		logger.Int("max_players", req.MaxPlayers),
	)

	return nil
}

// JoinCooperativeSessionRequest запрос на присоединение к cooperative сессии
type JoinCooperativeSessionRequest struct {
	ChatID   int64
	TgUserID int64
}

// JoinCooperativeSession присоединяет игрока к cooperative сессии
func (uc *ManageCooperativeUseCase) JoinCooperativeSession(ctx context.Context, req JoinCooperativeSessionRequest) error {
	if uc.sessionRepo == nil {
		return fmt.Errorf("session repository is not initialized")
	}
	if uc.playerRepo == nil {
		return fmt.Errorf("player repository is not initialized")
	}
	gs, err := uc.sessionRepo.GetByChatID(ctx, req.ChatID)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}

	if gs == nil {
		return fmt.Errorf("session not found")
	}

	if !gs.IsCooperative {
		return fmt.Errorf("session is not in cooperative mode")
	}

	if !gs.IsActive() {
		return fmt.Errorf("session is not active")
	}

	if len(gs.Players) >= gs.MaxPlayers {
		return fmt.Errorf("session is full (%d/%d players)", len(gs.Players), gs.MaxPlayers)
	}

	// Получаем или создаем игрока
	p, err := uc.playerRepo.GetByTgUserID(ctx, req.TgUserID)
	if err != nil {
		return fmt.Errorf("failed to get player: %w", err)
	}

	if p == nil {
		return fmt.Errorf("player not found, create character first")
	}

	// Добавляем игрока в сессию
	if err := gs.AddPlayerToSession(p); err != nil {
		return fmt.Errorf("failed to add player to session: %w", err)
	}

	if err := uc.sessionRepo.Save(ctx, gs); err != nil {
		return fmt.Errorf("failed to save session: %w", err)
	}

	logger.Info("Player joined cooperative session",
		logger.Int64("chat_id", req.ChatID),
		logger.Int64("tg_user_id", req.TgUserID),
		logger.Uint("player_id", p.ID),
		logger.Int("players_in_session", len(gs.Players)),
	)

	return nil
}

// GetCooperativeStatusResponse ответ со статусом cooperative сессии
type GetCooperativeStatusResponse struct {
	IsCooperative  bool
	MaxPlayers     int
	CurrentPlayers int
	ActivePlayer   *PlayerDTO
	Players        []PlayerDTO
}

// PlayerDTO DTO для игрока
type PlayerDTO struct {
	ID       uint
	TgUserID int64
	Name     string
	IsActive bool
}

// GetCooperativeStatus получает статус cooperative сессии
func (uc *ManageCooperativeUseCase) GetCooperativeStatus(ctx context.Context, chatID int64) (*GetCooperativeStatusResponse, error) {
	if uc.sessionRepo == nil {
		return nil, fmt.Errorf("session repository is not initialized")
	}
	gs, err := uc.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	if gs == nil {
		return nil, fmt.Errorf("session not found")
	}

	response := &GetCooperativeStatusResponse{
		IsCooperative:  gs.IsCooperative,
		MaxPlayers:     gs.MaxPlayers,
		CurrentPlayers: len(gs.Players),
		Players:        make([]PlayerDTO, 0, len(gs.Players)),
	}

	activePlayer := gs.GetActivePlayer()
	for _, p := range gs.Players {
		playerDTO := PlayerDTO{
			ID:       p.ID,
			TgUserID: p.TgUserID,
			Name:     p.Name,
			IsActive: activePlayer != nil && activePlayer.ID == p.ID,
		}
		response.Players = append(response.Players, playerDTO)

		if playerDTO.IsActive {
			response.ActivePlayer = &playerDTO
		}
	}

	return response, nil
}
