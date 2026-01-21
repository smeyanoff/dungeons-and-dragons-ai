package achievement

import "time"

// AchievementType тип достижения
type AchievementType string

const (
	// Combat достижения - связанные с боями
	AchievementTypeCombat AchievementType = "combat"
	// Quest достижения - связанные с квестами
	AchievementTypeQuest AchievementType = "quest"
	// Exploration достижения - связанные с исследованием
	AchievementTypeExploration AchievementType = "exploration"
	// Progress достижения - связанные с прогрессом персонажа
	AchievementTypeProgress AchievementType = "progress"
	// Collection достижения - связанные с коллекциями
	AchievementTypeCollection AchievementType = "collection"
	// Special достижения - особые/редкие достижения
	AchievementTypeSpecial AchievementType = "special"
)

// Rarity редкость достижения
type Rarity string

const (
	RarityCommon    Rarity = "common"    // Обычное
	RarityUncommon  Rarity = "uncommon"  // Необычное
	RarityRare      Rarity = "rare"      // Редкое
	RarityEpic      Rarity = "epic"      // Эпическое
	RarityLegendary Rarity = "legendary" // Легендарное
)

// Achievement представляет достижение в игре
type Achievement struct {
	ID          uint            `gorm:"primaryKey"`
	Code        string          `gorm:"uniqueIndex;type:varchar(64);not null"` // Уникальный код достижения
	Title       string          `gorm:"type:varchar(128);not null"`
	Description string          `gorm:"type:text"`
	Type        AchievementType `gorm:"type:varchar(32);not null"`
	Rarity      Rarity          `gorm:"type:varchar(16);default:'common'"`

	// Условия для получения достижения
	RequirementValue int    // Значение для проверки (например, 10 для "победить 10 монстров")
	RequirementKey   string // Ключ требования (например, "combat_wins", "quests_completed")

	// Награды за достижение
	ExperienceReward int // Опыт за получение достижения
	GoldReward       int // Золото за получение достижения

	// Метаданные
	Icon         string // Эмодзи или иконка достижения
	Category     string // Категория (для группировки)
	IsHidden     bool   // Скрытое достижение (показывается только после получения)
	IsRepeatable bool   // Повторяемое достижение (можно получить несколько раз)

	CreatedAt time.Time
	UpdatedAt time.Time
}

// PlayerAchievement представляет достижение, полученное игроком
type PlayerAchievement struct {
	ID            uint        `gorm:"primaryKey"`
	PlayerID      uint        `gorm:"index;not null"` // Связь с Player
	AchievementID uint        `gorm:"index;not null"` // Связь с Achievement
	Achievement   Achievement `gorm:"foreignKey:AchievementID"`

	// Прогресс к достижению (для достижений с прогрессом)
	Progress int `gorm:"default:0"` // Текущий прогресс (например, 7 из 10)

	// Метаданные
	EarnedAt    time.Time `gorm:"type:timestamp;not null"` // Время получения
	EarnedCount int       `gorm:"default:1"`               // Количество получений (для повторяемых)

	CreatedAt time.Time
	UpdatedAt time.Time
}

// AchievementProgress отслеживает прогресс игрока к достижению
type AchievementProgress struct {
	ID            uint `gorm:"primaryKey"`
	PlayerID      uint `gorm:"index;not null"`
	AchievementID uint `gorm:"index;not null"`

	// Текущий прогресс
	CurrentValue int `gorm:"default:0"`

	// Флаг получения
	IsCompleted bool `gorm:"default:false"`

	UpdatedAt time.Time
}

// IsCompleted проверяет, выполнено ли требование достижения
func (a *Achievement) IsCompleted(currentValue int) bool {
	return currentValue >= a.RequirementValue
}

// GetProgressPercentage возвращает процент выполнения достижения
func (a *Achievement) GetProgressPercentage(currentValue int) int {
	if a.RequirementValue <= 0 {
		return 100
	}
	percentage := (currentValue * 100) / a.RequirementValue
	if percentage > 100 {
		return 100
	}
	return percentage
}
