package session

import (
	"context"
	"time"

	"dungeons-and-dragons-ai/internal/game/domain/player"
	"dungeons-and-dragons-ai/internal/game/domain/world"

	"gorm.io/gorm"
)

// Companion представляет NPC компаньона в отряде игрока
type Companion struct {
	ID          uint   `gorm:"primaryKey"`
	GameSessionID uint `gorm:"index"`
	Name        string
	Description string
	Class       string
	Level       int
	HP          int
	MaxHP       int
	AC          int
	AttackBonus int
	DamageDice  string

	CreatedAt time.Time
	UpdatedAt time.Time
}

type State string

const (
	StateActive State = "active"
	StateDone   State = "done"
	StateFailed State = "failed"
	StatePaused State = "paused"
)

type GameSession struct {
	gorm.Model
	ChatID int64 `gorm:"uniqueIndex"`
	State  State

	World   world.World
	WorldID uint

	// CurrentLocationID — текущая локация игрока в мире (для карты и навигации)
	// nil означает "не установлено" (будет инициализировано первой локацией мира)
	CurrentLocationID *uint `gorm:"index"`

	// MapImagePath — путь к сгенерированной "красивой" карте мира (изображение)
	MapImagePath string `gorm:"type:text"`

	// Pending ability check (ожидаемая проверка характеристики)
	PendingAbilityCheckID          string `gorm:"type:varchar(64);index"`
	PendingAbilityCheckAbility     string `gorm:"type:varchar(32)"`
	PendingAbilityCheckDC          int
	PendingAbilityCheckRequestedAt *time.Time
	PendingAbilityCheckNotified    bool

	// Adaptive difficulty statistics (адаптивная сложность)
	SessionSuccessCount   int `gorm:"default:0"` // Количество успешных проверок в сессии
	SessionFailureCount   int `gorm:"default:0"` // Количество провальных проверок в сессии
	SessionChecksCount    int `gorm:"default:0"` // Общее количество проверок в сессии
	SessionDifficultyMod  int `gorm:"default:0"` // Модификатор сложности для сессии (-2 до +2)

	Players    []player.Player `gorm:"foreignKey:GameSessionID"`
	Companions []Companion     `gorm:"foreignKey:GameSessionID"` // NPC компаньоны игрока
}

func (s *GameSession) IsActive() bool {
	return s.State == StateActive
}

// End завершает игру, устанавливая статус в StateDone
func (s *GameSession) End() {
	s.State = StateDone
}

// FindPlayerByTgUserID находит игрока по Telegram User ID
// В приватных чатах chatID == userID, поэтому это работает для приватных чатов
// Для групповых чатов нужно передавать userID явно
func (s *GameSession) FindPlayerByTgUserID(tgUserID int64) *player.Player {
	for i := range s.Players {
		if s.Players[i].TgUserID == tgUserID {
			return &s.Players[i]
		}
	}
	return nil
}

// GetFirstPlayer возвращает первого игрока (для обратной совместимости)
// В будущем для мультиплеера лучше использовать FindPlayerByTgUserID
func (s *GameSession) GetFirstPlayer() *player.Player {
	if len(s.Players) == 0 {
		return nil
	}
	return &s.Players[0]
}

// AddCompanion добавляет компаньона в отряд
func (s *GameSession) AddCompanion(companion *Companion) {
	s.Companions = append(s.Companions, *companion)
}

// RemoveCompanion удаляет компаньона из отряда по ID
func (s *GameSession) RemoveCompanion(companionID uint) bool {
	for i, companion := range s.Companions {
		if companion.ID == companionID {
			s.Companions = append(s.Companions[:i], s.Companions[i+1:]...)
			return true
		}
	}
	return false
}

// GetCompanionByID находит компаньона по ID
func (s *GameSession) GetCompanionByID(companionID uint) *Companion {
	for i := range s.Companions {
		if s.Companions[i].ID == companionID {
			return &s.Companions[i]
		}
	}
	return nil
}

func (s *GameSession) HasPendingAbilityCheck() bool {
	return s.PendingAbilityCheckID != "" && s.PendingAbilityCheckAbility != "" && s.PendingAbilityCheckDC > 0
}

func (s *GameSession) SetPendingAbilityCheck(checkID, ability string, dc int) {
	now := time.Now()
	s.PendingAbilityCheckID = checkID
	s.PendingAbilityCheckAbility = ability
	s.PendingAbilityCheckDC = dc
	s.PendingAbilityCheckRequestedAt = &now
	s.PendingAbilityCheckNotified = false
}

func (s *GameSession) ClearPendingAbilityCheck() {
	s.PendingAbilityCheckID = ""
	s.PendingAbilityCheckAbility = ""
	s.PendingAbilityCheckDC = 0
	s.PendingAbilityCheckRequestedAt = nil
	s.PendingAbilityCheckNotified = false
}

// RecordAbilityCheckResult записывает результат проверки навыка для адаптивной сложности
func (s *GameSession) RecordAbilityCheckResult(success bool) {
	s.SessionChecksCount++
	if success {
		s.SessionSuccessCount++
	} else {
		s.SessionFailureCount++
	}
	s.updateDifficultyModifier()
}

// updateDifficultyModifier обновляет модификатор сложности на основе статистики сессии
func (s *GameSession) updateDifficultyModifier() {
	if s.SessionChecksCount < 3 {
		// Недостаточно данных для адаптации
		s.SessionDifficultyMod = 0
		return
	}

	successRate := float64(s.SessionSuccessCount) / float64(s.SessionChecksCount)

	// Адаптивная логика:
	// - Если успехов > 80%, делаем сложнее (+1)
	// - Если успехов > 90%, делаем еще сложнее (+2)
	// - Если успехов < 30%, делаем легче (-1)
	// - Если успехов < 15%, делаем еще легче (-2)
	// - Иначе оставляем без изменений (0)

	if successRate > 0.9 {
		s.SessionDifficultyMod = 2
	} else if successRate > 0.8 {
		s.SessionDifficultyMod = 1
	} else if successRate < 0.15 {
		s.SessionDifficultyMod = -2
	} else if successRate < 0.3 {
		s.SessionDifficultyMod = -1
	} else {
		s.SessionDifficultyMod = 0
	}
}

// GetAdaptiveDC возвращает DC с учетом адаптивной сложности
func (s *GameSession) GetAdaptiveDC(baseDC int) int {
	adaptiveDC := baseDC + s.SessionDifficultyMod

	// Ограничиваем DC в разумных пределах (8-20 для типичных проверок)
	if adaptiveDC < 8 {
		adaptiveDC = 8
	}
	if adaptiveDC > 20 {
		adaptiveDC = 20
	}

	return adaptiveDC
}

// GetDifficultyDescription возвращает текстовое описание текущей сложности
func (s *GameSession) GetDifficultyDescription() string {
	if s.SessionChecksCount < 3 {
		return "адаптация в процессе"
	}

	switch s.SessionDifficultyMod {
	case -2:
		return "очень легко"
	case -1:
		return "легко"
	case 0:
		return "нормально"
	case 1:
		return "сложно"
	case 2:
		return "очень сложно"
	default:
		return "нормально"
	}
}

type Repository interface {
	GetByChatID(ctx context.Context, chatID int64) (*GameSession, error)
	Save(ctx context.Context, session *GameSession) error
	Delete(ctx context.Context, chatID int64) error
}
