package inventory

import "testing"

func TestAddItem_SetsHealingAmount(t *testing.T) {
	inv := NewInventory(1)

	if err := inv.AddItem("Зелье лечения", "Восстанавливает HP", 0.5, 2, ItemTypePotion, 10); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(inv.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(inv.Items))
	}
	if inv.Items[0].HealingAmount != 10 {
		t.Errorf("expected HealingAmount=10, got %d", inv.Items[0].HealingAmount)
	}
	if inv.Items[0].Quantity != 2 {
		t.Errorf("expected Quantity=2, got %d", inv.Items[0].Quantity)
	}
}

func TestAddItem_MergeKeepsHealingAmount(t *testing.T) {
	inv := NewInventory(1)

	if err := inv.AddItem("Зелье лечения", "Восстанавливает HP", 0.5, 1, ItemTypePotion, 10); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Повторное добавление того же предмета без явного healing_amount не должно обнулять уже известное значение.
	if err := inv.AddItem("Зелье лечения", "Восстанавливает HP", 0.5, 1, ItemTypePotion, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(inv.Items) != 1 {
		t.Fatalf("expected items to merge into 1, got %d", len(inv.Items))
	}
	if inv.Items[0].Quantity != 2 {
		t.Errorf("expected merged Quantity=2, got %d", inv.Items[0].Quantity)
	}
	if inv.Items[0].HealingAmount != 10 {
		t.Errorf("expected HealingAmount to remain 10 after merge, got %d", inv.Items[0].HealingAmount)
	}
}

func TestRemoveItem_ReturnsRemovedSnapshotAndDecrementsQuantity(t *testing.T) {
	inv := NewInventory(1)
	if err := inv.AddItem("Зелье лечения", "Восстанавливает HP", 0.5, 3, ItemTypePotion, 10); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	removed, err := inv.RemoveItem("Зелье лечения", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if removed == nil {
		t.Fatal("expected non-nil removed item snapshot")
	}
	if removed.HealingAmount != 10 {
		t.Errorf("expected removed snapshot HealingAmount=10, got %d", removed.HealingAmount)
	}
	if removed.Quantity != 1 {
		t.Errorf("expected removed snapshot Quantity=1 (amount actually removed), got %d", removed.Quantity)
	}

	// Инвентарь должен уменьшиться, а не увеличиться.
	if len(inv.Items) != 1 || inv.Items[0].Quantity != 2 {
		t.Fatalf("expected remaining quantity=2 in inventory after removing 1 of 3, got %+v", inv.Items)
	}
}

func TestRemoveItem_LastUnitRemovesEntryEntirely(t *testing.T) {
	inv := NewInventory(1)
	if err := inv.AddItem("Факел", "Освещает путь", 0.5, 1, ItemTypeTool, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	removed, err := inv.RemoveItem("Факел", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if removed.Name != "Факел" {
		t.Errorf("expected removed item name 'Факел', got %q", removed.Name)
	}
	if len(inv.Items) != 0 {
		t.Errorf("expected item to be fully removed from inventory, got %+v", inv.Items)
	}
}

func TestRemoveItem_NotEnoughQuantity(t *testing.T) {
	inv := NewInventory(1)
	if err := inv.AddItem("Стрела", "Обычная стрела", 0.1, 2, ItemTypeMisc, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := inv.RemoveItem("Стрела", 5); err == nil {
		t.Error("expected error when removing more items than available")
	}
}

func TestRemoveItem_NotFound(t *testing.T) {
	inv := NewInventory(1)

	if _, err := inv.RemoveItem("Несуществующий предмет", 1); err == nil {
		t.Error("expected error when removing non-existent item")
	}
}

func TestAddItem_MergeIsCaseInsensitive(t *testing.T) {
	inv := NewInventory(1)
	if err := inv.AddItem("Меч Авроры", "Светящийся клинок", 2, 1, ItemTypeWeapon, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Тот же предмет, но с другим регистром и пробелами по краям - должен объединиться,
	// а не создать второй предмет с тем же именем.
	if err := inv.AddItem(" меч авроры ", "Светящийся клинок", 2, 1, ItemTypeWeapon, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(inv.Items) != 1 {
		t.Fatalf("expected items to merge into 1, got %d: %+v", len(inv.Items), inv.Items)
	}
	if inv.Items[0].Quantity != 2 {
		t.Errorf("expected merged Quantity=2, got %d", inv.Items[0].Quantity)
	}
}

func TestEquipItem_EquipsWeaponAndUnequipsPreviousInSameSlot(t *testing.T) {
	inv := NewInventory(1)
	_ = inv.AddItem("Кинжал", "Простой кинжал", 1, 1, ItemTypeWeapon, 0)
	_ = inv.AddItem("Меч Авроры", "Светящийся клинок", 2, 1, ItemTypeWeapon, 0)

	if _, err := inv.EquipItem("Кинжал"); err != nil {
		t.Fatalf("unexpected error equipping Кинжал: %v", err)
	}
	if !inv.Items[0].Equipped {
		t.Fatalf("expected Кинжал to be equipped")
	}

	equipped, err := inv.EquipItem("Меч Авроры")
	if err != nil {
		t.Fatalf("unexpected error equipping Меч Авроры: %v", err)
	}
	if !equipped.Equipped {
		t.Errorf("expected returned item to be equipped")
	}
	if inv.Items[0].Equipped {
		t.Errorf("expected Кинжал to be automatically unequipped when equipping another weapon")
	}
	if !inv.Items[1].Equipped {
		t.Errorf("expected Меч Авроры to be equipped")
	}
}

func TestEquipItem_NonEquippableType(t *testing.T) {
	inv := NewInventory(1)
	_ = inv.AddItem("Зелье лечения", "Восстанавливает HP", 0.5, 1, ItemTypePotion, 10)

	if _, err := inv.EquipItem("Зелье лечения"); err == nil {
		t.Error("expected error when equipping a non-equippable item type")
	}
}

func TestEquipItem_NotFound(t *testing.T) {
	inv := NewInventory(1)

	if _, err := inv.EquipItem("Несуществующий предмет"); err == nil {
		t.Error("expected error when equipping non-existent item")
	}
}

func TestUnequipItem_UnequipsAndErrorsIfNotEquipped(t *testing.T) {
	inv := NewInventory(1)
	_ = inv.AddItem("Кольчуга", "Лёгкая броня", 5, 1, ItemTypeArmor, 0)

	if _, err := inv.UnequipItem("Кольчуга"); err == nil {
		t.Error("expected error when unequipping an item that isn't equipped")
	}

	if _, err := inv.EquipItem("Кольчуга"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	unequipped, err := inv.UnequipItem("Кольчуга")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if unequipped.Equipped {
		t.Errorf("expected item to be unequipped")
	}
	if inv.Items[0].Equipped {
		t.Errorf("expected inventory item to reflect unequipped state")
	}
}

func TestGetEquippedItems(t *testing.T) {
	inv := NewInventory(1)
	_ = inv.AddItem("Кольчуга", "Лёгкая броня", 5, 1, ItemTypeArmor, 0)
	_ = inv.AddItem("Меч Авроры", "Светящийся клинок", 2, 1, ItemTypeWeapon, 0)
	_ = inv.AddItem("Факел", "Освещает путь", 0.5, 1, ItemTypeTool, 0)

	if len(inv.GetEquippedItems()) != 0 {
		t.Fatalf("expected no equipped items initially")
	}

	_, _ = inv.EquipItem("Кольчуга")
	_, _ = inv.EquipItem("Меч Авроры")

	equipped := inv.GetEquippedItems()
	if len(equipped) != 2 {
		t.Fatalf("expected 2 equipped items, got %d: %+v", len(equipped), equipped)
	}
}
