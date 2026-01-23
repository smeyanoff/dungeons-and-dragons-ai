package player_action

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	achievementapp "dungeons-and-dragons-ai/internal/game/application/achievement"
	characterapp "dungeons-and-dragons-ai/internal/game/application/character"
	diceapp "dungeons-and-dragons-ai/internal/game/application/dice"
	"dungeons-and-dragons-ai/internal/game/application/dm_analyzer"
	"dungeons-and-dragons-ai/internal/game/application/dm_tools"
	sessionapp "dungeons-and-dragons-ai/internal/game/application/session"
	imageapp "dungeons-and-dragons-ai/internal/game/application/image"
	locationeventapp "dungeons-and-dragons-ai/internal/game/application/location_event"
	spellapp "dungeons-and-dragons-ai/internal/game/application/spell"
	subscriptionapp "dungeons-and-dragons-ai/internal/game/application/subscription"
	worldeventapp "dungeons-and-dragons-ai/internal/game/application/world_event"
	"dungeons-and-dragons-ai/internal/game/domain/combat"
	"dungeons-and-dragons-ai/internal/game/domain/event"
	"dungeons-and-dragons-ai/internal/game/domain/inventory"
	"dungeons-and-dragons-ai/internal/game/domain/player"
	"dungeons-and-dragons-ai/internal/game/domain/quest"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/game/domain/subscription"
	"dungeons-and-dragons-ai/internal/game/domain/world"
	dmcache "dungeons-and-dragons-ai/internal/game/infrastructure/cache"
	"dungeons-and-dragons-ai/internal/llm/domain"
	ragapp "dungeons-and-dragons-ai/internal/rag/application"
	ragdomain "dungeons-and-dragons-ai/internal/rag/domain"
	"dungeons-and-dragons-ai/pkg/logger"

	"github.com/google/uuid"
)

var rng *rand.Rand

func init() {
	rng = rand.New(rand.NewSource(time.Now().UnixNano()))
}

type HandleActionUseCase struct {
	llm                     domain.LLM
	sessionRepo             session.Repository
	contextBuilder          ContextBuilder
	eventRepo               EventRepository
	indexDocUC              *ragapp.IndexDocument
	combatRepo              CombatRepository
	questRepo               QuestRepository
	inventoryRepo           InventoryRepository
	addExperienceUC         *characterapp.AddExperienceUseCase
	checkWorldEventsUC      *worldeventapp.CheckWorldEventsUseCase
	checkAchievementsUC     *achievementapp.CheckAchievementsUseCase // Для проверки достижений
	notificationService     achievementapp.NotificationService       // Для отправки уведомлений о достижениях
	generateImageUC         *imageapp.ImageGenerationUseCase         // Для генерации изображений
	useSpellUC              *spellapp.UseSpellUseCase                // Для использования заклинаний (опционально)
	responseCache           *dmcache.DMResponseCache
	actionValidator         *ActionValidator
	checkDailyProgressUC    DailyQuestProgressChecker               // Для отслеживания ежедневных заданий
	getSubscriptionUC       *subscriptionapp.GetSubscriptionUseCase // Для проверки Premium статуса для лимитов
	updateRatingUC          RatingUpdater                           // Опциональная зависимость для обновления рейтингов
	analyzePlayerActionUC   *dm_analyzer.AnalyzePlayerActionUseCase // Анализатор действий игрока для определения необходимости проверок
	generateLocationEventUC LocationEventGenerator                  // Генератор событий локаций
}

// RatingUpdater интерфейс для обновления рейтингов
type RatingUpdater interface {
	Execute(ctx context.Context, req RatingUpdateRequest) error
}

// RatingUpdateRequest запрос на обновление рейтинга
type RatingUpdateRequest struct {
	TgUserID int64
	ChatID   int64
}

type EventRepository interface {
	Save(ctx context.Context, e *event.StoryEvent) error
	SaveInTransaction(ctx context.Context, e *event.StoryEvent, fn func(tx interface{}) error) error
	GetBySessionID(ctx context.Context, sessionID uint, limit int) ([]event.StoryEvent, error)
}

type ContextBuilder interface {
	BuildContext(ctx context.Context, session *session.GameSession, playerMessage string) (string, error)
}

type CombatRepository interface {
	Save(ctx context.Context, c *combat.Combat) error
	GetActiveBySessionID(ctx context.Context, sessionID uint) (*combat.Combat, error)
}

// sessionRepoAdapter адаптирует session.Repository к dm_tools.GameSessionRepository
type sessionRepoAdapter struct {
	sessionRepo session.Repository
}

func (a *sessionRepoAdapter) GetByChatID(ctx context.Context, chatID int64) (*session.GameSession, error) {
	return a.sessionRepo.GetByChatID(ctx, chatID)
}

func (a *sessionRepoAdapter) Save(ctx context.Context, gs *session.GameSession) error {
	return a.sessionRepo.Save(ctx, gs)
}

// eventRepoAdapterForDMTools адаптирует EventRepository к dm_tools.EventRepository
type eventRepoAdapterForDMTools struct {
	repo EventRepository
}

func (a *eventRepoAdapterForDMTools) GetBySessionID(ctx context.Context, sessionID uint, limit int) ([]event.StoryEvent, error) {
	return a.repo.GetBySessionID(ctx, sessionID, limit)
}

func (a *eventRepoAdapterForDMTools) Save(ctx context.Context, e *event.StoryEvent) error {
	return a.repo.Save(ctx, e)
}

// LocationEventGenerator интерфейс для генерации событий локаций
type LocationEventGenerator interface {
	Execute(ctx context.Context, req locationeventapp.GenerateLocationEventRequest) (*locationeventapp.GenerateLocationEventResponse, error)
}

// sessionRepoAdapterForDM адаптирует session.Repository к dm_analyzer.SessionRepository
type sessionRepoAdapterForDM struct {
	sessionRepo session.Repository
}

func (a *sessionRepoAdapterForDM) GetByChatID(ctx context.Context, chatID int64) (*dm_analyzer.SessionSnapshot, error) {
	gs, err := a.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil || gs == nil {
		return nil, err
	}

	// Преобразуем локации мира в упрощенный формат для dm_analyzer
	locations := make([]dm_analyzer.LocationSnapshot, 0, len(gs.World.Locations))
	for _, loc := range gs.World.Locations {
		locations = append(locations, dm_analyzer.LocationSnapshot{
			ID:   loc.ID,
			Name: loc.Name,
		})
	}

	// Получаем информацию о персонаже
	var characterInfo *dm_analyzer.CharacterInfo
	if len(gs.Players) > 0 {
		player := gs.Players[0] // Берем первого игрока (основного)
		characterInfo = &dm_analyzer.CharacterInfo{
			ID:          player.ID,
			Name:        player.Name,
			Class:       string(player.Character.Class),
			Level:       player.Character.Level,
			Race:        string(player.Character.Race),
			Description: fmt.Sprintf("%s %s уровня %d", player.Character.Race, player.Character.Class, player.Character.Level),
		}
	}

	// Получаем информацию о текущей локации
	var currentLocation *dm_analyzer.LocationInfo
	if gs.CurrentLocationID != nil {
		// Ищем локацию по ID в мире
		for _, loc := range gs.World.Locations {
			if loc.ID == *gs.CurrentLocationID {
				currentLocation = &dm_analyzer.LocationInfo{
					ID:          loc.ID,
					Name:        loc.Name,
					Description: loc.Description,
				}
				break
			}
		}
	}

	return &dm_analyzer.SessionSnapshot{
		ID:              gs.ID,
		World: dm_analyzer.WorldSnapshot{
			ID:        gs.World.ID,
			Locations: locations,
		},
		CurrentLocation: currentLocation,
		Character:       characterInfo,
	}, nil
}

// playerRepoAdapter адаптирует доступ к игроку через session.Repository
type playerRepoAdapter struct {
	sessionRepo session.Repository
}

func (a *playerRepoAdapter) GetByTgUserIDAndSessionID(ctx context.Context, tgUserID int64, sessionID uint) (*player.Player, error) {
	// Получаем сессию
	gs, err := a.sessionRepo.GetByChatID(ctx, tgUserID)
	if err != nil {
		return nil, err
	}
	if gs == nil || gs.ID != sessionID {
		return nil, nil
	}
	// Находим игрока по TgUserID
	p := gs.FindPlayerByTgUserID(tgUserID)
	if p == nil {
		p = gs.GetFirstPlayer()
	}
	return p, nil
}

