package spell

import (
	"time"
)

// SpellSchool представляет школу магии
type SpellSchool string

const (
	SpellSchoolAbjuration    SpellSchool = "abjuration"    // Ограждение
	SpellSchoolConjuration   SpellSchool = "conjuration"   // Вызов
	SpellSchoolDivination    SpellSchool = "divination"    // Прорицание
	SpellSchoolEnchantment   SpellSchool = "enchantment"   // Очарование
	SpellSchoolEvocation     SpellSchool = "evocation"     // Воплощение
	SpellSchoolIllusion      SpellSchool = "illusion"      // Иллюзия
	SpellSchoolNecromancy    SpellSchool = "necromancy"    // Некромантия
	SpellSchoolTransmutation SpellSchool = "transmutation" // Преобразование
)

// SpellType представляет тип заклинания
type SpellType string

const (
	SpellTypeCantrip SpellType = "cantrip" // Заговор (0 уровень)
	SpellTypeLevel1  SpellType = "level1"  // Заклинание 1 уровня
	SpellTypeLevel2  SpellType = "level2"  // Заклинание 2 уровня
	SpellTypeLevel3  SpellType = "level3"  // Заклинание 3 уровня
	SpellTypeLevel4  SpellType = "level4"  // Заклинание 4 уровня
	SpellTypeLevel5  SpellType = "level5"  // Заклинание 5 уровня
	SpellTypeLevel6  SpellType = "level6"  // Заклинание 6 уровня
	SpellTypeLevel7  SpellType = "level7"  // Заклинание 7 уровня
	SpellTypeLevel8  SpellType = "level8"  // Заклинание 8 уровня
	SpellTypeLevel9  SpellType = "level9"  // Заклинание 9 уровня
)

// CastingTime представляет время каста заклинания
type CastingTime string

const (
	CastingTimeAction    CastingTime = "action"     // 1 действие
	CastingTimeBonus     CastingTime = "bonus"      // Бонусное действие
	CastingTimeReaction  CastingTime = "reaction"   // Реакция
	CastingTimeMinute    CastingTime = "1 minute"   // 1 минута
	CastingTime10Minutes CastingTime = "10 minutes" // 10 минут
	CastingTimeHour      CastingTime = "1 hour"     // 1 час
	CastingTime8Hours    CastingTime = "8 hours"    // 8 часов
)

// SpellRange представляет дистанцию заклинания
type SpellRange string

const (
	RangeTouch     SpellRange = "touch"     // Касание
	RangeSelf      SpellRange = "self"      // На себя
	Range30Feet    SpellRange = "30 feet"   // 30 футов
	Range60Feet    SpellRange = "60 feet"   // 60 футов
	Range90Feet    SpellRange = "90 feet"   // 90 футов
	Range120Feet   SpellRange = "120 feet"  // 120 футов
	RangeSight     SpellRange = "sight"     // В пределах видимости
	RangeUnlimited SpellRange = "unlimited" // Неограниченная
)

// Duration представляет длительность заклинания
type Duration string

const (
	DurationInstantaneous Duration = "instantaneous" // Мгновенное
	DurationConcentration Duration = "concentration" // Концентрация
	DurationRounds        Duration = "rounds"        // Раунды
	DurationMinutes       Duration = "minutes"       // Минуты
	DurationHours         Duration = "hours"         // Часы
	DurationDays          Duration = "days"          // Дни
	DurationUntilDispel   Duration = "until_dispel"  // До рассеивания
)

