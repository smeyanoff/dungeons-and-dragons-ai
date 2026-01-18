package persistence

import (
	"context"

	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/spell"
	"gorm.io/gorm"
)

type SpellRepository struct {
	db *gorm.DB
}

func NewSpellRepository(db *gorm.DB) *SpellRepository {
	return &SpellRepository{db: db}
}

// GetAll получает все заклинания
func (r *SpellRepository) GetAll(ctx context.Context) ([]*spell.Spell, error) {
	var spells []*spell.Spell
	err := r.db.WithContext(ctx).
		Order("level ASC, name ASC").
		Find(&spells).Error
	
	if err != nil {
		return nil, err
	}
	
	return spells, nil
}

// GetByID получает заклинание по ID
func (r *SpellRepository) GetByID(ctx context.Context, id uint) (*spell.Spell, error) {
	var s spell.Spell
	err := r.db.WithContext(ctx).
		Where("id = ?", id).
		First(&s).Error
	
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	
	return &s, nil
}

// GetByCode получает заклинание по коду
func (r *SpellRepository) GetByCode(ctx context.Context, code string) (*spell.Spell, error) {
	var s spell.Spell
	err := r.db.WithContext(ctx).
		Where("code = ?", code).
		First(&s).Error
	
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	
	return &s, nil
}

// GetByClass получает заклинания, доступные для класса
func (r *SpellRepository) GetByClass(ctx context.Context, class character.Class) ([]*spell.Spell, error) {
	var spells []*spell.Spell
	
	query := r.db.WithContext(ctx)
	
	switch class {
	case character.ClassWizard:
		query = query.Where("available_for_wizard = ?", true)
	case character.ClassCleric:
		query = query.Where("available_for_cleric = ?", true)
	case character.ClassRanger:
		query = query.Where("available_for_ranger = ?", true)
	default:
		// Для немагических классов возвращаем пустой список
		return []*spell.Spell{}, nil
	}
	
	err := query.Order("level ASC, name ASC").Find(&spells).Error
	
	if err != nil {
		return nil, err
	}
	
	return spells, nil
}

// GetByLevel получает заклинания указанного уровня
func (r *SpellRepository) GetByLevel(ctx context.Context, level int) ([]*spell.Spell, error) {
	var spells []*spell.Spell
	err := r.db.WithContext(ctx).
		Where("level = ?", level).
		Order("name ASC").
		Find(&spells).Error
	
	if err != nil {
		return nil, err
	}
	
	return spells, nil
}

// Save сохраняет заклинание
func (r *SpellRepository) Save(ctx context.Context, s *spell.Spell) error {
	return r.db.WithContext(ctx).
		Save(s).Error
}

// GetCharacterSpells получает все известные заклинания персонажа
func (r *SpellRepository) GetCharacterSpells(ctx context.Context, characterID uint) ([]*spell.CharacterSpell, error) {
	var characterSpells []*spell.CharacterSpell
	err := r.db.WithContext(ctx).
		Preload("Spell").
		Where("character_id = ?", characterID).
		Order("spell.level ASC, spell.name ASC").
		Find(&characterSpells).Error
	
	if err != nil {
		return nil, err
	}
	
	return characterSpells, nil
}

// GetCharacterSpell получает конкретное известное заклинание персонажа
func (r *SpellRepository) GetCharacterSpell(ctx context.Context, characterID uint, spellID uint) (*spell.CharacterSpell, error) {
	var cs spell.CharacterSpell
	err := r.db.WithContext(ctx).
		Preload("Spell").
		Where("character_id = ? AND spell_id = ?", characterID, spellID).
		First(&cs).Error
	
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	
	return &cs, nil
}

// SaveCharacterSpell сохраняет известное заклинание персонажа
func (r *SpellRepository) SaveCharacterSpell(ctx context.Context, cs *spell.CharacterSpell) error {
	return r.db.WithContext(ctx).
		Save(cs).Error
}

