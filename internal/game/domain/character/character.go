package character

import (
	"errors"
	
	"dungeons-and-dragons-ai/internal/game/domain/spell"
)

type Race string

const (
	RaceHuman    Race = "human"
	RaceElf      Race = "elf"
	RaceDwarf    Race = "dwarf"
	RaceOrc      Race = "orc"
	RaceHalfling Race = "halfling"
)

type Class string

const (
	ClassFighter Class = "fighter"
	ClassWizard  Class = "wizard"
	ClassRogue   Class = "rogue"
	ClassCleric  Class = "cleric"
	ClassRanger  Class = "ranger"
)

type Status string

const (
	StatusAlive Status = "alive"
	StatusDead  Status = "dead"
)

type Character struct {
	ID   uint `gorm:"primaryKey"`
	Name string

	Class Class `gorm:"type:varchar(32);not null"`
	Race  Race  `gorm:"type:varchar(32);not null"`

	Level      int
	Experience int // Опыт персонажа
	HP         int
	MaxHP      int

	Status Status `gorm:"type:varchar(16);not null"`

	Stats Stats
	
	// Система заклинаний (для магических классов)
	SpellSlots spell.SpellSlots `gorm:"embedded;embeddedPrefix:spell_slots_"` // Встраивание структуры SpellSlots в Character
}

func (c *Character) ApplyDamage(amount int) error {
	if c.Status == StatusDead {
		return errors.New("character already dead")
	}
	if amount <= 0 {
		return nil
	}

	c.HP -= amount

	if c.HP <= 0 {
		c.Kill()
	}

	return nil
}

func (c *Character) Kill() {
	c.HP = 0
	c.Status = StatusDead
}

func (c *Character) Heal(amount int) error {
	if c.Status == StatusDead {
		return errors.New("cannot heal dead character")
	}

	c.HP += amount
	if c.HP > c.MaxHP {
		c.HP = c.MaxHP
	}

	return nil
}

func calculateHP(class Class, stats Stats) int {
	// Базовое HP по классу (на 1 уровне) = максимум Hit Die
	// В D&D 5e на 1 уровне HP = максимум Hit Die + модификатор Телосложения
	baseHP := map[Class]int{
		ClassFighter: 10, // d10
		ClassWizard:  6,  // d6
		ClassRogue:   8,  // d8
		ClassCleric:  8,  // d8
		ClassRanger:  10, // d10
	}

	// Модификатор конституции
	conModifier := (stats.Constitution - 10) / 2

	hp := baseHP[class]
	if hp == 0 {
		hp = 8 // значение по умолчанию
	}

	hp = hp + conModifier

	// В D&D 5e минимальное HP на уровне = 1 (даже при очень низком Телосложении)
	if hp < 1 {
		hp = 1
	}

	return hp
}

func NewCharacter(
	name string,
	class Class,
	race Race,
	stats Stats,
) (*Character, error) {

	maxHP := calculateHP(class, stats)

	char := &Character{
		Name:       name,
		Class:      class,
		Race:       race,
		Level:      1,
		Experience: 0,
		HP:         maxHP,
		MaxHP:      maxHP,
		Status:     StatusAlive,
		Stats:      stats,
	}
	
	// Инициализируем слоты заклинаний для магических классов
	char.InitializeSpellSlots()

	return char, nil
}

// AddExperience начисляет опыт персонажу и проверяет повышение уровня
func (c *Character) AddExperience(amount int) (leveledUp bool, err error) {
	if c.Status == StatusDead {
		return false, errors.New("cannot add experience to dead character")
	}
	if amount <= 0 {
		return false, nil
	}

	c.Experience += amount

	// Проверяем, нужно ли повысить уровень
	requiredXP := getRequiredXPForLevel(c.Level + 1)
	if c.Experience >= requiredXP {
		return c.levelUp()
	}

	return false, nil
}

