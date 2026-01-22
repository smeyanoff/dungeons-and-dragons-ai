package dm_analyzer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"sync/atomic"
	"time"

	jsonrepair "dungeons-and-dragons-ai/internal/game/application/jsonrepair"
	locationeventapp "dungeons-and-dragons-ai/internal/game/application/location_event"
	"dungeons-and-dragons-ai/internal/game/domain/combat"
	"dungeons-and-dragons-ai/internal/game/domain/event"
	"dungeons-and-dragons-ai/internal/game/domain/inventory"
	"dungeons-and-dragons-ai/internal/game/domain/quest"
	"dungeons-and-dragons-ai/internal/game/domain/world"
	"dungeons-and-dragons-ai/internal/llm/domain"
	ragdomain "dungeons-and-dragons-ai/internal/rag/domain"

	"github.com/google/uuid"
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

	// Локации и NPC
	LocationVisited *Location `json:"location_visited,omitempty"` // Локация, которую игрок впервые посетил
	NPCMet          *NPC      `json:"npc_met,omitempty"`          // NPC, с которым игрок впервые встретился

	// Автоматически сгенерированные изображения
	GeneratedImages []GeneratedImage `json:"generated_images,omitempty"` // Пути к автоматически сгенерированным изображениям
}

var invalidAnalyzerJSONCount uint64
var emptyAnalyzerJSONCount uint64

// GeneratedImage представляет автоматически сгенерированное изображение
type GeneratedImage struct {
	Type       string `json:"type"`        // Тип: "item", "location", "npc"
	ImagePath  string `json:"image_path"`  // Путь к изображению
	EntityName string `json:"entity_name"` // Название сущности (предмет, локация, NPC)
}

// Enemy представляет врага в бою
type Enemy struct {
	Name        string `json:"name"`         // Имя врага
	HP          *int   `json:"hp"`           // HP врага (указатель для обработки null)
	AC          *int   `json:"ac"`           // Класс брони (указатель для обработки null)
	AttackBonus *int   `json:"attack_bonus"` // Бонус к атаке (указатель для обработки null)
}

// Item представляет предмет, полученный игроком
type Item struct {
	Name        string  `json:"name"`        // Название предмета
	Description string  `json:"description"` // Описание предмета
	Weight      float64 `json:"weight"`      // Вес в кг (оценка, если не указано)
	Quantity    int     `json:"quantity"`    // Количество (по умолчанию 1)
	Type        string  `json:"type"`        // Тип предмета: "weapon", "armor", "potion", "tool", "misc", "consumable"
}

// Location представляет локацию, которую игрок посетил
type Location struct {
	Name         string `json:"name"`           // Название локации
	Description  string `json:"description"`    // Описание локации
	IsFirstVisit bool   `json:"is_first_visit"` // Первое ли это посещение
}

// NPC представляет NPC, с которым игрок встретился
type NPC struct {
	Name           string `json:"name"`             // Имя NPC
	Description    string `json:"description"`      // Описание NPC
	IsFirstMeeting bool   `json:"is_first_meeting"` // Первая ли это встреча
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
	llm                     domain.LLM
	combatRepo              CombatRepository
	questRepo               QuestRepository
	inventoryRepo           InventoryRepository
	sessionID               uint
	chatID                  int64 // ChatID для отправки уведомлений
	worldID                 uint
	characterID             uint                      // ID персонажа игрока
	playerID                uint                      // ID игрока (для проверки достижений)
	combatStartMessage      string                    // Сообщение о порядке ходов при начале боя
	checkAchievementsUC     AchievementChecker        // Опциональная зависимость для проверки достижений
	notificationService     NotificationService       // Опциональная зависимость для отправки уведомлений
	imageGenerationService  ImageGenerationService    // Опциональная зависимость для автоматической генерации изображений
	userID                  int64                     // ID пользователя для генерации изображений (Telegram User ID)
	checkDailyProgressUC    DailyQuestProgressChecker // Опциональная зависимость для отслеживания ежедневных заданий
	tgUserID                int64                     // Telegram User ID для отслеживания ежедневных заданий
	generateLocationEventUC LocationEventGenerator    // Опциональная зависимость для генерации событий локаций
	sessionRepo             SessionRepository         // Репозиторий сессий для поиска локаций
	eventRepo               StoryEventRepository      // Репозиторий для записи событий истории
	indexDocUC              RAGIndexer                // Индексатор RAG для событий
}

// SessionRepository интерфейс для доступа к сессии (для поиска локаций)
type SessionRepository interface {
	GetByChatID(ctx context.Context, chatID int64) (*SessionSnapshot, error)
}

// SessionSnapshot — упрощенная структура сессии (для доступа к миру и локациям)
type SessionSnapshot struct {
	ID    uint
	World WorldSnapshot
}

// WorldSnapshot — упрощенная структура мира (для доступа к локациям)
type WorldSnapshot struct {
	ID        uint
	Locations []LocationSnapshot
}

// LocationSnapshot — упрощенная структура локации (для поиска по имени)
type LocationSnapshot struct {
	ID   uint
	Name string
}

// AchievementChecker интерфейс для проверки достижений
type AchievementChecker interface {
	Execute(ctx context.Context, req CheckAchievementsRequest) ([]AchievementUnlocked, error)
}

