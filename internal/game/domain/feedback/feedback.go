package feedback

import (
	"gorm.io/gorm"
)

// Feedback представляет фидбек от пользователя
type Feedback struct {
	gorm.Model
	ChatID  int64 `gorm:"index"`
	UserID  int64 `gorm:"index"`
	Message string `gorm:"type:text;not null"`
	
	// Метаданные
	UserFirstName string
	UserLastName  string
	UserUsername  string
}