func (a *playerRepoAdapter) Save(ctx context.Context, p *player.Player) error {
	// Получаем сессию
	gs, err := a.sessionRepo.GetByChatID(ctx, p.TgUserID)
	if err != nil {
		return err
	}
	if gs == nil {
		return fmt.Errorf("session not found for player")
	}
	// Обновляем персонажа в сессии
	found := false
	for i := range gs.Players {
		if gs.Players[i].ID == p.ID {
			gs.Players[i].Character = p.Character
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("player not found in session")
	}
	// Сохраняем сессию (это обновит игрока через GORM)
	return a.sessionRepo.Save(ctx, gs)
}

type QuestRepository interface {
	GetByWorldID(ctx context.Context, worldID uint) ([]*quest.Quest, error)
	Save(ctx context.Context, q *quest.Quest) error
}

// InventoryRepository интерфейс для работы с инвентарем
type InventoryRepository interface {
	GetByCharacterID(ctx context.Context, characterID uint) (*inventory.Inventory, error)
	Save(ctx context.Context, inv *inventory.Inventory) error
}

// DailyQuestProgressChecker интерфейс для отслеживания ежедневных заданий
type DailyQuestProgressChecker interface {
	Execute(ctx context.Context, req CheckDailyQuestProgressRequest) error
}

// CheckDailyQuestProgressRequest запрос на проверку прогресса ежедневного задания
type CheckDailyQuestProgressRequest struct {
	ChatID    int64
	TgUserID  int64
	QuestType quest.DailyQuestType
	Increment int
}

func NewHandleActionUseCase(
	llm domain.LLM,
	sessionRepo session.Repository,
	contextBuilder ContextBuilder,
	eventRepo EventRepository,
	indexDocUC *ragapp.IndexDocument,
	combatRepo CombatRepository,
	questRepo QuestRepository,
	inventoryRepo InventoryRepository,
	addExperienceUC *characterapp.AddExperienceUseCase,
	checkWorldEventsUC *worldeventapp.CheckWorldEventsUseCase,
	checkAchievementsUC *achievementapp.CheckAchievementsUseCase,
	notificationService achievementapp.NotificationService,
	generateImageUC *imageapp.ImageGenerationUseCase,
	useSpellUC *spellapp.UseSpellUseCase,
	responseCache *dmcache.DMResponseCache,
	actionValidator *ActionValidator,
	checkDailyProgressUC DailyQuestProgressChecker,
	getSubscriptionUC *subscriptionapp.GetSubscriptionUseCase,
	updateRatingUC RatingUpdater,
	analyzePlayerActionUC *dm_analyzer.AnalyzePlayerActionUseCase,
	generateLocationEventUC LocationEventGenerator,
) *HandleActionUseCase {
	return &HandleActionUseCase{
		llm:                     llm,
		sessionRepo:             sessionRepo,
		contextBuilder:          contextBuilder,
		eventRepo:               eventRepo,
		indexDocUC:              indexDocUC,
		combatRepo:              combatRepo,
		questRepo:               questRepo,
		inventoryRepo:           inventoryRepo,
		addExperienceUC:         addExperienceUC,
		checkWorldEventsUC:      checkWorldEventsUC,
		checkAchievementsUC:     checkAchievementsUC,
		notificationService:     notificationService,
		generateImageUC:         generateImageUC,
		useSpellUC:              useSpellUC,
		responseCache:           responseCache,
		actionValidator:         actionValidator,
		checkDailyProgressUC:    checkDailyProgressUC,
		getSubscriptionUC:       getSubscriptionUC,
		updateRatingUC:          updateRatingUC,
		analyzePlayerActionUC:   analyzePlayerActionUC,
		generateLocationEventUC: generateLocationEventUC,
	}
}

func (uc *HandleActionUseCase) Execute(
	ctx context.Context,
	chatID int64,
	tgUserID int64,
	playerMessage string,
) (string, error) {
	// Получаем сессию с таймаутом для БД
	dbCtx, dbCancel := context.WithTimeout(ctx, 10*time.Second)
	defer dbCancel()
	gs, err := uc.sessionRepo.GetByChatID(dbCtx, chatID)
	if err != nil {
		return "", fmt.Errorf("failed to get session: %w", err)
	}

	if gs == nil {
		return "Игра не начата. Используйте /newgame для начала новой игры.", nil
	}

	if !gs.IsActive() {
		return "Игра завершена. Используйте /newgame для начала новой игры.", nil
	}

	// Проверяем истекшие сессионные цели
	sessionGoalsUC := sessionapp.NewManageSessionGoalsUseCase(uc.sessionRepo)
	if err := sessionGoalsUC.CheckSessionExpiredGoals(ctx, chatID); err != nil {
		logger.Warn("Failed to check expired session goals",
			logger.ErrorField(err),
			logger.Int64("chat_id", chatID),
		)
		// Продолжаем выполнение, не прерывая игру
	}

	// Проверяем наличие персонажа перед обработкой действий
	player := gs.FindPlayerByTgUserID(tgUserID)
	if player == nil {
		if gs.IsCooperative {
			return "Ваш персонаж не найден в этой сессии. Используйте /join для присоединения к cooperative игре.", nil
		}
		return "Персонаж не создан. Используйте /createcharacter для создания персонажа.", nil
	}

	// В cooperative режиме проверяем, является ли ход этого игрока
	if gs.IsCooperative && !gs.IsPlayerTurn(tgUserID) {
		activePlayer := gs.GetActivePlayer()
		if activePlayer != nil {
			return fmt.Sprintf("Сейчас не ваш ход. Ожидаем действия от %s.", activePlayer.Name), nil
		}
		return "Сейчас не ваш ход. Подождите, пока другой игрок завершит свой ход.", nil
	}

	// Добавляем контекст с chat_id и tg_user_id для мониторинга LLM запросов
	llmCtx := context.WithValue(ctx, "chat_id", chatID)
	llmCtx = context.WithValue(llmCtx, "tg_user_id", player.TgUserID)
	llmCtx = context.WithValue(llmCtx, "session_id", gs.ID)

	// Информация об активном бое уже включена в контекст RAG
	// DM сам определит, нужно ли использовать combat tool для обработки атаки

	// Валидируем действие игрока перед обработкой
	if uc.actionValidator != nil {
		validationResult, err := uc.actionValidator.Validate(ctx, gs, playerMessage)
		if err != nil {
			logger.Warn("Failed to validate action",
				logger.ErrorField(err),
				logger.Uint("session_id", gs.ID),
			)
			// Пропускаем валидацию при ошибке, но логируем
		} else if !validationResult.Valid {
			// Действие не прошло валидацию, возвращаем сообщение об ошибке
			logger.Info("Action validation failed",
				logger.Uint("session_id", gs.ID),
				logger.String("reason", validationResult.Message),
			)
			return validationResult.Message, nil
		}
	}

	// Проверяем и активируем мировые события перед построением контекста
	if uc.checkWorldEventsUC != nil {
		// Если текущая локация еще не установлена (старые сессии), инициализируем первой локацией мира
		if gs.CurrentLocationID == nil && len(gs.World.Locations) > 0 {
			firstID := gs.World.Locations[0].ID
			if firstID != 0 {
				gs.CurrentLocationID = &firstID
				// Пытаемся сохранить, но не блокируем обработку действий при ошибке
				if err := uc.sessionRepo.Save(ctx, gs); err != nil {
					logger.Warn("Failed to initialize current location in session",
						logger.ErrorField(err),
						logger.Uint("session_id", gs.ID),
					)
				}
			}
		}

		checkCtx, checkCancel := context.WithTimeout(ctx, 5*time.Second)
		defer checkCancel()
		checkReq := worldeventapp.CheckWorldEventsRequest{
			WorldID:           gs.WorldID,
			CurrentLocationID: gs.CurrentLocationID,
		}
		eventsResp, err := uc.checkWorldEventsUC.Execute(checkCtx, checkReq, &gs.World)
		if err != nil {
			// Логируем ошибку, но не прерываем выполнение
			logger.Warn("Failed to check world events",
				logger.ErrorField(err),
				logger.Uint("session_id", gs.ID),
				logger.Uint("world_id", gs.WorldID),
			)
		} else if len(eventsResp.ActivatedEvents) > 0 {
			// События были активированы - это будет учтено в контексте
			logger.Info("World events activated",
				logger.Int("count", len(eventsResp.ActivatedEvents)),
				logger.Uint("session_id", gs.ID),
				logger.Uint("world_id", gs.WorldID),
			)
		}
	}

	// Строим контекст игры с таймаутом для RAG операций
	logger.Debug("Building game context",
		logger.Uint("session_id", gs.ID),
		logger.Int("message_length", len(playerMessage)),
	)
	ragCtx, ragCancel := context.WithTimeout(ctx, 15*time.Second)
	defer ragCancel()
	gameContext, err := uc.contextBuilder.BuildContext(ragCtx, gs, playerMessage)
	if err != nil {
		logger.Error("Failed to build context",
			logger.ErrorField(err),
			logger.Uint("session_id", gs.ID),
		)
		return "", fmt.Errorf("failed to build context: %w", err)
	}
	logger.Debug("Game context built",
		logger.Uint("session_id", gs.ID),
		logger.Int("context_length", len(gameContext)),
	)

	// Проверяем на команды изменения времени суток в естественном языке
	if timeChange := uc.parseTimeChangeCommand(playerMessage); timeChange != "" {
		logger.Info("Time change command detected",
			logger.String("command", timeChange),
			logger.Uint("session_id", gs.ID),
		)
		return uc.handleTimeChange(ctx, gs, timeChange)
	}

	// Анализируем действие игрока для определения необходимости проверок
	var actionAnalysis *dm_analyzer.PlayerActionAnalysis
	if uc.analyzePlayerActionUC != nil {
		analysisCtx, analysisCancel := context.WithTimeout(ctx, 10*time.Second)
		defer analysisCancel()
		analysis, err := uc.analyzePlayerActionUC.Execute(analysisCtx, gs, playerMessage, gameContext)
		if err != nil {
			logger.Warn("Failed to analyze player action",
				logger.ErrorField(err),
				logger.Uint("session_id", gs.ID),
			)
			// Продолжаем без анализа
		} else if analysis != nil {
			actionAnalysis = analysis
			logger.Debug("Player action analyzed",
				logger.Uint("session_id", gs.ID),
				logger.Bool("needs_ability_check", analysis.NeedsAbilityCheck),
				logger.Bool("simple_action", analysis.SimpleAction),
			)
			// Добавляем результат анализа в контекст для DM
			analysisContext := buildActionAnalysisContext(analysis)
			gameContext = gameContext + "\n\n--- Анализ действия игрока ---\n" + analysisContext
		}
	}

	// Сохраняем событие действия игрока ДО вызова LLM
	// Это гарантирует, что действие игрока будет сохранено даже если LLM вернет ошибку
	playerEvent := &event.StoryEvent{
		GameSessionID: gs.ID,
		AuthorType:    event.AuthorTypePlayer,
		Content:       playerMessage,
		CreatedAt:     time.Now(),
	}

	// Сохраняем событие в БД (атомарная операция)
	if err := uc.eventRepo.Save(dbCtx, playerEvent); err != nil {
		// Логируем ошибку, но не прерываем выполнение
		logger.Error("Failed to save player event",
			logger.ErrorField(err),
			logger.Uint("session_id", gs.ID),
		)
	} else {
		logger.Debug("Player event saved",
			logger.Uint("session_id", gs.ID),
			logger.Uint("event_id", playerEvent.ID),
		)
		// Индексируем событие игрока в RAG с таймаутом и повторными попытками
		doc := ragdomain.Document{
			ID:        uuid.New().String(),
			Source:    ragdomain.SourceEvent,
			SessionID: gs.ID,
			Text:      fmt.Sprintf("Игрок: %s", playerMessage),
			Timestamp: time.Now(),
		}
		// Пытаемся проиндексировать с повторными попытками
		if err := uc.indexDocUC.Execute(ragCtx, doc); err != nil {
			logger.Warn("Failed to index player event after retries (event saved in DB, but not indexed in RAG)",
				logger.ErrorField(err),
				logger.Uint("session_id", gs.ID),
				logger.Uint("event_id", playerEvent.ID),
				logger.String("doc_source", string(doc.Source)),
				logger.String("doc_timestamp", doc.Timestamp.Format(time.RFC3339)),
			)
			// Событие сохранено в БД, но не проиндексировано в RAG
			// Это не критично - событие все равно доступно через историю
		} else {
			logger.Debug("Player event indexed in RAG",
				logger.Uint("session_id", gs.ID),
				logger.String("doc_id", doc.ID),
			)
		}
	}

	// Если анализатор решил, что нужна проверка навыка — создаем pending check СИСТЕМОЙ
	// и возвращаем игроку текстовую подсказку. LLM-DM не участвует и не должен просить /roll.
	if actionAnalysis != nil && actionAnalysis.NeedsAbilityCheck && actionAnalysis.AbilityCheck != nil && !actionAnalysis.SimpleAction {
		checkMsg, handled, err := uc.createAbilityCheckFromAnalyzer(ctx, gs, actionAnalysis.AbilityCheck)
		if err != nil {
			logger.Warn("Failed to create pending ability check from analyzer",
				logger.ErrorField(err),
				logger.Uint("session_id", gs.ID),
			)
		} else if handled && strings.TrimSpace(checkMsg) != "" {
			// Сохраняем "DM/system" сообщение о необходимости проверки в историю и RAG
			dmEvent := &event.StoryEvent{
				GameSessionID: gs.ID,
				AuthorType:    event.AuthorTypeDM,
				Content:       checkMsg,
				CreatedAt:     time.Now(),
			}
			if err := uc.eventRepo.Save(ctx, dmEvent); err != nil {
				logger.Warn("Failed to save analyzer ability check prompt event",
					logger.ErrorField(err),
					logger.Uint("session_id", gs.ID),
				)
			} else if uc.indexDocUC != nil {
				doc := ragdomain.Document{
					ID:        uuid.New().String(),
					Source:    ragdomain.SourceEvent,
					SessionID: gs.ID,
					Text:      checkMsg,
					Timestamp: time.Now(),
				}
				_ = uc.indexDocUC.Execute(ctx, doc)
			}
			// Добавляем сообщение о проверке к игровому контексту, но НЕ возвращаем его сразу
			// Игра должна продолжаться с учетом необходимости проверки
			gameContext = gameContext + "\n\n" + checkMsg
		}
	}

	// Проверяем кэш ответов DM перед генерацией
	var response string
	var fromCache bool
	if uc.responseCache != nil {
		cachedResponse, found := uc.responseCache.Get(ctx, gs.ID, gameContext, playerMessage)
		if found {
			// Очищаем кэшированный ответ от технических деталей
			response = cleanTechnicalDetails(cachedResponse)
			response = sanitizePlayerFacingResponse(response)
			fromCache = true
			logger.Info("DM response retrieved from cache",
				logger.Uint("session_id", gs.ID),
			)
		}
	}

	// Если ответ не найден в кэше, генерируем новый
	if !fromCache {
		// Получаем игрока для инициализации tools
		player := gs.GetFirstPlayer()
		if player == nil {
			return "Персонаж не создан. Используйте /createcharacter для создания персонажа.", nil
		}

		// Создаем реестр инструментов и регистрируем их
		toolRegistry := uc.createToolRegistry(gs, player)

		// Формируем промпт для DM.
		// Важно: это лимиты ПО СИМВОЛАМ (не по токенам). Слишком низкие значения приводят к потере
		// важных частей контекста (локации/связи/квесты) и эффекту "обрезания" мира DM.
		maxContextLength := getEnvInt("DM_MAX_CONTEXT_CHARS", 16000)
		maxPromptLength := getEnvInt("DM_MAX_PROMPT_CHARS", 24000)
		contextForPrompt, removedBlocks, hardTruncated := truncateContextByPriority(gameContext, maxContextLength)
		if contextForPrompt != gameContext {
			logger.Warn("Game context truncated for prompt",
				logger.Uint("session_id", gs.ID),
				logger.Int("original_length", len(gameContext)),
				logger.Int("truncated_length", len(contextForPrompt)),
				logger.Int("max_context_length", maxContextLength),
				logger.Any("removed_blocks", removedBlocks),
				logger.Bool("hard_truncated", hardTruncated),
			)
		}
		prompt := BuildDMPrompt(contextForPrompt, playerMessage, player.GetPreferences())
		if len(prompt) > maxPromptLength {
			prompt = truncateMiddle(prompt, maxPromptLength)
			logger.Warn("Prompt length exceeded, truncating",
				logger.Uint("session_id", gs.ID),
				logger.Int("prompt_length", len(prompt)),
				logger.Int("max_length", maxPromptLength),
			)
		}

		// Получаем ответ от DM с поддержкой tools через multi-step loop
		logger.Info("Generating DM response with tools",
			logger.Uint("session_id", gs.ID),
			logger.Int("prompt_length", len(prompt)),
		)
		llmCtx, llmCancel := context.WithTimeout(ctx, 60*time.Second)
		defer llmCancel()
		startTime := time.Now()

		// Multi-step loop: генерация → выполнение инструментов → финальная генерация
		var imageResults []map[string]interface{}
		response, imageResults, err = uc.generateWithToolsLoop(llmCtx, prompt, toolRegistry, gs)
		duration := time.Since(startTime)
		if err != nil {
			logger.Error("Failed to generate DM response with tools",
				logger.ErrorField(err),
				logger.Uint("session_id", gs.ID),
				logger.Duration("duration", duration),
			)
			return "", fmt.Errorf("failed to generate DM response: %w", err)
		}
		logger.Info("DM response generated with tools",
			logger.Uint("session_id", gs.ID),
			logger.Int("response_length", len(response)),
			logger.Duration("duration", duration),
		)

		// Если были сгенерированы изображения, добавляем маркеры для отправки через Telegram
		if len(imageResults) > 0 {
			logger.Info("Images generated during DM response",
				logger.Uint("session_id", gs.ID),
				logger.Int("image_count", len(imageResults)),
			)
			// Добавляем маркер в ответ, чтобы bot.go мог обнаружить и отправить изображения
			imageMarkers := []string{}
			for _, img := range imageResults {
				if path, ok := img["image_path"].(string); ok {
					imageMarkers = append(imageMarkers, fmt.Sprintf("\n[IMAGE:%s]", path))
				}
			}
			if len(imageMarkers) > 0 {
				response = response + strings.Join(imageMarkers, "")
			}
		}

		// Сохраняем ответ в кэш
		if uc.responseCache != nil {
			uc.responseCache.Set(ctx, gs.ID, gameContext, playerMessage, response)
		}
	}

	// Сохраняем ответ DM
	dmEvent := &event.StoryEvent{
		GameSessionID: gs.ID,
		AuthorType:    event.AuthorTypeDM,
		Content:       response,
		CreatedAt:     time.Now(),
	}
	// Сохраняем событие в БД (атомарная операция)
	if err := uc.eventRepo.Save(dbCtx, dmEvent); err != nil {
		// Логируем ошибку, но не прерываем выполнение
		logger.Error("Failed to save DM event",
			logger.ErrorField(err),
			logger.Uint("session_id", gs.ID),
		)
	} else {
		logger.Debug("DM event saved",
			logger.Uint("session_id", gs.ID),
			logger.Uint("event_id", dmEvent.ID),
		)
		// Индексируем ответ DM в RAG с новым контекстом, таймаутом и повторными попытками
		// Создаем новый контекст, так как ragCtx мог быть просрочен после долгого вызова LLM
		indexCtx, indexCancel := context.WithTimeout(ctx, 15*time.Second)
		defer indexCancel()
		doc := ragdomain.Document{
			ID:        uuid.New().String(),
			Source:    ragdomain.SourceEvent,
			SessionID: gs.ID,
			Text:      fmt.Sprintf("DM: %s", response),
			Timestamp: time.Now(),
		}
		// Пытаемся проиндексировать с повторными попытками
		if err := uc.indexDocUC.Execute(indexCtx, doc); err != nil {
			logger.Warn("Failed to index DM event after retries (event saved in DB, but not indexed in RAG)",
				logger.ErrorField(err),
				logger.Uint("session_id", gs.ID),
				logger.Uint("event_id", dmEvent.ID),
				logger.String("doc_source", string(doc.Source)),
				logger.String("doc_timestamp", doc.Timestamp.Format(time.RFC3339)),
			)
			// Событие сохранено в БД, но не проиндексировано в RAG
			// Это не критично - событие все равно доступно через историю
		} else {
			logger.Debug("DM event indexed in RAG",
				logger.Uint("session_id", gs.ID),
				logger.String("doc_id", doc.ID),
			)
		}
	}

	// Анализируем ответ DM для автоматического определения боевых ситуаций, квестов и опыта
	// Получаем модифицированный response с маркерами изображений и сообщение о начале боя
	modifiedResponse, combatStartMessage := uc.analyzeDMResponse(ctx, gs, response)

	// Используем модифицированный response (с маркерами изображений)
	response = modifiedResponse

	// Добавляем сообщение о порядке ходов к ответу DM, если бой начался
	if combatStartMessage != "" {
		response = fmt.Sprintf("%s\n\n%s", response, combatStartMessage)
	}

	// Проверяем активный бой и автоматически выполняем ходы врагов
	// Это решает задачу #70: Автоматическое выполнение ходов врагов в бою
	if uc.combatRepo != nil {
		activeCombat, err := uc.combatRepo.GetActiveBySessionID(ctx, gs.ID)
		if err != nil {
			logger.Warn("Failed to get active combat for enemy turn check",
				logger.ErrorField(err),
				logger.Uint("session_id", gs.ID),
			)
		} else if activeCombat != nil && activeCombat.IsActive() {
			// Переходим к следующему ходу после действия игрока
			activeCombat.NextTurn()

			// Проверяем, чей следующий ход
			currentParticipant := activeCombat.GetCurrentParticipant()
			if currentParticipant != nil && !currentParticipant.IsPlayer {
				// Следующий ход принадлежит врагу - автоматически генерируем его действие
				logger.Info("Enemy turn detected, generating automatic enemy action",
					logger.Uint("session_id", gs.ID),
					logger.String("enemy_name", currentParticipant.GetName()),
				)

				// Сохраняем состояние боя после перехода хода
				if err := uc.combatRepo.Save(ctx, activeCombat); err != nil {
					logger.Error("Failed to save combat after next turn",
						logger.ErrorField(err),
						logger.Uint("session_id", gs.ID),
					)
				}

				// Генерируем автоматический ход врага
				enemyActionResponse, err := uc.generateEnemyTurn(ctx, gs, activeCombat, currentParticipant)
				if err != nil {
					logger.Error("Failed to generate enemy turn",
						logger.ErrorField(err),
						logger.Uint("session_id", gs.ID),
					)
					// Не прерываем выполнение - возвращаем ответ игрока даже если вражеский ход не сгенерирован
				} else if enemyActionResponse != "" {
					// Добавляем автоматический ход врага к ответу
					response = fmt.Sprintf("%s\n\n%s", response, enemyActionResponse)

					// Сохраняем ответ врага как событие DM
					enemyEvent := &event.StoryEvent{
						GameSessionID: gs.ID,
						AuthorType:    event.AuthorTypeDM,
						Content:       enemyActionResponse,
						CreatedAt:     time.Now(),
					}
					if err := uc.eventRepo.Save(ctx, enemyEvent); err != nil {
						logger.Warn("Failed to save enemy turn event",
							logger.ErrorField(err),
							logger.Uint("session_id", gs.ID),
						)
					}
				}
			}
		}
	}

	// В cooperative режиме переключаем ход к следующему игроку после успешного действия
	if gs.IsCooperative {
		gs.NextPlayerTurn()
		if err := uc.sessionRepo.Save(ctx, gs); err != nil {
			logger.Warn("Failed to switch player turn in cooperative mode",
				logger.ErrorField(err),
				logger.Uint("session_id", gs.ID),
			)
		}
	}

	return response, nil
}

// analyzeDMResponse анализирует ответ DM и выполняет автоматические действия
// Возвращает модифицированный response с добавленными маркерами изображений и сообщение о начале боя
func (uc *HandleActionUseCase) analyzeDMResponse(
	ctx context.Context,
	gs *session.GameSession,
	dmResponse string,
) (modifiedResponse string, combatStartMessage string) {
	// Получаем игрока (используем первого игрока для обратной совместимости)
	player := gs.GetFirstPlayer()
	if player == nil {
		// Нет игрока, нечего анализировать
		return dmResponse, ""
	}

	// Создаем анализатор
	analyzer := dm_analyzer.NewAnalyzeDMResponseUseCase(
		uc.llm,
		uc.combatRepo,
		uc.questRepo,
		uc.inventoryRepo,
		gs.ID,
		gs.ChatID, // Передаем chatID для отправки уведомлений
		gs.WorldID,
		player.CharacterID,
		player.ID,     // Передаем playerID для проверки достижений
		gs.AutoGenerateImages, // Используем настройку из сессии
	)

	// Настраиваем проверку достижений и уведомления в AnalyzeDMResponseUseCase
	if uc.checkAchievementsUC != nil {
		// Создаем адаптер для передачи CheckAchievementsUseCase в dm_analyzer
		achievementChecker := &achievementCheckerAdapter{checkAchievementsUC: uc.checkAchievementsUC}
		analyzer.SetCheckAchievementsUseCase(achievementChecker)

		// Настраиваем notification service для отправки уведомлений о достижениях
		// notification service будет настроен в main.go после создания bot
		// Для этого нужно создать адаптер или передать через HandleActionUseCase
		// Пока оставляем как есть - уведомления будут отправляться только если notification service настроен
	}

	// Настраиваем автоматическую генерацию изображений
	if uc.generateImageUC != nil {
		// Создаем адаптер для передачи ImageGenerationUseCase в dm_analyzer
		imageServiceAdapter := &imageGenerationServiceAdapter{uc: uc.generateImageUC}
		analyzer.SetImageGenerationService(imageServiceAdapter, player.TgUserID)
	}

	// Настраиваем отслеживание ежедневных заданий
	if uc.checkDailyProgressUC != nil {
		// Создаем адаптер для передачи CheckDailyQuestProgressUseCase в dm_analyzer
		dailyQuestAdapter := &dailyQuestProgressAdapter{checkDailyProgressUC: uc.checkDailyProgressUC}
		analyzer.SetCheckDailyProgress(dailyQuestAdapter, player.TgUserID)
	}

	// Настраиваем генератор событий локаций
	if uc.generateLocationEventUC != nil {
		analyzer.SetLocationEventGenerator(uc.generateLocationEventUC)
		// Создаем адаптер для передачи sessionRepo в dm_analyzer
		sessionRepoAdapter := &sessionRepoAdapterForDM{sessionRepo: uc.sessionRepo}
		analyzer.SetSessionRepository(sessionRepoAdapter)
	}

	// Настраиваем запись событий локации в историю и RAG
	if uc.eventRepo != nil {
		analyzer.SetStoryEventRepository(uc.eventRepo)
	}
	if uc.indexDocUC != nil {
		analyzer.SetRAGIndexer(uc.indexDocUC)
	}

	// Настраиваем доступ к полной сессии для естественных проверок
	analyzer.SetFullSessionRepository(uc.sessionRepo)

	// Анализируем ответ DM
	analysis, err := analyzer.Execute(ctx, dmResponse)
	if err != nil {
		logger.Warn("Failed to analyze DM response",
			logger.ErrorField(err),
			logger.Uint("session_id", gs.ID),
		)
		return dmResponse, ""
	}

	// Обновляем рейтинг при завершении квеста
	if uc.updateRatingUC != nil && analysis.QuestCompleted {
		ratingReq := RatingUpdateRequest{
			TgUserID: player.TgUserID,
			ChatID:   gs.ChatID,
		}
		if err := uc.updateRatingUC.Execute(ctx, ratingReq); err != nil {
			logger.Warn("Failed to update rating after quest completion",
				logger.ErrorField(err),
				logger.Uint("player_id", player.ID),
				logger.Int64("tg_user_id", player.TgUserID),
			)
		}
	}

	// Сохраняем сообщение о начале боя для возврата
	combatStartMessage = analysis.CombatStartMessage

	// Добавляем маркеры автоматически сгенерированных изображений в ответ DM
	// Изображения будут отправлены автоматически через extractImageMarkers в bot.go
	modifiedResponse = dmResponse
	if len(analysis.GeneratedImages) > 0 {
		logger.Info("Adding image markers to DM response",
			logger.Uint("session_id", gs.ID),
			logger.Int("image_count", len(analysis.GeneratedImages)),
		)
		// Добавляем маркеры изображений в конец ответа DM
		for _, img := range analysis.GeneratedImages {
			modifiedResponse += fmt.Sprintf("\n[IMAGE:%s]", img.ImagePath)
		}
	}

	// Начисляем опыт, если он был получен
	if analysis.ExperienceGained > 0 {
		logger.Info("Experience gained",
			logger.Uint("session_id", gs.ID),
			logger.Int("amount", analysis.ExperienceGained),
			logger.String("reason", analysis.ExperienceReason),
		)
		req := characterapp.AddExperienceRequest{
			ChatID: gs.ChatID,
			Amount: analysis.ExperienceGained,
			Reason: analysis.ExperienceReason,
		}
		_, leveledUp, err := uc.addExperienceUC.Execute(ctx, req)
		if err != nil {
			logger.Error("Failed to add experience",
				logger.ErrorField(err),
				logger.Uint("session_id", gs.ID),
			)
		} else if leveledUp {
			logger.Info("Player leveled up",
				logger.Uint("session_id", gs.ID),
				logger.Int("new_level", player.Character.Level+1),
			)
		}
	}

	// Обновляем прогресс сессионных целей
	uc.updateSessionGoalsProgress(ctx, gs, analysis)

	// Возвращаем модифицированный response с маркерами изображений и сообщение о начале боя
	return modifiedResponse, combatStartMessage
}

// generateEnemyTurn генерирует автоматический ход врага в бою
// Это решает задачу #70: Автоматическое выполнение ходов врагов в бою
func (uc *HandleActionUseCase) generateEnemyTurn(
	ctx context.Context,
	gs *session.GameSession,
	activeCombat *combat.Combat,
	enemyParticipant *combat.CombatParticipant,
) (string, error) {
	// Находим игрока как цель атаки
	var playerParticipant *combat.CombatParticipant
	for i := range activeCombat.Participants {
		if activeCombat.Participants[i].IsPlayer && activeCombat.Participants[i].IsAlive() {
			playerParticipant = &activeCombat.Participants[i]
			break
		}
	}

	if playerParticipant == nil {
		// Игрок мертв или не найден - враг не может атаковать
		return "", fmt.Errorf("player not found or dead")
	}

	// Получаем игрока для доступа к персонажу
	player := gs.GetFirstPlayer()
	if player == nil {
		return "", fmt.Errorf("player not found in session")
	}

	// Строим контекст боя для DM
	combatContext := uc.buildCombatContext(gs, activeCombat)

	// Формируем промпт для автоматического хода врага
	enemyName := enemyParticipant.GetName()
	enemyHP := enemyParticipant.GetHP()
	enemyMaxHP := enemyParticipant.GetMaxHP()
	playerName := playerParticipant.GetName()
	playerHP := playerParticipant.GetHP()
	playerMaxHP := playerParticipant.GetMaxHP()

	enemyTurnPrompt := fmt.Sprintf(`Ты — Dungeon Master в игре Dungeons & Dragons.

Контекст текущего боя:
%s

Сейчас ход врага "%s" (%d/%d HP). Игрок "%s" (%d/%d HP) является целью.

ОПИШИ действие врага и ОБЯЗАТЕЛЬНО используй инструмент 'perform_enemy_attack' для автоматической атаки врага по игроку.
Инструмент 'perform_enemy_attack' автоматически выполнит бросок кубиков, проверит попадание и применит урон.

ВАЖНО: 
- Опиши действие врага естественным образом (1-2 предложения)
- ОБЯЗАТЕЛЬНО используй инструмент 'perform_enemy_attack' с параметром target_name='%s'
- Ты можешь вызвать несколько инструментов одновременно, если нужно (например, проверить статус боя И атаковать)
- После получения результата инструмента опиши результат атаки естественным образом
- НЕ включай в ответ технические детали типа "Шаг 1", "Шаг 2", "tool_call" - просто опиши что происходит

Пример хорошего ответа: "%s рычит и бросается на %s! [здесь будет вызван инструмент perform_enemy_attack] Крылатый демон наносит удар когтями, попадая точно в цель!"

ВАЖНО: ОБЯЗАТЕЛЬНО используй инструмент 'perform_enemy_attack' - без него атака не будет выполнена!`,
		combatContext,
		enemyName, enemyHP, enemyMaxHP,
		playerName, playerHP, playerMaxHP,
		playerName,
		enemyName, playerName)

	// Создаем реестр инструментов и регистрируем perform_enemy_attack
	toolRegistry := dm_tools.NewToolRegistry()
	if uc.combatRepo != nil && uc.sessionRepo != nil {
		// Создаем адаптеры для передачи sessionRepo в tool
		// Для PerformEnemyAttackTool нужны sessionRepo и playerRepo
		sessionAdapter := &sessionRepoAdapter{sessionRepo: uc.sessionRepo}
		playerAdapter := &playerRepoAdapter{sessionRepo: uc.sessionRepo}
		// Преобразуем sessionRepoAdapter в dm_tools.GameSessionRepository через интерфейс
		// sessionRepoAdapter уже реализует нужный интерфейс, поэтому можем использовать его напрямую
		var gameSessionRepo dm_tools.GameSessionRepository = sessionAdapter
		toolRegistry.Register(dm_tools.NewPerformEnemyAttackTool(uc.combatRepo, gameSessionRepo, playerAdapter, gs.ID, gs.ChatID))
		toolRegistry.Register(dm_tools.NewCheckCombatStatusTool(uc.combatRepo, gs.ID))
		toolRegistry.Register(dm_tools.NewGetBattlefieldStatusTool(uc.combatRepo, gs.ID))
	}

	// Генерируем ответ DM с поддержкой tools
	logger.Info("Generating enemy turn with tools",
		logger.Uint("session_id", gs.ID),
		logger.String("enemy_name", enemyName),
	)

	llmCtx, llmCancel := context.WithTimeout(ctx, 60*time.Second)
	defer llmCancel()

	enemyResponse, _, err := uc.generateWithToolsLoop(llmCtx, enemyTurnPrompt, toolRegistry, gs)
	if err != nil {
		return "", fmt.Errorf("failed to generate enemy turn: %w", err)
	}

	logger.Info("Enemy turn generated",
		logger.Uint("session_id", gs.ID),
		logger.String("enemy_name", enemyName),
		logger.Int("response_length", len(enemyResponse)),
	)

	return enemyResponse, nil
}

// cleanTechnicalDetails удаляет технические детали из ответа DM
// Удаляет шаги, инструкции и другие технические элементы, которые не должны показываться пользователю
func cleanTechnicalDetails(text string) string {
	// Удаляем шаги типа "### Шаг 1:", "Шаг 1:", "### Шаг 2:" и т.д.
	re := regexp.MustCompile(`(?i)(###\s*)?(шаг\s*\d+[:.]|step\s*\d+[:.])\s*`)
	cleaned := re.ReplaceAllString(text, "")

	// Удаляем строки с техническими инструкциями
	lines := strings.Split(cleaned, "\n")
	var filteredLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		// Пропускаем строки, которые выглядят как технические инструкции
		if strings.HasPrefix(trimmed, "### ") &&
			(strings.Contains(lower, "шаг") ||
				strings.Contains(lower, "step") ||
				strings.Contains(lower, "описание") ||
				strings.Contains(lower, "выполнение") ||
				strings.Contains(lower, "результат")) {
			continue
		}
		// Пропускаем строки с tool-артефактами
		if strings.Contains(lower, "<tool") ||
			strings.Contains(lower, "tool_call") ||
			strings.Contains(lower, "tool_result") ||
			strings.Contains(lower, "tool_name") ||
			strings.Contains(lower, "arguments") {
			continue
		}
		// Пропускаем технические сообщения о выполнении инструментов
		if strings.Contains(lower, "получаются характеристики") ||
			strings.Contains(lower, "проверяется инвентарь") ||
			strings.Contains(lower, "проверяется статус поля боя") ||
			strings.Contains(lower, "проверяются способности") ||
			strings.Contains(lower, "выполняется проверка") ||
			strings.Contains(lower, "выполняется спасбросок") ||
			strings.Contains(lower, "оценивается результат") ||
			strings.Contains(lower, "выполняется боевая атака") ||
			strings.Contains(lower, "враг выполняет атаку") ||
			strings.Contains(lower, "применяется урон") ||
			strings.Contains(lower, "выполняется") ||
			strings.Contains(lower, "проверяется") ||
			strings.Contains(lower, "получаются") ||
			strings.Contains(lower, "оценивается") {
			continue
		}
		// Пропускаем пустые строки после удаления технических деталей
		if trimmed == "" && len(filteredLines) > 0 && filteredLines[len(filteredLines)-1] == "" {
			continue
		}
		filteredLines = append(filteredLines, line)
	}

	cleaned = strings.Join(filteredLines, "\n")
	// Удаляем множественные пустые строки
	cleaned = regexp.MustCompile(`\n{3,}`).ReplaceAllString(cleaned, "\n\n")
	return strings.TrimSpace(cleaned)
}

