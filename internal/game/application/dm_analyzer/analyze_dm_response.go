package dm_analyzer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	jsonrepair "dungeons-and-dragons-ai/internal/game/application/jsonrepair"
	locationeventapp "dungeons-and-dragons-ai/internal/game/application/location_event"
	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/combat"
	"dungeons-and-dragons-ai/internal/game/domain/event"
	"dungeons-and-dragons-ai/internal/game/domain/inventory"
	"dungeons-and-dragons-ai/internal/game/domain/quest"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/game/domain/world"
	"dungeons-and-dragons-ai/internal/llm/domain"
	ragdomain "dungeons-and-dragons-ai/internal/rag/domain"

	"github.com/google/uuid"
)

// DMResponseAnalysis содержит структурированный анализ ответа DM
type DMResponseAnalysis struct {
	// Боевая ситуация
	CombatDetected        bool    `json:"combat_detected"`                   // Начался ли бой
	CombatEnded           bool    `json:"combat_ended"`                      // Закончился ли бой (победа/поражение/отступление) — значимое событие для автогена изображений
	Enemies               []Enemy `json:"enemies,omitempty"`                 // Список врагов, если бой начался
	CombatStartMessage    string  `json:"combat_start_message,omitempty"`    // Сообщение о порядке ходов при начале боя
	CombatFallbackMessage string  `json:"combat_fallback_message,omitempty"` // Сообщение игроку при combat_detected и пустом списке врагов

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

	// Компаньоны — присоединение/уход по ходу сюжета (не через ручные команды /dismiss
	// и не через случайное событие локации recruit_companion, а из естественного текста DM)
	CompanionJoined *CompanionJoined `json:"companion_joined,omitempty"` // NPC, присоединившийся к отряду
	CompanionLeft   *CompanionLeft   `json:"companion_left,omitempty"`   // Компаньон, покинувший отряд
	// CompanionEventMessage — сообщение игроку о присоединении/уходе компаньона,
	// заполняется после обработки (не приходит от LLM)
	CompanionEventMessage string `json:"-"`

	// Автоматически сгенерированные изображения
	GeneratedImages []GeneratedImage `json:"generated_images,omitempty"` // Пути к автоматически сгенерированным изображениям

	// ImageLimitReachedMessage — сообщение игроку при достижении лимита изображений на сессию
	ImageLimitReachedMessage string `json:"image_limit_reached_message,omitempty"`
	// ImagesGeneratedInSession — число изображений в сессии после этого ответа (для сохранения в GameSession)
	ImagesGeneratedInSession int `json:"images_generated_in_session,omitempty"`

	// KeyFacts — устойчивые факты кампании (репутация, вехи квеста, решения с долгими
	// последствиями), которые должны быть видны DM независимо от текущей локации.
	KeyFacts []KeyFact `json:"key_facts,omitempty"`
}

// KeyFact представляет один устойчивый факт кампании, извлечённый из ответа DM.
type KeyFact struct {
	Category string `json:"category"` // reputation|quest|decision|relationship|npc_identity
	Text     string `json:"text"`     // Краткая формулировка факта
}

var invalidAnalyzerJSONCount uint64
var emptyAnalyzerJSONCount uint64

