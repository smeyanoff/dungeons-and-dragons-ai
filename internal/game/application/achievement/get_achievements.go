package achievement

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"dungeons-and-dragons-ai/internal/game/domain/achievement"
	"dungeons-and-dragons-ai/internal/game/domain/session"
)

type GetAchievementsUseCase struct {
	achievementRepo AchievementRepository
	sessionRepo     session.Repository
}

func NewGetAchievementsUseCase(
	achievementRepo AchievementRepository,
	sessionRepo session.Repository,
) *GetAchievementsUseCase {
	return &GetAchievementsUseCase{
		achievementRepo: achievementRepo,
		sessionRepo:     sessionRepo,
	}
}

type GetAchievementsRequest struct {
	ChatID   int64
	TgUserID int64
}

func (uc *GetAchievementsUseCase) Execute(ctx context.Context, req GetAchievementsRequest) (string, error) {
	// Получаем сессию
	gs, err := uc.sessionRepo.GetByChatID(ctx, req.ChatID)
	if err != nil {
		return "", fmt.Errorf("failed to get session: %w", err)
	}

	if gs == nil {
		return "Игра не начата. Используйте /newgame для начала новой игры.", nil
	}

	// Ищем игрока по TgUserID
	player := gs.FindPlayerByTgUserID(req.TgUserID)
	if player == nil {
		// Fallback: используем первого игрока для обратной совместимости
		player = gs.GetFirstPlayer()
		if player == nil {
			return "Персонаж не создан. Используйте /createcharacter для создания персонажа.", nil
		}
	}

	// Получаем все достижения игрока
	playerAchievements, err := uc.achievementRepo.GetPlayerAchievements(ctx, player.ID)
	if err != nil {
		return "", fmt.Errorf("failed to get player achievements: %w", err)
	}

	// Получаем все доступные достижения для отображения прогресса
	allAchievements, err := uc.achievementRepo.GetAll(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get all achievements: %w", err)
	}

	// Создаем карту полученных достижений для быстрого поиска
	earnedAchievements := make(map[uint]*achievement.PlayerAchievement)
	for _, pa := range playerAchievements {
		earnedAchievements[pa.AchievementID] = pa
	}

	// Получаем прогресс для всех достижений
	achievementsWithProgress := make([]*achievementInfo, 0, len(allAchievements))
	for _, a := range allAchievements {
		// Получаем прогресс достижения
		progress, err := uc.achievementRepo.GetAchievementProgress(ctx, player.ID, a.ID)
		if err != nil {
			// Игнорируем ошибки получения прогресса
			progress = nil
		}

		currentValue := 0
		isEarned := false
		earnedAt := ""
		if earned, exists := earnedAchievements[a.ID]; exists {
			isEarned = true
			currentValue = earned.Progress
			earnedAt = earned.EarnedAt.Format("02.01.2006")
		} else if progress != nil {
			currentValue = progress.CurrentValue
		}

		achievementsWithProgress = append(achievementsWithProgress, &achievementInfo{
			Achievement:  a,
			CurrentValue: currentValue,
			IsEarned:     isEarned,
			EarnedAt:     earnedAt,
		})
	}

	// Группируем по типу
	achievementsByType := make(map[achievement.AchievementType][]*achievementInfo)
	for _, ai := range achievementsWithProgress {
		achievementsByType[ai.Achievement.Type] = append(achievementsByType[ai.Achievement.Type], ai)
	}

	// Формируем текст
	var sb strings.Builder
	sb.WriteString("🏆 Ваши достижения\n\n")

	earnedCount := len(playerAchievements)
	sb.WriteString(fmt.Sprintf("Получено: %d из %d достижений\n\n", earnedCount, len(allAchievements)))

	// Определяем порядок типов для отображения
	typeOrder := []achievement.AchievementType{
		achievement.AchievementTypeProgress,
		achievement.AchievementTypeCombat,
		achievement.AchievementTypeQuest,
		achievement.AchievementTypeExploration,
		achievement.AchievementTypeCollection,
		achievement.AchievementTypeSpecial,
	}

	typeNames := map[achievement.AchievementType]string{
		achievement.AchievementTypeProgress:    "📊 Прогресс",
		achievement.AchievementTypeCombat:      "⚔️ Бой",
		achievement.AchievementTypeQuest:       "📜 Квесты",
		achievement.AchievementTypeExploration: "🗺️ Исследование",
		achievement.AchievementTypeCollection:  "🎒 Коллекции",
		achievement.AchievementTypeSpecial:     "⭐ Особые",
	}

	rarityIcons := map[achievement.Rarity]string{
		achievement.RarityCommon:    "⚪",
		achievement.RarityUncommon:  "🟢",
		achievement.RarityRare:      "🔵",
		achievement.RarityEpic:      "🟣",
		achievement.RarityLegendary: "🟠",
	}

	for _, achievementType := range typeOrder {
		achievements := achievementsByType[achievementType]
		if len(achievements) == 0 {
			continue
		}

		// Сортируем по редкости
		sort.Slice(achievements, func(i, j int) bool {
			rarityOrder := map[achievement.Rarity]int{
				achievement.RarityCommon:    1,
				achievement.RarityUncommon:  2,
				achievement.RarityRare:      3,
				achievement.RarityEpic:      4,
				achievement.RarityLegendary: 5,
			}
			return rarityOrder[achievements[i].Achievement.Rarity] > rarityOrder[achievements[j].Achievement.Rarity]
		})

		sb.WriteString(fmt.Sprintf("%s:\n", typeNames[achievementType]))
		for _, ai := range achievements {
			rarityIcon := rarityIcons[ai.Achievement.Rarity]
			icon := ai.Achievement.Icon
			if icon == "" {
				icon = "🏆"
			}

			status := "❌"
			progressText := ""
			if ai.IsEarned {
				status = "✅"
				progressText = fmt.Sprintf(" (Получено: %s)", ai.EarnedAt)
			} else if ai.Achievement.RequirementValue > 0 {
				percentage := ai.Achievement.GetProgressPercentage(ai.CurrentValue)
				progressText = fmt.Sprintf(" (%d/%d - %d%%)", ai.CurrentValue, ai.Achievement.RequirementValue, percentage)
			}

			// Для скрытых достижений, которые еще не получены, показываем только тип
			if ai.Achievement.IsHidden && !ai.IsEarned {
				sb.WriteString(fmt.Sprintf("  %s %s %s ??? - ???\n", rarityIcon, icon, status))
			} else {
				sb.WriteString(fmt.Sprintf("  %s %s %s %s - %s%s\n",
					rarityIcon, icon, status, ai.Achievement.Title, ai.Achievement.Description, progressText))
			}
		}
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

type achievementInfo struct {
	Achievement  *achievement.Achievement
	CurrentValue int
	IsEarned     bool
	EarnedAt     string
}
