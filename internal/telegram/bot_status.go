package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	inventoryapp "dungeons-and-dragons-ai/internal/game/application/inventory"
	"dungeons-and-dragons-ai/internal/game/domain/event"
	ragdomain "dungeons-and-dragons-ai/internal/rag/domain"
	"dungeons-and-dragons-ai/pkg/logger"

	"github.com/google/uuid"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (b *Bot) handleShowCharacter(ctx context.Context, chatID int64, tgUserID int64) error {
	// Получаем сессию
	gs, err := b.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}

	if gs == nil {
		msg := tgbotapi.NewMessage(chatID, "Игра не начата. Используйте /newgame для начала новой игры.")
		return b.sendMessage(msg)
	}

	// Ищем игрока по TgUserID (для приватных чатов chatID == tgUserID)
	// В групповых чатах это позволит найти правильного игрока
	player := gs.FindPlayerByTgUserID(tgUserID)
	if player == nil {
		// Fallback: используем первого игрока для обратной совместимости
		player = gs.GetFirstPlayer()
		if player == nil {
			msg := tgbotapi.NewMessage(chatID, "Персонаж не создан. Используйте /createcharacter для создания персонажа.")
			return b.sendMessage(msg)
		}
	}
	char := player.Character

	// Рассчитываем опыт до следующего уровня
	expToNext := char.GetExperienceToNextLevel()

	charText := fmt.Sprintf(`👤 Персонаж: %s

🏛️ Раса: %s
⚔️ Класс: %s
📊 Уровень: %d
⭐ Опыт: %d / %d (до следующего уровня: %d)
❤️ HP: %d/%d
💀 Статус: %s

📈 Характеристики:
💪 Сила: %d
🏃 Ловкость: %d
🛡️ Телосложение: %d
🧠 Интеллект: %d
🔮 Мудрость: %d
💬 Харизма: %d`,
		char.Name,
		char.Race,
		char.Class,
		char.Level,
		char.Experience,
		char.GetExperienceToNextLevel()+char.Experience,
		expToNext,
		char.HP,
		char.MaxHP,
		char.Status,
		char.Stats.Strength,
		char.Stats.Dexterity,
		char.Stats.Constitution,
		char.Stats.Intelligence,
		char.Stats.Wisdom,
		char.Stats.Charisma,
	)

	return b.sendLongMessage(chatID, charText)
}

func (b *Bot) handleHistory(ctx context.Context, chatID int64, args string) error {
	limit := 10 // по умолчанию последние 10 событий
	if args != "" {
		// Можно добавить парсинг лимита из args
		limit = 10
	}

	historyText, err := b.getHistoryUC.Execute(ctx, chatID, limit)
	if err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при получении истории: %v", err))
		return b.sendMessage(errorMsg)
	}

	return b.sendLongMessage(chatID, historyText)
}

func (b *Bot) handleInventory(ctx context.Context, chatID int64, tgUserID int64) error {
	inventoryText, err := b.getInventoryUC.Execute(ctx, chatID, tgUserID)
	if err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при получении инвентаря: %v", err))
		return b.sendMessage(errorMsg)
	}

	return b.sendLongMessage(chatID, inventoryText)
}

func (b *Bot) handlePickup(ctx context.Context, chatID int64, args string, tgUserID int64) error {
	// Парсим аргументы: /pickup <предмет> [количество]
	parts := strings.Fields(args)

	if len(parts) == 0 {
		msg := tgbotapi.NewMessage(chatID, "Укажите название предмета. Формат: /pickup <предмет> [количество]\n\nПример: /pickup меч\nПример: /pickup стрела 5")
		return b.sendMessage(msg)
	}

	// Пытаемся определить количество (последняя часть)
	quantity := 1
	itemName := ""

	if len(parts) > 1 {
		// Проверяем, является ли последняя часть числом
		if qty, err := strconv.Atoi(parts[len(parts)-1]); err == nil && qty > 0 {
			// Последняя часть - количество
			quantity = qty
			// Имя - все части кроме последней
			itemName = strings.Join(parts[:len(parts)-1], " ")
		} else {
			// Последняя часть не число - все части это название предмета
			itemName = strings.Join(parts, " ")
		}
	} else {
		// Только одна часть - это название предмета
		itemName = parts[0]
	}

	if itemName == "" {
		msg := tgbotapi.NewMessage(chatID, "Укажите название предмета. Формат: /pickup <предмет> [количество]")
		return b.sendMessage(msg)
	}

	req := inventoryapp.AddItemRequest{
		ChatID:   chatID,
		TgUserID: tgUserID,
		ItemName: itemName,
		Quantity: quantity,
		ItemType: "", // Определяется автоматически по названию
	}

	result, err := b.addItemUC.Execute(ctx, req)
	if err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при добавлении предмета: %v", err))
		return b.sendMessage(errorMsg)
	}

	return b.sendLongMessage(chatID, result)
}

