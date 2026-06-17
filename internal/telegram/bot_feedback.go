package telegram

import (
	"context"
	"fmt"
	"strings"

	"dungeons-and-dragons-ai/internal/game/domain/feedback"
	"dungeons-and-dragons-ai/pkg/logger"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (b *Bot) handleFeedback(ctx context.Context, chatID int64, args string, tgUserID int64, from *tgbotapi.User) error {
	feedbackText := strings.TrimSpace(args)
	if feedbackText != "" {
		b.feedbackStateMu.Lock()
		delete(b.feedbackState, chatID)
		b.feedbackStateMu.Unlock()

		return b.saveFeedbackDirectly(ctx, chatID, tgUserID, from, feedbackText, feedback.FeedbackTypeOther, feedback.FeedbackCategoryOther)
	}

	b.feedbackStateMu.Lock()
	delete(b.feedbackState, chatID)
	b.feedbackStateMu.Unlock()

	return b.startFeedbackDialog(ctx, chatID, tgUserID, from)
}

func (b *Bot) startFeedbackDialog(ctx context.Context, chatID int64, tgUserID int64, from *tgbotapi.User) error {
	b.feedbackStateMu.Lock()
	b.feedbackState[chatID] = &FeedbackDialogState{
		UserID: tgUserID,
		From:   from,
	}
	b.feedbackStateMu.Unlock()

	msg := tgbotapi.NewMessage(chatID, "📝 Выберите тип обратной связи:")

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🐛 Баг", "feedback_type_bug"),
			tgbotapi.NewInlineKeyboardButtonData("💡 Предложение", "feedback_type_suggestion"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❓ Вопрос", "feedback_type_question"),
			tgbotapi.NewInlineKeyboardButtonData("⭐ Похвала", "feedback_type_praise"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📋 Другое", "feedback_type_other"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Отмена", "feedback_cancel"),
		),
	)
	msg.ReplyMarkup = keyboard

	return b.sendMessage(msg)
}

func (b *Bot) handleFeedbackTypeSelection(ctx context.Context, chatID int64, query *tgbotapi.CallbackQuery, feedbackType feedback.FeedbackType) error {
	typeNames := map[feedback.FeedbackType]string{
		feedback.FeedbackTypeBug:        "Баг",
		feedback.FeedbackTypeSuggestion: "Предложение",
		feedback.FeedbackTypeQuestion:   "Вопрос",
		feedback.FeedbackTypePraise:     "Похвала",
		feedback.FeedbackTypeOther:      "Другое",
	}
	typeName := typeNames[feedbackType]
	if typeName == "" {
		typeName = "Другое"
	}

	callback := tgbotapi.NewCallback(query.ID, fmt.Sprintf("Выбран тип: %s", typeName))
	if _, err := b.api.Request(callback); err != nil {
		logger.Error("Failed to answer callback",
			logger.ErrorField(err),
		)
	}

	b.feedbackStateMu.Lock()
	state, exists := b.feedbackState[chatID]
	if !exists {
		state = &FeedbackDialogState{
			UserID: query.From.ID,
			From:   query.From,
		}
		b.feedbackState[chatID] = state
	}
	state.Type = feedbackType
	b.feedbackStateMu.Unlock()

	msg := tgbotapi.NewEditMessageText(
		chatID,
		query.Message.MessageID,
		fmt.Sprintf("📝 Тип: %s\n\nВыберите категорию:", typeName),
	)

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⚔️ Боевая система", "feedback_category_combat"),
			tgbotapi.NewInlineKeyboardButtonData("🎭 DM", "feedback_category_dm"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🖥️ Интерфейс", "feedback_category_interface"),
			tgbotapi.NewInlineKeyboardButtonData("🎮 Геймплей", "feedback_category_gameplay"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📋 Другое", "feedback_category_other"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Отмена", "feedback_cancel"),
		),
	)
	msg.ReplyMarkup = &keyboard

	return b.editMessage(msg, chatID, msg.Text)
}

