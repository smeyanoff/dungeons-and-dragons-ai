package context

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"dungeons-and-dragons-ai/internal/game/domain/combat"
	"dungeons-and-dragons-ai/internal/game/domain/event"
	"dungeons-and-dragons-ai/internal/game/domain/inventory"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/game/domain/world"
	"dungeons-and-dragons-ai/internal/metrics"
	"dungeons-and-dragons-ai/internal/rag/application"
	"dungeons-and-dragons-ai/pkg/logger"
)

type RAGContextBuilder struct {
	simpleBuilder    *SimpleContextBuilder
	retrieveUC       *application.RetrieveContext
	eventRepo        EventRepository
	inventoryRepo    InventoryRepository
	combatRepo       CombatRepository
	worldEventRepo   WorldEventRepository
	campaignFactRepo CampaignFactRepository
	config           ragContextConfig
	ragAttempts      int64
	ragFallbacks     int64
}

type EventRepository interface {
	GetBySessionID(ctx context.Context, sessionID uint, limit int) ([]event.StoryEvent, error)
	// GetRecentByLocation возвращает последние события сессии в конкретной локации ("память этой локации").
	GetRecentByLocation(ctx context.Context, sessionID uint, locationID uint, limit int) ([]event.StoryEvent, error)
}

type InventoryRepository interface {
	GetByCharacterID(ctx context.Context, characterID uint) (*inventory.Inventory, error)
}

type CombatRepository interface {
	GetActiveBySessionID(ctx context.Context, sessionID uint) (*combat.Combat, error)
}

type WorldEventRepository interface {
	GetActiveByWorldID(ctx context.Context, worldID uint) ([]world.WorldEvent, error)
}

// CampaignFactRepository — источник курируемых устойчивых фактов кампании ("память всей игры").
type CampaignFactRepository interface {
	GetByWorldID(ctx context.Context, worldID uint, limit int) ([]world.CampaignFact, error)
}

func NewRAGContextBuilder(
	simpleBuilder *SimpleContextBuilder,
	retrieveUC *application.RetrieveContext,
	eventRepo EventRepository,
	inventoryRepo InventoryRepository,
	combatRepo CombatRepository,
) *RAGContextBuilder {
	return &RAGContextBuilder{
		simpleBuilder: simpleBuilder,
		retrieveUC:    retrieveUC,
		eventRepo:     eventRepo,
		inventoryRepo: inventoryRepo,
		combatRepo:    combatRepo,
		config:        loadRAGContextConfig(),
	}
}

// SetWorldEventRepository устанавливает репозиторий мировых событий
func (b *RAGContextBuilder) SetWorldEventRepository(repo WorldEventRepository) {
	b.worldEventRepo = repo
}

// SetCampaignFactRepository устанавливает репозиторий ключевых фактов кампании
func (b *RAGContextBuilder) SetCampaignFactRepository(repo CampaignFactRepository) {
	b.campaignFactRepo = repo
}

