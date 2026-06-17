package persistence

import (
	"context"
	"time"

	"dungeons-and-dragons-ai/internal/game/domain/subscription"
	"gorm.io/gorm"
)

type SubscriptionRepository struct {
	db *gorm.DB
}

func NewSubscriptionRepository(db *gorm.DB) *SubscriptionRepository {
	return &SubscriptionRepository{db: db}
}

// GetByTgUserID получает подписку пользователя по Telegram User ID
func (r *SubscriptionRepository) GetByTgUserID(ctx context.Context, tgUserID int64) (*subscription.Subscription, error) {
	var sub subscription.Subscription
	err := r.db.WithContext(ctx).
		Where("tg_user_id = ?", tgUserID).
		First(&sub).Error

	if err == gorm.ErrRecordNotFound {
		// Если подписки нет, создаем бесплатную подписку
		return r.createFreeSubscription(ctx, tgUserID)
	}
	if err != nil {
		return nil, err
	}

	return &sub, nil
}

// createFreeSubscription создает бесплатную подписку для нового пользователя
func (r *SubscriptionRepository) createFreeSubscription(ctx context.Context, tgUserID int64) (*subscription.Subscription, error) {
	now := time.Now()
	sub := &subscription.Subscription{
		TgUserID:  tgUserID,
		Plan:      subscription.PlanFree,
		Status:    subscription.StatusActive,
		StartedAt: &now,
	}

	err := r.db.WithContext(ctx).Create(sub).Error
	if err != nil {
		return nil, err
	}

	return sub, nil
}

// Save сохраняет подписку
func (r *SubscriptionRepository) Save(ctx context.Context, sub *subscription.Subscription) error {
	return r.db.WithContext(ctx).Save(sub).Error
}

// UpdateStatus обновляет статус подписки
func (r *SubscriptionRepository) UpdateStatus(ctx context.Context, tgUserID int64, status subscription.Status) error {
	return r.db.WithContext(ctx).
		Model(&subscription.Subscription{}).
		Where("tg_user_id = ?", tgUserID).
		Update("status", status).Error
}

// GetActiveSubscriptions получает все активные подписки
func (r *SubscriptionRepository) GetActiveSubscriptions(ctx context.Context) ([]*subscription.Subscription, error) {
	var subs []*subscription.Subscription
	now := time.Now()

	err := r.db.WithContext(ctx).
		Where("status = ? AND (expires_at IS NULL OR expires_at > ?)", subscription.StatusActive, now).
		Find(&subs).Error

	if err != nil {
		return nil, err
	}

	return subs, nil
}

// GetExpiredSubscriptions получает истекшие подписки для обновления статуса
func (r *SubscriptionRepository) GetExpiredSubscriptions(ctx context.Context) ([]*subscription.Subscription, error) {
	var subs []*subscription.Subscription
	now := time.Now()

	err := r.db.WithContext(ctx).
		Where("status = ? AND expires_at IS NOT NULL AND expires_at <= ?", subscription.StatusActive, now).
		Find(&subs).Error

	if err != nil {
		return nil, err
	}

	return subs, nil
}

// UpdateExpiredSubscriptions обновляет статус истекших подписок
func (r *SubscriptionRepository) UpdateExpiredSubscriptions(ctx context.Context) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&subscription.Subscription{}).
		Where("status = ? AND expires_at IS NOT NULL AND expires_at <= ?", subscription.StatusActive, now).
		Update("status", subscription.StatusExpired).Error
}
