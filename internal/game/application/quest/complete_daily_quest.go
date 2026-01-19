package quest

import (
	"context"
	"fmt"
	"time"

	characterapp "dungeons-and-dragons-ai/internal/game/application/character"
	"dungeons-and-dragons-ai/internal/game/domain/quest"
	"dungeons-and-dragons-ai/internal/game/domain/session"
)

type CompleteDailyQuestUseCase struct {
	sessionRepo      session.Repository
	dailyQuestRepo   DailyQuestRepository
	playerRepo       PlayerRepository
	addExperienceUC  *characterapp.AddExperienceUseCase
}

func NewCompleteDailyQuestUseCase(
	sessionRepo session.Repository,
	dailyQuestRepo DailyQuestRepository,
	playerRepo PlayerRepository,
	addExperienceUC *characterapp.AddExperienceUseCase,
) *CompleteDailyQuestUseCase {
	return &CompleteDailyQuestUseCase{
		sessionRepo:     sessionRepo,
		dailyQuestRepo:  dailyQuestRepo,
		playerRepo:      playerRepo,
		addExperienceUC: addExperienceUC,
	}
}

// CompleteDailyQuestRequest запрос на завершение ежедневного задания
type CompleteDailyQuestRequest struct {
	ChatID    int64
	TgUserID  int64
	QuestType quest.DailyQuestType
}

// Execute завершает ежедневное задание и выдает награды
func (uc *CompleteDailyQuestUseCase) Execute(
	ctx context.Context,
	req CompleteDailyQuestRequest,
) error {
	// Получаем сессию
	gs, err := uc.sessionRepo.GetByChatID(ctx, req.ChatID)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}

	if gs == nil || !gs.IsActive() {
		return fmt.Errorf("game session not found or not active")
	}

	// Находим игрока
	p, err := uc.playerRepo.GetByTgUserIDAndSessionID(ctx, req.TgUserID, gs.ID)
	if err != nil {
		return fmt.Errorf("failed to get player: %w", err)
	}

	if p == nil {
		return fmt.Errorf("character not created")
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
		return fmt.Errorf("daily quest of type %s not found", req.QuestType)
	}

	// Получаем или создаем прогресс
	now := time.Now()
	progress, err := uc.dailyQuestRepo.GetOrCreateProgress(ctx, p.ID, targetQuest.ID, now)
	if err != nil {
		return fmt.Errorf("failed to get progress: %w", err)
	}

	// Проверяем, не завершено ли уже задание (проверяем только флаг Completed)
	if progress.Completed {
		return nil // Уже завершено, ничего не делаем
	}

	// Проверяем, выполнено ли задание (CurrentValue >= TargetValue)
	if progress.CurrentValue >= progress.TargetValue {
		// Завершаем задание
		progress.Complete()

		// Сохраняем прогресс
		if err := uc.dailyQuestRepo.SaveProgress(ctx, progress); err != nil {
			return fmt.Errorf("failed to save progress: %w", err)
		}

		// Выдаем награды
		if targetQuest.ExperienceReward > 0 {
			expReq := characterapp.AddExperienceRequest{
				ChatID: req.ChatID,
				Amount: targetQuest.ExperienceReward,
				Reason: "daily_quest_completed",
			}
			_, _, err := uc.addExperienceUC.Execute(ctx, expReq)
			if err != nil {
				// Логируем ошибку, но не прерываем выполнение
				// TODO: добавить логирование
			}
		}

		// TODO: Выдача золота (когда будет реализована система золота)
		// if targetQuest.GoldReward > 0 {
		//     // Выдать золото игроку
		// }

		// Обновляем стрик
		if err := uc.updateStreak(ctx, p.ID, now); err != nil {
			// Логируем ошибку, но не прерываем выполнение
			// TODO: добавить логирование
		}
	}

	return nil
}

// updateStreak обновляет стрик игрока
func (uc *CompleteDailyQuestUseCase) updateStreak(ctx context.Context, playerID uint, date time.Time) error {
	streak, err := uc.dailyQuestRepo.GetStreak(ctx, playerID)
	if err != nil {
		return err
	}

	// Проверяем, все ли задания за день выполнены
	progress, err := uc.dailyQuestRepo.GetPlayerProgress(ctx, playerID, date)
	if err != nil {
		return err
	}

	allCompleted := true
	for _, p := range progress {
		if !p.IsCompleted() {
			allCompleted = false
			break
		}
	}

	// Если все задания выполнены, обновляем стрик
	if allCompleted {
		lastDate := streak.LastDate
		startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
		
		// Проверяем, был ли стрик вчера (последовательный день)
		if lastDate.IsZero() {
			// Первый день стрика
			streak.StreakDays = 1
			streak.LastDate = startOfDay
		} else {
			lastStartOfDay := time.Date(lastDate.Year(), lastDate.Month(), lastDate.Day(), 0, 0, 0, 0, lastDate.Location())
			daysDiff := int(startOfDay.Sub(lastStartOfDay).Hours() / 24)
			
			if daysDiff == 1 {
				// Последовательный день
				streak.StreakDays++
				streak.LastDate = startOfDay
			} else if daysDiff > 1 {
				// Стрик прерван, начинаем заново
				streak.StreakDays = 1
				streak.LastDate = startOfDay
			}
			// Если daysDiff == 0, это тот же день, не обновляем
		}

		if err := uc.dailyQuestRepo.UpdateStreak(ctx, streak); err != nil {
			return err
		}
	}

	return nil
}
