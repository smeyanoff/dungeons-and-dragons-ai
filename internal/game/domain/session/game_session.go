package session

import (
	"context"
	"fmt"
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

type SessionGoalType string

const (
	GoalTypeExploration  SessionGoalType = "exploration"  // Исследовать определенное количество локаций
	GoalTypeCombat       SessionGoalType = "combat"       // Победить определенное количество врагов
	GoalTypeExperience   SessionGoalType = "experience"   // Получить определенное количество опыта
	GoalTypeTimeLimit    SessionGoalType = "time_limit"   // Закончить квест за определенное время
	GoalTypeAchievement  SessionGoalType = "achievement"  // Получить определенное достижение
)

type SessionGoalStatus string

const (
	GoalStatusActive    SessionGoalStatus = "active"
	GoalStatusCompleted SessionGoalStatus = "completed"
	GoalStatusFailed    SessionGoalStatus = "failed"
	GoalStatusExpired   SessionGoalStatus = "expired"
)

// SessionGoal представляет цель сессии игры
type SessionGoal struct {
	ID          uint              `gorm:"primaryKey"`
	GameSessionID uint            `gorm:"index"`
	Type        SessionGoalType   `gorm:"type:varchar(32);not null"`
	Description string            `gorm:"type:text"`
	Status      SessionGoalStatus `gorm:"type:varchar(16);not null;default:'active'"`

	// Параметры цели
	TargetValue   int `gorm:"default:0"`    // Целевое значение (количество локаций, опыт, etc.)
	CurrentValue  int `gorm:"default:0"`    // Текущее значение прогресса
	TimeLimit     *time.Time `gorm:"index"` // Ограничение по времени (опционально)

	// Метаданные
	CreatedAt time.Time
	UpdatedAt time.Time
}

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

	// Cooperative play fields
	IsCooperative     bool `gorm:"default:false"` // Включает режим совместной игры
	ActivePlayerID    *uint `gorm:"index"`        // ID активного игрока (для очередности ходов)
	MaxPlayers        int  `gorm:"default:1"`     // Максимальное количество игроков (1-3)
	PlayerTurnOrder   []uint `gorm:"-"`          // Порядок ходов игроков (в памяти, не в БД)

	// Pending ability check (ожидаемая проверка характеристики)
	PendingAbilityCheckID          string `gorm:"type:varchar(64);index"`
	PendingAbilityCheckAbility     string `gorm:"type:varchar(32)"`
	PendingAbilityCheckDC          int
	PendingAbilityCheckReason      string `gorm:"type:text"` // Причина проверки (что делает игрок)
	PendingAbilityCheckStakes      string `gorm:"type:text"` // Ставки (что на кону)
	PendingAbilityCheckRequestedAt *time.Time
	PendingAbilityCheckNotified    bool

	// Adaptive difficulty statistics (адаптивная сложность)
	SessionSuccessCount   int `gorm:"default:0"` // Количество успешных проверок в сессии
	SessionFailureCount   int `gorm:"default:0"` // Количество провальных проверок в сессии
	SessionChecksCount    int `gorm:"default:0"` // Общее количество проверок в сессии
	SessionDifficultyMod  int `gorm:"default:0"` // Модификатор сложности для сессии (-2 до +2)

	SessionGoals []SessionGoal `gorm:"foreignKey:GameSessionID"` // Цели текущей сессии

	// Cooldown система для ability checks
	AbilityCooldowns map[string]*time.Time `gorm:"-"` // Временные метки cooldown по типам проверок (не сохраняется в БД)

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

// GenerateSessionGoals генерирует случайные цели для сессии
func (s *GameSession) GenerateSessionGoals() {
	if len(s.SessionGoals) > 0 {
		// Цели уже сгенерированы
		return
	}

	now := time.Now()

	// Генерируем 2-3 цели разных типов
	goalTypes := []SessionGoalType{GoalTypeExploration, GoalTypeCombat, GoalTypeExperience}
	selectedTypes := make([]SessionGoalType, 0)

	// Выбираем 2-3 типа целей случайным образом
	for len(selectedTypes) < 2 || (len(selectedTypes) < 3 && time.Now().UnixNano()%2 == 0) {
		goalType := goalTypes[time.Now().UnixNano()%int64(len(goalTypes))]
		found := false
		for _, existing := range selectedTypes {
			if existing == goalType {
				found = true
				break
			}
		}
		if !found {
			selectedTypes = append(selectedTypes, goalType)
		}
	}

	// Создаем цели для выбранных типов
	for _, goalType := range selectedTypes {
		goal := SessionGoal{
			GameSessionID: s.ID,
			Type:          goalType,
			Status:        GoalStatusActive,
			CreatedAt:     now,
			UpdatedAt:     now,
		}

		switch goalType {
		case GoalTypeExploration:
			goal.TargetValue = 3 + int(time.Now().UnixNano()%3) // 3-5 локаций
			goal.Description = fmt.Sprintf("Исследовать %d новых локаций", goal.TargetValue)
			// Добавляем таймер для exploration целей (2-3 часа)
			timeLimit := now.Add(time.Duration(2 + int(time.Now().UnixNano()%2)) * time.Hour)
			goal.TimeLimit = &timeLimit
		case GoalTypeCombat:
			goal.TargetValue = 2 + int(time.Now().UnixNano()%3) // 2-4 побед в бою
			goal.Description = fmt.Sprintf("Победить в %d боях", goal.TargetValue)
			// Таймер для combat целей (1-2 часа)
			timeLimit := now.Add(time.Duration(1 + int(time.Now().UnixNano()%2)) * time.Hour)
			goal.TimeLimit = &timeLimit
		case GoalTypeExperience:
			goal.TargetValue = 500 + int(time.Now().UnixNano()%1000) // 500-1500 опыта
			goal.Description = fmt.Sprintf("Получить %d опыта", goal.TargetValue)
			// Таймер для experience целей (3-4 часа)
			timeLimit := now.Add(time.Duration(3 + int(time.Now().UnixNano()%2)) * time.Hour)
			goal.TimeLimit = &timeLimit
		}

		s.SessionGoals = append(s.SessionGoals, goal)
	}
}

// UpdateGoalProgress обновляет прогресс целей сессии
func (s *GameSession) UpdateGoalProgress(goalType SessionGoalType, increment int) {
	for i := range s.SessionGoals {
		goal := &s.SessionGoals[i]
		if goal.Type == goalType && goal.Status == GoalStatusActive {
			goal.CurrentValue += increment
			goal.UpdatedAt = time.Now()

			// Проверяем завершение цели
			if goal.CurrentValue >= goal.TargetValue {
				goal.Status = GoalStatusCompleted
			}
		}
	}
}

// GetActiveGoals возвращает активные цели сессии
func (s *GameSession) GetActiveGoals() []SessionGoal {
	activeGoals := make([]SessionGoal, 0)
	for _, goal := range s.SessionGoals {
		if goal.Status == GoalStatusActive {
			activeGoals = append(activeGoals, goal)
		}
	}
	return activeGoals
}

// GetCompletedGoals возвращает завершенные цели сессии
func (s *GameSession) GetCompletedGoals() []SessionGoal {
	completedGoals := make([]SessionGoal, 0)
	for _, goal := range s.SessionGoals {
		if goal.Status == GoalStatusCompleted {
			completedGoals = append(completedGoals, goal)
		}
	}
	return completedGoals
}

// InitializeCooldowns инициализирует карту cooldown
func (s *GameSession) InitializeCooldowns() {
	if s.AbilityCooldowns == nil {
		s.AbilityCooldowns = make(map[string]*time.Time)
	}
}

// EnableCooperativeMode включает режим совместной игры
func (s *GameSession) EnableCooperativeMode(maxPlayers int) {
	s.IsCooperative = true
	s.MaxPlayers = maxPlayers
	if s.MaxPlayers < 1 {
		s.MaxPlayers = 1
	}
	if s.MaxPlayers > 3 {
		s.MaxPlayers = 3
	}
	s.PlayerTurnOrder = make([]uint, 0, s.MaxPlayers)
}

// DisableCooperativeMode отключает режим совместной игры
func (s *GameSession) DisableCooperativeMode() {
	s.IsCooperative = false
	s.MaxPlayers = 1
	s.ActivePlayerID = nil
	s.PlayerTurnOrder = nil
}

// AddPlayerToSession добавляет игрока в сессию
func (s *GameSession) AddPlayerToSession(player *player.Player) error {
	if !s.IsCooperative && len(s.Players) > 0 {
		return fmt.Errorf("session is not in cooperative mode")
	}

	if len(s.Players) >= s.MaxPlayers {
		return fmt.Errorf("maximum players reached (%d)", s.MaxPlayers)
	}

	// Проверяем, не добавлен ли уже этот игрок
	for _, p := range s.Players {
		if p.TgUserID == player.TgUserID {
			return fmt.Errorf("player already in session")
		}
	}

	s.Players = append(s.Players, *player)
	s.PlayerTurnOrder = append(s.PlayerTurnOrder, player.ID)

	// Если это первый игрок, делаем его активным
	if s.ActivePlayerID == nil && len(s.Players) > 0 {
		s.ActivePlayerID = &s.Players[0].ID
	}

	return nil
}

// GetActivePlayer возвращает активного игрока (того, чья очередь ходить)
func (s *GameSession) GetActivePlayer() *player.Player {
	if s.ActivePlayerID == nil {
		return s.GetFirstPlayer()
	}

	for i := range s.Players {
		if s.Players[i].ID == *s.ActivePlayerID {
			return &s.Players[i]
		}
	}
	return nil
}

// NextPlayerTurn переключает ход к следующему игроку
func (s *GameSession) NextPlayerTurn() {
	if !s.IsCooperative || len(s.PlayerTurnOrder) <= 1 {
		return
	}

	if s.ActivePlayerID == nil {
		// Если активный игрок не установлен, устанавливаем первого
		if len(s.PlayerTurnOrder) > 0 {
			s.ActivePlayerID = &s.PlayerTurnOrder[0]
		}
		return
	}

	// Находим текущего активного игрока в порядке ходов
	currentIndex := -1
	for i, playerID := range s.PlayerTurnOrder {
		if playerID == *s.ActivePlayerID {
			currentIndex = i
			break
		}
	}

	if currentIndex == -1 {
		// Активный игрок не найден в порядке, сбрасываем на первого
		s.ActivePlayerID = &s.PlayerTurnOrder[0]
		return
	}

	// Переключаемся к следующему игроку
	nextIndex := (currentIndex + 1) % len(s.PlayerTurnOrder)
	s.ActivePlayerID = &s.PlayerTurnOrder[nextIndex]
}

// IsPlayerTurn проверяет, является ли ход указанного игрока
func (s *GameSession) IsPlayerTurn(tgUserID int64) bool {
	if !s.IsCooperative {
		return true // В одиночной игре всегда "ход" игрока
	}

	activePlayer := s.GetActivePlayer()
	if activePlayer == nil {
		return false
	}

	return activePlayer.TgUserID == tgUserID
}

// IsAbilityOnCooldown проверяет, находится ли способность на cooldown
func (s *GameSession) IsAbilityOnCooldown(abilityType string, cooldownDuration time.Duration) (bool, time.Duration) {
	s.InitializeCooldowns()

	if cooldownTime, exists := s.AbilityCooldowns[abilityType]; exists && cooldownTime != nil {
		timeSinceLastUse := time.Since(*cooldownTime)
		if timeSinceLastUse < cooldownDuration {
			remainingTime := cooldownDuration - timeSinceLastUse
			return true, remainingTime
		}
	}
	return false, 0
}

// SetAbilityCooldown устанавливает cooldown для способности
func (s *GameSession) SetAbilityCooldown(abilityType string) {
	s.InitializeCooldowns()
	now := time.Now()
	s.AbilityCooldowns[abilityType] = &now
}

// ClearExpiredCooldowns очищает истекшие cooldown
func (s *GameSession) ClearExpiredCooldowns(cooldownDuration time.Duration) {
	s.InitializeCooldowns()

	now := time.Now()
	for abilityType, cooldownTime := range s.AbilityCooldowns {
		if cooldownTime != nil && now.Sub(*cooldownTime) >= cooldownDuration {
			delete(s.AbilityCooldowns, abilityType)
		}
	}
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
	s.PendingAbilityCheckReason = ""
	s.PendingAbilityCheckStakes = ""
	s.PendingAbilityCheckRequestedAt = &now
	s.PendingAbilityCheckNotified = false
}

func (s *GameSession) SetPendingAbilityCheckWithContext(checkID, ability string, dc int, reason, stakes string) {
	now := time.Now()
	s.PendingAbilityCheckID = checkID
	s.PendingAbilityCheckAbility = ability
	s.PendingAbilityCheckDC = dc
	s.PendingAbilityCheckReason = reason
	s.PendingAbilityCheckStakes = stakes
	s.PendingAbilityCheckRequestedAt = &now
	s.PendingAbilityCheckNotified = false
}

func (s *GameSession) ClearPendingAbilityCheck() {
	s.PendingAbilityCheckID = ""
	s.PendingAbilityCheckAbility = ""
	s.PendingAbilityCheckDC = 0
	s.PendingAbilityCheckReason = ""
	s.PendingAbilityCheckStakes = ""
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
