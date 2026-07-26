package character

import (
	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/inventory"
)

// startingKitItem описывает один предмет стартового набора снаряжения.
type startingKitItem struct {
	name          string
	description   string
	weight        float64
	quantity      int
	itemType      inventory.ItemType
	healingAmount int
	equip         bool // сразу экипировать предмет (оружие/броня)
}

// startingGold — стартовое золото персонажа по классу.
var startingGold = map[character.Class]int{
	character.ClassFighter: 15,
	character.ClassWizard:  10,
	character.ClassRogue:   12,
	character.ClassCleric:  12,
	character.ClassRanger:  13,
}

// startingKits — стартовый набор снаряжения персонажа по классу.
var startingKits = map[character.Class][]startingKitItem{
	character.ClassFighter: {
		{name: "Длинный меч", description: "Стандартное оружие воина", weight: 1.5, quantity: 1, itemType: inventory.ItemTypeWeapon, equip: true},
		{name: "Кольчуга", description: "Тяжёлая броня", weight: 10, quantity: 1, itemType: inventory.ItemTypeArmor, equip: true},
		{name: "Зелье лечения", description: "Восстанавливает здоровье", weight: 0.5, quantity: 2, itemType: inventory.ItemTypePotion, healingAmount: 10},
	},
	character.ClassWizard: {
		{name: "Посох", description: "Магический фокусировщик, годится и как оружие ближнего боя", weight: 2, quantity: 1, itemType: inventory.ItemTypeWeapon, equip: true},
		{name: "Книга заклинаний", description: "Содержит известные заклинания", weight: 1.5, quantity: 1, itemType: inventory.ItemTypeTool},
		{name: "Зелье лечения", description: "Восстанавливает здоровье", weight: 0.5, quantity: 2, itemType: inventory.ItemTypePotion, healingAmount: 10},
	},
	character.ClassRogue: {
		{name: "Короткий меч", description: "Лёгкое быстрое оружие", weight: 1, quantity: 1, itemType: inventory.ItemTypeWeapon, equip: true},
		{name: "Кожаная броня", description: "Лёгкая броня", weight: 5, quantity: 1, itemType: inventory.ItemTypeArmor, equip: true},
		{name: "Воровские инструменты", description: "Для взлома замков и обезвреживания ловушек", weight: 1, quantity: 1, itemType: inventory.ItemTypeTool},
	},
	character.ClassCleric: {
		{name: "Булава", description: "Простое дробящее оружие", weight: 2, quantity: 1, itemType: inventory.ItemTypeWeapon, equip: true},
		{name: "Кольчуга", description: "Тяжёлая броня", weight: 10, quantity: 1, itemType: inventory.ItemTypeArmor, equip: true},
		{name: "Зелье лечения", description: "Восстанавливает здоровье", weight: 0.5, quantity: 2, itemType: inventory.ItemTypePotion, healingAmount: 10},
	},
	character.ClassRanger: {
		{name: "Короткий лук", description: "Дальнобойное оружие", weight: 2, quantity: 1, itemType: inventory.ItemTypeWeapon, equip: true},
		{name: "Кожаная броня", description: "Лёгкая броня", weight: 5, quantity: 1, itemType: inventory.ItemTypeArmor, equip: true},
		{name: "Зелье лечения", description: "Восстанавливает здоровье", weight: 0.5, quantity: 1, itemType: inventory.ItemTypePotion, healingAmount: 10},
	},
}

// buildStartingInventory собирает инвентарь нового персонажа: стартовое снаряжение
// (экипированное оружие/броня по классу) и немного золота на первые покупки.
func buildStartingInventory(characterID uint, class character.Class) *inventory.Inventory {
	inv := inventory.NewInventory(characterID)

	for _, it := range startingKits[class] {
		if err := inv.AddItem(it.name, it.description, it.weight, it.quantity, it.itemType, it.healingAmount); err != nil {
			continue
		}
		if it.equip {
			_, _ = inv.EquipItem(it.name)
		}
	}

	inv.Gold = startingGold[class]

	return inv
}
