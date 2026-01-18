package dm_analyzer

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"dungeons-and-dragons-ai/internal/game/domain/combat"
	"dungeons-and-dragons-ai/internal/game/domain/inventory"
	"dungeons-and-dragons-ai/internal/game/domain/quest"
	"dungeons-and-dragons-ai/internal/llm/domain"
)

// DMResponseAnalysis содержит структурированный анализ ответа DM
type DMResponseAnalysis struct {
	// Боевая ситуация
	CombatDetected     bool    `json:"combat_detected"`                // Начался ли бой
	Enemies            []Enemy `json:"enemies,omitempty"`              // Список врагов, если бой начался
	CombatStartMessage string  `json:"combat_start_message,omitempty"` // Сообщение о порядке ходов при начале боя

	// Квесты
	QuestCompleted bool   `json:"quest_completed"`       // Выполнен ли квест
	QuestFailed    bool   `json:"quest_failed"`          // Провален ли квест
	QuestTitle     string `json:"quest_title,omitempty"` // Название квеста (если выполнен/провален)

	// Опыт
	ExperienceGained int    `json:"experience_gained"`           // Количество опыта (0 если нет)
	ExperienceReason string `json:"experience_reason,omitempty"` // Причина начисления опыта

	// Предметы
	ItemsReceived []Item `json:"items_received,omitempty"` // Предметы, полученные игроком
}

// Enemy представляет врага в бою
type Enemy struct {
	Name        string `json:"name"`         // Имя врага
	HP          int    `json:"hp"`           // HP врага
	AC          int    `json:"ac"`           // Класс брони
	AttackBonus int    `json:"attack_bonus"` // Бонус к атаке
}

// Item представляет предмет, полученный игроком
type Item struct {
	Name        string  `json:"name"`        // Название предмета
	Description string  `json:"description"` // Описание предмета
	Weight      float64 `json:"weight"`      // Вес в кг (оценка, если не указано)
	Quantity    int     `json:"quantity"`    // Количество (по умолчанию 1)
	Type        string  `json:"type"`        // Тип предмета: "weapon", "armor", "potion", "tool", "misc", "consumable"
}

// CombatRepository интерфейс для работы с боями
type CombatRepository interface {
	Save(ctx context.Context, c *combat.Combat) error
	GetActiveBySessionID(ctx context.Context, sessionID uint) (*combat.Combat, error)
}

// QuestRepository интерфейс для работы с квестами
type QuestRepository interface {
	GetByWorldID(ctx context.Context, worldID uint) ([]*quest.Quest, error)
	Save(ctx context.Context, q *quest.Quest) error
}

// InventoryRepository интерфейс для работы с инвентарем
// Используется общий интерфейс из player_action пакета через алиас
type InventoryRepository interface {
	GetByCharacterID(ctx context.Context, characterID uint) (*inventory.Inventory, error)
	Save(ctx context.Context, inv *inventory.Inventory) error
}

type AnalyzeDMResponseUseCase struct {
	llm                domain.LLM
	combatRepo         CombatRepository
	questRepo          QuestRepository
	inventoryRepo      InventoryRepository
	sessionID          uint
	chatID             int64  // ChatID для отправки уведомлений
	worldID            uint
	characterID        uint   // ID персонажа игрока
	playerID           uint   // ID игрока (для проверки достижений)
	combatStartMessage string // Сообщение о порядке ходов при начале боя
	checkAchievementsUC AchievementChecker // Опциональная зависимость для проверки достижений
	notificationService NotificationService // Опциональная зависимость для отправки уведомлений
}

// AchievementChecker интерфейс для проверки достижений
type AchievementChecker interface {
	Execute(ctx context.Context, req CheckAchievementsRequest) ([]AchievementUnlocked, error)
}

// CheckAchievementsRequest запрос на проверку достижений
type CheckAchievementsRequest struct {
	PlayerID      uint
	RequirementKey string
	CurrentValue   int
}

// AchievementUnlocked разблокированное достижение
type AchievementUnlocked struct {
	Achievement Achievement
	Message     string
}

// Achievement достижение (для упрощения зависимостей)
type Achievement struct {
	Code        string
	Title       string
	Description string
}

// NotificationService интерфейс для отправки уведомлений о достижениях (из achievement пакета)
type NotificationService interface {
	SendAchievementNotification(ctx context.Context, chatID int64, message string) error
}