// levelUp повышает уровень персонажа
func (c *Character) levelUp() (bool, error) {
	if c.Status == StatusDead {
		return false, errors.New("cannot level up dead character")
	}

	oldLevel := c.Level
	c.Level++

	// Улучшаем характеристики каждые 4 уровня (4, 8, 12, 16, 20)
	if c.Level%4 == 0 {
		c.improveAbilityScore()
	}

	// Увеличиваем HP
	hpGain := calculateHPGain(c.Class, c.Stats)
	c.MaxHP += hpGain
	c.HP += hpGain // Восстанавливаем HP при повышении уровня

	// Обновляем слоты заклинаний для магических классов
	if c.IsSpellcaster() {
		newSlots := spell.CalculateSpellSlotsForLevel(string(c.Class), c.Level)
		// Сохраняем использованные слоты
		for i := 1; i <= 9; i++ {
			oldUsed := c.SpellSlots.GetUsedSlotsByLevel(i)
			newSlots.SetUsedSlotsByLevel(i, oldUsed)
		}
		c.SpellSlots = *newSlots
	}

	return c.Level > oldLevel, nil
}

// improveAbilityScore улучшает одну характеристику на 1
func (c *Character) improveAbilityScore() {
	// Улучшаем характеристику, которая больше всего подходит классу
	switch c.Class {
	case ClassFighter:
		c.Stats.Strength++
	case ClassWizard:
		c.Stats.Intelligence++
	case ClassRogue:
		c.Stats.Dexterity++
	case ClassCleric:
		c.Stats.Wisdom++
	case ClassRanger:
		c.Stats.Dexterity++
	default:
		c.Stats.Strength++ // По умолчанию
	}
}

// getRequiredXPForLevel возвращает требуемый опыт для уровня
func getRequiredXPForLevel(level int) int {
	// Стандартная таблица опыта D&D 5e (упрощенная)
	xpTable := map[int]int{
		1:  0,
		2:  300,
		3:  900,
		4:  2700,
		5:  6500,
		6:  14000,
		7:  23000,
		8:  34000,
		9:  48000,
		10: 64000,
		11: 85000,
		12: 100000,
		13: 120000,
		14: 140000,
		15: 165000,
		16: 195000,
		17: 225000,
		18: 265000,
		19: 305000,
		20: 355000,
	}

	if xp, ok := xpTable[level]; ok {
		return xp
	}

	// Для уровней выше 20 используем линейную прогрессию
	return 355000 + (level-20)*50000
}

// calculateHPGain возвращает прирост HP при повышении уровня
func calculateHPGain(class Class, stats Stats) int {
	// Hit Die по классу
	hitDie := map[Class]int{
		ClassFighter: 10,
		ClassWizard:  6,
		ClassRogue:   8,
		ClassCleric:  8,
		ClassRanger:  10,
	}

	hd := hitDie[class]
	if hd == 0 {
		hd = 8 // По умолчанию
	}

	// Модификатор конституции
	conModifier := (stats.Constitution - 10) / 2

	// Минимальный прирост HP = 1
	hpGain := hd/2 + 1 + conModifier
	if hpGain < 1 {
		hpGain = 1
	}

	return hpGain
}

// GetExperienceToNextLevel возвращает опыт, необходимый для следующего уровня
func (c *Character) GetExperienceToNextLevel() int {
	requiredXP := getRequiredXPForLevel(c.Level + 1)
	return requiredXP - c.Experience
}

// IsSpellcaster проверяет, является ли персонаж заклинателем
func (c *Character) IsSpellcaster() bool {
	return c.Class == ClassWizard || c.Class == ClassCleric || c.Class == ClassRanger
}

// InitializeSpellSlots инициализирует слоты заклинаний для магического класса
func (c *Character) InitializeSpellSlots() {
	if c.IsSpellcaster() {
		c.SpellSlots = *spell.CalculateSpellSlotsForLevel(string(c.Class), c.Level)
	}
}
