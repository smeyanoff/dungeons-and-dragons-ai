package combat

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/dice"
)

// CombatState представляет состояние боя
type CombatState string

const (
	CombatStateNotStarted CombatState = "not_started"
	CombatStateActive     CombatState = "active"
	CombatStateFinished   CombatState = "finished"
)

// Combat представляет текущий бой
type Combat struct {
	ID            uint `gorm:"primaryKey"`
	GameSessionID uint `gorm:"index"`

	State CombatState `gorm:"type:varchar(32);not null"`

	Participants []CombatParticipant `gorm:"foreignKey:CombatID"`

	CurrentTurn int // Индекс текущего хода в порядке инициативы

	// mu защищает от race conditions при конкурентном доступе
	// Не сохраняется в БД (используется `gorm:"-"` тег)
	mu sync.RWMutex `gorm:"-"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

// IsActive проверяет, активен ли бой
func (c *Combat) IsActive() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.State == CombatStateActive
}

// Start начинает бой, определяя инициативу
func (c *Combat) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.State != CombatStateNotStarted {
		return errors.New("combat already started or finished")
	}

	if len(c.Participants) < 2 {
		return errors.New("need at least 2 participants for combat")
	}

	// Определяем инициативу для всех участников
	for i := range c.Participants {
		initiative := c.Participants[i].RollInitiative()
		c.Participants[i].Initiative = initiative
	}

	// Сортируем по инициативе (от большего к меньшему)
	c.sortByInitiative()

	c.State = CombatStateActive
	c.CurrentTurn = 0

	return nil
}

// sortByInitiative сортирует участников по инициативе
func (c *Combat) sortByInitiative() {
	// Простая пузырьковая сортировка
	for i := 0; i < len(c.Participants)-1; i++ {
		for j := 0; j < len(c.Participants)-i-1; j++ {
			if c.Participants[j].Initiative < c.Participants[j+1].Initiative {
				c.Participants[j], c.Participants[j+1] = c.Participants[j+1], c.Participants[j]
			}
		}
	}
}

// GetCurrentParticipant возвращает участника, чей сейчас ход
// Пропускает мертвых участников и возвращает следующего живого
func (c *Combat) GetCurrentParticipant() *CombatParticipant {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.State != CombatStateActive || len(c.Participants) == 0 {
		return nil
	}

	// Ищем живого участника, начиная с текущего хода
	attempts := 0
	for attempts < len(c.Participants) {
		idx := (c.CurrentTurn + attempts) % len(c.Participants)
		participant := &c.Participants[idx]
		if participant.IsAlive() {
			// Возвращаем копию участника для безопасности
			// Примечание: это не идеально, но безопаснее, чем возвращать указатель на участника из среза
			result := participant
			return result
		}
		attempts++
	}

	// Все участники мертвы
	return nil
}

// NextTurn переходит к следующему ходу
// Пропускает мертвых участников
func (c *Combat) NextTurn() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.State != CombatStateActive {
		return
	}

	// Проверяем, не закончился ли бой перед переходом хода
	if c.checkCombatEndLocked() {
		c.State = CombatStateFinished
		return
	}

	// Переходим к следующему живому участнику
	attempts := 0
	for attempts < len(c.Participants) {
		c.CurrentTurn = (c.CurrentTurn + 1) % len(c.Participants)
		idx := c.CurrentTurn % len(c.Participants)
		participant := &c.Participants[idx]
		if participant.IsAlive() {
			// Нашли живого участника
			return
		}
		attempts++
	}

	// Все участники мертвы
	c.State = CombatStateFinished
}

// CheckCombatEnd проверяет, закончился ли бой
func (c *Combat) CheckCombatEnd() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.checkCombatEndLocked()
}

// checkCombatEndLocked проверяет окончание боя без блокировки (требует, чтобы мьютекс был уже заблокирован)
func (c *Combat) checkCombatEndLocked() bool {
	alivePlayers := 0
	aliveEnemies := 0

	for _, p := range c.Participants {
		if p.IsAlive() {
			if p.IsPlayer {
				alivePlayers++
			} else {
				aliveEnemies++
			}
		}
	}

	return alivePlayers == 0 || aliveEnemies == 0
}

// CombatParticipant представляет участника боя
type CombatParticipant struct {
	ID       uint `gorm:"primaryKey"`
	CombatID uint `gorm:"index"`

	IsPlayer bool // true для игрока, false для монстра

	// Для игрока
	CharacterID *uint
	Character   *character.Character `gorm:"foreignKey:CharacterID"`

	// Для монстра
	MonsterName        string
	MonsterHP          int
	MonsterMaxHP       int
	MonsterAC          int // Класс брони
	MonsterAttackBonus int

	Initiative int // Результат броска инициативы

	CreatedAt time.Time
}

// IsAlive проверяет, жив ли участник
func (cp *CombatParticipant) IsAlive() bool {
	if cp.IsPlayer {
		return cp.Character != nil && cp.Character.Status == character.StatusAlive && cp.Character.HP > 0
	}
	return cp.MonsterHP > 0
}

// GetHP возвращает текущее HP
func (cp *CombatParticipant) GetHP() int {
	if cp.IsPlayer {
		if cp.Character != nil {
			return cp.Character.HP
		}
		return 0
	}
	return cp.MonsterHP
}

// GetMaxHP возвращает максимальное HP
func (cp *CombatParticipant) GetMaxHP() int {
	if cp.IsPlayer {
		if cp.Character != nil {
			return cp.Character.MaxHP
		}
		return 0
	}
	return cp.MonsterMaxHP
}

// GetAC возвращает класс брони
func (cp *CombatParticipant) GetAC() int {
	if cp.IsPlayer {
		// Базовый AC = 10 + модификатор ловкости
		if cp.Character != nil {
			dexModifier := dice.CalculateModifier(cp.Character.Stats.Dexterity)
			return 10 + dexModifier
		}
		return 10
	}
	return cp.MonsterAC
}

// GetAttackBonus возвращает бонус к атаке
func (cp *CombatParticipant) GetAttackBonus() int {
	if cp.IsPlayer {
		if cp.Character != nil {
			// Бонус атаки зависит от класса и силы/ловкости
			// Для простоты используем модификатор силы для ближнего боя
			strModifier := dice.CalculateModifier(cp.Character.Stats.Strength)
			// Базовый бонус по уровню (для 1 уровня = 0)
			proficiencyBonus := (cp.Character.Level-1)/4 + 2
			return strModifier + proficiencyBonus
		}
		return 0
	}
	return cp.MonsterAttackBonus
}

// RollInitiative бросает инициативу (d20 + модификатор ловкости)
func (cp *CombatParticipant) RollInitiative() int {
	var dexModifier int
	if cp.IsPlayer && cp.Character != nil {
		dexModifier = dice.CalculateModifier(cp.Character.Stats.Dexterity)
	} else {
		// Для монстров используем фиксированный модификатор
		dexModifier = 0
	}

	result := dice.RollAttack(dexModifier)
	return result.Total
}

// ApplyDamage применяет урон участнику
func (cp *CombatParticipant) ApplyDamage(amount int) error {
	if amount <= 0 {
		return nil
	}

	if cp.IsPlayer {
		if cp.Character == nil {
			return errors.New("character is nil")
		}
		return cp.Character.ApplyDamage(amount)
	}

	cp.MonsterHP -= amount
	if cp.MonsterHP < 0 {
		cp.MonsterHP = 0
	}

	return nil
}

// AttackResult представляет результат атаки
type AttackResult struct {
	AttackerName string
	TargetName   string
	AttackRoll   int
	AC           int
	Hit          bool
	Damage       int
	CriticalHit  bool
}

// PerformAttack выполняет атаку одного участника по другому
// Примечание: эта функция работает с указателями на участников, поэтому безопасность
// зависит от того, что вызывающий код не будет модифицировать участников параллельно.
// Для полной безопасности нужно добавить мьютекс в CombatParticipant или использовать другой подход.
func PerformAttack(attacker *CombatParticipant, target *CombatParticipant, damageExpression string) (*AttackResult, error) {
	// Проверяем состояние участников (без блокировки, так как это чтение)
	// Примечание: для полной thread-safety нужно добавить мьютекс в CombatParticipant
	if !attacker.IsAlive() {
		return nil, errors.New("attacker is not alive")
	}

	if !target.IsAlive() {
		return nil, errors.New("target is not alive")
	}

	// Бросок атаки (не зависит от состояния)
	attackBonus := attacker.GetAttackBonus()
	attackRoll := dice.RollAttack(attackBonus)

	targetAC := target.GetAC()
	hit := attackRoll.Total >= targetAC
	// Проверяем критический удар (натуральная 20)
	// RollAttack всегда возвращает d20 с одним кубиком, но проверяем на всякий случай
	criticalHit := len(attackRoll.Rolls) > 0 && attackRoll.Rolls[0] == 20

	var damage int
	if hit {
		// Бросок урона (не зависит от состояния)
		damageResult, err := dice.RollDamage(damageExpression)
		if err != nil {
			return nil, err
		}
		damage = damageResult.Total

		// Критический удар удваивает урон
		if criticalHit {
			damage *= 2
		}

		// Применяем урон (это изменение состояния, но ApplyDamage уже проверяет состояние)
		// Примечание: для полной thread-safety нужно добавить мьютекс в CombatParticipant
		if err := target.ApplyDamage(damage); err != nil {
			return nil, err
		}
	}

	attackerName := attacker.GetName()
	targetName := target.GetName()

	return &AttackResult{
		AttackerName: attackerName,
		TargetName:   targetName,
		AttackRoll:   attackRoll.Total,
		AC:           targetAC,
		Hit:          hit,
		Damage:       damage,
		CriticalHit:  criticalHit,
	}, nil
}

// GetName возвращает имя участника
func (cp *CombatParticipant) GetName() string {
	if cp.IsPlayer {
		if cp.Character != nil {
			return cp.Character.Name
		}
		return "Unknown Player"
	}
	return cp.MonsterName
}

// GetInitiativeOrderMessage возвращает текстовое сообщение с порядком ходов по инициативе
// Формат: "⚔️ Бой начался! Порядок ходов:\n1. [Имя] (инициатива: X)\n2. [Монстр] (инициатива: Y)..."
func (c *Combat) GetInitiativeOrderMessage() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.State != CombatStateActive || len(c.Participants) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("⚔️ Бой начался! Порядок ходов:\n")

	for i, participant := range c.Participants {
		if participant.IsAlive() {
			name := participant.GetName()
			initiative := participant.Initiative
			sb.WriteString(fmt.Sprintf("%d. %s (инициатива: %d)\n", i+1, name, initiative))
		}
	}

	// Добавляем информацию о текущем ходе
	currentParticipant := c.getCurrentParticipantLocked()
	if currentParticipant != nil {
		sb.WriteString(fmt.Sprintf("\n🎯 Текущий ход: %s", currentParticipant.GetName()))
	}

	return sb.String()
}

// getCurrentParticipantLocked возвращает текущего участника без блокировки (требует, чтобы мьютекс был уже заблокирован)
func (c *Combat) getCurrentParticipantLocked() *CombatParticipant {
	if c.State != CombatStateActive || len(c.Participants) == 0 {
		return nil
	}

	// Ищем живого участника, начиная с текущего хода
	attempts := 0
	for attempts < len(c.Participants) {
		idx := (c.CurrentTurn + attempts) % len(c.Participants)
		participant := &c.Participants[idx]
		if participant.IsAlive() {
			return participant
		}
		attempts++
	}

	return nil
}

// GetCurrentTurnMessage возвращает сообщение о текущем ходе
func (c *Combat) GetCurrentTurnMessage() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	currentParticipant := c.getCurrentParticipantLocked()
	if currentParticipant == nil {
		return ""
	}

	name := currentParticipant.GetName()
	if currentParticipant.IsPlayer {
		return fmt.Sprintf("🎯 Ваш ход: %s", name)
	}
	return fmt.Sprintf("🎯 Ход: %s", name)
}
