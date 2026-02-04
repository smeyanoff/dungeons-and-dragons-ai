package telegram

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	abilitycheck "dungeons-and-dragons-ai/internal/game/application/ability_check"
	achievementapp "dungeons-and-dragons-ai/internal/game/application/achievement"
	"dungeons-and-dragons-ai/internal/game/application/campaign"
	characterapp "dungeons-and-dragons-ai/internal/game/application/character"
	combatapp "dungeons-and-dragons-ai/internal/game/application/combat"
	"dungeons-and-dragons-ai/internal/game/application/dice"
	dm_tools "dungeons-and-dragons-ai/internal/game/application/dm_tools"
	"dungeons-and-dragons-ai/internal/game/application/history"
	imageapp "dungeons-and-dragons-ai/internal/game/application/image"
	inventoryapp "dungeons-and-dragons-ai/internal/game/application/inventory"
	"dungeons-and-dragons-ai/internal/game/application/player_action"
	questapp "dungeons-and-dragons-ai/internal/game/application/quest"
	ratingapp "dungeons-and-dragons-ai/internal/game/application/rating"
	sessionapp "dungeons-and-dragons-ai/internal/game/application/session"
	spellapp "dungeons-and-dragons-ai/internal/game/application/spell"
	subscriptionapp "dungeons-and-dragons-ai/internal/game/application/subscription"
	mapapp "dungeons-and-dragons-ai/internal/game/application/worldmap"
	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/combat"
	"dungeons-and-dragons-ai/internal/game/domain/event"
	"dungeons-and-dragons-ai/internal/game/domain/feedback"
	"dungeons-and-dragons-ai/internal/game/domain/player"
	"dungeons-and-dragons-ai/internal/game/domain/rating"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/game/domain/subscription"
	"dungeons-and-dragons-ai/internal/game/domain/world"
	"dungeons-and-dragons-ai/internal/game/infrastructure/persistence"
	ragdomain "dungeons-and-dragons-ai/internal/rag/domain"
	"dungeons-and-dragons-ai/pkg/logger"

	"github.com/google/uuid"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	// TelegramMaxMessageLength максимальная длина сообщения в Telegram
	TelegramMaxMessageLength = 4096
	// TelegramSafeMessageLength безопасная длина для разбиения сообщений (с запасом для форматирования)
	TelegramSafeMessageLength = 4000
)

type Bot struct {
	api                   *tgbotapi.BotAPI
	initCampaignUC        *campaign.InitCampaignUseCase
	handleActionUC        *player_action.HandleActionUseCase
	createCharacterUC     *characterapp.CreateCharacterUseCase
	getHistoryUC          *history.GetHistoryUseCase
	getInventoryUC        *inventoryapp.GetInventoryUseCase
	addItemUC             *inventoryapp.AddItemUseCase
	handleCombatUC        *combatapp.HandleCombatUseCase
	rollDiceUC            *dice.RollDiceUseCase
	getQuestsUC           *questapp.GetQuestsUseCase
	getDailyQuestsUC      *questapp.GetDailyQuestsUseCase
	checkDailyProgressUC  *questapp.CheckDailyQuestProgressUseCase
	getMapUC              *mapapp.GetMapUseCase
	moveToLocationUC      *mapapp.MoveToLocationUseCase
	getAchievementsUC     *achievementapp.GetAchievementsUseCase
	getSpellsUC           *spellapp.GetSpellsUseCase
	useSpellUC            *spellapp.UseSpellUseCase
	generateImageUC       *imageapp.ImageGenerationUseCase
	getSubscriptionUC     *subscriptionapp.GetSubscriptionUseCase
	checkLimitsUC         *subscriptionapp.CheckLimitsUseCase
	getLeaderboardUC      *ratingapp.GetLeaderboardUseCase
	updateRatingUC        *ratingapp.UpdateRatingUseCase
	performAbilityCheckUC *abilitycheck.PerformAbilityCheckUseCase
	sessionRepo           session.Repository
	playerRepo            *persistence.PlayerRepository
	combatRepo            CombatRepository
	feedbackRepo          FeedbackRepository
	eventRepo             EventRepository      // Для сохранения результатов бросков в историю
	indexDocUC            IndexDocumentUseCase // Для индексации результатов бросков в RAG

	// Для улучшенной обработки ошибок Telegram API
	errorCount    int // Счетчик последовательных ошибок
	errorCountMu  sync.Mutex
	lastErrorTime time.Time
	circuitOpen   bool // Circuit breaker состояние
	circuitOpenMu sync.RWMutex

	// Состояние диалога feedback (chatID -> состояние)
	feedbackState   map[int64]*FeedbackDialogState
	feedbackStateMu sync.RWMutex

	// Health check состояние
	lastHealthCheck time.Time
	healthCheckMu   sync.RWMutex
}

// FeedbackDialogState состояние диалога feedback
type FeedbackDialogState struct {
	Type     feedback.FeedbackType
	Category feedback.FeedbackCategory
	UserID   int64
	From     *tgbotapi.User
}

// FeedbackRepository интерфейс для работы с фидбеком
type FeedbackRepository interface {
	Save(ctx context.Context, fb *feedback.Feedback) error
}

// EventRepository интерфейс для работы с событиями игры
type EventRepository interface {
	Save(ctx context.Context, e *event.StoryEvent) error
}

// IndexDocumentUseCase интерфейс для индексации документов в RAG
type IndexDocumentUseCase interface {
	Execute(ctx context.Context, doc ragdomain.Document) error
}

// CombatRepository интерфейс для работы с боем
type CombatRepository interface {
	GetActiveBySessionID(ctx context.Context, sessionID uint) (*combat.Combat, error)
	Save(ctx context.Context, c *combat.Combat) error
}

func NewBot(
	token string,
	initCampaignUC *campaign.InitCampaignUseCase,
	handleActionUC *player_action.HandleActionUseCase,
	createCharacterUC *characterapp.CreateCharacterUseCase,
	getHistoryUC *history.GetHistoryUseCase,
	getInventoryUC *inventoryapp.GetInventoryUseCase,
	addItemUC *inventoryapp.AddItemUseCase,
	handleCombatUC *combatapp.HandleCombatUseCase,
	rollDiceUC *dice.RollDiceUseCase,
	getQuestsUC *questapp.GetQuestsUseCase,
	getDailyQuestsUC *questapp.GetDailyQuestsUseCase,
	checkDailyProgressUC *questapp.CheckDailyQuestProgressUseCase,
	getMapUC *mapapp.GetMapUseCase,
	moveToLocationUC *mapapp.MoveToLocationUseCase,
	getAchievementsUC *achievementapp.GetAchievementsUseCase,
	getSpellsUC *spellapp.GetSpellsUseCase,
	useSpellUC *spellapp.UseSpellUseCase,
	generateImageUC *imageapp.ImageGenerationUseCase,
	getSubscriptionUC *subscriptionapp.GetSubscriptionUseCase,
	checkLimitsUC *subscriptionapp.CheckLimitsUseCase,
	getLeaderboardUC *ratingapp.GetLeaderboardUseCase,
	updateRatingUC *ratingapp.UpdateRatingUseCase,
	performAbilityCheckUC *abilitycheck.PerformAbilityCheckUseCase,
	sessionRepo session.Repository,
	playerRepo *persistence.PlayerRepository,
	combatRepo CombatRepository,
	feedbackRepo FeedbackRepository,
	eventRepo EventRepository,
	indexDocUC IndexDocumentUseCase,
) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("failed to create bot: %w", err)
	}

	// Configure HTTP client for better connection pooling and stability
	if httpClient, ok := api.Client.(*http.Client); ok {
		if httpClient.Transport == nil {
			transport := &http.Transport{
				MaxIdleConns:          100,
				IdleConnTimeout:       90 * time.Second,
				MaxIdleConnsPerHost:   10,
				ResponseHeaderTimeout: 30 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
				DisableKeepAlives:     false,
			}
			httpClient.Transport = transport
		}
		httpClient.Timeout = 60 * time.Second // Consistent with polling timeout
	}

	return newBotWithAPI(
		api,
		initCampaignUC,
		handleActionUC,
		createCharacterUC,
		getHistoryUC,
		getInventoryUC,
		addItemUC,
		handleCombatUC,
		rollDiceUC,
		getQuestsUC,
		getDailyQuestsUC,
		checkDailyProgressUC,
		getMapUC,
		moveToLocationUC,
		getAchievementsUC,
		getSpellsUC,
		useSpellUC,
		generateImageUC,
		getSubscriptionUC,
		checkLimitsUC,
		getLeaderboardUC,
		updateRatingUC,
		performAbilityCheckUC,
		sessionRepo,
		playerRepo,
		combatRepo,
		feedbackRepo,
		eventRepo,
		indexDocUC,
	)
}

// NewBotWithAPIEndpoint создаёт бота, используя кастомный Telegram API endpoint.
// Нужен для интеграционных тестов (например, с httptest server) и не влияет на прод-инициализацию.
//
// apiEndpoint должен иметь формат как tgbotapi.APIEndpoint (с %s placeholders), например:
//
//	http://127.0.0.1:12345/bot%s/%s
func NewBotWithAPIEndpoint(
	token string,
	apiEndpoint string,
	initCampaignUC *campaign.InitCampaignUseCase,
	handleActionUC *player_action.HandleActionUseCase,
	createCharacterUC *characterapp.CreateCharacterUseCase,
	getHistoryUC *history.GetHistoryUseCase,
	getInventoryUC *inventoryapp.GetInventoryUseCase,
	addItemUC *inventoryapp.AddItemUseCase,
	handleCombatUC *combatapp.HandleCombatUseCase,
	rollDiceUC *dice.RollDiceUseCase,
	getQuestsUC *questapp.GetQuestsUseCase,
	getDailyQuestsUC *questapp.GetDailyQuestsUseCase,
	checkDailyProgressUC *questapp.CheckDailyQuestProgressUseCase,
	getMapUC *mapapp.GetMapUseCase,
	moveToLocationUC *mapapp.MoveToLocationUseCase,
	getAchievementsUC *achievementapp.GetAchievementsUseCase,
	getSpellsUC *spellapp.GetSpellsUseCase,
	useSpellUC *spellapp.UseSpellUseCase,
	generateImageUC *imageapp.ImageGenerationUseCase,
	getSubscriptionUC *subscriptionapp.GetSubscriptionUseCase,
	checkLimitsUC *subscriptionapp.CheckLimitsUseCase,
	getLeaderboardUC *ratingapp.GetLeaderboardUseCase,
	updateRatingUC *ratingapp.UpdateRatingUseCase,
	performAbilityCheckUC *abilitycheck.PerformAbilityCheckUseCase,
	sessionRepo session.Repository,
	playerRepo *persistence.PlayerRepository,
	combatRepo CombatRepository,
	feedbackRepo FeedbackRepository,
	eventRepo EventRepository,
	indexDocUC IndexDocumentUseCase,
) (*Bot, error) {
	api, err := tgbotapi.NewBotAPIWithAPIEndpoint(token, apiEndpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to create bot: %w", err)
	}

	return newBotWithAPI(
		api,
		initCampaignUC,
		handleActionUC,
		createCharacterUC,
		getHistoryUC,
		getInventoryUC,
		addItemUC,
		handleCombatUC,
		rollDiceUC,
		getQuestsUC,
		getDailyQuestsUC,
		checkDailyProgressUC,
		getMapUC,
		moveToLocationUC,
		getAchievementsUC,
		getSpellsUC,
		useSpellUC,
		generateImageUC,
		getSubscriptionUC,
		checkLimitsUC,
		getLeaderboardUC,
		updateRatingUC,
		performAbilityCheckUC,
		sessionRepo,
		playerRepo,
		combatRepo,
		feedbackRepo,
		eventRepo,
		indexDocUC,
	)
}

func newBotWithAPI(
	api *tgbotapi.BotAPI,
	initCampaignUC *campaign.InitCampaignUseCase,
	handleActionUC *player_action.HandleActionUseCase,
	createCharacterUC *characterapp.CreateCharacterUseCase,
	getHistoryUC *history.GetHistoryUseCase,
	getInventoryUC *inventoryapp.GetInventoryUseCase,
	addItemUC *inventoryapp.AddItemUseCase,
	handleCombatUC *combatapp.HandleCombatUseCase,
	rollDiceUC *dice.RollDiceUseCase,
	getQuestsUC *questapp.GetQuestsUseCase,
	getDailyQuestsUC *questapp.GetDailyQuestsUseCase,
	checkDailyProgressUC *questapp.CheckDailyQuestProgressUseCase,
	getMapUC *mapapp.GetMapUseCase,
	moveToLocationUC *mapapp.MoveToLocationUseCase,
	getAchievementsUC *achievementapp.GetAchievementsUseCase,
	getSpellsUC *spellapp.GetSpellsUseCase,
	useSpellUC *spellapp.UseSpellUseCase,
	generateImageUC *imageapp.ImageGenerationUseCase,
	getSubscriptionUC *subscriptionapp.GetSubscriptionUseCase,
	checkLimitsUC *subscriptionapp.CheckLimitsUseCase,
	getLeaderboardUC *ratingapp.GetLeaderboardUseCase,
	updateRatingUC *ratingapp.UpdateRatingUseCase,
	performAbilityCheckUC *abilitycheck.PerformAbilityCheckUseCase,
	sessionRepo session.Repository,
	playerRepo *persistence.PlayerRepository,
	combatRepo CombatRepository,
	feedbackRepo FeedbackRepository,
	eventRepo EventRepository,
	indexDocUC IndexDocumentUseCase,
) (*Bot, error) {
	bot := &Bot{
		api:                   api,
		initCampaignUC:        initCampaignUC,
		handleActionUC:        handleActionUC,
		createCharacterUC:     createCharacterUC,
		getHistoryUC:          getHistoryUC,
		getInventoryUC:        getInventoryUC,
		addItemUC:             addItemUC,
		handleCombatUC:        handleCombatUC,
		rollDiceUC:            rollDiceUC,
		getQuestsUC:           getQuestsUC,
		getDailyQuestsUC:      getDailyQuestsUC,
		checkDailyProgressUC:  checkDailyProgressUC,
		getMapUC:              getMapUC,
		moveToLocationUC:      moveToLocationUC,
		getAchievementsUC:     getAchievementsUC,
		getSpellsUC:           getSpellsUC,
		useSpellUC:            useSpellUC,
		generateImageUC:       generateImageUC,
		getSubscriptionUC:     getSubscriptionUC,
		checkLimitsUC:         checkLimitsUC,
		getLeaderboardUC:      getLeaderboardUC,
		updateRatingUC:        updateRatingUC,
		performAbilityCheckUC: performAbilityCheckUC,
		sessionRepo:           sessionRepo,
		playerRepo:            playerRepo,
		combatRepo:            combatRepo,
		feedbackRepo:          feedbackRepo,
		eventRepo:             eventRepo,
		indexDocUC:            indexDocUC,
		feedbackState:         make(map[int64]*FeedbackDialogState),
	}

	// Настраиваем HTTP клиент с connection pooling для улучшения стабильности polling
	bot.configureHTTPClient()

	// Настраиваем Bot Commands Menu для отображения команд в интерфейсе Telegram
	if err := bot.setupBotCommands(); err != nil {
		logger.Warn("Failed to setup bot commands menu",
			logger.ErrorField(err),
		)
		// Не возвращаем ошибку, так как это не критично для работы бота
	}

	return bot, nil
}

// HandleUpdate — экспортируемая обертка над внутренней обработкой апдейтов.
// Полезно для интеграционных тестов, где мы хотим прогонять сценарии "как в Telegram",
// но без запуска polling-цикла b.Start().
func (b *Bot) HandleUpdate(ctx context.Context, update tgbotapi.Update) error {
	return b.handleUpdate(ctx, update)
}

// setupBotCommands настраивает Bot Commands Menu в Telegram
func (b *Bot) setupBotCommands() error {
	commands := []tgbotapi.BotCommand{
		{Command: "start", Description: "Начать работу с ботом"},
		{Command: "help", Description: "Показать справку по командам"},
		{Command: "newgame", Description: "Начать новую игру"},
		{Command: "endgame", Description: "Завершить игру"},
		{Command: "createcharacter", Description: "Создать персонажа"},
		{Command: "character", Description: "Информация о персонаже"},
		{Command: "inventory", Description: "Посмотреть инвентарь"},
		{Command: "pickup", Description: "Подобрать предмет"},
		{Command: "party", Description: "Управление отрядом"},
		{Command: "dismiss", Description: "Уволить компаньона"},
		{Command: "attack", Description: "Атаковать противника"},
		{Command: "battlefield", Description: "Показать поле боя"},
		{Command: "abilities", Description: "Способности персонажа"},
		{Command: "spells", Description: "Просмотр заклинаний"},
		{Command: "cast", Description: "Использовать заклинание"},
		{Command: "roll", Description: "Бросить кубик"},
		{Command: "history", Description: "История игры"},
		{Command: "quests", Description: "Активные квесты"},
		{Command: "daily", Description: "Ежедневные задания"},
		{Command: "map", Description: "Карта мира"},
		{Command: "achievements", Description: "Просмотр достижений"},
		{Command: "leaderboard", Description: "Рейтинг игроков"},
		{Command: "preferences", Description: "Настройки стиля"},
		{Command: "set_style", Description: "Изменить стиль повествования"},
		{Command: "set_detail", Description: "Изменить уровень детализации"},
		{Command: "set_language", Description: "Изменить язык"},
		{Command: "toggle_stats", Description: "Переключить статистику"},
		{Command: "wait_until", Description: "Изменить время суток"},
		{Command: "image", Description: "Сгенерировать изображение"},
		{Command: "flee", Description: "Попытаться выйти из боя"},
		{Command: "feedback", Description: "Отправить отзыв о игре"},
		{Command: "subscription", Description: "Информация о подписке"},
		{Command: "subscribe", Description: "Оформить подписку"},
	}

	cmd := tgbotapi.NewSetMyCommands(commands...)
	_, err := b.api.Request(cmd)
	if err != nil {
		return fmt.Errorf("failed to set bot commands: %w", err)
	}

	logger.Info("Bot commands menu configured successfully",
		logger.Int("commands_count", len(commands)),
	)

	return nil
}

// GetAPI возвращает BotAPI для использования в других компонентах (например, для уведомлений)
func (b *Bot) GetAPI() *tgbotapi.BotAPI {
	return b.api
}