func (b *Bot) handleFeedbackCategorySelection(ctx context.Context, chatID int64, query *tgbotapi.CallbackQuery, feedbackCategory feedback.FeedbackCategory) error {
	categoryNames := map[feedback.FeedbackCategory]string{
		feedback.FeedbackCategoryCombat:    "Боевая система",
		feedback.FeedbackCategoryDM:        "DM",
		feedback.FeedbackCategoryInterface: "Интерфейс",
		feedback.FeedbackCategoryGameplay:  "Геймплей",
		feedback.FeedbackCategoryOther:     "Другое",
	}
	categoryName := categoryNames[feedbackCategory]
	if categoryName == "" {
		categoryName = "Другое"
	}

	callback := tgbotapi.NewCallback(query.ID, fmt.Sprintf("Выбрана категория: %s", categoryName))
	if _, err := b.api.Request(callback); err != nil {
		logger.Error("Failed to answer callback",
			logger.ErrorField(err),
		)
	}

	b.feedbackStateMu.Lock()
	state, exists := b.feedbackState[chatID]
	if !exists {
		state = &FeedbackDialogState{
			UserID: query.From.ID,
			From:   query.From,
		}
		b.feedbackState[chatID] = state
	}
	state.Category = feedbackCategory
	b.feedbackStateMu.Unlock()

	typeNames := map[feedback.FeedbackType]string{
		feedback.FeedbackTypeBug:        "Баг",
		feedback.FeedbackTypeSuggestion: "Предложение",
		feedback.FeedbackTypeQuestion:   "Вопрос",
		feedback.FeedbackTypePraise:     "Похвала",
		feedback.FeedbackTypeOther:      "Другое",
	}
	typeName := typeNames[state.Type]
	if typeName == "" {
		typeName = "Другое"
	}

	msg := tgbotapi.NewEditMessageText(
		chatID,
		query.Message.MessageID,
		fmt.Sprintf("📝 Тип: %s\n📂 Категория: %s\n\n✍️ Теперь напишите ваш отзыв (просто отправьте текст сообщением):",
			typeName, categoryName),
	)
	msg.ReplyMarkup = nil

	return b.editMessage(msg, chatID, msg.Text)
}

func (b *Bot) handleFeedbackCancel(ctx context.Context, chatID int64, query *tgbotapi.CallbackQuery) error {
	callback := tgbotapi.NewCallback(query.ID, "Диалог отменен")
	if _, err := b.api.Request(callback); err != nil {
		logger.Error("Failed to answer callback",
			logger.ErrorField(err),
		)
	}

	b.feedbackStateMu.Lock()
	delete(b.feedbackState, chatID)
	b.feedbackStateMu.Unlock()

	msg := tgbotapi.NewEditMessageText(
		chatID,
		query.Message.MessageID,
		"❌ Диалог отменен. Используйте /feedback для начала нового отзыва.",
	)
	msg.ReplyMarkup = nil

	return b.editMessage(msg, chatID, msg.Text)
}

func (b *Bot) saveFeedbackDirectly(ctx context.Context, chatID int64, tgUserID int64, from *tgbotapi.User, feedbackText string, feedbackType feedback.FeedbackType, feedbackCategory feedback.FeedbackCategory) error {
	if b.feedbackRepo == nil {
		msg := tgbotapi.NewMessage(chatID, "Извините, система фидбека временно недоступна.")
		return b.sendMessage(msg)
	}

	fb := &feedback.Feedback{
		ChatID:   chatID,
		UserID:   tgUserID,
		Message:  feedbackText,
		Type:     feedbackType,
		Category: feedbackCategory,
	}

	if from != nil {
		fb.UserFirstName = from.FirstName
		fb.UserLastName = from.LastName
		fb.UserUsername = from.UserName
	}

	if err := b.feedbackRepo.Save(ctx, fb); err != nil {
		logger.Error("Failed to save feedback",
			logger.ErrorField(err),
			logger.Int64("chat_id", chatID),
			logger.Int64("user_id", tgUserID),
		)
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при сохранении отзыва: %v", err))
		return b.sendMessage(errorMsg)
	}

	// Ответ пользователю
	msg := tgbotapi.NewMessage(chatID, "✅ Спасибо за ваш отзыв! Мы обязательно рассмотрим его.")
	return b.sendMessage(msg)
}