// GeneratedImage представляет автоматически сгенерированное изображение
type GeneratedImage struct {
	Type       string `json:"type"`        // Тип: "item", "location", "npc"
	ImagePath  string `json:"image_path"`  // Путь к изображению (может быть пустым если не скачано)
	FileID     string `json:"file_id"`     // File ID для повторных попыток скачивания
	EntityName string `json:"entity_name"` // Название сущности (предмет, локация, NPC)
	Downloaded bool   `json:"downloaded"`  // Удалось ли скачать изображение
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
	Name          string  `json:"name"`           // Название предмета
	Description   string  `json:"description"`    // Описание предмета
	Weight        float64 `json:"weight"`         // Вес в кг (оценка, если не указано)
	Quantity      int     `json:"quantity"`       // Количество (по умолчанию 1)
	Type          string  `json:"type"`           // Тип предмета: "weapon", "armor", "potion", "tool", "misc", "consumable"
	HealingAmount int     `json:"healing_amount"` // Сколько HP восстанавливает ОДНА единица предмета при использовании (0 - не лечит)
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

// CompanionJoined представляет NPC, присоединившегося к отряду игрока по ходу сюжета
type CompanionJoined struct {
	Name        string `json:"name"`        // Имя нового компаньона
	Class       string `json:"class"`       // Класс/роль (Воин, Маг, Разбойник, Целитель и т.п.)
	Description string `json:"description"` // Краткое описание персонажа
}

// CompanionLeft представляет компаньона, покинувшего отряд по ходу сюжета
// (погиб, ушёл по своим делам, предал и т.п. — не ручной /dismiss)
type CompanionLeft struct {
	Name   string `json:"name"`   // Имя компаньона, покидающего отряд
	Reason string `json:"reason"` // Причина ухода из текста DM
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
	llm                      domain.LLM
	combatRepo               CombatRepository
	questRepo                QuestRepository
	inventoryRepo            InventoryRepository
	sessionID                uint
	chatID                   int64 // ChatID для отправки уведомлений
	worldID                  uint
	characterID              uint                      // ID персонажа игрока
	playerID                 uint                      // ID игрока (для проверки достижений)
	combatStartMessage       string                    // Сообщение о порядке ходов при начале боя
	companionEventMessage    string                    // Сообщение о присоединении/уходе компаньона
	imageLimitReachedMessage string                    // Сообщение при достижении лимита изображений на сессию
	checkAchievementsUC      AchievementChecker        // Опциональная зависимость для проверки достижений
	notificationService      NotificationService       // Опциональная зависимость для отправки уведомлений
	imageGenerationService   ImageGenerationService    // Опциональная зависимость для автоматической генерации изображений
	userID                   int64                     // ID пользователя для генерации изображений (Telegram User ID)
	checkDailyProgressUC     DailyQuestProgressChecker // Опциональная зависимость для отслеживания ежедневных заданий
	tgUserID                 int64                     // Telegram User ID для отслеживания ежедневных заданий
	generateLocationEventUC  LocationEventGenerator    // Опциональная зависимость для генерации событий локаций
	sessionRepo              SessionRepository         // Репозиторий сессий для поиска локаций
	fullSessionRepo          FullSessionRepository     // Репозиторий для доступа к полной сессии (для pending проверок)
	eventRepo                StoryEventRepository      // Репозиторий для записи событий истории
	indexDocUC               RAGIndexer                // Индексатор RAG для событий
	campaignFactRepo         CampaignFactRepository    // Репозиторий для записи ключевых фактов кампании
	imagesGeneratedInSession int                       // Счетчик изображений, сгенерированных в этой сессии
	maxImagesPerSession      int                       // Максимальное количество изображений за сессию
	autoGenerateImages       bool                      // Включить/отключить автоматическую генерацию изображений
}

// SessionRepository интерфейс для доступа к сессии (для поиска локаций)
type SessionRepository interface {
	GetByChatID(ctx context.Context, chatID int64) (*SessionSnapshot, error)
}

type FullSessionRepository interface {
	GetByChatID(ctx context.Context, chatID int64) (*session.GameSession, error)
	Save(ctx context.Context, session *session.GameSession) error
}

// SessionSnapshot — упрощенная структура сессии (для доступа к миру и локациям)
type SessionSnapshot struct {
	ID              uint
	World           WorldSnapshot
	CurrentLocation *LocationInfo  // Текущая локация персонажа
	Character       *CharacterInfo // Информация о персонаже
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

// LocationInfo — информация о текущей локации
type LocationInfo struct {
	ID          uint
	Name        string
	Description string
}

// CharacterInfo — информация о персонаже
type CharacterInfo struct {
	ID          uint
	Name        string
	Class       string
	Level       int
	Race        string
	Description string
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
	EntityName      string // Уникальное имя сущности для кэширования
	ForceRegenerate bool
	UserID          int64
	ChatID          int64 // ID игры (сессии), к которой привязан лимит "изображений за игру"
	SkipLimitCheck  bool
}

// GenerateImageResponse ответ на запрос генерации изображения
type GenerateImageResponse struct {
	ImagePath  string
	FileID     string
	FromCache  bool
	Downloaded bool
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
	autoGenerateImages bool, // Включить/отключить автоматическую генерацию изображений
	imagesGeneratedInSession int, // Текущее число изображений в сессии (из GameSession)
) *AnalyzeDMResponseUseCase {
	return &AnalyzeDMResponseUseCase{
		llm:                      llm,
		combatRepo:               combatRepo,
		questRepo:                questRepo,
		inventoryRepo:            inventoryRepo,
		sessionID:                sessionID,
		chatID:                   chatID,
		worldID:                  worldID,
		characterID:              characterID,
		playerID:                 playerID,
		imagesGeneratedInSession: imagesGeneratedInSession,
		maxImagesPerSession:      3, // Максимум 3 изображения за сессию
		autoGenerateImages:       autoGenerateImages,
	}
}

// SetCheckAchievementsUseCase устанавливает AchievementChecker для проверки достижений
func (uc *AnalyzeDMResponseUseCase) SetCheckAchievementsUseCase(checkAchievementsUC AchievementChecker) {
	uc.checkAchievementsUC = checkAchievementsUC
}

// SetAutoGenerateImages включает или отключает автоматическую генерацию изображений
func (uc *AnalyzeDMResponseUseCase) SetAutoGenerateImages(enabled bool) {
	uc.autoGenerateImages = enabled
	log.Printf("[DM Analyzer] Auto-generate images set to: %v", enabled)
}

// SetFullSessionRepository устанавливает репозиторий для доступа к полной сессии
func (uc *AnalyzeDMResponseUseCase) SetFullSessionRepository(repo FullSessionRepository) {
	uc.fullSessionRepo = repo
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

// CampaignFactRepository интерфейс для записи ключевых фактов кампании
type CampaignFactRepository interface {
	Save(ctx context.Context, fact *world.CampaignFact) error
}

// SetCampaignFactRepository устанавливает репозиторий ключевых фактов кампании
func (uc *AnalyzeDMResponseUseCase) SetCampaignFactRepository(repo CampaignFactRepository) {
	uc.campaignFactRepo = repo
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

	// Распознаем естественные запросы проверок в ответе DM
	if err := uc.recognizeNaturalAbilityChecks(ctx, dmResponse, analysis); err != nil {
		log.Printf("Failed to recognize natural ability checks: %v", err)
		// Логируем ошибку, но не прерываем выполнение
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
	// Сообщение при достижении лимита изображений на сессию
	analysis.ImageLimitReachedMessage = uc.imageLimitReachedMessage
	analysis.ImagesGeneratedInSession = uc.imagesGeneratedInSession

	// Сообщение о присоединении/уходе компаньона (если было обработано)
	if uc.companionEventMessage != "" {
		analysis.CompanionEventMessage = uc.companionEventMessage
		uc.companionEventMessage = "" // Очищаем для следующего использования
	}

	return analysis, nil
}

// validateCombatAnalysis проверяет корректность анализа боя с строгими проверками
func validateCombatAnalysis(analysis *DMResponseAnalysis) (*DMResponseAnalysis, error) {
	// Строгая валидация всех полей анализа
	validatedAnalysis, err := validateAnalysisStrict(analysis)
	if err != nil {
		return validatedAnalysis, err
	}

	// Дополнительная валидация полей боя
	if validatedAnalysis.CombatDetected {
		// Проверяем, что у всех врагов есть валидные характеристики
		for i, enemy := range validatedAnalysis.Enemies {
			if enemy.Name == "" {
				log.Printf("[DM Analyzer] Enemy %d has empty name, this is invalid for combat", i)
				validatedAnalysis.CombatDetected = false
				validatedAnalysis.Enemies = nil
				break
			}

			// Проверяем HP
			if enemy.HP == nil {
				defaultHP := 15
				log.Printf("[DM Analyzer] Enemy '%s' has null HP, setting default %d", enemy.Name, defaultHP)
				enemy.HP = &defaultHP
			} else if *enemy.HP <= 0 {
				defaultHP := 15
				log.Printf("[DM Analyzer] Enemy '%s' has invalid HP %d, setting default %d", enemy.Name, *enemy.HP, defaultHP)
				enemy.HP = &defaultHP
			}

			// Проверяем AC
			if enemy.AC == nil {
				defaultAC := 13
				log.Printf("[DM Analyzer] Enemy '%s' has null AC, setting default %d", enemy.Name, defaultAC)
				enemy.AC = &defaultAC
			} else if *enemy.AC <= 0 {
				defaultAC := 13
				log.Printf("[DM Analyzer] Enemy '%s' has invalid AC %d, setting default %d", enemy.Name, *enemy.AC, defaultAC)
				enemy.AC = &defaultAC
			}

			// Проверяем AttackBonus
			if enemy.AttackBonus == nil {
				defaultBonus := 3
				log.Printf("[DM Analyzer] Enemy '%s' has null attack_bonus, setting default %d", enemy.Name, defaultBonus)
				enemy.AttackBonus = &defaultBonus
			}

			// Обновляем врага в массиве
			validatedAnalysis.Enemies[i] = enemy
		}

		// Если после валидации не осталось валидных врагов, отключаем бой и задаём fallback
		if len(validatedAnalysis.Enemies) == 0 {
			log.Printf("[DM Analyzer] No valid enemies remain after validation, disabling combat")
			validatedAnalysis.CombatDetected = false
			validatedAnalysis.Enemies = nil
			validatedAnalysis.CombatFallbackMessage = "Опиши, с кем именно происходит бой, или уточни ситуацию."
		}
	}

	return validatedAnalysis, nil
}

// validateAnalysisStrict выполняет полную валидацию анализа с fallback логикой
func validateAnalysisStrict(analysis *DMResponseAnalysis) (*DMResponseAnalysis, error) {
	if analysis == nil {
		log.Printf("[DM Analyzer] Analysis is nil, returning empty analysis")
		return &DMResponseAnalysis{}, nil
	}

	// Проверяем на пустой/незначимый анализ (все поля по умолчанию)
	isEmptyAnalysis := !analysis.CombatDetected &&
		!analysis.CombatEnded &&
		len(analysis.Enemies) == 0 &&
		!analysis.QuestCompleted &&
		!analysis.QuestFailed &&
		analysis.QuestTitle == "" &&
		analysis.ExperienceGained == 0 &&
		analysis.ExperienceReason == "" &&
		len(analysis.ItemsReceived) == 0 &&
		analysis.LocationVisited == nil &&
		analysis.NPCMet == nil &&
		analysis.CompanionJoined == nil &&
		analysis.CompanionLeft == nil &&
		len(analysis.GeneratedImages) == 0 &&
		len(analysis.KeyFacts) == 0

	if isEmptyAnalysis {
		log.Printf("[DM Analyzer] Analysis appears to be empty/default values, this may indicate LLM parsing issues")
		// Возвращаем анализ как есть, но логируем для отладки
	}

	// Валидируем боевую ситуацию с дополнительными проверками
	if analysis.CombatDetected {
		validEnemies, hasValidEnemies := validateEnemiesStrict(analysis.Enemies)
		if !hasValidEnemies {
			log.Printf("[DM Analyzer] Combat detected but no valid enemies found, disabling combat")
			analysis.CombatDetected = false
			analysis.Enemies = nil
			analysis.CombatFallbackMessage = "Опиши, с кем именно происходит бой, или уточни ситуацию."
		} else {
			analysis.Enemies = validEnemies
			// Дополнительная проверка: убеждаемся что у всех врагов есть имена и характеристики
			validEnemyCount := 0
			for _, enemy := range analysis.Enemies {
				if enemy.Name != "" && enemy.HP != nil && *enemy.HP > 0 && enemy.AC != nil && *enemy.AC > 0 && enemy.AttackBonus != nil {
					validEnemyCount++
				}
			}
			if validEnemyCount == 0 {
				log.Printf("[DM Analyzer] Combat detected but no enemies have complete stats, disabling combat")
				analysis.CombatDetected = false
				analysis.Enemies = nil
				analysis.CombatFallbackMessage = "Опиши, с кем именно происходит бой, или уточни ситуацию."
			}
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

	// Валидируем присоединение компаньона
	if analysis.CompanionJoined != nil {
		if strings.TrimSpace(analysis.CompanionJoined.Name) == "" {
			log.Printf("[DM Analyzer] Companion joined but name is empty, clearing companion_joined")
			analysis.CompanionJoined = nil
		}
	}

	// Валидируем уход компаньона
	if analysis.CompanionLeft != nil {
		if strings.TrimSpace(analysis.CompanionLeft.Name) == "" {
			log.Printf("[DM Analyzer] Companion left but name is empty, clearing companion_left")
			analysis.CompanionLeft = nil
		}
	}

	// Компаньон не может одновременно присоединиться и покинуть отряд под одним и тем же именем
	if analysis.CompanionJoined != nil && analysis.CompanionLeft != nil &&
		strings.EqualFold(strings.TrimSpace(analysis.CompanionJoined.Name), strings.TrimSpace(analysis.CompanionLeft.Name)) {
		log.Printf("[DM Analyzer] Companion joined and left with the same name in one response, ignoring both")
		analysis.CompanionJoined = nil
		analysis.CompanionLeft = nil
	}

	return analysis, nil
}

// validateJSONSchemaStrict выполняет строгую валидацию JSON схемы перед отправкой в LLM
func validateJSONSchemaStrict(prompt string) error {
	// Проверяем наличие всех обязательных полей в промпте
	requiredFields := []string{
		"combat_detected",
		"combat_ended",
		"enemies",
		"quest_completed",
		"quest_failed",
		"quest_title",
		"experience_gained",
		"experience_reason",
		"items_received",
		"location_visited",
		"npc_met",
		"companion_joined",
		"companion_left",
		"generated_images",
	}

	for _, field := range requiredFields {
		fieldPattern := fmt.Sprintf(`"%s":`, field)
		if !strings.Contains(prompt, fieldPattern) {
			return fmt.Errorf("missing required field in prompt: %s", field)
		}
	}

	// Проверяем наличие критических инструкций
	criticalInstructions := []string{
		"ТОЛЬКО валидный JSON",
		"без текста до/после",
		"Все скобки и массивы ДОЛЖНЫ быть закрыты",
		"НЕ возвращай пустой JSON",
		"generated_images",
	}

	for _, instruction := range criticalInstructions {
		if !strings.Contains(prompt, instruction) {
			return fmt.Errorf("missing critical instruction in prompt: %s", instruction)
		}
	}

	return nil
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
	// Пустой JSON объект - это валидный случай, но он будет обработан отдельно
	// как пустой анализ
	if strings.TrimSpace(jsonStr) == "{}" {
		return nil
	}

	// Проверяем наличие хотя бы одного ключевого поля (менее строгие проверки)
	keyFields := []string{
		"combat_detected",
		"combat_ended",
		"enemies",
		"quest_completed",
		"quest_failed",
		"experience_gained",
		"items_received",
	}

	hasAtLeastOneField := false
	for _, field := range keyFields {
		fieldPattern := fmt.Sprintf(`"%s":`, field)
		if strings.Contains(jsonStr, fieldPattern) {
			hasAtLeastOneField = true
			break
		}
	}

	if !hasAtLeastOneField {
		return fmt.Errorf("missing any required fields")
	}

	// Проверяем структуру enemies массива только если поле присутствует
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

	// Проверяем структуру items_received массива только если поле присутствует
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

	// Pre-validation JSON структуры перед отправкой в LLM
	if err := validateAnalysisPromptStructure(prompt); err != nil {
		log.Printf("[DM Analyzer] Prompt validation failed: %v", err)
		return fallbackAnalysisFromResponse(dmResponse), nil
	}

	// Дополнительная strict валидация JSON схемы перед отправкой
	if err := validateJSONSchemaStrict(prompt); err != nil {
		log.Printf("[DM Analyzer] JSON schema validation failed: %v", err)
		return fallbackAnalysisFromResponse(dmResponse), nil
	}

	// Увеличен таймаут для анализа DM ответов
	llmCtx, llmCancel := context.WithTimeout(ctx, 90*time.Second) // Увеличиваем таймаут до 90 секунд
	defer llmCancel()

	// Устанавливаем лимит токенов для предотвращения усечения JSON
	maxTokens := 8192 // Увеличенный лимит токенов для полного JSON ответа
	raw, err := uc.llm.GenerateWithMaxTokens(llmCtx, prompt, maxTokens)
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
		cleaned = tryRepairTruncatedJSONForDMAnalysis(cleaned, true)
	}

	// Пытаемся восстановить JSON если он невалиден
	if !json.Valid([]byte(cleaned)) {
		log.Printf("[DM Analyzer] Invalid JSON after initial cleaning, attempting repair (attempt: %d)...", attempt+1)
		cleaned = tryRepairTruncatedJSONForDMAnalysis(cleaned, true)

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

	// Пустой анализ (все поля false/пусто) — легитимный результат для обычного
	// повествовательного хода без механических последствий, а не признак сбоя парсинга:
	// JSON успешно и строго распарсился (decodeStrictAnalysis выше не вернул ошибку).
	// Раньше это трактовалось как невалидный ответ и приводило к 5 лишним retry на
	// каждый обычный ход (доп. задержка и расход токенов на большинстве ходов, где
	// объективно ничего не произошло). Сохраняем только текстовый fallback как
	// подстраховку на случай, если DM явно описал бой, а анализатор его не заметил —
	// но без повторных вызовов LLM, сразу по первому пустому результату.
	if isEmptyAnalysis(analysis) {
		if detectsCombatMarker(dmResponse) {
			log.Printf("[DM Analyzer] Empty analysis but combat markers found in DM text, using text-based fallback (attempt: %d)", attempt+1)
			return fallbackAnalysisFromResponse(dmResponse), nil
		}
		log.Printf("[DM Analyzer] Empty analysis, no combat markers — trusting parsed result (attempt: %d)", attempt+1)
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
	// Проверяем достижения и ежедневные квесты по участию в бою (combat_participated)
	if analysis.CombatDetected && len(analysis.Enemies) > 0 {
		if uc.checkAchievementsUC != nil && uc.playerID > 0 {
			achievementReq := CheckAchievementsRequest{
				PlayerID:       uc.playerID,
				RequirementKey: "combat_participated",
				CurrentValue:   1, // Увеличиваем на 1 участие в бою
			}

			unlocked, err := uc.checkAchievementsUC.Execute(ctx, achievementReq)
			if err != nil {
				log.Printf("[DM Analyzer] Failed to check achievements after combat participation: %v", err)
			} else if len(unlocked) > 0 {
				// Логируем и отправляем уведомления о разблокированных достижениях
				for _, achievement := range unlocked {
					log.Printf("[DM Analyzer] Achievement unlocked after combat participation: %s (%s)",
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

		// Отслеживаем прогресс ежедневных заданий по участию в бою
		if uc.checkDailyProgressUC != nil && uc.tgUserID > 0 {
			dailyReq := DailyQuestProgressRequest{
				ChatID:    uc.chatID,
				TgUserID:  uc.tgUserID,
				QuestType: "win_combat", // Увеличиваем прогресс по победам в бою
				Increment: 0,            // Пока не победили, просто отмечаем участие
			}
			if err := uc.checkDailyProgressUC.Execute(ctx, dailyReq); err != nil {
				log.Printf("[DM Analyzer] Failed to check daily quest progress after combat participation: %v", err)
			}
		}
	}

	// Обрабатываем боевую ситуацию
	// ВАЖНО: Проверяем, нет ли уже активного боя перед обработкой врагов
	// Это предотвращает повторную генерацию врагов при каждом анализе ответа DM
	if analysis.CombatDetected && len(analysis.Enemies) > 0 {
		if uc.combatRepo == nil {
			log.Printf("[DM Analyzer] Combat repo is nil, skipping combat handling (session_id: %d)", uc.sessionID)
			analysis.CombatDetected = false
			analysis.Enemies = nil
		} else {
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

		// Проверяем достижения по коллекционированию предметов
		if uc.checkAchievementsUC != nil && uc.playerID > 0 {
			achievementReq := CheckAchievementsRequest{
				PlayerID:       uc.playerID,
				RequirementKey: "items_collected",
				CurrentValue:   len(analysis.ItemsReceived), // Увеличиваем на количество полученных предметов
			}

			unlocked, err := uc.checkAchievementsUC.Execute(ctx, achievementReq)
			if err != nil {
				log.Printf("[DM Analyzer] Failed to check achievements after item collection: %v", err)
			} else if len(unlocked) > 0 {
				// Логируем и отправляем уведомления о разблокированных достижениях
				for _, achievement := range unlocked {
					log.Printf("[DM Analyzer] Achievement unlocked after item collection: %s (%s)",
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

		// Автоген изображений только для значимых событий (новая локация, ключевой NPC, конец боя).
		// Предметы не генерируем автоматически — только по явному запросу или крупному событию.
	}

	// Обрабатываем посещение новой локации
	if analysis.LocationVisited != nil && analysis.LocationVisited.IsFirstVisit {
		// Проверяем достижения по исследованию локаций
		if uc.checkAchievementsUC != nil && uc.playerID > 0 {
			achievementReq := CheckAchievementsRequest{
				PlayerID:       uc.playerID,
				RequirementKey: "locations_visited",
				CurrentValue:   1, // Увеличиваем на 1 посещенную локацию
			}

			unlocked, err := uc.checkAchievementsUC.Execute(ctx, achievementReq)
			if err != nil {
				log.Printf("[DM Analyzer] Failed to check achievements after location visit: %v", err)
			} else if len(unlocked) > 0 {
				// Логируем и отправляем уведомления о разблокированных достижениях
				for _, achievement := range unlocked {
					log.Printf("[DM Analyzer] Achievement unlocked after location visit: %s (%s)",
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

		// Пассивная подстраховка на случай, если DM не вызвал save_campaign_fact сам:
		// фиксируем идентичность NPC (имя + роль/родство из описания) сразу при первом
		// представлении, чтобы DM не мог позже противоречиво переопределить, кто это.
		npcName := strings.TrimSpace(analysis.NPCMet.Name)
		npcDesc := strings.TrimSpace(analysis.NPCMet.Description)
		if npcName != "" && npcDesc != "" {
			analysis.KeyFacts = append(analysis.KeyFacts, KeyFact{
				Category: string(world.FactCategoryNPCIdentity),
				Text:     fmt.Sprintf("%s — %s", npcName, npcDesc),
			})
		}
	}

	// Обрабатываем конец боя (значимое событие для автогена изображений)
	if analysis.CombatEnded && uc.imageGenerationService != nil && uc.userID > 0 {
		generatedImage := uc.generateImageForCombatEnd(ctx)
		if generatedImage != nil {
			analysis.GeneratedImages = append(analysis.GeneratedImages, *generatedImage)
		}
	}

	// Обрабатываем присоединение/уход компаньонов по ходу сюжета
	if analysis.CompanionJoined != nil {
		if err := uc.handleCompanionJoined(ctx, analysis.CompanionJoined); err != nil {
			log.Printf("[DM Analyzer] Failed to handle companion joined: %v", err)
		}
	}
	if analysis.CompanionLeft != nil {
		if err := uc.handleCompanionLeft(ctx, analysis.CompanionLeft); err != nil {
			log.Printf("[DM Analyzer] Failed to handle companion left: %v", err)
		}
	}

	// Сохраняем ключевые факты кампании (глобальная память, не привязанная к локации)
	if len(analysis.KeyFacts) > 0 {
		uc.recordCampaignFacts(ctx, analysis.KeyFacts)
	}

	return nil
}

// recordCampaignFacts сохраняет устойчивые факты кампании, извлечённые из ответа DM.
// Ошибки записи не прерывают обработку ответа — эта память является дополнительной.
func (uc *AnalyzeDMResponseUseCase) recordCampaignFacts(ctx context.Context, facts []KeyFact) {
	if uc.campaignFactRepo == nil || uc.worldID == 0 {
		return
	}

	for _, kf := range facts {
		text := strings.TrimSpace(kf.Text)
		if text == "" {
			continue
		}

		category := world.CampaignFactCategory(strings.TrimSpace(kf.Category))
		switch category {
		case world.FactCategoryReputation, world.FactCategoryQuest, world.FactCategoryDecision,
			world.FactCategoryRelationship, world.FactCategoryItem, world.FactCategoryNPCIdentity:
			// валидная категория
		default:
			category = world.FactCategoryDecision
		}

		fact := &world.CampaignFact{
			WorldID:  uc.worldID,
			Category: category,
			Text:     text,
		}
		if err := uc.campaignFactRepo.Save(ctx, fact); err != nil {
			log.Printf("[DM Analyzer] Failed to save campaign fact: %v", err)
		}
	}
}

// companionStatRNG — источник случайности для генерации характеристик компаньонов,
// присоединяющихся по ходу сюжета (аналог rng в player_action.recruitCompanion).
var companionStatRNG = rand.New(rand.NewSource(time.Now().UnixNano()))

// handleCompanionJoined добавляет NPC в отряд игрока, когда DM явно описал
// присоединение компаньона по ходу сюжета (не через ручной /recruit или
// случайное событие локации). Ошибка не прерывает обработку остального анализа.
func (uc *AnalyzeDMResponseUseCase) handleCompanionJoined(ctx context.Context, joined *CompanionJoined) error {
	if uc.fullSessionRepo == nil {
		log.Printf("[DM Analyzer] Full session repo is nil, skipping companion joined handling")
		return nil
	}

	name := strings.TrimSpace(joined.Name)
	if name == "" {
		return nil
	}

	gs, err := uc.fullSessionRepo.GetByChatID(ctx, uc.chatID)
	if err != nil {
		return fmt.Errorf("failed to get session for companion joined: %w", err)
	}
	if gs == nil {
		return fmt.Errorf("session not found for companion joined")
	}

	// Не дублируем компаньона, если он уже в отряде
	for _, existing := range gs.Companions {
		if strings.EqualFold(existing.Name, name) {
			log.Printf("[DM Analyzer] Companion %q already in party, skipping duplicate join", name)
			return nil
		}
	}

	class := strings.TrimSpace(joined.Class)
	if class == "" {
		class = "Соратник"
	}
	description := strings.TrimSpace(joined.Description)
	if description == "" {
		description = fmt.Sprintf("%s присоединился к отряду", name)
	}

	level, hp, ac, attackBonus, damageDice := generateCompanionStats(class)

	companion := &session.Companion{
		GameSessionID: gs.ID,
		Name:          name,
		Description:   description,
		Class:         class,
		Level:         level,
		HP:            hp,
		MaxHP:         hp,
		AC:            ac,
		AttackBonus:   attackBonus,
		DamageDice:    damageDice,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	gs.AddCompanion(companion)

	if err := uc.fullSessionRepo.Save(ctx, gs); err != nil {
		return fmt.Errorf("failed to save session after companion joined: %w", err)
	}

	log.Printf("[DM Analyzer] Companion joined party: %s (%s, %d ур)", name, class, level)
	uc.companionEventMessage = fmt.Sprintf("🎉 К вашему отряду присоединился компаньон: %s (%s, %d ур)", name, class, level)

	return nil
}

// handleCompanionLeft удаляет NPC из отряда игрока, когда DM явно описал
// окончательный уход компаньона по ходу сюжета (гибель, расставание, предательство).
// Ошибка не прерывает обработку остального анализа.
func (uc *AnalyzeDMResponseUseCase) handleCompanionLeft(ctx context.Context, left *CompanionLeft) error {
	if uc.fullSessionRepo == nil {
		log.Printf("[DM Analyzer] Full session repo is nil, skipping companion left handling")
		return nil
	}

	name := strings.TrimSpace(left.Name)
	if name == "" {
		return nil
	}

	gs, err := uc.fullSessionRepo.GetByChatID(ctx, uc.chatID)
	if err != nil {
		return fmt.Errorf("failed to get session for companion left: %w", err)
	}
	if gs == nil {
		return fmt.Errorf("session not found for companion left")
	}

	var companionID uint
	found := false
	for _, existing := range gs.Companions {
		if strings.EqualFold(existing.Name, name) {
			companionID = existing.ID
			found = true
			break
		}
	}
	if !found {
		log.Printf("[DM Analyzer] Companion %q not found in party, nothing to remove", name)
		return nil
	}

	gs.RemoveCompanion(companionID)

	if err := uc.fullSessionRepo.Save(ctx, gs); err != nil {
		return fmt.Errorf("failed to save session after companion left: %w", err)
	}

	reason := strings.TrimSpace(left.Reason)
	log.Printf("[DM Analyzer] Companion left party: %s (reason: %s)", name, reason)
	if reason != "" {
		uc.companionEventMessage = fmt.Sprintf("👋 Компаньон %s покинул отряд. Причина: %s", name, reason)
	} else {
		uc.companionEventMessage = fmt.Sprintf("👋 Компаньон %s покинул отряд.", name)
	}

	return nil
}

// generateCompanionStats генерирует базовые характеристики компаньона на основе класса —
// аналогично player_action.recruitCompanion, но для компаньонов, присоединяющихся
// по ходу сюжета, а не через случайное событие локации.
func generateCompanionStats(class string) (level, hp, ac, attackBonus int, damageDice string) {
	level = 1 + companionStatRNG.Intn(3) // 1-3
	hp = 20 + companionStatRNG.Intn(30)  // 20-49
	ac = 12 + companionStatRNG.Intn(4)   // 12-15
	attackBonus = 2 + companionStatRNG.Intn(3)
	damageDice = "1d8"

	switch strings.ToLower(strings.TrimSpace(class)) {
	case "маг", "волшебник", "чародей":
		ac -= 1
		damageDice = "1d6"
	case "целитель", "жрец", "друид":
		attackBonus -= 1
	case "разбойник", "плут":
		attackBonus += 1
		damageDice = "1d6"
	}
	if ac < 10 {
		ac = 10
	}
	if attackBonus < 1 {
		attackBonus = 1
	}

	return level, hp, ac, attackBonus, damageDice
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

	// Проверяем лимит изображений за сессию
	if uc.imagesGeneratedInSession >= uc.maxImagesPerSession {
		log.Printf("[DM Analyzer] Session image limit reached (%d/%d), skipping item image generation", uc.imagesGeneratedInSession, uc.maxImagesPerSession)
		uc.imageLimitReachedMessage = "Достигнут лимит изображений на эту сессию. Можно запросить картинку вручную командой /image."
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
			EntityID:        0,         // Пока нет привязки к ID предмета в БД
			EntityName:      item.Name, // Используем имя предмета для кэширования
			ForceRegenerate: false,
			UserID:          uc.userID,
			ChatID:          uc.chatID,
			SkipLimitCheck:  false, // Проверяем лимиты
		}

		resp, err := uc.imageGenerationService.GenerateImage(imgCtx, req)
		if err != nil {
			// Graceful fallback: добавляем текстовое описание вместо изображения
			log.Printf("Failed to auto-generate image for item '%s', using text fallback: %v", item.Name, err)
			generatedImages = append(generatedImages, GeneratedImage{
				Type:       "item",
				ImagePath:  "", // Пустой путь означает текстовое описание
				FileID:     "",
				EntityName: item.Name,
				Downloaded: false,
			})
			// Продолжаем генерацию для остальных предметов
			continue
		}

		log.Printf("Auto-generated image for item: %s (path: %s)", item.Name, resp.ImagePath)

		// Увеличиваем счетчик изображений в сессии
		uc.imagesGeneratedInSession++

		// Добавляем путь к изображению в список
		generatedImages = append(generatedImages, GeneratedImage{
			Type:       "item",
			ImagePath:  resp.ImagePath,
			FileID:     resp.FileID,
			EntityName: item.Name,
			Downloaded: resp.Downloaded,
		})
	}

	return generatedImages
}

// generateImageForLocation автоматически генерирует изображение для локации
func (uc *AnalyzeDMResponseUseCase) generateImageForLocation(
	ctx context.Context,
	location Location,
) *GeneratedImage {
	if uc.imageGenerationService == nil || uc.userID == 0 || !uc.autoGenerateImages {
		return nil
	}

	// Проверяем лимит изображений за сессию
	if uc.imagesGeneratedInSession >= uc.maxImagesPerSession {
		log.Printf("[DM Analyzer] Session image limit reached (%d/%d), skipping location image generation", uc.imagesGeneratedInSession, uc.maxImagesPerSession)
		uc.imageLimitReachedMessage = "Достигнут лимит изображений на эту сессию. Можно запросить картинку вручную командой /image."
		return nil
	}

	// Создаем контекст с таймаутом для генерации изображений (увеличено до 180 секунд для обработки rate limiting)
	imgCtx, cancel := context.WithTimeout(ctx, 300*time.Second)
	defer cancel()

	// Получаем информацию о мире для более детального контекста
	var worldContext string
	// TODO: Добавить получение информации о мире через WorldRepository

	// Формируем детализированный системный промпт для генерации изображения локации
	systemPrompt := `Ты — талантливый художник в стиле фэнтези и Dungeons & Dragons. Создавай детализированные, атмосферные и захватывающие изображения локаций в стиле классического фэнтези-арта.

Стиль: цифровая живопись, высокая детализация, реалистичные текстуры, драматическое освещение, богатая цветовая палитра, элементы готического и классического фэнтези-арта.

Техники: глубокая перспектива, детальные архитектурные элементы, атмосферные эффекты (туман, световые лучи, частицы), богатые текстуры (камень, дерево, ткань), эмоциональная атмосфера.

Избегай: современные элементы, низкое качество, плоские изображения, чрезмерная стилизация.`

	// Формируем детализированный пользовательский промпт с контекстом
	userPrompt := fmt.Sprintf("Локация: %s", location.Name)

	if location.Description != "" {
		userPrompt += fmt.Sprintf("\nОписание: %s", location.Description)
	}

	if worldContext != "" {
		userPrompt += fmt.Sprintf("\nКонтекст мира: %s", worldContext)
	}

	// Добавляем атмосферу и детали на основе типа локации
	locationTypeDetails := getLocationTypeDetails(location.Name, location.Description)
	if locationTypeDetails != "" {
		userPrompt += fmt.Sprintf("\nДетали визуализации: %s", locationTypeDetails)
	}

	// Добавляем текущую ситуацию/время суток (базовые настройки)
	// TODO: Добавить анализ последнего ответа DM для определения времени суток и погоды

	log.Printf("[DM Analyzer] Generated detailed location prompt: %s", userPrompt)

	// Генерируем изображение
	req := GenerateImageRequest{
		SystemPrompt:    systemPrompt,
		UserPrompt:      userPrompt,
		Type:            "location",
		EntityID:        0,             // Пока нет привязки к ID локации в БД
		EntityName:      location.Name, // Используем имя локации для кэширования
		ForceRegenerate: false,
		UserID:          uc.userID,
		ChatID:          uc.chatID,
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

	// Увеличиваем счетчик изображений в сессии
	uc.imagesGeneratedInSession++

	return &GeneratedImage{
		Type:       "location",
		ImagePath:  resp.ImagePath,
		FileID:     resp.FileID,
		EntityName: location.Name,
		Downloaded: resp.Downloaded,
	}
}

// generateImageForNPC автоматически генерирует изображение для NPC
func (uc *AnalyzeDMResponseUseCase) generateImageForNPC(
	ctx context.Context,
	npc NPC,
) *GeneratedImage {
	if uc.imageGenerationService == nil || uc.userID == 0 || !uc.autoGenerateImages {
		return nil
	}

	// Проверяем лимит изображений за сессию
	if uc.imagesGeneratedInSession >= uc.maxImagesPerSession {
		log.Printf("[DM Analyzer] Session image limit reached (%d/%d), skipping NPC image generation", uc.imagesGeneratedInSession, uc.maxImagesPerSession)
		uc.imageLimitReachedMessage = "Достигнут лимит изображений на эту сессию. Можно запросить картинку вручную командой /image."
		return nil
	}

	// Создаем контекст с таймаутом для генерации изображений (увеличено до 180 секунд для обработки rate limiting)
	imgCtx, cancel := context.WithTimeout(ctx, 300*time.Second)
	defer cancel()

	// Получаем информацию о мире для более детального контекста
	var worldContext string
	// TODO: Добавить получение информации о мире через WorldRepository

	// Получаем текущую локацию для контекста
	var locationContext string
	// TODO: Добавить получение текущей локации

	// Формируем детализированный системный промпт для генерации изображения NPC
	systemPrompt := `Ты — талантливый художник в стиле фэнтези и Dungeons & Dragons. Создавай детализированные, выразительные портреты персонажей и NPC в стиле классического фэнтези-арта.

Стиль: цифровая живопись, реалистичные лица и выражения, богатые костюмы, детальные аксессуары, драматическое освещение, глубокие эмоции, элементы готического и ренессансного искусства.

Техники: детальная проработка черт лица, текстуры тканей и материалов, выразительные позы, богатые цветовые палитры, атмосферное освещение, профессиональный портретный стиль.

Избегай: карикатурность, современная одежда, низкое качество, чрезмерная стилизация.`

	// Формируем детализированный пользовательский промпт с контекстом
	userPrompt := fmt.Sprintf("Персонаж: %s", npc.Name)

	if npc.Description != "" {
		userPrompt += fmt.Sprintf("\nОписание: %s", npc.Description)
	}

	if worldContext != "" {
		userPrompt += fmt.Sprintf("\nМир: %s", worldContext)
	}

	if locationContext != "" {
		userPrompt += fmt.Sprintf("\nТекущая локация: %s", locationContext)
	}

	// Добавляем детали на основе типа NPC
	npcTypeDetails := getNPCTypeDetails(npc.Name, npc.Description)
	if npcTypeDetails != "" {
		userPrompt += fmt.Sprintf("\nДетали внешности: %s", npcTypeDetails)
	}

	// Добавляем текущую ситуацию/эмоциональное состояние (базовые настройки)
	// TODO: Добавить анализ последнего ответа DM для определения эмоционального состояния NPC

	log.Printf("[DM Analyzer] Generated detailed NPC prompt: %s", userPrompt)

	// Генерируем изображение
	req := GenerateImageRequest{
		SystemPrompt:    systemPrompt,
		UserPrompt:      userPrompt,
		Type:            "npc",
		EntityID:        0,        // Пока нет привязки к ID NPC в БД
		EntityName:      npc.Name, // Используем имя NPC для кэширования
		ForceRegenerate: false,
		UserID:          uc.userID,
		ChatID:          uc.chatID,
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

	// Увеличиваем счетчик изображений в сессии
	uc.imagesGeneratedInSession++

	return &GeneratedImage{
		Type:       "npc",
		ImagePath:  resp.ImagePath,
		FileID:     resp.FileID,
		EntityName: npc.Name,
		Downloaded: resp.Downloaded,
	}
}

// generateImageForCombatEnd автоматически генерирует изображение сцены после окончания боя (значимое событие для автогена).
func (uc *AnalyzeDMResponseUseCase) generateImageForCombatEnd(ctx context.Context) *GeneratedImage {
	if uc.imageGenerationService == nil || uc.userID == 0 || !uc.autoGenerateImages {
		return nil
	}
	if uc.imagesGeneratedInSession >= uc.maxImagesPerSession {
		log.Printf("[DM Analyzer] Session image limit reached (%d/%d), skipping combat end image generation", uc.imagesGeneratedInSession, uc.maxImagesPerSession)
		uc.imageLimitReachedMessage = "Достигнут лимит изображений на эту сессию. Можно запросить картинку вручную командой /image."
		return nil
	}
	imgCtx, cancel := context.WithTimeout(ctx, 300*time.Second)
	defer cancel()
	systemPrompt := `Ты — талантливый художник в стиле фэнтези и Dungeons & Dragons. Создавай атмосферные изображения сцен после боя: поле боя, победа, усталость героев, поверженные враги. Стиль: цифровая живопись, драматическое освещение, классический фэнтези.`
	userPrompt := "Сцена после победы в бою в стиле D&D: поле боя, герой уставший но победивший, атмосфера триумфа и завершённости."
	req := GenerateImageRequest{
		SystemPrompt:    systemPrompt,
		UserPrompt:      userPrompt,
		Type:            "custom",
		EntityID:        0,
		EntityName:      "combat_end",
		ForceRegenerate: false,
		UserID:          uc.userID,
		ChatID:          uc.chatID,
		SkipLimitCheck:  false,
	}
	resp, err := uc.imageGenerationService.GenerateImage(imgCtx, req)
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "429") || strings.Contains(errStr, "Too Many Requests") || strings.Contains(errStr, "rate limited") {
			log.Printf("[DM Analyzer] Rate limited during combat end image generation: %v", err)
		} else {
			log.Printf("[DM Analyzer] Failed to auto-generate image for combat end: %v", err)
		}
		return nil
	}
	log.Printf("[DM Analyzer] Auto-generated image for combat end (path: %s)", resp.ImagePath)
	uc.imagesGeneratedInSession++
	return &GeneratedImage{
		Type:       "custom",
		ImagePath:  resp.ImagePath,
		FileID:     resp.FileID,
		EntityName: "combat_end",
		Downloaded: resp.Downloaded,
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

	// Устанавливаем базовую информацию о персонаже для правильного отображения
	// Character будет загружен GORM автоматически при первом обращении
	if uc.sessionRepo != nil {
		session, err := uc.sessionRepo.GetByChatID(ctx, uc.chatID)
		if err == nil && session != nil && session.Character != nil {
			// Создаем Character с базовой информацией
			playerParticipant.Character = &character.Character{
				ID:    session.Character.ID,
				Name:  session.Character.Name,
				Class: character.Class(session.Character.Class),
				Level: session.Character.Level,
				Race:  character.Race(session.Character.Race),
			}
		}
	}

	newCombat.Participants = append(newCombat.Participants, playerParticipant)

	// TODO: Добавить активных компаньонов игрока в бой
	// (нужен доступ к полной GameSession вместо SessionSnapshot)

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

	var itemFacts []KeyFact

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

		// DM иногда повторно описывает вручение одного и того же уникального предмета в
		// последующих репликах (например, переспрашивает "берёшь меч?" уже после того, как
		// отдал его) — из-за этого экстрактор снова видит "вручение" и предлагает добавить
		// предмет ещё раз, пусть и под чуть другим названием. Для штучной экипировки
		// (оружие/броня/разное) пропускаем добавление, если в инвентаре уже есть предмет
		// того же типа с похожим названием.
		if itemType == inventory.ItemTypeWeapon || itemType == inventory.ItemTypeArmor || itemType == inventory.ItemTypeMisc {
			if hasSimilarItem(inv.Items, item.Name, itemType) {
				log.Printf("Skipping likely duplicate item '%s' (type: %s) - similar item already in inventory", item.Name, itemType)
				continue
			}
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
		if err := inv.AddItem(item.Name, description, weight, quantity, itemType, item.HealingAmount); err != nil {
			log.Printf("Failed to add item '%s' to inventory: %v", item.Name, err)
			// Продолжаем добавление остальных предметов даже при ошибке
			continue
		}

		log.Printf("Added item to inventory: %s (x%d, weight: %.2f kg, type: %s)",
			item.Name, quantity, weight, itemType)

		itemFacts = append(itemFacts, KeyFact{
			Category: string(world.FactCategoryItem),
			Text:     fmt.Sprintf("Персонаж получил предмет «%s» (тип: %s)", item.Name, itemType),
		})
	}

	// Сохраняем обновленный инвентарь
	if err := uc.inventoryRepo.Save(ctx, inv); err != nil {
		return fmt.Errorf("failed to save inventory: %w", err)
	}

	// Фиксируем получение предметов в ключевых фактах кампании, чтобы DM видел их в
	// контексте на любом следующем ходу (в отличие от "Инвентарь персонажа", который
	// подмешивается в промпт только по прямому запросу игрока) и не предлагал их повторно.
	if len(itemFacts) > 0 {
		uc.recordCampaignFacts(ctx, itemFacts)
	}

	return nil
}

// hasSimilarItem проверяет, есть ли среди предметов инвентаря того же типа предмет с похожим
// названием. Используется как эвристика против повторного добавления одного и того же
// уникального предмета, который DM описал вручённым игроку заново под чуть другим названием.
func hasSimilarItem(items []inventory.InventoryItem, name string, itemType inventory.ItemType) bool {
	for _, existing := range items {
		if existing.Type == itemType && similarItemNames(existing.Name, name) {
			return true
		}
	}
	return false
}

// similarItemNames сравнивает два названия предмета по пересечению значимых слов (длиной от 4
// символов, чтобы не учитывать предлоги и короткие слова вроде "меч"), не по точному совпадению
// строки - названия одного и того же уникального предмета от LLM нередко отличаются
// ("меч Авроры" / "светящийся клинок Авроры").
func similarItemNames(a, b string) bool {
	aLower := strings.ToLower(strings.TrimSpace(a))
	bLower := strings.ToLower(strings.TrimSpace(b))
	if aLower == bLower {
		return true
	}

	significantWords := func(s string) map[string]bool {
		words := make(map[string]bool)
		for _, w := range strings.Fields(s) {
			if len([]rune(w)) >= 4 {
				words[w] = true
			}
		}
		return words
	}

	aWords := significantWords(aLower)
	bWords := significantWords(bLower)
	if len(aWords) == 0 || len(bWords) == 0 {
		return false
	}

	matches := 0
	for w := range aWords {
		if bWords[w] {
			matches++
		}
	}

	smaller := len(aWords)
	if len(bWords) < smaller {
		smaller = len(bWords)
	}

	return float64(matches)/float64(smaller) >= 0.5
}

// buildAnalysisPrompt создает промпт для анализа ответа DM
func buildAnalysisPrompt(dmResponse string, strict bool) string {
	skeleton := `{"combat_detected":false,"combat_ended":false,"enemies":[],"quest_completed":false,"quest_failed":false,"quest_title":"","experience_gained":0,"experience_reason":"","items_received":[],"location_visited":null,"npc_met":null,"companion_joined":null,"companion_left":null,"generated_images":[],"key_facts":[]}`
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
  "combat_ended": true/false,
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
      "type": "weapon|armor|potion|tool|consumable|misc",
      "healing_amount": число
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
  },
  "companion_joined": {
    "name": "имя компаньона",
    "class": "класс/роль",
    "description": "краткое описание персонажа"
  },
  "companion_left": {
    "name": "имя компаньона",
    "reason": "причина ухода"
  },
  "generated_images": [],
  "key_facts": [
    {
      "category": "reputation|quest|decision|relationship",
      "text": "краткая формулировка факта"
    }
  ]
}

БОЙ И ВРАГИ (combat_detected):
- Устанавливай combat_detected=true ТОЛЬКО если в ответе DM явно описан НАЧАВШИЙСЯ бой с врагами
- Ключевые признаки боя: "нападает", "атакует", "бросается", "бьет", "стреляет", "заклинание", "удар", "рана", "ранение", "инициатива", "ход боя", "инициатива", "ранение", "рана"
- ДОПОЛНИТЕЛЬНЫЕ ПРИЗНАКИ БОЯ: "бросок атаки", "бросок на инициативу", "выстрел", "заклинание", "магия", "волшебство", "драка", "потасовка", "схватка", "побоище"
- РАСШИРЕННЫЕ ПРИЗНАКИ БОЯ: "спасбросок", "спас-бросок", "урон", "повреждение", "кровь", "оглушение", "паралич", "отравление", "слепота", "страх", "ужас", "смерть", "убийство"
- ПРИМЕРЫ, КОГДА combat_detected=true:
  * "Внезапно на вас нападает орк с топором!"
  * "Гоблин бросается на вас с кинжалом, инициатива!"
  * "Вы получаете ранение от стрелы!"
  * "Раздаётся боевой клич, начинается бой!"
  * "Враг наносит удар мечом, бросайте спасбросок!"
  * "Огромный тролль замахивается дубиной и атакует!"
  * "Из темноты выбегают волки и бросаются на вас!"
  * "Стражник обнажает меч и кричит: 'Стоять!'"
  * "Вас поражает молния от волшебника!"
  * "Медведь ревет и идёт в атаку!"
  * "Вы слышите боевой клич - начинается битва!"
  * "Гоблин атакует вас ржавым кинжалом!"
  * "Орк замахивается боевым топором и бросается вперёд!"
  * "Волки окружают вас, рыча и скаля зубы!"
  * "Из засады выскакивают разбойники с арбалетами!"
  * "Мумия оживает и тянет к вам свои бинты!"
  * "Призрак стонет и пытается вас коснуться ледяной рукой!"
  * "Вампир шипит и раскрывает свои клыки!"
  * "Оборотень воет и прыгает на вас с когтями!"
  * "Стражник обнажает меч и становится в боевую стойку!"
  * "Воин замахивается булавой и кричит боевой клич!"
  * "Лучник натягивает тетиву и целится в вас!"
  * "Маг начинает читать заклинание огненного шара!"
- ПРИМЕРЫ, КОГДА combat_detected=false:
  * "Впереди вы видите охрану у ворот" (только упоминание)
  * "Вы вспоминаете, как сражались с драконом" (воспоминания)
  * "Возможно, там будут монстры" (потенциальная угроза)
  * "Вы готовитесь к бою, проверяя оружие" (подготовка)
- НЕ устанавливай true если: только упоминание о потенциальной угрозе, планирование боя, воспоминания о прошлом бое
- Если combat_detected=true, ОБЯЗАТЕЛЬНО укажи хотя бы одного врага в массиве enemies
- Для каждого врага ОБЯЗАТЕЛЬНО укажи hp, ac и attack_bonus (только числа!)
- Если характеристики не указаны, используй значения по умолчанию:
  * hp: 15 (обычный враг), 35 (сильный), 75 (босс)
  * ac: 13 (обычный), 16 (сильный), 18 (босс)
  * attack_bonus: 3 (обычный), 5 (сильный), 7 (босс)

КВЕСТЫ (quest_completed/quest_failed):
- Устанавливай true только при явном завершении или провале квеста
- quest_title - название квеста из ответа DM

ОПЫТ (experience_gained):
- Начисляй только за значимые достижения: победа в бою, завершение квеста, важное открытие
- experience_reason - кратко объясни причину

ПРЕДМЕТЫ (items_received):
- Добавляй ТОЛЬКО если игрок явно получил НОВЫЙ предмет (слова: "получаешь", "находишь", "поднимаешь", "вручает", "дает", "дарит")
- НЕ добавляй если предмет только упоминается или игрок его еще не получил
- НЕ добавляй, если игрок ИСПОЛЬЗУЕТ/ПОТРЕБЛЯЕТ уже имеющийся у него предмет (слова: "пьешь", "выпиваешь", "используешь", "применяешь", "съедаешь", "расходуешь") - это НЕ получение предмета, а его расход, items_received для этого не предназначен
- Примеры, когда items_received НЕ добавляется: "Ты выпиваешь зелье лечения, тепло разливается по телу" / "Ты используешь последний факел" / "Ты съедаешь кусок хлеба из своих запасов"
- Если у полученного предмета есть эффект восстановления здоровья (лечебное зелье и т.п.) - обязательно укажи healing_amount (сколько HP восстанавливает ОДНА единица), иначе 0

ЛОКАЦИИ (location_visited):
- Устанавливай только при первом посещении новой локации
- Ключевые слова: "входишь в", "приходишь в", "попадаешь в", "оказываешься в", "перед тобой", "новое место"
- is_first_visit=true для новых локаций

NPC (npc_met):
- Устанавливай только при первой встрече с NPC
- Ключевые слова: "встречаешь", "видишь", "подходит", "появляется", "знакомишься"
- is_first_meeting=true для новых NPC
- description — ОБЯЗАТЕЛЬНО включи роль/родство/принадлежность NPC, если они прозвучали в тексте
  (например, "дочь старосты Ольгерда", "подмастерье кузнеца", "стражник у восточных ворот").
  Это описание используется как факт памяти о том, кто есть кто — не теряй эту деталь.

КОМПАНЬОНЫ, ПРИСОЕДИНЯЮЩИЕСЯ К ОТРЯДУ (companion_joined):
- Устанавливай companion_joined, ТОЛЬКО если NPC явно и окончательно присоединяется к отряду игрока как соратник/спутник
- Ключевые слова: "присоединяется к вам", "теперь с вами", "идёт с вами", "вступает в отряд", "готов сражаться на вашей стороне", "становится вашим спутником"
- НЕ устанавливай, если NPC просто помогает разово, даёт совет, сопровождает временно в пределах одной сцены или это союзник только на время одного боя
- НЕ устанавливай для NPC, которые просто идут в том же направлении или встречены мимоходом
- name — имя компаньона, class — его класс/роль (Воин, Маг, Разбойник, Целитель и т.п., по контексту), description — 1 короткое предложение

КОМПАНЬОНЫ, ПОКИДАЮЩИЕ ОТРЯД (companion_left):
- Устанавливай companion_left, ТОЛЬКО если ранее присоединившийся компаньон явно и окончательно покидает отряд по ходу сюжета: гибнет, уходит по своим делам, предаёт, остаётся в другом месте
- Ключевые слова: "покидает отряд", "прощается с вами", "остаётся здесь", "погибает", "больше не с вами", "уходит своей дорогой", "предаёт вас"
- НЕ устанавливай для временной разлуки в пределах одной сцены (например, компаньон отошёл в соседнюю комнату)
- name — имя компаньона, reason — краткая причина ухода из текста DM
- Устанавливай ТОЛЬКО одно из полей companion_joined/companion_left за раз для одного и того же имени компаньона в одном ответе

КРИТИЧЕСКИ ВАЖНО:
- ВСЕГДА возвращай ПОЛНЫЙ JSON со ВСЕМИ полями (даже если значения по умолчанию)
- Верни ТОЛЬКО валидный JSON, без текста до/после, без markdown, без комментариев
- Все строки в кавычках, числа без кавычек, булевы true/false
- Все скобки и массивы ДОЛЖНЫ быть закрыты - это критично!
- Массивы enemies, items_received, generated_images ДОЛЖНЫ быть закрыты ]
- Объекты location_visited, npc_met ДОЛЖНЫ быть закрыты }
- НЕ возвращай пустой JSON {} или неполный JSON
- JSON ДОЛЖЕН быть полным и завершённым - проверь все скобки перед отправкой
- Если сомневаешься - используй значения по умолчанию (false, 0, "", [], null)
- ОБЯЗАТЕЛЬНО включи поле "generated_images": [] в конце
- ЕСЛИ combat_detected=true, ТО ОБЯЗАТЕЛЬНО укажи хотя бы одного врага в enemies с полными характеристиками (name, hp, ac, attack_bonus)
- НЕ устанавливай combat_detected=true без enemies массива
- ПРОВЕРЬ: все массивы [], все объекты {}, все поля заполнены

КОНЕЦ БОЯ (combat_ended):
- Устанавливай combat_ended=true ТОЛЬКО когда в ответе DM явно описан КОНЕЦ боя: победа, враги повержены, отступление, разгром, бой завершён.
- Ключевые слова: "победа", "повержен", "побеждаете", "враг пал", "бой окончен", "закончился бой", "отступают", "разгром", "триумф".
- combat_ended=false во время боя или когда бой только начинается.

КЛЮЧЕВЫЕ ФАКТЫ КАМПАНИИ (key_facts):
- Добавляй запись, ТОЛЬКО если произошло что-то значимое для ВСЕЙ кампании, а не только для текущей локации:
  устойчивое изменение репутации, важное решение с долгими последствиями, веха главного или побочного квеста,
  заметный сдвиг в отношениях с NPC/фракцией, представление именованного NPC с его ролью/родством
- category: "reputation" (репутация), "quest" (веха квеста), "decision" (решение с последствиями),
  "relationship" (отношения с NPC/фракцией), "npc_identity" (кто есть кто — имя + роль/родство NPC)
- Для "npc_identity" — фиксируй сразу при первом представлении NPC (например, "Мира — дочь старосты
  Ольгерда"), чтобы позже не возникло противоречия (тот же NPC не должен стать "дочерью кузнеца")
- text — 1 короткое предложение, без пересказа диалога
- НЕ добавляй факты о рутинных или локальных действиях (осмотр комнаты, обычный диалог, находка мелкого предмета)
- Если ничего значимого для кампании не произошло, оставь key_facts пустым массивом []

ЗНАЧИМЫЕ СОБЫТИЯ ДЛЯ АВТОГЕНА ИЗОБРАЖЕНИЙ (только эти три; предметы — нет):
- Новая локация (location_visited с is_first_visit=true)
- Ключевой NPC — первая встреча (npc_met с is_first_meeting=true)
- Конец боя (combat_ended=true)
- Предметы (items_received) НЕ являются значимым событием для автогена изображений.

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
		// Расширенный fallback анализ: пытаемся извлечь информацию о врагах из текста
		enemies := extractEnemiesFromText(dmResponse)
		if len(enemies) == 0 {
			// Если не удалось извлечь врагов из текста, создаем базового врага
			defaultHP := 15
			defaultAC := 13
			defaultAttackBonus := 3
			enemies = []Enemy{
				{
					Name:        "Неизвестный противник",
					HP:          &defaultHP,
					AC:          &defaultAC,
					AttackBonus: &defaultAttackBonus,
				},
			}
		}
		analysis.CombatDetected = true
		analysis.Enemies = enemies
	}
	return analysis
}

// extractEnemiesFromText пытается извлечь информацию о врагах из текста DM ответа
func extractEnemiesFromText(dmResponse string) []Enemy {
	lower := strings.ToLower(dmResponse)
	var enemies []Enemy

	// Расширенный список известных врагов для поиска
	enemyTypes := map[string]struct{ hp, ac, attackBonus int }{
		// Маленькие гуманоиды
		"гоблин":    {hp: 10, ac: 15, attackBonus: 4},
		"кобольд":   {hp: 5, ac: 12, attackBonus: 2},
		"хобгоблин": {hp: 11, ac: 18, attackBonus: 3},

		// Средние гуманоиды
		"орк":     {hp: 20, ac: 13, attackBonus: 5},
		"полуорк": {hp: 18, ac: 12, attackBonus: 4},
		"огр":     {hp: 35, ac: 11, attackBonus: 6},

		// Большие существа
		"тролль":  {hp: 40, ac: 15, attackBonus: 7},
		"гигант":  {hp: 50, ac: 13, attackBonus: 8},
		"великан": {hp: 55, ac: 14, attackBonus: 8},

		// Нежить
		"скелет":  {hp: 13, ac: 15, attackBonus: 4},
		"зомби":   {hp: 22, ac: 12, attackBonus: 3},
		"призрак": {hp: 25, ac: 13, attackBonus: 5},
		"вампир":  {hp: 60, ac: 16, attackBonus: 7},
		"ли Lich": {hp: 70, ac: 15, attackBonus: 9},
		"смерть":  {hp: 100, ac: 20, attackBonus: 10},

		// Звери
		"волк":             {hp: 11, ac: 13, attackBonus: 4},
		"медведь":          {hp: 34, ac: 13, attackBonus: 6},
		"паук":             {hp: 16, ac: 13, attackBonus: 4},
		"паук гигантский":  {hp: 26, ac: 14, attackBonus: 6},
		"крыса":            {hp: 1, ac: 10, attackBonus: 0},
		"крыса гигантская": {hp: 7, ac: 12, attackBonus: 1},

		// Драконы и рептилии
		"дракон":          {hp: 80, ac: 18, attackBonus: 8},
		"дракончик":       {hp: 16, ac: 13, attackBonus: 4},
		"змея":            {hp: 2, ac: 13, attackBonus: 2},
		"змея гигантская": {hp: 11, ac: 14, attackBonus: 4},

		// Элементали и демоны
		"элементаль": {hp: 30, ac: 15, attackBonus: 5},
		"демон":      {hp: 45, ac: 15, attackBonus: 7},
		"дьявол":     {hp: 50, ac: 16, attackBonus: 8},
		"ангел":      {hp: 65, ac: 17, attackBonus: 8},

		// Гуманоидные враги
		"воин":      {hp: 35, ac: 16, attackBonus: 5},
		"рыцарь":    {hp: 45, ac: 18, attackBonus: 6},
		"варвар":    {hp: 50, ac: 14, attackBonus: 6},
		"лучник":    {hp: 25, ac: 14, attackBonus: 5},
		"разбойник": {hp: 28, ac: 14, attackBonus: 5},
		"страж":     {hp: 30, ac: 16, attackBonus: 4},
		"охранник":  {hp: 25, ac: 15, attackBonus: 4},
		"солдат":    {hp: 22, ac: 16, attackBonus: 4},

		// Магические существа
		"ведьма":    {hp: 30, ac: 13, attackBonus: 5},
		"маг":       {hp: 25, ac: 13, attackBonus: 5},
		"волшебник": {hp: 27, ac: 12, attackBonus: 5},
		"некромант": {hp: 33, ac: 12, attackBonus: 5},
		"фея":       {hp: 14, ac: 15, attackBonus: 4},

		// Мифические существа
		"минотавр": {hp: 42, ac: 14, attackBonus: 6},
		"гаргулья": {hp: 38, ac: 15, attackBonus: 4},
		"голем":    {hp: 60, ac: 16, attackBonus: 4},
		"горилла":  {hp: 19, ac: 12, attackBonus: 5},
		"тигр":     {hp: 37, ac: 12, attackBonus: 6},
		"лев":      {hp: 26, ac: 12, attackBonus: 5},
	}

	// Ищем упоминания врагов в тексте
	for enemyName, stats := range enemyTypes {
		if strings.Contains(lower, enemyName) {
			hp := stats.hp
			ac := stats.ac
			attackBonus := stats.attackBonus

			// Проверяем на множественное число или другие формы
			count := countEnemyMentions(dmResponse, enemyName)
			if count > 1 {
				// Если упоминается несколько раз, создаем несколько экземпляров
				for i := 0; i < count && i < 5; i++ { // Максимум 5 экземпляров одного типа
					enemies = append(enemies, Enemy{
						Name:        capitalizeFirst(enemyName),
						HP:          &hp,
						AC:          &ac,
						AttackBonus: &attackBonus,
					})
				}
			} else {
				enemies = append(enemies, Enemy{
					Name:        capitalizeFirst(enemyName),
					HP:          &hp,
					AC:          &ac,
					AttackBonus: &attackBonus,
				})
			}
		}
	}

	// Если не нашли известных врагов, но есть общие маркеры боя, создаем базового врага
	if len(enemies) == 0 && detectsCombatMarker(dmResponse) {
		defaultHP := 15
		defaultAC := 13
		defaultAttackBonus := 3
		enemies = append(enemies, Enemy{
			Name:        "Неизвестный противник",
			HP:          &defaultHP,
			AC:          &defaultAC,
			AttackBonus: &defaultAttackBonus,
		})
	}

	return enemies
}

// countEnemyMentions подсчитывает количество упоминаний врага в тексте
func countEnemyMentions(text, enemyName string) int {
	lower := strings.ToLower(text)
	enemyLower := strings.ToLower(enemyName)
	count := strings.Count(lower, enemyLower)

	// Расширенные правила для множественного числа и синонимов
	switch enemyLower {
	case "гоблин":
		count += strings.Count(lower, "гоблины")
		count += strings.Count(lower, "гоблина")
		count += strings.Count(lower, "гоблинов")
	case "кобольд":
		count += strings.Count(lower, "кобольды")
		count += strings.Count(lower, "кобольда")
		count += strings.Count(lower, "кобольдов")
	case "хобгоблин":
		count += strings.Count(lower, "хобгоблины")
		count += strings.Count(lower, "хобгоблина")
		count += strings.Count(lower, "хобгоблинов")
	case "орк":
		count += strings.Count(lower, "орки")
		count += strings.Count(lower, "орка")
		count += strings.Count(lower, "орков")
	case "полуорк":
		count += strings.Count(lower, "полуорки")
		count += strings.Count(lower, "полуорка")
		count += strings.Count(lower, "полуорков")
	case "огр":
		count += strings.Count(lower, "огры")
		count += strings.Count(lower, "огра")
		count += strings.Count(lower, "огров")
	case "тролль":
		count += strings.Count(lower, "тролли")
		count += strings.Count(lower, "троля")
		count += strings.Count(lower, "троллей")
	case "гигант":
		count += strings.Count(lower, "гиганты")
		count += strings.Count(lower, "гиганта")
		count += strings.Count(lower, "гигантов")
	case "великан":
		count += strings.Count(lower, "великаны")
		count += strings.Count(lower, "великана")
		count += strings.Count(lower, "великанов")
	case "скелет":
		count += strings.Count(lower, "скелеты")
		count += strings.Count(lower, "скелетов")
		count += strings.Count(lower, "скелета")
	case "зомби":
		count += strings.Count(lower, "зомби")
		count += strings.Count(lower, "зомби")
	case "призрак":
		count += strings.Count(lower, "призраки")
		count += strings.Count(lower, "призрака")
		count += strings.Count(lower, "призраков")
	case "вампир":
		count += strings.Count(lower, "вампиры")
		count += strings.Count(lower, "вампира")
		count += strings.Count(lower, "вампиров")
	case "ли Lich":
		count += strings.Count(lower, "личи")
		count += strings.Count(lower, "лича")
	case "волк":
		count += strings.Count(lower, "волки")
		count += strings.Count(lower, "волков")
		count += strings.Count(lower, "волка")
	case "медведь":
		count += strings.Count(lower, "медведи")
		count += strings.Count(lower, "медведя")
		count += strings.Count(lower, "медведей")
	case "паук":
		count += strings.Count(lower, "пауки")
		count += strings.Count(lower, "паука")
		count += strings.Count(lower, "пауков")
	case "паук гигантский":
		count += strings.Count(lower, "пауки гигантские")
		count += strings.Count(lower, "паука гигантского")
		count += strings.Count(lower, "пауков гигантских")
	case "крыса":
		count += strings.Count(lower, "крысы")
		count += strings.Count(lower, "крысу")
		count += strings.Count(lower, "крыс")
	case "крыса гигантская":
		count += strings.Count(lower, "крысы гигантские")
		count += strings.Count(lower, "крысу гигантскую")
		count += strings.Count(lower, "крыс гигантских")
	case "дракон":
		count += strings.Count(lower, "драконы")
		count += strings.Count(lower, "дракона")
		count += strings.Count(lower, "драконов")
	case "дракончик":
		count += strings.Count(lower, "дракончики")
		count += strings.Count(lower, "дракончика")
		count += strings.Count(lower, "дракончиков")
	case "змея":
		count += strings.Count(lower, "змеи")
		count += strings.Count(lower, "змею")
		count += strings.Count(lower, "змей")
	case "змея гигантская":
		count += strings.Count(lower, "змеи гигантские")
		count += strings.Count(lower, "змею гигантскую")
		count += strings.Count(lower, "змей гигантских")
	case "элементаль":
		count += strings.Count(lower, "элементали")
		count += strings.Count(lower, "элементаля")
		count += strings.Count(lower, "элементалей")
	case "демон":
		count += strings.Count(lower, "демоны")
		count += strings.Count(lower, "демона")
		count += strings.Count(lower, "демонов")
	case "дьявол":
		count += strings.Count(lower, "дьяволы")
		count += strings.Count(lower, "дьявола")
		count += strings.Count(lower, "дьяволов")
	case "ангел":
		count += strings.Count(lower, "ангелы")
		count += strings.Count(lower, "ангела")
		count += strings.Count(lower, "ангелов")
	case "воин":
		count += strings.Count(lower, "воины")
		count += strings.Count(lower, "воина")
		count += strings.Count(lower, "воинов")
	case "рыцарь":
		count += strings.Count(lower, "рыцари")
		count += strings.Count(lower, "рыцаря")
		count += strings.Count(lower, "рыцарей")
	case "варвар":
		count += strings.Count(lower, "варвары")
		count += strings.Count(lower, "варвара")
		count += strings.Count(lower, "варваров")
	case "лучник":
		count += strings.Count(lower, "лучники")
		count += strings.Count(lower, "лучника")
		count += strings.Count(lower, "лучников")
	case "разбойник":
		count += strings.Count(lower, "разбойники")
		count += strings.Count(lower, "разбойника")
		count += strings.Count(lower, "разбойников")
	case "страж":
		count += strings.Count(lower, "стражи")
		count += strings.Count(lower, "стража")
		count += strings.Count(lower, "стражей")
	case "охранник":
		count += strings.Count(lower, "охранники")
		count += strings.Count(lower, "охранника")
		count += strings.Count(lower, "охранников")
	case "солдат":
		count += strings.Count(lower, "солдаты")
		count += strings.Count(lower, "солдата")
		count += strings.Count(lower, "солдатов")
	case "ведьма":
		count += strings.Count(lower, "ведьмы")
		count += strings.Count(lower, "ведьму")
		count += strings.Count(lower, "ведьм")
	case "маг":
		count += strings.Count(lower, "маги")
		count += strings.Count(lower, "мага")
		count += strings.Count(lower, "магов")
	case "волшебник":
		count += strings.Count(lower, "волшебники")
		count += strings.Count(lower, "волшебника")
		count += strings.Count(lower, "волшебников")
	case "некромант":
		count += strings.Count(lower, "некроманты")
		count += strings.Count(lower, "некроманта")
		count += strings.Count(lower, "некромантов")
	case "фея":
		count += strings.Count(lower, "феи")
		count += strings.Count(lower, "фею")
		count += strings.Count(lower, "фей")
	case "минотавр":
		count += strings.Count(lower, "минотавры")
		count += strings.Count(lower, "минотавра")
		count += strings.Count(lower, "минотавров")
	case "гаргулья":
		count += strings.Count(lower, "гаргульи")
		count += strings.Count(lower, "гаргулью")
		count += strings.Count(lower, "гаргулий")
	case "голем":
		count += strings.Count(lower, "големы")
		count += strings.Count(lower, "глема")
		count += strings.Count(lower, "големов")
	case "горилла":
		count += strings.Count(lower, "гориллы")
		count += strings.Count(lower, "гориллу")
		count += strings.Count(lower, "горилл")
	case "тигр":
		count += strings.Count(lower, "тигры")
		count += strings.Count(lower, "тигра")
		count += strings.Count(lower, "тигров")
	case "лев":
		count += strings.Count(lower, "львы")
		count += strings.Count(lower, "льва")
		count += strings.Count(lower, "львов")
	}

	return count
}

// capitalizeFirst делает первую букву заглавной
func capitalizeFirst(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// validateAnalysisPromptStructure проверяет структуру промпта перед отправкой в LLM
func validateAnalysisPromptStructure(prompt string) error {
	requiredElements := []string{
		"combat_detected",
		"combat_ended",
		"enemies",
		"quest_completed",
		"quest_failed",
		"quest_title",
		"experience_gained",
		"experience_reason",
		"items_received",
		"location_visited",
		"npc_met",
		"generated_images",
	}

	for _, element := range requiredElements {
		if !strings.Contains(prompt, element) {
			return fmt.Errorf("missing required element in prompt: %s", element)
		}
	}

	// Проверяем, что промпт содержит пример правильного ответа
	if !strings.Contains(prompt, `"combat_detected":false`) {
		return fmt.Errorf("prompt missing example JSON structure")
	}

	return nil
}

// detectsCombatMarker — консервативная эвристика для fallback-детекции боя, когда
// структурный JSON-анализ ответа DM не удался (невалидный/пустой ответ после ретраев).
// Раньше здесь был список из ~150 общих слов (в т.ч. "заклинание", "магия", "меч",
// "щит", "огонь", "страх" и т.п.) с проверкой по началу слова — такие слова массово
// встречаются в обычном, не боевом фэнтезийном повествовании (лут, экипировка,
// атмосферные описания, тёмное сеттинговое лор), поэтому fallback регулярно спонтанно
// создавал "фантомный" бой с дефолтным врагом "Неизвестный противник" на ходах, где
// реального боя не было и никакой нарративной подводки к нему не давалось.
//
// Теперь fallback срабатывает только на сильных, практически однозначных сигналах:
// технический статус-тег, который DM обязан ставить в начале ответа во время боя
// (см. промпт "⚔️ КРИТИЧЕСКИ ВАЖНО: Статус боя в ответах"), явную механику
// броска/урона, или составную фразу конкретного враждебного действия — такие фразы,
// в отличие от одиночных существительных, почти не встречаются вне боевых сцен.
// Если анализатор действительно пропустит редкий боевой ход без этих сигналов —
// это не страшно: основной путь детекции боя — структурный JSON-анализ, а данная
// функция лишь подстраховка на случай его сбоя, и ложноотрицательный результат
// здесь безопаснее, чем ложное создание боя на пустом месте.
func detectsCombatMarker(dmResponse string) bool {
	lower := strings.ToLower(dmResponse)

	statusTags := []string{
		"⚔️ [в бою]", "⚔️ бой продолжается", "⚔️ ход боя",
	}
	for _, tag := range statusTags {
		if strings.Contains(lower, tag) {
			return true
		}
	}

	mechanicalMarkers := []string{
		"бросок атаки", "бросок на инициативу", "против ac",
		"критическое попадание", "критический удар",
		"спасбросок", "спас-бросок",
	}
	for _, marker := range mechanicalMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}

	// Небольшой список сильных однословных маркеров — в отличие от старого списка
	// (существительные вроде "меч"/"огонь"/"заклинание"), эти слова специфичны именно
	// для описания боя и почти не встречаются в мирном повествовании. Сравниваем по
	// целому слову (не по префиксу произвольной подстроки), чтобы не ловить их внутри
	// других слов.
	exactWordMarkers := map[string]bool{
		"бой": true, "битва": true, "атакует": true, "нападает": true,
	}
	// Стемы — тоже сравниваются по началу целого слова, чтобы поймать словоформы
	// ("сражение"/"сражается"/"сразились", "инициатива"/"инициативу"), но не задевать
	// случайные подстроки внутри других слов.
	prefixWordMarkers := []string{"сражен", "инициатив"}
	for _, w := range combatMarkerWordRe.FindAllString(lower, -1) {
		if exactWordMarkers[w] {
			return true
		}
		for _, p := range prefixWordMarkers {
			if strings.HasPrefix(w, p) {
				return true
			}
		}
	}

	phraseMarkers := []string{
		"гоблин атакует", "орк нападает", "дракон бросается", "волк прыгает",
		"медведь ревет", "тролль замахивается", "скелет поднимается", "зомби подходит",
		"паук плетет паутину", "змей шипит", "летучая мышь пищит", "призрак стонет",
		"вампир кусает", "оборотень воет", "ведьма колдует", "маг читает заклинание",
		"воин замахивается", "лучник натягивает тетиву", "рыцарь встает на защиту",
		"варвар кричит", "плут крадется", "жрец молится", "следопыт выслеживает",
		"некромант воскрешает", "иллюзионист обманывает", "алхимик бросает бомбу",
		"стрелок целится", "охотник подстерегает", "стражник обнажает меч",
		"капитан отдает приказ", "солдат строится в линию", "командир планирует атаку",
		"военачальник руководит", "генерал командует армией", "полководец планирует битву",
		"разбойник подстерегает", "бандит нападает", "пират грабит", "мародер рыщет",
		"охранник преграждает путь", "сторож караулит", "часовой замечает",
		"патруль идет", "дозорный осматривает", "разведчик докладывает",
		"бросается в атаку", "переходит в наступление", "открывает огонь",
		"начинает бой", "стартует битва", "завязывается сражение",
		"врывается в комнату", "выбегает из засады", "выходит из тени",
		"появляется внезапно", "нападает со спины", "бьет в спину",
		"атакует с фланга", "обходит с тыла", "окружает со всех сторон",
		"заводит в ловушку", "заманивает в капкан", "ставит силки",
		"натравливает собак", "спускает собак", "выпускает монстров",
		"вызывает демонов", "призывает духов", "заклинает элементалей",
	}
	for _, marker := range phraseMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}

	return false
}

// combatMarkerWordRe разбивает текст на слова (последовательности unicode-букв)
// для поиска однословных маркеров боя по началу/целому слову, а не по произвольной подстроке.
var combatMarkerWordRe = regexp.MustCompile(`[\p{L}]+`)

func isEmptyAnalysis(analysis *DMResponseAnalysis) bool {
	if analysis == nil {
		return true
	}

	return !analysis.CombatDetected &&
		!analysis.CombatEnded &&
		len(analysis.Enemies) == 0 &&
		!analysis.QuestCompleted &&
		!analysis.QuestFailed &&
		analysis.QuestTitle == "" &&
		analysis.ExperienceGained == 0 &&
		analysis.ExperienceReason == "" &&
		len(analysis.ItemsReceived) == 0 &&
		analysis.LocationVisited == nil &&
		analysis.NPCMet == nil &&
		analysis.CompanionJoined == nil &&
		analysis.CompanionLeft == nil &&
		len(analysis.GeneratedImages) == 0 &&
		len(analysis.KeyFacts) == 0
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

	// Пробуем jsonrepair.Repair
	repaired := jsonrepair.Repair(jsonStr)
	if json.Valid([]byte(repaired)) {
		log.Printf("[DM Analyzer] jsonrepair.Repair succeeded")
		return repaired
	}

	// Если jsonrepair не справился, применяем базовую логику
	jsonStr = repaired

	// Базовая логика закрытия незавершенных структур
	if !strings.HasPrefix(jsonStr, "{") {
		firstBrace := strings.Index(jsonStr, "{")
		if firstBrace > 0 {
			jsonStr = jsonStr[firstBrace:]
		} else {
			return "{}"
		}
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
			if openBraces < 0 {
				jsonStr = jsonStr[:i]
				break
			}
		case '[':
			openBrackets++
		case ']':
			openBrackets--
			if openBrackets < 0 {
				jsonStr = jsonStr[:i]
				break
			}
		}
	}

	result := strings.TrimRight(jsonStr, " \n\r\t,")

	// Закрываем незакрытые строки
	if inString {
		result += "\""
	}

	// Закрываем незакрытые массивы и объекты
	if openBraces > 0 || openBrackets > 0 {
		result = strings.TrimSuffix(result, ",")
		for i := 0; i < openBrackets; i++ {
			result += "]"
		}
		for i := 0; i < openBraces; i++ {
			result += "}"
		}
	}

	return result
}

// tryRepairTruncatedJSONForDMAnalysis пытается восстановить обрезанный JSON для DM анализа
func tryRepairTruncatedJSONForDMAnalysis(jsonStr string, isDMAnalysis bool) string {
	jsonStr = strings.TrimSpace(jsonStr)
	if jsonStr == "" {
		return "{}"
	}

	repaired := jsonrepair.Repair(jsonStr)
	if json.Valid([]byte(repaired)) {
		log.Printf("[DM Analyzer] jsonrepair.Repair succeeded")
		return repaired
	}
	jsonStr = repaired

	// Специальная логика для распространенных случаев усечения (только для DM анализа)
	if isDMAnalysis {
		// Проверяем на усечение в середине значений
		if strings.HasSuffix(jsonStr, `"npc_met":n`) {
			log.Printf("[DM Analyzer] Detected truncation at 'npc_met':n, completing to null")
			jsonStr += `ull,"generated_images":[]}`
		} else if strings.HasSuffix(jsonStr, `"npc_met":nu`) {
			jsonStr += `ll,"generated_images":[]}`
		} else if strings.HasSuffix(jsonStr, `"npc_met":nul`) {
			jsonStr += `l,"generated_images":[]}`
		} else if strings.HasSuffix(jsonStr, `"location_visited":n`) {
			log.Printf("[DM Analyzer] Detected truncation at 'location_visited':n, completing to null")
			jsonStr += `ull,"npc_met":null,"generated_images":[]}`
		} else if strings.HasSuffix(jsonStr, `"location_visited":nu`) {
			jsonStr += `ll,"npc_met":null,"generated_images":[]}`
		} else if strings.HasSuffix(jsonStr, `"location_visited":nul`) {
			jsonStr += `l,"npc_met":null,"generated_images":[]}`
		} else if strings.HasSuffix(jsonStr, `"combat_detected":f`) {
			log.Printf("[DM Analyzer] Detected truncation at 'combat_detected':f, completing to false")
			jsonStr += `alse,"enemies":[],"quest_completed":false,"quest_failed":false,"quest_title":"","experience_gained":0,"experience_reason":"","items_received":[],"location_visited":null,"npc_met":null,"generated_images":[]}`
		} else if strings.HasSuffix(jsonStr, `"combat_detected":t`) {
			log.Printf("[DM Analyzer] Detected truncation at 'combat_detected':t, completing to true")
			jsonStr += `rue,"enemies":[],"quest_completed":false,"quest_failed":false,"quest_title":"","experience_gained":0,"experience_reason":"","items_received":[],"location_visited":null,"npc_met":null,"generated_images":[]}`
		} else if strings.Contains(jsonStr, `"enemies":[`) && !strings.Contains(jsonStr, `"quest_completed"`) {
			// JSON обрывается в массиве врагов
			if strings.HasSuffix(jsonStr, `"enemies":[`) {
				log.Printf("[DM Analyzer] Detected truncation at enemies array start")
				jsonStr += `],"quest_completed":false,"quest_failed":false,"quest_title":"","experience_gained":0,"experience_reason":"","items_received":[],"location_visited":null,"npc_met":null,"generated_images":[]}`
			} else if strings.Contains(jsonStr, `"enemies":[`) && strings.HasSuffix(jsonStr, `{"name":`) {
				// Обрывается в начале объекта врага
				log.Printf("[DM Analyzer] Detected truncation in enemy object")
				jsonStr += `"Unknown","hp":15,"ac":13,"attack_bonus":3}],"quest_completed":false,"quest_failed":false,"quest_title":"","experience_gained":0,"experience_reason":"","items_received":[],"location_visited":null,"npc_met":null,"generated_images":[]}`
			} else if strings.Contains(jsonStr, `"enemies":[`) && strings.HasSuffix(jsonStr, `,"name":`) {
				// Обрывается после запятой в объекте врага
				log.Printf("[DM Analyzer] Detected truncation after enemy name comma")
				jsonStr += `"Unknown","hp":15,"ac":13,"attack_bonus":3}],"quest_completed":false,"quest_failed":false,"quest_title":"","experience_gained":0,"experience_reason":"","items_received":[],"location_visited":null,"npc_met":null,"generated_images":[]}`
			} else if strings.Contains(jsonStr, `"enemies":[`) && strings.HasSuffix(jsonStr, `,"hp":`) {
				// Обрывается после hp
				log.Printf("[DM Analyzer] Detected truncation after enemy hp")
				jsonStr += `15,"ac":13,"attack_bonus":3}],"quest_completed":false,"quest_failed":false,"quest_title":"","experience_gained":0,"experience_reason":"","items_received":[],"location_visited":null,"npc_met":null,"generated_images":[]}`
			} else if strings.Contains(jsonStr, `"enemies":[`) && strings.HasSuffix(jsonStr, `,"ac":`) {
				// Обрывается после ac
				log.Printf("[DM Analyzer] Detected truncation after enemy ac")
				jsonStr += `13,"attack_bonus":3}],"quest_completed":false,"quest_failed":false,"quest_title":"","experience_gained":0,"experience_reason":"","items_received":[],"location_visited":null,"npc_met":null,"generated_images":[]}`
			}
		} else if strings.Contains(jsonStr, `"npc_met":`) && !strings.Contains(jsonStr, `"generated_images"`) {
			log.Printf("[DM Analyzer] Detected truncation at npc_met field")
			// JSON обрывается на npc_met, добавляем завершение
			if strings.HasSuffix(jsonStr, `"npc_met":`) {
				jsonStr += `null,"generated_images":[]}`
			} else if strings.HasSuffix(jsonStr, `"npc_met":{`) {
				jsonStr += `"name":"","description":"","is_first_meeting":false},"generated_images":[]}`
			}
		} else if strings.Contains(jsonStr, `"location_visited":`) && !strings.Contains(jsonStr, `"npc_met"`) {
			// JSON обрывается на location_visited
			if strings.HasSuffix(jsonStr, `"location_visited":`) {
				jsonStr += `null,"npc_met":null,"generated_images":[]}`
			} else if strings.HasSuffix(jsonStr, `"location_visited":{`) {
				jsonStr += `"name":"","description":"","is_first_visit":false},"npc_met":null,"generated_images":[]}`
			}
		} else if !strings.Contains(jsonStr, `"generated_images"`) {
			// JSON не содержит generated_images, добавляем завершение
			jsonStr += `,"generated_images":[]}`
		} else if strings.Contains(jsonStr, `"combat_detected":true`) && !strings.Contains(jsonStr, `"enemies":[`) {
			// Бой обнаружен, но нет массива врагов - добавляем базового врага
			log.Printf("[DM Analyzer] Detected truncated JSON: combat_detected=true but no enemies array")
			jsonStr += `,"enemies":[{"name":"Неизвестный противник","hp":15,"ac":13,"attack_bonus":3}],"quest_completed":false,"quest_failed":false,"quest_title":"","experience_gained":0,"experience_reason":"","items_received":[],"location_visited":null,"npc_met":null,"generated_images":[]}`
		} else if strings.Contains(jsonStr, `"enemies":[`) && strings.HasSuffix(jsonStr, `"enemies":[`) {
			// Массив врагов пустой, добавляем базового врага
			log.Printf("[DM Analyzer] Detected truncated JSON: empty enemies array")
			jsonStr += `{"name":"Неизвестный противник","hp":15,"ac":13,"attack_bonus":3}],"quest_completed":false,"quest_failed":false,"quest_title":"","experience_gained":0,"experience_reason":"","items_received":[],"location_visited":null,"npc_met":null,"generated_images":[]}`
		} else if strings.Contains(jsonStr, `"items_received":[`) && !strings.Contains(jsonStr, `"location_visited"`) {
			// JSON обрывается в массиве предметов
			if strings.HasSuffix(jsonStr, `"items_received":[`) {
				log.Printf("[DM Analyzer] Detected truncation at items_received array start")
				jsonStr += `],"location_visited":null,"npc_met":null,"generated_images":[]}`
			} else if strings.Contains(jsonStr, `"items_received":[`) && strings.HasSuffix(jsonStr, `{"name":`) {
				// Обрывается в начале объекта предмета
				log.Printf("[DM Analyzer] Detected truncation in item object")
				jsonStr += `"Unknown","description":"","weight":0,"quantity":1,"type":"misc"}],"location_visited":null,"npc_met":null,"generated_images":[]}`
			} else if strings.Contains(jsonStr, `"items_received":[`) && strings.HasSuffix(jsonStr, `,"description":`) {
				// Обрывается после description
				log.Printf("[DM Analyzer] Detected truncation after item description")
				jsonStr += `"","weight":0,"quantity":1,"type":"misc"}],"location_visited":null,"npc_met":null,"generated_images":[]}`
			} else if strings.Contains(jsonStr, `"items_received":[`) && strings.HasSuffix(jsonStr, `,"weight":`) {
				// Обрывается после weight
				log.Printf("[DM Analyzer] Detected truncation after item weight")
				jsonStr += `0,"quantity":1,"type":"misc"}],"location_visited":null,"npc_met":null,"generated_images":[]}`
			} else if strings.Contains(jsonStr, `"items_received":[`) && strings.HasSuffix(jsonStr, `,"quantity":`) {
				// Обрывается после quantity
				log.Printf("[DM Analyzer] Detected truncation after item quantity")
				jsonStr += `1,"type":"misc"}],"location_visited":null,"npc_met":null,"generated_images":[]}`
			} else if strings.Contains(jsonStr, `"items_received":[`) && strings.HasSuffix(jsonStr, `,"type":`) {
				// Обрывается после type
				log.Printf("[DM Analyzer] Detected truncation after item type")
				jsonStr += `"misc"}],"location_visited":null,"npc_met":null,"generated_images":[]}`
			}
		} else if strings.Contains(jsonStr, `"experience_reason":`) && !strings.Contains(jsonStr, `"items_received"`) {
			// JSON обрывается на experience_reason
			if strings.HasSuffix(jsonStr, `"experience_reason":`) {
				jsonStr += `"","items_received":[],"location_visited":null,"npc_met":null,"generated_images":[]}`
			} else if strings.HasSuffix(jsonStr, `"experience_reason":"`) {
				jsonStr += `","items_received":[],"location_visited":null,"npc_met":null,"generated_images":[]}`
			}
		} else if strings.Contains(jsonStr, `"quest_title":`) && !strings.Contains(jsonStr, `"experience_gained"`) {
			// JSON обрывается на quest_title
			if strings.HasSuffix(jsonStr, `"quest_title":`) {
				jsonStr += `"","experience_gained":0,"experience_reason":"","items_received":[],"location_visited":null,"npc_met":null,"generated_images":[]}`
			} else if strings.HasSuffix(jsonStr, `"quest_title":"`) {
				jsonStr += `","experience_gained":0,"experience_reason":"","items_received":[],"location_visited":null,"npc_met":null,"generated_images":[]}`
			}
		} else if strings.Contains(jsonStr, `"quest_failed":`) && !strings.Contains(jsonStr, `"quest_title"`) {
			// JSON обрывается на quest_failed
			if strings.HasSuffix(jsonStr, `"quest_failed":`) {
				jsonStr += `false,"quest_title":"","experience_gained":0,"experience_reason":"","items_received":[],"location_visited":null,"npc_met":null,"generated_images":[]}`
			}
		} else if strings.Contains(jsonStr, `"quest_completed":`) && !strings.Contains(jsonStr, `"quest_failed"`) {
			// JSON обрывается на quest_completed
			if strings.HasSuffix(jsonStr, `"quest_completed":`) {
				jsonStr += `false,"quest_failed":false,"quest_title":"","experience_gained":0,"experience_reason":"","items_received":[],"location_visited":null,"npc_met":null,"generated_images":[]}`
			}
		}
	}

	// Проверяем валидность после ручного восстановления
	if json.Valid([]byte(jsonStr)) {
		return jsonStr
	}
	return jsonStr

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

	// Расширенные правила восстановления для обрезанных значений
	repairRules := map[string]string{
		`"combat_detected":`:   `"combat_detected":false`,
		`"enemies":`:           `"enemies":[]`,
		`"quest_completed":`:   `"quest_completed":false`,
		`"quest_failed":`:      `"quest_failed":false`,
		`"quest_title":`:       `"quest_title":""`,
		`"experience_gained":`: `"experience_gained":0`,
		`"experience_reason":`: `"experience_reason":""`,
		`"items_received":`:    `"items_received":[]`,
		`"location_visited":`:  `"location_visited":null`,
		`"npc_met":`:           `"npc_met":null`,
		`"generated_images":`:  `"generated_images":[]`,
	}

	// Специальные правила для восстановления обрезанных значений (типа "npc_met":n → "npc_met":null)
	truncationPatterns := map[string]map[string]string{
		`"npc_met":`: {
			"n":   "null",                                                  // "npc_met":n → "npc_met":null
			"nu":  "null",                                                  // "npc_met":nu → "npc_met":null
			"nul": "null",                                                  // "npc_met":nul → "npc_met":null
			"{":   `{"name":"","description":"","is_first_meeting":false}`, // "npc_met":{ → "npc_met":{...}
			`{"n`: `{"name":"","description":"","is_first_meeting":false}`, // "npc_met":{"n → "npc_met":{...}
		},
		`"location_visited":`: {
			"n":   "null",                                                // "location_visited":n → "location_visited":null
			"nu":  "null",                                                // "location_visited":nu → "location_visited":null
			"nul": "null",                                                // "location_visited":nul → "location_visited":null
			"{":   `{"name":"","description":"","is_first_visit":false}`, // "location_visited":{ → "location_visited":{...}
			`{"n`: `{"name":"","description":"","is_first_visit":false}`, // "location_visited":{"n → "location_visited":{...}
		},
		`"combat_detected":`: {
			"f":    "false", // "combat_detected":f → "combat_detected":false
			"fa":   "false", // "combat_detected":fa → "combat_detected":false
			"fal":  "false", // "combat_detected":fal → "combat_detected":false
			"fals": "false", // "combat_detected":fals → "combat_detected":false
			"t":    "true",  // "combat_detected":t → "combat_detected":true
			"tr":   "true",  // "combat_detected":tr → "combat_detected":true
			"tru":  "true",  // "combat_detected":tru → "combat_detected":true
		},
		`"quest_completed":`: {
			"f":    "false",
			"fa":   "false",
			"fal":  "false",
			"fals": "false",
			"t":    "true",
			"tr":   "true",
			"tru":  "true",
		},
		`"quest_failed":`: {
			"f":    "false",
			"fa":   "false",
			"fal":  "false",
			"fals": "false",
			"t":    "true",
			"tr":   "true",
			"tru":  "true",
		},
		`"experience_gained":`: {
			"0": "0",  // "experience_gained":0 → "experience_gained":0
			"1": "10", // "experience_gained":1 → "experience_gained":10 (предполагаем продолжение)
			"2": "20",
			"3": "30",
			"4": "40",
			"5": "50",
		},
		`"quest_title":`: {
			`"`:  `""`, // "quest_title":" → "quest_title":""
			`""`: `""`, // "quest_title":"" → "quest_title":""
		},
		`"experience_reason":`: {
			`"`:  `""`, // "experience_reason":" → "experience_reason":""
			`""`: `""`, // "experience_reason":"" → "experience_reason":""
		},
	}

	// Сначала проверяем специальные паттерны усечения
	for key, patterns := range truncationPatterns {
		if strings.Contains(jsonStr, key) {
			keyPos := strings.LastIndex(jsonStr, key)
			if keyPos >= 0 {
				afterKey := jsonStr[keyPos+len(key):]
				afterKey = strings.TrimSpace(afterKey)

				for prefix, completion := range patterns {
					if strings.HasPrefix(afterKey, prefix) && !strings.Contains(afterKey, completion) {
						beforeValue := jsonStr[:keyPos+len(key)]
						log.Printf("[DM Analyzer] Detected truncated value for '%s', prefix '%s', completing with '%s'", key, prefix, completion)
						return beforeValue + completion + "}"
					}
				}
			}
		}
	}

	// Проверяем каждый ключ на незавершенность
	for partialKey, fullReplacement := range repairRules {
		if strings.HasSuffix(jsonStr, partialKey) {
			log.Printf("[DM Analyzer] Detected truncated JSON ending with partial key '%s', completing with default value", partialKey)
			return jsonStr + fullReplacement + "}"
		}

		// Проверяем на незавершенные значения
		if strings.Contains(jsonStr, partialKey) {
			keyPos := strings.LastIndex(jsonStr, partialKey)
			if keyPos >= 0 {
				afterKey := jsonStr[keyPos+len(partialKey):]
				afterKey = strings.TrimSpace(afterKey)

				// Проверяем на незавершенные значения
				if afterKey == "" {
					beforeKey := jsonStr[:keyPos+len(partialKey)]
					log.Printf("[DM Analyzer] Key '%s' has no value, completing with default", partialKey)
					return beforeKey + fullReplacement + "}"
				}

				// Проверяем на незавершенные булевы значения
				if partialKey == `"combat_detected":` || partialKey == `"quest_completed":` || partialKey == `"quest_failed":` {
					if strings.HasPrefix(afterKey, "fal") && !strings.Contains(afterKey, "false") {
						beforeValue := jsonStr[:keyPos+len(partialKey)]
						log.Printf("[DM Analyzer] Boolean value incomplete, completing to 'false'")
						return beforeValue + "false}"
					}
					if strings.HasPrefix(afterKey, "tru") && !strings.Contains(afterKey, "true") {
						beforeValue := jsonStr[:keyPos+len(partialKey)]
						log.Printf("[DM Analyzer] Boolean value incomplete, completing to 'true'")
						return beforeValue + "true}"
					}
				}

				// Проверяем на незавершенные числовые значения
				if partialKey == `"experience_gained":` && afterKey != "" && !strings.Contains(afterKey, ",") {
					beforeValue := jsonStr[:keyPos+len(partialKey)]
					// Пытаемся завершить число
					if strings.HasPrefix(afterKey, "0") && len(afterKey) < 5 {
						log.Printf("[DM Analyzer] Numeric value incomplete, completing")
						return beforeValue + "0}"
					}
					// Для других цифр пытаемся угадать завершение
					if len(afterKey) == 1 && (afterKey[0] >= '0' && afterKey[0] <= '9') {
						completion := string(afterKey[0]) + "0"
						log.Printf("[DM Analyzer] Numeric value incomplete, completing to '%s'", completion)
						return beforeValue + completion + "}"
					}
				}

				// Проверяем на незавершенные строковые значения
				if partialKey == `"quest_title":` || partialKey == `"experience_reason":` {
					if strings.HasPrefix(afterKey, `"`) && !strings.Contains(afterKey, `",`) && !strings.HasSuffix(afterKey, `"}`) {
						beforeValue := jsonStr[:keyPos+len(partialKey)]
						log.Printf("[DM Analyzer] String value incomplete, completing with empty string")
						return beforeValue + `""}`
					}
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
	locationID := resp.Event.RequiredLocationID

	if uc.eventRepo != nil {
		eventItem := &event.StoryEvent{
			GameSessionID: uc.sessionID,
			LocationID:    locationID,
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
			ID:         uuid.New().String(),
			Source:     ragdomain.SourceEvent,
			SessionID:  uc.sessionID,
			LocationID: locationID,
			Text:       content,
			Timestamp:  time.Now(),
		}
		if err := uc.indexDocUC.Execute(ctx, storyDoc); err != nil {
			log.Printf("[DM Analyzer] Failed to index location event story in RAG: %v", err)
			// Graceful fallback - продолжаем без индексации
		}

		// Дополнительно индексируем location event как отдельный документ для лучшего поиска RAG
		// Это позволяет находить информацию о location events через семантический поиск
		locationDoc := ragdomain.Document{
			ID:         uuid.New().String(),
			Source:     ragdomain.SourceLocation,
			SessionID:  uc.sessionID,
			LocationID: locationID,
			Text:       fmt.Sprintf("Локация: %s. %s", resp.Event.Name, resp.Event.Description),
			Timestamp:  time.Now(),
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

// getLocationTypeDetails возвращает дополнительные детали визуализации на основе типа локации
func getLocationTypeDetails(name, description string) string {
	nameLower := strings.ToLower(name)
	descLower := strings.ToLower(description)

	// Леса и природные зоны
	if strings.Contains(nameLower, "лес") || strings.Contains(descLower, "лес") {
		if strings.Contains(nameLower, "древн") || strings.Contains(descLower, "древн") {
			return "древний мистический лес, огромные вековые деревья с магическими рунами, густой туман у корней, таинственный зеленоватый свет, скрытые тропы, атмосфера древней магии"
		} else if strings.Contains(nameLower, "эльф") || strings.Contains(descLower, "эльф") {
			return "таинственный эльфийский лес, стройные серебристые деревья, мягкий мох, солнечные лучи сквозь листву, гармония природы, элегантные формы, спокойная атмосфера"
		} else {
			return "густой лес, разнообразные деревья, подлесок, лесная подстилка, естественное освещение, дикая природа, звуки леса"
		}
	}

	// Горы и скалы
	if strings.Contains(nameLower, "гор") || strings.Contains(descLower, "гор") {
		return "величественные горы, острые пики, скалистые склоны, каменные уступы, глубокие ущелья, драматическое освещение, ощущение мощи природы"
	}

	// Замки и крепости
	if strings.Contains(nameLower, "замок") || strings.Contains(nameLower, "крепост") || strings.Contains(descLower, "замок") {
		if strings.Contains(nameLower, "древн") || strings.Contains(descLower, "разрушен") {
			return "древний разрушенный замок, каменные руины, плющ на стенах, обвалившиеся башни, таинственная атмосфера, следы былого величия"
		} else {
			return "могучий средневековый замок, высокие башни, толстые стены, бойницы, подъемный мост, флаги, атмосфера силы и защиты"
		}
	}

	// Подземелья и пещеры
	if strings.Contains(nameLower, "пещер") || strings.Contains(nameLower, "подзем") || strings.Contains(descLower, "пещер") {
		return "темная пещера, неровные стены, сталактиты и сталагмиты, факелы или магический свет, влажность, атмосфера тайны и опасности"
	}

	// Храмы и святыни
	if strings.Contains(nameLower, "храм") || strings.Contains(nameLower, "свят") || strings.Contains(descLower, "храм") {
		return "величественный храм, высокие колонны, священные символы, алтарь, витражи или магические окна, атмосфера святости и спокойствия"
	}

	// Деревни и города
	if strings.Contains(nameLower, "деревн") || strings.Contains(descLower, "деревн") {
		return "уютная деревня, деревянные дома, соломенные крыши, тропинки, заборы, домашние животные, атмосфера спокойствия и повседневности"
	}

	if strings.Contains(nameLower, "город") || strings.Contains(descLower, "город") {
		return "средневековый город, каменные здания, узкие улочки, рынок, жители, атмосфера жизни и торговли"
	}

	// Пустыни и степи
	if strings.Contains(nameLower, "пустын") || strings.Contains(descLower, "пустын") {
		return "пустынная местность, песчаные дюны, палящее солнце, редкая растительность, ощущение простора и суровости"
	}

	// Озера и реки
	if strings.Contains(nameLower, "озер") || strings.Contains(nameLower, "рек") || strings.Contains(descLower, "озер") {
		return "спокойное озеро или река, отражающаяся вода, береговая линия, растительность, атмосфера мира и красоты природы"
	}

	return "фэнтези локация, атмосферное освещение, детализированная среда, богатые текстуры, классический стиль фэнтези-арта"
}

// generateLocationScenario генерирует сценарий для локации с помощью LLM
func (uc *AnalyzeDMResponseUseCase) generateLocationScenario(location world.Location) (*world.LocationScenario, error) {
	if uc.llm == nil {
		// Fallback: генерируем простой сценарий без LLM
		return uc.generateSimpleLocationScenario(location), nil
	}

	prompt := fmt.Sprintf(`Создай увлекательный сценарий для локации в D&D 5e.

Локация: %s
Описание: %s

Сценарий должен включать:
- Название сценария
- Краткое описание ситуации
- Конкретную цель для игрока
- 3-5 ключевых событий
- 2-3 возможных исхода
- Награду за успешное завершение

Формат ответа (ТОЛЬКО JSON):
{
  "title": "Название сценария",
  "description": "Подробное описание ситуации",
  "objective": "Цель игрока",
  "key_events": ["Событие 1", "Событие 2", "Событие 3"],
  "possible_outcomes": ["Исход 1", "Исход 2"],
  "reward": "Описание награды"
}`, location.Name, location.Description)

	llmCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	raw, err := uc.llm.GenerateWithMaxTokens(llmCtx, prompt, 1000)
	if err != nil {
		log.Printf("[DM Analyzer] Failed to generate scenario with LLM: %v, using simple fallback", err)
		return uc.generateSimpleLocationScenario(location), nil
	}

	// Парсим JSON ответ
	var scenario world.LocationScenario
	cleaned := strings.TrimSpace(strings.Trim(raw, "```json"))
	if err := json.Unmarshal([]byte(cleaned), &scenario); err != nil {
		log.Printf("[DM Analyzer] Failed to parse scenario JSON: %v, using simple fallback", err)
		return uc.generateSimpleLocationScenario(location), nil
	}

	// Заполняем обязательные поля
	scenario.ID = fmt.Sprintf("scenario_%d_%d", location.ID, time.Now().Unix())
	scenario.Status = "not_started"
	scenario.CreatedAt = time.Now().Format(time.RFC3339)

	log.Printf("[DM Analyzer] Generated scenario for location %s: %s", location.Name, scenario.Title)
	return &scenario, nil
}

// generateSimpleLocationScenario создает простой сценарий без LLM
func (uc *AnalyzeDMResponseUseCase) generateSimpleLocationScenario(location world.Location) *world.LocationScenario {
	name := location.Name

	// Определяем тип сценария на основе названия
	var title, description, objective, reward string
	var keyEvents, possibleOutcomes []string

	if strings.Contains(strings.ToLower(name), "замок") || strings.Contains(strings.ToLower(name), "крепост") {
		title = "Тайна древнего замка"
		description = fmt.Sprintf("В локации %s скрываются древние секреты и опасности. Замок хранит память о былых временах и ждет достойного исследователя.", name)
		objective = "Найти и раскрыть главный секрет замка"
		keyEvents = []string{
			"Исследовать главный зал",
			"Встретить стража замка",
			"Решить древнюю загадку",
			"Противостоять защитному механизму",
		}
		possibleOutcomes = []string{
			"Раскрыть секрет и получить древний артефакт",
			"Быть изгнанным стражами замка",
			"Найти лишь часть истины",
		}
		reward = "Древний артефакт с магическими свойствами"
	} else if strings.Contains(strings.ToLower(name), "пещер") || strings.Contains(strings.ToLower(name), "гробниц") {
		title = "Сокровища подземного мира"
		description = fmt.Sprintf("Локация %s таит в себе богатства и опасности подземного мира. Темные коридоры хранят секреты давно ушедших эпох.", name)
		objective = "Найти и добыть ценный артефакт"
		keyEvents = []string{
			"Преодолеть охрану входа",
			"Исследовать основные коридоры",
			"Решить механизм защиты",
			"Противостоять стражу сокровищ",
		}
		possibleOutcomes = []string{
			"Получить ценный артефакт и богатства",
			"Быть побежденным стражами",
			"Найти лишь мелкие сокровища",
		}
		reward = "Ценный артефакт и золотые монеты"
	} else if strings.Contains(strings.ToLower(name), "лес") {
		title = "Тайны древнего леса"
		description = fmt.Sprintf("Локация %s полна жизни и тайн природы. Деревья хранят древние знания, а тропы ведут к скрытым сокровищам.", name)
		objective = "Найти и защитить священное место леса"
		keyEvents = []string{
			"Найти следы древних ритуалов",
			"Встретить хранителя леса",
			"Решить природную загадку",
			"Защитить священное место",
		}
		possibleOutcomes = []string{
			"Получить благословение природы",
			"Быть изгнанным из леса",
			"Найти лишь частичные знания",
		}
		reward = "Благословение природы и магические растения"
	} else {
		// Общий сценарий для остальных локаций
		title = fmt.Sprintf("Тайна локации %s", name)
		description = fmt.Sprintf("Локация %s хранит свои секреты и ждет исследователя. Здесь можно найти как сокровища, так и опасности.", name)
		objective = "Исследовать локацию и раскрыть её тайну"
		keyEvents = []string{
			"Осмотреть окрестности",
			"Найти ключевые объекты",
			"Встретить местных обитателей",
			"Решить основную загадку",
		}
		possibleOutcomes = []string{
			"Раскрыть тайну и получить награду",
			"Не справиться с вызовами",
			"Найти частичные ответы",
		}
		reward = "Ценный предмет или информация"
	}

	return &world.LocationScenario{
		ID:               fmt.Sprintf("simple_scenario_%d_%d", location.ID, time.Now().Unix()),
		Title:            title,
		Description:      description,
		Objective:        objective,
		KeyEvents:        keyEvents,
		PossibleOutcomes: possibleOutcomes,
		Reward:           reward,
		Status:           "not_started",
		CreatedAt:        time.Now().Format(time.RFC3339),
	}
}

// getNPCTypeDetails возвращает дополнительные детали внешности на основе типа NPC
func getNPCTypeDetails(name, description string) string {
	nameLower := strings.ToLower(name)
	descLower := strings.ToLower(description)

	// Эльфы
	if strings.Contains(nameLower, "эльф") || strings.Contains(descLower, "эльф") {
		if strings.Contains(nameLower, "темн") || strings.Contains(descLower, "темн") {
			return "темный эльф (дроу), светло-серая или черная кожа, белые волосы, красные глаза, элегантные черты лица, острые уши, темная кожаная одежда с серебряными украшениями"
		} else if strings.Contains(nameLower, "лесн") || strings.Contains(descLower, "лесн") {
			return "лесной эльф, светлая кожа, зеленые или каштановые волосы, зеленые глаза, татуировки листьев, одежда из натуральных тканей, венок из цветов"
		} else {
			return "высокий эльф, бледная кожа, золотистые или серебристые волосы, яркие глаза, острые черты лица, элегантная одежда, грациозная осанка"
		}
	}

	// Люди
	if strings.Contains(nameLower, "человек") || strings.Contains(descLower, "человек") || (!strings.Contains(nameLower, "эльф") && !strings.Contains(nameLower, "гном") && !strings.Contains(nameLower, "орк")) {
		// Определяем по профессии или характеристикам
		if strings.Contains(nameLower, "воин") || strings.Contains(descLower, "воин") || strings.Contains(nameLower, "рыцар") {
			return "сильный воин, мускулистое телосложение, шрамы, доспехи, уверенная поза, решительное выражение лица"
		} else if strings.Contains(nameLower, "маг") || strings.Contains(descLower, "маг") || strings.Contains(nameLower, "волшебник") {
			return "мудрый маг, длинная борода, мантия, посох, сосредоточенное выражение, магические символы на одежде"
		} else if strings.Contains(nameLower, "торгов") || strings.Contains(descLower, "торгов") {
			return "купец, практичная одежда, сумки с товарами, добродушное выражение, уверенная поза"
		} else if strings.Contains(nameLower, "трактирщик") || strings.Contains(descLower, "трактир") {
			return "трактирщик, крепкое телосложение, фартук, дружелюбное лицо, руки в муке или с кружкой"
		} else {
			return "обычный человек средних лет, практичная одежда, нормальное телосложение, нейтральное выражение лица"
		}
	}

	// Гномы
	if strings.Contains(nameLower, "гном") || strings.Contains(descLower, "гном") {
		return "низкорослый крепкий гном, рыжая или серая борода, рабочая одежда или доспехи, инструменты, решительное выражение лица"
	}

	// Орки
	if strings.Contains(nameLower, "орк") || strings.Contains(descLower, "орк") {
		return "массивный орк, зеленая кожа, клыки, боевые шрамы, грубая одежда или доспехи, свирепое выражение"
	}

	// Драконы или драконьи существа
	if strings.Contains(nameLower, "дракон") || strings.Contains(descLower, "дракон") {
		return "величавое драконье существо, чешуя, крылья, огненные глаза, величественная поза, магическая аура"
	}

	// Некроманты или темные маги
	if strings.Contains(nameLower, "некромант") || strings.Contains(descLower, "некромант") || strings.Contains(nameLower, "темн") {
		return "зловещий некромант, бледная кожа, темная мантия, магические артефакты, холодный взгляд, атмосфера смерти"
	}

	// Священнослужители
	if strings.Contains(nameLower, "жрец") || strings.Contains(nameLower, "священ") || strings.Contains(descLower, "жрец") {
		return "благочестивый священнослужитель, символ веры, мантия, спокойное выражение лица, атмосфера святости"
	}

	// По умолчанию
	return "персонаж фэнтези, детализированные черты лица, выразительное выражение, подходящая одежда и аксессуары, профессиональный портрет"
}

// recognizeNaturalAbilityChecks распознает естественные запросы проверок способностей в ответе DM
// и автоматически устанавливает pending проверки
func (uc *AnalyzeDMResponseUseCase) recognizeNaturalAbilityChecks(
	ctx context.Context,
	dmResponse string,
	analysis *DMResponseAnalysis,
) error {
	// Проверяем доступность репозитория полной сессии
	if uc.fullSessionRepo == nil {
		return nil // Репозиторий не настроен, пропускаем
	}

	// Проверяем, есть ли уже pending проверка
	gs, err := uc.fullSessionRepo.GetByChatID(ctx, uc.chatID)
	if err != nil {
		return fmt.Errorf("failed to get session for natural check: %w", err)
	}
	if gs == nil {
		return fmt.Errorf("session not found for natural check")
	}

	if gs.HasPendingAbilityCheck() {
		// Уже есть pending проверка, не устанавливаем новую
		return nil
	}

	// Ищем паттерны естественных запросов проверок
	checkRequest := uc.parseNaturalCheckRequest(dmResponse)
	if checkRequest == nil {
		// Нет запроса проверки в естественном языке
		return nil
	}

	// P2: не повторяем одну и ту же проверку в рамках текущей сцены (локации).
	if gs.IsAbilityCheckRepeatedInScene(checkRequest.Ability) {
		return nil
	}

	// Генерируем уникальный ID для проверки
	checkID := fmt.Sprintf("natural_%s_%d", checkRequest.Ability, time.Now().Unix())

	// Извлекаем причину и ставку из текста DM
	reason, stakes := uc.extractCheckContext(dmResponse)

	// Устанавливаем pending проверку с контекстом
	gs.SetPendingAbilityCheckWithContext(checkID, checkRequest.Ability, checkRequest.DC, reason, stakes)

	// Сохраняем сессию
	if err := uc.fullSessionRepo.Save(ctx, gs); err != nil {
		return fmt.Errorf("failed to save session with natural check: %w", err)
	}

	log.Printf("[DM Analyzer] Recognized natural ability check: %s DC %d (reason: %s, stakes: %s)",
		checkRequest.Ability, checkRequest.DC, reason, stakes)

	return nil
}

// NaturalCheckRequest представляет распознанный естественный запрос проверки
type NaturalCheckRequest struct {
	Ability string
	DC      int
}

// parseNaturalCheckRequest анализирует текст на предмет естественных запросов проверок
func (uc *AnalyzeDMResponseUseCase) parseNaturalCheckRequest(text string) *NaturalCheckRequest {
	text = strings.ToLower(text)

	// Паттерны для распознавания проверок
	patterns := map[string]string{
		// Сила
		"проверка силы":         "strength",
		"проверьте силу":        "strength",
		"проверка на силу":      "strength",
		"бросьте проверку силы": "strength",
		"проверка силы (str)":   "strength",
		"сила (str)":            "strength",

		// Ловкость
		"проверка ловкости":         "dexterity",
		"проверьте ловкость":        "dexterity",
		"проверка на ловкость":      "dexterity",
		"бросьте проверку ловкости": "dexterity",
		"проверка ловкости (dex)":   "dexterity",
		"ловкость (dex)":            "dexterity",

		// Телосложение
		"проверка телосложения":         "constitution",
		"проверьте телосложение":        "constitution",
		"проверка на телосложение":      "constitution",
		"бросьте проверку телосложения": "constitution",
		"проверка телосложения (con)":   "constitution",
		"телосложение (con)":            "constitution",

		// Интеллект
		"проверка интеллекта":         "intelligence",
		"проверьте интеллект":         "intelligence",
		"проверка на интеллект":       "intelligence",
		"бросьте проверку интеллекта": "intelligence",
		"проверка интеллекта (int)":   "intelligence",
		"интеллект (int)":             "intelligence",

		// Мудрость
		"проверка мудрости":         "wisdom",
		"проверьте мудрость":        "wisdom",
		"проверка на мудрость":      "wisdom",
		"бросьте проверку мудрости": "wisdom",
		"проверка мудрости (wis)":   "wisdom",
		"мудрость (wis)":            "wisdom",
		"проверка восприятия":       "wisdom", // Восприятие = мудрость

		// Харизма
		"проверка харизмы":         "charisma",
		"проверьте харизму":        "charisma",
		"проверка на харизму":      "charisma",
		"бросьте проверку харизмы": "charisma",
		"проверка харизмы (cha)":   "charisma",
		"харизма (cha)":            "charisma",
	}

	// Ищем совпадения с паттернами
	for pattern, ability := range patterns {
		if strings.Contains(text, pattern) {
			// Извлекаем DC из текста
			dc := uc.extractDCFromText(text)
			if dc == 0 {
				dc = 10 // DC по умолчанию, если не указан
			}

			return &NaturalCheckRequest{
				Ability: ability,
				DC:      dc,
			}
		}
	}

	return nil
}

// extractDCFromText извлекает значение DC из текста
func (uc *AnalyzeDMResponseUseCase) extractDCFromText(text string) int {
	// Ищем паттерны вроде "DC 15", "сложность 12", "dc15", "DC=10"
	re := regexp.MustCompile(`(?:dc|сложность)\s*[:=]?\s*(\d{1,2})`)
	matches := re.FindStringSubmatch(strings.ToLower(text))
	if len(matches) >= 2 {
		if dc, err := strconv.Atoi(matches[1]); err == nil && dc >= 1 && dc <= 30 {
			return dc
		}
	}
	return 0
}

// extractCheckContext извлекает причину и ставку проверки из текста DM
func (uc *AnalyzeDMResponseUseCase) extractCheckContext(text string) (reason, stakes string) {
	text = strings.ToLower(text)

	// Ищем причину (reason) - текст после слов "чтобы", "для того чтобы"
	if idx := strings.Index(text, "чтобы"); idx != -1 {
		reason = strings.TrimSpace(text[idx+5:])
		// Ограничиваем длину
		if len(reason) > 100 {
			reason = reason[:97] + "..."
		}
	} else if idx := strings.Index(text, "для того чтобы"); idx != -1 {
		reason = strings.TrimSpace(text[idx+14:])
		if len(reason) > 100 {
			reason = reason[:97] + "..."
		}
	}

	// Ищем ставку (stakes) - текст после слов "ставка", "на кону", "риск"
	stakesKeywords := []string{"ставка", "на кону", "риск", "последствия"}
	for _, keyword := range stakesKeywords {
		if idx := strings.Index(text, keyword); idx != -1 {
			// Берем текст после ключевого слова
			stakes = strings.TrimSpace(text[idx+len(keyword):])
			// Убираем возможные ":" или "="
			stakes = strings.TrimPrefix(stakes, ":")
			stakes = strings.TrimPrefix(stakes, "=")
			stakes = strings.TrimSpace(stakes)

			// Ограничиваем длину
			if len(stakes) > 100 {
				stakes = stakes[:97] + "..."
			}
			break
		}
	}

	return reason, stakes
}
