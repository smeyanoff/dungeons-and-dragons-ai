package quest

import "dungeons-and-dragons-ai/internal/game/domain/item"

type QuestStatus string

const (
	QuestStatusActive    QuestStatus = "active"
	QuestStatusCompleted QuestStatus = "completed"
	QuestStatusFailed    QuestStatus = "failed"
)

type Quest struct {
	ID               uint `gorm:"primaryKey"`
	WorldID          uint `gorm:"index"`
	Title            string
	Description      string
	Status           QuestStatus `gorm:"type:varchar(16);default:'active'"`
	ExperienceReward int         // Опыт за выполнение квеста
	Items            []item.Item `gorm:"many2many:quest_items;"`
}

func New(title, description string) *Quest {
	return &Quest{
		Title:            title,
		Description:      description,
		Status:           QuestStatusActive,
		ExperienceReward: 100, // Базовый опыт за квест
		Items:            []item.Item{},
	}
}

func (q *Quest) AddItem(it *item.Item) {
	q.Items = append(q.Items, *it)
}

// Complete помечает квест как выполненный
func (q *Quest) Complete() {
	q.Status = QuestStatusCompleted
}

// Fail помечает квест как проваленный
func (q *Quest) Fail() {
	q.Status = QuestStatusFailed
}

// IsActive проверяет, активен ли квест
func (q *Quest) IsActive() bool {
	return q.Status == QuestStatusActive
}

// SetExperienceReward устанавливает награду опытом
func (q *Quest) SetExperienceReward(amount int) {
	if amount < 0 {
		amount = 0
	}
	q.ExperienceReward = amount
}
