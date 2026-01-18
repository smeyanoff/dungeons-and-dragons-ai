package achievement

import (
	"context"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// TelegramNotificationService реализация NotificationService для отправки уведомлений через Telegram
type TelegramNotificationService struct {
	api *tgbotapi.BotAPI
}

// NewTelegramNotificationService создает новый TelegramNotificationService
func NewTelegramNotificationService(api *tgbotapi.BotAPI) *TelegramNotificationService {
	return &TelegramNotificationService{
		api: api,
	}
}

// TelegramBotInterface интерфейс для получения BotAPI из telegram.Bot
// Это позволяет избежать циклических зависимостей
type TelegramBotInterface interface {
	GetAPI() *tgbotapi.BotAPI
}

// NewTelegramNotificationServiceFromBot создает TelegramNotificationService из telegram.Bot
func NewTelegramNotificationServiceFromBot(bot TelegramBotInterface) *TelegramNotificationService {
	return &TelegramNotificationService{
		api: bot.GetAPI(),
	}
}

// SendAchievementNotification отправляет уведомление о разблокированном достижении через Telegram
func (t *TelegramNotificationService) SendAchievementNotification(ctx context.Context, chatID int64, message string) error {
	msg := tgbotapi.NewMessage(chatID, message)
	_, err := t.api.Send(msg)
	return err
}