func NewAnalyzeDMResponseUseCase(
	llm domain.LLM,
	combatRepo CombatRepository,
	questRepo QuestRepository,
	inventoryRepo InventoryRepository,
	sessionID uint,
	chatID int64, // ChatID для отправки уведомлений
	worldID uint,
	characterID uint,
	playerID uint, // Добавлен playerID для проверки достижений
) *AnalyzeDMResponseUseCase {
	return &AnalyzeDMResponseUseCase{
		llm:           llm,
		combatRepo:    combatRepo,
		questRepo:     questRepo,
		inventoryRepo: inventoryRepo,
		sessionID:     sessionID,
		chatID:        chatID,
		worldID:       worldID,
		characterID:   characterID,
		playerID:      playerID,
	}
}

// SetCheckAchievementsUseCase устанавливает AchievementChecker для проверки достижений
func (uc *AnalyzeDMResponseUseCase) SetCheckAchievementsUseCase(checkAchievementsUC AchievementChecker) {
	uc.checkAchievementsUC = checkAchievementsUC
}

// SetNotificationService устанавливает NotificationService для отправки уведомлений
func (uc *AnalyzeDMResponseUseCase) SetNotificationService(notificationService NotificationService) {
	uc.notificationService = notificationService
}

// Execute анализирует ответ DM и выполняет необходимые действия
func (uc *AnalyzeDMResponseUseCase) Execute(
	ctx context.Context,
	dmResponse string,
) (*DMResponseAnalysis, error) {
	// Анализируем ответ DM с помощью LLM
	analysis, err := uc.analyzeWithLLM(ctx, dmResponse)
	if err != nil {
		log.Printf("Failed to analyze DM response with LLM: %v", err)
		// Возвращаем пустой анализ, но не прерываем выполнение
		return &DMResponseAnalysis{}, nil
	}

	// Обрабатываем обнаруженные события
	if err := uc.processAnalysis(ctx, analysis); err != nil {
		log.Printf("Failed to process DM analysis: %v", err)
		// Логируем ошибку, но не прерываем выполнение
	}

	// Добавляем сообщение о начале боя в анализ (если оно было сгенерировано)
	if uc.combatStartMessage != "" {
		analysis.CombatStartMessage = uc.combatStartMessage
		uc.combatStartMessage = "" // Очищаем для следующего использования
	}

	return analysis, nil
}

// analyzeWithLLM использует LLM для анализа ответа DM
func (uc *AnalyzeDMResponseUseCase) analyzeWithLLM(
	ctx context.Context,
	dmResponse string,
) (*DMResponseAnalysis, error) {
	prompt := buildAnalysisPrompt(dmResponse)

	llmCtx, llmCancel := context.WithTimeout(ctx, 10*time.Second)
	defer llmCancel()

	// Увеличено с 512 до 1024 для предотвращения обрезанного JSON
	raw, err := uc.llm.GenerateWithMaxTokens(llmCtx, prompt, 1024)
	if err != nil {
		return nil, fmt.Errorf("LLM error: %w", err)
	}

	// Очищаем ответ от markdown блоков и лишнего текста
	cleaned := cleanJSONResponse(raw)

	// Логируем оригинальный ответ для анализа проблем
	log.Printf("[DM Analyzer] Raw LLM response (length: %d): %s", len(raw), raw[:min(200, len(raw))])

	// Пытаемся восстановить JSON если он невалиден
	if !json.Valid([]byte(cleaned)) {
		log.Printf("[DM Analyzer] Invalid JSON after initial cleaning, attempting repair...")
		cleaned = tryRepairTruncatedJSON(cleaned)

		if !json.Valid([]byte(cleaned)) {
			// Пробуем более агрессивную очистку
			cleaned = aggressiveJSONClean(cleaned)

			if !json.Valid([]byte(cleaned)) {
				// Логируем проблемный ответ для анализа
				log.Printf("[DM Analyzer] Failed to parse JSON after all repair attempts")
				log.Printf("[DM Analyzer] Cleaned JSON (length: %d): %s", len(cleaned), cleaned)

				// Возвращаем пустой анализ вместо ошибки (fallback механизм)
				// Это позволяет системе продолжать работать даже при проблемах с парсингом
				log.Printf("[DM Analyzer] Returning empty analysis as fallback")
				return &DMResponseAnalysis{}, nil
			}
		}
	}

	var analysis DMResponseAnalysis
	if err := json.Unmarshal([]byte(cleaned), &analysis); err != nil {
		// Логируем ошибку парсинга для анализа
		log.Printf("[DM Analyzer] Failed to unmarshal JSON: %v", err)
		log.Printf("[DM Analyzer] Cleaned JSON that failed to parse: %s", cleaned)

		// Возвращаем пустой анализ вместо ошибки (fallback механизм)
		log.Printf("[DM Analyzer] Returning empty analysis as fallback")
		return &DMResponseAnalysis{}, nil
	}

	return &analysis, nil
}