// Spell представляет заклинание
type Spell struct {
	ID          uint   `gorm:"primaryKey"`
	Code        string `gorm:"uniqueIndex;type:varchar(64);not null"` // Уникальный код заклинания
	Name        string `gorm:"type:varchar(128);not null"`
	Description string `gorm:"type:text"`

	// Уровень и школа магии
	Level  int         `gorm:"not null;default:0"` // Уровень заклинания (0 для заговоров)
	School SpellSchool `gorm:"type:varchar(32);not null"`
	Type   SpellType   `gorm:"type:varchar(16);not null"`

	// Время каста и дистанция
	CastingTime   CastingTime `gorm:"type:varchar(32);not null"`
	Range         SpellRange  `gorm:"type:varchar(32);not null"`
	Duration      Duration    `gorm:"type:varchar(32);not null"`
	DurationValue int         `gorm:"default:0"` // Значение длительности (количество раундов/минут/часов)

	// Компоненты
	Verbal       bool   `gorm:"default:false"`     // Вербальный компонент
	Somatic      bool   `gorm:"default:false"`     // Соматический компонент
	Material     bool   `gorm:"default:false"`     // Материальный компонент
	MaterialCost string `gorm:"type:varchar(256)"` // Описание материального компонента

	// Эффекты заклинания
	Damage   string `gorm:"type:varchar(64)"` // Урон (например, "1d8+3")
	Healing  string `gorm:"type:varchar(64)"` // Лечение (например, "2d4+2")
	Effect   string `gorm:"type:text"`        // Описание эффекта
	SaveType string `gorm:"type:varchar(16)"` // Тип спасброска (например, "DEX", "WIS")
	SaveDC   int    `gorm:"default:0"`        // Сложность спасброска

	// Для каких классов доступно
	AvailableForWizard bool `gorm:"default:false"`
	AvailableForCleric bool `gorm:"default:false"`
	AvailableForRanger bool `gorm:"default:false"`

	// Метаданные
	Icon          string `gorm:"type:varchar(32)"` // Иконка эмодзи
	Ritual        bool   `gorm:"default:false"`    // Ритуальное заклинание
	Concentration bool   `gorm:"default:false"`    // Требует концентрации

	CreatedAt time.Time
	UpdatedAt time.Time
}

// CharacterSpell представляет известное заклинание персонажа
type CharacterSpell struct {
	ID          uint  `gorm:"primaryKey"`
	CharacterID uint  `gorm:"index;not null"`
	SpellID     uint  `gorm:"index;not null"`
	Spell       Spell `gorm:"foreignKey:SpellID"`

	Prepared  bool      `gorm:"default:false"`           // Подготовлено ли заклинание (для подготовленных заклинаний)
	LearnedAt time.Time `gorm:"type:timestamp;not null"` // Когда выучено

	CreatedAt time.Time
	UpdatedAt time.Time
}

// SpellSlot представляет слот заклинания персонажа
type SpellSlot struct {
	Level int `gorm:"primaryKey"` // Уровень слота (1-9)
	Used  int `gorm:"default:0"`  // Использовано слотов этого уровня
	Max   int `gorm:"default:0"`  // Максимальное количество слотов
}

// SpellSlots представляет все слоты заклинаний персонажа
type SpellSlots struct {
	Level1 int `gorm:"default:0"`
	Level2 int `gorm:"default:0"`
	Level3 int `gorm:"default:0"`
	Level4 int `gorm:"default:0"`
	Level5 int `gorm:"default:0"`
	Level6 int `gorm:"default:0"`
	Level7 int `gorm:"default:0"`
	Level8 int `gorm:"default:0"`
	Level9 int `gorm:"default:0"`

	UsedLevel1 int `gorm:"default:0"`
	UsedLevel2 int `gorm:"default:0"`
	UsedLevel3 int `gorm:"default:0"`
	UsedLevel4 int `gorm:"default:0"`
	UsedLevel5 int `gorm:"default:0"`
	UsedLevel6 int `gorm:"default:0"`
	UsedLevel7 int `gorm:"default:0"`
	UsedLevel8 int `gorm:"default:0"`
	UsedLevel9 int `gorm:"default:0"`
}

// GetSlotsByLevel возвращает максимальное количество слотов для указанного уровня
func (ss *SpellSlots) GetSlotsByLevel(level int) int {
	switch level {
	case 1:
		return ss.Level1
	case 2:
		return ss.Level2
	case 3:
		return ss.Level3
	case 4:
		return ss.Level4
	case 5:
		return ss.Level5
	case 6:
		return ss.Level6
	case 7:
		return ss.Level7
	case 8:
		return ss.Level8
	case 9:
		return ss.Level9
	default:
		return 0
	}
}

