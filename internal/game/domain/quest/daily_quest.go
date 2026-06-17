package quest

import "time"

// DailyQuestType тип ежедневного задания
type DailyQuestType string

const (
	DailyQuestTypeCompleteQuest   DailyQuestType = "complete_quest"   // Завершить квест
	DailyQuestTypeWinCombat       DailyQuestType = "win_combat"       // Победить в бою
	DailyQuestTypeExploreLocation DailyQuestType = "explore_location" // Исследовать локацию
)

// DailyQuest представляет ежедневное задание
type DailyQuest struct {
	ID               uint           `gorm:"primaryKey"`
	Type             DailyQuestType `gorm:"type:varchar(32);not null"`
	Title            string
	Description      string
	TargetValue      int // Целевое значение (например, количество побед в бою)
	ExperienceReward int // Награда опытом
	GoldReward       int // Награда золотом (внутриигровая валюта)
	CreatedAt        time.Time
}

// DailyQuestProgress представляет прогресс игрока по ежедневному заданию
type DailyQuestProgress struct {
	ID           uint       `gorm:"primaryKey"`
	PlayerID     uint       `gorm:"index"`
	DailyQuestID uint       `gorm:"index"`
	DailyQuest   DailyQuest `gorm:"foreignKey:DailyQuestID"` // Связанное задание
	CurrentValue int        // Текущее значение прогресса
	TargetValue  int        // Целевое значение (копируется из DailyQuest для быстрого доступа)
	Completed    bool       `gorm:"default:false"`
	CompletedAt  *time.Time
	Date         time.Time `gorm:"index"` // Дата задания (для определения ежедневного сброса)
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// DailyQuestStreak представляет стрик игрока (последовательные дни выполнения заданий)
type DailyQuestStreak struct {
	ID         uint      `gorm:"primaryKey"`
	PlayerID   uint      `gorm:"uniqueIndex"`
	StreakDays int       // Количество последовательных дней
	LastDate   time.Time // Дата последнего выполнения
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// NewDailyQuest создает новое ежедневное задание
func NewDailyQuest(questType DailyQuestType, title, description string, targetValue, expReward, goldReward int) *DailyQuest {
	return &DailyQuest{
		Type:             questType,
		Title:            title,
		Description:      description,
		TargetValue:      targetValue,
		ExperienceReward: expReward,
		GoldReward:       goldReward,
		CreatedAt:        time.Now(),
	}
}

// IsCompleted проверяет, выполнено ли задание
func (p *DailyQuestProgress) IsCompleted() bool {
	return p.Completed || p.CurrentValue >= p.TargetValue
}

// IncrementProgress увеличивает прогресс задания
func (p *DailyQuestProgress) IncrementProgress(amount int) {
	if p.Completed {
		return // Уже выполнено
	}
	p.CurrentValue += amount
	// Проверяем, выполнено ли задание
	if p.CurrentValue >= p.TargetValue {
		p.Complete()
	}
}

// Complete помечает задание как выполненное
func (p *DailyQuestProgress) Complete() {
	if !p.Completed {
		p.Completed = true
		now := time.Now()
		p.CompletedAt = &now
	}
}

// GetDailyQuestTypes возвращает список типов ежедневных заданий
func GetDailyQuestTypes() []DailyQuestType {
	return []DailyQuestType{
		DailyQuestTypeCompleteQuest,
		DailyQuestTypeWinCombat,
		DailyQuestTypeExploreLocation,
	}
}
