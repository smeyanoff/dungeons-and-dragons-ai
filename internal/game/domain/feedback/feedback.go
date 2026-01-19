package feedback

import (
	"gorm.io/gorm"
)

// FeedbackType тип обратной связи
type FeedbackType string

const (
	FeedbackTypeBug        FeedbackType = "bug"        // Баг
	FeedbackTypeSuggestion FeedbackType = "suggestion" // Предложение
	FeedbackTypeQuestion   FeedbackType = "question"   // Вопрос
	FeedbackTypePraise     FeedbackType = "praise"     // Похвала
	FeedbackTypeOther      FeedbackType = "other"      // Другое
)

// FeedbackCategory категория обратной связи
type FeedbackCategory string

const (
	FeedbackCategoryCombat    FeedbackCategory = "combat"    // Боевая система
	FeedbackCategoryDM        FeedbackCategory = "dm"        // DM (Dungeon Master)
	FeedbackCategoryInterface FeedbackCategory = "interface" // Интерфейс
	FeedbackCategoryGameplay  FeedbackCategory = "gameplay"  // Геймплей
	FeedbackCategoryOther     FeedbackCategory = "other"     // Другое
)

// Feedback представляет фидбек от пользователя
type Feedback struct {
	gorm.Model
	ChatID  int64 `gorm:"index"`
	UserID  int64 `gorm:"index"`
	Message string `gorm:"type:text;not null"`
	
	// Тип и категория обратной связи
	Type     FeedbackType     `gorm:"type:varchar(32)"`
	Category FeedbackCategory `gorm:"type:varchar(32)"`
	
	// Метаданные
	UserFirstName string
	UserLastName  string
	UserUsername  string
}
