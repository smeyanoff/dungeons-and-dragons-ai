package character

// AbilityType представляет тип способности
type AbilityType string

const (
	AbilityTypeFeat   AbilityType = "feat"   // Перк
	AbilityTypeSpell  AbilityType = "spell"  // Заклинание
	AbilityTypeClass  AbilityType = "class"  // Классовая способность
)

// AbilityUseType представляет тип использования способности
type AbilityUseType string

const (
	AbilityUsePassive AbilityUseType = "passive" // Пассивная способность
	AbilityUseActive  AbilityUseType = "active"  // Активная способность
)

// Ability представляет базовую способность персонажа
type Ability struct {
	ID          uint          `gorm:"primaryKey"`
	CharacterID uint          `gorm:"index"`
	Name        string
	Description string
	Type        AbilityType   `gorm:"type:varchar(32);not null"`
	UseType     AbilityUseType `gorm:"type:varchar(32);not null"`
	
	// Для заклинаний
	SpellLevel  int    // Уровень заклинания (1-9)
	SpellSchool string // Школа магии (для заклинаний)
	
	// Ограничения использования
	UsesPerDay   int // Использований в день (0 = без ограничений)
	UsesRemaining int // Оставшихся использований сегодня
}

// GetCharacterAbilities возвращает список способностей персонажа по классу и уровню
// Это базовая реализация - в будущем можно расширить для разных классов и рас
func GetCharacterAbilities(class Class, level int) []Ability {
	var abilities []Ability

	switch class {
	case ClassFighter:
		abilities = append(abilities, Ability{
			Name:        "Боевой стиль",
			Description: "Повышает мастерство в ближнем бою",
			Type:        AbilityTypeClass,
			UseType:     AbilityUsePassive,
		})
		if level >= 2 {
			abilities = append(abilities, Ability{
				Name:        "Второе дыхание",
				Description: "Восстанавливает 1d10+level HP один раз в день",
				Type:        AbilityTypeClass,
				UseType:     AbilityUseActive,
				UsesPerDay:   1,
				UsesRemaining: 1,
			})
		}
		
	case ClassWizard:
		abilities = append(abilities, Ability{
			Name:        "Заклинание",
			Description: "Может использовать заклинания из книги заклинаний",
			Type:        AbilityTypeSpell,
			UseType:     AbilityUseActive,
		})
		// Базовые заклинания для мага 1 уровня
		abilities = append(abilities, Ability{
			Name:        "Магическая стрела",
			Description: "Наносит 1d4+1 магического урона",
			Type:        AbilityTypeSpell,
			UseType:     AbilityUseActive,
			SpellLevel:  1,
			SpellSchool: "Эвокация",
		})
		abilities = append(abilities, Ability{
			Name:        "Щит",
			Description: "Временно повышает AC на +5 до начала следующего хода",
			Type:        AbilityTypeSpell,
			UseType:     AbilityUseActive,
			SpellLevel:  1,
			SpellSchool: "Абjурация",
		})
		
	case ClassRogue:
		abilities = append(abilities, Ability{
			Name:        "Скрытность",
			Description: "Мастерство в скрытности и незаметности",
			Type:        AbilityTypeClass,
			UseType:     AbilityUsePassive,
		})
		abilities = append(abilities, Ability{
			Name:        "Критический удар",
			Description: "Удваивает урон при атаке из скрытности",
			Type:        AbilityTypeClass,
			UseType:     AbilityUseActive,
		})
		
	case ClassCleric:
		abilities = append(abilities, Ability{
			Name:        "Божественная магия",
			Description: "Может использовать заклинания через божественную силу",
			Type:        AbilityTypeSpell,
			UseType:     AbilityUseActive,
		})
		abilities = append(abilities, Ability{
			Name:        "Лечение ран",
			Description: "Восстанавливает 1d8+модификатор мудрости HP",
			Type:        AbilityTypeSpell,
			UseType:     AbilityUseActive,
			SpellLevel:  1,
			SpellSchool: "Исцеление",
		})
		
	case ClassRanger:
		abilities = append(abilities, Ability{
			Name:        "Следопыт",
			Description: "Мастерство в отслеживании и выживании",
			Type:        AbilityTypeClass,
			UseType:     AbilityUsePassive,
		})
		abilities = append(abilities, Ability{
			Name:        "Излюбленный враг",
			Description: "Бонус +2 к урону против выбранного типа врагов",
			Type:        AbilityTypeClass,
			UseType:     AbilityUsePassive,
		})
	}

	return abilities
}
