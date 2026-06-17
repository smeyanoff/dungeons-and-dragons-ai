package persistence

import (
	"context"

	"gorm.io/gorm"

	"dungeons-and-dragons-ai/internal/game/domain/feedback"
)

type FeedbackRepository struct {
	db *gorm.DB
}

func NewFeedbackRepository(db *gorm.DB) *FeedbackRepository {
	return &FeedbackRepository{
		db: db,
	}
}

// Save сохраняет фидбек в БД
func (r *FeedbackRepository) Save(ctx context.Context, fb *feedback.Feedback) error {
	return r.db.WithContext(ctx).Create(fb).Error
}

// GetByChatID возвращает все фидбеки от пользователя (для отладки/анализа)
func (r *FeedbackRepository) GetByChatID(ctx context.Context, chatID int64, limit int) ([]*feedback.Feedback, error) {
	var feedbacks []*feedback.Feedback
	err := r.db.WithContext(ctx).
		Where("chat_id = ?", chatID).
		Order("created_at DESC").
		Limit(limit).
		Find(&feedbacks).Error
	if err != nil {
		return nil, err
	}
	return feedbacks, nil
}

// GetAll возвращает все фидбеки (для модератора/Product Manager)
func (r *FeedbackRepository) GetAll(ctx context.Context, limit, offset int) ([]*feedback.Feedback, error) {
	var feedbacks []*feedback.Feedback
	err := r.db.WithContext(ctx).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&feedbacks).Error
	if err != nil {
		return nil, err
	}
	return feedbacks, nil
}
