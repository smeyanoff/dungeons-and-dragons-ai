package telegram

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"dungeons-and-dragons-ai/pkg/logger"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (b *Bot) maybeSendPendingAbilityCheckPrompt(ctx context.Context, chatID int64) error {
	gs, err := b.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil || gs == nil || !gs.HasPendingAbilityCheck() || gs.PendingAbilityCheckNotified {
		return err
	}

	abilityName := formatAbilityName(gs.PendingAbilityCheckAbility)
	text := fmt.Sprintf("🎲 Проверка %s (DC %d). Напишите /roll, чтобы бросить кубик. Можно отправить результат числом (например: 17).", abilityName, gs.PendingAbilityCheckDC)

	msg := tgbotapi.NewMessage(chatID, text)

	if err := b.sendMessage(msg); err != nil {
		return err
	}

	gs.PendingAbilityCheckNotified = true
	if err := b.sessionRepo.Save(ctx, gs); err != nil {
		logger.Warn("Failed to mark pending check as notified",
			logger.ErrorField(err),
			logger.Int64("chat_id", chatID),
		)
	}

	return nil
}

func (b *Bot) tryHandleManualAbilityCheck(ctx context.Context, chatID int64, text string) (bool, error) {
	if b.performAbilityCheckUC == nil {
		return false, nil
	}

	manualRoll, ok := parseManualAbilityCheckInput(text)
	if !ok {
		return false, nil
	}

	gs, err := b.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil || gs == nil || !gs.HasPendingAbilityCheck() {
		return false, err
	}

	result, err := b.performAbilityCheckUC.ExecuteWithBaseRoll(ctx, chatID, manualRoll)
	if err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при проверке: %v", err))
		return true, b.sendMessage(errorMsg)
	}

	// Отправляем результат проверки
	if err := b.sendLongMessage(chatID, result.Message); err != nil {
		return true, err
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
				continueMessage = fmt.Sprintf("✅ УСПЕХ проверки! %s успешно %s. Теперь опиши положительные последствия: %s",
					result.CharacterName, reason, stakes)
			} else {
				continueMessage = fmt.Sprintf("✅ УСПЕХ проверки! %s прошел проверку %s (DC %d, результат %d). Продолжи историю с положительными последствиями.",
					result.CharacterName, result.Ability, result.DC, result.Total)
			}
		} else {
			if reason != "" && stakes != "" {
				continueMessage = fmt.Sprintf("❌ ПРОВАЛ проверки! %s не смог %s. Теперь опиши негативные последствия: %s",
					result.CharacterName, reason, stakes)
			} else {
				continueMessage = fmt.Sprintf("❌ ПРОВАЛ проверки! %s провалил проверку %s (DC %d, результат %d). Продолжи историю с негативными последствиями.",
					result.CharacterName, result.Ability, result.DC, result.Total)
			}
		}

		// Для автоматических продолжений используем tgUserID первого игрока
		systemTgUserID := int64(0)
		if gs != nil && gs.GetFirstPlayer() != nil {
			systemTgUserID = gs.GetFirstPlayer().TgUserID
		}
		return true, b.handlePlayerAction(ctx, chatID, systemTgUserID, continueMessage)
	}
	return true, nil
}

func parseManualAbilityCheckInput(text string) (int, bool) {
	normalized := strings.TrimSpace(strings.ToLower(text))
	if normalized == "" {
		return 0, false
	}

	if regexp.MustCompile(`^\d{1,2}$`).MatchString(normalized) {
		value, err := strconv.Atoi(normalized)
		if err == nil && value >= 1 && value <= 20 {
			return value, true
		}
		return 0, false
	}

	if strings.HasPrefix(normalized, "результат") || strings.HasPrefix(normalized, "result") {
		re := regexp.MustCompile(`\b(\d{1,2})\b`)
		match := re.FindStringSubmatch(normalized)
		if len(match) >= 2 {
			value, err := strconv.Atoi(match[1])
			if err == nil && value >= 1 && value <= 20 {
				return value, true
			}
		}
	}

	return 0, false
}

func formatAbilityName(ability string) string {
	switch ability {
	case "strength":
		return "Сила (STR)"
	case "dexterity":
		return "Ловкость (DEX)"
	case "constitution":
		return "Телосложение (CON)"
	case "intelligence":
		return "Интеллект (INT)"
	case "wisdom":
		return "Мудрость (WIS)"
	case "charisma":
		return "Харизма (CHA)"
	default:
		return ability
	}
}