// CheckAchievementsRequest запрос на проверку достижений
type CheckAchievementsRequest struct {
	PlayerID       uint
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

// DailyQuestProgressChecker интерфейс для отслеживания прогресса ежедневных заданий
type DailyQuestProgressChecker interface {
	Execute(ctx context.Context, req DailyQuestProgressRequest) error
}

// DailyQuestProgressRequest запрос на проверку прогресса ежедневного задания
type DailyQuestProgressRequest struct {
	ChatID    int64
	TgUserID  int64
	QuestType string // Тип задания: "complete_quest", "win_combat", "explore_location"
	Increment int    // На сколько увеличить прогресс
}

// ImageGenerationService интерфейс для автоматической генерации изображений
type ImageGenerationService interface {
	GenerateImage(ctx context.Context, req GenerateImageRequest) (*GenerateImageResponse, error)
}

// GenerateImageRequest запрос на генерацию изображения
type GenerateImageRequest struct {
	SystemPrompt    string
	UserPrompt      string
	Type            string // "location", "npc", "item", "character", "custom"
	EntityID        uint
	ForceRegenerate bool
	UserID          int64
	SkipLimitCheck  bool
}

// GenerateImageResponse ответ на запрос генерации изображения
type GenerateImageResponse struct {
	ImagePath string
	FileID    string
	FromCache bool
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

// SetImageGenerationService устанавливает ImageGenerationService для автоматической генерации изображений
func (uc *AnalyzeDMResponseUseCase) SetImageGenerationService(imageService ImageGenerationService, userID int64) {
	uc.imageGenerationService = imageService
	uc.userID = userID
}

func (uc *AnalyzeDMResponseUseCase) SetCheckDailyProgress(checkDailyProgressUC DailyQuestProgressChecker, tgUserID int64) {
	uc.checkDailyProgressUC = checkDailyProgressUC
	uc.tgUserID = tgUserID
}

// LocationEventGenerator интерфейс для генерации событий локаций
type LocationEventGenerator interface {
	Execute(ctx context.Context, req locationeventapp.GenerateLocationEventRequest) (*locationeventapp.GenerateLocationEventResponse, error)
}

// StoryEventRepository интерфейс для записи истории игры
type StoryEventRepository interface {
	Save(ctx context.Context, e *event.StoryEvent) error
}

// RAGIndexer интерфейс для индексации событий в RAG
type RAGIndexer interface {
	Execute(ctx context.Context, doc ragdomain.Document) error
}

// SetLocationEventGenerator устанавливает генератор событий локаций
func (uc *AnalyzeDMResponseUseCase) SetLocationEventGenerator(generator LocationEventGenerator) {
	uc.generateLocationEventUC = generator
}

// SetSessionRepository устанавливает репозиторий сессий для поиска локаций
func (uc *AnalyzeDMResponseUseCase) SetSessionRepository(sessionRepo SessionRepository) {
	uc.sessionRepo = sessionRepo
}

// SetStoryEventRepository устанавливает репозиторий событий для записи истории
func (uc *AnalyzeDMResponseUseCase) SetStoryEventRepository(eventRepo StoryEventRepository) {
	uc.eventRepo = eventRepo
}

// SetRAGIndexer устанавливает индексатор RAG для событий
func (uc *AnalyzeDMResponseUseCase) SetRAGIndexer(indexer RAGIndexer) {
	uc.indexDocUC = indexer
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

// validateCombatAnalysis проверяет корректность анализа боя с строгими проверками
func validateCombatAnalysis(analysis *DMResponseAnalysis) (*DMResponseAnalysis, error) {
	// Строгая валидация всех полей анализа
	return validateAnalysisStrict(analysis)
}

// validateAnalysisStrict выполняет полную валидацию анализа с fallback логикой
func validateAnalysisStrict(analysis *DMResponseAnalysis) (*DMResponseAnalysis, error) {
	if analysis == nil {
		log.Printf("[DM Analyzer] Analysis is nil, returning empty analysis")
		return &DMResponseAnalysis{}, nil
	}

	// Валидируем боевую ситуацию
	if analysis.CombatDetected {
		validEnemies, hasValidEnemies := validateEnemiesStrict(analysis.Enemies)
		if !hasValidEnemies {
			log.Printf("[DM Analyzer] Combat detected but no valid enemies found, disabling combat")
			analysis.CombatDetected = false
			analysis.Enemies = nil
		} else {
			analysis.Enemies = validEnemies
		}
	}

	// Валидируем квесты
	if analysis.QuestCompleted || analysis.QuestFailed {
		if analysis.QuestTitle == "" {
			log.Printf("[DM Analyzer] Quest status changed but title is empty, clearing quest flags")
			analysis.QuestCompleted = false
			analysis.QuestFailed = false
		}
	}

	// Валидируем опыт
	if analysis.ExperienceGained < 0 {
		log.Printf("[DM Analyzer] Experience gained is negative (%d), setting to 0", analysis.ExperienceGained)
		analysis.ExperienceGained = 0
		analysis.ExperienceReason = ""
	}
	if analysis.ExperienceGained > 0 && analysis.ExperienceReason == "" {
		log.Printf("[DM Analyzer] Experience gained (%d) but no reason provided, setting default", analysis.ExperienceGained)
		analysis.ExperienceReason = "за достижения в игре"
	}

	// Валидируем предметы
	validItems := make([]Item, 0, len(analysis.ItemsReceived))
	for i, item := range analysis.ItemsReceived {
		if item.Name == "" || strings.TrimSpace(item.Name) == "" {
			log.Printf("[DM Analyzer] Item %d has empty name, skipping", i)
			continue
		}
		if item.Quantity <= 0 {
			item.Quantity = 1
		}
		if item.Weight < 0 {
			item.Weight = 0.1 // минимальный вес
		}
		if item.Type == "" {
			item.Type = "misc"
		}
		validItems = append(validItems, item)
	}
	analysis.ItemsReceived = validItems

	// Валидируем локации
	if analysis.LocationVisited != nil {
		if analysis.LocationVisited.Name == "" {
			log.Printf("[DM Analyzer] Location visited but name is empty, clearing location")
			analysis.LocationVisited = nil
		}
	}

	// Валидируем NPC
	if analysis.NPCMet != nil {
		if analysis.NPCMet.Name == "" {
			log.Printf("[DM Analyzer] NPC met but name is empty, clearing NPC")
			analysis.NPCMet = nil
		}
	}

	return analysis, nil
}

// validateEnemiesStrict выполняет строгую валидацию врагов
func validateEnemiesStrict(enemies []Enemy) ([]Enemy, bool) {
	if len(enemies) == 0 {
		return nil, false
	}

	validEnemies := make([]Enemy, 0, len(enemies))
	for i, enemy := range enemies {
		// Строгая проверка имени
		if enemy.Name == "" || strings.TrimSpace(enemy.Name) == "" {
			log.Printf("[DM Analyzer] Enemy %d has empty name, skipping", i)
			continue
		}

		// Проверяем на слишком длинное имя (защита от мусора)
		if len(enemy.Name) > 100 {
			log.Printf("[DM Analyzer] Enemy %d name too long (%d chars), truncating", i, len(enemy.Name))
			enemy.Name = enemy.Name[:100]
		}

		// Строгая валидация HP
		if enemy.HP == nil {
			defaultHP := 15 // более реалистичное значение по умолчанию
			log.Printf("[DM Analyzer] Enemy '%s' has null HP, setting default %d", enemy.Name, defaultHP)
			enemy.HP = &defaultHP
		} else if *enemy.HP <= 0 {
			defaultHP := 15
			log.Printf("[DM Analyzer] Enemy '%s' has invalid HP (%d), setting default %d", enemy.Name, *enemy.HP, defaultHP)
			enemy.HP = &defaultHP
		} else if *enemy.HP > 1000 { // защита от нереалистичных значений
			maxHP := 500
			log.Printf("[DM Analyzer] Enemy '%s' has unrealistic HP (%d), capping to %d", enemy.Name, *enemy.HP, maxHP)
			enemy.HP = &maxHP
		}

		// Строгая валидация AC
		if enemy.AC == nil {
			defaultAC := 13 // более реалистичное значение по умолчанию
			log.Printf("[DM Analyzer] Enemy '%s' has null AC, setting default %d", enemy.Name, defaultAC)
			enemy.AC = &defaultAC
		} else if *enemy.AC <= 0 {
			defaultAC := 13
			log.Printf("[DM Analyzer] Enemy '%s' has invalid AC (%d), setting default %d", enemy.Name, *enemy.AC, defaultAC)
			enemy.AC = &defaultAC
		} else if *enemy.AC > 30 { // защита от нереалистичных значений
			maxAC := 25
			log.Printf("[DM Analyzer] Enemy '%s' has unrealistic AC (%d), capping to %d", enemy.Name, *enemy.AC, maxAC)
			enemy.AC = &maxAC
		}

		// Строгая валидация attack_bonus
		if enemy.AttackBonus == nil {
			defaultBonus := 3 // более реалистичное значение по умолчанию
			log.Printf("[DM Analyzer] Enemy '%s' has null attack_bonus, setting default %d", enemy.Name, defaultBonus)
			enemy.AttackBonus = &defaultBonus
		} else if *enemy.AttackBonus < -5 || *enemy.AttackBonus > 20 { // защита от нереалистичных значений
			defaultBonus := 3
			log.Printf("[DM Analyzer] Enemy '%s' has unrealistic attack_bonus (%d), setting default %d", enemy.Name, *enemy.AttackBonus, defaultBonus)
			enemy.AttackBonus = &defaultBonus
		}

		validEnemies = append(validEnemies, enemy)
	}

	return validEnemies, len(validEnemies) > 0
}

// validateJSONSchema проверяет JSON на соответствие ожидаемой схеме анализа DM
func validateJSONSchema(jsonStr string) error {
	// Проверяем наличие обязательных полей верхнего уровня
	requiredFields := []string{
		"combat_detected",
		"enemies",
		"quest_completed",
		"quest_failed",
		"quest_title",
		"experience_gained",
		"experience_reason",
		"items_received",
	}

	for _, field := range requiredFields {
		fieldPattern := fmt.Sprintf(`"%s":`, field)
		if !strings.Contains(jsonStr, fieldPattern) {
			return fmt.Errorf("missing required field: %s", field)
		}
	}

	// Проверяем структуру enemies массива
	if strings.Contains(jsonStr, `"enemies":`) {
		enemiesStart := strings.Index(jsonStr, `"enemies":`)
		if enemiesStart >= 0 {
			// Ищем начало массива enemies
			bracketStart := strings.Index(jsonStr[enemiesStart:], "[")
			bracketEnd := strings.Index(jsonStr[enemiesStart:], "]")
			if bracketStart >= 0 && bracketEnd > bracketStart {
				enemiesContent := jsonStr[enemiesStart+bracketStart : enemiesStart+bracketEnd+1]
				// Проверяем, что enemies это массив
				if !strings.HasPrefix(strings.TrimSpace(enemiesContent), "[") {
					return fmt.Errorf("enemies field is not an array")
				}
			}
		}
	}

	// Проверяем структуру items_received массива
	if strings.Contains(jsonStr, `"items_received":`) {
		itemsStart := strings.Index(jsonStr, `"items_received":`)
		if itemsStart >= 0 {
			bracketStart := strings.Index(jsonStr[itemsStart:], "[")
			bracketEnd := strings.Index(jsonStr[itemsStart:], "]")
			if bracketStart >= 0 && bracketEnd > bracketStart {
				itemsContent := jsonStr[itemsStart+bracketStart : itemsStart+bracketEnd+1]
				if !strings.HasPrefix(strings.TrimSpace(itemsContent), "[") {
					return fmt.Errorf("items_received field is not an array")
				}
			}
		}
	}

	// Проверяем на потенциально проблемные значения
	if strings.Contains(jsonStr, `"combat_detected": true`) {
		// Если бой обнаружен, проверяем наличие enemies
		if !strings.Contains(jsonStr, `"enemies": [`) || strings.Contains(jsonStr, `"enemies": []`) {
			log.Printf("[DM Analyzer] Warning: combat_detected=true but enemies array is empty or missing")
		}
	}

	return nil
}

// analyzeWithLLM использует LLM для анализа ответа DM
func (uc *AnalyzeDMResponseUseCase) analyzeWithLLM(
	ctx context.Context,
	dmResponse string,
) (*DMResponseAnalysis, error) {
	const maxRetries = 5 // Увеличено для лучшего восстановления truncated JSON
	return uc.analyzeWithLLMWithRetry(ctx, dmResponse, 0, maxRetries)
}

// analyzeWithLLMWithRetry использует LLM для анализа ответа DM с retry механизмом
func (uc *AnalyzeDMResponseUseCase) analyzeWithLLMWithRetry(
	ctx context.Context,
	dmResponse string,
	attempt int,
	maxRetries int,
) (*DMResponseAnalysis, error) {
	// Валидируем входные данные перед отправкой в LLM
	if err := validateDMResponseInput(dmResponse); err != nil {
		log.Printf("[DM Analyzer] Invalid DM response input: %v, returning fallback analysis", err)
		return fallbackAnalysisFromResponse(dmResponse), nil
	}

	prompt := buildAnalysisPrompt(dmResponse, attempt > 0)

	// Увеличено до 30 секунд, так как без ограничения токенов ответ может быть длиннее
	llmCtx, llmCancel := context.WithTimeout(ctx, 60*time.Second) // Увеличиваем таймаут для анализа
	defer llmCancel()

	// Убрано ограничение на токены для анализа ответа DM и генерации противников
	raw, err := uc.llm.Generate(llmCtx, prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM error: %w", err)
	}

	// Очищаем ответ от markdown блоков и лишнего текста
	cleaned := cleanJSONResponse(raw)

	// Логируем оригинальный ответ для анализа проблем
	log.Printf("[DM Analyzer] Raw LLM response (length: %d, attempt: %d): %s", len(raw), attempt+1, raw[:min(200, len(raw))])

	// Проверяем на truncated JSON и пытаемся восстановить
	isTruncated := looksLikeTruncatedJSON(cleaned)
	if isTruncated {
		log.Printf("[DM Analyzer] Detected truncated JSON response (attempt: %d), attempting repair...", attempt+1)
		cleaned = tryRepairTruncatedJSON(cleaned)
	}

	// Пытаемся восстановить JSON если он невалиден
	if !json.Valid([]byte(cleaned)) {
		log.Printf("[DM Analyzer] Invalid JSON after initial cleaning, attempting repair (attempt: %d)...", attempt+1)
		cleaned = tryRepairTruncatedJSON(cleaned)

		if !json.Valid([]byte(cleaned)) {
			// Пробуем более агрессивную очистку
			cleaned = aggressiveJSONClean(cleaned)

			if !json.Valid([]byte(cleaned)) {
				recordAnalyzerJSONFailure("invalid_json")
				// Логируем проблемный ответ для анализа
				log.Printf("[DM Analyzer] Failed to parse JSON after all repair attempts (attempt: %d)", attempt+1)
				log.Printf("[DM Analyzer] Cleaned JSON (length: %d): %s", len(cleaned), cleaned)

				// Если это не последняя попытка, повторяем запрос
				if attempt < maxRetries {
					log.Printf("[DM Analyzer] Retrying LLM request (attempt %d/%d)", attempt+2, maxRetries+1)
					return uc.analyzeWithLLMWithRetry(ctx, dmResponse, attempt+1, maxRetries)
				}

				// Возвращаем детерминированный fallback вместо ошибки
				log.Printf("[DM Analyzer] Returning fallback analysis after %d attempts", attempt+1)
				return fallbackAnalysisFromResponse(dmResponse), nil
			}
		}
	}

	// Предварительная валидация JSON схемы перед парсингом
	if err := validateJSONSchema(cleaned); err != nil {
		recordAnalyzerJSONFailure("invalid_schema")
		log.Printf("[DM Analyzer] JSON schema validation failed: %v (attempt: %d)", err, attempt+1)
		log.Printf("[DM Analyzer] Invalid JSON: %s", cleaned)
		// Если это не последняя попытка, повторяем запрос
		if attempt < maxRetries {
			log.Printf("[DM Analyzer] Retrying LLM request due to schema validation error (attempt %d/%d)", attempt+2, maxRetries+1)
			return uc.analyzeWithLLMWithRetry(ctx, dmResponse, attempt+1, maxRetries)
		}
		// Возвращаем детерминированный fallback вместо ошибки
		log.Printf("[DM Analyzer] Returning fallback analysis after schema validation failures")
		return fallbackAnalysisFromResponse(dmResponse), nil
	}

	analysis, err := decodeStrictAnalysis(cleaned)
	if err != nil {
		recordAnalyzerJSONFailure("invalid_schema")
		// Логируем ошибку парсинга для анализа
		log.Printf("[DM Analyzer] Failed to unmarshal JSON strictly: %v (attempt: %d)", err, attempt+1)
		log.Printf("[DM Analyzer] Cleaned JSON that failed to parse: %s", cleaned)

		// Если это не последняя попытка, повторяем запрос
		if attempt < maxRetries {
			log.Printf("[DM Analyzer] Retrying LLM request due to strict unmarshal error (attempt %d/%d)", attempt+2, maxRetries+1)
			return uc.analyzeWithLLMWithRetry(ctx, dmResponse, attempt+1, maxRetries)
		}

		// Возвращаем детерминированный fallback вместо ошибки
		log.Printf("[DM Analyzer] Returning fallback analysis after %d attempts", attempt+1)
		return fallbackAnalysisFromResponse(dmResponse), nil
	}

	if isEmptyAnalysis(analysis) {
		recordAnalyzerJSONFailure("empty_json")
		log.Printf("[DM Analyzer] Empty analysis detected (attempt: %d)", attempt+1)
		if attempt < maxRetries {
			log.Printf("[DM Analyzer] Retrying LLM request due to empty analysis (attempt %d/%d)", attempt+2, maxRetries+1)
			return uc.analyzeWithLLMWithRetry(ctx, dmResponse, attempt+1, maxRetries)
		}
		log.Printf("[DM Analyzer] Returning fallback analysis after %d attempts (empty analysis)", attempt+1)
		return fallbackAnalysisFromResponse(dmResponse), nil
	}

	// Валидируем анализ боя
	analysis, err = validateCombatAnalysis(analysis)
	if err != nil {
		log.Printf("[DM Analyzer] Combat analysis validation failed: %v, returning fallback", err)
		return fallbackAnalysisFromResponse(dmResponse), nil
	}

	return analysis, nil
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
	if isEmptyAnalysis(analysis) {
		log.Printf("[DM Analyzer] Empty analysis, skipping state changes")
		return nil
	}
	// Обрабатываем боевую ситуацию
	// ВАЖНО: Проверяем, нет ли уже активного боя перед обработкой врагов
	// Это предотвращает повторную генерацию врагов при каждом анализе ответа DM
	if analysis.CombatDetected && len(analysis.Enemies) > 0 {
		// Проверяем, нет ли уже активного боя
		activeCombat, err := uc.combatRepo.GetActiveBySessionID(ctx, uc.sessionID)
		if err != nil {
			log.Printf("[DM Analyzer] Failed to check active combat: %v", err)
			// Продолжаем обработку, так как это не критично
		} else if activeCombat != nil {
			// Бой уже активен, не обрабатываем врагов из анализа
			// Это предотвращает повторную генерацию врагов с новыми HP
			log.Printf("[DM Analyzer] Combat already active, ignoring enemies from analysis (session_id: %d)", uc.sessionID)
			// Сбрасываем флаг combat_detected, чтобы не обрабатывать врагов
			analysis.CombatDetected = false
			analysis.Enemies = nil
		} else {
			// Боя нет, создаем новый
			if err := uc.handleCombatStart(ctx, analysis.Enemies); err != nil {
				return fmt.Errorf("failed to start combat: %w", err)
			}
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

		// Автоматически генерируем изображения для полученных предметов
		if uc.imageGenerationService != nil && uc.userID > 0 {
			generatedImages := uc.generateImagesForItems(ctx, analysis.ItemsReceived)
			// Добавляем пути к изображениям в анализ для последующей отправки
			analysis.GeneratedImages = append(analysis.GeneratedImages, generatedImages...)
		}
	}

	// Обрабатываем посещение новой локации
	if analysis.LocationVisited != nil && analysis.LocationVisited.IsFirstVisit {
		// Автоматически генерируем событие локации при первом посещении
		if uc.generateLocationEventUC != nil && uc.sessionRepo != nil {
			// Ищем локацию по имени в мире
			locationID := uc.findLocationIDByName(ctx, analysis.LocationVisited.Name)
			if locationID > 0 {
				locationReq := locationeventapp.GenerateLocationEventRequest{
					WorldID:      uc.worldID,
					LocationID:   locationID,
					LocationName: analysis.LocationVisited.Name,
					IsFirstVisit: true,
				}
				eventResp, err := uc.generateLocationEventUC.Execute(ctx, locationReq)
				if err != nil {
					log.Printf("[DM Analyzer] Failed to generate location event: %v", err)
				} else if eventResp != nil && eventResp.Description != "" {
					log.Printf("[DM Analyzer] Location event generated: %s", eventResp.Description)
					// Событие сохранено в БД, дополнительно фиксируем в истории + RAG
					if err := uc.recordLocationEvent(ctx, eventResp); err != nil {
						log.Printf("[DM Analyzer] Failed to record location event in history/RAG: %v", err)
					}
				}
			}
		}

		// Автоматически генерируем изображение для локации
		if uc.imageGenerationService != nil && uc.userID > 0 {
			generatedImage := uc.generateImageForLocation(ctx, *analysis.LocationVisited)
			if generatedImage != nil {
				analysis.GeneratedImages = append(analysis.GeneratedImages, *generatedImage)
			}
		}

		// Отслеживаем прогресс ежедневных заданий при исследовании локации
		if uc.checkDailyProgressUC != nil && uc.tgUserID > 0 {
			dailyReq := DailyQuestProgressRequest{
				ChatID:    uc.chatID,
				TgUserID:  uc.tgUserID,
				QuestType: "explore_location",
				Increment: 1,
			}
			if err := uc.checkDailyProgressUC.Execute(ctx, dailyReq); err != nil {
				log.Printf("[DM Analyzer] Failed to check daily quest progress after location exploration: %v", err)
			}
		}
	}

	// Обрабатываем встречу с NPC
	if analysis.NPCMet != nil && analysis.NPCMet.IsFirstMeeting {
		// Автоматически генерируем изображение для NPC
		if uc.imageGenerationService != nil && uc.userID > 0 {
			generatedImage := uc.generateImageForNPC(ctx, *analysis.NPCMet)
			if generatedImage != nil {
				analysis.GeneratedImages = append(analysis.GeneratedImages, *generatedImage)
			}
		}
	}

	return nil
}

// generateImagesForItems автоматически генерирует изображения для полученных предметов
// Возвращает пути к сгенерированным изображениям (синхронно, но с таймаутом)
func (uc *AnalyzeDMResponseUseCase) generateImagesForItems(
	ctx context.Context,
	items []Item,
) []GeneratedImage {
	if uc.imageGenerationService == nil || uc.userID == 0 {
		return nil
	}

	// Создаем контекст с таймаутом для генерации изображений (увеличено до 180 секунд для обработки rate limiting)
	imgCtx, cancel := context.WithTimeout(ctx, 300*time.Second)
	defer cancel()

	var generatedImages []GeneratedImage

	for _, item := range items {
		// Формируем промпт для генерации изображения предмета
		systemPrompt := "Ты — талантливый художник в стиле фэнтези и Dungeons & Dragons. Создавай детализированные изображения предметов и артефактов в стиле классического фэнтези-арта."

		userPrompt := item.Name
		if item.Description != "" {
			// Используем описание предмета для более детального изображения
			userPrompt = fmt.Sprintf("%s, %s", item.Name, item.Description)
		}

		// Генерируем изображение (синхронно, но с таймаутом)
		req := GenerateImageRequest{
			SystemPrompt:    systemPrompt,
			UserPrompt:      userPrompt,
			Type:            "item",
			EntityID:        0, // Пока нет привязки к ID предмета в БД
			ForceRegenerate: false,
			UserID:          uc.userID,
			SkipLimitCheck:  false, // Проверяем лимиты
		}

		resp, err := uc.imageGenerationService.GenerateImage(imgCtx, req)
		if err != nil {
			// Graceful fallback: добавляем текстовое описание вместо изображения
			log.Printf("Failed to auto-generate image for item '%s', using text fallback: %v", item.Name, err)
			generatedImages = append(generatedImages, GeneratedImage{
				Type:       "item",
				ImagePath:  "", // Пустой путь означает текстовое описание
				EntityName: item.Name,
			})
			// Продолжаем генерацию для остальных предметов
			continue
		}

		log.Printf("Auto-generated image for item: %s (path: %s)", item.Name, resp.ImagePath)

		// Добавляем путь к изображению в список
		generatedImages = append(generatedImages, GeneratedImage{
			Type:       "item",
			ImagePath:  resp.ImagePath,
			EntityName: item.Name,
		})
	}

	return generatedImages
}

// generateImageForLocation автоматически генерирует изображение для локации
func (uc *AnalyzeDMResponseUseCase) generateImageForLocation(
	ctx context.Context,
	location Location,
) *GeneratedImage {
	if uc.imageGenerationService == nil || uc.userID == 0 {
		return nil
	}

	// Создаем контекст с таймаутом для генерации изображений (увеличено до 180 секунд для обработки rate limiting)
	imgCtx, cancel := context.WithTimeout(ctx, 300*time.Second)
	defer cancel()

	// Формируем промпт для генерации изображения локации
	systemPrompt := "Ты — талантливый художник в стиле фэнтези и Dungeons & Dragons. Создавай детализированные, атмосферные изображения локаций и окружающей среды в стиле классического фэнтези-арта."

	userPrompt := location.Name
	if location.Description != "" {
		userPrompt = fmt.Sprintf("%s, %s", location.Name, location.Description)
	}

	// Генерируем изображение
	req := GenerateImageRequest{
		SystemPrompt:    systemPrompt,
		UserPrompt:      userPrompt,
		Type:            "location",
		EntityID:        0, // Пока нет привязки к ID локации в БД
		ForceRegenerate: false,
		UserID:          uc.userID,
		SkipLimitCheck:  false, // Проверяем лимиты
	}

	resp, err := uc.imageGenerationService.GenerateImage(imgCtx, req)
	if err != nil {
		// Graceful fallback: не возвращаем изображение, но логируем
		errStr := err.Error()
		if strings.Contains(errStr, "429") || strings.Contains(errStr, "Too Many Requests") || strings.Contains(errStr, "rate limited") {
			log.Printf("Rate limited during image generation for location '%s', skipping auto-generation: %v", location.Name, err)
		} else {
			log.Printf("Failed to auto-generate image for location '%s', using text fallback: %v", location.Name, err)
		}
		return nil
	}

	log.Printf("Auto-generated image for location: %s (path: %s)", location.Name, resp.ImagePath)

	return &GeneratedImage{
		Type:       "location",
		ImagePath:  resp.ImagePath,
		EntityName: location.Name,
	}
}

// generateImageForNPC автоматически генерирует изображение для NPC
func (uc *AnalyzeDMResponseUseCase) generateImageForNPC(
	ctx context.Context,
	npc NPC,
) *GeneratedImage {
	if uc.imageGenerationService == nil || uc.userID == 0 {
		return nil
	}

	// Создаем контекст с таймаутом для генерации изображений (увеличено до 180 секунд для обработки rate limiting)
	imgCtx, cancel := context.WithTimeout(ctx, 300*time.Second)
	defer cancel()

	// Формируем промпт для генерации изображения NPC
	systemPrompt := "Ты — талантливый художник в стиле фэнтези и Dungeons & Dragons. Создавай детализированные изображения персонажей и NPC в стиле классического фэнтези-арта."

	userPrompt := npc.Name
	if npc.Description != "" {
		userPrompt = fmt.Sprintf("%s, %s", npc.Name, npc.Description)
	}

	// Генерируем изображение
	req := GenerateImageRequest{
		SystemPrompt:    systemPrompt,
		UserPrompt:      userPrompt,
		Type:            "npc",
		EntityID:        0, // Пока нет привязки к ID NPC в БД
		ForceRegenerate: false,
		UserID:          uc.userID,
		SkipLimitCheck:  false, // Проверяем лимиты
	}

	resp, err := uc.imageGenerationService.GenerateImage(imgCtx, req)
	if err != nil {
		// Graceful fallback: не возвращаем изображение, но логируем
		errStr := err.Error()
		if strings.Contains(errStr, "429") || strings.Contains(errStr, "Too Many Requests") || strings.Contains(errStr, "rate limited") {
			log.Printf("Rate limited during image generation for NPC '%s', skipping auto-generation: %v", npc.Name, err)
		} else {
			log.Printf("Failed to auto-generate image for NPC '%s', using text fallback: %v", npc.Name, err)
		}
		return nil
	}

	log.Printf("Auto-generated image for NPC: %s (path: %s)", npc.Name, resp.ImagePath)

	return &GeneratedImage{
		Type:       "npc",
		ImagePath:  resp.ImagePath,
		EntityName: npc.Name,
	}
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
		// Используем значения по умолчанию, если HP или другие параметры не указаны (null) или равны 0
		hp := 10 // Значение по умолчанию для HP монстра
		if enemy.HP != nil && *enemy.HP > 0 {
			hp = *enemy.HP
		}

		ac := 12 // Значение по умолчанию для AC монстра
		if enemy.AC != nil && *enemy.AC > 0 {
			ac = *enemy.AC
		}

		attackBonus := 2 // Значение по умолчанию для бонуса атаки
		if enemy.AttackBonus != nil && *enemy.AttackBonus > 0 {
			attackBonus = *enemy.AttackBonus
		}

		// Пропускаем врагов без имени
		if enemy.Name == "" {
			hpVal := 0
			acVal := 0
			if enemy.HP != nil {
				hpVal = *enemy.HP
			}
			if enemy.AC != nil {
				acVal = *enemy.AC
			}
			log.Printf("[DM Analyzer] Skipping enemy without name (HP: %d, AC: %d)", hpVal, acVal)
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

	// Отслеживаем прогресс ежедневных заданий при завершении квеста
	if uc.checkDailyProgressUC != nil && uc.tgUserID > 0 && analysis.QuestCompleted {
		dailyReq := DailyQuestProgressRequest{
			ChatID:    uc.chatID,
			TgUserID:  uc.tgUserID,
			QuestType: "complete_quest",
			Increment: 1,
		}
		if err := uc.checkDailyProgressUC.Execute(ctx, dailyReq); err != nil {
			log.Printf("[DM Analyzer] Failed to check daily quest progress after quest completion: %v", err)
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
func buildAnalysisPrompt(dmResponse string, strict bool) string {
	skeleton := `{"combat_detected":false,"enemies":[],"quest_completed":false,"quest_failed":false,"quest_title":"","experience_gained":0,"experience_reason":"","items_received":[],"location_visited":null,"npc_met":null,"generated_images":[]}`
	criticalFooter := ""
	if strict {
		criticalFooter = "\n\nСТРОГОЕ ПРАВИЛО ДЛЯ РЕТРАЯ:\n- НЕ возвращай пустой JSON {} или пустой ответ\n- Если нет событий, верни этот скелет без изменений: " + skeleton
	}

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
  ],
  "location_visited": {
    "name": "название локации",
    "description": "описание локации",
    "is_first_visit": true/false
  },
  "npc_met": {
    "name": "имя NPC",
    "description": "описание NPC",
    "is_first_meeting": true/false
  }
}

Важно:
- Если бой начался, обязательно укажи хотя бы одного врага
- Для каждого врага ОБЯЗАТЕЛЬНО укажи hp, ac и attack_bonus (числовые значения, не null!)
- Если в ответе DM не указаны характеристики врага, используй разумные значения по умолчанию:
  * hp: 10-30 для обычных врагов, 30-60 для сильных, 60+ для боссов
  * ac: 12-15 для обычных врагов, 15-18 для сильных, 18+ для боссов
  * attack_bonus: 2-4 для обычных врагов, 4-6 для сильных, 6+ для боссов
- Если квест выполнен или провален, укажи название квеста
- Опыт начисляется только за значимые достижения (завершение квеста, победа в бою)
- Предметы добавляй только если в ответе DM явно указано, что игрок получил/нашел/поднял предмет (ключевые слова: "получаешь", "находишь", "поднимаешь", "нашел", "берешь", "взял", "дал", "подарил")
- Не добавляй предметы, если они только упоминаются в описании или не были получены игроком
- location_visited указывай только если DM описывает новую локацию, которую игрок впервые посещает (ключевые слова: "входишь", "приходишь", "оказываешься", "достигаешь", "перед тобой", "новое место")
- npc_met указывай только если DM описывает встречу с новым NPC (ключевые слова: "встречаешь", "видишь", "подходит", "появляется", "знакомишься")
- Если информации недостаточно, используй значения по умолчанию (false, 0, пустые строки, пустые массивы, null)
- ВСЕГДА возвращай полный JSON со всеми полями (даже если все значения по умолчанию)

КРИТИЧЕСКИ ВАЖНО:
- Верни ТОЛЬКО валидный JSON, без дополнительного текста до или после JSON
- Не добавляй markdown блоки кода
- Не добавляй объяснения или комментарии
- Убедись, что все строки в кавычках, все числа без кавычек, все булевы значения - true/false
- Убедись, что все скобки и массивы закрыты
- ОБЯЗАТЕЛЬНО заверши JSON полностью - все открывающие скобки { должны быть закрыты }, все массивы [ должны быть закрыты ]
- НЕ возвращай пустой JSON объект {} и не возвращай пустую строку
- НЕ обрезай JSON в середине структуры - если не хватает места, верни сокращенную но полную структуру

Пример правильного ответа:
%s%s`, dmResponse, skeleton, criticalFooter)
}

// cleanJSONResponse очищает ответ LLM от markdown блоков кода и лишнего текста
func cleanJSONResponse(raw string) string {
	return jsonrepair.Clean(raw)
}

func decodeStrictAnalysis(input string) (*DMResponseAnalysis, error) {
	dec := json.NewDecoder(strings.NewReader(input))
	dec.DisallowUnknownFields()

	var analysis DMResponseAnalysis
	if err := dec.Decode(&analysis); err != nil {
		return nil, err
	}

	// Проверяем, что после JSON нет лишних токенов
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("extra data after JSON")
	}

	return &analysis, nil
}

func fallbackAnalysisFromResponse(dmResponse string) *DMResponseAnalysis {
	analysis := &DMResponseAnalysis{}
	if detectsCombatMarker(dmResponse) {
		defaultHP := 15
		defaultAC := 13
		defaultAttackBonus := 3
		analysis.CombatDetected = true
		analysis.Enemies = []Enemy{
			{
				Name:        "Неизвестный противник",
				HP:          &defaultHP,
				AC:          &defaultAC,
				AttackBonus: &defaultAttackBonus,
			},
		}
	}
	return analysis
}

func detectsCombatMarker(dmResponse string) bool {
	lower := strings.ToLower(dmResponse)
	markers := []string{
		"бой", "сражен", "битва", "атака", "атакует", "нападает",
		"враг", "противник", "монстр", "инициатив", "удар",
	}
	for _, marker := range markers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func isEmptyAnalysis(analysis *DMResponseAnalysis) bool {
	if analysis == nil {
		return true
	}

	return !analysis.CombatDetected &&
		len(analysis.Enemies) == 0 &&
		!analysis.QuestCompleted &&
		!analysis.QuestFailed &&
		analysis.QuestTitle == "" &&
		analysis.ExperienceGained == 0 &&
		analysis.ExperienceReason == "" &&
		len(analysis.ItemsReceived) == 0 &&
		analysis.LocationVisited == nil &&
		analysis.NPCMet == nil &&
		len(analysis.GeneratedImages) == 0
}

func recordAnalyzerJSONFailure(reason string) {
	count := atomic.AddUint64(&invalidAnalyzerJSONCount, 1)
	log.Printf("[DM Analyzer] analyzer_json_%s count=%d", reason, count)
	if reason == "empty_json" {
		emptyCount := atomic.AddUint64(&emptyAnalyzerJSONCount, 1)
		log.Printf("[DM Analyzer] analyzer_empty_json_rate count=%d", emptyCount)
	}
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

	repaired := jsonrepair.Repair(jsonStr)
	if json.Valid([]byte(repaired)) {
		return repaired
	}
	jsonStr = repaired

	// Если строка не начинается с {, пытаемся найти начало JSON
	if !strings.HasPrefix(jsonStr, "{") {
		firstBrace := strings.Index(jsonStr, "{")
		if firstBrace > 0 {
			jsonStr = jsonStr[firstBrace:]
		} else {
			// Нет открывающей скобки, возвращаем пустой объект
			return "{}"
		}
	}

	openBraces := 0
	openBrackets := 0
	inString := false
	escapeNext := false
	lastValidPos := 0
	inKey := false
	inValue := false

scanLoop:
	for i := 0; i < len(jsonStr); i++ {
		char := jsonStr[i]

		if escapeNext {
			escapeNext = false
			lastValidPos = i + 1
			continue
		}

		if char == '\\' {
			escapeNext = true
			lastValidPos = i + 1
			continue
		}

		if char == '"' && !escapeNext {
			inString = !inString
			lastValidPos = i + 1
			continue
		}

		if inString {
			lastValidPos = i + 1
			continue
		}

		// Обновляем позицию последнего валидного символа
		lastValidPos = i + 1

		switch char {
		case '{':
			openBraces++
		case '}':
			openBraces--
			if openBraces < 0 {
				// Закрывающих скобок больше, чем открывающих - обрезаем до этого места
				jsonStr = jsonStr[:i]
				break scanLoop
			}
		case '[':
			openBrackets++
		case ']':
			openBrackets--
			if openBrackets < 0 {
				// Закрывающих скобок больше, чем открывающих - обрезаем до этого места
				jsonStr = jsonStr[:i]
				break scanLoop
			}
		case ':':
			if !inKey && !inValue {
				inValue = true
			}
		case ',':
			if !inKey && !inValue {
				inKey = true
			}
		}
	}

	// Обрезаем до последней валидной позиции, если строка была обрезана
	if lastValidPos < len(jsonStr) {
		jsonStr = jsonStr[:lastValidPos]
	}

	result := strings.TrimRight(jsonStr, " \n\r\t,")

	// Исправляем незавершенные ключи и значения для DMResponseAnalysis структуры
	result = tryRepairDMResponseJSON(result)

	// Закрываем незакрытые строки
	if inString {
		result += "\""
	}

	// Закрываем незакрытые массивы и объекты
	if openBraces > 0 || openBrackets > 0 {
		// Удаляем последнюю запятую, если она есть
		result = strings.TrimSuffix(result, ",")
		// Закрываем массивы перед объектами
		for i := 0; i < openBrackets; i++ {
			result += "]"
		}
		for i := 0; i < openBraces; i++ {
			result += "}"
		}
	}

	return result
}

// validateDMResponseInput проверяет входные данные перед отправкой в LLM
func validateDMResponseInput(dmResponse string) error {
	if strings.TrimSpace(dmResponse) == "" {
		return fmt.Errorf("empty DM response")
	}

	if len(dmResponse) > 50000 { // Защита от слишком длинных ответов
		return fmt.Errorf("DM response too long: %d characters", len(dmResponse))
	}

	// Проверяем на наличие потенциально проблемных символов
	if strings.Contains(dmResponse, "\x00") {
		return fmt.Errorf("DM response contains null bytes")
	}

	return nil
}

// tryRepairDMResponseJSON пытается исправить незавершенные ключи и значения для DMResponseAnalysis структуры
func tryRepairDMResponseJSON(jsonStr string) string {
	jsonStr = strings.TrimSpace(jsonStr)

	// Проверяем на незавершенные ключи и значения, характерные для DMResponseAnalysis
	repairRules := map[string]string{
		`"combat_detected":`:          `"combat_detected":false`,
		`"enemies":`:                  `"enemies":[]`,
		`"quest_completed":`:          `"quest_completed":false`,
		`"quest_failed":`:             `"quest_failed":false`,
		`"quest_title":`:              `"quest_title":""`,
		`"experience_gained":`:        `"experience_gained":0`,
		`"experience_reason":`:        `"experience_reason":""`,
		`"items_received":`:           `"items_received":[]`,
		`"location_visited":`:         `"location_visited":null`,
		`"npc_met":`:                  `"npc_met":null`,
		`"generated_images":`:         `"generated_images":[]`,
	}

	// Проверяем каждый ключ на незавершенность
	for partialKey, fullReplacement := range repairRules {
		if strings.HasSuffix(jsonStr, partialKey) {
			log.Printf("[DM Analyzer] Detected truncated JSON ending with partial key '%s', completing with default value", partialKey)
			return jsonStr + fullReplacement + "}"
		}

		// Проверяем на незавершенные значения (например, "combat_detected":fal или "experience_gained":)
		if strings.Contains(jsonStr, partialKey) {
			// Ищем позицию ключа
			keyPos := strings.LastIndex(jsonStr, partialKey)
			if keyPos >= 0 {
				// Берем часть после ключа
				afterKey := jsonStr[keyPos+len(partialKey):]
				afterKey = strings.TrimSpace(afterKey)

				// Проверяем на незавершенные значения
				if afterKey == "" {
					// Ключ без значения, добавляем полное значение
					beforeKey := jsonStr[:keyPos+len(partialKey)]
					log.Printf("[DM Analyzer] Key '%s' has no value, completing with default", partialKey)
					return beforeKey + fullReplacement + "}"
				}

				// Проверяем на незавершенные булевы значения
				if partialKey == `"combat_detected":` && strings.HasPrefix(afterKey, "fal") {
					beforeValue := jsonStr[:keyPos+len(partialKey)]
					log.Printf("[DM Analyzer] Boolean value 'fal' incomplete, completing to 'false'")
					return beforeValue + "false}"
				}
				if partialKey == `"combat_detected":` && strings.HasPrefix(afterKey, "tru") {
					beforeValue := jsonStr[:keyPos+len(partialKey)]
					log.Printf("[DM Analyzer] Boolean value 'tru' incomplete, completing to 'true'")
					return beforeValue + "true}"
				}

				// Проверяем на незавершенные числовые значения
				if partialKey == `"experience_gained":` && (afterKey == "" || strings.HasPrefix(afterKey, "0") && len(afterKey) < 2) {
					beforeValue := jsonStr[:keyPos+len(partialKey)]
					log.Printf("[DM Analyzer] Numeric value incomplete, completing to '0'")
					return beforeValue + "0}"
				}

				// Проверяем на незавершенные строковые значения
				if (partialKey == `"quest_title":` || partialKey == `"experience_reason":`) &&
					(strings.HasPrefix(afterKey, `"`) && !strings.Contains(afterKey, `"`)) {
					beforeValue := jsonStr[:keyPos+len(partialKey)]
					log.Printf("[DM Analyzer] String value incomplete, completing with empty string")
					return beforeValue + `""}`
				}
			}
		}
	}

	// Проверяем на незавершенные массивы врагов
	if strings.HasSuffix(jsonStr, `"enemies":[`) {
		log.Printf("[DM Analyzer] Detected truncated JSON ending with 'enemies:[', completing with empty array")
		return jsonStr + "]"
	}

	// Проверяем на незавершенные массивы предметов
	if strings.HasSuffix(jsonStr, `"items_received":[`) {
		log.Printf("[DM Analyzer] Detected truncated JSON ending with 'items_received:[', completing with empty array")
		return jsonStr + "]"
	}

	// Проверяем на незавершенные массивы изображений
	if strings.HasSuffix(jsonStr, `"generated_images":[`) {
		log.Printf("[DM Analyzer] Detected truncated JSON ending with 'generated_images:[', completing with empty array")
		return jsonStr + "]"
	}

	return jsonStr
}

func (uc *AnalyzeDMResponseUseCase) recordLocationEvent(
	ctx context.Context,
	resp *locationeventapp.GenerateLocationEventResponse,
) error {
	if resp == nil || resp.Event == nil {
		return nil
	}
	if uc.eventRepo == nil && uc.indexDocUC == nil {
		return nil
	}

	content := buildLocationEventStory(resp.Event, resp.Description)

	if uc.eventRepo != nil {
		eventItem := &event.StoryEvent{
			GameSessionID: uc.sessionID,
			AuthorType:    event.AuthorTypeDM,
			Content:       content,
			CreatedAt:     time.Now(),
		}
		if err := uc.eventRepo.Save(ctx, eventItem); err != nil {
			return fmt.Errorf("failed to save location story event: %w", err)
		}
	}

	if uc.indexDocUC != nil {
		// Индексируем location event как StoryEvent для истории
		storyDoc := ragdomain.Document{
			ID:        uuid.New().String(),
			Source:    ragdomain.SourceEvent,
			SessionID: uc.sessionID,
			Text:      content,
			Timestamp: time.Now(),
		}
		if err := uc.indexDocUC.Execute(ctx, storyDoc); err != nil {
			log.Printf("[DM Analyzer] Failed to index location event story in RAG: %v", err)
			// Graceful fallback - продолжаем без индексации
		}

		// Дополнительно индексируем location event как отдельный документ для лучшего поиска RAG
		// Это позволяет находить информацию о location events через семантический поиск
		locationDoc := ragdomain.Document{
			ID:        uuid.New().String(),
			Source:    ragdomain.SourceLocation,
			SessionID: uc.sessionID,
			Text:      fmt.Sprintf("Локация: %s. %s", resp.Event.Name, resp.Event.Description),
			Timestamp: time.Now(),
		}
		if err := uc.indexDocUC.Execute(ctx, locationDoc); err != nil {
			log.Printf("[DM Analyzer] Failed to index location event as separate document: %v", err)
			// Graceful fallback - продолжаем без дополнительной индексации
		}
	}

	return nil
}

func buildLocationEventStory(ev *world.WorldEvent, fallbackDescription string) string {
	if ev == nil {
		return fallbackDescription
	}

	var meta world.LocationEventMetadata
	if len(ev.Metadata) > 0 {
		_ = json.Unmarshal(ev.Metadata, &meta)
	}

	description := ev.Description
	if description == "" {
		description = fallbackDescription
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("Событие локации: %s", ev.Name))
	if description != "" {
		parts = append(parts, description)
	}
	if meta.Hook != "" && meta.Hook != description {
		parts = append(parts, fmt.Sprintf("Крючок: %s", meta.Hook))
	}
	if len(meta.Options) > 0 {
		parts = append(parts, fmt.Sprintf("Варианты: %s", strings.Join(meta.Options, ", ")))
	}
	if len(meta.SuggestedChecks) > 0 {
		parts = append(parts, fmt.Sprintf("Проверки: %s", strings.Join(meta.SuggestedChecks, ", ")))
	}
	if meta.Stakes != "" {
		parts = append(parts, fmt.Sprintf("Ставки: %s", meta.Stakes))
	}

	return strings.Join(parts, "\n")
}


// findLocationIDByName ищет локацию по имени в мире и возвращает её ID
func (uc *AnalyzeDMResponseUseCase) findLocationIDByName(ctx context.Context, locationName string) uint {
	if uc.sessionRepo == nil || locationName == "" {
		return 0
	}

	// Получаем сессию для доступа к миру
	session, err := uc.sessionRepo.GetByChatID(ctx, uc.chatID)
	if err != nil || session == nil {
		return 0
	}

	// Ищем локацию по имени (без учета регистра)
	locationNameLower := strings.ToLower(locationName)
	for _, loc := range session.World.Locations {
		if strings.ToLower(loc.Name) == locationNameLower {
			return loc.ID
		}
	}

	return 0
}

// looksLikeTruncatedJSON проверяет, выглядит ли JSON как обрезанный
func looksLikeTruncatedJSON(jsonStr string) bool {
	jsonStr = strings.TrimSpace(jsonStr)
	if jsonStr == "" {
		return false
	}

	openBraces := strings.Count(jsonStr, "{") - strings.Count(jsonStr, "}")
	openBrackets := strings.Count(jsonStr, "[") - strings.Count(jsonStr, "]")

	// Если есть незакрытые скобки, вероятно truncated
	if openBraces > 0 || openBrackets > 0 {
		return true
	}

	// Если заканчивается на незавершенную строку (нечетное количество кавычек)
	quoteCount := strings.Count(jsonStr, `"`)
	if quoteCount%2 != 0 {
		return true
	}

	// Если заканчивается на запятую или двоеточие
	if len(jsonStr) > 0 {
		lastChar := jsonStr[len(jsonStr)-1]
		if lastChar == ',' || lastChar == ':' {
			return true
		}
	}

	return false
}

