package character

import (
	"testing"
)

func TestNewCharacter(t *testing.T) {
	stats := Stats{
		Strength:     16,
		Dexterity:    14,
		Constitution: 15,
		Intelligence: 12,
		Wisdom:       13,
		Charisma:     10,
	}

	char, err := NewCharacter("Test Hero", ClassFighter, RaceHuman, stats)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if char.Name != "Test Hero" {
		t.Errorf("expected name 'Test Hero', got '%s'", char.Name)
	}

	if char.Class != ClassFighter {
		t.Errorf("expected class %s, got %s", ClassFighter, char.Class)
	}

	if char.Race != RaceHuman {
		t.Errorf("expected race %s, got %s", RaceHuman, char.Race)
	}

	if char.Level != 1 {
		t.Errorf("expected level 1, got %d", char.Level)
	}

	if char.Status != StatusAlive {
		t.Errorf("expected status %s, got %s", StatusAlive, char.Status)
	}

	if char.HP <= 0 {
		t.Errorf("expected HP > 0, got %d", char.HP)
	}

	if char.MaxHP <= 0 {
		t.Errorf("expected MaxHP > 0, got %d", char.MaxHP)
	}

	if char.HP != char.MaxHP {
		t.Errorf("expected HP == MaxHP for new character, got HP=%d, MaxHP=%d", char.HP, char.MaxHP)
	}
}

func TestApplyDamage(t *testing.T) {
	stats := Stats{
		Strength:     16,
		Dexterity:    14,
		Constitution: 15,
		Intelligence: 12,
		Wisdom:       13,
		Charisma:     10,
	}

	char, _ := NewCharacter("Test Hero", ClassFighter, RaceHuman, stats)
	initialHP := char.HP

	// Применяем урон
	damage := 5
	err := char.ApplyDamage(damage)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if char.HP != initialHP-damage {
		t.Errorf("expected HP %d, got %d", initialHP-damage, char.HP)
	}

	if char.Status != StatusAlive {
		t.Errorf("expected status %s, got %s", StatusAlive, char.Status)
	}
}

