package quest

import (
	"context"
	"fmt"
	"strings"

	"dungeons-and-dragons-ai/internal/game/domain/quest"
	"dungeons-and-dragons-ai/internal/game/domain/session"
)

type QuestRepository interface {
	GetByWorldID(ctx context.Context, worldID uint) ([]*quest.Quest, error)
	Save(ctx context.Context, q *quest.Quest) error
}

type GetQuestsUseCase struct {
	sessionRepo session.Repository
	questRepo   QuestRepository
}

func NewGetQuestsUseCase(sessionRepo session.Repository, questRepo QuestRepository) *GetQuestsUseCase {
	return &GetQuestsUseCase{
		sessionRepo: sessionRepo,
		questRepo:   questRepo,
	}
}

func (uc *GetQuestsUseCase) Execute(
	ctx context.Context,
	chatID int64,
) (string, error) {
	// Получаем сессию
	gs, err := uc.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("failed to get session: %w", err)
	}

	if gs == nil {
		return "", fmt.Errorf("game session not found, use /newgame first")
	}

	if !gs.IsActive() {
		return "", fmt.Errorf("game session is not active")
	}

	// Получаем главный квест из мира
	if gs.World.MainQuest == nil {
		return "Нет активных квестов.", nil
	}

	var quests []*quest.Quest
	mainQuest := gs.World.MainQuest

	// Добавляем главный квест
	quests = append(quests, mainQuest)

	// Получаем дополнительные квесты из мира (если будут)
	// Пока используем только главный квест

	var result strings.Builder
	result.WriteString("📜 Активные квесты:\n\n")

	for i, q := range quests {
		if !q.IsActive() {
			continue
		}

		statusEmoji := "🟢"
		if q.Status == quest.QuestStatusCompleted {
			statusEmoji = "✅"
		} else if q.Status == quest.QuestStatusFailed {
			statusEmoji = "❌"
		}

		result.WriteString(fmt.Sprintf("%s %d. %s\n", statusEmoji, i+1, q.Title))
		result.WriteString(fmt.Sprintf("   %s\n", q.Description))

		if q.ExperienceReward > 0 {
			result.WriteString(fmt.Sprintf("   ⭐ Награда: %d опыта\n", q.ExperienceReward))
		}

		if len(q.Items) > 0 {
			result.WriteString("   🎁 Предметы:\n")
			for _, item := range q.Items {
				result.WriteString(fmt.Sprintf("      - %s (%s)\n", item.Name, item.Purpose))
			}
		}

		result.WriteString("\n")
	}

	if result.Len() == len("📜 Активные квесты:\n\n") {
		return "Нет активных квестов.", nil
	}

	return result.String(), nil
}
