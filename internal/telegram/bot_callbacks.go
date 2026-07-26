package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	characterapp "dungeons-and-dragons-ai/internal/game/application/character"
	mapapp "dungeons-and-dragons-ai/internal/game/application/worldmap"
	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/feedback"
	"dungeons-and-dragons-ai/pkg/logger"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (b *Bot) handleCallbackQuery(ctx context.Context, query *tgbotapi.CallbackQuery) error {
	chatID := query.Message.Chat.ID
	data := query.Data

	logger.Debug("Handling callback query",
		logger.String("data", data),
		logger.Int64("chat_id", chatID),
		logger.Int64("user_id", query.From.ID),
	)

	// Формат: race_<race> | class_<race>_<class> | create_<name>_<race>_<class> | map_to_<location_id>
	if strings.HasPrefix(data, "race_") {
		race := strings.TrimPrefix(data, "race_")
		return b.handleRaceSelection(ctx, chatID, query, race)
	} else if strings.HasPrefix(data, "class_") {
		parts := strings.Split(strings.TrimPrefix(data, "class_"), "_")
		if len(parts) >= 2 {
			race := parts[0]
			class := parts[1]
			return b.handleClassSelection(ctx, chatID, query, race, class)
		}
	} else if strings.HasPrefix(data, "create_") {
		parts := strings.Split(strings.TrimPrefix(data, "create_"), "_")
		if len(parts) >= 3 {
			name := parts[0]
			race := parts[1]
			class := parts[2]
			return b.handleCreateCharacterFromCallback(ctx, chatID, query, name, race, class)
		}
	} else if strings.HasPrefix(data, "feedback_type_") {
		feedbackType := strings.TrimPrefix(data, "feedback_type_")
		return b.handleFeedbackTypeSelection(ctx, chatID, query, feedback.FeedbackType(feedbackType))
	} else if strings.HasPrefix(data, "feedback_category_") {
		feedbackCategory := strings.TrimPrefix(data, "feedback_category_")
		return b.handleFeedbackCategorySelection(ctx, chatID, query, feedback.FeedbackCategory(feedbackCategory))
	} else if data == "feedback_cancel" {
		return b.handleFeedbackCancel(ctx, chatID, query)
	} else if strings.HasPrefix(data, "map_to_") {
		idStr := strings.TrimPrefix(data, "map_to_")
		locID, err := strconv.ParseUint(idStr, 10, 64)
		if err != nil || locID == 0 {
			callback := tgbotapi.NewCallback(query.ID, "Некорректная локация")
			if _, cbErr := b.api.Request(callback); cbErr != nil {
				logger.Warn("Failed to answer invalid map location callback",
					logger.ErrorField(cbErr),
					logger.Int64("chat_id", chatID),
					logger.String("callback_data", data),
				)
			}
			return nil
		}
		return b.handleMapMoveCallback(ctx, query, uint(locID))
	}

	callback := tgbotapi.NewCallback(query.ID, "Неизвестная команда")
	_, err := b.api.Request(callback)
	return err
}

func (b *Bot) handleMapMoveCallback(ctx context.Context, query *tgbotapi.CallbackQuery, toLocationID uint) error {
	chatID := query.Message.Chat.ID
	tgUserID := int64(0)
	if query.From != nil {
		tgUserID = query.From.ID
	}

	callback := tgbotapi.NewCallback(query.ID, "Перемещаюсь...")
	if _, cbErr := b.api.Request(callback); cbErr != nil {
		logger.Warn("Failed to answer map move callback",
			logger.ErrorField(cbErr),
			logger.Int64("chat_id", chatID),
		)
	}

	if b.moveToLocationUC == nil {
		edit := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID, "Навигация по карте временно недоступна.")
		return b.editMessage(edit, chatID, "Навигация по карте временно недоступна.")
	}

	gsForContext, _ := b.sessionRepo.GetByChatID(ctx, chatID)
	ctxWithIDs := context.WithValue(ctx, "chat_id", chatID)
	if tgUserID != 0 {
		ctxWithIDs = context.WithValue(ctxWithIDs, "tg_user_id", tgUserID)
	}
	if gsForContext != nil {
		ctxWithIDs = context.WithValue(ctxWithIDs, "session_id", gsForContext.ID)
	}

	resp, err := b.moveToLocationUC.Execute(ctxWithIDs, mapapp.MoveToLocationRequest{
		ChatID:       chatID,
		ToLocationID: &toLocationID,
	})
	if err != nil {
		edit := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID, fmt.Sprintf("Не удалось переместиться: %v", err))
		return b.editMessage(edit, chatID, edit.Text)
	}

	gs, _ := b.sessionRepo.GetByChatID(ctx, chatID)
	markup := b.buildMapNavigationKeyboard(gs)

	edit := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID, resp.Message)
	if markup != nil {
		edit.ReplyMarkup = markup
	}
	return b.editMessage(edit, chatID, resp.Message)
}

