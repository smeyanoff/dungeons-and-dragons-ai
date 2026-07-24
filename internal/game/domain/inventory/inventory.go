package inventory

import (
	"errors"
)

type Inventory struct {
	ID          uint `gorm:"primaryKey"`
	CharacterID uint `gorm:"uniqueIndex"`

	Items []InventoryItem `gorm:"foreignKey:InventoryID"`
}

type InventoryItem struct {
	ID          uint `gorm:"primaryKey"`
	InventoryID uint `gorm:"index"`

	Name        string
	Description string
	Weight      float64 // вес в кг
	Quantity    int     // количество
	Type        ItemType

	HealingAmount int // сколько HP восстанавливает ОДНА единица предмета при использовании (0 - предмет не лечит)
}

type ItemType string

const (
	ItemTypeWeapon     ItemType = "weapon"
	ItemTypeArmor      ItemType = "armor"
	ItemTypePotion     ItemType = "potion"
	ItemTypeTool       ItemType = "tool"
	ItemTypeMisc       ItemType = "misc"
	ItemTypeConsumable ItemType = "consumable"
)

const (
	MaxWeight = 30.0 // максимальный вес инвентаря в кг
)

func NewInventory(characterID uint) *Inventory {
	return &Inventory{
		CharacterID: characterID,
		Items:       []InventoryItem{},
	}
}

func (inv *Inventory) AddItem(name, description string, weight float64, quantity int, itemType ItemType, healingAmount int) error {
	// Проверяем вес
	totalWeight := inv.GetTotalWeight()
	if totalWeight+weight*float64(quantity) > MaxWeight {
		return errors.New("инвентарь переполнен")
	}

	// Ищем существующий предмет того же типа
	for i := range inv.Items {
		if inv.Items[i].Name == name && inv.Items[i].Type == itemType {
			inv.Items[i].Quantity += quantity
			// Если у существующей записи ещё не было указано лечение - подхватываем новое значение
			if inv.Items[i].HealingAmount == 0 && healingAmount > 0 {
				inv.Items[i].HealingAmount = healingAmount
			}
			return nil
		}
	}

	// Добавляем новый предмет
	inv.Items = append(inv.Items, InventoryItem{
		Name:          name,
		Description:   description,
		Weight:        weight,
		Quantity:      quantity,
		Type:          itemType,
		HealingAmount: healingAmount,
	})

	return nil
}

// RemoveItem удаляет quantity единиц предмета name из инвентаря.
// Возвращает снимок удалённого предмета (с Quantity = фактически удалённое количество),
// чтобы вызывающий код мог применить эффекты предмета (например, лечение) при его использовании.
func (inv *Inventory) RemoveItem(name string, quantity int) (*InventoryItem, error) {
	for i := range inv.Items {
		if inv.Items[i].Name == name {
			if inv.Items[i].Quantity < quantity {
				return nil, errors.New("недостаточно предметов")
			}
			removed := inv.Items[i]
			removed.Quantity = quantity
			inv.Items[i].Quantity -= quantity
			if inv.Items[i].Quantity <= 0 {
				// Удаляем предмет из списка
				inv.Items = append(inv.Items[:i], inv.Items[i+1:]...)
			}
			return &removed, nil
		}
	}
	return nil, errors.New("предмет не найден")
}

func (inv *Inventory) GetTotalWeight() float64 {
	total := 0.0
	for _, item := range inv.Items {
		total += item.Weight * float64(item.Quantity)
	}
	return total
}

func (inv *Inventory) GetItemCount() int {
	count := 0
	for _, item := range inv.Items {
		count += item.Quantity
	}
	return count
}