// DeleteCharacterSpell удаляет известное заклинание персонажа
func (r *SpellRepository) DeleteCharacterSpell(ctx context.Context, characterID uint, spellID uint) error {
	return r.db.WithContext(ctx).
		Where("character_id = ? AND spell_id = ?", characterID, spellID).
		Delete(&spell.CharacterSpell{}).Error
}

// InitDefaultSpells инициализирует базовые заклинания в БД
func (r *SpellRepository) InitDefaultSpells(ctx context.Context) error {
	// Проверяем, есть ли уже заклинания в БД
	var count int64
	r.db.WithContext(ctx).Model(&spell.Spell{}).Count(&count)
	if count > 0 {
		// Заклинания уже инициализированы
		return nil
	}
	
	defaultSpells := []*spell.Spell{
		// Заговоры (0 уровень)
		{
			Code:              "fire_bolt",
			Name:              "Огненный снаряд",
			Description:       "Вы создаете огненный снаряд, который летит к цели и наносит урон 1d10 огнем.",
			Level:             0,
			School:            spell.SpellSchoolEvocation,
			Type:              spell.SpellTypeCantrip,
			CastingTime:       spell.CastingTimeAction,
			Range:             spell.Range120Feet,
			Duration:          spell.DurationInstantaneous,
			Verbal:            true,
			Somatic:           true,
			Damage:            "1d10",
			AvailableForWizard: true,
			Icon:              "🔥",
		},
		{
			Code:              "light",
			Name:              "Свет",
			Description:       "Вы касаетесь одного объекта размером не больше 10 футов в любом измерении. До конца заклинания объект излучает яркий свет в радиусе 20 футов и тусклый свет в дополнительных 20 футах.",
			Level:             0,
			School:            spell.SpellSchoolEvocation,
			Type:              spell.SpellTypeCantrip,
			CastingTime:       spell.CastingTimeAction,
			Range:             spell.RangeTouch,
			Duration:          spell.DurationHours,
			DurationValue:     1,
			Verbal:            true,
			Material:          true,
			MaterialCost:      "Светлячок или кусок фосфоресцирующего мха",
			AvailableForCleric: true,
			Icon:              "💡",
		},
		{
			Code:              "guidance",
			Name:              "Руководство",
			Description:       "Вы касаетесь одного согласного существа. Один раз до окончания заклинания цель может бросить к4 и добавить результат к одной проверке характеристики на свой выбор.",
			Level:             0,
			School:            spell.SpellSchoolDivination,
			Type:              spell.SpellTypeCantrip,
			CastingTime:       spell.CastingTimeAction,
			Range:             spell.RangeTouch,
			Duration:          spell.DurationMinutes,
			DurationValue:     1,
			Verbal:            true,
			Somatic:           true,
			AvailableForCleric: true,
			Icon:              "🌟",
		},
		
		// Заклинания 1 уровня
		{
			Code:              "magic_missile",
			Name:              "Магическая стрела",
			Description:       "Вы создаете три светящиеся стрелы магической силы. Каждая стрела попадает в существо по вашему выбору, видимое вами в пределах дистанции. Каждая стрела наносит 1d4+1 урона силовым полем.",
			Level:             1,
			School:            spell.SpellSchoolEvocation,
			Type:              spell.SpellTypeLevel1,
			CastingTime:       spell.CastingTimeAction,
			Range:             spell.Range120Feet,
			Duration:          spell.DurationInstantaneous,
			Verbal:            true,
			Somatic:           true,
			Damage:            "1d4+1",
			AvailableForWizard: true,
			Icon:              "✨",
		},
		{
			Code:              "cure_wounds",
			Name:              "Лечение ран",
			Description:       "Существо, которого вы касаетесь, восстанавливает количество хитов, равное 1d8 + ваш модификатор базовой характеристики.",
			Level:             1,
			School:            spell.SpellSchoolEvocation,
			Type:              spell.SpellTypeLevel1,
			CastingTime:       spell.CastingTimeAction,
			Range:             spell.RangeTouch,
			Duration:          spell.DurationInstantaneous,
			Verbal:            true,
			Somatic:           true,
			Healing:           "1d8",
			AvailableForCleric: true,
			Icon:              "💚",
		},
		{
			Code:              "healing_word",
			Name:              "Лечащее слово",
			Description:       "Существо по вашему выбору, видимое вами в пределах дистанции, восстанавливает количество хитов, равное 1d4 + ваш модификатор базовой характеристики.",
			Level:             1,
			School:            spell.SpellSchoolEvocation,
			Type:              spell.SpellTypeLevel1,
			CastingTime:       spell.CastingTimeBonus,
			Range:             spell.Range60Feet,
			Duration:          spell.DurationInstantaneous,
			Verbal:            true,
			Healing:           "1d4",
			AvailableForCleric: true,
			Icon:              "💚",
		},
		{
			Code:              "shield",
			Name:              "Щит",
			Description:       "Пока заклинание активно, ваш класс брони увеличивается на +5, включая атаку, вызвавшую это заклинание. Вы также получаете иммунитет к магическим стрелам.",
			Level:             1,
			School:            spell.SpellSchoolAbjuration,
			Type:              spell.SpellTypeLevel1,
			CastingTime:       spell.CastingTimeReaction,
			Range:             spell.RangeSelf,
			Duration:          spell.DurationRounds,
			DurationValue:     1,
			Verbal:            true,
			Somatic:           true,
			Concentration:     false,
			AvailableForWizard: true,
			Icon:              "🛡️",
		},
		{
			Code:              "hunters_mark",
			Name:              "Метка охотника",
			Description:       "Вы выбираете одно существо, видимое вами в пределах дистанции, и мистическим способом отмечаете его как своей добычей. Пока заклинание активно, вы наносите дополнительный 1d6 урона по цели.",
			Level:             1,
			School:            spell.SpellSchoolDivination,
			Type:              spell.SpellTypeLevel1,
			CastingTime:       spell.CastingTimeBonus,
			Range:             spell.Range90Feet,
			Duration:          spell.DurationConcentration,
			DurationValue:     3600, // 1 час в секундах
			Verbal:            true,
			Concentration:     true,
			AvailableForRanger: true,
			Icon:              "🎯",
		},
		
		// Заклинания 2 уровня
		{
			Code:              "fireball",
			Name:              "Огненный шар",
			Description:       "Яркая вспышка вылетает из вашей указывающей руки в точку по вашему выбору в пределах дистанции и затем взрывается с оглушительным грохотом. Каждое существо в сфере радиусом 20 футов должно совершить спасбросок Ловкости.",
			Level:             2,
			School:            spell.SpellSchoolEvocation,
			Type:              spell.SpellTypeLevel2,
			CastingTime:       spell.CastingTimeAction,
			Range:             spell.Range120Feet,
			Duration:          spell.DurationInstantaneous,
			Verbal:            true,
			Somatic:           true,
			Material:          true,
			MaterialCost:      "Небольшой шарик из гуано летучей мыши и серы",
			Damage:            "8d6",
			SaveType:          "DEX",
			SaveDC:            8, // Будет рассчитываться от характеристики
			AvailableForWizard: true,
			Icon:              "🔥",
		},
		{
			Code:              "lesser_restoration",
			Name:              "Малое восстановление",
			Description:       "Вы касаетесь существа и можете закончить либо одну болезнь, либо одно состояние (ослеплен, оглушен, парализован или отравлен) на выборе цели.",
			Level:             2,
			School:            spell.SpellSchoolAbjuration,
			Type:              spell.SpellTypeLevel2,
			CastingTime:       spell.CastingTimeAction,
			Range:             spell.RangeTouch,
			Duration:          spell.DurationInstantaneous,
			Verbal:            true,
			Somatic:           true,
			AvailableForCleric: true,
			Icon:              "✨",
		},
	}
	
	// Сохраняем все заклинания
	for _, s := range defaultSpells {
		if err := r.Save(ctx, s); err != nil {
			return err
		}
	}
	
	return nil
}