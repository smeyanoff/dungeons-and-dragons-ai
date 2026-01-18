package achievement

import (
	"context"
	"fmt"
	"time"

	"dungeons-and-dragons-ai/internal/game/domain/achievement"
	"dungeons-and-dragons-ai/internal/game/infrastructure/persistence"
)

type CheckAchievementsUseCase struct {
	achievementRepo *persistence.AchievementRepository
	playerRepo      *persistence.PlayerRepository
}

func NewCheckAchievementsUseCase(
	achievementRepo *persistence.AchievementRepository,
	playerRepo *persistence.PlayerRepository,
) *CheckAchievementsUseCase {
	return &CheckAchievementsUseCase{
		achievementRepo: achievementRepo,
		playerRepo:      playerRepo,
	}
}

type CheckAchievementsRequest struct {
	PlayerID      uint
	RequirementKey string
	CurrentValue   int
}

type AchievementUnlocked struct {
	Achievement *achievement.Achievement
	Message     string
}

func (uc *CheckAchievementsUseCase) Execute(ctx context.Context, req CheckAchievementsRequest) ([]*AchievementUnlocked, error) {
	var unlocked []*AchievementUnlocked
	
	// Получаем все достижения с указанным ключом требования
	allAchievements, err := uc.achievementRepo.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get achievements: %w", err)
	}
	
	// Фильтруем достижения по ключу требования
	var relevantAchievements []*achievement.Achievement
	for _, a := range allAchievements {
		if a.RequirementKey == req.RequirementKey {
			relevantAchievements = append(relevantAchievements, a)
		}
	}
	
	// Проверяем каждое достижение
	for _, a := range relevantAchievements {
		// Проверяем, получено ли уже это достижение
		existing, err := uc.achievementRepo.GetPlayerAchievementByCode(ctx, req.PlayerID, a.Code)
		if err != nil {
			return nil, fmt.Errorf("failed to check player achievement: %w", err)
		}
		
		// Если достижение не повторяемое и уже получено, пропускаем
		if existing != nil && !a.IsRepeatable {
			continue
		}
		
		// Получаем текущий прогресс
		progress, err := uc.achievementRepo.GetAchievementProgress(ctx, req.PlayerID, a.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get achievement progress: %w", err)
		}
		
		// Вычисляем новое значение прогресса
		// Если прогресс есть, увеличиваем на переданное значение, иначе используем переданное значение
		newValue := req.CurrentValue
		if progress != nil {
			// Если текущее значение меньше переданного, используем переданное
			// Это позволяет как увеличивать прогресс, так и устанавливать его напрямую
			if req.CurrentValue > 0 {
				// Если передано положительное значение, увеличиваем прогресс
				// Для достижений типа "combat_wins" это будет увеличивать счетчик побед
				newValue = progress.CurrentValue + req.CurrentValue
			} else {
				// Если передано 0 или отрицательное, используем текущее значение
				newValue = progress.CurrentValue
			}
		}
		
		// Проверяем, выполнено ли условие с новым значением
		if a.IsCompleted(newValue) {
			// Обновляем или создаем прогресс
			if progress == nil {
				progress = &achievement.AchievementProgress{
					PlayerID:      req.PlayerID,
					AchievementID: a.ID,
					CurrentValue:  newValue,
					IsCompleted:   true,
				}
			} else {
				progress.CurrentValue = newValue
				progress.IsCompleted = true
			}
			
			if err := uc.achievementRepo.SaveAchievementProgress(ctx, progress); err != nil {
				return nil, fmt.Errorf("failed to save achievement progress: %w", err)
			}
			
			// Если достижение еще не получено, создаем его
			if existing == nil {
				playerAchievement := &achievement.PlayerAchievement{
					PlayerID:      req.PlayerID,
					AchievementID: a.ID,
					Progress:      newValue,
					EarnedAt:      time.Now(),
					EarnedCount:   1,
				}
				
				if err := uc.achievementRepo.SavePlayerAchievement(ctx, playerAchievement); err != nil {
					return nil, fmt.Errorf("failed to save player achievement: %w", err)
				}
				
				// Формируем сообщение о разблокировке
				icon := a.Icon
				if icon == "" {
					icon = "🏆"
				}
				message := fmt.Sprintf("🎉 Достижение разблокировано!\n\n%s %s\n%s\n\nНаграда: +%d опыта", icon, a.Title, a.Description, a.ExperienceReward)
				if a.GoldReward > 0 {
					message += fmt.Sprintf(", +%d золота", a.GoldReward)
				}
				
				unlocked = append(unlocked, &AchievementUnlocked{
					Achievement: a,
					Message:     message,
				})
			} else if a.IsRepeatable {
				// Для повторяемых достижений увеличиваем счетчик
				existing.EarnedCount++
				existing.Progress = newValue
				if err := uc.achievementRepo.SavePlayerAchievement(ctx, existing); err != nil {
					return nil, fmt.Errorf("failed to update player achievement: %w", err)
				}
			}
		} else {
			// Обновляем прогресс, даже если достижение еще не получено
			// progress уже получен выше, просто обновляем его
			if progress == nil {
				progress = &achievement.AchievementProgress{
					PlayerID:      req.PlayerID,
					AchievementID: a.ID,
					CurrentValue:  newValue,
					IsCompleted:   false,
				}
			} else {
				progress.CurrentValue = newValue
				progress.IsCompleted = false
			}
			
			if err := uc.achievementRepo.SaveAchievementProgress(ctx, progress); err != nil {
				return nil, fmt.Errorf("failed to save achievement progress: %w", err)
			}
		}
	}
	
	return unlocked, nil
}
