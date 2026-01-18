package player_action

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	achievementapp "dungeons-and-dragons-ai/internal/game/application/achievement"
	"dungeons-and-dragons-ai/internal/game/application/dm_analyzer"
	"dungeons-and-dragons-ai/internal/game/application/dm_tools"
	imageapp "dungeons-and-dragons-ai/internal/game/application/image"
	worldeventapp "dungeons-and-dragons-ai/internal/game/application/world_event"
	characterapp "dungeons-and-dragons-ai/internal/game/application/character"
	dmcache "dungeons-and-dragons-ai/internal/game/infrastructure/cache"
	"dungeons-and-dragons-ai/internal/game/domain/combat"
	"dungeons-and-dragons-ai/internal/game/domain/event"
	"dungeons-and-dragons-ai/internal/game/domain/inventory"
	"dungeons-and-dragons-ai/internal/game/domain/player"
	"dungeons-and-dragons-ai/internal/game/domain/quest"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/llm/domain"
	ragapp "dungeons-and-dragons-ai/internal/rag/application"
	ragdomain "dungeons-and-dragons-ai/internal/rag/domain"
	"dungeons-and-dragons-ai/pkg/logger"

	"github.com/google/uuid"
)

type HandleActionUseCase struct {
	llm                domain.LLM
	sessionRepo        session.Repository
	contextBuilder     ContextBuilder
	eventRepo          EventRepository
	indexDocUC         *ragapp.IndexDocument
	combatRepo         CombatRepository
	questRepo          QuestRepository
	inventoryRepo      InventoryRepository
	addExperienceUC    *characterapp.AddExperienceUseCase
	checkWorldEventsUC *worldeventapp.CheckWorldEventsUseCase
	checkAchievementsUC *achievementapp.CheckAchievementsUseCase // Для проверки достижений
	notificationService achievementapp.NotificationService        // Для отправки уведомлений о достижениях
	generateImageUC    *imageapp.ImageGenerationUseCase          // Для генерации изображений
	responseCache      *dmcache.DMResponseCache
	actionValidator    *ActionValidator
}

type EventRepository interface {
	Save(ctx context.Context, e *event.StoryEvent) error
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
	responseCache *dmcache.DMResponseCache,
	actionValidator *ActionValidator,
) *HandleActionUseCase {
	return &HandleActionUseCase{
		llm:                llm,
		sessionRepo:        sessionRepo,
		contextBuilder:     contextBuilder,
		eventRepo:          eventRepo,
		indexDocUC:         indexDocUC,
		combatRepo:         combatRepo,
		questRepo:          questRepo,
		inventoryRepo:      inventoryRepo,
		addExperienceUC:    addExperienceUC,
		checkWorldEventsUC: checkWorldEventsUC,
		checkAchievementsUC: checkAchievementsUC,
		notificationService: notificationService,
		generateImageUC:    generateImageUC,
		responseCache:      responseCache,
		actionValidator:    actionValidator,
	}
}