func sanitizePlayerFacingResponse(text string) string {
	if strings.TrimSpace(text) == "" {
		return text
	}

	// Удаляем fenced-блоки (```...```)
	fencePattern := regexp.MustCompile("(?s)```.*?```")
	text = fencePattern.ReplaceAllString(text, "")

	lines := strings.Split(text, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)

		// Убираем явные технические секции инструментов
		if strings.Contains(lower, "результаты вызова инструментов") ||
			strings.Contains(lower, "tool_call") ||
			strings.Contains(lower, "tool_result") ||
			strings.Contains(lower, "tool_name") ||
			strings.Contains(lower, "arguments") {
			continue
		}

		// Убираем строки с JSON-объектами/массивами (обычно это tool-артефакты)
		if (strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) ||
			(strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")) {
			continue
		}

		filtered = append(filtered, line)
	}

	result := strings.TrimSpace(strings.Join(filtered, "\n"))
	if result == "" {
		return ""
	}

	// Удаляем множественные пустые строки после фильтрации
	result = regexp.MustCompile(`\n{3,}`).ReplaceAllString(result, "\n\n")
	return strings.TrimSpace(result)
}

func truncateMiddle(text string, maxLen int) string {
	if maxLen <= 0 || len(text) <= maxLen {
		return text
	}
	trimmed := strings.TrimSpace(text)
	if len(trimmed) <= maxLen {
		return trimmed
	}
	marker := "\n...[context truncated]...\n"
	if maxLen <= len(marker)+10 {
		// ВАЖНО: не режем UTF-8 в середине руны — иначе получаем невалидные байты (и падаем при записи в Postgres).
		n := maxLen
		if n > len(trimmed) {
			n = len(trimmed)
		}
		for n > 0 && !utf8.ValidString(trimmed[:n]) {
			n--
		}
		return trimmed[:n]
	}
	headLen := (maxLen - len(marker)) * 2 / 3
	tailLen := maxLen - len(marker) - headLen
	if headLen < 0 {
		headLen = 0
	}
	if tailLen < 0 {
		tailLen = 0
	}
	// Подбираем границы так, чтобы обе части были валидным UTF-8.
	headN := headLen
	if headN > len(trimmed) {
		headN = len(trimmed)
	}
	for headN > 0 && !utf8.ValidString(trimmed[:headN]) {
		headN--
	}
	head := trimmed[:headN]

	tailStart := len(trimmed) - tailLen
	if tailStart < 0 {
		tailStart = 0
	}
	for tailStart < len(trimmed) && !utf8.ValidString(trimmed[tailStart:]) {
		tailStart++
	}
	tail := trimmed[tailStart:]

	return strings.TrimSpace(head) + marker + strings.TrimSpace(tail)
}