func (b *RAGContextBuilder) BuildContext(
	ctx context.Context,
	gs *session.GameSession,
	playerMessage string,
) (string, error) {
	// Эти лимиты — по СИМВОЛАМ, не по токенам.
	maxRecentEvents := b.config.maxRecentEvents
	maxEventChars := b.config.maxEventChars
	maxRAGDocs := b.config.maxRAGDocs
	maxRAGDocChars := b.config.maxRAGDocChars
	maxRAGTotalChars := b.config.maxRAGTotalChars

	// Сначала получаем базовый контекст с таймаутом для БД операций
	dbCtx, dbCancel := context.WithTimeout(ctx, 10*time.Second)
	defer dbCancel()
	baseContext, err := b.simpleBuilder.BuildContext(dbCtx, gs, playerMessage)
	if err != nil {
		return "", err
	}

	// Формируем контекст с RAG
	var parts []string
	parts = append(parts, baseContext)

	// Добавляем информацию о количестве игроков в сессии
	playerCount := len(gs.Players)
	parts = append(parts, "\n--- Игроки в сессии ---")
	if playerCount == 1 {
		parts = append(parts, "Количество игроков: 1 (игрок один)")
	} else {
		parts = append(parts, fmt.Sprintf("Количество игроков: %d", playerCount))
	}

	// Добавляем информацию о персонаже ДО попытки RAG
	// Это гарантирует, что информация о персонаже будет в контексте даже при ошибке RAG
	var playerCharacterID uint
	if len(gs.Players) > 0 {
		player := gs.GetFirstPlayer()
		if player != nil {
			char := player.Character
			playerCharacterID = char.ID
			parts = append(parts, "\n--- Персонаж игрока ---")
			parts = append(parts, fmt.Sprintf("Имя: %s", char.Name))
			parts = append(parts, fmt.Sprintf("Раса: %s, Класс: %s", char.Race, char.Class))
			parts = append(parts, fmt.Sprintf("Уровень: %d, HP: %d/%d", char.Level, char.HP, char.MaxHP))
			parts = append(parts, fmt.Sprintf("Характеристики: STR %d, DEX %d, CON %d, INT %d, WIS %d, CHA %d",
				char.Stats.Strength, char.Stats.Dexterity, char.Stats.Constitution,
				char.Stats.Intelligence, char.Stats.Wisdom, char.Stats.Charisma))
		}
	}

	// Проверяем, является ли запрос запросом об инвентаре
	// Если да, добавляем информацию об инвентаре в контекст
	if b.isInventoryQuery(playerMessage) && playerCharacterID > 0 && b.inventoryRepo != nil {
		invCtx, invCancel := context.WithTimeout(ctx, 5*time.Second)
		defer invCancel()
		inv, err := b.inventoryRepo.GetByCharacterID(invCtx, playerCharacterID)
		if err != nil {
			logger.Warn("Failed to get inventory for context",
				logger.ErrorField(err),
				logger.Uint("character_id", playerCharacterID),
			)
		} else if inv != nil {
			parts = append(parts, "\n--- Инвентарь персонажа ---")
			if len(inv.Items) == 0 {
				parts = append(parts, "Инвентарь пуст.")
			} else {
				parts = append(parts, fmt.Sprintf("Общий вес: %.2f кг / %.2f кг", inv.GetTotalWeight(), inventory.MaxWeight))
				parts = append(parts, "Предметы:")
				for _, item := range inv.Items {
					equippedSuffix := ""
					if item.Equipped {
						equippedSuffix = " [экипировано]"
					}
					if item.Quantity > 1 {
						parts = append(parts, fmt.Sprintf("- %s (x%d) - %s (%.2f кг)%s", item.Name, item.Quantity, item.Description, item.Weight, equippedSuffix))
					} else {
						parts = append(parts, fmt.Sprintf("- %s - %s (%.2f кг)%s", item.Name, item.Description, item.Weight, equippedSuffix))
					}
				}
			}
			logger.Debug("Added inventory to context",
				logger.Uint("character_id", playerCharacterID),
				logger.Int("items_count", len(inv.Items)),
			)
		}
	}

	// Ключевые факты кампании — курируемая "память всей игры", не привязанная к текущей локации.
	if b.campaignFactRepo != nil && gs.World.ID > 0 {
		factsCtx, factsCancel := context.WithTimeout(ctx, 5*time.Second)
		facts, err := b.campaignFactRepo.GetByWorldID(factsCtx, gs.World.ID, 15)
		factsCancel()
		if err != nil {
			logger.Warn("Failed to get campaign facts for context",
				logger.ErrorField(err),
				logger.Uint("world_id", gs.World.ID),
			)
		} else if len(facts) > 0 {
			parts = append(parts, "\n--- Ключевые факты кампании ---")
			for _, f := range facts {
				parts = append(parts, fmt.Sprintf("[%s] %s", f.Category, f.Text))
			}
			logger.Debug("Added campaign facts to context",
				logger.Uint("world_id", gs.World.ID),
				logger.Int("facts_count", len(facts)),
			)
		}
	}

	// Тонкая нить непрерывности: несколько последних сообщений без фильтра по локации,
	// чтобы не терять разговорный поток сразу после перехода в новую локацию.
	if b.eventRepo != nil {
		const continuityLimit = 4
		recentEvents, err := b.eventRepo.GetBySessionID(ctx, gs.ID, continuityLimit)
		if err != nil {
			logger.Warn("Failed to get recent events for context",
				logger.ErrorField(err),
				logger.Uint("session_id", gs.ID),
			)
		} else if len(recentEvents) > 0 {
			parts = append(parts, "\n--- Последние сообщения ---")
			parts = append(parts, eventLinesTiered(recentEvents, maxEventChars, b.config.minEventChars)...)
			logger.Debug("Added recent events to context",
				logger.Uint("session_id", gs.ID),
				logger.Int("events_count", len(recentEvents)),
			)
		}
	}

	// Память этой локации: события сессии, произошедшие именно здесь.
	// Отдельно от общей нити непрерывности выше — это то, что "имеет значение конкретно
	// в этой локации" и не должно смешиваться с событиями из других мест.
	if b.eventRepo != nil && gs.CurrentLocationID != nil {
		locEvents, err := b.eventRepo.GetRecentByLocation(ctx, gs.ID, *gs.CurrentLocationID, maxRecentEvents)
		if err != nil {
			logger.Warn("Failed to get location events for context",
				logger.ErrorField(err),
				logger.Uint("session_id", gs.ID),
				logger.Uint("location_id", *gs.CurrentLocationID),
			)
		} else if len(locEvents) > 0 {
			parts = append(parts, "\n--- Память этой локации ---")
			parts = append(parts, eventLinesTiered(locEvents, maxEventChars, b.config.minEventChars)...)
			logger.Debug("Added location events to context",
				logger.Uint("session_id", gs.ID),
				logger.Uint("location_id", *gs.CurrentLocationID),
				logger.Int("events_count", len(locEvents)),
			)
		}
	}

	// Добавляем информацию о текущем ходе боя, если есть активный бой
	if b.combatRepo != nil {
		combatCtx, combatCancel := context.WithTimeout(ctx, 5*time.Second)
		defer combatCancel()
		activeCombat, err := b.combatRepo.GetActiveBySessionID(combatCtx, gs.ID)
		if err != nil {
			logger.Warn("Failed to get active combat for context",
				logger.ErrorField(err),
				logger.Uint("session_id", gs.ID),
			)
		} else if activeCombat != nil && activeCombat.IsActive() {
			parts = append(parts, "\n--- Текущий бой ---")
			currentTurnMessage := activeCombat.GetCurrentTurnMessage()
			if currentTurnMessage != "" {
				parts = append(parts, currentTurnMessage)
			}

			// Подсчитываем количество игроков и врагов в бою
			playersInCombat := 0
			enemiesInCombat := 0
			for _, p := range activeCombat.Participants {
				if p.IsAlive() {
					if p.IsPlayer {
						playersInCombat++
					} else {
						enemiesInCombat++
					}
				}
			}
			parts = append(parts, fmt.Sprintf("Игроков в бою: %d, Врагов: %d", playersInCombat, enemiesInCombat))

			// Если игрок один в бою, явно указываем это
			if playerCount == 1 && playersInCombat == 1 {
				parts = append(parts, "⚠️ ВАЖНО: Игрок один в бою. Нет союзников, товарищей или других NPC.")
			}

			// Добавляем информацию об участниках боя и их HP
			parts = append(parts, "Участники боя:")
			for i, participant := range activeCombat.Participants {
				if participant.IsAlive() {
					name := participant.GetName()
					hp := participant.GetHP()
					maxHP := participant.GetMaxHP()
					initiative := participant.Initiative
					var role string
					if participant.IsPlayer {
						role = "Игрок"
					} else {
						role = "Враг"
					}
					parts = append(parts, fmt.Sprintf("%d. %s (%s) - HP: %d/%d, Инициатива: %d", i+1, name, role, hp, maxHP, initiative))
				}
			}

			// КРИТИЧЕСКИ ВАЖНО: Инструкция для DM о невыдумывании участников
			parts = append(parts, "")
			parts = append(parts, "⚠️ КРИТИЧЕСКИ ВАЖНО: Используй ТОЛЬКО реальных участников боя из списка выше.")
			parts = append(parts, "НЕ выдумывай союзников, товарищей или NPC, которых нет в списке участников боя.")
			parts = append(parts, "Если игрок один в бою, НЕ упоминай 'товарищей', 'союзников' или других NPC.")
			parts = append(parts, "Все участники боя перечислены выше - используй ТОЛЬКО их.")
			// КРИТИЧНО: Индикатор статуса боя в ответах
			parts = append(parts, "\n⚔️ КРИТИЧЕСКИ ВАЖНО: Статус боя в ответах")
			parts = append(parts, "ВСЕГДА упоминай статус боя в НАЧАЛЕ своего ответа, если идет активный бой.")
			parts = append(parts, "Формат: '⚔️ [В БОЮ] ...' или '⚔️ Бой продолжается. ...' или '⚔️ Ход боя: ...'")
			parts = append(parts, "Пример: '⚔️ [В БОЮ] Твой ход. Ты атакуешь гоблина...'")
			parts = append(parts, "Это необходимо, чтобы игрок всегда понимал, что находится в бою.")
			parts = append(parts, "")
			parts = append(parts, "Инструкции по использованию combat tools:")
			parts = append(parts, "1. Если игрок описывает атакующее действие (например, 'атакую', 'бью мечом', 'броситься вперед'), ОБЯЗАТЕЛЬНО используй инструмент 'perform_combat_attack' для обработки атаки игрока. Этот инструмент автоматически выполнит бросок кубиков, проверит попадание и применит урон.")
			parts = append(parts, "2. Когда описываешь атаку врага по игроку, ОБЯЗАТЕЛЬНО используй инструмент 'perform_enemy_attack' (предпочтительно) или 'apply_damage' с параметрами target_type='player', target_name='player', damage_amount=<количество урона>. Инструмент 'perform_enemy_attack' автоматически выполнит бросок кубиков для атаки врага, проверит попадание и применит урон.")
			parts = append(parts, "3. После атаки врага система автоматически переходит к следующему ходу. Если следующий ход игрока, ОБЯЗАТЕЛЬНО явно сообщи об этом игроку, используя информацию из результата инструмента 'perform_enemy_attack' (поле 'next_turn'). Пример: '🎯 Теперь твой ход, {имя игрока}! Что ты делаешь?'")
			parts = append(parts, "4. Если игрок описывает использование заклинания (например, 'кастую огненный шар', 'использую лечение ран', 'бросаю магическую стрелу'), ОБЯЗАТЕЛЬНО используй инструмент 'use_spell' для проверки изучения заклинания и его использования. НИКОГДА не описывай использование заклинания без вызова инструмента 'use_spell' - это приведет к ошибке, так как система не знает, изучено ли заклинание персонажем.")
			parts = append(parts, "5. Используй инструмент 'get_battlefield_status' для визуализации поля боя с информацией о всех участниках (HP, AC, инициатива, текущий ход).")
			parts = append(parts, "6. После получения результата от инструмента, опиши результат атаки/заклинания в своем ответе игроку, включив информацию об уроне и текущем HP.")
			parts = append(parts, "")
			parts = append(parts, "Инструкции по использованию validation tools:")
			parts = append(parts, "1. Если игрок пытается использовать предмет (выпить зелье, надеть доспех, применить предмет, открыть проход ключом/квестовым предметом), ОБЯЗАТЕЛЬНО используй инструмент 'validate_item_usage' для проверки наличия предмета в инвентаре перед описанием использования. Без предмета в инвентаре не выдумывай, что он у игрока появился, и не открывай путь.")
			parts = append(parts, "2. Если игрок пытается выполнить действие, требующее физических характеристик (поднять тяжелый предмет, перепрыгнуть препятствие, проявить ловкость), используй инструмент 'check_stat_requirements' для проверки достаточности характеристик.")
			parts = append(parts, "3. На основе результатов validation tools решай, может ли игрок выполнить действие, и описывай соответствующий результат.")
			parts = append(parts, "")
			parts = append(parts, "📋 Проверки/броски игрока (важно):")
			parts = append(parts, "• НЕ проси игрока бросать кубики и НЕ предлагай /roll — система сама запросит бросок, если нужен")
			logger.Debug("Added combat information to context",
				logger.Uint("session_id", gs.ID),
				logger.Int("participants_count", len(activeCombat.Participants)),
			)
		}
	}

	// Используем RAG для поиска релевантных событий с таймаутом
	// Используем сообщение игрока как запрос для поиска
	if b.retrieveUC == nil {
		logger.Warn("RAG retrieve use case is nil, skipping RAG context",
			logger.Uint("session_id", gs.ID),
		)
		return strings.Join(parts, "\n"), nil
	}
	logger.Debug("Retrieving RAG context",
		logger.Uint("session_id", gs.ID),
		logger.String("query", playerMessage),
	)
	ragCtx, ragCancel := context.WithTimeout(ctx, 15*time.Second)
	defer ragCancel()
	atomic.AddInt64(&b.ragAttempts, 1)
	// Ограничиваем семантический поиск текущей локацией, если она известна — это ловит более
	// старые визиты в то же место, которые вышли за пределы окна "Память этой локации" выше.
	ragDocs, err := b.retrieveUC.Execute(ragCtx, gs.ID, gs.CurrentLocationID, playerMessage, maxRAGDocs)
	if err != nil {
		// Если RAG не работает, возвращаем базовый контекст с информацией о персонаже
		fallbacks := atomic.AddInt64(&b.ragFallbacks, 1)
		attempts := atomic.LoadInt64(&b.ragAttempts)
		fallbackRatio := 0.0
		if attempts > 0 {
			fallbackRatio = float64(fallbacks) / float64(attempts)
		}
		metrics.IncrementRAGFailure()
		logger.Warn("Failed to retrieve RAG context",
			logger.ErrorField(err),
			logger.Uint("session_id", gs.ID),
			logger.String("query", playerMessage),
			logger.Int64("rag_attempts", attempts),
			logger.Int64("rag_fallbacks", fallbacks),
			logger.Float64("rag_fallback_ratio", fallbackRatio),
		)
		parts = append(parts, "\n--- RAG fallback ---")
		parts = append(parts, "⚠️ История из RAG недоступна.")
		parts = append(parts, "Сделай краткое резюме текущей сцены (2-3 предложения) на основе последних сообщений выше.")
		parts = append(parts, "Задай один уточняющий вопрос, чтобы продолжить. Не упоминай сбой игроку.")
		return strings.Join(parts, "\n"), nil
	}

	logger.Debug("RAG search completed",
		logger.Uint("session_id", gs.ID),
		logger.Int("found_docs", len(ragDocs)),
		logger.String("query", playerMessage),
	)

	if len(ragDocs) > 0 {
		parts = append(parts, "\n--- Память этой локации (похожие моменты, найдено через поиск) ---")
		totalChars := 0
		addedCount := 0
		for i, doc := range ragDocs {
			if i >= maxRAGDocs || totalChars >= maxRAGTotalChars {
				break
			}
			docText := strings.TrimSpace(doc.Text)
			if docText == "" {
				logger.Debug("Skipping empty RAG document",
					logger.Uint("session_id", gs.ID),
					logger.Int("doc_index", i),
				)
				continue
			}
			docText = truncateText(docText, maxRAGDocChars)
			if totalChars+len(docText) > maxRAGTotalChars {
				docText = truncateText(docText, maxRAGTotalChars-totalChars)
			}
			if docText == "" {
				continue
			}
			totalChars += len(docText)
			parts = append(parts, fmt.Sprintf("[%d] %s", addedCount+1, docText))
			addedCount++
		}
		if addedCount == 0 {
			metrics.IncrementRAGEmptyResult()
			logger.Warn("RAG found documents but none were added to context (empty text after trim)",
				logger.Uint("session_id", gs.ID),
				logger.Int("docs_found", len(ragDocs)),
			)
		}
		logger.Debug("Added RAG documents to context",
			logger.Uint("session_id", gs.ID),
			logger.Int("docs_found", len(ragDocs)),
			logger.Int("docs_added", addedCount),
		)
	} else {
		logger.Debug("No RAG documents found",
			logger.Uint("session_id", gs.ID),
		)
	}

	// Мини-ивенты теперь генерируются только в ответах DM, а не добавляются в контекст RAG
	// Это предотвращает их повторение и спам

	return strings.Join(parts, "\n"), nil
}