func (b *Bot) Start(ctx context.Context) error {
	logger.Info("Bot started",
		logger.String("username", b.api.Self.UserName),
		logger.Int64("bot_id", int64(b.api.Self.ID)),
	)

	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 60 // Увеличиваем timeout для стабильности и уменьшения EOF ошибок

	offset := 0
	backoff := 1 * time.Second
	const maxBackoff = 120 * time.Second // Увеличиваем максимальный backoff для лучшей стабильности
	const logInterval = 60 * time.Second // Увеличиваем интервал логирования
	lastPollLog := time.Time{}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	consecutiveErrors := 0 // Счетчик последовательных ошибок

	for {
		select {
		case <-ctx.Done():
			logger.Info("Bot stopping",
				logger.ErrorField(ctx.Err()),
			)
			return ctx.Err()
		default:
		}

		updateConfig.Offset = offset
		updates, err := b.api.GetUpdates(updateConfig)
		if err != nil {
			consecutiveErrors++
			errStr := err.Error()

			// Классифицируем тип ошибки для лучшей обработки
			isEOFError := strings.Contains(errStr, "unexpected EOF")
			isTimeoutError := strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline exceeded")
			isNetworkError := strings.Contains(errStr, "connection") || strings.Contains(errStr, "network") ||
				strings.Contains(errStr, "reset") || strings.Contains(errStr, "broken pipe")
			isRateLimitError := strings.Contains(errStr, "429") || strings.Contains(errStr, "Too Many Requests")

			// Вычисляем backoff в зависимости от типа ошибки
			var currentBackoff time.Duration
			var backoffMultiplier float64

			if isEOFError {
				// EOF ошибки: быстрый recovery, так как соединение разорвано
				currentBackoff = time.Duration(float64(backoff) * 0.3) // Используем 30% от текущего backoff
				if currentBackoff < 2*time.Second {
					currentBackoff = 2 * time.Second
				}
				backoffMultiplier = 1.2 // Медленный рост для EOF

				if shouldLogPollError(&lastPollLog, 15*time.Second) { // Логируем EOF чаще
					logger.Warn("Telegram polling EOF error (connection interrupted)",
						logger.ErrorField(sanitizeTelegramError(err)),
						logger.String("backoff", currentBackoff.String()),
						logger.Int("consecutive_errors", consecutiveErrors),
					)
				}
			} else if isTimeoutError {
				// Timeout ошибки: умеренный backoff
				currentBackoff = backoff
				backoffMultiplier = 1.5

				if shouldLogPollError(&lastPollLog, 30*time.Second) {
					logger.Warn("Telegram polling timeout error",
						logger.ErrorField(sanitizeTelegramError(err)),
						logger.String("backoff", currentBackoff.String()),
						logger.Int("consecutive_errors", consecutiveErrors),
					)
				}
			} else if isNetworkError {
				// Network ошибки: быстрый recovery с exponential backoff
				currentBackoff = backoff
				backoffMultiplier = 2.0

				if shouldLogPollError(&lastPollLog, 20*time.Second) {
					logger.Warn("Telegram polling network error",
						logger.ErrorField(sanitizeTelegramError(err)),
						logger.String("backoff", currentBackoff.String()),
						logger.Int("consecutive_errors", consecutiveErrors),
					)
				}
			} else if isRateLimitError {
				// Rate limit ошибки: значительный backoff
				currentBackoff = time.Duration(float64(backoff) * 3) // Увеличиваем backoff в 3 раза
				backoffMultiplier = 2.5

				if shouldLogPollError(&lastPollLog, 10*time.Second) {
					logger.Warn("Telegram polling rate limit error",
						logger.ErrorField(sanitizeTelegramError(err)),
						logger.String("backoff", currentBackoff.String()),
						logger.Int("consecutive_errors", consecutiveErrors),
					)
				}
			} else {
				// Другие ошибки: стандартный exponential backoff
				currentBackoff = backoff
				backoffMultiplier = 2.0

				if shouldLogPollError(&lastPollLog, logInterval) {
					logger.Warn("Telegram polling error",
						logger.ErrorField(sanitizeTelegramError(err)),
						logger.String("backoff", currentBackoff.String()),
						logger.Int("consecutive_errors", consecutiveErrors),
					)
				}
			}

			// Применяем jitter для предотвращения thundering herd
			sleepWithJitter(ctx, currentBackoff, rng)

			// Обновляем backoff с учетом типа ошибки
			backoff = time.Duration(float64(backoff) * backoffMultiplier)

			// Ограничиваем backoff максимальным значением
			if backoff > maxBackoff {
				backoff = maxBackoff
			}

			// Сбрасываем счетчик ошибок при достижении максимального backoff
			if backoff >= maxBackoff && consecutiveErrors > 10 {
				logger.Warn("Telegram polling reached maximum backoff, resetting error counter",
					logger.Int("consecutive_errors", consecutiveErrors),
				)
				consecutiveErrors = 0
			}

			continue
		}

		// Успешное получение обновлений - сбрасываем backoff и счетчики ошибок
		backoff = 1 * time.Second
		consecutiveErrors = 0

		backoff = 1 * time.Second
		for _, update := range updates {
			if update.UpdateID >= offset {
				offset = update.UpdateID + 1
			}
			if err := b.handleUpdate(ctx, update); err != nil {
				// Логируем только после нескольких неудачных попыток подряд
				b.errorCountMu.Lock()
				b.errorCount++
				shouldLog := b.errorCount >= 3 // Логируем после 3 ошибок подряд
				b.errorCountMu.Unlock()

				if shouldLog {
					logger.Error("Error handling update (multiple consecutive errors)",
						logger.ErrorField(err),
						logger.Int("update_id", update.UpdateID),
						logger.Int("consecutive_errors", b.errorCount),
					)
				} else {
					logger.Debug("Error handling update (suppressed logging)",
						logger.ErrorField(err),
						logger.Int("update_id", update.UpdateID),
					)
				}
			} else {
				// Сбрасываем счетчик ошибок при успешной обработке
				b.errorCountMu.Lock()
				if b.errorCount > 0 {
					b.errorCount = 0
				}
				b.errorCountMu.Unlock()
			}
		}
	}
}

func shouldLogPollError(lastLog *time.Time, interval time.Duration) bool {
	if lastLog == nil {
		return true
	}
	now := time.Now()
	if lastLog.IsZero() || now.Sub(*lastLog) >= interval {
		*lastLog = now
		return true
	}
	return false
}

func sleepWithJitter(ctx context.Context, base time.Duration, rng *rand.Rand) {
	if base <= 0 {
		return
	}
	jitterFactor := 0.5 + rng.Float64()
	sleep := time.Duration(float64(base) * jitterFactor)
	select {
	case <-ctx.Done():
	case <-time.After(sleep):
	}
}

// getMapKeys возвращает ключи map[string]interface{} для логирования
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func isDuplicateChatIDError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "duplicate key value violates unique constraint") &&
		strings.Contains(msg, "idx_game_sessions_chat_id")
}

// configureHTTPClient настраивает HTTP клиент с connection pooling для улучшения стабильности polling
func (b *Bot) configureHTTPClient() {
	// Создаем HTTP transport с настройками connection pooling
	transport := &http.Transport{
		MaxIdleConns:        100,              // Максимальное количество idle соединений
		MaxIdleConnsPerHost: 10,               // Максимальное количество idle соединений на хост
		MaxConnsPerHost:     20,               // Максимальное количество соединений на хост
		IdleConnTimeout:     90 * time.Second, // Таймаут для idle соединений
		DisableKeepAlives:   false,            // Включаем keep-alive для connection pooling
	}

	// Создаем HTTP клиент с настроенным transport
	httpClient := &http.Client{
		Transport: transport,
		Timeout:   120 * time.Second, // Общий таймаут для HTTP запросов
	}

	// Настраиваем BotAPI для использования нашего HTTP клиента
	// Проверяем, есть ли возможность установить HTTP клиент в tgbotapi.BotAPI
	if httpClientField := reflect.ValueOf(b.api).Elem().FieldByName("Client"); httpClientField.IsValid() && httpClientField.CanSet() {
		httpClientField.Set(reflect.ValueOf(httpClient))
		logger.Info("Configured HTTP client with connection pooling for Telegram Bot API")
	} else {
		logger.Warn("Unable to configure custom HTTP client - tgbotapi.BotAPI may not support client injection")
	}
}

func (b *Bot) handleUpdate(ctx context.Context, update tgbotapi.Update) error {
	// Обработка callback query (кнопки)
	if update.CallbackQuery != nil {
		logger.Debug("Handling callback query",
			logger.String("data", update.CallbackQuery.Data),
			logger.Int64("chat_id", update.CallbackQuery.Message.Chat.ID),
		)
		return b.handleCallbackQuery(ctx, update.CallbackQuery)
	}

	if update.Message == nil {
		return nil
	}

	chatID := update.Message.Chat.ID
	text := update.Message.Text
	userID := update.Message.From.ID

	// Команды
	if update.Message.IsCommand() {
		logger.Info("Handling command",
			logger.String("command", update.Message.Command()),
			logger.String("args", update.Message.CommandArguments()),
			logger.Int64("chat_id", chatID),
			logger.Int64("user_id", int64(userID)),
		)
		// Для команды /feedback передаем также информацию о пользователе для метаданных
		return b.handleCommand(ctx, chatID, update.Message.Command(), update.Message.CommandArguments(), int64(userID), update.Message.From)
	}

	// Проверяем, не является ли сообщение командой, которая не была распознана как команда
	// (например, если пользователь написал /battlefield с пробелом или другим форматированием)
	if strings.HasPrefix(text, "/") {
		// Извлекаем команду и аргументы
		parts := strings.Fields(text)
		if len(parts) > 0 {
			command := strings.TrimPrefix(parts[0], "/")
			args := ""
			if len(parts) > 1 {
				args = strings.Join(parts[1:], " ")
			}
			// Проверяем, является ли это известной командой
			if b.isKnownCommand(command) {
				logger.Info("Command not recognized as command by Telegram, handling manually",
					logger.String("command", command),
					logger.String("text", text),
					logger.Int64("chat_id", chatID),
				)
				return b.handleCommand(ctx, chatID, command, args, int64(userID), update.Message.From)
			}
		}
	}

	// Проверяем, есть ли активный диалог feedback
	b.feedbackStateMu.RLock()
	feedbackState, hasFeedbackDialog := b.feedbackState[chatID]
	b.feedbackStateMu.RUnlock()

	if hasFeedbackDialog && feedbackState != nil && feedbackState.Type != "" && feedbackState.Category != "" {
		// Пользователь вводит текст для feedback
		feedbackText := strings.TrimSpace(text)
		if feedbackText == "" {
			msg := tgbotapi.NewMessage(chatID, "Пожалуйста, введите текст вашего отзыва.")
			return b.sendMessage(msg)
		}

		// Сохраняем feedback
		err := b.saveFeedbackDirectly(ctx, chatID, int64(userID), update.Message.From, feedbackText, feedbackState.Type, feedbackState.Category)

		// Очищаем состояние диалога
		b.feedbackStateMu.Lock()
		delete(b.feedbackState, chatID)
		b.feedbackStateMu.Unlock()

		return err
	}

	// Проверяем, не находится ли пользователь в состоянии feedback диалога
	b.feedbackStateMu.RLock()
	feedbackState, inFeedbackDialog := b.feedbackState[chatID]
	b.feedbackStateMu.RUnlock()

	if inFeedbackDialog && feedbackState != nil && feedbackState.Type != "" && feedbackState.Category != "" {
		// Пользователь вводит текст для feedback
		feedbackText := strings.TrimSpace(text)
		if feedbackText == "" {
			msg := tgbotapi.NewMessage(chatID, "Пожалуйста, введите текст вашего отзыва.")
			return b.sendMessage(msg)
		}

		// Сохраняем feedback
		userFrom := feedbackState.From
		if userFrom == nil {
			userFrom = update.Message.From
		}
		err := b.saveFeedbackDirectly(ctx, chatID, int64(userID), userFrom, feedbackText, feedbackState.Type, feedbackState.Category)

		// Очищаем состояние диалога
		b.feedbackStateMu.Lock()
		delete(b.feedbackState, chatID)
		b.feedbackStateMu.Unlock()

		return err
	}

	// Проверяем ручной ввод результата проверки навыка (офлайн-кубики)
	handled, err := b.tryHandleManualAbilityCheck(ctx, chatID, text)
	if handled {
		return err
	}

	// Обычные сообщения - действия игрока
	logger.Debug("Handling player action",
		logger.Int64("chat_id", chatID),
		logger.Int64("user_id", int64(userID)),
		logger.Int("message_length", len(text)),
	)
	return b.handlePlayerAction(ctx, chatID, int64(userID), text)
}

// isKnownCommand проверяет, является ли строка известной командой
func (b *Bot) isKnownCommand(command string) bool {
	knownCommands := map[string]bool{
		"start":           true,
		"help":            true,
		"newgame":         true,
		"endgame":         true,
		"createcharacter": true,
		"character":       true,
		"history":         true,
		"inventory":       true,
		"roll":            true,
		"quests":          true,
		"daily":           true,
		"map":             true,
		"achievements":    true,
		"spells":          true,
		"cast":            true,
		"image":           true,
		"subscribe":       true,
		"subscription":    true,
		"flee":            true,
		"run":             true,
		"feedback":        true,
		"attack":          true,
		"pickup":          true,
		"battlefield":     true,
		"abilities":       true,
		"cooperative":     true,
		"join":            true,
		"leave":           true,
		"coopstatus":      true,
	}
	return knownCommands[strings.ToLower(command)]
}

func (b *Bot) handleCommand(ctx context.Context, chatID int64, command, args string, tgUserID int64, from *tgbotapi.User) error {
	switch command {
	case "start":
		return b.handleStart(ctx, chatID)
	case "help":
		return b.handleHelp(ctx, chatID)
	case "newgame":
		return b.handleNewGame(ctx, chatID, tgUserID, args)
	case "endgame":
		return b.handleEndGame(ctx, chatID)
	case "createcharacter":
		return b.handleCreateCharacter(ctx, chatID, args)
	case "character":
		return b.handleShowCharacter(ctx, chatID, tgUserID)
	case "history":
		return b.handleHistory(ctx, chatID, args)
	case "inventory":
		return b.handleInventory(ctx, chatID, tgUserID)
	case "roll":
		return b.handleRoll(ctx, chatID, args)
	case "quests":
		return b.handleQuests(ctx, chatID)
	case "daily":
		return b.handleDaily(ctx, chatID, tgUserID)
	case "map":
		return b.handleMap(ctx, chatID)
	case "achievements":
		return b.handleAchievements(ctx, chatID, tgUserID)
	case "party":
		return b.handleParty(ctx, chatID, tgUserID)
	case "dismiss":
		return b.handleDismiss(ctx, chatID, tgUserID, args)
	case "preferences":
		return b.handlePreferences(ctx, chatID, tgUserID)
	case "set_style":
		return b.handleSetStyle(ctx, chatID, tgUserID, args)
	case "set_detail":
		return b.handleSetDetail(ctx, chatID, tgUserID, args)
	case "set_language":
		return b.handleSetLanguage(ctx, chatID, tgUserID, args)
	case "toggle_stats":
		return b.handleToggleStats(ctx, chatID, tgUserID)
	case "leaderboard":
		return b.handleLeaderboard(ctx, chatID, tgUserID, args)
	case "spells":
		return b.handleSpells(ctx, chatID, tgUserID)
	case "cast":
		return b.handleCast(ctx, chatID, tgUserID, args)
	case "image":
		return b.handleImage(ctx, chatID, tgUserID, args)
	case "autoimage":
		return b.handleToggleAutoImage(ctx, chatID, tgUserID, args)
	case "subscribe":
		return b.handleSubscribe(ctx, chatID, tgUserID, args)
	case "subscription":
		return b.handleSubscription(ctx, chatID, tgUserID)
	case "flee", "run":
		return b.handleFlee(ctx, chatID)
	case "feedback":
		return b.handleFeedback(ctx, chatID, args, tgUserID, from)
	case "attack":
		return b.handleAttack(ctx, chatID, args)
	case "pickup":
		return b.handlePickup(ctx, chatID, args, tgUserID)
	case "battlefield":
		return b.handleBattlefield(ctx, chatID, args)
	case "abilities":
		return b.handleAbilities(ctx, chatID, args)
	case "wait_until", "time":
		return b.handleWaitUntil(ctx, chatID, args)
	case "progress":
		return b.handleProgress(ctx, chatID, tgUserID)
	case "cooperative":
		return b.handleCooperative(ctx, chatID, tgUserID, args)
	case "join":
		return b.handleJoin(ctx, chatID, tgUserID)
	case "leave":
		return b.handleLeave(ctx, chatID, tgUserID)
	case "coopstatus":
		return b.handleCoopStatus(ctx, chatID)
	default:
		msg := tgbotapi.NewMessage(chatID, "Неизвестная команда. Используйте /help для списка команд")
		return b.sendMessage(msg)
	}
}

func (b *Bot) handleStart(ctx context.Context, chatID int64) error {
	return b.handleHelp(ctx, chatID)
}

func (b *Bot) handleHelp(ctx context.Context, chatID int64) error {
	text := `🎲 Добро пожаловать в Dungeons & Dragons AI!

Я ваш Dungeon Master. Используйте команды:

🎮 Основные команды:
/newgame <тема> - начать новую игру
/endgame - завершить текущую игру
/help - показать эту справку

👤 Персонаж:
/createcharacter - создать персонажа (интерактивно или через команду)
/character - посмотреть информацию о персонаже
/progress - посмотреть прогресс и статистику персонажа

🎒 Инвентарь и предметы:
/inventory - посмотреть инвентарь
/pickup <предмет> [количество] - подобрать предмет в инвентарь

⚔️ Бой:
/attack - атаковать противника (во время боя)
/battlefield [format] - показать поле боя (format: table/compact/detailed)

🎲 Игра:
/roll <выражение> - бросить кубик (например: /roll d20, /roll 2d6+3)
/history - посмотреть историю игры
/quests - посмотреть активные квесты
/map - посмотреть карту мира
/wait_until <время> - изменить время суток (morning/noon/afternoon/evening/night/midnight)
/flee - попытаться выйти из боя (во время боя)
/abilities [filter] - показать способности персонажа (filter: all/spells/feats/class)
/spells - показать заклинания персонажа
/cast <название> [цель] - использовать заклинание (например: /cast Огненный снаряд)
/feedback <текст> - отправить отзыв о игре

👥 Отряд:
/party - посмотреть состав вашего отряда (лидер + компаньоны)
/dismiss <имя> - уволить компаньона из отряда (например: /dismiss Алара)

⚙️ Персонализация:
/preferences - посмотреть текущие настройки стиля повествования
/set_style <стиль> - изменить стиль (balanced/dark/light/detailed/minimalist)
/set_detail <уровень> - изменить детализация (low/medium/high)
/set_language <язык> - изменить язык (ru/en)
/toggle_stats - переключить отображение статистики

💡 После начала игры просто пишите мне, что хотите сделать, и я буду описывать что происходит!`

	return b.sendLongMessage(chatID, text)
}

// HealthCheck проверяет подключение к Telegram API
func (b *Bot) HealthCheck(ctx context.Context) error {
	if b == nil || b.api == nil {
		return fmt.Errorf("telegram bot not initialized")
	}

	// Кэшируем результат health check на 30 секунд
	b.healthCheckMu.RLock()
	if time.Since(b.lastHealthCheck) < 30*time.Second {
		b.healthCheckMu.RUnlock()
		return nil // Предыдущий health check был успешен
	}
	b.healthCheckMu.RUnlock()

	// Выполняем быстрый health check через GetMe
	_, err := b.api.GetMe()
	if err != nil {
		return fmt.Errorf("telegram API health check failed: %w", err)
	}

	// Обновляем время последнего успешного health check
	b.healthCheckMu.Lock()
	b.lastHealthCheck = time.Now()
	b.healthCheckMu.Unlock()

	return nil
}

