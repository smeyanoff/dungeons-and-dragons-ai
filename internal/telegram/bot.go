package telegram

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"time"

	abilitycheck "dungeons-and-dragons-ai/internal/game/application/ability_check"
	achievementapp "dungeons-and-dragons-ai/internal/game/application/achievement"
	"dungeons-and-dragons-ai/internal/game/application/campaign"
	characterapp "dungeons-and-dragons-ai/internal/game/application/character"
	combatapp "dungeons-and-dragons-ai/internal/game/application/combat"
	"dungeons-and-dragons-ai/internal/game/application/dice"
	"dungeons-and-dragons-ai/internal/game/application/history"
	imageapp "dungeons-and-dragons-ai/internal/game/application/image"
	inventoryapp "dungeons-and-dragons-ai/internal/game/application/inventory"
	"dungeons-and-dragons-ai/internal/game/application/player_action"
	questapp "dungeons-and-dragons-ai/internal/game/application/quest"
	ratingapp "dungeons-and-dragons-ai/internal/game/application/rating"
	spellapp "dungeons-and-dragons-ai/internal/game/application/spell"
	subscriptionapp "dungeons-and-dragons-ai/internal/game/application/subscription"
	mapapp "dungeons-and-dragons-ai/internal/game/application/worldmap"
	"dungeons-and-dragons-ai/internal/game/domain/combat"
	"dungeons-and-dragons-ai/internal/game/domain/event"
	"dungeons-and-dragons-ai/internal/game/domain/feedback"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/game/infrastructure/persistence"
	"dungeons-and-dragons-ai/internal/metrics"
	ragdomain "dungeons-and-dragons-ai/internal/rag/domain"
	"dungeons-and-dragons-ai/pkg/logger"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	// TelegramMaxMessageLength is max Telegram message length.
	TelegramMaxMessageLength = 4096
	// TelegramSafeMessageLength leaves room for part indicators.
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
	eventRepo             EventRepository      // saves dice rolls to history
	indexDocUC            IndexDocumentUseCase // indexes rolls in RAG

	// Circuit breaker for Telegram API.
	errorCount    int
	errorCountMu  sync.Mutex
	lastErrorTime time.Time
	circuitOpen   bool
	circuitOpenMu sync.RWMutex

	// Feedback dialog state (chatID -> state).
	feedbackState   map[int64]*FeedbackDialogState
	feedbackStateMu sync.RWMutex

	// Health check state.
	lastHealthCheck time.Time
	healthCheckMu   sync.RWMutex

	// httpTransport — транспорт HTTP-клиента Telegram Bot API. Хранится, чтобы принудительно
	// закрывать простаивающие соединения при сетевых ошибках polling (см. Start).
	httpTransport *http.Transport
}

type FeedbackDialogState struct {
	Type     feedback.FeedbackType
	Category feedback.FeedbackCategory
	UserID   int64
	From     *tgbotapi.User
}

type FeedbackRepository interface {
	Save(ctx context.Context, fb *feedback.Feedback) error
}

type EventRepository interface {
	Save(ctx context.Context, e *event.StoryEvent) error
}

type IndexDocumentUseCase interface {
	Execute(ctx context.Context, doc ragdomain.Document) error
}

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

	// Configure HTTP client for better connection pooling and stability.
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
		httpClient.Timeout = 60 * time.Second
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

	bot.configureHTTPClient()

	if err := bot.setupBotCommands(); err != nil {
		logger.Warn("Failed to setup bot commands menu",
			logger.ErrorField(err),
		)
	}

	return bot, nil
}

func (b *Bot) HandleUpdate(ctx context.Context, update tgbotapi.Update) error {
	return b.handleUpdate(ctx, update)
}

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
		{Command: "summary", Description: "Резюме сессии (где мы и что дальше)"},
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

func (b *Bot) GetAPI() *tgbotapi.BotAPI {
	return b.api
}