// generateMiniEvent генерирует случайный мини-ивент для атмосферы
func generateMiniEvent() string {
	// Разные мини-ивенты для разных типов атмосферы
	forestEvents := []string{
		"🌲 Легкий ветерок колышет листву деревьев, создавая приятную прохладу.",
		"🐦 Вдали слышны крики ворон, кружащих над верхушками деревьев.",
		"🌿 Тропа усыпана опавшими листьями, которые тихо шуршат под ногами.",
		"🐿️ В ветвях деревьев мелькают белки, занятые своими повседневными делами.",
		"🦉 Из чащи леса доносится таинственный уханье совы.",
		"🌸 Воздух наполнен ароматом цветущих трав и свежей зелени.",
		"🐛 По стволу дерева медленно ползет яркая гусеница.",
	}

	caveEvents := []string{
		"💧 С потолка пещеры медленно капает вода, отзываясь эхом.",
		"🦇 В темноте слышны тихие шорохи летучих мышей.",
		"💎 В стенах пещеры мерцают кристаллы, отражая слабый свет.",
		"🌑 Воздух здесь прохладный и влажный, с запахом камня и земли.",
		"🕯️ Где-то в глубине пещеры мерцает слабый таинственный свет.",
	}

	castleEvents := []string{
		"🏰 Старые каменные стены хранят память о былых временах.",
		"🕸️ В углах паутины тихо колышутся от сквозняка.",
		"📜 На стенах видны потускневшие гобелены с изображением древних битв.",
		"🔥 В камине потрескивают дрова, создавая уютную атмосферу.",
		"👻 Иногда кажется, что за спиной кто-то прошелестел.",
	}

	townEvents := []string{
		"🏘️ Издалека доносятся звуки городской жизни и разговоры прохожих.",
		"🛒 Рядом с вами проходит торговец с тележкой, предлагая свои товары.",
		"🎶 Где-то неподалеку играет музыкант, наполняя воздух мелодией.",
		"🏪 Витрины магазинов манят разнообразием товаров.",
		"👥 Группа горожан о чем-то оживленно беседует на углу улицы.",
	}

	// Общие атмосферные мини-ивенты
	generalEvents := []string{
		"☀️ Солнечные лучи пробиваются сквозь густую листву, создавая игру света и тени.",
		"💨 Порыв ветра доносит едва уловимый запах свежескошенной травы и земли.",
		"🌸 Рядом с тропой распустились яркие полевые цветы, привлекая бабочек.",
		"🌧️ Где-то вдали прогремел отдаленный раскат грома, предвещая дождь.",
		"🦋 Над поляной порхают разноцветные бабочки, создавая атмосферу покоя.",
		"🌙 Небо затянуто легкими облаками, смягчающими яркий свет солнца.",
		"🌤️ Легкая дымка стелется над землей, создавая ощущение тайны.",
	}

	// Выбираем подходящий набор мини-ивентов (пока что случайный, в будущем можно сделать зависимым от локации)
	eventSets := [][]string{forestEvents, caveEvents, castleEvents, townEvents, generalEvents}
	selectedSet := eventSets[rand.Intn(len(eventSets))]

	if len(selectedSet) == 0 {
		return ""
	}

	return selectedSet[rand.Intn(len(selectedSet))]
}

