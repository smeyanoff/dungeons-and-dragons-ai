package character

import (
	"fmt"
	"testing"

	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/inventory"
)

func TestBuildStartingInventory_SetsCharacterID(t *testing.T) {
	inv := buildStartingInventory(42, character.ClassFighter)

	if inv.CharacterID != 42 {
		t.Errorf("expected CharacterID=42, got %d", inv.CharacterID)
	}
}

func TestBuildStartingInventory_UnknownClassGrantsNothing(t *testing.T) {
	inv := buildStartingInventory(1, character.Class("bard"))

	if len(inv.Items) != 0 {
		t.Errorf("expected no items for a class without a defined starting kit, got %+v", inv.Items)
	}
	if inv.Gold != 0 {
		t.Errorf("expected 0 gold for a class without defined starting gold, got %d", inv.Gold)
	}
}

// TestBuildStartingInventory_EveryKnownClass проверяет инвентарь для каждого класса,
// у которого определён стартовый набор: золото совпадает с startingGold, все предметы
// набора реально попали в инвентарь (не были отброшены из-за переполнения веса),
// и ровно предметы с equip=true оказались экипированы.
func TestBuildStartingInventory_EveryKnownClass(t *testing.T) {
	for class, kit := range startingKits {
		t.Run(string(class), func(t *testing.T) {
			inv := buildStartingInventory(1, class)

			if inv.Gold != startingGold[class] {
				t.Errorf("class %s: expected gold=%d, got %d", class, startingGold[class], inv.Gold)
			}

			if len(inv.Items) != len(kit) {
				t.Fatalf("class %s: expected %d distinct items, got %d: %+v", class, len(kit), len(inv.Items), inv.Items)
			}

			wantEquippedCount := 0
			for _, kitItem := range kit {
				if kitItem.equip {
					wantEquippedCount++
				}

				found := false
				for _, invItem := range inv.Items {
					if invItem.Name != kitItem.name {
						continue
					}
					found = true
					if invItem.Quantity != kitItem.quantity {
						t.Errorf("class %s, item %q: expected quantity=%d, got %d", class, kitItem.name, kitItem.quantity, invItem.Quantity)
					}
					if invItem.Type != kitItem.itemType {
						t.Errorf("class %s, item %q: expected type=%s, got %s", class, kitItem.name, kitItem.itemType, invItem.Type)
					}
					if invItem.Equipped != kitItem.equip {
						t.Errorf("class %s, item %q: expected Equipped=%v, got %v", class, kitItem.name, kitItem.equip, invItem.Equipped)
					}
				}
				if !found {
					t.Errorf("class %s: item %q from the kit is missing in the resulting inventory", class, kitItem.name)
				}
			}

			if got := len(inv.GetEquippedItems()); got != wantEquippedCount {
				t.Errorf("class %s: expected %d equipped items, got %d", class, wantEquippedCount, got)
			}

			// Стартовый набор не должен переполнять инвентарь по весу - иначе AddItem
			// молча отбросит часть предметов (buildStartingInventory игнорирует такие ошибки).
			if inv.GetTotalWeight() > inventory.MaxWeight {
				t.Errorf("class %s: starting kit weight %.2f exceeds MaxWeight %.2f", class, inv.GetTotalWeight(), inventory.MaxWeight)
			}
		})
	}
}

// TestBuildStartingInventory_AtMostOneEquippedWeaponAndArmor гарантирует, что стартовый набор
// не экипирует одновременно два предмета в один слот (оружие/броню) - EquipItem снимает
// предыдущий предмет того же типа, так что дубли в кит-таблице привели бы к тихой потере предмета.
func TestBuildStartingInventory_AtMostOneEquippedWeaponAndArmor(t *testing.T) {
	for class := range startingKits {
		t.Run(string(class), func(t *testing.T) {
			inv := buildStartingInventory(1, class)

			equippedByType := map[inventory.ItemType]int{}
			for _, item := range inv.GetEquippedItems() {
				equippedByType[item.Type]++
			}

			for itemType, count := range equippedByType {
				if count > 1 {
					t.Errorf("class %s: expected at most 1 equipped item of type %s, got %d", class, itemType, count)
				}
			}
		})
	}
}

func TestBuildStartingInventory_FighterKitContents(t *testing.T) {
	inv := buildStartingInventory(7, character.ClassFighter)

	if inv.Gold != 15 {
		t.Errorf("expected fighter starting gold=15, got %d", inv.Gold)
	}

	equipped := inv.GetEquippedItems()
	if len(equipped) != 2 {
		t.Fatalf("expected fighter to start with 2 equipped items (weapon+armor), got %d: %+v", len(equipped), equipped)
	}

	weapon, err := findItem(inv, "Длинный меч")
	if err != nil {
		t.Fatal(err)
	}
	if weapon.Type != inventory.ItemTypeWeapon || !weapon.Equipped {
		t.Errorf("expected Длинный меч to be an equipped weapon, got %+v", weapon)
	}

	armor, err := findItem(inv, "Кольчуга")
	if err != nil {
		t.Fatal(err)
	}
	if armor.Type != inventory.ItemTypeArmor || !armor.Equipped {
		t.Errorf("expected Кольчуга to be equipped armor, got %+v", armor)
	}

	potion, err := findItem(inv, "Зелье лечения")
	if err != nil {
		t.Fatal(err)
	}
	if potion.Equipped {
		t.Errorf("potions should not be equipped")
	}
	if potion.HealingAmount != 10 {
		t.Errorf("expected healing potion HealingAmount=10, got %d", potion.HealingAmount)
	}
}

func findItem(inv *inventory.Inventory, name string) (*inventory.InventoryItem, error) {
	for i := range inv.Items {
		if inv.Items[i].Name == name {
			return &inv.Items[i], nil
		}
	}
	return nil, fmt.Errorf("item not found in inventory: %s", name)
}
