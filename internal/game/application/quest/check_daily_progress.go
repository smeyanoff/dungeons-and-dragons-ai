package quest

import (
	"context"
	"fmt"
	"time"

	"dungeons-and-dragons-ai/internal/game/domain/quest"
	"dungeons-and-dragons-ai/internal/game/domain/session"
)

type CheckDailyQuestProgressUseCase struct {
	sessionRepo    session.Repository
	dailyQuestRepo DailyQuestRepository
	playerRepo     PlayerRepository
	completeUC     *CompleteDailyQuestUseCase
}

func NewCheckDailyQuestProgressUseCase(
	sessionRepo session.Repository,
	dailyQuestRepo DailyQuestRepository,
	playerRepo PlayerRepository,
	completeUC *CompleteDailyQuestUseCase,
) *CheckDailyQuestProgressUseCase {
	return &CheckDailyQuestProgressUseCase{
		sessionRepo:    sessionRepo,
		dailyQuestRepo: dailyQuestRepo,
		playerRepo:     playerRepo,
		completeUC:     completeUC,
	}
}

// CheckProgressRequest запрос на проверку прогресса
type CheckProgressRequest struct {
	ChatID   int64
	TgUserID int64
	QuestType quest.DailyQuestType
	Increment int // На сколько увеличить прогресс
}

// Execute проверяет и обновляет прогресс ежедневного задания
func (uc *CheckDailyQuestProgressUseCase) Execute(
	ctx context.Context,
	req CheckProgressRequest,
) error {
	// Получаем сессию
	gs, err := uc.sessionRepo.GetByChatID(ctx, req.ChatID)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}

	if gs == nil || !gs.IsActive() {
		return nil // Игровая сессия не активна, пропускаем
	}

	// Находим игрока
	p, err := uc.playerRepo.GetByTgUserIDAndSessionID(ctx, req.TgUserID, gs.ID)
	if err != nil {
		return fmt.Errorf("failed to get player: %w", err)
	}

	if p == nil {
		return nil // Персонаж не создан, пропускаем
	}

	// Получаем ежедневные задания на сегодня
	dailyQuests, err := uc.dailyQuestRepo.GetTodayQuests(ctx)
	if err != nil {
		return fmt.Errorf("failed to get daily quests: %w", err)
	}

	// Находим задание нужного типа
	var targetQuest *quest.DailyQuest
	for _, dq := range dailyQuests {
		if dq.Type == req.QuestType {
			targetQuest = dq
			break
		}
	}

	if targetQuest == nil {
		return nil // Задание не найдено, пропускаем
	}

	// Получаем или создаем прогресс
	now := time.Now()
	progress, err := uc.dailyQuestRepo.GetOrCreateProgress(ctx, p.ID, targetQuest.ID, now)
	if err != nil {
		return fmt.Errorf("failed to get progress: %w", err)
	}

	// Проверяем, не завершено ли уже задание
	if progress.IsCompleted() {
		return nil // Уже завершено
	}

	// Увеличиваем прогресс
	progress.IncrementProgress(req.Increment)

	// Сохраняем прогресс
	if err := uc.dailyQuestRepo.SaveProgress(ctx, progress); err != nil {
		return fmt.Errorf("failed to save progress: %w", err)
	}

	// Если задание выполнено, завершаем его и выдаем награды
	if progress.IsCompleted() {
		completeReq := CompleteDailyQuestRequest{
			ChatID:    req.ChatID,
			TgUserID:  req.TgUserID,
			QuestType: req.QuestType,
		}
		if err := uc.completeUC.Execute(ctx, completeReq); err != nil {
			// Логируем ошибку, но не прерываем выполнение
			// TODO: добавить логирование
		}
	}

	return nil
}
