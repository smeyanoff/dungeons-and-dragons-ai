package combat

import (
	"context"
	"fmt"

	achievementapp "dungeons-and-dragons-ai/internal/game/application/achievement"
	questapp "dungeons-and-dragons-ai/internal/game/application/quest"
	"dungeons-and-dragons-ai/internal/game/domain/combat"
	"dungeons-and-dragons-ai/internal/game/domain/quest"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/pkg/logger"
)

type HandleCombatUseCase struct {
	combatRepo          CombatRepository
	sessionRepo         session.Repository
	checkAchievementsUC *achievementapp.CheckAchievementsUseCase // Опциональная зависимость для проверки достижений
	notificationService achievementapp.NotificationService        // Опциональная зависимость для отправки уведомлений
	checkDailyProgressUC *questapp.CheckDailyQuestProgressUseCase // Опциональная зависимость для проверки ежедневных заданий
	updateRatingUC      RatingUpdater                             // Опциональная зависимость для обновления рейтингов
}

// RatingUpdater интерфейс для обновления рейтингов
type RatingUpdater interface {
	Execute(ctx context.Context, req RatingUpdateRequest) error
}

// RatingUpdateRequest запрос на обновление рейтинга
type RatingUpdateRequest struct {
	TgUserID int64
	ChatID   int64
}

type CombatRepository interface {
	GetActiveBySessionID(ctx context.Context, sessionID uint) (*combat.Combat, error)
	Save(ctx context.Context, c *combat.Combat) error
}

func NewHandleCombatUseCase(
	combatRepo CombatRepository,
	sessionRepo session.Repository,
) *HandleCombatUseCase {
	return &HandleCombatUseCase{
		combatRepo:  combatRepo,
		sessionRepo: sessionRepo,
	}
}

// SetCheckAchievementsUseCase устанавливает CheckAchievementsUseCase для проверки достижений
func (uc *HandleCombatUseCase) SetCheckAchievementsUseCase(checkAchievementsUC *achievementapp.CheckAchievementsUseCase) {
	uc.checkAchievementsUC = checkAchievementsUC
}

// SetNotificationService устанавливает NotificationService для отправки уведомлений
func (uc *HandleCombatUseCase) SetNotificationService(notificationService achievementapp.NotificationService) {
	uc.notificationService = notificationService
}

// SetCheckDailyProgressUseCase устанавливает CheckDailyQuestProgressUseCase для проверки ежедневных заданий
func (uc *HandleCombatUseCase) SetCheckDailyProgressUseCase(checkDailyProgressUC *questapp.CheckDailyQuestProgressUseCase) {
	uc.checkDailyProgressUC = checkDailyProgressUC
}

// SetRatingUpdater устанавливает RatingUpdater для обновления рейтингов
func (uc *HandleCombatUseCase) SetRatingUpdater(updateRatingUC RatingUpdater) {
	uc.updateRatingUC = updateRatingUC
}