type contextBlock struct {
	title    string
	content  string
	priority int
}

func truncateContextByPriority(gameContext string, maxLen int) (string, []string, bool) {
	if len(gameContext) <= maxLen {
		return gameContext, nil, false
	}

	blocks := splitContextBlocks(gameContext)
	if len(blocks) == 0 {
		return truncateMiddle(gameContext, maxLen), []string{"all"}, true
	}

	totalLen := totalBlocksLength(blocks)
	if totalLen <= maxLen {
		return joinBlocks(blocks), nil, false
	}

	// Сначала пытаемся суммировать блоки вместо их удаления
	summarizedBlocks := summarizeBlocks(blocks, maxLen)

	// Проверяем, удалось ли уложиться в лимит после summarization
	totalLen = totalBlocksLength(summarizedBlocks)
	if totalLen <= maxLen {
		return joinBlocks(summarizedBlocks), nil, false
	}

	// Если summarization недостаточно, удаляем блоки с самым низким приоритетом
	removed := map[int]bool{}
	removedTitles := []string{}

	removable := []int{}
	for i, block := range summarizedBlocks {
		if block.priority > 1 {
			removable = append(removable, i)
		}
	}

	sort.Slice(removable, func(i, j int) bool {
		left := summarizedBlocks[removable[i]]
		right := summarizedBlocks[removable[j]]
		if left.priority != right.priority {
			return left.priority > right.priority
		}
		return len(left.content) > len(right.content)
	})

	for _, idx := range removable {
		if totalLen <= maxLen {
			break
		}
		removed[idx] = true
		removedTitles = append(removedTitles, summarizedBlocks[idx].title)
		totalLen -= len(summarizedBlocks[idx].content)
	}

	resultBlocks := []contextBlock{}
	for i, block := range summarizedBlocks {
		if !removed[i] {
			resultBlocks = append(resultBlocks, block)
		}
	}

	result := joinBlocks(resultBlocks)
	if len(result) > maxLen {
		return truncateMiddle(result, maxLen), removedTitles, true
	}

	return result, removedTitles, false
}

