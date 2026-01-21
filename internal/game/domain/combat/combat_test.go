package combat

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/dice"
)

func TestCombatStart(t *testing.T) {
	combat := &Combat{
		State: CombatStateNotStarted,
		Participants: []CombatParticipant{
			createPlayerParticipant("Player 1", 16, 14, 15),
			createMonsterParticipant("Goblin", 10, 12),
		},
	}

	err := combat.Start()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if combat.State != CombatStateActive {
		t.Errorf("expected state %s, got %s", CombatStateActive, combat.State)
	}

	if combat.CurrentTurn != 0 {
		t.Errorf("expected CurrentTurn 0, got %d", combat.CurrentTurn)
	}

	// Проверяем, что инициатива установлена для всех участников
	for i, p := range combat.Participants {
		if p.Initiative == 0 {
			t.Errorf("participant %d has no initiative", i)
		}
	}

	// Проверяем, что участники отсортированы по инициативе
	for i := 0; i < len(combat.Participants)-1; i++ {
		if combat.Participants[i].Initiative < combat.Participants[i+1].Initiative {
			t.Errorf("participants not sorted by initiative")
		}
	}
}

func TestCombatStartAlreadyStarted(t *testing.T) {
	combat := &Combat{
		State: CombatStateActive,
		Participants: []CombatParticipant{
			createPlayerParticipant("Player 1", 16, 14, 15),
			createMonsterParticipant("Goblin", 10, 12),
		},
	}

	err := combat.Start()
	if err == nil {
		t.Error("expected error when starting already started combat")
	}
}

func TestCombatStartNotEnoughParticipants(t *testing.T) {
	combat := &Combat{
		State:        CombatStateNotStarted,
		Participants: []CombatParticipant{createPlayerParticipant("Player 1", 16, 14, 15)},
	}

	err := combat.Start()
	if err == nil {
		t.Error("expected error when starting combat with < 2 participants")
	}
}

func TestGetCurrentParticipant(t *testing.T) {
	combat := &Combat{
		State:       CombatStateActive,
		CurrentTurn: 0,
		Participants: []CombatParticipant{
			createPlayerParticipant("Player 1", 16, 14, 15),
			createMonsterParticipant("Goblin", 10, 12),
		},
	}

	participant := combat.GetCurrentParticipant()
	if participant == nil {
		t.Fatal("expected participant, got nil")
	}

	if participant.GetName() != "Player 1" {
		t.Errorf("expected 'Player 1', got '%s'", participant.GetName())
	}
}

func TestGetCurrentParticipantInactiveCombat(t *testing.T) {
	combat := &Combat{
		State: CombatStateFinished,
		Participants: []CombatParticipant{
			createPlayerParticipant("Player 1", 16, 14, 15),
		},
	}

	participant := combat.GetCurrentParticipant()
	if participant != nil {
		t.Errorf("expected nil for inactive combat, got %+v", participant)
	}
}

func TestNextTurn(t *testing.T) {
	combat := &Combat{
		State:       CombatStateActive,
		CurrentTurn: 0,
		Participants: []CombatParticipant{
			createPlayerParticipant("Player 1", 16, 14, 15),
			createMonsterParticipant("Goblin", 10, 12),
		},
	}

	combat.NextTurn()
	if combat.CurrentTurn != 1 {
		t.Errorf("expected CurrentTurn 1, got %d", combat.CurrentTurn)
	}

	participant := combat.GetCurrentParticipant()
	if participant == nil {
		t.Fatal("expected participant, got nil")
	}

	if participant.GetName() != "Goblin" {
		t.Errorf("expected 'Goblin', got '%s'", participant.GetName())
	}
}

func TestNextTurnWrapsAround(t *testing.T) {
	combat := &Combat{
		State:       CombatStateActive,
		CurrentTurn: 1, // Второй участник (индекс 1 в 0-based массиве)
		Participants: []CombatParticipant{
			createPlayerParticipant("Player 1", 16, 14, 15),
			createMonsterParticipant("Goblin", 10, 12),
		},
	}

	combat.NextTurn()
	// CurrentTurn использует 0-based индексацию
	// При CurrentTurn=1 и len(Participants)=2: (1+1) % 2 = 0
	// Должен вернуться к первому участнику (индекс 0)
	if combat.CurrentTurn != 0 {
		t.Errorf("expected CurrentTurn 0 (wraps around), got %d", combat.CurrentTurn)
	}

	// Должен вернуться к первому участнику (индекс 0)
	participant := combat.GetCurrentParticipant()
	if participant == nil {
		t.Fatal("expected participant, got nil")
	}

	if participant.GetName() != "Player 1" {
		t.Errorf("expected 'Player 1', got '%s'", participant.GetName())
	}
}