// GetUsedSlotsByLevel возвращает использованное количество слотов для указанного уровня
func (ss *SpellSlots) GetUsedSlotsByLevel(level int) int {
	switch level {
	case 1:
		return ss.UsedLevel1
	case 2:
		return ss.UsedLevel2
	case 3:
		return ss.UsedLevel3
	case 4:
		return ss.UsedLevel4
	case 5:
		return ss.UsedLevel5
	case 6:
		return ss.UsedLevel6
	case 7:
		return ss.UsedLevel7
	case 8:
		return ss.UsedLevel8
	case 9:
		return ss.UsedLevel9
	default:
		return 0
	}
}

// SetSlotsByLevel устанавливает максимальное количество слотов для указанного уровня
func (ss *SpellSlots) SetSlotsByLevel(level int, count int) {
	switch level {
	case 1:
		ss.Level1 = count
	case 2:
		ss.Level2 = count
	case 3:
		ss.Level3 = count
	case 4:
		ss.Level4 = count
	case 5:
		ss.Level5 = count
	case 6:
		ss.Level6 = count
	case 7:
		ss.Level7 = count
	case 8:
		ss.Level8 = count
	case 9:
		ss.Level9 = count
	}
}

// SetUsedSlotsByLevel устанавливает использованное количество слотов для указанного уровня
func (ss *SpellSlots) SetUsedSlotsByLevel(level int, count int) {
	switch level {
	case 1:
		ss.UsedLevel1 = count
	case 2:
		ss.UsedLevel2 = count
	case 3:
		ss.UsedLevel3 = count
	case 4:
		ss.UsedLevel4 = count
	case 5:
		ss.UsedLevel5 = count
	case 6:
		ss.UsedLevel6 = count
	case 7:
		ss.UsedLevel7 = count
	case 8:
		ss.UsedLevel8 = count
	case 9:
		ss.UsedLevel9 = count
	}
}

// UseSpellSlot использует слот заклинания указанного уровня
func (ss *SpellSlots) UseSpellSlot(level int) bool {
	maxSlots := ss.GetSlotsByLevel(level)
	usedSlots := ss.GetUsedSlotsByLevel(level)

	if usedSlots >= maxSlots {
		return false // Нет доступных слотов
	}

	ss.SetUsedSlotsByLevel(level, usedSlots+1)
	return true
}

// RestoreSpellSlots восстанавливает все слоты заклинаний (после длинного отдыха)
func (ss *SpellSlots) RestoreSpellSlots() {
	ss.UsedLevel1 = 0
	ss.UsedLevel2 = 0
	ss.UsedLevel3 = 0
	ss.UsedLevel4 = 0
	ss.UsedLevel5 = 0
	ss.UsedLevel6 = 0
	ss.UsedLevel7 = 0
	ss.UsedLevel8 = 0
	ss.UsedLevel9 = 0
}

// IsAvailableForClass проверяет, доступно ли заклинание для класса
// Принимает строку вместо character.Class чтобы разорвать циклическую зависимость
func (s *Spell) IsAvailableForClass(class string) bool {
	switch class {
	case "wizard":
		return s.AvailableForWizard
	case "cleric":
		return s.AvailableForCleric
	case "ranger":
		return s.AvailableForRanger
	default:
		return false
	}
}

// IsCantrip проверяет, является ли заклинание заговором (0 уровень)
func (s *Spell) IsCantrip() bool {
	return s.Level == 0
}