func TestApplyDamageKillsCharacter(t *testing.T) {
	stats := Stats{
		Strength:     16,
		Dexterity:    14,
		Constitution: 15,
		Intelligence: 12,
		Wisdom:       13,
		Charisma:     10,
	}

	char, _ := NewCharacter("Test Hero", ClassFighter, RaceHuman, stats)
	maxHP := char.MaxHP

	// Применяем смертельный урон
	err := char.ApplyDamage(maxHP + 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if char.HP != 0 {
		t.Errorf("expected HP 0, got %d", char.HP)
	}

	if char.Status != StatusDead {
		t.Errorf("expected status %s, got %s", StatusDead, char.Status)
	}
}

func TestApplyDamageToDeadCharacter(t *testing.T) {
	stats := Stats{
		Strength:     16,
		Dexterity:    14,
		Constitution: 15,
		Intelligence: 12,
		Wisdom:       13,
		Charisma:     10,
	}

	char, _ := NewCharacter("Test Hero", ClassFighter, RaceHuman, stats)
	char.Kill()

	// Попытка нанести урон мертвому персонажу
	err := char.ApplyDamage(5)
	if err == nil {
		t.Error("expected error when applying damage to dead character")
	}

	if char.HP != 0 {
		t.Errorf("expected HP 0, got %d", char.HP)
	}
}

func TestApplyZeroDamage(t *testing.T) {
	stats := Stats{
		Strength:     16,
		Dexterity:    14,
		Constitution: 15,
		Intelligence: 12,
		Wisdom:       13,
		Charisma:     10,
	}

	char, _ := NewCharacter("Test Hero", ClassFighter, RaceHuman, stats)
	initialHP := char.HP

	err := char.ApplyDamage(0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if char.HP != initialHP {
		t.Errorf("expected HP unchanged (%d), got %d", initialHP, char.HP)
	}
}

func TestHeal(t *testing.T) {
	stats := Stats{
		Strength:     16,
		Dexterity:    14,
		Constitution: 15,
		Intelligence: 12,
		Wisdom:       13,
		Charisma:     10,
	}

	char, _ := NewCharacter("Test Hero", ClassFighter, RaceHuman, stats)
	char.ApplyDamage(10) // Наносим урон
	damagedHP := char.HP

	healAmount := 5
	err := char.Heal(healAmount)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if char.HP != damagedHP+healAmount {
		t.Errorf("expected HP %d, got %d", damagedHP+healAmount, char.HP)
	}
}

func TestHealExceedsMaxHP(t *testing.T) {
	stats := Stats{
		Strength:     16,
		Dexterity:    14,
		Constitution: 15,
		Intelligence: 12,
		Wisdom:       13,
		Charisma:     10,
	}

	char, _ := NewCharacter("Test Hero", ClassFighter, RaceHuman, stats)
	char.ApplyDamage(5)
	maxHP := char.MaxHP

	// Лечим больше, чем нужно
	err := char.Heal(100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if char.HP != maxHP {
		t.Errorf("expected HP capped at %d, got %d", maxHP, char.HP)
	}
}

func TestHealDeadCharacter(t *testing.T) {
	stats := Stats{
		Strength:     16,
		Dexterity:    14,
		Constitution: 15,
		Intelligence: 12,
		Wisdom:       13,
		Charisma:     10,
	}

	char, _ := NewCharacter("Test Hero", ClassFighter, RaceHuman, stats)
	char.Kill()

	err := char.Heal(10)
	if err == nil {
		t.Error("expected error when healing dead character")
	}

	if char.HP != 0 {
		t.Errorf("expected HP 0, got %d", char.HP)
	}
}

func TestKill(t *testing.T) {
	stats := Stats{
		Strength:     16,
		Dexterity:    14,
		Constitution: 15,
		Intelligence: 12,
		Wisdom:       13,
		Charisma:     10,
	}

	char, _ := NewCharacter("Test Hero", ClassFighter, RaceHuman, stats)
	char.Kill()

	if char.HP != 0 {
		t.Errorf("expected HP 0, got %d", char.HP)
	}

	if char.Status != StatusDead {
		t.Errorf("expected status %s, got %s", StatusDead, char.Status)
	}
}

func TestHPCalculationByClass(t *testing.T) {
	stats := Stats{
		Constitution: 14, // +2 modifier
	}

	classHP := map[Class]int{
		ClassFighter: 10,
		ClassWizard:  6,
		ClassRogue:   8,
		ClassCleric:  8,
		ClassRanger:  10,
	}

	for class, expectedBaseHP := range classHP {
		t.Run(string(class), func(t *testing.T) {
			char, err := NewCharacter("Test", class, RaceHuman, stats)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			expectedHP := expectedBaseHP + 2 // +2 от конституции
			if char.MaxHP != expectedHP {
				t.Errorf("expected MaxHP %d for %s, got %d", expectedHP, class, char.MaxHP)
			}
		})
	}
}

func TestHPCalculationMinimumHP(t *testing.T) {
	// Тест для проверки минимального HP = 1 (правило D&D 5e)
	// Даже при очень низком Телосложении HP не должно быть меньше 1
	stats := Stats{
		Constitution: 3, // -4 modifier (очень низкое Телосложение)
	}

	// Для волшебника: базовое HP = 6, модификатор = -4, результат = 2
	// Но по правилам D&D 5e минимальное HP = 1
	char, err := NewCharacter("Weak Wizard", ClassWizard, RaceHuman, stats)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// HP должно быть минимум 1, даже если расчет дает меньше
	if char.MaxHP < 1 {
		t.Errorf("expected MinHP >= 1, got %d", char.MaxHP)
	}

	// Для волшебника с Телосложением 3: 6 + (-4) = 2, но минимум 1
	// Но на самом деле с нашим исправлением генерации характеристик, Телосложение не может быть меньше 8
	// Поэтому этот тест проверяет, что даже если Телосложение очень низкое, HP >= 1
	if char.MaxHP < 1 {
		t.Errorf("HP should be at least 1, got %d", char.MaxHP)
	}
}