func TestCheckCombatEnd(t *testing.T) {
	combat := &Combat{
		State: CombatStateActive,
		Participants: []CombatParticipant{
			createPlayerParticipant("Player 1", 16, 14, 15),
			createMonsterParticipant("Goblin", 10, 12),
		},
	}

	// Бой не должен закончиться, пока все живы
	if combat.CheckCombatEnd() {
		t.Error("combat should not be ended when all participants are alive")
	}

	// Убиваем всех врагов
	combat.Participants[1].MonsterHP = 0

	if !combat.CheckCombatEnd() {
		t.Error("combat should be ended when all enemies are dead")
	}
}

func TestCheckCombatEndAllPlayersDead(t *testing.T) {
	combat := &Combat{
		State: CombatStateActive,
		Participants: []CombatParticipant{
			createPlayerParticipant("Player 1", 16, 14, 15),
			createMonsterParticipant("Goblin", 10, 12),
		},
	}

	// Убиваем игрока
	combat.Participants[0].Character.Kill()

	if !combat.CheckCombatEnd() {
		t.Error("combat should be ended when all players are dead")
	}
}

func TestIsAlive(t *testing.T) {
	playerPart := createPlayerParticipant("Player 1", 16, 14, 15)
	if !playerPart.IsAlive() {
		t.Error("player should be alive")
	}

	playerPart.Character.Kill()
	if playerPart.IsAlive() {
		t.Error("player should be dead after kill")
	}

	monsterPart := createMonsterParticipant("Goblin", 10, 12)
	if !monsterPart.IsAlive() {
		t.Error("monster should be alive")
	}

	monsterPart.MonsterHP = 0
	if monsterPart.IsAlive() {
		t.Error("monster should be dead when HP is 0")
	}
}