func (b *Bot) handleRoll(ctx context.Context, chatID int64, args string) error {
	// Очищаем аргументы от лишних символов (обратные апострофы, кавычки и т.д.)
	cleanedArgs := strings.TrimSpace(args)
	cleanedArgs = strings.Trim(cleanedArgs, "`\"'")

	// Если есть pending ability check, /roll (или /roll d20) резолвит его
	if b.performAbilityCheckUC != nil {
		gs, err := b.sessionRepo.GetByChatID(ctx, chatID)
		if err == nil && gs != nil && gs.HasPendingAbilityCheck() {
			if cleanedArgs == "" || strings.EqualFold(cleanedArgs, "d20") {
				result, err := b.performAbilityCheckUC.Execute(ctx, chatID)
				if err != nil {
					errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при проверке: %v", err))
					return b.sendMessage(errorMsg)
				}

				// Отправляем результат проверки
				if err := b.sendLongMessage(chatID, result.Message); err != nil {
					return err
				}

				// Автоматически продолжаем повествование после проверки (если DM доступен)
				if b.handleActionUC != nil {
					// Получаем контекст проверки из сессии
					gs, _ := b.sessionRepo.GetByChatID(ctx, chatID)
					reason := ""
					stakes := ""
					if gs != nil {
						reason = gs.PendingAbilityCheckReason
						stakes = gs.PendingAbilityCheckStakes
					}

					var continueMessage string
					if result.Success {
						if reason != "" && stakes != "" {
							continueMessage = fmt.Sprintf("🎲 РЕЗУЛЬТАТ ПРОВЕРКИ: УСПЕХ! %s успешно %s (DC %d, бросок %d). Опиши положительные последствия: %s",
								result.CharacterName, reason, result.DC, result.Total, stakes)
						} else {
							continueMessage = fmt.Sprintf("🎲 РЕЗУЛЬТАТ ПРОВЕРКИ: УСПЕХ! %s прошел проверку %s (DC %d, бросок %d). Продолжи историю с положительными последствиями.",
								result.CharacterName, result.Ability, result.DC, result.Total)
						}
					} else {
						if reason != "" && stakes != "" {
							continueMessage = fmt.Sprintf("🎲 РЕЗУЛЬТАТ ПРОВЕРКИ: ПРОВАЛ! %s не смог %s (DC %d, бросок %d). Опиши негативные последствия: %s",
								result.CharacterName, reason, result.DC, result.Total, stakes)
						} else {
							continueMessage = fmt.Sprintf("🎲 РЕЗУЛЬТАТ ПРОВЕРКИ: ПРОВАЛ! %s провалил проверку %s (DC %d, бросок %d). Продолжи историю с негативными последствиями.",
								result.CharacterName, result.Ability, result.DC, result.Total)
						}
					}

					// Для автоматических продолжений используем tgUserID первого игрока
					systemTgUserID := int64(0)
					if gs.GetFirstPlayer() != nil {
						systemTgUserID = gs.GetFirstPlayer().TgUserID
					}
					return b.handlePlayerAction(ctx, chatID, systemTgUserID, continueMessage)
				}
				return nil
			}

			if manualRoll, ok := parseManualAbilityCheckInput(cleanedArgs); ok {
				result, err := b.performAbilityCheckUC.ExecuteWithBaseRoll(ctx, chatID, manualRoll)
				if err != nil {
					errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при проверке: %v", err))
					return b.sendMessage(errorMsg)
				}

				// Отправляем результат проверки
				if err := b.sendLongMessage(chatID, result.Message); err != nil {
					return err
				}

				// Автоматически продолжаем повествование после проверки (если DM доступен)
				if b.handleActionUC != nil {
					// Получаем контекст проверки из сессии
					gs, _ := b.sessionRepo.GetByChatID(ctx, chatID)
					reason := ""
					stakes := ""
					if gs != nil {
						reason = gs.PendingAbilityCheckReason
						stakes = gs.PendingAbilityCheckStakes
					}

					var continueMessage string
					if result.Success {
						if reason != "" && stakes != "" {
							continueMessage = fmt.Sprintf("🎲 РЕЗУЛЬТАТ ПРОВЕРКИ: УСПЕХ! %s успешно %s (DC %d, бросок %d). Опиши положительные последствия: %s",
								result.CharacterName, reason, result.DC, result.Total, stakes)
						} else {
							continueMessage = fmt.Sprintf("🎲 РЕЗУЛЬТАТ ПРОВЕРКИ: УСПЕХ! %s прошел проверку %s (DC %d, бросок %d). Продолжи историю с положительными последствиями.",
								result.CharacterName, result.Ability, result.DC, result.Total)
						}
					} else {
						if reason != "" && stakes != "" {
							continueMessage = fmt.Sprintf("🎲 РЕЗУЛЬТАТ ПРОВЕРКИ: ПРОВАЛ! %s не смог %s (DC %d, бросок %d). Опиши негативные последствия: %s",
								result.CharacterName, reason, result.DC, result.Total, stakes)
						} else {
							continueMessage = fmt.Sprintf("🎲 РЕЗУЛЬТАТ ПРОВЕРКИ: ПРОВАЛ! %s провалил проверку %s (DC %d, бросок %d). Продолжи историю с негативными последствиями.",
								result.CharacterName, result.Ability, result.DC, result.Total)
						}
					}

					// Для автоматических продолжений используем tgUserID первого игрока
					systemTgUserID := int64(0)
					if gs.GetFirstPlayer() != nil {
						systemTgUserID = gs.GetFirstPlayer().TgUserID
					}
					return b.handlePlayerAction(ctx, chatID, systemTgUserID, continueMessage)
				}
				return nil
			}
		}
	}

	if cleanedArgs == "" {
		msg := tgbotapi.NewMessage(chatID, "Укажите выражение для броска, например: /roll d20 или /roll 2d6+3.")
		return b.sendMessage(msg)
	}

	result, err := b.rollDiceUC.Execute(ctx, cleanedArgs)
	if err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при броске кубика: %v\n\nИспользуйте формат: /roll d20 или /roll 2d6+3", err))
		return b.sendMessage(errorMsg)
	}

	// Сохраняем результат броска в историю событий, чтобы DM мог его видеть
	if b.eventRepo != nil {
		gs, err := b.sessionRepo.GetByChatID(ctx, chatID)
		if err == nil && gs != nil && gs.IsActive() {
			// Создаем событие для результата броска
			rollEvent := &event.StoryEvent{
				GameSessionID: gs.ID,
				AuthorType:    event.AuthorTypePlayer,
				Content:       result, // Сохраняем полный результат броска
				CreatedAt:     time.Now(),
			}

			// Сохраняем событие (не блокируем отправку сообщения при ошибке)
			if err := b.eventRepo.Save(ctx, rollEvent); err != nil {
				logger.Warn("Failed to save dice roll event",
					logger.ErrorField(err),
					logger.Int64("chat_id", chatID),
					logger.Uint("session_id", gs.ID),
				)
			} else {
				logger.Debug("Dice roll event saved",
					logger.Int64("chat_id", chatID),
					logger.Uint("session_id", gs.ID),
					logger.String("result", result),
				)

				// Индексируем результат броска в RAG, чтобы DM мог его видеть
				if b.indexDocUC != nil {
					ragCtx, ragCancel := context.WithTimeout(ctx, 10*time.Second)
					defer ragCancel()

					doc := ragdomain.Document{
						ID:        uuid.New().String(),
						Source:    ragdomain.SourceEvent,
						SessionID: gs.ID,
						Text:      "Игрок бросил кубик: " + result,
						Timestamp: time.Now(),
					}

					// Индексируем с таймаутом (не блокируем отправку сообщения при ошибке)
					if err := b.indexDocUC.Execute(ragCtx, doc); err != nil {
						logger.Warn("Failed to index dice roll event in RAG (event saved in DB, but not indexed)",
							logger.ErrorField(err),
							logger.Int64("chat_id", chatID),
							logger.Uint("session_id", gs.ID),
						)
					} else {
						logger.Debug("Dice roll event indexed in RAG",
							logger.Int64("chat_id", chatID),
							logger.Uint("session_id", gs.ID),
						)
					}
				}
			}
		}
	}

	return b.sendLongMessage(chatID, result)
}

func (b *Bot) handleQuests(ctx context.Context, chatID int64) error {
	questsText, err := b.getQuestsUC.Execute(ctx, chatID)
	if err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при получении квестов: %v", err))
		return b.sendMessage(errorMsg)
	}

	return b.sendLongMessage(chatID, questsText)
}

func (b *Bot) handleDaily(ctx context.Context, chatID int64, tgUserID int64) error {
	dailyText, err := b.getDailyQuestsUC.Execute(ctx, chatID, tgUserID)
	if err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при получении ежедневных заданий: %v", err))
		return b.sendMessage(errorMsg)
	}

	return b.sendLongMessage(chatID, dailyText)
}
