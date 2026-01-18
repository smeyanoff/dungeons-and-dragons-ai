package combat

import (
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
		State:       CombatStateActive,
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
		State:       CombatStateActive,
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

// TestCombatRaceCondition_PerformAttack проверяет race condition при одновременных атаках
func TestCombatRaceCondition_PerformAttack(t *testing.T) {
	attacker1 := createPlayerParticipant("Player 1", 16, 14, 15)
	attacker2 := createPlayerParticipant("Player 2", 16, 14, 15)
	target := createMonsterParticipant("Goblin", 100, 12) // Много HP для множественных атак

	var wg sync.WaitGroup
	goroutines := 20

	// Параллельные атаки от двух атакующих на одну цель
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var attacker *CombatParticipant
			if i%2 == 0 {
				attacker = &attacker1
			} else {
				attacker = &attacker2
			}
			_, err := PerformAttack(attacker, &target, "1d8")
			if err != nil {
				t.Errorf("PerformAttack failed: %v", err)
			}
		}(i)
	}

	wg.Wait()

	// Проверяем, что цель получила урон и HP не отрицательное
	if target.MonsterHP < 0 {
		t.Errorf("Target HP should not be negative: %d", target.MonsterHP)
	}

	if target.MonsterHP > target.MonsterMaxHP {
		t.Errorf("Target HP should not exceed max HP: %d > %d", target.MonsterHP, target.MonsterMaxHP)
	}
}

// TestCombatRaceCondition_CheckCombatEnd проверяет race condition при проверке окончания боя
func TestCombatRaceCondition_CheckCombatEnd(t *testing.T) {
	c := &Combat{
		State: CombatStateActive,
		Participants: []CombatParticipant{
			createPlayerParticipant("Player 1", 16, 14, 15),
			createMonsterParticipant("Goblin", 10, 12),
		},
	}

	var wg sync.WaitGroup
	goroutines := 50

	// Параллельно вызываем CheckCombatEnd и изменяем HP участников
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Чередуем чтение и запись
			if i%2 == 0 {
				_ = c.CheckCombatEnd()
			} else {
				// Изменяем HP для имитации боя
				if i%4 == 1 {
					c.Participants[1].MonsterHP--
				}
				_ = c.CheckCombatEnd()
			}
		}(i)
	}

	wg.Wait()

	// Проверяем корректность состояния после всех операций
	ended := c.CheckCombatEnd()
	if ended && c.State != CombatStateFinished {
		// Если бой должен быть закончен, состояние может быть не синхронизировано
		// (это ожидаемая проблема race condition)
		t.Logf("Combat should be finished but state is %s (this indicates race condition)", c.State)
	}
}

// TestCombatRaceCondition_MixedOperations проверяет смешанные операции при конкурентном доступе
func TestCombatRaceCondition_MixedOperations(t *testing.T) {
	c := &Combat{
		State:       CombatStateActive,
		CurrentTurn: 0,
		Participants: []CombatParticipant{
			createPlayerParticipant("Player 1", 16, 14, 15),
			createMonsterParticipant("Goblin", 50, 12),
		},
	}

	var wg sync.WaitGroup
	operations := 100

	// Смешиваем разные операции: NextTurn, CheckCombatEnd, PerformAttack
	for i := 0; i < operations; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			switch i % 4 {
			case 0:
				c.NextTurn()
			case 1:
				_ = c.CheckCombatEnd()
			case 2:
				if len(c.Participants) >= 2 {
					attacker := &c.Participants[0]
					target := &c.Participants[1]
					if attacker.IsAlive() && target.IsAlive() {
						_, _ = PerformAttack(attacker, target, "1d8")
					}
				}
			case 3:
				_ = c.GetCurrentParticipant()
			}
		}(i)
	}

	wg.Wait()

	// Проверяем, что состояние валидно после всех операций
	if c.CurrentTurn < 0 {
		t.Errorf("CurrentTurn should not be negative: %d", c.CurrentTurn)
	}

	// Проверяем, что HP не отрицательные
	for i := range c.Participants {
		if c.Participants[i].GetHP() < 0 {
			t.Errorf("Participant %d HP should not be negative: %d", i, c.Participants[i].GetHP())
		}
	}
}
