package achievement

import (
	"context"
)

// NotificationService интерфейс для отправки уведомлений о достижениях
type NotificationService interface {
	// SendAchievementNotification отправляет уведомление о разблокированном достижении
	SendAchievementNotification(ctx context.Context, chatID int64, message string) error
}

// NoOpNotificationService пустая реализация NotificationService (для тестов или когда уведомления не нужны)
type NoOpNotificationService struct{}

func (n *NoOpNotificationService) SendAchievementNotification(ctx context.Context, chatID int64, message string) error {
	// Ничего не делаем - пустая реализация
	return nil
}