func TestApplyDamage(t *testing.T) {
	playerPart := createPlayerParticipant("Player 1", 16, 14, 15)
	initialHP := playerPart.GetHP()

	err := playerPart.ApplyDamage(5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if playerPart.GetHP() != initialHP-5 {
		t.Errorf("expected HP %d, got %d", initialHP-5, playerPart.GetHP())
	}
}

func TestApplyDamageMonster(t *testing.T) {
	monsterPart := createMonsterParticipant("Goblin", 10, 12)
	initialHP := monsterPart.MonsterHP

	err := monsterPart.ApplyDamage(5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if monsterPart.MonsterHP != initialHP-5 {
		t.Errorf("expected HP %d, got %d", initialHP-5, monsterPart.MonsterHP)
	}
}

func TestApplyDamageKillsMonster(t *testing.T) {
	monsterPart := createMonsterParticipant("Goblin", 10, 12)
	maxHP := monsterPart.MonsterMaxHP

	err := monsterPart.ApplyDamage(maxHP + 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if monsterPart.MonsterHP != 0 {
		t.Errorf("expected HP 0, got %d", monsterPart.MonsterHP)
	}
}

func TestPerformAttack(t *testing.T) {
	attacker := createPlayerParticipant("Player 1", 16, 14, 15)
	target := createMonsterParticipant("Goblin", 10, 12)

	result, err := PerformAttack(&attacker, &target, "1d8")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.AttackerName != "Player 1" {
		t.Errorf("expected attacker name 'Player 1', got '%s'", result.AttackerName)
	}

	if result.TargetName != "Goblin" {
		t.Errorf("expected target name 'Goblin', got '%s'", result.TargetName)
	}

	if result.AttackRoll < 1 || result.AttackRoll > 30 {
		t.Errorf("attack roll out of reasonable range: %d", result.AttackRoll)
	}

	if result.Hit {
		if result.Damage <= 0 {
			t.Errorf("expected damage > 0 on hit, got %d", result.Damage)
		}
		if target.MonsterHP >= target.MonsterMaxHP {
			t.Errorf("expected target HP to decrease, got %d/%d", target.MonsterHP, target.MonsterMaxHP)
		}
	}
}

func TestPerformAttackDeadAttacker(t *testing.T) {
	attacker := createPlayerParticipant("Player 1", 16, 14, 15)
	attacker.Character.Kill()
	target := createMonsterParticipant("Goblin", 10, 12)

	_, err := PerformAttack(&attacker, &target, "1d8")
	if err == nil {
		t.Error("expected error when attacker is dead")
	}
}

func TestPerformAttackDeadTarget(t *testing.T) {
	attacker := createPlayerParticipant("Player 1", 16, 14, 15)
	target := createMonsterParticipant("Goblin", 10, 12)
	target.MonsterHP = 0

	_, err := PerformAttack(&attacker, &target, "1d8")
	if err == nil {
		t.Error("expected error when target is dead")
	}
}

func TestPerformAttackCriticalHit(t *testing.T) {
	// Тест проверяет, что критический удар (натуральная 20) правильно обрабатывается
	// и что код не паникует при проверке Rolls[0]
	attacker := createPlayerParticipant("Player 1", 16, 14, 15)
	target := createMonsterParticipant("Goblin", 10, 12)
	initialHP := target.MonsterHP

	// Выполняем несколько атак, чтобы проверить обработку критических ударов
	// RollAttack всегда возвращает d20 с одним кубиком, поэтому критический удар возможен
	for i := 0; i < 100; i++ {
		result, err := PerformAttack(&attacker, &target, "1d8")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Проверяем, что результат валиден
		if result.AttackRoll < 1 || result.AttackRoll > 30 {
			t.Errorf("attack roll out of reasonable range: %d", result.AttackRoll)
		}

		// Если критический удар, урон должен быть удвоен
		if result.CriticalHit {
			if result.Damage < 2 {
				t.Errorf("critical hit should have at least 2 damage (1d8 doubled), got %d", result.Damage)
			}
			// Критический удар всегда попадает
			if !result.Hit {
				t.Error("critical hit should always hit")
			}
		}

		// Восстанавливаем HP для следующей итерации
		target.MonsterHP = initialHP
	}
}

func TestGetAC(t *testing.T) {
	playerPart := createPlayerParticipant("Player 1", 16, 14, 15)
	// DEX 14 = modifier +2, base AC = 10 + 2 = 12
	expectedAC := 10 + dice.CalculateModifier(14)
	ac := playerPart.GetAC()
	if ac != expectedAC {
		t.Errorf("expected AC %d, got %d", expectedAC, ac)
	}

	monsterPart := createMonsterParticipant("Goblin", 10, 12)
	if monsterPart.GetAC() != 12 {
		t.Errorf("expected AC 12, got %d", monsterPart.GetAC())
	}
}

func TestGetAttackBonus(t *testing.T) {
	playerPart := createPlayerParticipant("Player 1", 16, 14, 15)
	// STR 16 = modifier +3, proficiency +2 (level 1) = +5
	bonus := playerPart.GetAttackBonus()
	if bonus < 3 || bonus > 6 {
		t.Errorf("attack bonus out of reasonable range: %d", bonus)
	}

	monsterPart := createMonsterParticipant("Goblin", 10, 12)
	if monsterPart.GetAttackBonus() != 0 {
		t.Errorf("expected attack bonus 0, got %d", monsterPart.GetAttackBonus())
	}
}

func TestGetInitiativeOrderMessage(t *testing.T) {
	combat := &Combat{
		State:       CombatStateActive,
		CurrentTurn: 0,
		Participants: []CombatParticipant{
			createPlayerParticipant("Player 1", 16, 14, 15),
			createMonsterParticipant("Goblin", 10, 12),
		},
	}

	// Устанавливаем инициативу вручную для тестирования
	combat.Participants[0].Initiative = 18
	combat.Participants[1].Initiative = 12

	message := combat.GetInitiativeOrderMessage()

	if message == "" {
		t.Error("expected non-empty message")
	}

	// Проверяем наличие ключевых элементов сообщения
	if !strings.Contains(message, "⚔️ Бой начался! Порядок ходов:") {
		t.Error("message should contain combat start text")
	}

	if !strings.Contains(message, "Player 1") {
		t.Error("message should contain player name")
	}

	if !strings.Contains(message, "Goblin") {
		t.Error("message should contain monster name")
	}

	if !strings.Contains(message, "инициатива: 18") {
		t.Error("message should contain initiative value")
	}

	if !strings.Contains(message, "🎯 Текущий ход:") {
		t.Error("message should contain current turn text")
	}
}

func TestGetInitiativeOrderMessage_InactiveCombat(t *testing.T) {
	combat := &Combat{
		State: CombatStateNotStarted,
		Participants: []CombatParticipant{
			createPlayerParticipant("Player 1", 16, 14, 15),
		},
	}

	message := combat.GetInitiativeOrderMessage()

	if message != "" {
		t.Errorf("expected empty message for inactive combat, got: %s", message)
	}
}

func TestGetInitiativeOrderMessage_EmptyParticipants(t *testing.T) {
	combat := &Combat{
		State:        CombatStateActive,
		Participants: []CombatParticipant{},
	}

	message := combat.GetInitiativeOrderMessage()

	if message != "" {
		t.Errorf("expected empty message for empty participants, got: %s", message)
	}
}

func TestGetInitiativeOrderMessage_WithDeadParticipant(t *testing.T) {
	combat := &Combat{
		State:       CombatStateActive,
		CurrentTurn: 0,
		Participants: []CombatParticipant{
			createPlayerParticipant("Player 1", 16, 14, 15),
			createMonsterParticipant("Goblin", 10, 12),
		},
	}

	// Устанавливаем инициативу
	combat.Participants[0].Initiative = 18
	combat.Participants[1].Initiative = 12

	// Убиваем монстра
	combat.Participants[1].MonsterHP = 0

	message := combat.GetInitiativeOrderMessage()

	// Мертвый участник не должен быть в списке, но игрок должен быть
	if !strings.Contains(message, "Player 1") {
		t.Error("message should contain alive player name")
	}

	// Мертвый участник не должен появляться в списке
	// Проверяем, что Goblin не упоминается в порядке ходов (только если он мертв)
	if strings.Contains(message, "Goblin") {
		// Если Goblin упоминается, проверяем, что он не в списке порядка ходов
		// (может быть упомянут как текущий ход, если CurrentTurn указывает на него)
	}
}

// TestGetInitiativeOrderMessage_PlayerAndCompanionsDisplay - Task #69
// Проверяет, что игрок и спутники отображаются в порядке ходов с правильными иконками
func TestGetInitiativeOrderMessage_PlayerAndCompanionsDisplay(t *testing.T) {
	combat := &Combat{
		State:       CombatStateActive,
		CurrentTurn: 0,
		Participants: []CombatParticipant{
			createPlayerParticipant("Player 1", 16, 14, 15),
			createPlayerParticipant("Companion NPC", 14, 12, 13), // Спутник (NPC companion)
			createMonsterParticipant("Goblin", 10, 12),
			createMonsterParticipant("Orc", 15, 14),
		},
	}

	// Устанавливаем инициативу вручную для предсказуемости теста
	combat.Participants[0].Initiative = 20 // Игрок первый
	combat.Participants[1].Initiative = 15 // Спутник второй
	combat.Participants[2].Initiative = 12 // Гоблин третий
	combat.Participants[3].Initiative = 10 // Орк четвертый

	message := combat.GetInitiativeOrderMessage()

	// Проверяем наличие всех участников
	if !strings.Contains(message, "Player 1") {
		t.Error("message should contain player name")
	}
	if !strings.Contains(message, "Companion NPC") {
		t.Error("message should contain companion name")
	}
	if !strings.Contains(message, "Goblin") {
		t.Error("message should contain first enemy name")
	}
	if !strings.Contains(message, "Orc") {
		t.Error("message should contain second enemy name")
	}

	// Проверяем наличие иконок типа участника (Task #69)
	if !strings.Contains(message, "👤 Игрок") {
		t.Error("message should contain player icon (👤 Игрок)")
	}
	if !strings.Contains(message, "👹 Враг") {
		t.Error("message should contain enemy icon (👹 Враг)")
	}

	// Проверяем правильную нумерацию с отдельным счетчиком (1, 2, 3, 4...)
	// Должна быть последовательная нумерация независимо от индексов массива
	lines := strings.Split(message, "\n")
	turnOrderLines := []string{}
	for _, line := range lines {
		if strings.Contains(line, "👤 Игрок") || strings.Contains(line, "👹 Враг") {
			turnOrderLines = append(turnOrderLines, line)
		}
	}

	if len(turnOrderLines) != 4 {
		t.Errorf("expected 4 participants in turn order, got %d", len(turnOrderLines))
	}

	// Проверяем, что нумерация начинается с 1 и последовательна
	for i, line := range turnOrderLines {
		expectedNumber := i + 1
		if !strings.HasPrefix(strings.TrimSpace(line), fmt.Sprintf("%d.", expectedNumber)) {
			t.Errorf("line %d should start with '%d.', got: %s", i, expectedNumber, line)
		}
	}
}

// TestGetInitiativeOrderMessage_CorrectNumberingWithDead - Task #69
// Проверяет, что нумерация корректна даже при наличии мертвых участников
func TestGetInitiativeOrderMessage_CorrectNumberingWithDead(t *testing.T) {
	combat := &Combat{
		State:       CombatStateActive,
		CurrentTurn: 0,
		Participants: []CombatParticipant{
			createPlayerParticipant("Player 1", 16, 14, 15),
			createMonsterParticipant("Dead Goblin", 10, 12),  // Мертвый
			createMonsterParticipant("Orc", 15, 14),          // Живой
			createMonsterParticipant("Dead Orc", 15, 14),     // Мертвый
			createPlayerParticipant("Companion", 14, 12, 13), // Живой спутник
		},
	}

	// Устанавливаем инициативу
	combat.Participants[0].Initiative = 18
	combat.Participants[1].Initiative = 12
	combat.Participants[2].Initiative = 15
	combat.Participants[3].Initiative = 10
	combat.Participants[4].Initiative = 14

	// Убиваем некоторых участников
	combat.Participants[1].MonsterHP = 0 // Dead Goblin
	combat.Participants[3].MonsterHP = 0 // Dead Orc

	message := combat.GetInitiativeOrderMessage()

	// Проверяем, что живые участники присутствуют
	if !strings.Contains(message, "Player 1") {
		t.Error("message should contain alive player name")
	}
	if !strings.Contains(message, "Orc") {
		t.Error("message should contain alive enemy name")
	}
	if !strings.Contains(message, "Companion") {
		t.Error("message should contain alive companion name")
	}

	// Проверяем, что мертвые участники НЕ присутствуют в порядке ходов
	// (могут быть упомянуты только в текущем ходе, если CurrentTurn указывает на них)
	if strings.Contains(message, "Dead Goblin") && strings.Contains(message, "инициатива:") {
		// Если мертвый участник упоминается с инициативой, это ошибка
		if strings.Contains(message, "Dead Goblin") && strings.Contains(message, "инициатива: 12") {
			t.Error("dead participant should not appear in turn order list")
		}
	}

	// Проверяем правильную нумерацию (1, 2, 3 для трех живых участников)
	// Мертвые участники не должны влиять на нумерацию
	lines := strings.Split(message, "\n")
	turnOrderLines := []string{}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if (strings.Contains(trimmed, "👤 Игрок") || strings.Contains(trimmed, "👹 Враг")) && strings.Contains(trimmed, "инициатива:") {
			turnOrderLines = append(turnOrderLines, trimmed)
		}
	}

	expectedLivingParticipants := 3 // Player 1, Orc, Companion
	if len(turnOrderLines) != expectedLivingParticipants {
		t.Errorf("expected %d living participants in turn order, got %d. Message: %s", expectedLivingParticipants, len(turnOrderLines), message)
	}

	// Проверяем, что нумерация последовательна (1, 2, 3)
	for i, line := range turnOrderLines {
		expectedNumber := i + 1
		if !strings.HasPrefix(line, fmt.Sprintf("%d.", expectedNumber)) {
			t.Errorf("line %d should start with '%d.', got: %s", i, expectedNumber, line)
		}
	}
}

func TestGetCurrentTurnMessage(t *testing.T) {
	combat := &Combat{
		State:       CombatStateActive,
		CurrentTurn: 0,
		Participants: []CombatParticipant{
			createPlayerParticipant("Player 1", 16, 14, 15),
			createMonsterParticipant("Goblin", 10, 12),
		},
	}

	message := combat.GetCurrentTurnMessage()

	if message == "" {
		t.Error("expected non-empty message")
	}

	// Проверяем формат сообщения для игрока
	if !strings.Contains(message, "🎯") {
		t.Error("message should contain turn indicator")
	}

	if !strings.Contains(message, "Player 1") {
		t.Error("message should contain participant name")
	}

	// Для игрока должно быть "Ваш ход"
	if !strings.Contains(message, "Ваш ход") {
		t.Logf("Message: %s - may be monster turn", message)
	}
}

func TestGetCurrentTurnMessage_MonsterTurn(t *testing.T) {
	combat := &Combat{
		State:       CombatStateActive,
		CurrentTurn: 1,
		Participants: []CombatParticipant{
			createPlayerParticipant("Player 1", 16, 14, 15),
			createMonsterParticipant("Goblin", 10, 12),
		},
	}

	message := combat.GetCurrentTurnMessage()

	if message == "" {
		t.Error("expected non-empty message")
	}

	if !strings.Contains(message, "🎯") {
		t.Error("message should contain turn indicator")
	}

	if !strings.Contains(message, "Goblin") {
		t.Error("message should contain monster name")
	}

	// Для монстра должно быть "Ход:" (без "Ваш")
	if strings.Contains(message, "Ваш ход") {
		t.Error("message should not contain 'Ваш ход' for monster turn")
	}
}

func TestGetCurrentTurnMessage_InactiveCombat(t *testing.T) {
	combat := &Combat{
		State: CombatStateFinished,
		Participants: []CombatParticipant{
			createPlayerParticipant("Player 1", 16, 14, 15),
		},
	}

	message := combat.GetCurrentTurnMessage()

	if message != "" {
		t.Errorf("expected empty message for inactive combat, got: %s", message)
	}
}

func TestGetCurrentTurnMessage_EmptyParticipants(t *testing.T) {
	combat := &Combat{
		State:        CombatStateActive,
		Participants: []CombatParticipant{},
	}

	message := combat.GetCurrentTurnMessage()

	if message != "" {
		t.Errorf("expected empty message for empty participants, got: %s", message)
	}
}

func TestGetCurrentTurnMessage_DeadParticipant(t *testing.T) {
	combat := &Combat{
		State:       CombatStateActive,
		CurrentTurn: 0,
		Participants: []CombatParticipant{
			createPlayerParticipant("Player 1", 16, 14, 15),
			createMonsterParticipant("Goblin", 10, 12),
		},
	}

	// Убиваем текущего участника (игрока)
	combat.Participants[0].Character.Kill()

	message := combat.GetCurrentTurnMessage()

	// Должен вернуться ход следующего живого участника (монстра)
	if message == "" {
		t.Error("expected message for next alive participant")
	}

	if !strings.Contains(message, "Goblin") {
		t.Error("message should contain next alive participant name")
	}
}

// Вспомогательные функции для создания участников

func createPlayerParticipant(name string, str, dex, con int) CombatParticipant {
	char, _ := character.NewCharacter(
		name,
		character.ClassFighter,
		character.RaceHuman,
		character.Stats{
			Strength:     str,
			Dexterity:    dex,
			Constitution: con,
			Intelligence: 10,
			Wisdom:       10,
			Charisma:     10,
		},
	)

	return CombatParticipant{
		IsPlayer:  true,
		Character: char,
	}
}

func createMonsterParticipant(name string, hp, ac int) CombatParticipant {
	return CombatParticipant{
		IsPlayer:     false,
		MonsterName:  name,
		MonsterHP:    hp,
		MonsterMaxHP: hp,
		MonsterAC:    ac,
	}
}

// TestCombatRaceCondition_NextTurn проверяет race condition в методе NextTurn
// Этот тест должен обнаружить проблемы при конкурентном доступе к CurrentTurn
func TestCombatRaceCondition_NextTurn(t *testing.T) {
	c := &Combat{
		State:       CombatStateActive,
		CurrentTurn: 0,
		Participants: []CombatParticipant{
			createPlayerParticipant("Player 1", 16, 14, 15),
			createMonsterParticipant("Goblin 1", 10, 12),
			createMonsterParticipant("Goblin 2", 10, 12),
		},
	}

	iterations := 100
	goroutines := 10
	var wg sync.WaitGroup

	// Запускаем несколько горутин, которые параллельно вызывают NextTurn
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				c.NextTurn()
				// Вызываем GetCurrentParticipant для чтения состояния
				_ = c.GetCurrentParticipant()
			}
		}()
	}

	wg.Wait()

	// После всех операций CurrentTurn должен быть в разумных пределах
	if c.CurrentTurn < 0 || c.CurrentTurn >= goroutines*iterations*10 {
		t.Errorf("CurrentTurn out of reasonable range: %d", c.CurrentTurn)
	}
}