func (b *Bot) handleNewGame(ctx context.Context, chatID int64, tgUserID int64, theme string) error {
	if theme == "" {
		theme = "классическое фэнтези"
	}

	// Проверяем существующую сессию
	existingSession, err := b.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil {
		return fmt.Errorf("failed to check existing session: %w", err)
	}

	if existingSession != nil {
		if existingSession.IsActive() {
			msg := tgbotapi.NewMessage(chatID, "У вас уже есть активная игра. Используйте /endgame для завершения текущей игры перед началом новой.")
			return b.sendMessage(msg)
		}
		// Если есть завершенная сессия, удаляем её перед созданием новой
		// Это предотвращает нарушение уникального индекса на chat_id
		logger.Info("Removing completed session before creating new one",
			logger.Int64("chat_id", chatID),
			logger.Uint("old_session_id", existingSession.ID),
			logger.String("old_state", string(existingSession.State)),
		)
		if err := b.sessionRepo.Delete(ctx, chatID); err != nil {
			logger.Error("Failed to delete completed session",
				logger.ErrorField(err),
				logger.Int64("chat_id", chatID),
			)
			// Не возвращаем ошибку, пытаемся продолжить создание новой игры
		}
	}

	// Отправляем сообщение о начале генерации
	logger.Info("Starting new game",
		logger.Int64("chat_id", chatID),
		logger.String("theme", theme),
	)
	msg := tgbotapi.NewMessage(chatID, "🎲 Создаю мир... Это может занять несколько секунд.")
	if err := b.sendMessage(msg); err != nil {
		logger.Error("Failed to send message",
			logger.ErrorField(err),
			logger.Int64("chat_id", chatID),
		)
	}

	// Создаём кампанию
	llmCtx := context.WithValue(ctx, "chat_id", chatID)
	llmCtx = context.WithValue(llmCtx, "tg_user_id", tgUserID)
	world, err := b.initCampaignUC.Execute(llmCtx, theme)
	if err != nil {
		logger.Error("Failed to create campaign",
			logger.ErrorField(err),
			logger.Int64("chat_id", chatID),
			logger.String("theme", theme),
		)
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при создании игры: %v", err))
		if sendErr := b.sendMessage(errorMsg); sendErr != nil {
			logger.Error("Failed to send error message",
				logger.ErrorField(sendErr),
				logger.Int64("chat_id", chatID),
			)
		}
		return err
	}
	logger.Info("Campaign created successfully",
		logger.Int64("chat_id", chatID),
		logger.Uint("world_id", world.ID),
		logger.String("world_name", world.Name),
	)

	// Создаём игровую сессию
	gs := &session.GameSession{
		ChatID:  chatID,
		State:   session.StateActive,
		World:   *world,
		WorldID: world.ID,
	}

	if err := b.sessionRepo.Save(ctx, gs); err != nil {
		if isDuplicateChatIDError(err) {
			// На всякий случай проверяем, не существует ли активная сессия, и пробуем очистить завершенную.
			existing, getErr := b.sessionRepo.GetByChatID(ctx, chatID)
			if getErr == nil && existing != nil && existing.IsActive() {
				msg := tgbotapi.NewMessage(chatID, "У вас уже есть активная игра. Используйте /endgame для завершения текущей игры перед началом новой.")
				return b.sendMessage(msg)
			}
			if delErr := b.sessionRepo.Delete(ctx, chatID); delErr != nil {
				logger.Error("Failed to delete existing session after duplicate key error",
					logger.ErrorField(delErr),
					logger.Int64("chat_id", chatID),
				)
			} else {
				logger.Info("Deleted existing session after duplicate key error, retrying save",
					logger.Int64("chat_id", chatID),
				)
				if retryErr := b.sessionRepo.Save(ctx, gs); retryErr == nil {
					goto sessionSaved
				} else {
					err = retryErr
				}
			}
		}

		logger.Error("Failed to save game session",
			logger.ErrorField(err),
			logger.Int64("chat_id", chatID),
			logger.Uint("world_id", world.ID),
		)
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при сохранении сессии: %v", err))
		if sendErr := b.sendMessage(errorMsg); sendErr != nil {
			logger.Error("Failed to send error message",
				logger.ErrorField(sendErr),
				logger.Int64("chat_id", chatID),
			)
		}
		return err
	}
sessionSaved:
	logger.Info("Game session saved",
		logger.Int64("chat_id", chatID),
		logger.Uint("session_id", gs.ID),
	)

	// Генерируем цели для сессии
	gs.GenerateSessionGoals()
	if err := b.sessionRepo.Save(ctx, gs); err != nil {
		logger.Error("Failed to save session goals",
			logger.ErrorField(err),
			logger.Int64("chat_id", chatID),
		)
		// Не возвращаем ошибку, продолжаем игру без целей
	}

	// Инициализируем текущую локацию (по умолчанию первая локация мира)
	// Это нужно для /map и навигации по кнопкам
	if gs.CurrentLocationID == nil && len(gs.World.Locations) > 0 {
		firstID := gs.World.Locations[0].ID
		if firstID != 0 {
			gs.CurrentLocationID = &firstID
			if err := b.sessionRepo.Save(ctx, gs); err != nil {
				logger.Warn("Failed to set initial current location",
					logger.ErrorField(err),
					logger.Int64("chat_id", chatID),
				)
			}
		}
	}

	// Генерируем красивую карту-изображение мира (если доступна генерация изображений)
	// Сохраняем путь в сессии, чтобы /map мог показать картинку
	if b.generateImageUC != nil && gs.MapImagePath == "" && len(gs.World.Locations) > 0 {
		var sb strings.Builder
		sb.WriteString("Fantasy world map, top-down, detailed, colored, parchment style, Dungeons & Dragons.\n")
		sb.WriteString(fmt.Sprintf("World: %s.\n", gs.World.Name))
		if gs.World.Description != "" {
			sb.WriteString(fmt.Sprintf("World description: %s.\n", gs.World.Description))
		}
		sb.WriteString("Locations:\n")
		for _, loc := range gs.World.Locations {
			sb.WriteString(fmt.Sprintf("- %s\n", loc.Name))
		}
		sb.WriteString("Connections (direction -> destination):\n")
		// Строим карту ID->name для удобного отображения связей
		locNameByID := map[uint]string{}
		for _, loc := range gs.World.Locations {
			locNameByID[loc.ID] = loc.Name
		}
		for _, loc := range gs.World.Locations {
			for _, conn := range loc.Connections {
				toName := locNameByID[conn.ToLocationID]
				if toName == "" {
					toName = fmt.Sprintf("Location #%d", conn.ToLocationID)
				}
				sb.WriteString(fmt.Sprintf("- %s: %s -> %s\n", loc.Name, conn.Direction, toName))
			}
		}

		imgCtx, cancel := context.WithTimeout(ctx, 120*time.Second) // Увеличиваем таймаут для изображений с учетом retry
		defer cancel()
		req := imageapp.GenerateImageRequest{
			SystemPrompt:    "You are a fantasy cartographer. Create beautiful D&D-style maps with clear landmarks and readable layout. No text labels on the map itself.",
			UserPrompt:      sb.String(),
			Type:            "custom",
			EntityID:        gs.WorldID,
			ForceRegenerate: false,
			UserID:          0,
			SkipLimitCheck:  true,
		}
		resp, err := b.generateImageUC.Execute(imgCtx, req)
		if err != nil {
			logger.Warn("Failed to generate world map image",
				logger.ErrorField(err),
				logger.Int64("chat_id", chatID),
				logger.Uint("world_id", gs.WorldID),
			)
		} else if resp != nil && resp.ImagePath != "" {
			gs.MapImagePath = resp.ImagePath
			if err := b.sessionRepo.Save(ctx, gs); err != nil {
				logger.Warn("Failed to save map image path to session",
					logger.ErrorField(err),
					logger.Int64("chat_id", chatID),
				)
			}
		}
	}

	// Отправляем приветственное сообщение
	welcomeText := fmt.Sprintf(`🎮 Игра начата!

Мир: %s
%s

Главный квест: %s
%s

Используйте команды или просто пишите мне, что хотите сделать!`,
		world.Name,
		world.Description,
		world.MainQuest.Title,
		world.MainQuest.Description)

	return b.sendLongMessage(chatID, welcomeText)
}

func (b *Bot) handlePlayerAction(ctx context.Context, chatID int64, tgUserID int64, text string) error {
	// Отправляем индикатор печати
	actionMsg := tgbotapi.NewMessage(chatID, "🤔 Думаю...")
	sentMsg, err := b.api.Send(actionMsg)
	indicatorSent := err == nil // Запоминаем, был ли индикатор успешно отправлен

	if err != nil {
		// Если не удалось отправить индикатор, продолжаем выполнение
		// но логируем ошибку для диагностики
		logger.Warn("Failed to send typing indicator",
			logger.ErrorField(err),
			logger.Int64("chat_id", chatID),
		)
	}

	// Получаем ответ от DM
	logger.Debug("Processing player action",
		logger.Int64("chat_id", chatID),
		logger.Int64("tg_user_id", tgUserID),
		logger.Int("message_length", len(text)),
	)
	response, err := b.handleActionUC.Execute(ctx, chatID, tgUserID, text)

	if err != nil {
		logger.Error("Failed to handle player action",
			logger.ErrorField(err),
			logger.Int64("chat_id", chatID),
		)
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка: %v", err))
		if sendErr := b.sendMessage(errorMsg); sendErr != nil {
			logger.Error("Failed to send error message",
				logger.ErrorField(sendErr),
				logger.Int64("chat_id", chatID),
			)
		}
		return err
	}
	logger.Debug("Player action processed",
		logger.Int64("chat_id", chatID),
		logger.Int("response_length", len(response)),
	)

	// Проверяем наличие маркеров изображений в ответе и отправляем их
	imagePaths := b.extractImageMarkers(response)
	if len(imagePaths) > 0 {
		logger.Info("Sending generated images",
			logger.Int64("chat_id", chatID),
			logger.Int("image_count", len(imagePaths)),
		)
		for _, imagePath := range imagePaths {
			// Удаляем маркер из текста перед отправкой
			response = strings.ReplaceAll(response, fmt.Sprintf("[IMAGE:%s]", imagePath), "")
			// Отправляем изображение
			if err := b.sendPhoto(ctx, chatID, imagePath, ""); err != nil {
				logger.Warn("Failed to send generated image",
					logger.ErrorField(err),
					logger.Int64("chat_id", chatID),
					logger.String("image_path", imagePath),
				)
				// Не прерываем выполнение, просто логируем ошибку
			}
		}
		// Очищаем пробелы и переносы строк после удаления маркеров
		response = strings.TrimSpace(response)
	}

	// Обновляем сообщение с ответом
	// Если ответ слишком длинный, отправляем новое сообщение вместо редактирования
	var sendErr error
	if len(response) > TelegramMaxMessageLength {
		// Удаляем индикатор печати, если он был отправлен
		if indicatorSent {
			deleteMsg := tgbotapi.NewDeleteMessage(chatID, sentMsg.MessageID)
			if _, err := b.api.Send(deleteMsg); err != nil {
				logger.Warn("Failed to delete typing indicator",
					logger.ErrorField(err),
					logger.Int64("chat_id", chatID),
				)
			}
		}
		// Отправляем разбитое сообщение
		sendErr = b.sendLongMessage(chatID, response)
	} else if !indicatorSent {
		// Если индикатор не был отправлен, просто отправляем новое сообщение
		sendErr = b.sendLongMessage(chatID, response)
	} else {
		edit := tgbotapi.NewEditMessageText(chatID, sentMsg.MessageID, response)
		sendErr = b.editMessage(edit, chatID, response)
	}

	promptErr := b.maybeSendPendingAbilityCheckPrompt(ctx, chatID)
	if sendErr != nil {
		return sendErr
	}
	return promptErr
}

func (b *Bot) handleCreateCharacter(ctx context.Context, chatID int64, args string) error {
	// Парсим аргументы: /createcharacter имя раса класс
	parts := strings.Fields(args)

	var name string
	var race character.Race
	var class character.Class

	if len(parts) >= 1 && parts[0] != "" {
		name = parts[0]
	} else {
		// Интерактивное создание через кнопки
		return b.showCharacterCreationMenu(ctx, chatID)
	}

	if len(parts) >= 2 {
		race = character.Race(strings.ToLower(parts[1]))
	} else {
		race = character.RaceHuman // по умолчанию
	}

	if len(parts) >= 3 {
		class = character.Class(strings.ToLower(parts[2]))
	} else {
		class = character.ClassFighter // по умолчанию
	}

	// Валидация расы
	validRaces := map[character.Race]bool{
		character.RaceHuman:    true,
		character.RaceElf:      true,
		character.RaceDwarf:    true,
		character.RaceOrc:      true,
		character.RaceHalfling: true,
	}
	if !validRaces[race] {
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Неизвестная раса: %s. Доступные: human, elf, dwarf, orc, halfling", race))
		return b.sendMessage(msg)
	}

	// Валидация класса
	validClasses := map[character.Class]bool{
		character.ClassFighter: true,
		character.ClassWizard:  true,
		character.ClassRogue:   true,
		character.ClassCleric:  true,
		character.ClassRanger:  true,
	}
	if !validClasses[class] {
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Неизвестный класс: %s. Доступные: fighter, wizard, rogue, cleric, ranger", class))
		return b.sendMessage(msg)
	}

	// Создаем персонажа
	req := characterapp.CreateCharacterRequest{
		ChatID: chatID,
		Name:   name,
		Race:   race,
		Class:  class,
	}

	player, err := b.createCharacterUC.Execute(ctx, req)
	if err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при создании персонажа: %v", err))
		return b.sendMessage(errorMsg)
	}

	// Формируем сообщение с информацией о персонаже
	charText := fmt.Sprintf(`✅ Персонаж создан!

👤 Имя: %s
🏛️ Раса: %s
⚔️ Класс: %s
📊 Уровень: %d
❤️ HP: %d/%d

📈 Характеристики:
💪 Сила: %d
🏃 Ловкость: %d
🛡️ Телосложение: %d
🧠 Интеллект: %d
🔮 Мудрость: %d
💬 Харизма: %d`,
		player.Character.Name,
		player.Character.Race,
		player.Character.Class,
		player.Character.Level,
		player.Character.HP,
		player.Character.MaxHP,
		player.Character.Stats.Strength,
		player.Character.Stats.Dexterity,
		player.Character.Stats.Constitution,
		player.Character.Stats.Intelligence,
		player.Character.Stats.Wisdom,
		player.Character.Stats.Charisma,
	)

	return b.sendLongMessage(chatID, charText)
}

func (b *Bot) showCharacterCreationMenu(ctx context.Context, chatID int64) error {
	text := `🎭 Создание персонажа

Используйте команду:
/createcharacter <имя> <раса> <класс>

Пример: /createcharacter Гендальф elf wizard

Или начните создание через кнопки:`

	msg := tgbotapi.NewMessage(chatID, text)

	// Создаем кнопки для выбора расы
	raceButtons := [][]tgbotapi.InlineKeyboardButton{
		{
			tgbotapi.NewInlineKeyboardButtonData("👤 Человек", "race_human"),
			tgbotapi.NewInlineKeyboardButtonData("🧝 Эльф", "race_elf"),
		},
		{
			tgbotapi.NewInlineKeyboardButtonData("⛏️ Дварф", "race_dwarf"),
			tgbotapi.NewInlineKeyboardButtonData("👹 Орк", "race_orc"),
		},
		{
			tgbotapi.NewInlineKeyboardButtonData("🧙 Хоббит", "race_halfling"),
		},
	}

	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(raceButtons...)

	return b.sendMessage(msg)
}

func (b *Bot) handleCallbackQuery(ctx context.Context, query *tgbotapi.CallbackQuery) error {
	chatID := query.Message.Chat.ID
	data := query.Data

	logger.Debug("Handling callback query",
		logger.String("data", data),
		logger.Int64("chat_id", chatID),
		logger.Int64("user_id", query.From.ID),
	)

	// Парсим callback data
	// Формат: race_<race> | class_<race>_<class> | create_<name>_<race>_<class> | map_to_<location_id>
	if strings.HasPrefix(data, "race_") {
		// Выбор расы
		race := strings.TrimPrefix(data, "race_")
		return b.handleRaceSelection(ctx, chatID, query, race)
	} else if strings.HasPrefix(data, "class_") {
		// Выбор класса (формат: class_<race>_<class>)
		parts := strings.Split(strings.TrimPrefix(data, "class_"), "_")
		if len(parts) >= 2 {
			race := parts[0]
			class := parts[1]
			return b.handleClassSelection(ctx, chatID, query, race, class)
		}
	} else if strings.HasPrefix(data, "create_") {
		// Завершение создания персонажа (формат: create_<name>_<race>_<class>)
		parts := strings.Split(strings.TrimPrefix(data, "create_"), "_")
		if len(parts) >= 3 {
			name := parts[0]
			race := parts[1]
			class := parts[2]
			return b.handleCreateCharacterFromCallback(ctx, chatID, query, name, race, class)
		}
	} else if strings.HasPrefix(data, "feedback_type_") {
		// Выбор типа feedback
		feedbackType := strings.TrimPrefix(data, "feedback_type_")
		return b.handleFeedbackTypeSelection(ctx, chatID, query, feedback.FeedbackType(feedbackType))
	} else if strings.HasPrefix(data, "feedback_category_") {
		// Выбор категории feedback
		feedbackCategory := strings.TrimPrefix(data, "feedback_category_")
		return b.handleFeedbackCategorySelection(ctx, chatID, query, feedback.FeedbackCategory(feedbackCategory))
	} else if data == "feedback_cancel" {
		// Отмена диалога feedback
		return b.handleFeedbackCancel(ctx, chatID, query)
	} else if strings.HasPrefix(data, "map_to_") {
		// Навигация по карте: переход в локацию
		idStr := strings.TrimPrefix(data, "map_to_")
		locID, err := strconv.ParseUint(idStr, 10, 64)
		if err != nil || locID == 0 {
			callback := tgbotapi.NewCallback(query.ID, "Некорректная локация")
			_, _ = b.api.Request(callback)
			return nil
		}
		return b.handleMapMoveCallback(ctx, query, uint(locID))
	}

	// Неизвестный callback
	callback := tgbotapi.NewCallback(query.ID, "Неизвестная команда")
	_, err := b.api.Request(callback)
	return err
}

func (b *Bot) handleMapMoveCallback(ctx context.Context, query *tgbotapi.CallbackQuery, toLocationID uint) error {
	chatID := query.Message.Chat.ID
	tgUserID := int64(0)
	if query.From != nil {
		tgUserID = query.From.ID
	}

	// Отвечаем на callback, чтобы Telegram убрал "loading"
	callback := tgbotapi.NewCallback(query.ID, "Перемещаюсь...")
	_, _ = b.api.Request(callback)

	if b.moveToLocationUC == nil {
		edit := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID, "Навигация по карте временно недоступна.")
		return b.editMessage(edit, chatID, "Навигация по карте временно недоступна.")
	}

	gsForContext, _ := b.sessionRepo.GetByChatID(ctx, chatID)
	ctxWithIDs := context.WithValue(ctx, "chat_id", chatID)
	if tgUserID != 0 {
		ctxWithIDs = context.WithValue(ctxWithIDs, "tg_user_id", tgUserID)
	}
	if gsForContext != nil {
		ctxWithIDs = context.WithValue(ctxWithIDs, "session_id", gsForContext.ID)
	}

	resp, err := b.moveToLocationUC.Execute(ctxWithIDs, mapapp.MoveToLocationRequest{
		ChatID:       chatID,
		ToLocationID: &toLocationID,
	})
	if err != nil {
		edit := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID, fmt.Sprintf("Не удалось переместиться: %v", err))
		return b.editMessage(edit, chatID, edit.Text)
	}

	// Перестраиваем клавиатуру под новую локацию
	gs, _ := b.sessionRepo.GetByChatID(ctx, chatID)
	markup := b.buildMapNavigationKeyboard(gs)

	edit := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID, resp.Message)
	if markup != nil {
		edit.ReplyMarkup = markup
	}
	return b.editMessage(edit, chatID, resp.Message)
}

func (b *Bot) handleRaceSelection(ctx context.Context, chatID int64, query *tgbotapi.CallbackQuery, race string) error {
	// Отвечаем на callback
	callback := tgbotapi.NewCallback(query.ID, fmt.Sprintf("Выбрана раса: %s", race))
	if _, err := b.api.Request(callback); err != nil {
		logger.Error("Failed to answer callback",
			logger.ErrorField(err),
		)
	}

	// Обновляем сообщение с кнопками выбора класса
	text := fmt.Sprintf(`🎭 Создание персонажа

✅ Выбрана раса: %s

Теперь выберите класс:`, race)

	edit := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID, text)

	// Создаем кнопки для выбора класса с указанием расы
	classButtons := [][]tgbotapi.InlineKeyboardButton{
		{
			tgbotapi.NewInlineKeyboardButtonData("⚔️ Воин", fmt.Sprintf("class_%s_fighter", race)),
			tgbotapi.NewInlineKeyboardButtonData("🔮 Маг", fmt.Sprintf("class_%s_wizard", race)),
		},
		{
			tgbotapi.NewInlineKeyboardButtonData("🗡️ Вор", fmt.Sprintf("class_%s_rogue", race)),
			tgbotapi.NewInlineKeyboardButtonData("✨ Жрец", fmt.Sprintf("class_%s_cleric", race)),
		},
		{
			tgbotapi.NewInlineKeyboardButtonData("🏹 Следопыт", fmt.Sprintf("class_%s_ranger", race)),
		},
		{
			tgbotapi.NewInlineKeyboardButtonData("⬅️ Назад", "race_human"), // Кнопка возврата
		},
	}

	edit.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: classButtons}
	return b.editMessage(edit, chatID, text)
}

func (b *Bot) handleClassSelection(ctx context.Context, chatID int64, query *tgbotapi.CallbackQuery, race, class string) error {
	// Отвечаем на callback
	callback := tgbotapi.NewCallback(query.ID, fmt.Sprintf("Выбран класс: %s", class))
	if _, err := b.api.Request(callback); err != nil {
		logger.Error("Failed to answer callback",
			logger.ErrorField(err),
		)
	}

	// Запрашиваем имя персонажа
	text := fmt.Sprintf(`🎭 Создание персонажа

✅ Раса: %s
✅ Класс: %s

📝 Теперь введите имя персонажа текстовым сообщением, или используйте команду:
/createcharacter <имя> %s %s

Пример: /createcharacter Гендальф %s %s`, race, class, race, class, race, class)

	edit := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID, text)

	// Используем username как дефолтное имя
	defaultName := query.From.UserName
	if defaultName == "" {
		defaultName = query.From.FirstName
	}
	if defaultName == "" {
		defaultName = "Герой"
	}

	// Кнопка для создания с дефолтным именем
	buttons := [][]tgbotapi.InlineKeyboardButton{
		{
			tgbotapi.NewInlineKeyboardButtonData(
				fmt.Sprintf("✅ Создать с именем '%s'", defaultName),
				fmt.Sprintf("create_%s_%s_%s", defaultName, race, class),
			),
		},
		{
			tgbotapi.NewInlineKeyboardButtonData("⬅️ Назад к выбору расы", "race_human"),
		},
	}

	edit.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: buttons}
	_, err := b.api.Send(edit)
	return err
}