// eventLinesTiered форматирует список событий (от новых к старым) с убывающим по возрасту
// бюджетом символов: самое свежее сообщение получает maxChars целиком (важны детали для
// непрерывности разговора), а каждое следующее, более старое — вдвое меньше, до пола minChars
// (там уже достаточно компактной фактической выжимки, а не дословного текста). Так средний
// расход токенов на пачку событий заметно ниже, чем при пересылке всех сообщений полным текстом.
// events ожидаются в хронологическом порядке (старые → новые — как их возвращают
// GetBySessionID/GetRecentByLocation). Полный бюджет символов достаётся самому свежему
// (последнему) событию, более старым — по убывающей, с полом minChars.
func eventLinesTiered(events []event.StoryEvent, maxChars, minChars int) []string {
	if minChars <= 0 || minChars > maxChars {
		minChars = maxChars
	}
	lines := make([]string, len(events))
	budget := maxChars
	for i := len(events) - 1; i >= 0; i-- {
		lines[i] = formatStoryEventLine(events[i], budget)
		if budget > minChars {
			budget /= 2
			if budget < minChars {
				budget = minChars
			}
		}
	}
	return lines
}

// formatStoryEventLine форматирует одно событие истории для показа в контексте DM.
func formatStoryEventLine(e event.StoryEvent, maxEventChars int) string {
	var authorPrefix string
	switch e.AuthorType {
	case event.AuthorTypePlayer:
		authorPrefix = "Игрок"
	case event.AuthorTypeDM:
		authorPrefix = "DM"
	case event.AuthorTypeNPC:
		authorPrefix = e.AuthorName
	default:
		authorPrefix = "?"
	}
	return fmt.Sprintf("%s: %s", authorPrefix, truncateText(e.Content, maxEventChars))
}

