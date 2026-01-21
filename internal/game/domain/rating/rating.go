package rating

import "time"

// RatingMetricType тип метрики для рейтинга
type RatingMetricType string

const (
	MetricTypeLevel           RatingMetricType = "level"            // Уровень персонажа
	MetricTypeExperience      RatingMetricType = "experience"       // Общий опыт
	MetricTypeCombatWins      RatingMetricType = "combat_wins"      // Победы в боях
	MetricTypeQuestsCompleted RatingMetricType = "quests_completed" // Завершенные квесты
	MetricTypeTotalRating     RatingMetricType = "total_rating"     // Общий рейтинг (комплексный)
)

// PlayerRating представляет рейтинг игрока по различным метрикам
type PlayerRating struct {
	ID         uint             `gorm:"primaryKey"`
	TgUserID   int64            `gorm:"uniqueIndex:idx_rating_user_metric;index"`
	MetricType RatingMetricType `gorm:"type:varchar(32);not null;uniqueIndex:idx_rating_user_metric"`

	// Значения метрик (берется из PlayerStats)
	Level           int `gorm:"default:0"`
	Experience      int `gorm:"default:0"`
	CombatWins      int `gorm:"default:0"`
	QuestsCompleted int `gorm:"default:0"`

	// Рассчитанный рейтинг (для сортировки в лидерборде)
	RatingScore int `gorm:"default:0;index"`

	// Ранги (позиция в лидерборде)
	Rank         int `gorm:"default:0"` // Текущий ранг по этой метрике
	PreviousRank int `gorm:"default:0"` // Предыдущий ранг (для отслеживания изменений)

	// Временные метки
	LastUpdated time.Time `gorm:"index"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// PlayerStats представляет текущие статистики игрока для расчета рейтинга
// Эти данные агрегируются из различных источников (Character, Combat, Quest)
type PlayerStats struct {
	TgUserID        int64
	Level           int
	Experience      int
	CombatWins      int
	QuestsCompleted int
}

// CalculateTotalRating рассчитывает общий рейтинг на основе всех метрик
// Использует весовые коэффициенты для разных метрик
func (r *PlayerRating) CalculateTotalRating() int {
	// Весовые коэффициенты для разных метрик
	const (
		levelWeight           = 100 // Уровень - самый важный
		experienceWeight      = 1   // Опыт напрямую влияет
		combatWinsWeight      = 50  // Победы в боях
		questsCompletedWeight = 30  // Завершенные квесты
	)

	score := 0
	score += r.Level * levelWeight
	score += r.Experience * experienceWeight
	score += r.CombatWins * combatWinsWeight
	score += r.QuestsCompleted * questsCompletedWeight

	return score
}

// UpdateFromStats обновляет рейтинг из статистики игрока
func (r *PlayerRating) UpdateFromStats(stats PlayerStats) {
	r.Level = stats.Level
	r.Experience = stats.Experience
	r.CombatWins = stats.CombatWins
	r.QuestsCompleted = stats.QuestsCompleted

	// Рассчитываем общий рейтинг для метрики total_rating
	if r.MetricType == MetricTypeTotalRating {
		r.RatingScore = r.CalculateTotalRating()
	} else {
		// Для конкретных метрик рейтинг = значение метрики
		switch r.MetricType {
		case MetricTypeLevel:
			r.RatingScore = stats.Level
		case MetricTypeExperience:
			r.RatingScore = stats.Experience
		case MetricTypeCombatWins:
			r.RatingScore = stats.CombatWins
		case MetricTypeQuestsCompleted:
			r.RatingScore = stats.QuestsCompleted
		}
	}

	r.LastUpdated = time.Now()
}

// LeaderboardEntry представляет запись в лидерборде
type LeaderboardEntry struct {
	Rank        int // Позиция в рейтинге
	TgUserID    int64
	PlayerName  string
	RatingScore int
	MetricValue int // Значение метрики (для отображения)
}