func (b *Bot) handleCreateCharacterFromCallback(ctx context.Context, chatID int64, query *tgbotapi.CallbackQuery, name, raceStr, classStr string) error {
	// Отвечаем на callback
	callback := tgbotapi.NewCallback(query.ID, "Создаю персонажа...")
	if _, err := b.api.Request(callback); err != nil {
		logger.Error("Failed to answer callback",
			logger.ErrorField(err),
		)
	}

	// Парсим расу и класс
	race := character.Race(strings.ToLower(raceStr))
	class := character.Class(strings.ToLower(classStr))

	// Валидация расы
	validRaces := map[character.Race]bool{
		character.RaceHuman:    true,
		character.RaceElf:      true,
		character.RaceDwarf:    true,
		character.RaceOrc:      true,
		character.RaceHalfling: true,
	}
	if !validRaces[race] {
		errorMsg := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID,
			fmt.Sprintf("❌ Ошибка: Неизвестная раса: %s", race))
		return b.editMessage(errorMsg, chatID, fmt.Sprintf("❌ Ошибка: Неизвестная раса: %s", race))
	}

	// Валидация класса
	validClasses := map[character.Class]bool{
		character.ClassFighter: true,
		character.ClassWizard:  true,
		character.ClassRogue:   true,
		character.ClassCleric:  true,
		character.ClassRanger:  true,
	}
	if !validClasses[class] {
		errorMsg := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID,
			fmt.Sprintf("❌ Ошибка: Неизвестный класс: %s", class))
		return b.editMessage(errorMsg, chatID, fmt.Sprintf("❌ Ошибка: Неизвестный класс: %s", class))
	}

	// Создаем персонажа
	req := characterapp.CreateCharacterRequest{
		ChatID: chatID,
		Name:   name,
		Race:   race,
		Class:  class,
	}

	player, err := b.createCharacterUC.Execute(ctx, req)
	if err != nil {
		errorMsg := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID,
			fmt.Sprintf("❌ Ошибка при создании персонажа: %v", err))
		if sendErr := b.editMessage(errorMsg, chatID, fmt.Sprintf("❌ Ошибка при создании персонажа: %v", err)); sendErr != nil {
			return sendErr
		}
		return err
	}

	// Формируем сообщение с информацией о персонаже
	charText := fmt.Sprintf(`✅ Персонаж создан!

👤 Имя: %s
🏛️ Раса: %s
⚔️ Класс: %s
📊 Уровень: %d
❤️ HP: %d/%d

📈 Характеристики:
💪 Сила: %d
🏃 Ловкость: %d
🛡️ Телосложение: %d
🧠 Интеллект: %d
🔮 Мудрость: %d
💬 Харизма: %d`,
		player.Character.Name,
		player.Character.Race,
		player.Character.Class,
		player.Character.Level,
		player.Character.HP,
		player.Character.MaxHP,
		player.Character.Stats.Strength,
		player.Character.Stats.Dexterity,
		player.Character.Stats.Constitution,
		player.Character.Stats.Intelligence,
		player.Character.Stats.Wisdom,
		player.Character.Stats.Charisma,
	)

	// Обновляем сообщение с результатом
	resultMsg := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID, charText)
	resultMsg.ReplyMarkup = nil // Убираем кнопки

	return b.editMessage(resultMsg, chatID, charText)
}

func (b *Bot) maybeSendPendingAbilityCheckPrompt(ctx context.Context, chatID int64) error {
	gs, err := b.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil || gs == nil || !gs.HasPendingAbilityCheck() || gs.PendingAbilityCheckNotified {
		return err
	}

	abilityName := formatAbilityName(gs.PendingAbilityCheckAbility)
	text := fmt.Sprintf("🎲 Проверка %s (DC %d). Напишите /roll, чтобы бросить кубик. Можно отправить результат числом (например: 17).", abilityName, gs.PendingAbilityCheckDC)

	msg := tgbotapi.NewMessage(chatID, text)

	if err := b.sendMessage(msg); err != nil {
		return err
	}

	gs.PendingAbilityCheckNotified = true
	if err := b.sessionRepo.Save(ctx, gs); err != nil {
		logger.Warn("Failed to mark pending check as notified",
			logger.ErrorField(err),
			logger.Int64("chat_id", chatID),
		)
	}

	return nil
}

func (b *Bot) tryHandleManualAbilityCheck(ctx context.Context, chatID int64, text string) (bool, error) {
	if b.performAbilityCheckUC == nil {
		return false, nil
	}

	manualRoll, ok := parseManualAbilityCheckInput(text)
	if !ok {
		return false, nil
	}

	gs, err := b.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil || gs == nil || !gs.HasPendingAbilityCheck() {
		return false, err
	}

	result, err := b.performAbilityCheckUC.ExecuteWithBaseRoll(ctx, chatID, manualRoll)
	if err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при проверке: %v", err))
		return true, b.sendMessage(errorMsg)
	}

	// Отправляем результат проверки
	if err := b.sendLongMessage(chatID, result.Message); err != nil {
		return true, err
	}

	// Автоматически продолжаем повествование после проверки (если DM доступен)
	if b.handleActionUC != nil {
		// Получаем контекст проверки из сессии
		gs, _ := b.sessionRepo.GetByChatID(ctx, chatID)
		reason := ""
		stakes := ""
		if gs != nil {
			reason = gs.PendingAbilityCheckReason
			stakes = gs.PendingAbilityCheckStakes
		}

		var continueMessage string
		if result.Success {
			if reason != "" && stakes != "" {
				continueMessage = fmt.Sprintf("✅ УСПЕХ проверки! %s успешно %s. Теперь опиши положительные последствия: %s",
					result.CharacterName, reason, stakes)
			} else {
				continueMessage = fmt.Sprintf("✅ УСПЕХ проверки! %s прошел проверку %s (DC %d, результат %d). Продолжи историю с положительными последствиями.",
					result.CharacterName, result.Ability, result.DC, result.Total)
			}
		} else {
			if reason != "" && stakes != "" {
				continueMessage = fmt.Sprintf("❌ ПРОВАЛ проверки! %s не смог %s. Теперь опиши негативные последствия: %s",
					result.CharacterName, reason, stakes)
			} else {
				continueMessage = fmt.Sprintf("❌ ПРОВАЛ проверки! %s провалил проверку %s (DC %d, результат %d). Продолжи историю с негативными последствиями.",
					result.CharacterName, result.Ability, result.DC, result.Total)
			}
		}

		// Для автоматических продолжений используем tgUserID первого игрока
		systemTgUserID := int64(0)
		if gs.GetFirstPlayer() != nil {
			systemTgUserID = gs.GetFirstPlayer().TgUserID
		}
		return true, b.handlePlayerAction(ctx, chatID, systemTgUserID, continueMessage)
	}
	return true, nil
}

func parseManualAbilityCheckInput(text string) (int, bool) {
	normalized := strings.TrimSpace(strings.ToLower(text))
	if normalized == "" {
		return 0, false
	}

	if regexp.MustCompile(`^\d{1,2}$`).MatchString(normalized) {
		value, err := strconv.Atoi(normalized)
		if err == nil && value >= 1 && value <= 20 {
			return value, true
		}
		return 0, false
	}

	if strings.HasPrefix(normalized, "результат") || strings.HasPrefix(normalized, "result") {
		re := regexp.MustCompile(`\b(\d{1,2})\b`)
		match := re.FindStringSubmatch(normalized)
		if len(match) >= 2 {
			value, err := strconv.Atoi(match[1])
			if err == nil && value >= 1 && value <= 20 {
				return value, true
			}
		}
	}

	return 0, false
}

func formatAbilityName(ability string) string {
	switch ability {
	case "strength":
		return "Сила (STR)"
	case "dexterity":
		return "Ловкость (DEX)"
	case "constitution":
		return "Телосложение (CON)"
	case "intelligence":
		return "Интеллект (INT)"
	case "wisdom":
		return "Мудрость (WIS)"
	case "charisma":
		return "Харизма (CHA)"
	default:
		return ability
	}
}

func (b *Bot) handleShowCharacter(ctx context.Context, chatID int64, tgUserID int64) error {
	// Получаем сессию
	gs, err := b.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}

	if gs == nil {
		msg := tgbotapi.NewMessage(chatID, "Игра не начата. Используйте /newgame для начала новой игры.")
		return b.sendMessage(msg)
	}

	// Ищем игрока по TgUserID (для приватных чатов chatID == tgUserID)
	// В групповых чатах это позволит найти правильного игрока
	player := gs.FindPlayerByTgUserID(tgUserID)
	if player == nil {
		// Fallback: используем первого игрока для обратной совместимости
		player = gs.GetFirstPlayer()
		if player == nil {
			msg := tgbotapi.NewMessage(chatID, "Персонаж не создан. Используйте /createcharacter для создания персонажа.")
			return b.sendMessage(msg)
		}
	}
	char := player.Character

	// Рассчитываем опыт до следующего уровня
	expToNext := char.GetExperienceToNextLevel()

	charText := fmt.Sprintf(`👤 Персонаж: %s

🏛️ Раса: %s
⚔️ Класс: %s
📊 Уровень: %d
⭐ Опыт: %d / %d (до следующего уровня: %d)
❤️ HP: %d/%d
💀 Статус: %s

📈 Характеристики:
💪 Сила: %d
🏃 Ловкость: %d
🛡️ Телосложение: %d
🧠 Интеллект: %d
🔮 Мудрость: %d
💬 Харизма: %d`,
		char.Name,
		char.Race,
		char.Class,
		char.Level,
		char.Experience,
		char.GetExperienceToNextLevel()+char.Experience,
		expToNext,
		char.HP,
		char.MaxHP,
		char.Status,
		char.Stats.Strength,
		char.Stats.Dexterity,
		char.Stats.Constitution,
		char.Stats.Intelligence,
		char.Stats.Wisdom,
		char.Stats.Charisma,
	)

	return b.sendLongMessage(chatID, charText)
}

func (b *Bot) handleHistory(ctx context.Context, chatID int64, args string) error {
	limit := 10 // по умолчанию последние 10 событий
	if args != "" {
		// Можно добавить парсинг лимита из args
		limit = 10
	}

	historyText, err := b.getHistoryUC.Execute(ctx, chatID, limit)
	if err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при получении истории: %v", err))
		return b.sendMessage(errorMsg)
	}

	return b.sendLongMessage(chatID, historyText)
}

func (b *Bot) handleInventory(ctx context.Context, chatID int64, tgUserID int64) error {
	inventoryText, err := b.getInventoryUC.Execute(ctx, chatID, tgUserID)
	if err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при получении инвентаря: %v", err))
		return b.sendMessage(errorMsg)
	}

	return b.sendLongMessage(chatID, inventoryText)
}

func (b *Bot) handlePickup(ctx context.Context, chatID int64, args string, tgUserID int64) error {
	// Парсим аргументы: /pickup <предмет> [количество]
	parts := strings.Fields(args)

	if len(parts) == 0 {
		msg := tgbotapi.NewMessage(chatID, "Укажите название предмета. Формат: /pickup <предмет> [количество]\n\nПример: /pickup меч\nПример: /pickup стрела 5")
		return b.sendMessage(msg)
	}

	// Пытаемся определить количество (последняя часть)
	quantity := 1
	itemName := ""

	if len(parts) > 1 {
		// Проверяем, является ли последняя часть числом
		if qty, err := strconv.Atoi(parts[len(parts)-1]); err == nil && qty > 0 {
			// Последняя часть - количество
			quantity = qty
			// Имя - все части кроме последней
			itemName = strings.Join(parts[:len(parts)-1], " ")
		} else {
			// Последняя часть не число - все части это название предмета
			itemName = strings.Join(parts, " ")
		}
	} else {
		// Только одна часть - это название предмета
		itemName = parts[0]
	}

	if itemName == "" {
		msg := tgbotapi.NewMessage(chatID, "Укажите название предмета. Формат: /pickup <предмет> [количество]")
		return b.sendMessage(msg)
	}

	req := inventoryapp.AddItemRequest{
		ChatID:   chatID,
		TgUserID: tgUserID,
		ItemName: itemName,
		Quantity: quantity,
		ItemType: "", // Определяется автоматически по названию
	}

	result, err := b.addItemUC.Execute(ctx, req)
	if err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при добавлении предмета: %v", err))
		return b.sendMessage(errorMsg)
	}

	return b.sendLongMessage(chatID, result)
}

func (b *Bot) handleRoll(ctx context.Context, chatID int64, args string) error {
	// Очищаем аргументы от лишних символов (обратные апострофы, кавычки и т.д.)
	cleanedArgs := strings.TrimSpace(args)
	cleanedArgs = strings.Trim(cleanedArgs, "`\"'")

	// Если есть pending ability check, /roll (или /roll d20) резолвит его
	if b.performAbilityCheckUC != nil {
		gs, err := b.sessionRepo.GetByChatID(ctx, chatID)
		if err == nil && gs != nil && gs.HasPendingAbilityCheck() {
			if cleanedArgs == "" || strings.EqualFold(cleanedArgs, "d20") {
				result, err := b.performAbilityCheckUC.Execute(ctx, chatID)
				if err != nil {
					errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при проверке: %v", err))
					return b.sendMessage(errorMsg)
				}

				// Отправляем результат проверки
				if err := b.sendLongMessage(chatID, result.Message); err != nil {
					return err
				}

				// Автоматически продолжаем повествование после проверки (если DM доступен)
				if b.handleActionUC != nil {
					// Получаем контекст проверки из сессии
					gs, _ := b.sessionRepo.GetByChatID(ctx, chatID)
					reason := ""
					stakes := ""
					if gs != nil {
						reason = gs.PendingAbilityCheckReason
						stakes = gs.PendingAbilityCheckStakes
					}

					var continueMessage string
					if result.Success {
						if reason != "" && stakes != "" {
							continueMessage = fmt.Sprintf("🎲 РЕЗУЛЬТАТ ПРОВЕРКИ: УСПЕХ! %s успешно %s (DC %d, бросок %d). Опиши положительные последствия: %s",
								result.CharacterName, reason, result.DC, result.Total, stakes)
						} else {
							continueMessage = fmt.Sprintf("🎲 РЕЗУЛЬТАТ ПРОВЕРКИ: УСПЕХ! %s прошел проверку %s (DC %d, бросок %d). Продолжи историю с положительными последствиями.",
								result.CharacterName, result.Ability, result.DC, result.Total)
						}
					} else {
						if reason != "" && stakes != "" {
							continueMessage = fmt.Sprintf("🎲 РЕЗУЛЬТАТ ПРОВЕРКИ: ПРОВАЛ! %s не смог %s (DC %d, бросок %d). Опиши негативные последствия: %s",
								result.CharacterName, reason, result.DC, result.Total, stakes)
						} else {
							continueMessage = fmt.Sprintf("🎲 РЕЗУЛЬТАТ ПРОВЕРКИ: ПРОВАЛ! %s провалил проверку %s (DC %d, бросок %d). Продолжи историю с негативными последствиями.",
								result.CharacterName, result.Ability, result.DC, result.Total)
						}
					}

					// Для автоматических продолжений используем tgUserID первого игрока
					systemTgUserID := int64(0)
					if gs.GetFirstPlayer() != nil {
						systemTgUserID = gs.GetFirstPlayer().TgUserID
					}
					return b.handlePlayerAction(ctx, chatID, systemTgUserID, continueMessage)
				}
				return nil
			}

			if manualRoll, ok := parseManualAbilityCheckInput(cleanedArgs); ok {
				result, err := b.performAbilityCheckUC.ExecuteWithBaseRoll(ctx, chatID, manualRoll)
				if err != nil {
					errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при проверке: %v", err))
					return b.sendMessage(errorMsg)
				}

				// Отправляем результат проверки
				if err := b.sendLongMessage(chatID, result.Message); err != nil {
					return err
				}

				// Автоматически продолжаем повествование после проверки (если DM доступен)
				if b.handleActionUC != nil {
					// Получаем контекст проверки из сессии
					gs, _ := b.sessionRepo.GetByChatID(ctx, chatID)
					reason := ""
					stakes := ""
					if gs != nil {
						reason = gs.PendingAbilityCheckReason
						stakes = gs.PendingAbilityCheckStakes
					}

					var continueMessage string
					if result.Success {
						if reason != "" && stakes != "" {
							continueMessage = fmt.Sprintf("🎲 РЕЗУЛЬТАТ ПРОВЕРКИ: УСПЕХ! %s успешно %s (DC %d, бросок %d). Опиши положительные последствия: %s",
								result.CharacterName, reason, result.DC, result.Total, stakes)
						} else {
							continueMessage = fmt.Sprintf("🎲 РЕЗУЛЬТАТ ПРОВЕРКИ: УСПЕХ! %s прошел проверку %s (DC %d, бросок %d). Продолжи историю с положительными последствиями.",
								result.CharacterName, result.Ability, result.DC, result.Total)
						}
					} else {
						if reason != "" && stakes != "" {
							continueMessage = fmt.Sprintf("🎲 РЕЗУЛЬТАТ ПРОВЕРКИ: ПРОВАЛ! %s не смог %s (DC %d, бросок %d). Опиши негативные последствия: %s",
								result.CharacterName, reason, result.DC, result.Total, stakes)
						} else {
							continueMessage = fmt.Sprintf("🎲 РЕЗУЛЬТАТ ПРОВЕРКИ: ПРОВАЛ! %s провалил проверку %s (DC %d, бросок %d). Продолжи историю с негативными последствиями.",
								result.CharacterName, result.Ability, result.DC, result.Total)
						}
					}

					// Для автоматических продолжений используем tgUserID первого игрока
					systemTgUserID := int64(0)
					if gs.GetFirstPlayer() != nil {
						systemTgUserID = gs.GetFirstPlayer().TgUserID
					}
					return b.handlePlayerAction(ctx, chatID, systemTgUserID, continueMessage)
				}
				return nil
			}
		}
	}

	if cleanedArgs == "" {
		msg := tgbotapi.NewMessage(chatID, "Укажите выражение для броска, например: /roll d20 или /roll 2d6+3.")
		return b.sendMessage(msg)
	}

	result, err := b.rollDiceUC.Execute(ctx, cleanedArgs)
	if err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при броске кубика: %v\n\nИспользуйте формат: /roll d20 или /roll 2d6+3", err))
		return b.sendMessage(errorMsg)
	}

	// Сохраняем результат броска в историю событий, чтобы DM мог его видеть
	if b.eventRepo != nil {
		gs, err := b.sessionRepo.GetByChatID(ctx, chatID)
		if err == nil && gs != nil && gs.IsActive() {
			// Создаем событие для результата броска
			rollEvent := &event.StoryEvent{
				GameSessionID: gs.ID,
				AuthorType:    event.AuthorTypePlayer,
				Content:       result, // Сохраняем полный результат броска
				CreatedAt:     time.Now(),
			}

			// Сохраняем событие (не блокируем отправку сообщения при ошибке)
			if err := b.eventRepo.Save(ctx, rollEvent); err != nil {
				logger.Warn("Failed to save dice roll event",
					logger.ErrorField(err),
					logger.Int64("chat_id", chatID),
					logger.Uint("session_id", gs.ID),
				)
			} else {
				logger.Debug("Dice roll event saved",
					logger.Int64("chat_id", chatID),
					logger.Uint("session_id", gs.ID),
					logger.String("result", result),
				)

				// Индексируем результат броска в RAG, чтобы DM мог его видеть
				if b.indexDocUC != nil {
					ragCtx, ragCancel := context.WithTimeout(ctx, 10*time.Second)
					defer ragCancel()

					doc := ragdomain.Document{
						ID:        uuid.New().String(),
						Source:    ragdomain.SourceEvent,
						SessionID: gs.ID,
						Text:      "Игрок бросил кубик: " + result,
						Timestamp: time.Now(),
					}

					// Индексируем с таймаутом (не блокируем отправку сообщения при ошибке)
					if err := b.indexDocUC.Execute(ragCtx, doc); err != nil {
						logger.Warn("Failed to index dice roll event in RAG (event saved in DB, but not indexed)",
							logger.ErrorField(err),
							logger.Int64("chat_id", chatID),
							logger.Uint("session_id", gs.ID),
						)
					} else {
						logger.Debug("Dice roll event indexed in RAG",
							logger.Int64("chat_id", chatID),
							logger.Uint("session_id", gs.ID),
						)
					}
				}
			}
		}
	}

	return b.sendLongMessage(chatID, result)
}

