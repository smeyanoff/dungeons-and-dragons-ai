package history

import (
	"context"
	"fmt"
	"strings"

	"dungeons-and-dragons-ai/internal/game/domain/event"
	"dungeons-and-dragons-ai/internal/game/domain/session"
)

type GetHistoryUseCase struct {
	sessionRepo session.Repository
	eventRepo   EventRepository
}

type EventRepository interface {
	GetBySessionID(ctx context.Context, sessionID uint, limit int) ([]event.StoryEvent, error)
}

func NewGetHistoryUseCase(
	sessionRepo session.Repository,
	eventRepo EventRepository,
) *GetHistoryUseCase {
	return &GetHistoryUseCase{
		sessionRepo: sessionRepo,
		eventRepo:   eventRepo,
	}
}

func (uc *GetHistoryUseCase) Execute(
	ctx context.Context,
	chatID int64,
	limit int,
) (string, error) {
	// Получаем сессию
	gs, err := uc.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("failed to get session: %w", err)
	}

	if gs == nil {
		return "Игра не начата. Используйте /newgame для начала новой игры.", nil
	}

	// Получаем события
	events, err := uc.eventRepo.GetBySessionID(ctx, gs.ID, limit)
	if err != nil {
		return "", fmt.Errorf("failed to get events: %w", err)
	}

	if len(events) == 0 {
		return "История игры пуста.", nil
	}

	// Формируем текст истории
	var parts []string
	parts = append(parts, "📜 История игры:\n")

	for _, e := range events {
		var authorPrefix string
		switch e.AuthorType {
		case event.AuthorTypePlayer:
			authorPrefix = "👤 Игрок"
		case event.AuthorTypeDM:
			authorPrefix = "🎲 DM"
		case event.AuthorTypeNPC:
			authorPrefix = "🗣️ " + e.AuthorName
		default:
			authorPrefix = "❓"
		}

		parts = append(parts, fmt.Sprintf("%s: %s", authorPrefix, e.Content))
	}

	return strings.Join(parts, "\n\n"), nil
}