func splitContextBlocks(gameContext string) []contextBlock {
	segments := strings.Split(gameContext, "\n--- ")
	blocks := []contextBlock{}

	if len(segments) == 0 {
		return blocks
	}

	base := strings.TrimSpace(segments[0])
	if base != "" {
		blocks = append(blocks, contextBlock{
			title:    "base",
			content:  base,
			priority: contextBlockPriority("base"),
		})
	}

	for i := 1; i < len(segments); i++ {
		segment := segments[i]
		header := segment
		body := ""
		if idx := strings.Index(segment, "\n"); idx >= 0 {
			header = segment[:idx]
			body = segment[idx+1:]
		}
		title := strings.TrimSuffix(strings.TrimSpace(header), " ---")
		content := strings.TrimSpace("--- " + header)
		if strings.TrimSpace(body) != "" {
			content = content + "\n" + strings.TrimSpace(body)
		}
		if title == "" {
			title = "unknown"
		}
		blocks = append(blocks, contextBlock{
			title:    title,
			content:  content,
			priority: contextBlockPriority(title),
		})
	}

	return blocks
}

// summarizeBlocks сокращает содержимое блоков для экономии места
func summarizeBlocks(blocks []contextBlock, targetMaxLen int) []contextBlock {
	result := make([]contextBlock, len(blocks))

	for i, block := range blocks {
		result[i] = contextBlock{
			title:    block.title,
			priority: block.priority,
			content:  summarizeBlockContent(block.title, block.content, targetMaxLen/len(blocks)), // Распределяем лимит между блоками
		}
	}

	return result
}

// summarizeBlockContent сокращает содержимое конкретного блока
func summarizeBlockContent(title, content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}

	switch title {
	case "Последние сообщения":
		return summarizeRecentMessages(content, maxLen)
	case "Релевантная история игры (найдено через поиск)":
		return summarizeRAGHistory(content, maxLen)
	case "Инвентарь персонажа":
		return summarizeInventory(content, maxLen)
	case "Игроки в сессии":
		return summarizePlayers(content, maxLen)
	default:
		// Для остальных блоков используем простое усечение
		return truncateMiddle(content, maxLen)
	}
}

// summarizeRecentMessages сокращает блок последних сообщений
func summarizeRecentMessages(content string, maxLen int) string {
	lines := strings.Split(content, "\n")
	if len(lines) <= 3 {
		return truncateMiddle(content, maxLen)
	}

	// Оставляем только последние 3 сообщения
	header := lines[0] // "--- Последние сообщения ---"
	recentLines := lines[len(lines)-3:]
	result := header + "\n" + strings.Join(recentLines, "\n")

	if len(result) > maxLen {
		return truncateMiddle(result, maxLen)
	}

	return result
}

// summarizeRAGHistory сокращает блок RAG истории
func summarizeRAGHistory(content string, maxLen int) string {
	lines := strings.Split(content, "\n")
	if len(lines) <= 5 {
		return truncateMiddle(content, maxLen)
	}

	// Оставляем заголовок и 3 наиболее релевантных результата
	header := lines[0] // "--- Релевантная история игры (найдено через поиск) ---"
	resultLines := []string{header}

	// Пропускаем пустые строки и берем первые 3 результата
	count := 0
	for i := 1; i < len(lines) && count < 3; i++ {
		line := strings.TrimSpace(lines[i])
		if line != "" && !strings.HasPrefix(line, "-") {
			resultLines = append(resultLines, lines[i])
			count++
		}
	}

	result := strings.Join(resultLines, "\n")
	if len(result) > maxLen {
		return truncateMiddle(result, maxLen)
	}

	return result
}

// summarizeInventory сокращает блок инвентаря
func summarizeInventory(content string, maxLen int) string {
	lines := strings.Split(content, "\n")
	if len(lines) <= 5 {
		return truncateMiddle(content, maxLen)
	}

	// Оставляем заголовок, общее описание и 3 наиболее важных предмета
	headerLines := []string{}
	itemLines := []string{}

	for _, line := range lines {
		if strings.Contains(line, "---") || strings.Contains(line, "Общий вес") || strings.Contains(line, "Инвентарь пуст") {
			headerLines = append(headerLines, line)
		} else if strings.HasPrefix(line, "- ") && len(itemLines) < 3 {
			itemLines = append(itemLines, line)
		}
	}

	result := strings.Join(headerLines, "\n")
	if len(itemLines) > 0 {
		result += "\nПредметы (выборка):\n" + strings.Join(itemLines, "\n")
		if len(lines) > len(headerLines)+len(itemLines) {
			result += "\n... и другие предметы"
		}
	}

	if len(result) > maxLen {
		return truncateMiddle(result, maxLen)
	}

	return result
}

// summarizePlayers сокращает блок информации об игроках
func summarizePlayers(content string, maxLen int) string {
	lines := strings.Split(content, "\n")
	if len(lines) <= 3 {
		return truncateMiddle(content, maxLen)
	}

	// Оставляем только количество игроков
	for _, line := range lines {
		if strings.Contains(line, "Количество игроков") {
			result := lines[0] + "\n" + line // header + count
			if len(result) > maxLen {
				return truncateMiddle(result, maxLen)
			}
			return result
		}
	}

	return truncateMiddle(content, maxLen)
}

func contextBlockPriority(title string) int {
	switch title {
	case "Инструкции по проверкам навыков (ABILITY CHECKS)":
		return 1
	case "Текущая локация":
		return 1
	case "Текущий бой":
		return 1
	case "Ожидается проверка навыка":
		return 1
	case "Персонаж игрока":
		return 2
	case "Последние сообщения":
		return 2
	case "Инвентарь персонажа":
		return 2
	case "Релевантная история игры (найдено через поиск)":
		return 3
	case "Игроки в сессии":
		return 4
	case "base":
		return 2
	default:
		return 3
	}
}

func totalBlocksLength(blocks []contextBlock) int {
	length := 0
	for _, block := range blocks {
		length += len(block.content)
	}
	if len(blocks) > 1 {
		length += len(blocks) - 1
	}
	return length
}

func joinBlocks(blocks []contextBlock) string {
	parts := []string{}
	for _, block := range blocks {
		if strings.TrimSpace(block.content) != "" {
			parts = append(parts, block.content)
		}
	}
	return strings.Join(parts, "\n")
}

// buildCombatContext строит контекст боя для DM
func (uc *HandleActionUseCase) buildCombatContext(gs *session.GameSession, activeCombat *combat.Combat) string {
	var parts []string
	parts = append(parts, "--- Текущий бой ---")
	parts = append(parts, fmt.Sprintf("Статус: %s", activeCombat.State))

	// Добавляем информацию об участниках
	parts = append(parts, "\nУчастники боя:")
	for i, participant := range activeCombat.Participants {
		if participant.IsAlive() {
			name := participant.GetName()
			hp := participant.GetHP()
			maxHP := participant.GetMaxHP()
			ac := participant.GetAC()
			participantType := "👹 Враг"
			if participant.IsPlayer {
				participantType = "👤 Игрок"
			}
			isCurrent := i == activeCombat.CurrentTurn
			currentMarker := ""
			if isCurrent {
				currentMarker = " ← ТЕКУЩИЙ ХОД"
			}
			parts = append(parts, fmt.Sprintf("%s %s: %d/%d HP, AC %d%s",
				participantType, name, hp, maxHP, ac, currentMarker))
		}
	}

	return strings.Join(parts, "\n")
}

// buildDMPrompt - алиас для обратной совместимости
// Deprecated: используйте BuildDMPrompt вместо этого
func buildDMPrompt(gameContext, playerMessage string) string {
	// Используем настройки по умолчанию для обратной совместимости
	defaultPrefs := player.UserPreferences{
		NarrativeStyle: player.NarrativeStyleBalanced,
		DetailLevel:    player.DetailLevelMedium,
		Language:       "ru",
		ShowStats:      true,
	}
	return BuildDMPrompt(gameContext, playerMessage, defaultPrefs)
}

// buildActionAnalysisContext формирует контекст для DM на основе анализа действия игрока
func buildActionAnalysisContext(analysis *dm_analyzer.PlayerActionAnalysis) string {
	var parts []string

	if analysis.SimpleAction {
		parts = append(parts, "⚠️ Действие игрока простое - просто опиши результат естественным языком БЕЗ проверки.")
		if analysis.Recommendation != "" {
			parts = append(parts, fmt.Sprintf("Рекомендация: %s", analysis.Recommendation))
		}
		return strings.Join(parts, "\n")
	}

	if analysis.NeedsAbilityCheck && analysis.AbilityCheck != nil {
		parts = append(parts, "✅ Нужна проверка навыка:")
		parts = append(parts, fmt.Sprintf("- Характеристика: %s", analysis.AbilityCheck.Ability))
		parts = append(parts, fmt.Sprintf("- DC: %d", analysis.AbilityCheck.DC))
		parts = append(parts, fmt.Sprintf("- Причина: %s", analysis.AbilityCheck.Reason))
		if strings.TrimSpace(analysis.AbilityCheck.Stakes) != "" {
			parts = append(parts, fmt.Sprintf("- Ставки: %s", analysis.AbilityCheck.Stakes))
		}
		parts = append(parts, "⚠️ ВАЖНО: НЕ проси игрока бросать кубики. Система сама запросит /roll, если нужно.")
	}

	if analysis.NeedsPredefinedCheck && analysis.PredefinedCheck != nil {
		parts = append(parts, "📍 Нужна предопределенная проверка из локации:")
		parts = append(parts, fmt.Sprintf("- Локация: %s", analysis.PredefinedCheck.LocationName))
		parts = append(parts, fmt.Sprintf("- Индекс проверки: %d", analysis.PredefinedCheck.CheckIndex))
		parts = append(parts, fmt.Sprintf("- Причина: %s", analysis.PredefinedCheck.Reason))
		parts = append(parts, "⚠️ ВАЖНО: НЕ проси игрока бросать кубики. Система сама запросит /roll, если нужно.")
	}

	if analysis.NeedsRandomRoll && analysis.RandomRoll != nil {
		parts = append(parts, "🎲 Нужен случайный бросок кубика:")
		parts = append(parts, fmt.Sprintf("- Выражение: %s", analysis.RandomRoll.DiceExpression))
		parts = append(parts, fmt.Sprintf("- Причина: %s", analysis.RandomRoll.Reason))
		parts = append(parts, "⚠️ ВАЖНО: Используй инструмент 'roll_dice' с указанным выражением. НЕ проси игрока бросать кубики.")
	}

	if analysis.Recommendation != "" {
		parts = append(parts, "")
		parts = append(parts, fmt.Sprintf("Рекомендация: %s", analysis.Recommendation))
	}

	return strings.Join(parts, "\n")
}

func formatAbilityNameForPlayer(ability string) string {
	switch ability {
	case "strength":
		return "Силы (STR)"
	case "dexterity":
		return "Ловкости (DEX)"
	case "constitution":
		return "Телосложения (CON)"
	case "intelligence":
		return "Интеллекта (INT)"
	case "wisdom":
		return "Мудрости (WIS)"
	case "charisma":
		return "Харизмы (CHA)"
	default:
		return ability
	}
}