func (b *Bot) handleQuests(ctx context.Context, chatID int64) error {
	questsText, err := b.getQuestsUC.Execute(ctx, chatID)
	if err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при получении квестов: %v", err))
		return b.sendMessage(errorMsg)
	}

	return b.sendLongMessage(chatID, questsText)
}

func (b *Bot) handleDaily(ctx context.Context, chatID int64, tgUserID int64) error {
	dailyText, err := b.getDailyQuestsUC.Execute(ctx, chatID, tgUserID)
	if err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при получении ежедневных заданий: %v", err))
		return b.sendMessage(errorMsg)
	}

	return b.sendLongMessage(chatID, dailyText)
}

func (b *Bot) handleParty(ctx context.Context, chatID int64, tgUserID int64) error {
	// Получаем сессию
	gs, err := b.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при получении сессии: %v", err))
		return b.sendMessage(errorMsg)
	}
	if gs == nil {
		msg := tgbotapi.NewMessage(chatID, "Игра не начата. Используйте /newgame для начала новой игры.")
		return b.sendMessage(msg)
	}

	// Находим игрока
	player := gs.FindPlayerByTgUserID(tgUserID)
	if player == nil {
		player = gs.GetFirstPlayer()
		if player == nil {
			msg := tgbotapi.NewMessage(chatID, "Персонаж не создан. Используйте /createcharacter для создания персонажа.")
			return b.sendMessage(msg)
		}
	}

	var result strings.Builder
	result.WriteString("👥 Ваш отряд\n\n")

	// Информация об игроке
	result.WriteString(fmt.Sprintf("🎯 **Лидер:** %s (%s %d уровня)\n", player.Character.Name, player.Character.Class, player.Character.Level))
	result.WriteString(fmt.Sprintf("   Здоровье: %d/%d HP\n\n", player.Character.HP, player.Character.MaxHP))

	// Информация о компаньонах
	if len(gs.Companions) > 0 {
		result.WriteString("🛡️ **Компаньоны:**\n")
		for i, companion := range gs.Companions {
			status := "❤️"
			if companion.HP <= 0 {
				status = "💀"
			} else if companion.HP < companion.MaxHP/2 {
				status = "💔"
			}

			result.WriteString(fmt.Sprintf("%d. %s %s (%s %d ур)\n", i+1, status, companion.Name, companion.Class, companion.Level))
			result.WriteString(fmt.Sprintf("   Здоровье: %d/%d HP, Защита: %d\n", companion.HP, companion.MaxHP, companion.AC))
		}
	} else {
		result.WriteString("🛡️ **Компаньоны:** Нет компаньонов\n")
	}

	// Статистика отряда
	totalHP := player.Character.HP
	totalMaxHP := player.Character.MaxHP
	for _, companion := range gs.Companions {
		if companion.HP > 0 {
			totalHP += companion.HP
			totalMaxHP += companion.MaxHP
		}
	}

	result.WriteString(fmt.Sprintf("\n📊 **Общая сила отряда:** %d/%d HP\n", totalHP, totalMaxHP))
	result.WriteString(fmt.Sprintf("👤 **Количество участников:** %d\n", 1+len(gs.Companions)))

	return b.sendLongMessage(chatID, result.String())
}

func (b *Bot) handleDismiss(ctx context.Context, chatID int64, tgUserID int64, args string) error {
	if args == "" {
		msg := tgbotapi.NewMessage(chatID, "Укажите имя компаньона для увольнения. Пример: /dismiss Алара")
		return b.sendMessage(msg)
	}

	// Получаем сессию
	gs, err := b.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при получении сессии: %v", err))
		return b.sendMessage(errorMsg)
	}
	if gs == nil {
		msg := tgbotapi.NewMessage(chatID, "Игра не начата. Используйте /newgame для начала новой игры.")
		return b.sendMessage(msg)
	}

	// Ищем компаньона по имени
	var foundCompanion *session.Companion
	var companionIndex = -1
	for i, companion := range gs.Companions {
		if strings.EqualFold(companion.Name, args) {
			foundCompanion = &gs.Companions[i]
			companionIndex = i
			break
		}
	}

	if foundCompanion == nil {
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Компаньон '%s' не найден в вашем отряде.", args))
		return b.sendMessage(msg)
	}

	// Удаляем компаньона
	gs.Companions = append(gs.Companions[:companionIndex], gs.Companions[companionIndex+1:]...)

	// Сохраняем сессию
	if err := b.sessionRepo.Save(ctx, gs); err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при сохранении сессии: %v", err))
		return b.sendMessage(errorMsg)
	}

	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("✅ Компаньон %s покинул ваш отряд.", foundCompanion.Name))
	return b.sendMessage(msg)
}

func (b *Bot) handlePreferences(ctx context.Context, chatID int64, tgUserID int64) error {
	// Получаем сессию
	gs, err := b.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при получении сессии: %v", err))
		return b.sendMessage(errorMsg)
	}
	if gs == nil {
		msg := tgbotapi.NewMessage(chatID, "Игра не начата. Используйте /newgame для начала новой игры.")
		return b.sendMessage(msg)
	}

	// Находим игрока
	player := gs.FindPlayerByTgUserID(tgUserID)
	if player == nil {
		player = gs.GetFirstPlayer()
		if player == nil {
			msg := tgbotapi.NewMessage(chatID, "Персонаж не создан. Используйте /createcharacter для создания персонажа.")
			return b.sendMessage(msg)
		}
	}

	// Получаем текущие настройки
	prefs := player.GetPreferences()

	var result strings.Builder
	result.WriteString("⚙️ Настройки персонализации\n\n")
	result.WriteString("Текущие настройки:\n")
	result.WriteString(fmt.Sprintf("📖 Стиль повествования: %s\n", getNarrativeStyleName(prefs.NarrativeStyle)))
	result.WriteString(fmt.Sprintf("📝 Уровень детализации: %s\n", getDetailLevelName(prefs.DetailLevel)))
	result.WriteString(fmt.Sprintf("🌐 Язык: %s\n", prefs.Language))
	result.WriteString(fmt.Sprintf("📊 Показывать статистику: %s\n\n", boolToEmoji(prefs.ShowStats)))

	result.WriteString("Для изменения настроек используйте:\n")
	result.WriteString("/set_style <balanced|dark|light|detailed|minimalist>\n")
	result.WriteString("/set_detail <low|medium|high>\n")
	result.WriteString("/set_language <ru|en>\n")
	result.WriteString("/toggle_stats\n")

	return b.sendLongMessage(chatID, result.String())
}

// Вспомогательные функции для отображения настроек
func getNarrativeStyleName(style player.NarrativeStyle) string {
	switch style {
	case player.NarrativeStyleDark:
		return "Темный 🌑"
	case player.NarrativeStyleLight:
		return "Светлый ☀️"
	case player.NarrativeStyleDetailed:
		return "Детализированный 📖"
	case player.NarrativeStyleMinimalist:
		return "Минималистичный 📄"
	default:
		return "Сбалансированный ⚖️"
	}
}

func getDetailLevelName(level player.DetailLevel) string {
	switch level {
	case player.DetailLevelLow:
		return "Низкий 📉"
	case player.DetailLevelHigh:
		return "Высокий 📈"
	default:
		return "Средний 📊"
	}
}

func boolToEmoji(b bool) string {
	if b {
		return "✅"
	}
	return "❌"
}

func (b *Bot) handleSetStyle(ctx context.Context, chatID int64, tgUserID int64, args string) error {
	if args == "" {
		msg := tgbotapi.NewMessage(chatID, "Укажите стиль: balanced, dark, light, detailed, minimalist")
		return b.sendMessage(msg)
	}

	var newStyle player.NarrativeStyle
	switch strings.ToLower(args) {
	case "balanced":
		newStyle = player.NarrativeStyleBalanced
	case "dark":
		newStyle = player.NarrativeStyleDark
	case "light":
		newStyle = player.NarrativeStyleLight
	case "detailed":
		newStyle = player.NarrativeStyleDetailed
	case "minimalist":
		newStyle = player.NarrativeStyleMinimalist
	default:
		msg := tgbotapi.NewMessage(chatID, "Неверный стиль. Используйте: balanced, dark, light, detailed, minimalist")
		return b.sendMessage(msg)
	}

	return b.updatePlayerPreference(ctx, chatID, tgUserID, func(prefs *player.UserPreferences) {
		prefs.NarrativeStyle = newStyle
	}, func(prefs player.UserPreferences) string {
		return fmt.Sprintf("✅ Стиль повествования изменен на: %s", getNarrativeStyleName(newStyle))
	})
}

func (b *Bot) handleSetDetail(ctx context.Context, chatID int64, tgUserID int64, args string) error {
	if args == "" {
		msg := tgbotapi.NewMessage(chatID, "Укажите уровень детализации: low, medium, high")
		return b.sendMessage(msg)
	}

	var newLevel player.DetailLevel
	switch strings.ToLower(args) {
	case "low":
		newLevel = player.DetailLevelLow
	case "medium":
		newLevel = player.DetailLevelMedium
	case "high":
		newLevel = player.DetailLevelHigh
	default:
		msg := tgbotapi.NewMessage(chatID, "Неверный уровень. Используйте: low, medium, high")
		return b.sendMessage(msg)
	}

	return b.updatePlayerPreference(ctx, chatID, tgUserID, func(prefs *player.UserPreferences) {
		prefs.DetailLevel = newLevel
	}, func(prefs player.UserPreferences) string {
		return fmt.Sprintf("✅ Уровень детализации изменен на: %s", getDetailLevelName(newLevel))
	})
}

func (b *Bot) handleSetLanguage(ctx context.Context, chatID int64, tgUserID int64, args string) error {
	if args == "" {
		msg := tgbotapi.NewMessage(chatID, "Укажите язык: ru, en")
		return b.sendMessage(msg)
	}

	language := strings.ToLower(args)
	if language != "ru" && language != "en" {
		msg := tgbotapi.NewMessage(chatID, "Неверный язык. Используйте: ru, en")
		return b.sendMessage(msg)
	}

	return b.updatePlayerPreference(ctx, chatID, tgUserID, func(prefs *player.UserPreferences) {
		prefs.Language = language
	}, func(prefs player.UserPreferences) string {
		return fmt.Sprintf("✅ Язык изменен на: %s", strings.ToUpper(language))
	})
}

func (b *Bot) handleToggleStats(ctx context.Context, chatID int64, tgUserID int64) error {
	return b.updatePlayerPreference(ctx, chatID, tgUserID, func(prefs *player.UserPreferences) {
		prefs.ShowStats = !prefs.ShowStats
	}, func(prefs player.UserPreferences) string {
		return fmt.Sprintf("✅ Отображение статистики: %s", boolToEmoji(prefs.ShowStats))
	})
}

// updatePlayerPreference обновляет настройки игрока
func (b *Bot) updatePlayerPreference(ctx context.Context, chatID int64, tgUserID int64, updateFunc func(*player.UserPreferences), messageFunc func(player.UserPreferences) string) error {
	// Получаем сессию
	gs, err := b.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при получении сессии: %v", err))
		return b.sendMessage(errorMsg)
	}
	if gs == nil {
		msg := tgbotapi.NewMessage(chatID, "Игра не начата.")
		return b.sendMessage(msg)
	}

	// Находим игрока
	player := gs.FindPlayerByTgUserID(tgUserID)
	if player == nil {
		player = gs.GetFirstPlayer()
		if player == nil {
			msg := tgbotapi.NewMessage(chatID, "Персонаж не создан.")
			return b.sendMessage(msg)
		}
	}

	// Получаем текущие настройки
	prefs := player.GetPreferences()

	// Применяем изменения
	updateFunc(&prefs)

	// Сохраняем настройки
	if err := player.SetPreferences(prefs); err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при сохранении настроек: %v", err))
		return b.sendMessage(errorMsg)
	}

	// Сохраняем сессию
	if err := b.sessionRepo.Save(ctx, gs); err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при сохранении сессии: %v", err))
		return b.sendMessage(errorMsg)
	}

	msg := tgbotapi.NewMessage(chatID, messageFunc(prefs))
	return b.sendMessage(msg)
}

func (b *Bot) handleMap(ctx context.Context, chatID int64) error {
	// Получаем сессию (нужно для текущей локации и картинки карты)
	gs, err := b.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при получении сессии: %v", err))
		return b.sendMessage(errorMsg)
	}
	if gs == nil {
		msg := tgbotapi.NewMessage(chatID, "Игра не начата. Используйте /newgame для начала новой игры.")
		return b.sendMessage(msg)
	}

	// Если есть красивая карта-изображение — показываем её
	if gs.MapImagePath != "" {
		if err := b.sendPhoto(ctx, chatID, gs.MapImagePath, "🗺️ Карта мира"); err != nil {
			logger.Warn("Failed to send map image",
				logger.ErrorField(err),
				logger.Int64("chat_id", chatID),
			)
		}
	}

	mapText, err := b.getMapUC.Execute(ctx, chatID)
	if err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при получении карты: %v", err))
		return b.sendMessage(errorMsg)
	}

	// 1) Отправляем текст карты (может быть длинным)
	if err := b.sendLongMessage(chatID, mapText); err != nil {
		return err
	}

	// 2) Отправляем кнопки навигации отдельным коротким сообщением
	markup := b.buildMapNavigationKeyboard(gs)
	if markup == nil {
		return nil
	}
	navMsg := tgbotapi.NewMessage(chatID, "🧭 Куда идём дальше?")
	navMsg.ReplyMarkup = markup
	return b.sendMessage(navMsg)
}

func (b *Bot) buildMapNavigationKeyboard(gs *session.GameSession) *tgbotapi.InlineKeyboardMarkup {
	if gs == nil || len(gs.World.Locations) == 0 {
		return nil
	}

	// Быстрый доступ к локациям
	locationMap := make(map[uint]*world.Location, len(gs.World.Locations))
	for i := range gs.World.Locations {
		loc := &gs.World.Locations[i]
		locationMap[loc.ID] = loc
	}

	// Текущая локация
	var current *world.Location
	if gs.CurrentLocationID != nil {
		current = locationMap[*gs.CurrentLocationID]
	}
	if current == nil || len(current.Connections) == 0 {
		// Если текущая локация не задана или без связей, ищем первую с доступными связями
		for i := range gs.World.Locations {
			if len(gs.World.Locations[i].Connections) > 0 {
				current = &gs.World.Locations[i]
				break
			}
		}
	}
	if current == nil || len(current.Connections) == 0 {
		return nil
	}

	var rows [][]tgbotapi.InlineKeyboardButton
	var row []tgbotapi.InlineKeyboardButton

	// Кнопки по направлениям
	for _, conn := range current.Connections {
		toName := "???"
		if toLoc := locationMap[conn.ToLocationID]; toLoc != nil {
			toName = toLoc.Name
		}
		dirSym := mapDirectionSymbol(conn.Direction)
		btnText := fmt.Sprintf("%s %s", dirSym, toName)
		btnData := fmt.Sprintf("map_to_%d", conn.ToLocationID)
		row = append(row, tgbotapi.NewInlineKeyboardButtonData(btnText, btnData))
		if len(row) == 2 {
			rows = append(rows, row)
			row = nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}

	m := tgbotapi.NewInlineKeyboardMarkup(rows...)
	return &m
}

func mapDirectionSymbol(direction string) string {
	switch strings.ToLower(direction) {
	case "north", "n":
		return "⬆️"
	case "south", "s":
		return "⬇️"
	case "east", "e":
		return "➡️"
	case "west", "w":
		return "⬅️"
	case "up", "u":
		return "⬆️⬆️"
	case "down", "d":
		return "⬇️⬇️"
	case "portal":
		return "🌀"
	case "path", "road":
		return "🛤️"
	default:
		return "→"
	}
}

func (b *Bot) handleAchievements(ctx context.Context, chatID int64, tgUserID int64) error {
	if b.getAchievementsUC == nil {
		msg := tgbotapi.NewMessage(chatID, "Система достижений временно недоступна.")
		return b.sendMessage(msg)
	}

	req := achievementapp.GetAchievementsRequest{
		ChatID:   chatID,
		TgUserID: tgUserID,
	}

	achievementsText, err := b.getAchievementsUC.Execute(ctx, req)
	if err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при получении достижений: %v", err))
		return b.sendMessage(errorMsg)
	}

	return b.sendLongMessage(chatID, achievementsText)
}

// handleSpells обрабатывает команду /spells для просмотра заклинаний
func (b *Bot) handleSpells(ctx context.Context, chatID int64, tgUserID int64) error {
	req := spellapp.GetSpellsRequest{
		ChatID:   chatID,
		TgUserID: tgUserID,
	}

	spellsText, err := b.getSpellsUC.Execute(ctx, req)
	if err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при получении заклинаний: %v", err))
		return b.sendMessage(errorMsg)
	}

	return b.sendLongMessage(chatID, spellsText)
}

// handleCast обрабатывает команду /cast для использования заклинания
func (b *Bot) handleCast(ctx context.Context, chatID int64, tgUserID int64, args string) error {
	if b.useSpellUC == nil {
		msg := tgbotapi.NewMessage(chatID, "Система использования заклинаний временно недоступна.")
		return b.sendMessage(msg)
	}

	// Парсим аргументы: /cast <название_заклинания> [цель]
	parts := strings.Fields(args)
	if len(parts) == 0 {
		msg := tgbotapi.NewMessage(chatID, `✨ Использование заклинания

Используйте команду:
/cast <название_заклинания> [цель]

Примеры:
/cast Огненный снаряд
/cast Лечение ран
/cast Магическая стрела goblin

Используйте /spells для просмотра доступных заклинаний.`)
		return b.sendMessage(msg)
	}

	spellName := parts[0]
	target := ""
	if len(parts) > 1 {
		target = strings.Join(parts[1:], " ")
	}

	req := spellapp.UseSpellRequest{
		ChatID:    chatID,
		TgUserID:  tgUserID,
		SpellName: spellName,
		Target:    target,
	}

	resp, err := b.useSpellUC.Execute(ctx, req)
	if err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при использовании заклинания: %v", err))
		return b.sendMessage(errorMsg)
	}

	if !resp.Success {
		errorMsg := tgbotapi.NewMessage(chatID, resp.Message)
		return b.sendMessage(errorMsg)
	}

	return b.sendLongMessage(chatID, resp.Message)
}

// handleSubscription обрабатывает команду /subscription для просмотра информации о подписке
func (b *Bot) handleSubscription(ctx context.Context, chatID int64, tgUserID int64) error {
	if b.getSubscriptionUC == nil {
		msg := tgbotapi.NewMessage(chatID, "Система подписок временно недоступна.")
		return b.sendMessage(msg)
	}

	req := subscriptionapp.GetSubscriptionRequest{
		TgUserID: tgUserID,
	}

	resp, err := b.getSubscriptionUC.Execute(ctx, req)
	if err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при получении информации о подписке: %v", err))
		return b.sendMessage(errorMsg)
	}

	// Формируем подробное сообщение о подписке
	var message strings.Builder
	message.WriteString(resp.Message)
	message.WriteString("\n\n")

	details := resp.PlanDetails
	message.WriteString(fmt.Sprintf("📋 Тариф: %s\n", details.Name))

	if details.Price > 0 {
		message.WriteString(fmt.Sprintf("💰 Цена: %d₽/мес\n", details.Price))
	}

	message.WriteString("\n📊 Лимиты:\n")
	if details.MaxActiveGames == 0 {
		message.WriteString("  ✅ Активных игр: безлимит\n")
	} else {
		message.WriteString(fmt.Sprintf("  📝 Активных игр: %d\n", details.MaxActiveGames))
	}

	if details.MaxMessagesPerDay == 0 {
		message.WriteString("  ✅ Сообщений/день: безлимит\n")
	} else {
		message.WriteString(fmt.Sprintf("  💬 Сообщений/день: %d\n", details.MaxMessagesPerDay))
	}

	if details.MaxImagesPerDay == 0 {
		message.WriteString("  ✅ Изображений/день: безлимит\n")
	} else {
		message.WriteString(fmt.Sprintf("  🖼️ Изображений/день: %d\n", details.MaxImagesPerDay))
	}

	if details.MaxSaves == 0 {
		message.WriteString("  ✅ Сохранений: безлимит\n")
	} else {
		message.WriteString(fmt.Sprintf("  💾 Сохранений: %d\n", details.MaxSaves))
	}

	message.WriteString(fmt.Sprintf("  🎒 Слотов инвентаря: %d\n", details.MaxInventorySlots))

	if resp.DaysRemaining > 0 {
		message.WriteString(fmt.Sprintf("\n⏰ Осталось дней: %d", resp.DaysRemaining))
	} else if resp.DaysRemaining == -1 {
		message.WriteString("\n✨ Бессрочная подписка")
	}

	return b.sendLongMessage(chatID, message.String())
}