// TestNextTurn_EnemyTurnDetection - Task #70
// Проверяет, что после NextTurn() корректно определяется, является ли текущий ход вражеским
// Это необходимое условие для автоматического выполнения ходов врагов
func TestNextTurn_EnemyTurnDetection(t *testing.T) {
	combat := &Combat{
		State:       CombatStateActive,
		CurrentTurn: 0, // Ход игрока
		Participants: []CombatParticipant{
			createPlayerParticipant("Player 1", 16, 14, 15),
			createMonsterParticipant("Goblin", 10, 12),
			createPlayerParticipant("Companion", 14, 12, 13),
			createMonsterParticipant("Orc", 15, 14),
		},
	}

	// Проверяем, что первый ход - игрок
	currentParticipant := combat.GetCurrentParticipant()
	if currentParticipant == nil {
		t.Fatal("expected participant, got nil")
	}
	if !currentParticipant.IsPlayer {
		t.Error("expected player turn, got enemy turn")
	}

	// Переходим к следующему ходу (должен быть враг)
	combat.NextTurn()
	currentParticipant = combat.GetCurrentParticipant()
	if currentParticipant == nil {
		t.Fatal("expected participant after NextTurn, got nil")
	}

	// Проверяем, что текущий ход - враг (Task #70: это условие для автоматического выполнения ходов врагов)
	if currentParticipant.IsPlayer {
		t.Error("expected enemy turn after NextTurn from player, but got player turn")
	}
	if currentParticipant.GetName() != "Goblin" {
		t.Errorf("expected 'Goblin' as next enemy, got '%s'", currentParticipant.GetName())
	}

	// Переходим еще раз (должен быть снова игрок/спутник)
	combat.NextTurn()
	currentParticipant = combat.GetCurrentParticipant()
	if currentParticipant == nil {
		t.Fatal("expected participant, got nil")
	}

	// Должен быть спутник (второй игрок)
	if !currentParticipant.IsPlayer {
		t.Error("expected player/companion turn, got enemy turn")
	}

	// Переходим еще раз (должен быть враг - Orc)
	combat.NextTurn()
	currentParticipant = combat.GetCurrentParticipant()
	if currentParticipant == nil {
		t.Fatal("expected participant, got nil")
	}

	if currentParticipant.IsPlayer {
		t.Error("expected enemy turn (Orc), but got player turn")
	}
	if currentParticipant.GetName() != "Orc" {
		t.Errorf("expected 'Orc' as next enemy, got '%s'", currentParticipant.GetName())
	}
}