// createAbilityCheckFromAnalyzer создает pending ability check на основе вывода анализатора,
// без участия LLM-DM (DM не должен вызывать request_ability_check и не должен просить /roll).
func (uc *HandleActionUseCase) createAbilityCheckFromAnalyzer(
	ctx context.Context,
	gs *session.GameSession,
	check *dm_analyzer.AbilityCheckDetails,
) (string, bool, error) {
	if uc.sessionRepo == nil || gs == nil || check == nil {
		return "", false, nil
	}
	ability := strings.TrimSpace(check.Ability)
	reason := strings.TrimSpace(check.Reason)
	stakes := strings.TrimSpace(check.Stakes)
	dc := check.DC
	if ability == "" || dc <= 0 {
		return "", false, nil
	}
	if reason == "" {
		reason = "попытка с неопределенным исходом"
	}
	if stakes == "" {
		stakes = "успех даст преимущество, провал усложнит ситуацию"
	}

	eventRepoAdapter := &eventRepoAdapterForDMTools{repo: uc.eventRepo}
	tool := dm_tools.NewRequestAbilityCheckTool(uc.sessionRepo, eventRepoAdapter, gs.ChatID)

	result, err := tool.Execute(ctx, map[string]interface{}{
		"ability": ability,
		"dc":      dc,
		"reason":  reason,
		"stakes":  stakes,
	})
	if err != nil {
		return "", false, err
	}

	m, ok := result.(map[string]interface{})
	if !ok {
		// Если формат неожиданный — просто не перехватываем поток, чтобы не ломать игру.
		return "", false, nil
	}

	// Ветки отказов/авто-резолва/лимитов: возвращаем предупреждение как player-facing ответ.
	if autoResolved, _ := m["auto_resolved"].(bool); autoResolved {
		msg, _ := m["message"].(string)
		if strings.TrimSpace(msg) == "" {
			msg = "✅ Проверка не требуется — действие выполняется автоматически."
		}
		return msg, true, nil
	}
	for _, key := range []string{"rejected", "already_checked", "cooldown", "budget_exceeded", "already_pending"} {
		if v, _ := m[key].(bool); v {
			msg, _ := m["warning"].(string)
			if strings.TrimSpace(msg) == "" {
				msg = "⚠️ Проверка сейчас недоступна. Продолжай сцену без нового броска."
			}
			return msg, true, nil
		}
	}

	abilityName := formatAbilityNameForPlayer(ability)
	playerMsg := fmt.Sprintf(
		"🎲 Нужна проверка %s (DC %d).\nПричина: %s.\nНа кону: %s.\n\nНапишите /roll, чтобы бросить кубик. Можно отправить результат числом (например: 17).",
		abilityName, dc, reason, stakes,
	)

	// Чтобы не было дублирующего системного промпта из Telegram слоя — помечаем pending check как notified.
	// (RequestAbilityCheckTool выставляет notified=false, потому что обычно подсказку отправляет Telegram бот.)
	updated, getErr := uc.sessionRepo.GetByChatID(ctx, gs.ChatID)
	if getErr == nil && updated != nil && updated.HasPendingAbilityCheck() {
		updated.PendingAbilityCheckNotified = true
		_ = uc.sessionRepo.Save(ctx, updated)
	}

	return playerMsg, true, nil
}

// createToolRegistry создает реестр инструментов и регистрирует все доступные tools
func (uc *HandleActionUseCase) createToolRegistry(gs *session.GameSession, player *player.Player) *dm_tools.ToolRegistry {
	registry := dm_tools.NewToolRegistry()

	// Регистрируем инструменты для работы с инвентарем
	if uc.inventoryRepo != nil && player.CharacterID > 0 {
		registry.Register(dm_tools.NewGetInventoryTool(uc.inventoryRepo, player.CharacterID))
		registry.Register(dm_tools.NewAddItemTool(uc.inventoryRepo, player.CharacterID))
		registry.Register(dm_tools.NewRemoveItemTool(uc.inventoryRepo, player.CharacterID))
		// Регистрируем инструменты для валидации действий
		registry.Register(dm_tools.NewValidateItemUsageTool(uc.inventoryRepo, player.CharacterID))
	}

	// Регистрируем инструменты для работы с характеристиками персонажа
	if uc.sessionRepo != nil {
		registry.Register(dm_tools.NewGetCharacterStatsTool(uc.sessionRepo, gs.ChatID))
		registry.Register(dm_tools.NewGetCharacterAbilitiesTool(uc.sessionRepo, gs.ChatID))
		// Регистрируем инструмент для проверки требований к характеристикам
		registry.Register(dm_tools.NewCheckStatRequirementsTool(uc.sessionRepo, gs.ChatID))
	}

	// Регистрируем инструменты для работы с боем
	if uc.combatRepo != nil && uc.sessionRepo != nil {
		registry.Register(dm_tools.NewCheckCombatStatusTool(uc.combatRepo, gs.ID))
		registry.Register(dm_tools.NewGetBattlefieldStatusTool(uc.combatRepo, gs.ID))
		registry.Register(dm_tools.NewGetCombatParticipantStatsTool(uc.combatRepo, gs.ID))
		registry.Register(dm_tools.NewCompareAttackVsDefenseTool(uc.combatRepo, gs.ID))
		// Создаем адаптеры для GameSessionRepository и PlayerRepository
		sessionRepoAdapter := &sessionRepoAdapter{sessionRepo: uc.sessionRepo}
		playerRepoAdapter := &playerRepoAdapter{sessionRepo: uc.sessionRepo}
		registry.Register(dm_tools.NewPerformCombatAttackTool(uc.combatRepo, sessionRepoAdapter, gs.ID))
		registry.Register(dm_tools.NewPerformEnemyAttackTool(uc.combatRepo, sessionRepoAdapter, playerRepoAdapter, gs.ID, gs.ChatID))
		registry.Register(dm_tools.NewApplyDamageTool(uc.combatRepo, sessionRepoAdapter, playerRepoAdapter, gs.ID, gs.ChatID))
	}

	// Регистрируем инструмент для генерации изображений (если доступен)
	if uc.generateImageUC != nil {
		// Получаем userID из игрока (используем TgUserID если есть, иначе ChatID)
		userID := gs.ChatID // По умолчанию используем ChatID
		if player != nil && player.TgUserID > 0 {
			userID = player.TgUserID
		}
		// Создаем адаптер для разрыва циклической зависимости
		imageService := imageapp.NewImageGenerationServiceAdapter(uc.generateImageUC)
		// Создаем адаптер для проверки Premium статуса
		var subscriptionChecker dm_tools.SubscriptionChecker
		if uc.getSubscriptionUC != nil {
			subscriptionChecker = &subscriptionCheckerAdapter{
				getSubscriptionUC: uc.getSubscriptionUC,
			}
		}
		registry.Register(dm_tools.NewGenerateImageTool(imageService, gs.ChatID, userID, subscriptionChecker))
	}

	// Регистрируем инструмент для использования заклинаний (если доступен)
	if uc.useSpellUC != nil && uc.sessionRepo != nil {
		sessionAdapter := &sessionRepoAdapter{sessionRepo: uc.sessionRepo}
		var gameSessionRepo dm_tools.GameSessionRepository = sessionAdapter
		registry.Register(dm_tools.NewUseSpellTool(uc.useSpellUC, gameSessionRepo, gs.ID, gs.ChatID))
	}

	// Регистрируем инструмент для бросков кубиков DM
	if uc.eventRepo != nil {
		rollDiceUC := diceapp.NewRollDiceUseCase()
		eventRepoAdapter := &eventRepoAdapterForDMTools{repo: uc.eventRepo}
		registry.Register(dm_tools.NewRollDiceTool(rollDiceUC, eventRepoAdapter, gs.ID))
	}

	// Регистрируем инструмент для отправки дополнительных сообщений
	if uc.eventRepo != nil {
		eventRepoAdapter := &eventRepoAdapterForDMTools{repo: uc.eventRepo}
		registry.Register(dm_tools.NewSendFollowupMessageTool(eventRepoAdapter, gs.ID, gs.ChatID))
	}

	return registry
}

// generateWithToolsLoop реализует multi-step loop для работы с tools
// Возвращает финальный ответ DM с явно включенными результатами combat tools
func (uc *HandleActionUseCase) generateWithToolsLoop(ctx context.Context, prompt string, toolRegistry *dm_tools.ToolRegistry, gs *session.GameSession) (string, []map[string]interface{}, error) {
	const maxIterations = 3 // Максимальное количество итераций для предотвращения бесконечного цикла

	allTools := toolRegistry.GetAll()
	currentPrompt := prompt

	// Собираем результаты combat tools для явного отображения в чате
	var combatResults []string
	// Собираем информацию о сгенерированных изображениях
	var imageResults []map[string]interface{}

	for iteration := 0; iteration < maxIterations; iteration++ {
		// Генерируем ответ с инструментами
		// Explicitly type the tools slice to help IDE resolve the type correctly
		var tools []dm_tools.Tool = allTools
		llmResponse, err := uc.llm.GenerateWithTools(ctx, currentPrompt, tools)
		if err != nil {
			return "", nil, fmt.Errorf("failed to generate response with tools: %w", err)
		}

		// Если нет вызовов инструментов, возвращаем финальный ответ с результатами combat tools
		if llmResponse.Finished || len(llmResponse.ToolCalls) == 0 {
			finalResponse := llmResponse.Content
			// Очищаем теги tool_call если они есть
			if strings.Contains(finalResponse, "<tool_call") {
				finalResponse = dm_tools.CleanToolCallTags(finalResponse)
			}
			// Удаляем технические детали из ответа (шаги, инструкции)
			finalResponse = cleanTechnicalDetails(finalResponse)
			finalResponse = sanitizePlayerFacingResponse(finalResponse)

			// Добавляем результаты combat tools в начало ответа, если они есть
			if len(combatResults) > 0 {
				combatSection := strings.Join(combatResults, "\n\n")
				return fmt.Sprintf("%s\n\n%s", combatSection, finalResponse), imageResults, nil
			}

			return finalResponse, imageResults, nil
		}

		// Выполняем вызовы инструментов
		logger.Info("DM requested tool calls",
			logger.Uint("session_id", gs.ID),
			logger.Int("iteration", iteration+1),
			logger.Int("tool_calls_count", len(llmResponse.ToolCalls)),
		)

		// Логируем каждый вызов инструмента
		for i, call := range llmResponse.ToolCalls {
			argsJSON, _ := json.Marshal(call.Arguments)
			logger.Info("DM tool call requested",
				logger.Uint("session_id", gs.ID),
				logger.Int("iteration", iteration+1),
				logger.Int("call_index", i+1),
				logger.Int("total_calls", len(llmResponse.ToolCalls)),
				logger.String("tool_name", call.Name),
				logger.String("arguments", string(argsJSON)),
			)
		}

		executor := dm_tools.NewToolExecutor(toolRegistry)
		results, err := executor.ExecuteToolCalls(ctx, llmResponse.ToolCalls)
		if err != nil {
			logger.Warn("Failed to execute tool calls",
				logger.Uint("session_id", gs.ID),
				logger.ErrorField(err),
				logger.Int("iteration", iteration+1),
			)
			// Продолжаем без результатов инструментов - очищаем теги tool_call
			cleanedResponse := llmResponse.Content
			if strings.Contains(cleanedResponse, "<tool_call") {
				cleanedResponse = dm_tools.CleanToolCallTags(cleanedResponse)
			}
			cleanedResponse = cleanTechnicalDetails(cleanedResponse)
			cleanedResponse = sanitizePlayerFacingResponse(cleanedResponse)
			// Возвращаем ответ с собранными результатами combat tools
			if len(combatResults) > 0 {
				combatSection := strings.Join(combatResults, "\n\n")
				return fmt.Sprintf("%s\n\n%s", combatSection, cleanedResponse), imageResults, nil
			}
			return cleanedResponse, imageResults, nil
		}

		// Извлекаем результаты combat tools для явного отображения
		// Извлекаем результаты generate_image tool для отправки изображений
		for _, result := range results {
			if result.Success && (result.ToolName == "perform_combat_attack" ||
				result.ToolName == "apply_damage" ||
				result.ToolName == "perform_enemy_attack") {
				combatMsg := extractCombatToolMessage(result)
				if combatMsg != "" {
					combatResults = append(combatResults, combatMsg)
				}
			}

			// Извлекаем результаты generate_image tool для отправки изображений
			if result.Success && result.ToolName == "generate_image" {
				if resultMap, ok := result.Result.(map[string]interface{}); ok {
					if imagePath, ok := resultMap["image_path"].(string); ok && imagePath != "" {
						imageInfo := map[string]interface{}{
							"image_path": imagePath,
						}
						if desc, ok := resultMap["description"].(string); ok {
							imageInfo["description"] = desc
						}
						if msg, ok := resultMap["message"].(string); ok {
							imageInfo["message"] = msg
						}
						imageResults = append(imageResults, imageInfo)
					}
				}
			}

			// Обработка результата evaluate_check для завершения событий локации
			if result.Success && result.ToolName == "evaluate_check" {
				if err := uc.resolveLocationEventFromCheck(ctx, gs, result); err != nil {
					logger.Warn("Failed to resolve location event from evaluate_check",
						logger.ErrorField(err),
						logger.Uint("session_id", gs.ID),
					)
				}
			}
		}

		// Форматируем результаты для передачи обратно DM
		toolResults := dm_tools.FormatToolResults(results)

		// Формируем новый промпт с результатами инструментов
		currentPrompt = fmt.Sprintf(`%s

%s

Ты получил результаты вызова инструментов. Используй эту информацию для формирования финального ответа игроку. 
Не вызывай инструменты повторно, просто используй полученную информацию.`,
			currentPrompt, toolResults)

		// Подсчитываем успешные и неуспешные вызовы
		successfulCount := 0
		failedCount := 0
		for _, result := range results {
			if result.Success {
				successfulCount++
			} else {
				failedCount++
			}
		}

		logger.Info("Tool calls execution completed",
			logger.Uint("session_id", gs.ID),
			logger.Int("iteration", iteration+1),
			logger.Int("total_calls", len(llmResponse.ToolCalls)),
			logger.Int("successful", successfulCount),
			logger.Int("failed", failedCount),
		)
	}

	// Если достигнут максимум итераций, возвращаем последний ответ без tool calls
	llmResponse, err := uc.llm.Generate(ctx, currentPrompt)
	if err != nil {
		return "", imageResults, fmt.Errorf("failed to generate final response: %w", err)
	}

	cleanedResponse := llmResponse
	if strings.Contains(cleanedResponse, "<tool_call") {
		cleanedResponse = dm_tools.CleanToolCallTags(cleanedResponse)
	}
	// Удаляем технические детали из ответа
	cleanedResponse = cleanTechnicalDetails(cleanedResponse)
	cleanedResponse = sanitizePlayerFacingResponse(cleanedResponse)

	// Добавляем результаты combat tools в начало ответа, если они есть
	if len(combatResults) > 0 {
		combatSection := strings.Join(combatResults, "\n\n")
		return fmt.Sprintf("%s\n\n%s", combatSection, cleanedResponse), imageResults, nil
	}

	return cleanedResponse, imageResults, nil
}

