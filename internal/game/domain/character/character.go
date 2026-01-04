package character

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

	Level int
	HP    int
	MaxHP int

	Status Status `gorm:"type:varchar(16);not null"`

	Stats Stats
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

func NewCharacter(
	name string,
	class Class,
	race Race,
	stats Stats,
) (*Character, error) {

	maxHP := calculateHP(class, stats)

	return &Character{
		Name:   name,
		Class:  class,
		Race:   race,
		Level:  1,
		HP:     maxHP,
		MaxHP:  maxHP,
		Status: StatusAlive,
		Stats:  stats,
	}, nil
}