// CalculateSpellSlotsForLevel рассчитывает слоты заклинаний для персонажа определенного класса и уровня
// Принимает строку вместо character.Class чтобы разорвать циклическую зависимость
func CalculateSpellSlotsForLevel(class string, level int) *SpellSlots {
	slots := &SpellSlots{}

	// Таблица слотов заклинаний по классу и уровню (D&D 5e)
	switch class {
	case "wizard":
		// Wizard spell slots
		wizardSlots := map[int][9]int{
			1:  {2, 0, 0, 0, 0, 0, 0, 0, 0}, // Level 1: 2 slots level 1
			2:  {3, 0, 0, 0, 0, 0, 0, 0, 0},
			3:  {4, 2, 0, 0, 0, 0, 0, 0, 0},
			4:  {4, 3, 0, 0, 0, 0, 0, 0, 0},
			5:  {4, 3, 2, 0, 0, 0, 0, 0, 0},
			6:  {4, 3, 3, 0, 0, 0, 0, 0, 0},
			7:  {4, 3, 3, 1, 0, 0, 0, 0, 0},
			8:  {4, 3, 3, 2, 0, 0, 0, 0, 0},
			9:  {4, 3, 3, 3, 1, 0, 0, 0, 0},
			10: {4, 3, 3, 3, 2, 0, 0, 0, 0},
		}
		if levelSlots, ok := wizardSlots[level]; ok {
			for i := 0; i < 9; i++ {
				slots.SetSlotsByLevel(i+1, levelSlots[i])
			}
		} else if level > 10 {
			// Для уровней выше 10 используем слоты 10 уровня
			level10Slots := wizardSlots[10]
			for i := 0; i < 9; i++ {
				slots.SetSlotsByLevel(i+1, level10Slots[i])
			}
		}

	case "cleric":
		// Cleric spell slots (тот же прогресс, что и у wizard)
		clericSlots := map[int][9]int{
			1:  {2, 0, 0, 0, 0, 0, 0, 0, 0},
			2:  {3, 0, 0, 0, 0, 0, 0, 0, 0},
			3:  {4, 2, 0, 0, 0, 0, 0, 0, 0},
			4:  {4, 3, 0, 0, 0, 0, 0, 0, 0},
			5:  {4, 3, 2, 0, 0, 0, 0, 0, 0},
			6:  {4, 3, 3, 0, 0, 0, 0, 0, 0},
			7:  {4, 3, 3, 1, 0, 0, 0, 0, 0},
			8:  {4, 3, 3, 2, 0, 0, 0, 0, 0},
			9:  {4, 3, 3, 3, 1, 0, 0, 0, 0},
			10: {4, 3, 3, 3, 2, 0, 0, 0, 0},
		}
		if levelSlots, ok := clericSlots[level]; ok {
			for i := 0; i < 9; i++ {
				slots.SetSlotsByLevel(i+1, levelSlots[i])
			}
		} else if level > 10 {
			level10Slots := clericSlots[10]
			for i := 0; i < 9; i++ {
				slots.SetSlotsByLevel(i+1, level10Slots[i])
			}
		}

	case "ranger":
		// Ranger spell slots (меньше слотов, как half-caster)
		rangerSlots := map[int][9]int{
			2:  {2, 0, 0, 0, 0, 0, 0, 0, 0}, // Ranger получает заклинания с 2 уровня
			3:  {3, 0, 0, 0, 0, 0, 0, 0, 0},
			4:  {3, 0, 0, 0, 0, 0, 0, 0, 0},
			5:  {4, 2, 0, 0, 0, 0, 0, 0, 0},
			6:  {4, 2, 0, 0, 0, 0, 0, 0, 0},
			7:  {4, 3, 0, 0, 0, 0, 0, 0, 0},
			8:  {4, 3, 0, 0, 0, 0, 0, 0, 0},
			9:  {4, 3, 2, 0, 0, 0, 0, 0, 0},
			10: {4, 3, 2, 0, 0, 0, 0, 0, 0},
		}
		if level < 2 {
			// Ranger не имеет заклинаний на 1 уровне
			return slots
		}
		if levelSlots, ok := rangerSlots[level]; ok {
			for i := 0; i < 9; i++ {
				slots.SetSlotsByLevel(i+1, levelSlots[i])
			}
		} else if level > 10 {
			level10Slots := rangerSlots[10]
			for i := 0; i < 9; i++ {
				slots.SetSlotsByLevel(i+1, level10Slots[i])
			}
		}
	}

	return slots
}
