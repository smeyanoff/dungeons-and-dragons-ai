package inventory

import (
	"errors"
	"strings"
)

type Inventory struct {
	ID          uint `gorm:"primaryKey"`
	CharacterID uint `gorm:"uniqueIndex"`

	Gold int `gorm:"default:0"` // деньги персонажа (золотые монеты)

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

	HealingAmount int  // сколько HP восстанавливает ОДНА единица предмета при использовании (0 - предмет не лечит)
	Equipped      bool // надет/используется ли предмет персонажем прямо сейчас (актуально для weapon и armor)
}

// IsEquippable определяет, можно ли надеть/взять в руки предмет данного типа.
// Расходники, инструменты и разное экипировать нельзя - только оружие и броню.
func (t ItemType) IsEquippable() bool {
	return t == ItemTypeWeapon || t == ItemTypeArmor
}

// normalizeItemName приводит название предмета к виду, устойчивому к регистру и пробелам,
// чтобы "Меч" и "меч " считались одним и тем же предметом.
func normalizeItemName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
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

	// Ищем существующий предмет того же типа (сравнение без учета регистра и пробелов)
	for i := range inv.Items {
		if normalizeItemName(inv.Items[i].Name) == normalizeItemName(name) && inv.Items[i].Type == itemType {
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
		if normalizeItemName(inv.Items[i].Name) == normalizeItemName(name) {
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

// EquipItem надевает/берет в руки предмет по имени. Оружие и броня занимают по одному "слоту":
// если у персонажа уже что-то экипировано в этом типе, оно автоматически снимается.
// Возвращает указатель на экипированный предмет.
func (inv *Inventory) EquipItem(name string) (*InventoryItem, error) {
	target := normalizeItemName(name)

	idx := -1
	for i := range inv.Items {
		if normalizeItemName(inv.Items[i].Name) == target {
			idx = i
			break
		}
	}
	if idx == -1 {
		return nil, errors.New("предмет не найден")
	}

	if !inv.Items[idx].Type.IsEquippable() {
		return nil, errors.New("этот предмет нельзя экипировать")
	}

	if inv.Items[idx].Equipped {
		return &inv.Items[idx], nil
	}

	// Снимаем всё, что уже занимает этот же слот (тип предмета)
	for i := range inv.Items {
		if i != idx && inv.Items[i].Type == inv.Items[idx].Type {
			inv.Items[i].Equipped = false
		}
	}

	inv.Items[idx].Equipped = true
	return &inv.Items[idx], nil
}

// UnequipItem снимает предмет по имени. Если предмет не был экипирован - ошибка.
func (inv *Inventory) UnequipItem(name string) (*InventoryItem, error) {
	target := normalizeItemName(name)

	for i := range inv.Items {
		if normalizeItemName(inv.Items[i].Name) == target {
			if !inv.Items[i].Equipped {
				return nil, errors.New("предмет не экипирован")
			}
			inv.Items[i].Equipped = false
			return &inv.Items[i], nil
		}
	}
	return nil, errors.New("предмет не найден")
}

// GetEquippedItems возвращает все экипированные предметы персонажа.
func (inv *Inventory) GetEquippedItems() []InventoryItem {
	equipped := make([]InventoryItem, 0)
	for _, item := range inv.Items {
		if item.Equipped {
			equipped = append(equipped, item)
		}
	}
	return equipped
}