// handleSubscribe обрабатывает команду /subscribe для оформления подписки
func (b *Bot) handleSubscribe(ctx context.Context, chatID int64, tgUserID int64, args string) error {
	if b.getSubscriptionUC == nil {
		msg := tgbotapi.NewMessage(chatID, "Система подписок временно недоступна.")
		return b.sendMessage(msg)
	}

	// Получаем текущую подписку
	req := subscriptionapp.GetSubscriptionRequest{
		TgUserID: tgUserID,
	}

	resp, err := b.getSubscriptionUC.Execute(ctx, req)
	if err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при получении информации о подписке: %v", err))
		return b.sendMessage(errorMsg)
	}

	// Формируем сообщение с доступными тарифами
	var message strings.Builder
	message.WriteString("💎 Доступные тарифы:\n\n")

	message.WriteString("🆓 Free - Бесплатно\n")
	message.WriteString("  • 1 активная игра\n")
	message.WriteString("  • 50 сообщений/день\n")
	message.WriteString("  • 5 изображений/день\n")
	message.WriteString("  • 1 сохранение\n")
	message.WriteString("  • 30 слотов инвентаря\n\n")

	message.WriteString("⭐ Premium - 299₽/мес\n")
	message.WriteString("  • Безлимит игр\n")
	message.WriteString("  • Безлимит сообщений\n")
	message.WriteString("  • Безлимит изображений\n")
	message.WriteString("  • 10 сохранений\n")
	message.WriteString("  • 50 слотов инвентаря\n")
	message.WriteString("  • Приоритетная обработка\n")
	message.WriteString("  • Эксклюзивные миры\n")
	message.WriteString("  • Приоритетная поддержка\n\n")

	message.WriteString("👑 Pro - 599₽/мес\n")
	message.WriteString("  • Все из Premium\n")
	message.WriteString("  • Мультиплеер до 8 игроков\n")
	message.WriteString("  • API доступ\n")
	message.WriteString("  • Кастомные моды\n")
	message.WriteString("  • 70 слотов инвентаря\n\n")

	if resp.Subscription.IsActive() && resp.Subscription.Plan != subscription.PlanFree {
		message.WriteString(fmt.Sprintf("ℹ️ У вас уже активна подписка %s\n", resp.Subscription.Plan))
		if resp.DaysRemaining > 0 {
			message.WriteString(fmt.Sprintf("Осталось дней: %d\n", resp.DaysRemaining))
		}
	} else {
		message.WriteString("⚠️ Интеграция с платежными системами в разработке.\n")
		message.WriteString("Для оформления подписки свяжитесь с поддержкой.")
	}

	return b.sendLongMessage(chatID, message.String())
}

// handleImage обрабатывает команду /image для генерации изображений
func (b *Bot) handleImage(ctx context.Context, chatID int64, tgUserID int64, args string) error {
	if b.generateImageUC == nil {
		msg := tgbotapi.NewMessage(chatID, "Генерация изображений временно недоступна.")
		return b.sendMessage(msg)
	}

	// Проверяем лимит изображений для пользователя (если доступна проверка подписки)
	var skipLimitCheck bool
	if b.checkLimitsUC != nil {
		// Проверяем лимит изображений
		limitReq := subscriptionapp.CheckLimitRequest{
			TgUserID:  tgUserID,
			LimitType: subscriptionapp.LimitTypeImagesPerDay,
		}
		limitResp, err := b.checkLimitsUC.Execute(ctx, limitReq)
		if err == nil {
			if !limitResp.Allowed {
				msg := tgbotapi.NewMessage(chatID, limitResp.Message)
				return b.sendMessage(msg)
			}
			// Если лимит 0 (безлимит), пропускаем проверку лимитера
			if limitResp.Limit == 0 {
				skipLimitCheck = true
			}
		}
	}

	// Если аргументы не указаны, показываем справку
	if args == "" {
		msg := tgbotapi.NewMessage(chatID, `🎨 Генерация изображений

Используйте команду:
/image <описание>

Примеры:
/image розовый кот
/image древний лес с магическими рунами
/image эльфийский воин в доспехах

📝 Генерация изображений ограничена 5 изображениями в день для Free пользователей.
Для Premium пользователей лимит снят.

💡 Изображения автоматически кэшируются для повторного использования.`)
		return b.sendMessage(msg)
	}

	// Отправляем сообщение о начале генерации
	statusMsg := tgbotapi.NewMessage(chatID, "🎨 Генерирую изображение... Это может занять несколько секунд.")
	if err := b.sendMessage(statusMsg); err != nil {
		logger.Warn("Failed to send status message",
			logger.ErrorField(err),
			logger.Int64("chat_id", chatID),
		)
	}

	// Генерируем изображение
	req := imageapp.GenerateImageRequest{
		SystemPrompt:    "Ты — талантливый художник в стиле фэнтези и D&D. Создавай детализированные и атмосферные изображения.",
		UserPrompt:      args,
		Type:            "custom",
		EntityID:        0,
		ForceRegenerate: false,
		UserID:          tgUserID,
		SkipLimitCheck:  skipLimitCheck, // Проверяется через checkLimitsUC
	}

	resp, err := b.generateImageUC.Execute(ctx, req)
	if err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при генерации изображения: %v\n\nВозможно, достигнут дневной лимит генерации (5 изображений/день).", err))
		return b.sendMessage(errorMsg)
	}

	// Отправляем изображение
	if err := b.sendPhoto(ctx, chatID, resp.ImagePath, "🎨 Ваше изображение готово!"); err != nil {
		logger.Error("Failed to send image",
			logger.ErrorField(err),
			logger.Int64("chat_id", chatID),
			logger.String("image_path", resp.ImagePath),
		)
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Изображение сгенерировано, но произошла ошибка при отправке: %v", err))
		return b.sendMessage(errorMsg)
	}

	return nil
}

// sendPhoto отправляет фото через Telegram Bot API
func (b *Bot) sendPhoto(ctx context.Context, chatID int64, photoPath string, caption string) error {
	photo := tgbotapi.NewPhoto(chatID, tgbotapi.FilePath(photoPath))
	photo.Caption = caption

	_, err := b.api.Send(photo)
	if err != nil {
		return fmt.Errorf("failed to send photo: %w", err)
	}

	return nil
}

// extractImageMarkers извлекает пути к изображениям из маркеров [IMAGE:path] в тексте
func (b *Bot) extractImageMarkers(text string) []string {
	// Регулярное выражение для поиска маркеров [IMAGE:path]
	re := regexp.MustCompile(`\[IMAGE:([^\]]+)\]`)
	matches := re.FindAllStringSubmatch(text, -1)

	if len(matches) == 0 {
		return nil
	}

	imagePaths := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) >= 2 {
			imagePaths = append(imagePaths, match[1])
		}
	}

	return imagePaths
}

// handleAttack обрабатывает команду /attack или боевое действие игрока
// action - опциональное описание действия (например, "атакую мечом")
// Если action пустое, используется стандартная атака
func (b *Bot) handleAttack(ctx context.Context, chatID int64, action string) error {
	// Если action пустое, используем стандартное описание
	if action == "" {
		action = "атакую"
	}

	logger.Info("Handling combat action",
		logger.Int64("chat_id", chatID),
		logger.String("action", action),
	)

	// Вызываем боевую систему
	result, err := b.handleCombatUC.Execute(ctx, chatID, action)
	if err != nil {
		logger.Error("Failed to handle combat action",
			logger.ErrorField(err),
			logger.Int64("chat_id", chatID),
		)
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка: %v", err))
		return b.sendMessage(errorMsg)
	}

	// Отправляем результат боя
	return b.sendLongMessage(chatID, result)
}

// handleBattlefield обрабатывает команду /battlefield для отображения поля боя
func (b *Bot) handleBattlefield(ctx context.Context, chatID int64, args string) error {
	if b.sessionRepo == nil {
		logger.Error("Session repository is not initialized for battlefield")
		errorMsg := tgbotapi.NewMessage(chatID, "Ошибка: репозиторий сессий недоступен")
		return b.sendMessage(errorMsg)
	}
	if b.combatRepo == nil {
		logger.Error("Combat repository is not initialized for battlefield")
		errorMsg := tgbotapi.NewMessage(chatID, "Ошибка: боевая система недоступна")
		return b.sendMessage(errorMsg)
	}
	// Получаем сессию
	gs, err := b.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil {
		logger.Error("Failed to get session",
			logger.ErrorField(err),
			logger.Int64("chat_id", chatID),
		)
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка: %v", err))
		return b.sendMessage(errorMsg)
	}

	if gs == nil {
		msg := tgbotapi.NewMessage(chatID, "Игра не начата. Используйте /newgame для начала новой игры.")
		return b.sendMessage(msg)
	}

	// Парсим формат из аргументов
	format := "table"
	if args != "" {
		parts := strings.Fields(args)
		if len(parts) > 0 {
			format = strings.ToLower(parts[0])
			// Валидация формата
			if format != "table" && format != "compact" && format != "detailed" {
				format = "table"
			}
		}
	}

	// Получаем активный бой напрямую через combatRepo для надежности
	activeCombat, err := b.combatRepo.GetActiveBySessionID(ctx, gs.ID)
	if err != nil {
		logger.Error("Failed to get combat",
			logger.ErrorField(err),
			logger.Uint("session_id", gs.ID),
		)
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при получении боя: %v", err))
		return b.sendMessage(errorMsg)
	}

	if activeCombat == nil || !activeCombat.IsActive() {
		msg := tgbotapi.NewMessage(chatID, "Сейчас нет активного боя.")
		return b.sendMessage(msg)
	}

	// Создаем адаптер для CombatRepository из bot.go к dm_tools.CombatRepository
	combatRepoAdapter := &combatRepositoryAdapter{repo: b.combatRepo}

	// Используем GetBattlefieldStatusTool для форматирования поля боя
	tool := dm_tools.NewGetBattlefieldStatusTool(combatRepoAdapter, gs.ID)

	// Выполняем tool напрямую с нужным форматом
	toolArgs := map[string]interface{}{
		"format": format,
	}

	result, err := tool.Execute(ctx, toolArgs)
	if err != nil {
		logger.Error("Failed to get battlefield status",
			logger.ErrorField(err),
			logger.Uint("session_id", gs.ID),
		)
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при получении поля боя: %v", err))
		return b.sendMessage(errorMsg)
	}

	// Извлекаем визуализацию из результата tool
	resultMap, ok := result.(map[string]interface{})
	if !ok {
		logger.Error("Invalid battlefield tool result format",
			logger.String("result_type", fmt.Sprintf("%T", result)),
			logger.Uint("session_id", gs.ID),
		)
		errorMsg := tgbotapi.NewMessage(chatID, "Ошибка: неверный формат результата")
		return b.sendMessage(errorMsg)
	}

	// Логируем результат для отладки
	logger.Debug("Battlefield tool result",
		logger.Int64("chat_id", chatID),
		logger.Uint("session_id", gs.ID),
		logger.String("format", format),
		logger.Any("result_keys", getMapKeys(resultMap)),
	)

	battlefieldView, ok := resultMap["battlefield"].(string)
	if !ok {
		logger.Error("Battlefield field not found in tool result",
			logger.Int64("chat_id", chatID),
			logger.Uint("session_id", gs.ID),
			logger.Any("result_keys", getMapKeys(resultMap)),
		)
		errorMsg := tgbotapi.NewMessage(chatID, "Ошибка: поле battlefield не найдено")
		return b.sendMessage(errorMsg)
	}

	if battlefieldView == "" {
		logger.Error("Battlefield view is empty",
			logger.Int64("chat_id", chatID),
			logger.Uint("session_id", gs.ID),
			logger.String("format", format),
			logger.Int("participants", len(activeCombat.Participants)),
			logger.String("combat_state", string(activeCombat.State)),
		)
		// Если нет поля боя, проверяем сообщение
		if msg, ok := resultMap["message"].(string); ok {
			return b.sendLongMessage(chatID, msg)
		}
		errorMsg := tgbotapi.NewMessage(chatID, "Не удалось получить визуализацию поля боя (пустой результат)")
		return b.sendMessage(errorMsg)
	}

	// Добавляем заголовок с форматом
	header := fmt.Sprintf("⚔️ Поле боя (формат: %s)\n\n", format)
	return b.sendLongMessage(chatID, header+battlefieldView)
}

// handleAbilities обрабатывает команду /abilities для отображения способностей персонажа
func (b *Bot) handleAbilities(ctx context.Context, chatID int64, args string) error {
	// Получаем сессию
	gs, err := b.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil {
		logger.Error("Failed to get session",
			logger.ErrorField(err),
			logger.Int64("chat_id", chatID),
		)
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка: %v", err))
		return b.sendMessage(errorMsg)
	}

	if gs == nil {
		msg := tgbotapi.NewMessage(chatID, "Игра не начата. Используйте /newgame для начала новой игры.")
		return b.sendMessage(msg)
	}

	// Парсим фильтр из аргументов
	filterType := "all"
	if args != "" {
		parts := strings.Fields(args)
		if len(parts) > 0 {
			filterType = strings.ToLower(parts[0])
			// Валидация фильтра
			if filterType != "all" && filterType != "spells" && filterType != "feats" && filterType != "class" {
				filterType = "all"
			}
		}
	}

	// Используем GetCharacterAbilitiesTool напрямую, без вызова DM
	// Создаем адаптер для SessionRepository из bot.go к dm_tools.SessionRepository
	adapter := &sessionRepoAdapter{sessionRepo: b.sessionRepo}
	tool := dm_tools.NewGetCharacterAbilitiesTool(adapter, chatID)

	// Выполняем tool напрямую с нужным фильтром
	toolArgs := map[string]interface{}{
		"filter_type": filterType,
	}

	result, err := tool.Execute(ctx, toolArgs)
	if err != nil {
		logger.Error("Failed to get abilities",
			logger.ErrorField(err),
			logger.Int64("chat_id", chatID),
		)
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка: %v\n\nИспользуйте: /abilities [all|spells|feats|class]", err))
		return b.sendMessage(errorMsg)
	}

	// Форматируем результат для отображения
	resultMap, ok := result.(map[string]interface{})
	if !ok {
		errorMsg := tgbotapi.NewMessage(chatID, "Ошибка: неверный формат результата")
		return b.sendMessage(errorMsg)
	}

	// Формируем читаемое сообщение со способностями
	var parts []string
	parts = append(parts, fmt.Sprintf("📊 Способности персонажа: %s", resultMap["character_name"]))
	parts = append(parts, fmt.Sprintf("⚔️ Класс: %s | 📊 Уровень: %v", resultMap["character_class"], resultMap["character_level"]))

	if filterType != "all" {
		parts = append(parts, fmt.Sprintf("🔍 Фильтр: %s", filterType))
	}

	parts = append(parts, "")

	abilities, ok := resultMap["abilities"].([]interface{})
	if !ok || len(abilities) == 0 {
		parts = append(parts, "У персонажа нет способностей выбранного типа.")
	} else {
		parts = append(parts, fmt.Sprintf("Всего способностей: %v\n", resultMap["total_abilities"]))

		// Группируем способности по типу
		spells := []map[string]interface{}{}
		feats := []map[string]interface{}{}
		classAbilities := []map[string]interface{}{}

		for _, ab := range abilities {
			abMap, ok := ab.(map[string]interface{})
			if !ok {
				continue
			}

			abType, _ := abMap["type"].(string)
			switch abType {
			case "spell":
				spells = append(spells, abMap)
			case "feat":
				feats = append(feats, abMap)
			case "class":
				classAbilities = append(classAbilities, abMap)
			}
		}

		// Выводим способности по группам
		if len(classAbilities) > 0 && (filterType == "all" || filterType == "class") {
			parts = append(parts, "⚔️ Классовые способности:")
			for _, ab := range classAbilities {
				name, _ := ab["name"].(string)
				desc, _ := ab["description"].(string)
				useType, _ := ab["use_type"].(string)
				parts = append(parts, fmt.Sprintf("  • %s (%s)", name, useType))
				parts = append(parts, fmt.Sprintf("    %s", desc))
				if usesPerDay, ok := ab["uses_per_day"].(float64); ok && usesPerDay > 0 {
					usesRemaining, _ := ab["uses_remaining"].(float64)
					parts = append(parts, fmt.Sprintf("    Использований: %.0f/%.0f в день", usesRemaining, usesPerDay))
				}
				parts = append(parts, "")
			}
		}

		if len(spells) > 0 && (filterType == "all" || filterType == "spells") {
			parts = append(parts, "🔮 Заклинания:")
			for _, ab := range spells {
				name, _ := ab["name"].(string)
				desc, _ := ab["description"].(string)
				spellLevel, _ := ab["spell_level"].(float64)
				spellSchool, _ := ab["spell_school"].(string)
				parts = append(parts, fmt.Sprintf("  • %s (Уровень %.0f, %s)", name, spellLevel, spellSchool))
				parts = append(parts, fmt.Sprintf("    %s", desc))
				parts = append(parts, "")
			}
		}

		if len(feats) > 0 && (filterType == "all" || filterType == "feats") {
			parts = append(parts, "⭐ Перки:")
			for _, ab := range feats {
				name, _ := ab["name"].(string)
				desc, _ := ab["description"].(string)
				parts = append(parts, fmt.Sprintf("  • %s", name))
				parts = append(parts, fmt.Sprintf("    %s", desc))
				parts = append(parts, "")
			}
		}
	}

	return b.sendLongMessage(chatID, strings.Join(parts, "\n"))
}

// handleFlee обрабатывает команду /flee для выхода из боя
func (b *Bot) handleFlee(ctx context.Context, chatID int64) error {
	// Получаем сессию
	gs, err := b.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil {
		logger.Error("Failed to get session",
			logger.ErrorField(err),
			logger.Int64("chat_id", chatID),
		)
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при получении сессии: %v", err))
		return b.sendMessage(errorMsg)
	}

	if gs == nil {
		msg := tgbotapi.NewMessage(chatID, "Игра не начата. Используйте /newgame для начала новой игры.")
		return b.sendMessage(msg)
	}

	// Получаем активный бой
	if b.combatRepo == nil {
		msg := tgbotapi.NewMessage(chatID, "Ошибка: система боя недоступна.")
		return b.sendMessage(msg)
	}

	activeCombat, err := b.combatRepo.GetActiveBySessionID(ctx, gs.ID)
	if err != nil {
		logger.Error("Failed to get active combat",
			logger.ErrorField(err),
			logger.Int64("chat_id", chatID),
			logger.Uint("session_id", gs.ID),
		)
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при получении информации о бое: %v", err))
		return b.sendMessage(errorMsg)
	}

	if activeCombat == nil || !activeCombat.IsActive() {
		msg := tgbotapi.NewMessage(chatID, "Сейчас нет активного боя. Команда /flee доступна только во время боя.")
		return b.sendMessage(msg)
	}

	// Завершаем бой
	activeCombat.State = combat.CombatStateFinished

	// Сохраняем изменения
	if err := b.combatRepo.Save(ctx, activeCombat); err != nil {
		logger.Error("Failed to save combat",
			logger.ErrorField(err),
			logger.Int64("chat_id", chatID),
			logger.Uint("session_id", gs.ID),
		)
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при завершении боя: %v", err))
		return b.sendMessage(errorMsg)
	}

	logger.Info("Combat ended via /flee command",
		logger.Int64("chat_id", chatID),
		logger.Uint("session_id", gs.ID),
		logger.Uint("combat_id", activeCombat.ID),
	)

	// Формируем сообщение о попытке бегства
	// DM опишет результат бегства при следующем действии игрока
	fleeText := `🏃 Попытка бегства...

Вы попытались выйти из боя. Бой завершен.

Продолжайте играть - DM опишет результат вашего бегства.`

	msg := tgbotapi.NewMessage(chatID, fleeText)
	return b.sendMessage(msg)
}