func (b *Bot) handleRaceSelection(ctx context.Context, chatID int64, query *tgbotapi.CallbackQuery, race string) error {
	callback := tgbotapi.NewCallback(query.ID, fmt.Sprintf("Выбрана раса: %s", race))
	if _, err := b.api.Request(callback); err != nil {
		logger.Error("Failed to answer callback",
			logger.ErrorField(err),
		)
	}

	text := fmt.Sprintf(`🎭 Создание персонажа

✅ Выбрана раса: %s

Теперь выберите класс:`, race)

	edit := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID, text)

	classButtons := [][]tgbotapi.InlineKeyboardButton{
		{
			tgbotapi.NewInlineKeyboardButtonData("⚔️ Воин", fmt.Sprintf("class_%s_fighter", race)),
			tgbotapi.NewInlineKeyboardButtonData("🔮 Маг", fmt.Sprintf("class_%s_wizard", race)),
		},
		{
			tgbotapi.NewInlineKeyboardButtonData("🗡️ Вор", fmt.Sprintf("class_%s_rogue", race)),
			tgbotapi.NewInlineKeyboardButtonData("✨ Жрец", fmt.Sprintf("class_%s_cleric", race)),
		},
		{
			tgbotapi.NewInlineKeyboardButtonData("🏹 Следопыт", fmt.Sprintf("class_%s_ranger", race)),
		},
		{
			tgbotapi.NewInlineKeyboardButtonData("⬅️ Назад", "race_human"),
		},
	}

	edit.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: classButtons}
	return b.editMessage(edit, chatID, text)
}

func (b *Bot) handleClassSelection(ctx context.Context, chatID int64, query *tgbotapi.CallbackQuery, race, class string) error {
	callback := tgbotapi.NewCallback(query.ID, fmt.Sprintf("Выбран класс: %s", class))
	if _, err := b.api.Request(callback); err != nil {
		logger.Error("Failed to answer callback",
			logger.ErrorField(err),
		)
	}

	text := fmt.Sprintf(`🎭 Создание персонажа

✅ Раса: %s
✅ Класс: %s

📝 Теперь введите имя персонажа текстовым сообщением, или используйте команду:
/createcharacter <имя> %s %s

Пример: /createcharacter Гендальф %s %s`, race, class, race, class, race, class)

	edit := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID, text)

	defaultName := query.From.UserName
	if defaultName == "" {
		defaultName = query.From.FirstName
	}
	if defaultName == "" {
		defaultName = "Герой"
	}

	buttons := [][]tgbotapi.InlineKeyboardButton{
		{
			tgbotapi.NewInlineKeyboardButtonData(
				fmt.Sprintf("✅ Создать с именем '%s'", defaultName),
				fmt.Sprintf("create_%s_%s_%s", defaultName, race, class),
			),
		},
		{
			tgbotapi.NewInlineKeyboardButtonData("⬅️ Назад к выбору расы", "race_human"),
		},
	}

	edit.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: buttons}
	return b.editMessage(edit, chatID, text)
}

func (b *Bot) handleCreateCharacterFromCallback(ctx context.Context, chatID int64, query *tgbotapi.CallbackQuery, name, raceStr, classStr string) error {
	callback := tgbotapi.NewCallback(query.ID, "Создаю персонажа...")
	if _, err := b.api.Request(callback); err != nil {
		logger.Error("Failed to answer callback",
			logger.ErrorField(err),
		)
	}

	race := character.Race(strings.ToLower(raceStr))
	class := character.Class(strings.ToLower(classStr))

	validRaces := map[character.Race]bool{
		character.RaceHuman:    true,
		character.RaceElf:      true,
		character.RaceDwarf:    true,
		character.RaceOrc:      true,
		character.RaceHalfling: true,
	}
	if !validRaces[race] {
		errorMsg := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID,
			fmt.Sprintf("❌ Ошибка: Неизвестная раса: %s", race))
		return b.editMessage(errorMsg, chatID, fmt.Sprintf("❌ Ошибка: Неизвестная раса: %s", race))
	}

	validClasses := map[character.Class]bool{
		character.ClassFighter: true,
		character.ClassWizard:  true,
		character.ClassRogue:   true,
		character.ClassCleric:  true,
		character.ClassRanger:  true,
	}
	if !validClasses[class] {
		errorMsg := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID,
			fmt.Sprintf("❌ Ошибка: Неизвестный класс: %s", class))
		return b.editMessage(errorMsg, chatID, fmt.Sprintf("❌ Ошибка: Неизвестный класс: %s", class))
	}

	req := characterapp.CreateCharacterRequest{
		ChatID: chatID,
		Name:   name,
		Race:   race,
		Class:  class,
	}

	player, err := b.createCharacterUC.Execute(ctx, req)
	if err != nil {
		errorMsg := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID,
			fmt.Sprintf("❌ Ошибка при создании персонажа: %v", err))
		if sendErr := b.editMessage(errorMsg, chatID, fmt.Sprintf("❌ Ошибка при создании персонажа: %v", err)); sendErr != nil {
			return sendErr
		}
		return err
	}

	charText := fmt.Sprintf(`✅ Персонаж создан!

👤 Имя: %s
🏛️ Раса: %s
⚔️ Класс: %s
📊 Уровень: %d
❤️ HP: %d/%d

📈 Характеристики:
💪 Сила: %d
🏃 Ловкость: %d
🛡️ Телосложение: %d
🧠 Интеллект: %d
🔮 Мудрость: %d
💬 Харизма: %d

🎒 Стартовое снаряжение и немного золота уже ждут в /inventory`,
		player.Character.Name,
		player.Character.Race,
		player.Character.Class,
		player.Character.Level,
		player.Character.HP,
		player.Character.MaxHP,
		player.Character.Stats.Strength,
		player.Character.Stats.Dexterity,
		player.Character.Stats.Constitution,
		player.Character.Stats.Intelligence,
		player.Character.Stats.Wisdom,
		player.Character.Stats.Charisma,
	)

	resultMsg := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID, charText)
	resultMsg.ReplyMarkup = nil

	return b.editMessage(resultMsg, chatID, charText)
}