func truncateText(text string, maxLen int) string {
	trimmed := strings.TrimSpace(text)
	if maxLen <= 0 || len(trimmed) <= maxLen {
		return trimmed
	}
	return strings.TrimSpace(trimmed[:maxLen]) + "…"
}

// isInventoryQuery определяет, является ли сообщение игрока запросом об инвентаре
func (b *RAGContextBuilder) isInventoryQuery(message string) bool {
	message = strings.ToLower(message)
	inventoryKeywords := []string{
		"инвентарь",
		"что у меня",
		"что у меня есть",
		"предметы",
		"что в инвентаре",
		"что в сумке",
		"что у меня в сумке",
		"что у меня в карманах",
		"что я ношу",
		"что у меня с собой",
		"мой инвентарь",
		"покажи инвентарь",
		"покажи что у меня",
		"что у меня в инвентаре",
	}
	for _, keyword := range inventoryKeywords {
		if strings.Contains(message, keyword) {
			return true
		}
	}
	return false
}

type ragContextConfig struct {
	maxRecentEvents  int
	maxEventChars    int
	minEventChars    int
	maxRAGDocs       int
	maxRAGDocChars   int
	maxRAGTotalChars int
}

func loadRAGContextConfig() ragContextConfig {
	return ragContextConfig{
		maxRecentEvents:  getEnvInt("RAG_MAX_RECENT_EVENTS", 8),
		maxEventChars:    getEnvInt("RAG_MAX_EVENT_CHARS", 800),
		// minEventChars — пол для тиерной обрезки в eventLinesTiered: чем старше сообщение
		// в списке, тем меньше символов ему выделяется (полный текст нужен только для самых
		// свежих реплик, старые важны как краткий факт, а не дословно). Это компромисс между
		// полной пересылкой сырых сообщений и извлечением фактов отдельным LLM-вызовом —
		// экономит токены без дополнительной LLM-суммаризации.
		minEventChars:    getEnvInt("RAG_MIN_EVENT_CHARS", 150),
		maxRAGDocs:       getEnvInt("RAG_MAX_DOCS", 6),
		maxRAGDocChars:   getEnvInt("RAG_MAX_DOC_CHARS", 1200),
		maxRAGTotalChars: getEnvInt("RAG_MAX_TOTAL_CHARS", 5000),
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