// Execute обрабатывает боевое действие игрока
func (uc *HandleCombatUseCase) Execute(
	ctx context.Context,
	chatID int64,
	action string, // например, "атакую", "бью мечом"
) (string, error) {
	// Получаем сессию
	gs, err := uc.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("failed to get session: %w", err)
	}

	if gs == nil {
		return "Игра не начата. Используйте /newgame для начала новой игры.", nil
	}

	// Получаем активный бой
	activeCombat, err := uc.combatRepo.GetActiveBySessionID(ctx, gs.ID)
	if err != nil {
		return "", fmt.Errorf("failed to get combat: %w", err)
	}

	if activeCombat == nil {
		return "Сейчас нет активного боя.", nil
	}

	// Находим игрока в бою
	var playerParticipant *combat.CombatParticipant
	for i := range activeCombat.Participants {
		if activeCombat.Participants[i].IsPlayer && activeCombat.Participants[i].IsAlive() {
			playerParticipant = &activeCombat.Participants[i]
			break
		}
	}

	if playerParticipant == nil {
		return "Ваш персонаж не участвует в бою или мертв.", nil
	}

	// Находим цель (первого живого врага)
	var targetParticipant *combat.CombatParticipant
	for i := range activeCombat.Participants {
		if !activeCombat.Participants[i].IsPlayer && activeCombat.Participants[i].IsAlive() {
			targetParticipant = &activeCombat.Participants[i]
			break
		}
	}

	if targetParticipant == nil {
		// Все враги мертвы
		activeCombat.State = combat.CombatStateFinished
		if err := uc.combatRepo.Save(ctx, activeCombat); err != nil {
			logger.Error("Failed to save combat state after victory",
				logger.ErrorField(err),
				logger.Uint("session_id", activeCombat.GameSessionID),
			)
			// Продолжаем выполнение, так как бой уже завершен логически
		}
		return "🎉 Все враги побеждены! Бой окончен.", nil
	}

	// Выполняем атаку
	// Для простоты используем стандартный урон по классу
	damageExpression := getDamageByClass(string(playerParticipant.Character.Class))

	result, err := combat.PerformAttack(playerParticipant, targetParticipant, damageExpression)
	if err != nil {
		return "", fmt.Errorf("failed to perform attack: %w", err)
	}

	// Формируем описание результата
	var resultText string
	if result.CriticalHit {
		resultText = "🎯 КРИТИЧЕСКИЙ УДАР!\n"
	}

	if result.Hit {
		resultText += fmt.Sprintf("✅ %s атакует %s и попадает! (бросок: %d против AC %d)\n",
			result.AttackerName, result.TargetName, result.AttackRoll, result.AC)
		resultText += fmt.Sprintf("💥 Урон: %d\n", result.Damage)
		resultText += fmt.Sprintf("❤️ %s: %d/%d HP",
			result.TargetName, targetParticipant.GetHP(), targetParticipant.GetMaxHP())
	} else {
		resultText += fmt.Sprintf("❌ %s атакует %s, но промахивается! (бросок: %d против AC %d)",
			result.AttackerName, result.TargetName, result.AttackRoll, result.AC)
	}

	// Сохраняем состояние боя
	if err := uc.combatRepo.Save(ctx, activeCombat); err != nil {
		return "", fmt.Errorf("failed to save combat: %w", err)
	}

	// Проверяем, не закончился ли бой
	if !targetParticipant.IsAlive() {
		resultText += "\n\n💀 " + targetParticipant.GetName() + " повержен!"
	}

	// Проверяем, не закончился ли бой полностью
	if activeCombat.CheckCombatEnd() {
		activeCombat.State = combat.CombatStateFinished
		if err := uc.combatRepo.Save(ctx, activeCombat); err != nil {
			// Логируем ошибку, но не прерываем выполнение - результат боя уже сформирован
			// Пользователь увидит результат боя, но состояние может быть не сохранено
			resultText += fmt.Sprintf("\n\n⚠️ Бой завершен, но произошла ошибка при сохранении: %v", err)
		} else {
			// Проверяем, кто победил
			alivePlayers := 0
			for _, p := range activeCombat.Participants {
				if p.IsPlayer && p.IsAlive() {
					alivePlayers++
				}
			}

			if alivePlayers > 0 {
				resultText += "\n\n🎉 Победа! Все враги повержены!"
				
				// Находим игрока через сессию для получения playerID и TgUserID
				player := gs.GetFirstPlayer()
				if player != nil {
					// Проверяем достижения по победам в боях
					if uc.checkAchievementsUC != nil {
						// Упрощенное решение: передаем 1, а CheckAchievementsUseCase сам увеличит прогресс
						// CheckAchievementsUseCase автоматически увеличивает существующий прогресс на переданное значение
						achievementReq := achievementapp.CheckAchievementsRequest{
							PlayerID:       player.ID,
							RequirementKey: "combat_wins",
							CurrentValue:   1, // Увеличиваем на 1 победу
						}
						
						unlocked, err := uc.checkAchievementsUC.Execute(ctx, achievementReq)
						if err != nil {
							logger.Warn("Failed to check achievements after combat victory",
								logger.ErrorField(err),
								logger.Uint("player_id", player.ID),
							)
						} else if len(unlocked) > 0 {
							// Логируем и отправляем уведомления о разблокированных достижениях
							for _, achievement := range unlocked {
								logger.Info("Achievement unlocked after combat victory",
									logger.Uint("player_id", player.ID),
									logger.String("achievement_code", achievement.Achievement.Code),
									logger.String("achievement_title", achievement.Achievement.Title),
								)
								// Добавляем уведомление о разблокированном достижении в результат
								resultText += fmt.Sprintf("\n\n🏆 %s", achievement.Message)
								
								// Отправляем уведомление пользователю через notification service (если есть)
								if uc.notificationService != nil {
									if err := uc.notificationService.SendAchievementNotification(ctx, chatID, achievement.Message); err != nil {
										logger.Warn("Failed to send achievement notification",
											logger.ErrorField(err),
											logger.Uint("player_id", player.ID),
											logger.String("achievement_code", achievement.Achievement.Code),
										)
									}
								}
							}
						}
					}
					
					// Проверяем прогресс ежедневных заданий по победам в боях
					if uc.checkDailyProgressUC != nil {
						dailyProgressReq := questapp.CheckProgressRequest{
							ChatID:    chatID,
							TgUserID:  player.TgUserID,
							QuestType: quest.DailyQuestTypeWinCombat,
							Increment: 1, // Увеличиваем прогресс на 1 победу
						}
						
						if err := uc.checkDailyProgressUC.Execute(ctx, dailyProgressReq); err != nil {
							logger.Warn("Failed to check daily quest progress after combat victory",
								logger.ErrorField(err),
								logger.Uint("player_id", player.ID),
								logger.Int64("tg_user_id", player.TgUserID),
							)
						} else {
							logger.Debug("Daily quest progress checked after combat victory",
								logger.Uint("player_id", player.ID),
								logger.Int64("tg_user_id", player.TgUserID),
							)
						}
					}
					
					// Обновляем рейтинг игрока после победы в бою
					if uc.updateRatingUC != nil {
						ratingReq := RatingUpdateRequest{
							TgUserID: player.TgUserID,
							ChatID:   chatID,
						}
						if err := uc.updateRatingUC.Execute(ctx, ratingReq); err != nil {
							logger.Warn("Failed to update rating after combat victory",
								logger.ErrorField(err),
								logger.Uint("player_id", player.ID),
								logger.Int64("tg_user_id", player.TgUserID),
							)
						}
					}
				}
			} else {
				resultText += "\n\n💀 Поражение! Все игроки повержены!"
			}
		}
	}

	return resultText, nil
}

// getDamageByClass возвращает выражение урона по классу
func getDamageByClass(class string) string {
	damageMap := map[string]string{
		"fighter": "1d8", // Меч
		"wizard":  "1d6", // Магический урон
		"rogue":   "1d6", // Кинжал
		"cleric":  "1d8", // Булава
		"ranger":  "1d8", // Лук
	}

	if dmg, ok := damageMap[class]; ok {
		return dmg
	}
	return "1d6" // По умолчанию
}