func (uc *HandleActionUseCase) resolveLocationEventFromCheck(
	ctx context.Context,
	gs *session.GameSession,
	result dm_tools.ToolResult,
) error {
	if gs == nil || gs.CurrentLocationID == nil || len(gs.World.Events) == 0 {
		return nil
	}

	resultMap, ok := result.Result.(map[string]interface{})
	if !ok {
		return nil
	}

	success, ok := resultMap["success"].(bool)
	if !ok {
		return nil
	}
	message, _ := resultMap["message"].(string)

	var target *world.WorldEvent
	for i := range gs.World.Events {
		ev := &gs.World.Events[i]
		if !isLocationEventType(ev.Type) || ev.Status != world.WorldEventStatusActive {
			continue
		}
		if ev.RequiredLocationID != nil && *ev.RequiredLocationID == *gs.CurrentLocationID {
			target = ev
			break
		}
	}

	if target == nil {
		return nil
	}

	now := time.Now()

	// Обработка события с ветками развития
	outcome, selectedBranch := uc.processLocationEventWithBranches(target, success)

	if success {
		target.Complete()
		target.UpdatedAt = now
		target.Metadata = updateLocationEventMetadataWithOutcome(target.Metadata, "resolved_success", outcome)
	} else {
		target.Cancel()
		target.CompletedAt = &now
		target.UpdatedAt = now
		target.Metadata = updateLocationEventMetadataWithOutcome(target.Metadata, "resolved_fail", outcome)
	}

	if err := uc.sessionRepo.Save(ctx, gs); err != nil {
		return fmt.Errorf("failed to save location event outcome: %w", err)
	}

	content := buildLocationEventOutcomeStoryWithBranch(target, outcome, selectedBranch, message)

	if uc.eventRepo != nil {
		eventItem := &event.StoryEvent{
			GameSessionID: gs.ID,
			AuthorType:    event.AuthorTypeDM,
			Content:       content,
			CreatedAt:     time.Now(),
		}
		if err := uc.eventRepo.Save(ctx, eventItem); err != nil {
			return fmt.Errorf("failed to save location outcome story event: %w", err)
		}
	}

	if uc.indexDocUC != nil {
		doc := ragdomain.Document{
			ID:        uuid.New().String(),
			Source:    ragdomain.SourceEvent,
			SessionID: gs.ID,
			Text:      content,
			Timestamp: time.Now(),
		}
		if err := uc.indexDocUC.Execute(ctx, doc); err != nil {
			logger.Warn("Failed to index location outcome in RAG, continuing with DB storage only",
				logger.ErrorField(err),
				logger.Uint("session_id", gs.ID),
			)
			// Продолжаем - событие сохранено в БД, RAG индексация опциональна
		}
	}

	return nil
}

func isLocationEventType(t world.WorldEventType) bool {
	switch t {
	case world.WorldEventTypeLocationNPC,
		world.WorldEventTypeLocationItem,
		world.WorldEventTypeLocationTrap,
		world.WorldEventTypeLocationPuzzle,
		world.WorldEventTypeLocationEncounter:
		return true
	default:
		return false
	}
}

func updateLocationEventMetadataStatus(meta []byte, status string) []byte {
	if len(meta) == 0 {
		return meta
	}
	var payload world.LocationEventMetadata
	if err := json.Unmarshal(meta, &payload); err != nil {
		return meta
	}
	payload.Status = status
	updated, err := json.Marshal(payload)
	if err != nil {
		return meta
	}
	return updated
}

// processLocationEventWithBranches обрабатывает событие с ветками развития
func (uc *HandleActionUseCase) processLocationEventWithBranches(ev *world.WorldEvent, success bool) (*world.LocationEventOutcome, *world.LocationEventBranch) {
	var meta world.LocationEventMetadata
	if len(ev.Metadata) > 0 {
		_ = json.Unmarshal(ev.Metadata, &meta)
	}

	// Если нет веток, используем старую логику
	if len(meta.Branches) == 0 {
		outcome := &world.LocationEventOutcome{
			Success:      success,
			Description:  "Проверка завершена",
			Consequences: "Событие разрешено",
		}
		return outcome, nil
	}

	// Выбираем ветку случайным образом (в будущем можно сделать выбор на основе действия игрока)
	selectedBranch := &meta.Branches[rng.Intn(len(meta.Branches))]

	// Определяем исход на основе успеха и ветки
	outcome := &world.LocationEventOutcome{
		BranchID: selectedBranch.ID,
		Success:  success,
	}

	if success {
		outcome.Description = fmt.Sprintf("Вы успешно выбрали подход: %s", selectedBranch.Name)
		outcome.Consequences = selectedBranch.Consequences
		outcome.Reward = selectedBranch.Reward

		// Специальная обработка рекрутинга компаньона
		if selectedBranch.ID == "recruit_companion" {
			if err := uc.recruitCompanion(ev, outcome); err != nil {
				logger.Error("Failed to recruit companion", logger.ErrorField(err))
			}
		}
	} else {
		outcome.Description = fmt.Sprintf("Ваш подход '%s' не удался", selectedBranch.Name)
		outcome.Consequences = "Неудача привела к негативным последствиям"
	}

	return outcome, selectedBranch
}

// recruitCompanion рекрутирует NPC как компаньона игрока
func (uc *HandleActionUseCase) recruitCompanion(ev *world.WorldEvent, outcome *world.LocationEventOutcome) error {
	// Получаем сессию
	gs, err := uc.sessionRepo.GetByChatID(context.Background(), 0) // TODO: передать правильный chatID
	if err != nil {
		return err
	}
	if gs == nil {
		return fmt.Errorf("session not found")
	}

	// Создаем случайного компаньона
	companionClasses := []string{"Воин", "Маг", "Разбойник", "Целитель"}
	companionNames := []string{"Алара", "Борис", "Виктор", "Галина", "Дмитрий", "Елена", "Жанна", "Захар"}

	selectedClass := companionClasses[rng.Intn(len(companionClasses))]
	selectedName := companionNames[rng.Intn(len(companionNames))]

	companion := &session.Companion{
		GameSessionID: gs.ID,
		Name:          selectedName,
		Description:   fmt.Sprintf("%s класса %s", selectedName, selectedClass),
		Class:         selectedClass,
		Level:         1 + rng.Intn(3), // Уровень 1-3
		HP:            20 + rng.Intn(30), // HP 20-49
		MaxHP:         20 + rng.Intn(30),
		AC:            12 + rng.Intn(4), // AC 12-15
		AttackBonus:   2 + rng.Intn(3), // +2 to +4
		DamageDice:    "1d8", // Простое оружие
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	// Устанавливаем HP равным MaxHP
	companion.HP = companion.MaxHP

	// Добавляем компаньона в сессию
	gs.AddCompanion(companion)

	// Сохраняем сессию
	if err := uc.sessionRepo.Save(context.Background(), gs); err != nil {
		return err
	}

	outcome.Reward = fmt.Sprintf("Новый компаньон: %s (%s %d уровня)", companion.Name, companion.Class, companion.Level)
	return nil
}

// updateLocationEventMetadataWithOutcome обновляет метаданные с исходом события
func updateLocationEventMetadataWithOutcome(meta []byte, status string, outcome *world.LocationEventOutcome) []byte {
	if len(meta) == 0 {
		return meta
	}
	var payload world.LocationEventMetadata
	if err := json.Unmarshal(meta, &payload); err != nil {
		return meta
	}
	payload.Status = status
	payload.Outcome = outcome
	updated, err := json.Marshal(payload)
	if err != nil {
		return meta
	}
	return updated
}

func buildLocationEventOutcomeStory(ev *world.WorldEvent, outcome, checkMessage string) string {
	if ev == nil {
		return outcome
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("Событие локации: %s", ev.Name))
	if outcome != "" {
		parts = append(parts, outcome)
	}
	if checkMessage != "" {
		parts = append(parts, fmt.Sprintf("Результат проверки: %s", checkMessage))
	}
	if ev.Description != "" {
		parts = append(parts, fmt.Sprintf("Описание: %s", ev.Description))
	}

	return strings.Join(parts, "\n")
}

// buildLocationEventOutcomeStoryWithBranch строит историю исхода события с веткой развития
func buildLocationEventOutcomeStoryWithBranch(ev *world.WorldEvent, outcome *world.LocationEventOutcome, selectedBranch *world.LocationEventBranch, checkMessage string) string {
	if ev == nil {
		return "Событие завершено"
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("🎲 Событие локации: %s", ev.Name))

	if selectedBranch != nil {
		parts = append(parts, fmt.Sprintf("📋 Выбранный подход: %s", selectedBranch.Name))
		parts = append(parts, fmt.Sprintf("💡 Описание: %s", selectedBranch.Description))
	}

	if outcome != nil {
		status := "✅"
		if !outcome.Success {
			status = "❌"
		}
		parts = append(parts, fmt.Sprintf("%s Исход: %s", status, outcome.Description))
		parts = append(parts, fmt.Sprintf("📖 Последствия: %s", outcome.Consequences))

		if outcome.Reward != "" {
			parts = append(parts, fmt.Sprintf("🎁 Награда: %s", outcome.Reward))
		}
	}

	if checkMessage != "" {
		parts = append(parts, fmt.Sprintf("🎯 Результат проверки: %s", checkMessage))
	}

	return strings.Join(parts, "\n")
}

// extractNumber безопасно извлекает число из interface{}, обрабатывая int и float64
func extractNumber(v interface{}) float64 {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case float64:
		return val
	case float32:
		return float64(val)
	default:
		return 0
	}
}

func getEnvInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		logger.Warn("Invalid env value, using fallback",
			logger.String("key", key),
			logger.String("value", raw),
			logger.Int("fallback", fallback),
		)
		return fallback
	}
	return value
}