func (uc *HandleActionUseCase) Execute(
	ctx context.Context,
	chatID int64,
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

	// Проверяем наличие персонажа перед обработкой действий
	player := gs.GetFirstPlayer()
	if player == nil {
		return "Персонаж не создан. Используйте /createcharacter для создания персонажа.", nil
	}

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
		checkCtx, checkCancel := context.WithTimeout(ctx, 5*time.Second)
		defer checkCancel()
		checkReq := worldeventapp.CheckWorldEventsRequest{
			WorldID:          gs.WorldID,
			CurrentLocationID: nil, // TODO: добавить отслеживание текущей локации игрока
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

	// Сохраняем событие действия игрока ДО вызова LLM
	// Это гарантирует, что действие игрока будет сохранено даже если LLM вернет ошибку
	playerEvent := &event.StoryEvent{
		GameSessionID: gs.ID,
		AuthorType:    event.AuthorTypePlayer,
		Content:       playerMessage,
		CreatedAt:     time.Now(),
	}
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
		// Индексируем событие игрока в RAG с таймаутом
		doc := ragdomain.Document{
			ID:        uuid.New().String(),
			Source:    ragdomain.SourceEvent,
			SessionID: gs.ID,
			Text:      fmt.Sprintf("Игрок: %s", playerMessage),
			Timestamp: time.Now(),
		}
		if err := uc.indexDocUC.Execute(ragCtx, doc); err != nil {
			logger.Warn("Failed to index player event",
				logger.ErrorField(err),
				logger.Uint("session_id", gs.ID),
			)
		} else {
			logger.Debug("Player event indexed in RAG",
				logger.Uint("session_id", gs.ID),
				logger.String("doc_id", doc.ID),
			)
		}
	}

	// Проверяем кэш ответов DM перед генерацией
	var response string
	var fromCache bool
	if uc.responseCache != nil {
		cachedResponse, found := uc.responseCache.Get(ctx, gs.ID, gameContext, playerMessage)
		if found {
			response = cachedResponse
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

		// Формируем промпт для DM
		prompt := buildDMPrompt(gameContext, playerMessage)

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
		response, imageResults, err = uc.generateWithToolsLoop(llmCtx, prompt, toolRegistry, gs.ID)
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
		// Индексируем ответ DM в RAG с новым контекстом и таймаутом
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
		if err := uc.indexDocUC.Execute(indexCtx, doc); err != nil {
			logger.Warn("Failed to index DM event",
				logger.ErrorField(err),
				logger.Uint("session_id", gs.ID),
			)
		} else {
			logger.Debug("DM event indexed in RAG",
				logger.Uint("session_id", gs.ID),
				logger.String("doc_id", doc.ID),
			)
		}
	}

	// Анализируем ответ DM для автоматического определения боевых ситуаций, квестов и опыта
	// Получаем сообщение о начале боя, если оно было сгенерировано
	combatStartMessage := uc.analyzeDMResponse(ctx, gs, response)

	// Добавляем сообщение о порядке ходов к ответу DM, если бой начался
	if combatStartMessage != "" {
		response = fmt.Sprintf("%s\n\n%s", response, combatStartMessage)
	}

	return response, nil
}

// analyzeDMResponse анализирует ответ DM и выполняет автоматические действия
// Возвращает сообщение о начале боя (порядок ходов), если бой начался
func (uc *HandleActionUseCase) analyzeDMResponse(
	ctx context.Context,
	gs *session.GameSession,
	dmResponse string,
) string {
	// Получаем игрока (используем первого игрока для обратной совместимости)
	player := gs.GetFirstPlayer()
	if player == nil {
		// Нет игрока, нечего анализировать
		return ""
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
		player.ID, // Передаем playerID для проверки достижений
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

	// Анализируем ответ DM
	analysis, err := analyzer.Execute(ctx, dmResponse)
	if err != nil {
		logger.Warn("Failed to analyze DM response",
			logger.ErrorField(err),
			logger.Uint("session_id", gs.ID),
		)
		return ""
	}

	// Сохраняем сообщение о начале боя для возврата
	combatStartMessage := analysis.CombatStartMessage

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

	// Возвращаем сообщение о начале боя
	return combatStartMessage
}

func buildDMPrompt(gameContext, playerMessage string) string {
	// Проверяем, есть ли в контексте информация о активном бое
	hasActiveCombat := strings.Contains(gameContext, "--- Текущий бой ---") || 
		strings.Contains(gameContext, "⚔️ КРИТИЧЕСКИ ВАЖНО: Статус боя в ответах")
	
	combatInstruction := ""
	if hasActiveCombat {
		combatInstruction = "\n\n⚔️ ВАЖНО: Если в контексте указано, что идет активный бой, ВСЕГДА упоминай статус боя в НАЧАЛЕ ответа с префиксом '⚔️ [В БОЮ]' или '⚔️ Бой продолжается.'. Это критично для понимания игроком боевой ситуации."
	}
	
	return fmt.Sprintf(`Ты — Dungeon Master в игре Dungeons & Dragons.

Контекст текущей игры:
%s

Игрок написал: "%s"%s

Ответь как DM, описывая что происходит в игре. Будь креативным и вовлекающим. 
Отвечай на русском языке. Держи ответ в пределах 2-3 предложений, если не требуется больше деталей.`, gameContext, playerMessage, combatInstruction)
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
		registry.Register(dm_tools.NewRequestAbilityCheckTool(uc.sessionRepo, gs.ChatID))
		registry.Register(dm_tools.NewRequestSavingThrowTool(uc.sessionRepo, gs.ChatID))
		registry.Register(dm_tools.NewEvaluateCheckTool(uc.sessionRepo, gs.ChatID))
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
		registry.Register(dm_tools.NewGenerateImageTool(imageService, gs.ChatID, userID))
	}
	
	return registry
}

// generateWithToolsLoop реализует multi-step loop для работы с tools
// Возвращает финальный ответ DM с явно включенными результатами combat tools
func (uc *HandleActionUseCase) generateWithToolsLoop(ctx context.Context, prompt string, toolRegistry *dm_tools.ToolRegistry, sessionID uint) (string, []map[string]interface{}, error) {
	const maxIterations = 3 // Максимальное количество итераций для предотвращения бесконечного цикла
	
	allTools := toolRegistry.GetAll()
	currentPrompt := prompt
	
	// Собираем результаты combat tools для явного отображения в чате
	var combatResults []string
	// Собираем информацию о сгенерированных изображениях
	var imageResults []map[string]interface{}
	
	for iteration := 0; iteration < maxIterations; iteration++ {
		// Генерируем ответ с инструментами
		llmResponse, err := uc.llm.GenerateWithTools(ctx, currentPrompt, allTools)
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
			
			// Добавляем результаты combat tools в начало ответа, если они есть
			if len(combatResults) > 0 {
				combatSection := strings.Join(combatResults, "\n\n")
				return fmt.Sprintf("%s\n\n%s", combatSection, finalResponse), imageResults, nil
			}
			
			return finalResponse, imageResults, nil
		}
		
		// Выполняем вызовы инструментов
		logger.Info("DM requested tool calls",
			logger.Uint("session_id", sessionID),
			logger.Int("iteration", iteration+1),
			logger.Int("tool_calls_count", len(llmResponse.ToolCalls)),
		)
		
		// Логируем каждый вызов инструмента
		for i, call := range llmResponse.ToolCalls {
			argsJSON, _ := json.Marshal(call.Arguments)
			logger.Info("DM tool call requested",
				logger.Uint("session_id", sessionID),
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
				logger.Uint("session_id", sessionID),
				logger.ErrorField(err),
				logger.Int("iteration", iteration+1),
			)
			// Продолжаем без результатов инструментов - очищаем теги tool_call
			cleanedResponse := llmResponse.Content
			if strings.Contains(cleanedResponse, "<tool_call") {
				cleanedResponse = dm_tools.CleanToolCallTags(cleanedResponse)
			}
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
			logger.Uint("session_id", sessionID),
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
	
	// Добавляем результаты combat tools в начало ответа, если они есть
	if len(combatResults) > 0 {
		combatSection := strings.Join(combatResults, "\n\n")
		return fmt.Sprintf("%s\n\n%s", combatSection, cleanedResponse), imageResults, nil
	}
	
	return cleanedResponse, imageResults, nil
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

