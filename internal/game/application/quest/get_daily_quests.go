package quest

import (
	"context"
	"fmt"
	"strings"
	"time"

	"dungeons-and-dragons-ai/internal/game/domain/player"
	"dungeons-and-dragons-ai/internal/game/domain/quest"
	"dungeons-and-dragons-ai/internal/game/domain/session"
)

type DailyQuestRepository interface {
	GetTodayQuests(ctx context.Context) ([]*quest.DailyQuest, error)
	GetPlayerProgress(ctx context.Context, playerID uint, date time.Time) ([]*quest.DailyQuestProgress, error)
	GetStreak(ctx context.Context, playerID uint) (*quest.DailyQuestStreak, error)
	GetOrCreateProgress(ctx context.Context, playerID uint, dailyQuestID uint, date time.Time) (*quest.DailyQuestProgress, error)
	SaveProgress(ctx context.Context, progress *quest.DailyQuestProgress) error
	UpdateStreak(ctx context.Context, streak *quest.DailyQuestStreak) error
}

type GetDailyQuestsUseCase struct {
	sessionRepo      session.Repository
	dailyQuestRepo   DailyQuestRepository
	playerRepo       PlayerRepository
}

type PlayerRepository interface {
	GetByTgUserIDAndSessionID(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error)
}

func NewGetDailyQuestsUseCase(
	sessionRepo session.Repository,
	dailyQuestRepo DailyQuestRepository,
	playerRepo PlayerRepository,
) *GetDailyQuestsUseCase {
	return &GetDailyQuestsUseCase{
		sessionRepo:    sessionRepo,
		dailyQuestRepo: dailyQuestRepo,
		playerRepo:      playerRepo,
	}
}

func (uc *GetDailyQuestsUseCase) Execute(
	ctx context.Context,
	chatID int64,
	tgUserID int64,
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

	// Находим игрока
	p, err := uc.playerRepo.GetByTgUserIDAndSessionID(ctx, tgUserID, gs.ID)
	if err != nil {
		return "", fmt.Errorf("failed to get player: %w", err)
	}

	if p == nil {
		return "", fmt.Errorf("character not created, use /createcharacter first")
	}

	// Получаем ежедневные задания на сегодня
	dailyQuests, err := uc.dailyQuestRepo.GetTodayQuests(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get daily quests: %w", err)
	}

	// Получаем прогресс игрока
	now := time.Now()
	progress, err := uc.dailyQuestRepo.GetPlayerProgress(ctx, p.ID, now)
	if err != nil {
		return "", fmt.Errorf("failed to get progress: %w", err)
	}

	// Создаем map для быстрого поиска прогресса по ID задания
	progressMap := make(map[uint]*quest.DailyQuestProgress)
	for i := range progress {
		progressMap[progress[i].DailyQuestID] = progress[i]
	}

	// Получаем стрик игрока
	streak, err := uc.dailyQuestRepo.GetStreak(ctx, p.ID)
	if err != nil {
		return "", fmt.Errorf("failed to get streak: %w", err)
	}

	var result strings.Builder
	result.WriteString("📅 Ежедневные задания\n\n")

	// Отображаем стрик
	if streak.StreakDays > 0 {
		result.WriteString(fmt.Sprintf("🔥 Стрик: %d дней подряд\n\n", streak.StreakDays))
	}

	// Отображаем задания с прогрессом
	for i, dq := range dailyQuests {
		prog, hasProgress := progressMap[dq.ID]

		statusEmoji := "⚪"
		progressText := "0"
		targetText := fmt.Sprintf("%d", dq.TargetValue)

		if hasProgress {
			if prog.IsCompleted() {
				statusEmoji = "✅"
				progressText = fmt.Sprintf("%d", prog.TargetValue)
			} else {
				statusEmoji = "🔄"
				progressText = fmt.Sprintf("%d", prog.CurrentValue)
			}
		}

		result.WriteString(fmt.Sprintf("%s %d. %s\n", statusEmoji, i+1, dq.Title))
		result.WriteString(fmt.Sprintf("   %s\n", dq.Description))
		result.WriteString(fmt.Sprintf("   Прогресс: %s/%s\n", progressText, targetText))

		if dq.ExperienceReward > 0 || dq.GoldReward > 0 {
			rewards := []string{}
			if dq.ExperienceReward > 0 {
				rewards = append(rewards, fmt.Sprintf("%d опыта", dq.ExperienceReward))
			}
			if dq.GoldReward > 0 {
				rewards = append(rewards, fmt.Sprintf("%d золота", dq.GoldReward))
			}
			result.WriteString(fmt.Sprintf("   ⭐ Награда: %s\n", strings.Join(rewards, ", ")))
		}

		result.WriteString("\n")
	}

	// Информация о еженедельном бонусе
	result.WriteString("💎 Еженедельный бонус: Выполните все задания за неделю для получения дополнительных наград!\n")

	return result.String(), nil
}
