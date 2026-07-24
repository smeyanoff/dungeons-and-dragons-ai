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