// extractCombatToolMessage извлекает читаемое сообщение из результата combat tool для явного отображения в чате
func extractCombatToolMessage(result dm_tools.ToolResult) string {
	resultMap, ok := result.Result.(map[string]interface{})
	if !ok {
		return ""
	}

	var parts []string

	if result.ToolName == "perform_combat_attack" {
		// Форматируем результат атаки
		hit, _ := resultMap["hit"].(bool)
		criticalHit, _ := resultMap["critical_hit"].(bool)
		attackerName, _ := resultMap["attacker_name"].(string)
		targetName, _ := resultMap["target_name"].(string)
		attackRoll := extractNumber(resultMap["attack_roll"])
		ac := extractNumber(resultMap["ac"])
		damage := extractNumber(resultMap["damage"])
		targetHP := extractNumber(resultMap["target_hp"])
		targetMaxHP := extractNumber(resultMap["target_max_hp"])

		if criticalHit {
			parts = append(parts, "🎯 КРИТИЧЕСКИЙ УДАР!")
		}

		if hit {
			parts = append(parts, fmt.Sprintf("⚔️ %s атакует %s и попадает! (бросок: %.0f против AC %.0f)", attackerName, targetName, attackRoll, ac))
			parts = append(parts, fmt.Sprintf("💥 Урон: %.0f", damage))
			parts = append(parts, fmt.Sprintf("❤️ %s: %.0f/%.0f HP", targetName, targetHP, targetMaxHP))
		} else {
			parts = append(parts, fmt.Sprintf("❌ %s атакует %s, но промахивается! (бросок: %.0f против AC %.0f)", attackerName, targetName, attackRoll, ac))
		}

		if combatFinished, ok := resultMap["combat_finished"].(bool); ok && combatFinished {
			if victory, ok := resultMap["victory"].(bool); ok {
				if victory {
					parts = append(parts, "🎉 Победа! Все враги повержены!")
				} else {
					parts = append(parts, "💀 Поражение! Все игроки повержены!")
				}
			}
		}
	} else if result.ToolName == "apply_damage" {
		// Форматируем результат нанесения урона
		if message, ok := resultMap["message"].(string); ok && message != "" {
			parts = append(parts, message)
		} else {
			// Если нет сообщения, формируем сами
			targetName, _ := resultMap["target_name"].(string)
			damage, _ := resultMap["damage"].(float64)
			newHP, _ := resultMap["new_hp"].(float64)
			maxHP, _ := resultMap["max_hp"].(float64)
			isDead, _ := resultMap["is_dead"].(bool)

			parts = append(parts, fmt.Sprintf("💥 %s получил(а) %.0f урона. HP: %.0f/%.0f", targetName, damage, newHP, maxHP))
			if isDead {
				parts = append(parts, fmt.Sprintf("💀 %s повержен(а)!", targetName))
			}
		}

		if combatFinished, ok := resultMap["combat_finished"].(bool); ok && combatFinished {
			if victory, ok := resultMap["victory"].(bool); ok {
				if victory {
					parts = append(parts, "🎉 Победа! Все враги повержены!")
				} else {
					parts = append(parts, "💀 Поражение! Все игроки повержены!")
				}
			}
		}
	} else if result.ToolName == "perform_enemy_attack" {
		// Форматируем результат атаки врага
		hit, _ := resultMap["hit"].(bool)
		criticalHit, _ := resultMap["critical_hit"].(bool)
		attackerName, _ := resultMap["attacker_name"].(string)
		targetName, _ := resultMap["target_name"].(string)
		attackRoll := extractNumber(resultMap["attack_roll"])
		ac := extractNumber(resultMap["ac"])
		damage := extractNumber(resultMap["damage"])
		targetHP := extractNumber(resultMap["target_hp"])
		targetMaxHP := extractNumber(resultMap["target_max_hp"])

		if criticalHit {
			parts = append(parts, "🎯 КРИТИЧЕСКИЙ УДАР!")
		}

		if hit {
			parts = append(parts, fmt.Sprintf("⚔️ %s атакует %s и попадает! (бросок: %.0f против AC %.0f)", attackerName, targetName, attackRoll, ac))
			parts = append(parts, fmt.Sprintf("💥 Урон: %.0f", damage))
			parts = append(parts, fmt.Sprintf("❤️ %s: %.0f/%.0f HP", targetName, targetHP, targetMaxHP))
		} else {
			parts = append(parts, fmt.Sprintf("❌ %s атакует %s, но промахивается! (бросок: %.0f против AC %.0f)", attackerName, targetName, attackRoll, ac))
		}

		if combatFinished, ok := resultMap["combat_finished"].(bool); ok && combatFinished {
			if victory, ok := resultMap["victory"].(bool); ok {
				if victory {
					parts = append(parts, "🎉 Победа! Все враги повержены!")
				} else {
					parts = append(parts, "💀 Поражение! Все игроки повержены!")
				}
			}
		}

		// Добавляем информацию о следующем ходе после атаки врага
		if nextTurn, ok := resultMap["next_turn"].(string); ok && nextTurn != "" {
			parts = append(parts, "")
			parts = append(parts, nextTurn)
		}
	}

	if len(parts) > 0 {
		return strings.Join(parts, "\n")
	}
	return ""
}

// achievementCheckerAdapter адаптирует achievementapp.CheckAchievementsUseCase к dm_analyzer.AchievementChecker
type achievementCheckerAdapter struct {
	checkAchievementsUC *achievementapp.CheckAchievementsUseCase
}

// dailyQuestProgressAdapter адаптирует CheckDailyQuestProgressUseCase к интерфейсу dm_analyzer.DailyQuestProgressChecker
type dailyQuestProgressAdapter struct {
	checkDailyProgressUC DailyQuestProgressChecker
}

func (a *dailyQuestProgressAdapter) Execute(
	ctx context.Context,
	req dm_analyzer.DailyQuestProgressRequest,
) error {
	// Преобразуем запрос из dm_analyzer в формат quest.CheckDailyQuestProgressUseCase
	// Нужно преобразовать QuestType из строки в quest.DailyQuestType
	questType := quest.DailyQuestType(req.QuestType)
	progressReq := CheckDailyQuestProgressRequest{
		ChatID:    req.ChatID,
		TgUserID:  req.TgUserID,
		QuestType: questType,
		Increment: req.Increment,
	}

	return a.checkDailyProgressUC.Execute(ctx, progressReq)
}

// imageGenerationServiceAdapter адаптирует ImageGenerationUseCase к интерфейсу dm_analyzer.ImageGenerationService
type imageGenerationServiceAdapter struct {
	uc *imageapp.ImageGenerationUseCase
}

func (a *imageGenerationServiceAdapter) GenerateImage(ctx context.Context, req dm_analyzer.GenerateImageRequest) (*dm_analyzer.GenerateImageResponse, error) {
	// Преобразуем запрос из dm_analyzer в внутренний формат
	internalReq := imageapp.GenerateImageRequest{
		SystemPrompt:    req.SystemPrompt,
		UserPrompt:      req.UserPrompt,
		Type:            req.Type,
		EntityID:        req.EntityID,
		ForceRegenerate: req.ForceRegenerate,
		UserID:          req.UserID,
		SkipLimitCheck:  req.SkipLimitCheck,
	}

	// Выполняем генерацию
	resp, err := a.uc.Execute(ctx, internalReq)
	if err != nil {
		return nil, err
	}

	// Преобразуем ответ из внутреннего формата в формат dm_analyzer
	return &dm_analyzer.GenerateImageResponse{
		ImagePath: resp.ImagePath,
		FileID:    resp.FileID,
		FromCache: resp.FromCache,
	}, nil
}

func (a *achievementCheckerAdapter) Execute(
	ctx context.Context,
	req dm_analyzer.CheckAchievementsRequest,
) ([]dm_analyzer.AchievementUnlocked, error) {
	achievementReq := achievementapp.CheckAchievementsRequest{
		PlayerID:       req.PlayerID,
		RequirementKey: req.RequirementKey,
		CurrentValue:   req.CurrentValue,
	}

	unlocked, err := a.checkAchievementsUC.Execute(ctx, achievementReq)
	if err != nil {
		return nil, err
	}

	// Конвертируем в формат dm_analyzer
	result := make([]dm_analyzer.AchievementUnlocked, len(unlocked))
	for i, u := range unlocked {
		result[i] = dm_analyzer.AchievementUnlocked{
			Achievement: dm_analyzer.Achievement{
				Code:        u.Achievement.Code,
				Title:       u.Achievement.Title,
				Description: u.Achievement.Description,
			},
			Message: u.Message,
		}
	}

	return result, nil
}


// subscriptionCheckerAdapter адаптирует subscriptionapp.GetSubscriptionUseCase к интерфейсу dm_tools.SubscriptionChecker
type subscriptionCheckerAdapter struct {
	getSubscriptionUC *subscriptionapp.GetSubscriptionUseCase
}

func (a *subscriptionCheckerAdapter) IsPremium(ctx context.Context, tgUserID int64) (bool, error) {
	req := subscriptionapp.GetSubscriptionRequest{
		TgUserID: tgUserID,
	}
	resp, err := a.getSubscriptionUC.Execute(ctx, req)
	if err != nil {
		return false, err
	}
	if resp == nil || resp.Subscription == nil {
		return false, nil
	}
	// Premium и Pro пользователи имеют безлимитную генерацию изображений
	return resp.Subscription.IsActive() &&
		(resp.Subscription.Plan == subscription.PlanPremium ||
			resp.Subscription.Plan == subscription.PlanPro), nil
}

// parseTimeChangeCommand анализирует сообщение игрока на предмет команд изменения времени
func (uc *HandleActionUseCase) parseTimeChangeCommand(message string) string {
	message = strings.ToLower(strings.TrimSpace(message))

	// Паттерны для распознавания команд времени
	timePatterns := map[string]string{
		// Русские паттерны
		"подождать до утра":     "morning",
		"дождаться утра":        "morning",
		"подождать до вечера":   "evening",
		"дождаться вечера":      "evening",
		"подождать до ночи":     "night",
		"дождаться ночи":        "night",
		"подождать до полудня":  "noon",
		"дождаться полудня":     "noon",
		"подождать до дня":      "noon",
		"дождаться дня":         "noon",
		"подождать до полночь":  "midnight",
		"дождаться полночь":     "midnight",

		// Английские паттерны
		"wait until morning":    "morning",
		"wait until evening":    "evening",
		"wait until night":      "night",
		"wait until noon":       "noon",
		"wait until midnight":   "midnight",
	}

	for pattern, timeOfDay := range timePatterns {
		if strings.Contains(message, pattern) {
			return timeOfDay
		}
	}

	return ""
}

// handleTimeChange обрабатывает изменение времени суток
func (uc *HandleActionUseCase) handleTimeChange(ctx context.Context, gs *session.GameSession, newTimeOfDay string) (string, error) {
	oldTime := gs.World.TimeOfDay

	// Изменяем время суток
	gs.World.SetTimeOfDay(newTimeOfDay)

	// Сохраняем изменения в БД
	dbCtx, dbCancel := context.WithTimeout(ctx, 5*time.Second)
	defer dbCancel()

	if err := uc.sessionRepo.Save(dbCtx, gs); err != nil {
		logger.Error("Failed to save time change",
			logger.ErrorField(err),
			logger.Uint("session_id", gs.ID),
		)
		return "", fmt.Errorf("failed to save time change: %w", err)
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

	response := fmt.Sprintf("⏰ Вы ждете подходящего момента...\n\n%s → %s\n\nВремя в мире изменилось. %s наступил.",
		timeDescriptions[oldTime], timeDescriptions[newTimeOfDay], strings.ToLower(timeDescriptions[newTimeOfDay]))

	logger.Info("Time changed via natural language",
		logger.String("old_time", oldTime),
		logger.String("new_time", newTimeOfDay),
		logger.Uint("session_id", gs.ID),
	)

	return response, nil
}

// updateSessionGoalsProgress обновляет прогресс сессионных целей на основе анализа ответа DM
func (uc *HandleActionUseCase) updateSessionGoalsProgress(
	ctx context.Context,
	gs *session.GameSession,
	analysis *dm_analyzer.DMResponseAnalysis,
) {
	// Обновляем прогресс исследования локаций
	if analysis.LocationVisited != nil && analysis.LocationVisited.IsFirstVisit {
		logger.Info("Location visited - updating exploration goal progress",
			logger.Uint("session_id", gs.ID),
			logger.String("location", analysis.LocationVisited.Name),
		)
		gs.UpdateGoalProgress(session.GoalTypeExploration, 1)
	}

	// Обновляем прогресс получения опыта
	if analysis.ExperienceGained > 0 {
		logger.Info("Experience gained - updating experience goal progress",
			logger.Uint("session_id", gs.ID),
			logger.Int("experience", analysis.ExperienceGained),
		)
		gs.UpdateGoalProgress(session.GoalTypeExperience, analysis.ExperienceGained)
	}

	// Сохраняем обновленный прогресс целей
	if err := uc.sessionRepo.Save(ctx, gs); err != nil {
		logger.Warn("Failed to save session goals progress",
			logger.ErrorField(err),
			logger.Uint("session_id", gs.ID),
		)
	}
}