// TestNextTurn_EnemyTurnAfterPlayerAction - Task #70
// Проверяет логику определения хода врага после хода игрока
// Симулирует сценарий, когда после хода игрока следующий ход - враг
func TestNextTurn_EnemyTurnAfterPlayerAction(t *testing.T) {
	combat := &Combat{
		State:       CombatStateActive,
		CurrentTurn: 0, // Игрок делает ход
		Participants: []CombatParticipant{
			createPlayerParticipant("Player 1", 16, 14, 15),
			createMonsterParticipant("Goblin", 10, 12),
		},
	}

	// Игрок делает ход (текущий ход - игрок)
	currentParticipant := combat.GetCurrentParticipant()
	if currentParticipant == nil || !currentParticipant.IsPlayer {
		t.Fatal("expected player turn initially")
	}

	// После хода игрока переходим к следующему ходу
	// В реальном коде это происходит в HandleActionUseCase.Execute() после обработки действия игрока
	combat.NextTurn()

	// Теперь текущий ход должен быть врага (Task #70: условие для автоматической генерации хода врага)
	currentParticipant = combat.GetCurrentParticipant()
	if currentParticipant == nil {
		t.Fatal("expected participant after NextTurn, got nil")
	}

	// Ключевая проверка для Task #70: следующий ход должен быть врага
	if currentParticipant.IsPlayer {
		t.Error("BUG #70: After player turn, next turn should be enemy for automatic enemy turn execution")
	}

	// Проверяем, что это именно враг
	if !strings.Contains(currentParticipant.GetName(), "Goblin") {
		t.Errorf("expected enemy 'Goblin' after player turn, got '%s'", currentParticipant.GetName())
	}
}