func (b *Bot) Start(ctx context.Context) error {
	logger.Info("Bot started",
		logger.String("username", b.api.Self.UserName),
		logger.Int64("bot_id", int64(b.api.Self.ID)),
	)

	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 60

	offset := 0
	backoff := 1 * time.Second
	const maxBackoff = 120 * time.Second
	const logInterval = 60 * time.Second
	lastPollLog := time.Time{}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	consecutiveErrors := 0

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
			metrics.IncrementTelegramPollingError()
			errStr := err.Error()

			isEOFError := strings.Contains(errStr, "unexpected EOF")
			isTimeoutError := strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline exceeded")
			isNetworkError := strings.Contains(errStr, "connection") || strings.Contains(errStr, "network") ||
				strings.Contains(errStr, "reset") || strings.Contains(errStr, "broken pipe")
			isRateLimitError := strings.Contains(errStr, "429") || strings.Contains(errStr, "Too Many Requests")

			// Защита от повторного попадания на уже нерабочее соединение при ретрае.
			// DisableKeepAlives в configureHTTPClient уже должен устранять эту причину EOF,
			// но это дешёвая дополнительная страховка на случай переиспользования пула.
			if (isEOFError || isNetworkError) && b.httpTransport != nil {
				b.httpTransport.CloseIdleConnections()
			}

			var currentBackoff time.Duration
			var backoffMultiplier float64

			switch {
			case isEOFError:
				currentBackoff = time.Duration(float64(backoff) * 0.3)
				if currentBackoff < 2*time.Second {
					currentBackoff = 2 * time.Second
				}
				backoffMultiplier = 1.2

				if shouldLogPollError(&lastPollLog, 15*time.Second) {
					logger.Warn("Telegram polling EOF error (connection interrupted)",
						logger.ErrorField(sanitizeTelegramError(err)),
						logger.String("backoff", currentBackoff.String()),
						logger.Int("consecutive_errors", consecutiveErrors),
					)
				}
			case isTimeoutError:
				currentBackoff = backoff
				backoffMultiplier = 1.5
				if shouldLogPollError(&lastPollLog, 30*time.Second) {
					logger.Warn("Telegram polling timeout error",
						logger.ErrorField(sanitizeTelegramError(err)),
						logger.String("backoff", currentBackoff.String()),
						logger.Int("consecutive_errors", consecutiveErrors),
					)
				}
			case isNetworkError:
				currentBackoff = backoff
				backoffMultiplier = 2.0
				if shouldLogPollError(&lastPollLog, 20*time.Second) {
					logger.Warn("Telegram polling network error",
						logger.ErrorField(sanitizeTelegramError(err)),
						logger.String("backoff", currentBackoff.String()),
						logger.Int("consecutive_errors", consecutiveErrors),
					)
				}
			case isRateLimitError:
				currentBackoff = time.Duration(float64(backoff) * 3)
				backoffMultiplier = 2.5
				if shouldLogPollError(&lastPollLog, 10*time.Second) {
					logger.Warn("Telegram polling rate limit error",
						logger.ErrorField(sanitizeTelegramError(err)),
						logger.String("backoff", currentBackoff.String()),
						logger.Int("consecutive_errors", consecutiveErrors),
					)
				}
			default:
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

			sleepWithJitter(ctx, currentBackoff, rng)

			backoff = time.Duration(float64(backoff) * backoffMultiplier)
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			if backoff >= maxBackoff && consecutiveErrors > 10 {
				logger.Warn("Telegram polling reached maximum backoff, resetting error counter",
					logger.Int("consecutive_errors", consecutiveErrors),
				)
				consecutiveErrors = 0
			}
			continue
		}

		backoff = 1 * time.Second
		consecutiveErrors = 0

		for _, update := range updates {
			if update.UpdateID >= offset {
				offset = update.UpdateID + 1
			}
			if err := b.handleUpdate(ctx, update); err != nil {
				b.errorCountMu.Lock()
				b.errorCount++
				shouldLog := b.errorCount >= 3
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

func (b *Bot) configureHTTPClient() {
	// DisableKeepAlives: true — намеренно.
	// GetUpdates — long-polling запрос (держит соединение до 60 сек в ожидании апдейтов от Telegram),
	// после чего цикл обработки апдейтов синхронно вызывает GigaChat/БД/генерацию изображений,
	// что может занимать десятки секунд. Всё это время соединение простаивает в пуле keep-alive.
	// Если инфраструктура Telegram (или промежуточный прокси) за это время молча закрывает
	// "неактивное" TCP-соединение, следующий вызов GetUpdates пытается переиспользовать уже
	// мёртвое соединение и получает "unexpected EOF" при чтении ответа — см. PROBLEMS.md P1 #5.
	// Каждый запрос на новом соединении полностью устраняет этот класс ошибок; накладные расходы
	// (TCP+TLS handshake) пренебрежимо малы на фоне 60-секундного long-poll.
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		MaxConnsPerHost:     20,
		IdleConnTimeout:     90 * time.Second,
		DisableKeepAlives:   true,
	}

	httpClient := &http.Client{
		Transport: transport,
		Timeout:   120 * time.Second,
	}

	if httpClientField := reflect.ValueOf(b.api).Elem().FieldByName("Client"); httpClientField.IsValid() && httpClientField.CanSet() {
		httpClientField.Set(reflect.ValueOf(httpClient))
		b.httpTransport = transport
		logger.Info("Configured HTTP client for Telegram Bot API (keep-alive disabled to avoid stale-connection EOF on long-polling)")
	} else {
		logger.Warn("Unable to configure custom HTTP client - tgbotapi.BotAPI may not support client injection")
	}
}

func (b *Bot) handleUpdate(ctx context.Context, update tgbotapi.Update) error {
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

	if update.Message.IsCommand() {
		logger.Info("Handling command",
			logger.String("command", update.Message.Command()),
			logger.String("args", update.Message.CommandArguments()),
			logger.Int64("chat_id", chatID),
			logger.Int64("user_id", int64(userID)),
		)
		return b.handleCommand(ctx, chatID, update.Message.Command(), update.Message.CommandArguments(), int64(userID), update.Message.From)
	}

	if strings.HasPrefix(text, "/") {
		parts := strings.Fields(text)
		if len(parts) > 0 {
			command := strings.TrimPrefix(parts[0], "/")
			args := ""
			if len(parts) > 1 {
				args = strings.Join(parts[1:], " ")
			}
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

	b.feedbackStateMu.RLock()
	feedbackState, hasFeedbackDialog := b.feedbackState[chatID]
	b.feedbackStateMu.RUnlock()

	if hasFeedbackDialog && feedbackState != nil && feedbackState.Type != "" && feedbackState.Category != "" {
		feedbackText := strings.TrimSpace(text)
		if feedbackText == "" {
			msg := tgbotapi.NewMessage(chatID, "Пожалуйста, введите текст вашего отзыва.")
			return b.sendMessage(msg)
		}

		err := b.saveFeedbackDirectly(ctx, chatID, int64(userID), update.Message.From, feedbackText, feedbackState.Type, feedbackState.Category)

		b.feedbackStateMu.Lock()
		delete(b.feedbackState, chatID)
		b.feedbackStateMu.Unlock()

		return err
	}

	handled, err := b.tryHandleManualAbilityCheck(ctx, chatID, text)
	if handled {
		return err
	}

	logger.Debug("Handling player action",
		logger.Int64("chat_id", chatID),
		logger.Int64("user_id", int64(userID)),
		logger.Int("message_length", len(text)),
	)
	return b.handlePlayerAction(ctx, chatID, int64(userID), text)
}