func (b *Bot) handleFeedback(ctx context.Context, chatID int64, args string, tgUserID int64, from *tgbotapi.User) error {
	// Если есть аргументы, используем старый формат для обратной совместимости
	feedbackText := strings.TrimSpace(args)
	if feedbackText != "" {
		// Отменяем активный диалог, если есть
		b.feedbackStateMu.Lock()
		delete(b.feedbackState, chatID)
		b.feedbackStateMu.Unlock()

		return b.saveFeedbackDirectly(ctx, chatID, tgUserID, from, feedbackText, feedback.FeedbackTypeOther, feedback.FeedbackCategoryOther)
	}

	// Отменяем активный диалог, если есть (пользователь хочет начать заново)
	b.feedbackStateMu.Lock()
	delete(b.feedbackState, chatID)
	b.feedbackStateMu.Unlock()

	// Начинаем интерактивный диалог
	return b.startFeedbackDialog(ctx, chatID, tgUserID, from)
}

// startFeedbackDialog начинает интерактивный диалог feedback
func (b *Bot) startFeedbackDialog(ctx context.Context, chatID int64, tgUserID int64, from *tgbotapi.User) error {
	// Инициализируем состояние диалога
	b.feedbackStateMu.Lock()
	b.feedbackState[chatID] = &FeedbackDialogState{
		UserID: tgUserID,
		From:   from,
	}
	b.feedbackStateMu.Unlock()

	// Показываем кнопки для выбора типа обратной связи
	msg := tgbotapi.NewMessage(chatID, "📝 Выберите тип обратной связи:")

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🐛 Баг", "feedback_type_bug"),
			tgbotapi.NewInlineKeyboardButtonData("💡 Предложение", "feedback_type_suggestion"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❓ Вопрос", "feedback_type_question"),
			tgbotapi.NewInlineKeyboardButtonData("⭐ Похвала", "feedback_type_praise"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📋 Другое", "feedback_type_other"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Отмена", "feedback_cancel"),
		),
	)
	msg.ReplyMarkup = keyboard

	return b.sendMessage(msg)
}

// handleFeedbackTypeSelection обрабатывает выбор типа feedback
func (b *Bot) handleFeedbackTypeSelection(ctx context.Context, chatID int64, query *tgbotapi.CallbackQuery, feedbackType feedback.FeedbackType) error {
	// Отвечаем на callback
	typeNames := map[feedback.FeedbackType]string{
		feedback.FeedbackTypeBug:        "Баг",
		feedback.FeedbackTypeSuggestion: "Предложение",
		feedback.FeedbackTypeQuestion:   "Вопрос",
		feedback.FeedbackTypePraise:     "Похвала",
		feedback.FeedbackTypeOther:      "Другое",
	}
	typeName := typeNames[feedbackType]
	if typeName == "" {
		typeName = "Другое"
	}

	callback := tgbotapi.NewCallback(query.ID, fmt.Sprintf("Выбран тип: %s", typeName))
	if _, err := b.api.Request(callback); err != nil {
		logger.Error("Failed to answer callback",
			logger.ErrorField(err),
		)
	}

	// Сохраняем тип в состоянии
	b.feedbackStateMu.Lock()
	state, exists := b.feedbackState[chatID]
	if !exists {
		state = &FeedbackDialogState{
			UserID: query.From.ID,
			From:   query.From,
		}
		b.feedbackState[chatID] = state
	}
	state.Type = feedbackType
	b.feedbackStateMu.Unlock()

	// Показываем кнопки для выбора категории
	msg := tgbotapi.NewEditMessageText(
		chatID,
		query.Message.MessageID,
		fmt.Sprintf("📝 Тип: %s\n\nВыберите категорию:", typeName),
	)

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⚔️ Боевая система", "feedback_category_combat"),
			tgbotapi.NewInlineKeyboardButtonData("🎭 DM", "feedback_category_dm"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🖥️ Интерфейс", "feedback_category_interface"),
			tgbotapi.NewInlineKeyboardButtonData("🎮 Геймплей", "feedback_category_gameplay"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📋 Другое", "feedback_category_other"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Отмена", "feedback_cancel"),
		),
	)
	msg.ReplyMarkup = &keyboard

	_, err := b.api.Send(msg)
	return err
}

// handleFeedbackCategorySelection обрабатывает выбор категории feedback
func (b *Bot) handleFeedbackCategorySelection(ctx context.Context, chatID int64, query *tgbotapi.CallbackQuery, feedbackCategory feedback.FeedbackCategory) error {
	// Отвечаем на callback
	categoryNames := map[feedback.FeedbackCategory]string{
		feedback.FeedbackCategoryCombat:    "Боевая система",
		feedback.FeedbackCategoryDM:        "DM",
		feedback.FeedbackCategoryInterface: "Интерфейс",
		feedback.FeedbackCategoryGameplay:  "Геймплей",
		feedback.FeedbackCategoryOther:     "Другое",
	}
	categoryName := categoryNames[feedbackCategory]
	if categoryName == "" {
		categoryName = "Другое"
	}

	callback := tgbotapi.NewCallback(query.ID, fmt.Sprintf("Выбрана категория: %s", categoryName))
	if _, err := b.api.Request(callback); err != nil {
		logger.Error("Failed to answer callback",
			logger.ErrorField(err),
		)
	}

	// Сохраняем категорию в состоянии
	b.feedbackStateMu.Lock()
	state, exists := b.feedbackState[chatID]
	if !exists {
		state = &FeedbackDialogState{
			UserID: query.From.ID,
			From:   query.From,
		}
		b.feedbackState[chatID] = state
	}
	state.Category = feedbackCategory
	b.feedbackStateMu.Unlock()

	// Получаем названия типа и категории для отображения
	typeNames := map[feedback.FeedbackType]string{
		feedback.FeedbackTypeBug:        "Баг",
		feedback.FeedbackTypeSuggestion: "Предложение",
		feedback.FeedbackTypeQuestion:   "Вопрос",
		feedback.FeedbackTypePraise:     "Похвала",
		feedback.FeedbackTypeOther:      "Другое",
	}
	typeName := typeNames[state.Type]
	if typeName == "" {
		typeName = "Другое"
	}

	// Просим ввести текст отзыва
	msg := tgbotapi.NewEditMessageText(
		chatID,
		query.Message.MessageID,
		fmt.Sprintf("📝 Тип: %s\n📂 Категория: %s\n\n✍️ Теперь напишите ваш отзыв (просто отправьте текст сообщением):",
			typeName, categoryName),
	)
	msg.ReplyMarkup = nil // Убираем кнопки

	_, err := b.api.Send(msg)
	return err
}

// handleFeedbackCancel обрабатывает отмену диалога feedback
func (b *Bot) handleFeedbackCancel(ctx context.Context, chatID int64, query *tgbotapi.CallbackQuery) error {
	// Отвечаем на callback
	callback := tgbotapi.NewCallback(query.ID, "Диалог отменен")
	if _, err := b.api.Request(callback); err != nil {
		logger.Error("Failed to answer callback",
			logger.ErrorField(err),
		)
	}

	// Удаляем состояние диалога
	b.feedbackStateMu.Lock()
	delete(b.feedbackState, chatID)
	b.feedbackStateMu.Unlock()

	// Обновляем сообщение
	msg := tgbotapi.NewEditMessageText(
		chatID,
		query.Message.MessageID,
		"❌ Диалог отменен. Используйте /feedback для начала нового отзыва.",
	)
	msg.ReplyMarkup = nil

	_, err := b.api.Send(msg)
	return err
}

// saveFeedbackDirectly сохраняет feedback напрямую (для обратной совместимости)
func (b *Bot) saveFeedbackDirectly(ctx context.Context, chatID int64, tgUserID int64, from *tgbotapi.User, feedbackText string, feedbackType feedback.FeedbackType, feedbackCategory feedback.FeedbackCategory) error {
	if b.feedbackRepo == nil {
		msg := tgbotapi.NewMessage(chatID, "Извините, система фидбека временно недоступна.")
		return b.sendMessage(msg)
	}

	// Создаем фидбек
	fb := &feedback.Feedback{
		ChatID:   chatID,
		UserID:   tgUserID,
		Message:  feedbackText,
		Type:     feedbackType,
		Category: feedbackCategory,
	}

	// Добавляем метаданные пользователя, если доступны
	if from != nil {
		fb.UserFirstName = from.FirstName
		fb.UserLastName = from.LastName
		fb.UserUsername = from.UserName
	}

	// Сохраняем фидбек
	if err := b.feedbackRepo.Save(ctx, fb); err != nil {
		logger.Error("Failed to save feedback",
			logger.ErrorField(err),
			logger.Int64("chat_id", chatID),
			logger.Int64("user_id", tgUserID),
		)
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при сохранении отзыва: %v", err))
		return b.sendMessage(errorMsg)
	}

	logger.Info("Feedback saved",
		logger.Int64("chat_id", chatID),
		logger.Int64("user_id", tgUserID),
		logger.Uint("feedback_id", fb.ID),
	)

	// Отправляем подтверждение пользователю
	msg := tgbotapi.NewMessage(chatID, "✅ Спасибо за ваш отзыв! Он поможет нам улучшить игру. 🎲")
	return b.sendMessage(msg)
}

func (b *Bot) handleEndGame(ctx context.Context, chatID int64) error {
	// Получаем текущую сессию
	gs, err := b.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil {
		logger.Error("Failed to get session",
			logger.ErrorField(err),
			logger.Int64("chat_id", chatID),
		)
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при получении сессии: %v", err))
		return b.sendMessage(errorMsg)
	}

	if gs == nil {
		msg := tgbotapi.NewMessage(chatID, "У вас нет активной игры. Используйте /newgame для начала новой игры.")
		return b.sendMessage(msg)
	}

	if !gs.IsActive() {
		msg := tgbotapi.NewMessage(chatID, "Игра уже завершена. Используйте /newgame для начала новой игры.")
		return b.sendMessage(msg)
	}

	// Завершаем игру
	gs.End()

	// Сохраняем изменения
	if err := b.sessionRepo.Save(ctx, gs); err != nil {
		logger.Error("Failed to save session",
			logger.ErrorField(err),
			logger.Int64("chat_id", chatID),
		)
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при сохранении сессии: %v", err))
		return b.sendMessage(errorMsg)
	}

	logger.Info("Game ended",
		logger.Int64("chat_id", chatID),
		logger.Uint("session_id", gs.ID),
	)

	// Формируем информативное сообщение о завершении
	endText := fmt.Sprintf(`✅ Игра завершена!

Мир: %s
%s

Используйте /newgame для начала новой игры.`,
		gs.World.Name,
		gs.World.Description)

	msg := tgbotapi.NewMessage(chatID, endText)
	return b.sendMessage(msg)
}

// sendMessage отправляет сообщение с проверкой ошибок, логированием и retry механизмом
func (b *Bot) sendMessage(msg tgbotapi.MessageConfig) error {
	return b.sendMessageWithRetry(context.Background(), msg, 0)
}

// sendMessageWithRetry отправляет сообщение с retry механизмом и exponential backoff
func (b *Bot) sendMessageWithRetry(ctx context.Context, msg tgbotapi.MessageConfig, attempt int) error {
	const maxRetries = 3
	const initialBackoff = 100 * time.Millisecond
	const maxBackoff = 5 * time.Second

	// Очищаем текст сообщения от невалидных UTF-8 последовательностей перед отправкой
	// Telegram API требует, чтобы все строки были в UTF-8
	msg.Text = sanitizeUTF8(msg.Text)

	// Проверяем circuit breaker
	b.circuitOpenMu.RLock()
	circuitOpen := b.circuitOpen
	b.circuitOpenMu.RUnlock()

	if circuitOpen {
		// Circuit breaker открыт - проверяем, можно ли попробовать снова
		b.errorCountMu.Lock()
		timeSinceLastError := time.Since(b.lastErrorTime)
		if timeSinceLastError > 30*time.Second {
			// Прошло достаточно времени, закрываем circuit breaker
			b.circuitOpenMu.Lock()
			b.circuitOpen = false
			b.circuitOpenMu.Unlock()
			b.errorCount = 0
		}
		b.errorCountMu.Unlock()

		if b.circuitOpen {
			return fmt.Errorf("circuit breaker is open, too many errors")
		}
	}

	_, err := b.api.Send(msg)
	if err == nil {
		// Успешная отправка - сбрасываем счетчик ошибок и закрываем circuit breaker
		b.errorCountMu.Lock()
		b.errorCount = 0
		b.errorCountMu.Unlock()
		b.circuitOpenMu.Lock()
		b.circuitOpen = false
		b.circuitOpenMu.Unlock()
		return nil
	}

	// Ошибка отправки
	b.errorCountMu.Lock()
	b.errorCount++
	b.lastErrorTime = time.Now()
	errorCount := b.errorCount
	b.errorCountMu.Unlock()

	// Проверяем, нужно ли открыть circuit breaker (после 10 ошибок подряд)
	if errorCount >= 10 {
		b.circuitOpenMu.Lock()
		b.circuitOpen = true
		b.circuitOpenMu.Unlock()
		logger.Warn("Telegram API circuit breaker opened due to too many errors",
			logger.Int("consecutive_errors", errorCount),
		)
	}

	// Проверяем, стоит ли повторять попытку
	errStr := err.Error()
	shouldRetry := false
	if strings.Contains(errStr, "unexpected EOF") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "connection") ||
		strings.Contains(errStr, "network") {
		shouldRetry = true
	}

	if !shouldRetry || attempt >= maxRetries {
		// Не retry или исчерпаны попытки
		if attempt >= maxRetries {
			logger.Warn("Failed to send message after max retries",
				logger.ErrorField(sanitizeTelegramError(err)),
				logger.Int64("chat_id", msg.ChatID),
				logger.Int("attempts", attempt+1),
			)
		}
		return err
	}

	// Вычисляем exponential backoff: 100ms, 200ms, 400ms
	// Защита от integer overflow: ограничиваем сдвиг безопасными пределами
	// time.Duration это int64, поэтому 1<<30 безопасно
	const maxSafeShift = 30
	safeAttempt := attempt
	if safeAttempt < 0 {
		safeAttempt = 0
	} else if safeAttempt > maxSafeShift {
		safeAttempt = maxSafeShift
	}
	// #nosec G115 - защита от overflow реализована выше: safeAttempt ограничен до maxSafeShift=30
	// что безопасно для int64/time.Duration (максимальный безопасный сдвиг для int64)
	backoff := initialBackoff * time.Duration(1<<uint(safeAttempt))
	if backoff > maxBackoff {
		backoff = maxBackoff
	}

	// Ждем перед повтором
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(backoff):
	}

	// Повторяем попытку
	return b.sendMessageWithRetry(ctx, msg, attempt+1)
}

// sanitizeUTF8 очищает строку от невалидных UTF-8 последовательностей
// Telegram API требует, чтобы все строки были в UTF-8
func sanitizeUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}

	// Если строка содержит невалидные UTF-8 последовательности, очищаем их
	var result strings.Builder
	result.Grow(len(s))

	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		if r == utf8.RuneError && size == 1 {
			// Невалидная UTF-8 последовательность - заменяем на символ замены UTF-8 (U+FFFD)
			result.WriteRune('\uFFFD')
			s = s[1:]
		} else {
			result.WriteRune(r)
			s = s[size:]
		}
	}

	return result.String()
}

// sanitizeTelegramError редактирует потенциальные секреты (Telegram Bot Token) из ошибок,
// чтобы они не попадали в логи.
func sanitizeTelegramError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	redacted := redactTelegramBotToken(msg)
	if redacted == msg {
		return err
	}
	return fmt.Errorf("%s", redacted)
}

// redactTelegramBotToken заменяет `...api.telegram.org/bot<token>/...` на `...api.telegram.org/bot***/...`.
func redactTelegramBotToken(s string) string {
	const marker = "api.telegram.org/bot"
	idx := strings.Index(s, marker)
	if idx == -1 {
		return s
	}
	start := idx + len(marker) // позиция сразу после "bot"
	// токен заканчивается перед следующим "/"
	end := strings.Index(s[start:], "/")
	if end == -1 {
		// на всякий случай: если "/" нет, редактируем до конца строки
		return s[:start] + "***"
	}
	end = start + end
	return s[:start] + "***" + s[end:]
}

// sendLongMessage разбивает длинные сообщения на части и отправляет их
// Telegram имеет лимит 4096 символов на сообщение
func (b *Bot) sendLongMessage(chatID int64, text string) error {
	// Очищаем текст от невалидных UTF-8 последовательностей перед отправкой
	text = sanitizeUTF8(text)

	if len(text) <= TelegramMaxMessageLength {
		msg := tgbotapi.NewMessage(chatID, text)
		return b.sendMessage(msg)
	}

	// Разбиваем на части по безопасной длине
	parts := splitMessage(text, TelegramSafeMessageLength)

	var lastErr error
	for i, part := range parts {
		// Очищаем каждую часть от невалидных UTF-8 последовательностей
		part = sanitizeUTF8(part)
		msg := tgbotapi.NewMessage(chatID, part)
		if len(parts) > 1 {
			// Добавляем индикатор части для многочастных сообщений
			indicator := fmt.Sprintf("(%d/%d)\n", i+1, len(parts))
			// Проверяем, что индикатор не превышает лимит вместе с частью
			if len(indicator)+len(part) > TelegramMaxMessageLength {
				// Если индикатор слишком длинный, уменьшаем часть
				maxPartLen := TelegramMaxMessageLength - len(indicator)
				if maxPartLen > 0 {
					part = part[:maxPartLen]
				} else {
					// Если индикатор сам по себе превышает лимит, отправляем без него
					indicator = ""
				}
			}
			msg.Text = indicator + part
		}
		if err := b.sendMessage(msg); err != nil {
			lastErr = err
			logger.Error("Failed to send message part",
				logger.ErrorField(err),
				logger.Int64("chat_id", chatID),
				logger.Int("part", i+1),
				logger.Int("total_parts", len(parts)),
			)
		}
	}

	return lastErr
}

// splitMessage разбивает текст на части заданной максимальной длины
// Старается разбивать по предложениям или словам
func splitMessage(text string, maxLen int) []string {
	if len(text) <= maxLen {
		return []string{text}
	}

	var parts []string
	current := ""

	// Разбиваем по строкам
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		// Если добавление строки превысит лимит, сохраняем текущую часть
		if len(current)+len(line)+1 > maxLen && current != "" {
			parts = append(parts, current)
			current = ""
		}

		if len(line) > maxLen {
			// Строка слишком длинная, разбиваем по словам
			words := strings.Fields(line)
			if len(words) == 0 {
				// Строка без пробелов - принудительно разбиваем по maxLen
				for i := 0; i < len(line); i += maxLen {
					end := i + maxLen
					if end > len(line) {
						end = len(line)
					}
					parts = append(parts, line[i:end])
				}
				current = ""
			} else {
				for _, word := range words {
					// Если слово само по себе превышает лимит, разбиваем его принудительно
					if len(word) > maxLen {
						// Сохраняем текущую часть, если есть
						if current != "" {
							parts = append(parts, current)
							current = ""
						}
						// Разбиваем длинное слово на части
						for i := 0; i < len(word); i += maxLen {
							end := i + maxLen
							if end > len(word) {
								end = len(word)
							}
							parts = append(parts, word[i:end])
						}
					} else {
						if len(current)+len(word)+1 > maxLen && current != "" {
							parts = append(parts, current)
							current = ""
						}
						if current != "" {
							current += " "
						}
						current += word
					}
				}
			}
		} else {
			if current != "" {
				current += "\n"
			}
			current += line
		}
	}

	if current != "" {
		parts = append(parts, current)
	}

	return parts
}