// min возвращает минимальное из двух чисел
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// processAnalysis обрабатывает результаты анализа
func (uc *AnalyzeDMResponseUseCase) processAnalysis(
	ctx context.Context,
	analysis *DMResponseAnalysis,
) error {
	// Обрабатываем боевую ситуацию
	if analysis.CombatDetected && len(analysis.Enemies) > 0 {
		if err := uc.handleCombatStart(ctx, analysis.Enemies); err != nil {
			return fmt.Errorf("failed to start combat: %w", err)
		}
	}

	// Обрабатываем выполнение квеста
	if analysis.QuestCompleted || analysis.QuestFailed {
		if err := uc.handleQuestStatus(ctx, analysis); err != nil {
			return fmt.Errorf("failed to update quest status: %w", err)
		}
	}

	// Обрабатываем полученные предметы
	if len(analysis.ItemsReceived) > 0 {
		if err := uc.handleItemsReceived(ctx, analysis.ItemsReceived); err != nil {
			return fmt.Errorf("failed to add items to inventory: %w", err)
		}
	}

	return nil
}

// handleCombatStart создает новый бой, если его еще нет
func (uc *AnalyzeDMResponseUseCase) handleCombatStart(
	ctx context.Context,
	enemies []Enemy,
) error {
	// Проверяем, нет ли уже активного боя
	activeCombat, err := uc.combatRepo.GetActiveBySessionID(ctx, uc.sessionID)
	if err != nil {
		return fmt.Errorf("failed to check active combat: %w", err)
	}

	if activeCombat != nil {
		// Бой уже активен, не создаем новый
		return nil
	}

	// Создаем новый бой
	newCombat := &combat.Combat{
		GameSessionID: uc.sessionID,
		State:         combat.CombatStateNotStarted,
		Participants:  make([]combat.CombatParticipant, 0),
		CurrentTurn:   0,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	// Добавляем игрока
	playerParticipant := combat.CombatParticipant{
		IsPlayer:    true,
		CharacterID: &uc.characterID,
		CreatedAt:   time.Now(),
	}
	newCombat.Participants = append(newCombat.Participants, playerParticipant)

	// Добавляем врагов
	for _, enemy := range enemies {
		// Используем значения по умолчанию, если HP или другие параметры не указаны или равны 0
		hp := enemy.HP
		if hp <= 0 {
			hp = 10 // Значение по умолчанию для HP монстра
		}

		ac := enemy.AC
		if ac <= 0 {
			ac = 12 // Значение по умолчанию для AC монстра
		}

		attackBonus := enemy.AttackBonus
		if attackBonus < 0 {
			attackBonus = 2 // Значение по умолчанию для бонуса атаки
		}

		// Пропускаем врагов без имени
		if enemy.Name == "" {
			log.Printf("[DM Analyzer] Skipping enemy without name (HP: %d, AC: %d)", enemy.HP, enemy.AC)
			continue
		}

		enemyParticipant := combat.CombatParticipant{
			IsPlayer:           false,
			MonsterName:        enemy.Name,
			MonsterHP:          hp,
			MonsterMaxHP:       hp,
			MonsterAC:          ac,
			MonsterAttackBonus: attackBonus,
			CreatedAt:          time.Now(),
		}
		newCombat.Participants = append(newCombat.Participants, enemyParticipant)
	}

	// Начинаем бой
	if err := newCombat.Start(); err != nil {
		return fmt.Errorf("failed to start combat: %w", err)
	}

	// Сохраняем бой
	if err := uc.combatRepo.Save(ctx, newCombat); err != nil {
		return fmt.Errorf("failed to save combat: %w", err)
	}

	// Генерируем сообщение о порядке ходов для озвучивания DM
	uc.combatStartMessage = newCombat.GetInitiativeOrderMessage()

	// Проверяем достижения по участию в бою
	if uc.checkAchievementsUC != nil && uc.playerID > 0 {
		// Проверяем достижения по участию в бою (combat_participated)
		achievementReq := CheckAchievementsRequest{
			PlayerID:       uc.playerID,
			RequirementKey: "combat_participated",
			CurrentValue:   1, // Увеличиваем на 1 участие
		}
		
		unlocked, err := uc.checkAchievementsUC.Execute(ctx, achievementReq)
		if err != nil {
			log.Printf("[DM Analyzer] Failed to check achievements after combat start: %v", err)
		} else if len(unlocked) > 0 {
			// Логируем и отправляем уведомления о разблокированных достижениях
			for _, achievement := range unlocked {
				log.Printf("[DM Analyzer] Achievement unlocked after combat start: %s (%s)", 
					achievement.Achievement.Code, achievement.Achievement.Title)
				
				// Отправляем уведомление пользователю, если есть notification service
				if uc.notificationService != nil {
					if err := uc.notificationService.SendAchievementNotification(ctx, uc.chatID, achievement.Message); err != nil {
						log.Printf("[DM Analyzer] Failed to send achievement notification: %v", err)
					}
				}
			}
		}
	}

	return nil
}

// handleQuestStatus обновляет статус квеста
func (uc *AnalyzeDMResponseUseCase) handleQuestStatus(
	ctx context.Context,
	analysis *DMResponseAnalysis,
) error {
	// Получаем все квесты мира
	quests, err := uc.questRepo.GetByWorldID(ctx, uc.worldID)
	if err != nil {
		return fmt.Errorf("failed to get quests: %w", err)
	}

	// Ищем квест по названию или используем главный квест
	var targetQuest *quest.Quest
	if analysis.QuestTitle != "" {
		for _, q := range quests {
			if strings.Contains(strings.ToLower(q.Title), strings.ToLower(analysis.QuestTitle)) {
				targetQuest = q
				break
			}
		}
	}

	// Если не нашли по названию, берем первый активный квест
	if targetQuest == nil {
		for _, q := range quests {
			if q.IsActive() {
				targetQuest = q
				break
			}
		}
	}

	if targetQuest == nil {
		// Нет активных квестов для обновления
		return nil
	}

	// Обновляем статус квеста
	if analysis.QuestCompleted {
		targetQuest.Complete()
	} else if analysis.QuestFailed {
		targetQuest.Fail()
	}

	// Сохраняем изменения
	if err := uc.questRepo.Save(ctx, targetQuest); err != nil {
		return fmt.Errorf("failed to save quest: %w", err)
	}

	// Проверяем достижения по завершению квеста
	if uc.checkAchievementsUC != nil && uc.playerID > 0 && analysis.QuestCompleted {
		// Проверяем достижения по завершению квестов (quests_completed)
		achievementReq := CheckAchievementsRequest{
			PlayerID:       uc.playerID,
			RequirementKey: "quests_completed",
			CurrentValue:   1, // Увеличиваем на 1 завершенный квест
		}
		
		unlocked, err := uc.checkAchievementsUC.Execute(ctx, achievementReq)
		if err != nil {
			log.Printf("[DM Analyzer] Failed to check achievements after quest completion: %v", err)
		} else if len(unlocked) > 0 {
			// Логируем и отправляем уведомления о разблокированных достижениях
			for _, achievement := range unlocked {
				log.Printf("[DM Analyzer] Achievement unlocked after quest completion: %s (%s)", 
					achievement.Achievement.Code, achievement.Achievement.Title)
				
				// Отправляем уведомление пользователю, если есть notification service
				if uc.notificationService != nil {
					if err := uc.notificationService.SendAchievementNotification(ctx, uc.chatID, achievement.Message); err != nil {
						log.Printf("[DM Analyzer] Failed to send achievement notification: %v", err)
					}
				}
			}
		}
	}

	return nil
}

// handleItemsReceived добавляет полученные предметы в инвентарь
func (uc *AnalyzeDMResponseUseCase) handleItemsReceived(
	ctx context.Context,
	items []Item,
) error {
	if uc.inventoryRepo == nil {
		// Если репозиторий не настроен, просто логируем
		log.Printf("Inventory repository not configured, skipping item addition")
		return nil
	}

	// Получаем инвентарь персонажа
	inv, err := uc.inventoryRepo.GetByCharacterID(ctx, uc.characterID)
	if err != nil {
		return fmt.Errorf("failed to get inventory: %w", err)
	}

	// Добавляем каждый предмет в инвентарь
	for _, item := range items {
		// Определяем тип предмета
		itemType := inventory.ItemTypeMisc
		switch strings.ToLower(item.Type) {
		case "weapon":
			itemType = inventory.ItemTypeWeapon
		case "armor":
			itemType = inventory.ItemTypeArmor
		case "potion":
			itemType = inventory.ItemTypePotion
		case "tool":
			itemType = inventory.ItemTypeTool
		case "consumable":
			itemType = inventory.ItemTypeConsumable
		default:
			itemType = inventory.ItemTypeMisc
		}

		// Устанавливаем дефолтные значения
		quantity := item.Quantity
		if quantity <= 0 {
			quantity = 1
		}

		weight := item.Weight
		if weight <= 0 {
			// Дефолтный вес зависит от типа предмета
			weight = 1.0 // по умолчанию 1 кг
			switch itemType {
			case inventory.ItemTypeWeapon:
				weight = 2.0
			case inventory.ItemTypeArmor:
				weight = 5.0
			case inventory.ItemTypePotion:
				weight = 0.5
			case inventory.ItemTypeTool:
				weight = 1.5
			case inventory.ItemTypeConsumable:
				weight = 0.3
			}
		}

		description := item.Description
		if description == "" {
			description = fmt.Sprintf("Предмет типа %s", item.Type)
		}

		// Добавляем предмет в инвентарь
		if err := inv.AddItem(item.Name, description, weight, quantity, itemType); err != nil {
			log.Printf("Failed to add item '%s' to inventory: %v", item.Name, err)
			// Продолжаем добавление остальных предметов даже при ошибке
			continue
		}

		log.Printf("Added item to inventory: %s (x%d, weight: %.2f kg, type: %s)",
			item.Name, quantity, weight, itemType)
	}

	// Сохраняем обновленный инвентарь
	if err := uc.inventoryRepo.Save(ctx, inv); err != nil {
		return fmt.Errorf("failed to save inventory: %w", err)
	}

	return nil
}

// buildAnalysisPrompt создает промпт для анализа ответа DM
func buildAnalysisPrompt(dmResponse string) string {
	return fmt.Sprintf(`Ты анализируешь ответ Dungeon Master в игре D&D и извлекаешь структурированную информацию.

Ответ DM:
"%s"

Проанализируй ответ и верни JSON с следующей структурой:
{
  "combat_detected": true/false,
  "enemies": [
    {
      "name": "имя врага",
      "hp": число,
      "ac": число,
      "attack_bonus": число
    }
  ],
  "quest_completed": true/false,
  "quest_failed": true/false,
  "quest_title": "название",
  "experience_gained": число,
  "experience_reason": "причина",
  "items_received": [
    {
      "name": "название предмета",
      "description": "описание предмета",
      "weight": число,
      "quantity": число,
      "type": "weapon|armor|potion|tool|consumable|misc"
    }
  ]
}

Важно:
- Если бой начался, обязательно укажи хотя бы одного врага
- Если квест выполнен или провален, укажи название квеста
- Опыт начисляется только за значимые достижения (завершение квеста, победа в бою)
- Предметы добавляй только если в ответе DM явно указано, что игрок получил/нашел/поднял предмет (ключевые слова: "получаешь", "находишь", "поднимаешь", "нашел", "берешь", "взял", "дал", "подарил")
- Не добавляй предметы, если они только упоминаются в описании или не были получены игроком
- Если информации недостаточно, используй значения по умолчанию (false, 0, пустые строки, пустые массивы)

КРИТИЧЕСКИ ВАЖНО:
- Верни ТОЛЬКО валидный JSON, без дополнительного текста до или после JSON
- Не добавляй markdown блоки кода
- Не добавляй объяснения или комментарии
- Убедись, что все строки в кавычках, все числа без кавычек, все булевы значения - true/false
- Убедись, что все скобки и массивы закрыты
- ОБЯЗАТЕЛЬНО заверши JSON полностью - все открывающие скобки { должны быть закрыты }, все массивы [ должны быть закрыты ]
- Если не можешь завершить JSON полностью, верни пустой JSON объект {}
- НЕ обрезай JSON в середине структуры - если не хватает места, верни сокращенную но полную структуру

Пример правильного ответа:
{"combat_detected":false,"enemies":[],"quest_completed":false,"quest_failed":false,"quest_title":"","experience_gained":0,"experience_reason":"","items_received":[]}`, dmResponse)
}

// cleanJSONResponse очищает ответ LLM от markdown блоков кода и лишнего текста
func cleanJSONResponse(raw string) string {
	raw = strings.TrimSpace(raw)

	// Удаляем markdown блоки кода
	if strings.HasPrefix(raw, "```json") {
		raw = strings.TrimPrefix(raw, "```json")
		raw = strings.TrimSpace(raw)
	} else if strings.HasPrefix(raw, "```") {
		raw = strings.TrimPrefix(raw, "```")
		raw = strings.TrimSpace(raw)
	}

	// Удаляем закрывающий markdown блок
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	// Удаляем текст до первого { (если есть префиксный текст)
	firstBrace := strings.Index(raw, "{")
	if firstBrace > 0 {
		raw = raw[firstBrace:]
	}

	// Удаляем текст после последнего } (если есть постфиксный текст)
	lastBrace := strings.LastIndex(raw, "}")
	if lastBrace >= 0 && lastBrace < len(raw)-1 {
		raw = raw[:lastBrace+1]
	}

	return strings.TrimSpace(raw)
}

// aggressiveJSONClean применяет более агрессивную очистку JSON
func aggressiveJSONClean(jsonStr string) string {
	jsonStr = strings.TrimSpace(jsonStr)
	if jsonStr == "" {
		return "{}"
	}

	// Удаляем все символы до первого {
	firstBrace := strings.Index(jsonStr, "{")
	if firstBrace > 0 {
		jsonStr = jsonStr[firstBrace:]
	}

	// Удаляем все символы после последнего }
	lastBrace := strings.LastIndex(jsonStr, "}")
	if lastBrace >= 0 && lastBrace < len(jsonStr)-1 {
		jsonStr = jsonStr[:lastBrace+1]
	}

	// Удаляем комментарии (однострочные // и многострочные /* */)
	// Это не стандартный JSON, но иногда LLM добавляет комментарии
	jsonStr = removeJSONComments(jsonStr)

	// Исправляем незакрытые строки и скобки
	jsonStr = tryRepairTruncatedJSON(jsonStr)

	return strings.TrimSpace(jsonStr)
}

// removeJSONComments удаляет комментарии из JSON (не стандартные, но иногда LLM их добавляет)
func removeJSONComments(jsonStr string) string {
	// Удаляем однострочные комментарии // ... \n
	lines := strings.Split(jsonStr, "\n")
	var cleanedLines []string
	for _, line := range lines {
		// Удаляем комментарии после //, но только если это не в строке
		commentIdx := strings.Index(line, "//")
		if commentIdx >= 0 {
			// Проверяем, не в строке ли это (грубая проверка)
			beforeComment := line[:commentIdx]
			quoteCount := strings.Count(beforeComment, "\"")
			if quoteCount%2 == 0 {
				// Четное количество кавычек - не в строке, удаляем комментарий
				line = strings.TrimSpace(line[:commentIdx])
			}
		}
		if strings.TrimSpace(line) != "" {
			cleanedLines = append(cleanedLines, line)
		}
	}
	return strings.Join(cleanedLines, "\n")
}

// tryRepairTruncatedJSON пытается восстановить обрезанный JSON
func tryRepairTruncatedJSON(jsonStr string) string {
	jsonStr = strings.TrimSpace(jsonStr)
	if jsonStr == "" {
		return "{}"
	}

	openBraces := 0
	openBrackets := 0
	inString := false
	escapeNext := false

	for i := 0; i < len(jsonStr); i++ {
		char := jsonStr[i]

		if escapeNext {
			escapeNext = false
			continue
		}

		if char == '\\' {
			escapeNext = true
			continue
		}

		if char == '"' && !escapeNext {
			inString = !inString
			continue
		}

		if inString {
			continue
		}

		switch char {
		case '{':
			openBraces++
		case '}':
			openBraces--
		case '[':
			openBrackets++
		case ']':
			openBrackets--
		}
	}

	result := jsonStr
	if inString {
		result += "\""
	}

	if openBraces > 0 || openBrackets > 0 || inString {
		result = strings.TrimRight(result, " \n\r\t")
		if !inString && strings.HasSuffix(result, ",") {
			result = strings.TrimSuffix(result, ",")
		}
		for i := 0; i < openBraces; i++ {
			result += "}"
		}
		for i := 0; i < openBrackets; i++ {
			result += "]"
		}
		return result
	}

	return jsonStr
}
