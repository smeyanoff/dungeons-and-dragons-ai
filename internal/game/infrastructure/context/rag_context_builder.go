package context

import (
	"context"
	"fmt"
	"strings"
	"time"

	"dungeons-and-dragons-ai/internal/game/domain/combat"
	"dungeons-and-dragons-ai/internal/game/domain/event"
	"dungeons-and-dragons-ai/internal/game/domain/inventory"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/rag/application"
	"dungeons-and-dragons-ai/pkg/logger"
)

type RAGContextBuilder struct {
	simpleBuilder *SimpleContextBuilder
	retrieveUC    *application.RetrieveContext
	eventRepo     EventRepository
	inventoryRepo InventoryRepository
	combatRepo    CombatRepository
}

type EventRepository interface {
	GetBySessionID(ctx context.Context, sessionID uint, limit int) ([]event.StoryEvent, error)
}

type InventoryRepository interface {
	GetByCharacterID(ctx context.Context, characterID uint) (*inventory.Inventory, error)
}

type CombatRepository interface {
	GetActiveBySessionID(ctx context.Context, sessionID uint) (*combat.Combat, error)
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
	}
}

func (b *RAGContextBuilder) BuildContext(
	ctx context.Context,
	gs *session.GameSession,
	playerMessage string,
) (string, error) {
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
					if item.Quantity > 1 {
						parts = append(parts, fmt.Sprintf("- %s (x%d) - %s (%.2f кг)", item.Name, item.Quantity, item.Description, item.Weight))
					} else {
						parts = append(parts, fmt.Sprintf("- %s - %s (%.2f кг)", item.Name, item.Description, item.Weight))
					}
				}
			}
			logger.Debug("Added inventory to context",
				logger.Uint("character_id", playerCharacterID),
				logger.Int("items_count", len(inv.Items)),
			)
		}
	}

	// Получаем последние сообщения из истории (последние 5-10 сообщений)
	// Это обеспечивает контекст недавних событий
	if b.eventRepo != nil {
		recentEvents, err := b.eventRepo.GetBySessionID(ctx, gs.ID, 10)
		if err != nil {
			logger.Warn("Failed to get recent events for context",
				logger.ErrorField(err),
				logger.Uint("session_id", gs.ID),
			)
		} else if len(recentEvents) > 0 {
			parts = append(parts, "\n--- Последние сообщения ---")
			// Берем последние 5-10 сообщений для контекста
			startIdx := 0
			if len(recentEvents) > 5 {
				startIdx = len(recentEvents) - 5
			}
			for i := startIdx; i < len(recentEvents); i++ {
				e := recentEvents[i]
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
				parts = append(parts, fmt.Sprintf("%s: %s", authorPrefix, e.Content))
			}
			logger.Debug("Added recent events to context",
				logger.Uint("session_id", gs.ID),
				logger.Int("events_count", len(recentEvents)-startIdx),
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
			parts = append(parts, "3. Используй инструмент 'get_battlefield_status' для визуализации поля боя с информацией о всех участниках (HP, AC, инициатива, текущий ход).")
			parts = append(parts, "4. После получения результата от инструмента, опиши результат атаки в своем ответе игроку, включив информацию об уроне и текущем HP.")
			parts = append(parts, "")
			parts = append(parts, "Инструкции по использованию validation tools:")
			parts = append(parts, "1. Если игрок пытается использовать предмет (выпить зелье, надеть доспех, применить предмет), ОБЯЗАТЕЛЬНО используй инструмент 'validate_item_usage' для проверки наличия предмета в инвентаре перед описанием использования.")
			parts = append(parts, "2. Если игрок пытается выполнить действие, требующее физических характеристик (поднять тяжелый предмет, перепрыгнуть препятствие, проявить ловкость), используй инструмент 'check_stat_requirements' для проверки достаточности характеристик.")
			parts = append(parts, "3. На основе результатов validation tools решай, может ли игрок выполнить действие, и описывай соответствующий результат.")
			logger.Debug("Added combat information to context",
				logger.Uint("session_id", gs.ID),
				logger.Int("participants_count", len(activeCombat.Participants)),
			)
		}
	}

	// Используем RAG для поиска релевантных событий с таймаутом
	// Используем сообщение игрока как запрос для поиска
	logger.Debug("Retrieving RAG context",
		logger.Uint("session_id", gs.ID),
		logger.String("query", playerMessage),
	)
	ragCtx, ragCancel := context.WithTimeout(ctx, 15*time.Second)
	defer ragCancel()
	ragDocs, err := b.retrieveUC.Execute(ragCtx, gs.ID, playerMessage, 5)
	if err != nil {
		// Если RAG не работает, возвращаем базовый контекст с информацией о персонаже
		logger.Warn("Failed to retrieve RAG context",
			logger.ErrorField(err),
			logger.Uint("session_id", gs.ID),
		)
		return strings.Join(parts, "\n"), nil
	}

	if len(ragDocs) > 0 {
		parts = append(parts, "\n--- Релевантная история игры (найдено через поиск) ---")
		for i, doc := range ragDocs {
			parts = append(parts, fmt.Sprintf("[%d] %s", i+1, doc.Text))
		}
		logger.Debug("Added RAG documents to context",
			logger.Uint("session_id", gs.ID),
			logger.Int("docs_count", len(ragDocs)),
		)
	} else {
		logger.Debug("No RAG documents found",
			logger.Uint("session_id", gs.ID),
		)
	}

	return strings.Join(parts, "\n"), nil
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