// handleLeaderboard обрабатывает команду /leaderboard для отображения рейтинга игроков
func (b *Bot) handleLeaderboard(ctx context.Context, chatID int64, tgUserID int64, args string) error {
	if b.getLeaderboardUC == nil {
		msg := tgbotapi.NewMessage(chatID, "Система рейтингов временно недоступна.")
		return b.sendMessage(msg)
	}

	// Парсим метрику из аргументов (по умолчанию - общий рейтинг)
	metricType := rating.MetricTypeTotalRating
	limit := 10

	if args != "" {
		parts := strings.Fields(strings.ToLower(args))
		if len(parts) > 0 {
			// Парсим тип метрики
			switch parts[0] {
			case "level", "уровень", "л":
				metricType = rating.MetricTypeLevel
			case "experience", "exp", "опыт", "о":
				metricType = rating.MetricTypeExperience
			case "wins", "combat", "победы", "п":
				metricType = rating.MetricTypeCombatWins
			case "quests", "квесты", "к":
				metricType = rating.MetricTypeQuestsCompleted
			case "total", "общий", "т":
				metricType = rating.MetricTypeTotalRating
			}

			// Парсим лимит (если указан второй аргумент)
			if len(parts) > 1 {
				if parsedLimit, err := strconv.Atoi(parts[1]); err == nil && parsedLimit > 0 && parsedLimit <= 100 {
					limit = parsedLimit
				}
			}
		}
	}

	// Получаем лидерборд
	req := ratingapp.GetLeaderboardRequest{
		MetricType: metricType,
		Limit:      limit,
		TgUserID:   tgUserID,
	}

	resp, err := b.getLeaderboardUC.Execute(ctx, req)
	if err != nil {
		logger.Error("Failed to get leaderboard",
			logger.ErrorField(err),
			logger.Int64("chat_id", chatID),
		)
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при получении рейтинга: %v", err))
		return b.sendMessage(errorMsg)
	}

	// Формируем сообщение с лидербордом
	var result strings.Builder
	result.WriteString(fmt.Sprintf("🏆 Лидерборд: %s\n\n", resp.MetricType))

	if len(resp.Entries) == 0 {
		result.WriteString("Пока нет игроков в рейтинге.\n")
		result.WriteString("Играйте, чтобы попасть в топ!")
	} else {
		// Формируем таблицу лидерборда
		for _, entry := range resp.Entries {
			// Эмодзи для медалей
			var medal string
			switch entry.Rank {
			case 1:
				medal = "🥇"
			case 2:
				medal = "🥈"
			case 3:
				medal = "🥉"
			default:
				medal = fmt.Sprintf("%d.", entry.Rank)
			}

			result.WriteString(fmt.Sprintf("%s %s: %d\n", medal, entry.PlayerName, entry.MetricValue))
		}

		// Добавляем информацию о ранге пользователя
		if resp.UserRank > 0 {
			result.WriteString(fmt.Sprintf("\n📊 Ваш ранг: #%d (рейтинг: %d)", resp.UserRank, resp.UserRating))
		}
	}

	// Добавляем подсказку по командам
	result.WriteString("\n\n💡 Использование: /leaderboard [тип] [лимит]\n")
	result.WriteString("Типы: level, experience, wins, quests, total\n")
	result.WriteString("Пример: /leaderboard level 20")

	return b.sendLongMessage(chatID, result.String())
}

// combatRepositoryAdapter адаптирует CombatRepository из bot.go к dm_tools.CombatRepository
type combatRepositoryAdapter struct {
	repo CombatRepository
}

func (a *combatRepositoryAdapter) GetActiveBySessionID(ctx context.Context, sessionID uint) (*combat.Combat, error) {
	return a.repo.GetActiveBySessionID(ctx, sessionID)
}

func (a *combatRepositoryAdapter) Save(ctx context.Context, c *combat.Combat) error {
	return a.repo.Save(ctx, c)
}

// sessionRepoAdapter адаптирует session.Repository из bot.go к dm_tools.SessionRepository
type sessionRepoAdapter struct {
	sessionRepo session.Repository
}

func (a *sessionRepoAdapter) GetByChatID(ctx context.Context, chatID int64) (*session.GameSession, error) {
	return a.sessionRepo.GetByChatID(ctx, chatID)
}

func (a *sessionRepoAdapter) Save(ctx context.Context, gs *session.GameSession) error {
	return a.sessionRepo.Save(ctx, gs)
}

// editMessage редактирует сообщение, обрабатывая ошибку "message is not modified" как не критичную
func (b *Bot) editMessage(edit tgbotapi.EditMessageTextConfig, chatID int64, fallbackText string) error {
	_, err := b.api.Send(edit)
	if err != nil {
		// Проверяем, является ли это ошибкой "message is not modified"
		// Это нормальное поведение при повторных нажатиях на кнопки
		errStr := err.Error()
		if strings.Contains(errStr, "message is not modified") ||
			strings.Contains(errStr, "message_not_modified") {
			// Это не критичная ошибка, логируем на уровне DEBUG
			logger.Debug("Message is not modified (expected behavior)",
				logger.String("error", errStr),
				logger.Int64("chat_id", chatID),
			)
			return nil // Возвращаем nil, так как это ожидаемое поведение
		}

		// Для других ошибок логируем на уровне WARN и отправляем новое сообщение
		logger.Warn("Failed to edit message",
			logger.ErrorField(err),
			logger.Int64("chat_id", chatID),
		)
		// Если редактирование не удалось, отправляем новое сообщение
		return b.sendLongMessage(chatID, fallbackText)
	}
	return nil
}

func (b *Bot) handleWaitUntil(ctx context.Context, chatID int64, args string) error {
	// Получаем сессию
	gs, err := b.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при получении сессии: %v", err))
		return b.sendMessage(errorMsg)
	}
	if gs == nil {
		msg := tgbotapi.NewMessage(chatID, "Игра не начата. Используйте /newgame для начала новой игры.")
		return b.sendMessage(msg)
	}

	// Парсим аргумент времени
	args = strings.TrimSpace(args)
	var newTimeOfDay string

	switch strings.ToLower(args) {
	case "утро", "утра", "morning":
		newTimeOfDay = "morning"
	case "день", "полдень", "noon":
		newTimeOfDay = "noon"
	case "вечер", "evening":
		newTimeOfDay = "evening"
	case "ночь", "night":
		newTimeOfDay = "night"
	case "полночь", "midnight":
		newTimeOfDay = "midnight"
	case "":
		// Показываем текущее время и доступные варианты
		currentTime := gs.World.TimeOfDay
		timeDescriptions := map[string]string{
			"morning":   "🌅 Утро",
			"noon":      "☀️ Полдень",
			"afternoon": "🌇 День",
			"evening":   "🌆 Вечер",
			"night":     "🌙 Ночь",
			"midnight":  "🕛 Полночь",
		}

		text := fmt.Sprintf("🕐 Текущее время: %s\n\nДоступные варианты:\n", timeDescriptions[currentTime])
		for timeKey, desc := range timeDescriptions {
			text += fmt.Sprintf("%s - /wait_until %s\n", desc, timeKey)
		}
		text += "\nТакже можно использовать русские названия: утро, день, вечер, ночь, полночь"

		msg := tgbotapi.NewMessage(chatID, text)
		return b.sendMessage(msg)
	default:
		msg := tgbotapi.NewMessage(chatID, "Неверное время суток. Используйте: morning/noon/afternoon/evening/night/midnight или русские названия: утро/день/вечер/ночь/полночь")
		return b.sendMessage(msg)
	}

	// Изменяем время суток
	oldTime := gs.World.TimeOfDay
	gs.World.SetTimeOfDay(newTimeOfDay)

	// Сохраняем изменения
	if err := b.sessionRepo.Save(ctx, gs); err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при сохранении времени: %v", err))
		return b.sendMessage(errorMsg)
	}

	// Описания времени для красивого вывода
	timeDescriptions := map[string]string{
		"morning":   "🌅 Утро",
		"noon":      "☀️ Полдень",
		"afternoon": "🌇 День",
		"evening":   "🌆 Вечер",
		"night":     "🌙 Ночь",
		"midnight":  "🕛 Полночь",
	}

	text := fmt.Sprintf("🕐 Время суток изменено!\n%s → %s\n\nВ мире наступил %s.",
		timeDescriptions[oldTime], timeDescriptions[newTimeOfDay], strings.ToLower(timeDescriptions[newTimeOfDay]))

	msg := tgbotapi.NewMessage(chatID, text)
	return b.sendMessage(msg)
}

func (b *Bot) handleProgress(ctx context.Context, chatID int64, tgUserID int64) error {
	// Получаем сессию
	gs, err := b.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}

	if gs == nil {
		msg := tgbotapi.NewMessage(chatID, "Игра не начата. Используйте /newgame для начала новой игры.")
		return b.sendMessage(msg)
	}

	// Ищем игрока по TgUserID
	player := gs.FindPlayerByTgUserID(tgUserID)
	if player == nil {
		// Fallback: используем первого игрока для обратной совместимости
		player = gs.GetFirstPlayer()
		if player == nil {
			msg := tgbotapi.NewMessage(chatID, "Персонаж не создан. Используйте /createcharacter для создания персонажа.")
			return b.sendMessage(msg)
		}
	}
	char := player.Character

	// Рассчитываем опыт до следующего уровня
	expToNext := char.GetExperienceToNextLevel()

	// Рассчитываем процент успеха в сессии
	var successRate float64
	if gs.SessionChecksCount > 0 {
		successRate = float64(gs.SessionSuccessCount) / float64(gs.SessionChecksCount) * 100
	}

	// Определяем текущую локацию
	locationName := "Неизвестная локация"
	if gs.CurrentLocationID != nil {
		// Ищем локацию в массиве Locations мира
		for _, loc := range gs.World.Locations {
			if loc.ID == *gs.CurrentLocationID {
				locationName = loc.Name
				break
			}
		}
	}

	// Создаем визуальный прогресс-бар для опыта
	expProgress := ""
	currentLevelMin := character.GetRequiredXPForLevel(char.Level)
	nextLevelMin := character.GetRequiredXPForLevel(char.Level + 1)
	if nextLevelMin > currentLevelMin {
		expRange := nextLevelMin - currentLevelMin
		currentInLevel := char.Experience - currentLevelMin
		if currentInLevel < 0 {
			currentInLevel = 0
		}
		progressPercent := float64(currentInLevel) / float64(expRange)
		if progressPercent > 1 {
			progressPercent = 1
		}

		// Создаем прогресс-бар из 10 символов
		filled := int(progressPercent * 10)
		for i := 0; i < 10; i++ {
			if i < filled {
				expProgress += "█"
			} else {
				expProgress += "░"
			}
		}
	}

	// Создаем визуальный прогресс-бар для здоровья
	hpProgress := ""
	if char.MaxHP > 0 {
		hpPercent := float64(char.HP) / float64(char.MaxHP)
		if hpPercent < 0 {
			hpPercent = 0
		}

		// Создаем прогресс-бар из 10 символов
		filled := int(hpPercent * 10)
		for i := 0; i < 10; i++ {
			if i < filled {
				if hpPercent > 0.6 {
					hpProgress += "🟢" // Зеленый для хорошего здоровья
				} else if hpPercent > 0.3 {
					hpProgress += "🟡" // Желтый для среднего здоровья
				} else {
					hpProgress += "🔴" // Красный для низкого здоровья
				}
			} else {
				hpProgress += "⚫"
			}
		}
	}

	progressText := fmt.Sprintf(`📊 **Прогресс персонажа: %s**

🏆 **Уровень и опыт:**
└ Уровень: %d
└ Опыт: %d / %d (%d до следующего уровня)
└ Прогресс: %s (%.1f%%)

❤️ **Здоровье:**
└ HP: %d / %d
└ Статус: %s
└ Прогресс: %s

🎯 **Статистика сессии:**
└ Проверки: %d всего (%d успехов, %d провалов)
└ Процент успеха: %.1f%%
└ Модификатор сложности: %+d
└ Текущая локация: %s

📅 Сессия начата: %s`,
		char.Name,
		char.Level,
		char.Experience,
		nextLevelMin,
		expToNext,
		expProgress,
		float64(char.Experience-currentLevelMin)/float64(nextLevelMin-currentLevelMin)*100,
		char.HP,
		char.MaxHP,
		char.Status,
		hpProgress,
		gs.SessionChecksCount,
		gs.SessionSuccessCount,
		gs.SessionFailureCount,
		successRate,
		gs.SessionDifficultyMod,
		locationName,
		gs.CreatedAt.Format("02.01.2006 15:04"),
	)

	// Добавляем информацию о сессионных целях
	goalsText := "\n🎯 **Цели сессии:**\n"
	activeGoals := gs.GetActiveGoals()
	completedGoals := gs.GetCompletedGoals()

	if len(activeGoals) == 0 && len(completedGoals) == 0 {
		goalsText += "Цели для этой сессии еще не сгенерированы."
	} else {
		for _, goal := range activeGoals {
			progressPercent := float64(goal.CurrentValue) / float64(goal.TargetValue) * 100
			timeInfo := ""
			if goal.TimeLimit != nil {
				timeLeft := time.Until(*goal.TimeLimit)
				if timeLeft > 0 {
					hours := int(timeLeft.Hours())
					minutes := int(timeLeft.Minutes()) % 60
					timeInfo = fmt.Sprintf(" ⏰ %dh %dm", hours, minutes)
				} else {
					timeInfo = " ⏰ истекло"
				}
			}
			goalsText += fmt.Sprintf("└ %s: %d/%d (%.1f%%)%s\n", goal.Description, goal.CurrentValue, goal.TargetValue, progressPercent, timeInfo)
		}
		for _, goal := range completedGoals {
			goalsText += fmt.Sprintf("✅ %s: %d/%d (завершена)\n", goal.Description, goal.CurrentValue, goal.TargetValue)
		}
	}

	progressText += goalsText

	// Добавляем информацию о cooldown проверок способностей
	cooldownText := "\n⏰ **Cooldown проверок способностей:**\n"
	const cooldownDuration = 30 * time.Second
	hasActiveCooldowns := false

	gs.InitializeCooldowns()
	for abilityType, cooldownTime := range gs.AbilityCooldowns {
		if cooldownTime != nil {
			if onCooldown, remainingTime := gs.IsAbilityOnCooldown(abilityType, cooldownDuration); onCooldown {
				abilityName := getAbilityDisplayName(abilityType)
				cooldownText += fmt.Sprintf("└ %s: %.0f сек\n", abilityName, remainingTime.Seconds())
				hasActiveCooldowns = true
			}
		}
	}

	if !hasActiveCooldowns {
		cooldownText += "Все проверки доступны"
	}

	progressText += cooldownText

	msg := tgbotapi.NewMessage(chatID, progressText)
	msg.ParseMode = tgbotapi.ModeMarkdown
	return b.sendMessage(msg)
}

// handleCooperative обрабатывает команду /cooperative
func (b *Bot) handleCooperative(ctx context.Context, chatID int64, tgUserID int64, args string) error {
	// Парсим аргументы: /cooperative <max_players>
	maxPlayers := 2 // По умолчанию 2 игрока
	if args != "" {
		if parsed, err := strconv.Atoi(args); err == nil && parsed >= 2 && parsed <= 3 {
			maxPlayers = parsed
		}
	}

	playerRepoAdapter := &playerRepoAdapter{repo: b.playerRepo}
	cooperativeUC := sessionapp.NewManageCooperativeUseCase(b.sessionRepo, playerRepoAdapter)
	err := cooperativeUC.EnableCooperativeMode(ctx, sessionapp.EnableCooperativeRequest{
		ChatID:     chatID,
		MaxPlayers: maxPlayers,
	})

	if err != nil {
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка включения cooperative режима: %v", err))
		return b.sendMessage(msg)
	}

	msg := tgbotapi.NewMessage(chatID,
		fmt.Sprintf("🎮 Cooperative режим включен!\nМаксимум игроков: %d\n\nДругие игроки могут присоединиться командой /join", maxPlayers))
	return b.sendMessage(msg)
}

// handleJoin обрабатывает команду /join
func (b *Bot) handleJoin(ctx context.Context, chatID int64, tgUserID int64) error {
	playerRepoAdapter := &playerRepoAdapter{repo: b.playerRepo}
	cooperativeUC := sessionapp.NewManageCooperativeUseCase(b.sessionRepo, playerRepoAdapter)
	err := cooperativeUC.JoinCooperativeSession(ctx, sessionapp.JoinCooperativeSessionRequest{
		ChatID:   chatID,
		TgUserID: tgUserID,
	})

	if err != nil {
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка присоединения к игре: %v", err))
		return b.sendMessage(msg)
	}

	msg := tgbotapi.NewMessage(chatID, "✅ Вы успешно присоединились к cooperative игре!")
	return b.sendMessage(msg)
}

// handleLeave обрабатывает команду /leave
func (b *Bot) handleLeave(ctx context.Context, chatID int64, tgUserID int64) error {
	msg := tgbotapi.NewMessage(chatID, "Функция выхода из cooperative игры пока не реализована. Используйте /endgame для завершения всей сессии.")
	return b.sendMessage(msg)
}

// handleCoopStatus обрабатывает команду /coopstatus
func (b *Bot) handleCoopStatus(ctx context.Context, chatID int64) error {
	cooperativeUC := sessionapp.NewManageCooperativeUseCase(b.sessionRepo, nil)
	status, err := cooperativeUC.GetCooperativeStatus(ctx, chatID)
	if err != nil {
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка получения статуса: %v", err))
		return b.sendMessage(msg)
	}

	var text string
	if status.IsCooperative {
		text = fmt.Sprintf("🎮 **Cooperative режим активен**\nИгроков: %d/%d\n\n", status.CurrentPlayers, status.MaxPlayers)
		for _, player := range status.Players {
			activeMark := ""
			if player.IsActive {
				activeMark = " 👈"
			}
			text += fmt.Sprintf("• Игрок %d%s\n", player.ID, activeMark)
		}
	} else {
		text = "🎮 Cooperative режим отключен\nИспользуйте /cooperative для включения"
	}

	msg := tgbotapi.NewMessage(chatID, text)
	return b.sendMessage(msg)
}

// playerRepoAdapter адаптирует persistence.PlayerRepository к sessionapp.PlayerRepository
type playerRepoAdapter struct {
	repo *persistence.PlayerRepository
}

func (a *playerRepoAdapter) GetByTgUserID(ctx context.Context, tgUserID int64) (*player.Player, error) {
	return a.repo.GetByTgUserID(ctx, tgUserID)
}

func (a *playerRepoAdapter) Save(ctx context.Context, p *player.Player) error {
	return a.repo.Save(ctx, p)
}

// handleToggleAutoImage включает/отключает автоматическую генерацию изображений
func (b *Bot) handleToggleAutoImage(ctx context.Context, chatID int64, tgUserID int64, args string) error {
	// Получаем текущую сессию
	gs, err := b.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}

	if gs == nil {
		msg := tgbotapi.NewMessage(chatID, "Игра не начата. Используйте /newgame для начала новой игры.")
		return b.sendMessage(msg)
	}

	// Получаем анализатор из контекста (если он есть в сессии)
	// Пока что просто возвращаем сообщение о статусе
	currentStatus := "отключена"
	if gs.AutoGenerateImages {
		currentStatus = "включена"
	}

	var message string
	if args == "on" || args == "включить" {
		gs.AutoGenerateImages = true
		// Сохраняем изменение в БД
		if err := b.sessionRepo.Save(ctx, gs); err != nil {
			logger.Warn("Failed to save auto-generate images setting", logger.ErrorField(err))
		}
		message = "✅ Автоматическая генерация изображений включена!\n\nТеперь при посещении новых локаций и встрече с NPC будут автоматически генерироваться изображения."
	} else if args == "off" || args == "отключить" {
		gs.AutoGenerateImages = false
		// Сохраняем изменение в БД
		if err := b.sessionRepo.Save(ctx, gs); err != nil {
			logger.Warn("Failed to save auto-generate images setting", logger.ErrorField(err))
		}
		message = "❌ Автоматическая генерация изображений отключена.\n\nИзображения больше не будут генерироваться автоматически."
	} else {
		message = fmt.Sprintf("📸 Автоматическая генерация изображений: %s\n\nКоманды:\n• /autoimage on (включить) - автоматически генерировать изображения локаций и NPC\n• /autoimage off (отключить) - отключить автоматическую генерацию\n• /image [описание] - сгенерировать изображение вручную", currentStatus)
	}

	msg := tgbotapi.NewMessage(chatID, message)
	return b.sendMessage(msg)
}

// getAbilityDisplayName возвращает читаемое название способности
func getAbilityDisplayName(abilityType string) string {
	switch abilityType {
	case "strength":
		return "Сила (STR)"
	case "dexterity":
		return "Ловкость (DEX)"
	case "constitution":
		return "Телосложение (CON)"
	case "intelligence":
		return "Интеллект (INT)"
	case "wisdom":
		return "Мудрость (WIS)"
	case "charisma":
		return "Харизма (CHA)"
	default:
		return abilityType
	}
}
