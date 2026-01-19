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
			parts = append(parts, "3. После атаки врага система автоматически переходит к следующему ходу. Если следующий ход игрока, ОБЯЗАТЕЛЬНО явно сообщи об этом игроку, используя информацию из результата инструмента 'perform_enemy_attack' (поле 'next_turn'). Пример: '🎯 Теперь твой ход, {имя игрока}! Что ты делаешь?'")
			parts = append(parts, "4. Если игрок описывает использование заклинания (например, 'кастую огненный шар', 'использую лечение ран', 'бросаю магическую стрелу'), ОБЯЗАТЕЛЬНО используй инструмент 'use_spell' для проверки изучения заклинания и его использования. НИКОГДА не описывай использование заклинания без вызова инструмента 'use_spell' - это приведет к ошибке, так как система не знает, изучено ли заклинание персонажем.")
			parts = append(parts, "5. Используй инструмент 'get_battlefield_status' для визуализации поля боя с информацией о всех участниках (HP, AC, инициатива, текущий ход).")
			parts = append(parts, "6. После получения результата от инструмента, опиши результат атаки/заклинания в своем ответе игроку, включив информацию об уроне и текущем HP.")
			parts = append(parts, "")
			parts = append(parts, "Инструкции по использованию validation tools:")
			parts = append(parts, "1. Если игрок пытается использовать предмет (выпить зелье, надеть доспех, применить предмет), ОБЯЗАТЕЛЬНО используй инструмент 'validate_item_usage' для проверки наличия предмета в инвентаре перед описанием использования.")
			parts = append(parts, "2. Если игрок пытается выполнить действие, требующее физических характеристик (поднять тяжелый предмет, перепрыгнуть препятствие, проявить ловкость), используй инструмент 'check_stat_requirements' для проверки достаточности характеристик.")
			parts = append(parts, "3. На основе результатов validation tools решай, может ли игрок выполнить действие, и описывай соответствующий результат.")
			parts = append(parts, "")
			parts = append(parts, "Инструкции по использованию ability check tools (ПРОВЕРКИ НАВЫКОВ):")
			parts = append(parts, "⚠️ КРИТИЧЕСКИ ВАЖНО: Когда игрок выполняет действие, требующее проверки навыка, ОБЯЗАТЕЛЬНО используй инструмент 'request_ability_check' для запроса проверки у игрока.")
			parts = append(parts, "")
			parts = append(parts, "Когда использовать 'request_ability_check':")
			parts = append(parts, "- Поиск скрытых механизмов, ловушек, секретных дверей → 'intelligence' или 'wisdom' (DC 12-15)")
			parts = append(parts, "- Проверка восприятия (заметить что-то, услышать звук, увидеть детали) → 'wisdom' (DC 10-15)")
			parts = append(parts, "- Проверка ловкости (украсть, открыть замок, акробатика) → 'dexterity' (DC 12-18)")
			parts = append(parts, "- Проверка силы (сломать дверь, поднять тяжелый предмет, толкнуть) → 'strength' (DC 12-18)")
			parts = append(parts, "- Проверка интеллекта (вспомнить информацию, понять механизм, расшифровать) → 'intelligence' (DC 12-18)")
			parts = append(parts, "- Проверка харизмы (убедить, обмануть, запугать NPC) → 'charisma' (DC 12-18)")
			parts = append(parts, "")
			parts = append(parts, "Как использовать:")
			parts = append(parts, "1. Определи, какая характеристика нужна для действия игрока")
			parts = append(parts, "2. Определи DC (Difficulty Class) в зависимости от сложности:")
			parts = append(parts, "   - Легко: DC 10-12")
			parts = append(parts, "   - Средне: DC 13-15")
			parts = append(parts, "   - Сложно: DC 16-18")
			parts = append(parts, "   - Очень сложно: DC 19-20")
			parts = append(parts, "3. ⚠️ ОБЯЗАТЕЛЬНО вызови 'request_ability_check' с параметрами ability И dc (dc ОБЯЗАТЕЛЕН!).")
			parts = append(parts, "   - ability: название характеристики ('strength', 'dexterity', 'constitution', 'intelligence', 'wisdom', 'charisma')")
			parts = append(parts, "   - dc: сложность проверки (ОБЯЗАТЕЛЬНО укажи DC: 10-легко, 13-средне, 16-сложно, 19-очень сложно)")
			parts = append(parts, "4. После получения результата от инструмента:")
			parts = append(parts, "   - Опиши ситуацию естественным языком")
			parts = append(parts, "   - ⚠️ ОБЯЗАТЕЛЬНО четко укажи команду: 'Используй команду /roll d20' (не просто 'брось кубик', а именно команду)")
			parts = append(parts, "   - Объясни что такое DC простыми словами")
			parts = append(parts, "   - Пример: 'Ты ощущаешь вибрации энергии портала. Сделай проверку Мудрости. Тебе нужно выбросить на кубике d20 число, которое вместе с твоим модификатором Мудрости будет не меньше 12 (DC 12). Используй команду /roll d20.'")
			parts = append(parts, "5. ⚠️ КРИТИЧЕСКИ ВАЖНО: После того, как попросил игрока бросить кубик, АВТОМАТИЧЕСКИ проверь историю игры.")
			parts = append(parts, "   - Результаты бросков автоматически сохраняются в историю с текстом 'Игрок бросил кубик: 🎲 Бросок d20: **<число>**'")
			parts = append(parts, "   - НЕ ЖДИ сообщения от игрока - сразу проверь историю и обработай результат")
			parts = append(parts, "6. Если в истории есть результат броска, ОБЯЗАТЕЛЬНО:")
			parts = append(parts, "   a) Извлеки число из результата броска (например, из '🎲 Бросок d20: **14**' извлеки 14)")
			parts = append(parts, "   b) Добавь модификатор из результата 'request_ability_check' (поле 'modifier') к числу броска")
			parts = append(parts, "   c) ОБЯЗАТЕЛЬНО вызови инструмент 'evaluate_check' с параметрами:")
			parts = append(parts, "      - ability: та же характеристика, что использовалась в 'request_ability_check'")
			parts = append(parts, "      - dc: та же DC, что использовалась в 'request_ability_check'")
			parts = append(parts, "      - roll_result: итоговый результат (бросок + модификатор)")
			parts = append(parts, "   d) ⚠️ ОБЯЗАТЕЛЬНО явно покажи игроку результат проверки естественным языком:")
			parts = append(parts, "      - ВСЕГДА начинай с явного указания успеха или провала: '✅ Успех!' или '❌ Провал!'")
			parts = append(parts, "      - Если success=true: опиши что произошло в результате успешной проверки (например, '✅ Успех! Ты заметил скрытую ловушку на сундуке и успешно её обезвредил.')")
			parts = append(parts, "      - Если success=false: опиши что произошло в результате провала (например, '❌ Провал! Ты не заметил ничего подозрительного на сундуке.')")
			parts = append(parts, "   e) Используй поле 'message' из результата 'evaluate_check' как основу, но опиши результат естественным языком, как настоящий DM")
			parts = append(parts, "   ⚠️ КРИТИЧЕСКИ ВАЖНО: ВСЕГДА явно показывай успех (✅ Успех!) или провал (❌ Провал!) в начале описания результата - игрок должен сразу понимать, прошла ли проверка")
			parts = append(parts, "")
			parts = append(parts, "⚠️ КРИТИЧЕСКИ ВАЖНО:")
			parts = append(parts, "- НЕ вызывай 'request_ability_check' БЕЗ указания DC - это приведет к ошибке")
			parts = append(parts, "- НЕ описывай результат проверки до броска игрока")
			parts = append(parts, "- НЕ описывай результат проверки без использования 'evaluate_check'")
			parts = append(parts, "- НЕ ЖДИ сообщения от игрока после того, как попросил бросить кубик - проверь историю автоматически")
			parts = append(parts, "- ВСЕГДА явно показывай игроку успех или провал проверки")
			logger.Debug("Added combat information to context",
				logger.Uint("session_id", gs.ID),
				logger.Int("participants_count", len(activeCombat.Participants)),
			)
		}
	}

	// Добавляем общие инструкции по использованию ability check tools (вне боя)
	parts = append(parts, "\n--- Инструкции по проверкам навыков (ABILITY CHECKS) ---")
	parts = append(parts, "⚠️ ОБЩИЕ ПРАВИЛА ФОРМАТА ОТВЕТОВ:")
	parts = append(parts, "- ВСЕГДА отвечай естественным языком, как настоящий Dungeon Master")
	parts = append(parts, "- НЕ показывай игроку технические сообщения типа 'Выполняется проверка...', 'Оценивается результат...', 'Бросок d20: /roll d20', 'результат броска извлечён из истории игры'")
	parts = append(parts, "- НЕ показывай промежуточные шаги обработки - просто описывай что происходит в игре")
	parts = append(parts, "- ⚠️ ВСЕГДА четко указывай команду: 'Используй команду /roll d20' (не просто 'брось кубик', а именно команду)")
	parts = append(parts, "- ⚠️ ВСЕГДА явно показывай успех (✅ Успех!) или провал (❌ Провал!) в начале описания результата проверки")
	parts = append(parts, "- Пример ПРАВИЛЬНОГО ответа при запросе проверки: 'Ты пытаешься вспомнить информацию о кристаллах. Сделай проверку Мудрости. Тебе нужно выбросить на кубике d20 число, которое вместе с твоим модификатором Мудрости будет не меньше 14 (это называется DC - сложность проверки). Используй команду /roll d20.'")
	parts = append(parts, "- Пример ПРАВИЛЬНОГО ответа при результате проверки: '✅ Успех! Ты вспоминаешь, что эти кристаллы используются для создания магических артефактов.' или '❌ Провал! Ты пытаешься вспомнить информацию о кристаллах, но ничего конкретного не приходит на ум.'")
	parts = append(parts, "- Пример НЕПРАВИЛЬНОГО ответа: 'Выполняется проверка Мудрости (WIS) (DC 14). Бросок d20: /roll d20. (результат броска извлечён из истории игры). Оценивается результат проверки характеристики. Опиши результат проверки игроку.'")
	parts = append(parts, "")
	parts = append(parts, "⚠️ КРИТИЧЕСКИ ВАЖНО - ГЛАВНОЕ ПРАВИЛО:")
	parts = append(parts, "- НЕ проси игрока бросать кубики для каждого действия - это делает игру неинтересной")
	parts = append(parts, "- Используй проверки ТОЛЬКО когда действие имеет неопределенный исход и важно для сюжета")
	parts = append(parts, "- Для простых действий (идти, говорить, брать предмет, подняться по ступеням) просто опиши результат БЕЗ запроса проверки")
	parts = append(parts, "- НЕ используй проверки для абстрактных понятий ('проверить удачу', 'проверить реакцию', 'проверить решимость')")
	parts = append(parts, "")
	parts = append(parts, "⚠️ КРИТИЧЕСКИ ВАЖНО: Когда игрок выполняет действие, требующее проверки навыка, ОБЯЗАТЕЛЬНО используй инструмент 'request_ability_check' для запроса проверки у игрока.")
	parts = append(parts, "")
	parts = append(parts, "⚠️ КРИТИЧЕСКИ ВАЖНО - ПРАВИЛО ОДНОЙ ПРОВЕРКИ:")
	parts = append(parts, "- В D&D проверка навыка для конкретной задачи выполняется ТОЛЬКО ОДИН РАЗ")
	parts = append(parts, "- Если проверка провалена, игрок НЕ МОЖЕТ повторить её для той же задачи")
	parts = append(parts, "- Если игрок хочет попробовать снова, он должен использовать ДРУГОЙ подход (другой навык, другой метод, дополнительная информация)")
	parts = append(parts, "- Примеры ОДНОЙ задачи (нельзя повторять):")
	parts = append(parts, "  * 'прочитать записи в сумке' и 'изучить оставшиеся записи' - это ОДНА задача")
	parts = append(parts, "  * 'разобрать текст на свитке' и 'попробовать снова разобрать текст' - это ОДНА задача")
	parts = append(parts, "- Примеры РАЗНЫХ задач (можно использовать разные навыки):")
	parts = append(parts, "  * 'прочитать записи' (Интеллект) и 'найти подсказки в записях' (Мудрость) - это РАЗНЫЕ задачи")
	parts = append(parts, "")
	parts = append(parts, "⚠️ КРИТИЧЕСКИ ВАЖНО - ПЕРЕД запросом проверки:")
	parts = append(parts, "- ВСЕГДА проверяй историю игры (раздел 'Последние сообщения') ПЕРЕД тем, как просить игрока бросить кубик")
	parts = append(parts, "- Если в истории уже есть результат броска кубика (текст 'Игрок бросил кубик: 🎲 Бросок d20: **<число>**'), НЕ проси игрока бросить снова")
	parts = append(parts, "- Вместо этого СРАЗУ обработай существующий результат: извлеки число, добавь модификатор, вызови 'evaluate_check', покажи результат игроку")
	parts = append(parts, "- ⚠️ НИКОГДА не проси игрока бросить кубик снова, если результат уже есть в истории - это создает бесконечный цикл")
	parts = append(parts, "")
	parts = append(parts, "⚠️ КРИТИЧЕСКИ ВАЖНО - Когда НЕ использовать проверки:")
	parts = append(parts, "- НЕ используй проверки для простых действий, которые всегда успешны:")
	parts = append(parts, "  * Подняться по ступеням, лестнице (если нет препятствий) - просто опиши что игрок поднялся")
	parts = append(parts, "  * Войти в здание, храм (если дверь открыта) - просто опиши что игрок вошел")
	parts = append(parts, "  * Пройти по дороге, тропинке - просто опиши что игрок идет")
	parts = append(parts, "  * Взять предмет, поднять предмет - просто опиши что игрок взял предмет")
	parts = append(parts, "  * Открыть обычную дверь (не запертую) - просто опиши что игрок открыл дверь")
	parts = append(parts, "  * Сесть, встать, лечь - просто опиши действие")
	parts = append(parts, "  * Говорить, кричать - просто опиши что игрок говорит")
	parts = append(parts, "- Если действие простое и не требует проверки, просто опиши результат естественным языком БЕЗ запроса проверки")
	parts = append(parts, "")
	parts = append(parts, "Примеры действий, требующих проверки навыка:")
	parts = append(parts, "- Поиск скрытых механизмов, ловушек, секретных дверей → 'intelligence' или 'wisdom'")
	parts = append(parts, "- Проверка восприятия (заметить что-то, услышать звук, увидеть детали) → 'wisdom'")
	parts = append(parts, "- Проверка ловкости (украсть, открыть замок, акробатика) → 'dexterity'")
	parts = append(parts, "- Проверка силы (сломать дверь, поднять тяжелый предмет, толкнуть) → 'strength'")
	parts = append(parts, "- Проверка интеллекта (вспомнить информацию, понять механизм, расшифровать) → 'intelligence'")
	parts = append(parts, "- Проверка харизмы (убедить, обмануть, запугать NPC) → 'charisma'")
	parts = append(parts, "")
	parts = append(parts, "Процесс использования:")
	parts = append(parts, "0. ⚠️ КРИТИЧЕСКИ ВАЖНО - ПЕРЕД запросом проверки:")
	parts = append(parts, "   - ВСЕГДА сначала проверь историю игры (раздел 'Последние сообщения' или 'Релевантная история игры')")
	parts = append(parts, "   - Если в истории есть результат броска кубика (текст 'Игрок бросил кубик: 🎲 Бросок d20: **<число>**'), НЕ запрашивай новую проверку через 'request_ability_check'")
	parts = append(parts, "   - Если в истории есть результат проверки (успех или провал) для похожей задачи, НЕ запрашивай новую проверку - это будет повторением")
	parts = append(parts, "   - Примеры ОДНОЙ задачи (нельзя повторять проверку):")
	parts = append(parts, "     * 'прочитать записи в сумке' и 'изучить оставшиеся записи' - это ОДНА задача")
	parts = append(parts, "     * 'разобрать текст на свитке' и 'попробовать снова разобрать текст' - это ОДНА задача")
	parts = append(parts, "     * 'сосредоточиться и прочитать записи' и 'изучить записи внимательнее' - это ОДНА задача")
	parts = append(parts, "   - Вместо этого СРАЗУ обработай существующий результат:")
	parts = append(parts, "     * Извлеки число из результата броска")
	parts = append(parts, "     * Определи характеристику и DC для проверки (на основе контекста ситуации)")
	parts = append(parts, "     * Вызови 'evaluate_check' с правильными параметрами")
	parts = append(parts, "     * Покажи результат игроку (✅ Успех! или ❌ Провал!)")
	parts = append(parts, "   - ⚠️ НИКОГДА не проси игрока бросить кубик снова, если результат уже есть в истории - это создает бесконечный цикл")
	parts = append(parts, "   - ⚠️ НИКОГДА не показывай технические сообщения типа 'Выполняется проверка Мудрости (WIS) (DC 10)' - это внутренние процессы")
	parts = append(parts, "1. Если в истории НЕТ результата броска, определи характеристику и DC (Difficulty Class) для действия:")
	parts = append(parts, "   - Легко: DC 10-12")
	parts = append(parts, "   - Средне: DC 13-15")
	parts = append(parts, "   - Сложно: DC 16-18")
	parts = append(parts, "   - Очень сложно: DC 19-20")
	parts = append(parts, "2. ⚠️ ОБЯЗАТЕЛЬНО вызови 'request_ability_check' с параметрами ability И dc (dc ОБЯЗАТЕЛЕН!). Этот инструмент вернет информацию о модификаторе персонажа для данной характеристики.")
	parts = append(parts, "   - ⚠️ ВАЖНО: Если инструмент вернул поле 'already_checked: true' и 'warning', это означает, что проверка этой характеристики уже была выполнена")
	parts = append(parts, "   - В D&D проверка навыка выполняется только один раз - если она провалена, нужно попробовать другой подход")
	parts = append(parts, "   - В этом случае НЕ проси игрока бросить кубик снова - вместо этого сообщи игроку естественным языком, что он уже пытался выполнить эту проверку, и предложи попробовать другой подход")
	parts = append(parts, "   - Используй поле 'warning' из результата инструмента как основу, но опиши это естественным языком, как настоящий DM")
	parts = append(parts, "   - Пример: 'Ты уже пытался разобрать текст на свитке, но не смог. Попробуй другой подход - может быть, поискать словарь или справочник, который поможет расшифровать текст, или попросить помощи у кого-то, кто знает этот язык.'")
	parts = append(parts, "   - ⚠️ КРИТИЧЕСКИ ВАЖНО: НИКОГДА не предлагай игроку повторить проверку после провала - в D&D проверка навыка выполняется только один раз")
	parts = append(parts, "3. Если инструмент НЕ вернул 'already_checked: true', опиши ситуацию естественным языком и попроси игрока бросить кубик d20. ВАЖНО:")
	parts = append(parts, "   - ⚠️ ОБЯЗАТЕЛЬНО четко укажи команду: 'Используй команду /roll d20' (не просто 'брось кубик', а именно команду)")
	parts = append(parts, "   - Объясни что такое DC простыми словами")
	parts = append(parts, "   - Примеры ПРАВИЛЬНЫХ формулировок:")
	parts = append(parts, "     * 'Ты пытаешься вспомнить информацию о кристаллах. Сделай проверку Мудрости. Тебе нужно выбросить на кубике d20 число, которое вместе с твоим модификатором Мудрости будет не меньше 14 (это называется DC - сложность проверки). Используй команду /roll d20.'")
	parts = append(parts, "     * 'Ты внимательно осматриваешь сундук, ища скрытые механизмы. Сделай проверку Интеллекта. Тебе нужно выбросить на кубике d20 число, которое вместе с твоим модификатором Интеллекта будет не меньше 12 (DC 12). Используй команду /roll d20.'")
	parts = append(parts, "     * 'Ты ощущаешь вибрации энергии портала. Сделай проверку Мудрости. Брось d20 и добавь свой модификатор Мудрости - если получится 12 или больше, проверка успешна (DC 12). Используй команду /roll d20.'")
	parts = append(parts, "   - ⚠️ НЕ показывай технические сообщения типа 'Выполняется проверка...' или 'Бросок d20: /roll d20' - это внутренние процессы")
	parts = append(parts, "4. ⚠️ КРИТИЧЕСКИ ВАЖНО: После того, как попросил игрока бросить кубик, СРАЗУ в следующем ответе АВТОМАТИЧЕСКИ проверь историю игры (раздел 'Последние сообщения' или 'Релевантная история игры').")
	parts = append(parts, "   - Результаты бросков автоматически сохраняются в историю с текстом 'Игрок бросил кубик: 🎲 Бросок d20: **<число>**'")
	parts = append(parts, "   - ⚠️ КРИТИЧЕСКИ ВАЖНО: НЕ ЖДИ следующего сообщения от игрока - если в истории есть результат броска (даже если это последнее сообщение в истории), СРАЗУ обработай его")
	parts = append(parts, "   - Если игрок написал просто число (например, '10'), это может быть результат броска - проверь историю, там должен быть полный результат")
	parts = append(parts, "   - ⚠️ ВАЖНО: Если ты только что попросил игрока бросить кубик, и в истории уже есть результат броска (даже если это последнее сообщение), СРАЗУ обработай его - не жди нового сообщения от игрока")
	parts = append(parts, "   - ⚠️ ВАЖНО: Если игрок написал '/roll d20' или просто число после того, как ты попросил бросить кубик, это означает что он уже бросил - проверь историю СРАЗУ и обработай результат")
	parts = append(parts, "   - ⚠️ КРИТИЧЕСКИ ВАЖНО: НЕ проси игрока бросить кубик снова, если в истории уже есть результат броска - обработай существующий результат")
	parts = append(parts, "   - ⚠️ КРИТИЧЕСКИ ВАЖНО: Если в истории есть несколько результатов бросков, используй ПОСЛЕДНИЙ результат броска для обработки")
	parts = append(parts, "5. Если в истории есть результат броска, ОБЯЗАТЕЛЬНО:")
	parts = append(parts, "   a) Извлеки число из результата броска (например, из '🎲 Бросок d20: **14**' извлеки 14)")
	parts = append(parts, "   b) Добавь модификатор из результата 'request_ability_check' (поле 'modifier') к числу броска (например, если бросок 14, а модификатор +2, то итоговый результат = 14 + 2 = 16)")
	parts = append(parts, "   c) ОБЯЗАТЕЛЬНО вызови инструмент 'evaluate_check' с параметрами:")
	parts = append(parts, "      - ability: та же характеристика, что использовалась в 'request_ability_check'")
	parts = append(parts, "      - dc: та же DC, что использовалась в 'request_ability_check'")
	parts = append(parts, "      - roll_result: итоговый результат (бросок + модификатор)")
	parts = append(parts, "   d) ⚠️ ОБЯЗАТЕЛЬНО явно покажи игроку результат проверки естественным языком:")
	parts = append(parts, "      - ВСЕГДА начинай с явного указания успеха или провала: '✅ Успех!' или '❌ Провал!'")
	parts = append(parts, "      - Если success=true: опиши что произошло в результате успешной проверки естественным языком")
	parts = append(parts, "        Пример: '✅ Успех! Ты вспоминаешь, что эти кристаллы используются для создания магических артефактов. Они могут усиливать заклинания, но требуют особой осторожности при обращении.'")
	parts = append(parts, "      - Если success=false: опиши что произошло в результате провала естественным языком")
	parts = append(parts, "        Пример: '❌ Провал! Ты пытаешься вспомнить информацию о кристаллах, но ничего конкретного не приходит на ум. Возможно, тебе нужно больше времени или дополнительная информация.'")
	parts = append(parts, "   e) Используй поле 'message' из результата 'evaluate_check' как основу, но опиши результат естественным языком, как настоящий DM")
	parts = append(parts, "   ⚠️ КРИТИЧЕСКИ ВАЖНО: ВСЕГДА явно показывай успех (✅ Успех!) или провал (❌ Провал!) в начале описания результата - игрок должен сразу понимать, прошла ли проверка")
	parts = append(parts, "   ⚠️ КРИТИЧЕСКИ ВАЖНО: НИКОГДА не предлагай игроку повторить проверку после провала - в D&D проверка навыка выполняется только один раз")
	parts = append(parts, "   ⚠️ КРИТИЧЕСКИ ВАЖНО: НИКОГДА не проси игрока использовать команду /roll d20 снова для той же проверки после провала")
	parts = append(parts, "   ⚠️ КРИТИЧЕСКИ ВАЖНО: Если проверка провалена, предложи игроку попробовать другой подход:")
	parts = append(parts, "     * Использовать другой навык (например, вместо Интеллекта использовать Мудрость)")
	parts = append(parts, "     * Найти дополнительную информацию (книги, словари, справочники)")
	parts = append(parts, "     * Попросить помощи у NPC")
	parts = append(parts, "     * Вернуться к этому позже (в другой ситуации)")
	parts = append(parts, "   ⚠️ НЕ показывай технические сообщения типа 'Оценивается результат проверки' или 'Опиши результат проверки игроку' - это внутренние процессы")
	parts = append(parts, "")
	parts = append(parts, "⚠️ КРИТИЧЕСКИ ВАЖНО:")
	parts = append(parts, "- НЕ вызывай 'request_ability_check' БЕЗ указания DC - это приведет к ошибке")
	parts = append(parts, "- НЕ описывай результат проверки до броска игрока")
	parts = append(parts, "- НЕ описывай результат проверки без использования 'evaluate_check'")
	parts = append(parts, "- НЕ ЖДИ сообщения от игрока после того, как попросил бросить кубик - СРАЗУ в следующем ответе проверь историю автоматически")
	parts = append(parts, "- ⚠️ КРИТИЧЕСКИ ВАЖНО: ПЕРЕД тем как просить игрока бросить кубик, проверь историю игры - если там уже есть результат броска (текст 'Игрок бросил кубик: 🎲 Бросок d20: **<число>**'), НЕ проси игрока бросить снова, а обработай существующий результат")
	parts = append(parts, "- ⚠️ КРИТИЧЕСКИ ВАЖНО: Если в истории есть результат броска, НЕ проси игрока бросить кубик снова - обработай существующий результат через 'evaluate_check' и покажи результат игроку")
	parts = append(parts, "- ВСЕГДА явно показывай игроку успех или провал проверки (✅ Успех! или ❌ Провал!)")
	parts = append(parts, "- Если проверка успешна, опиши что произошло в результате успеха")
	parts = append(parts, "- Если проверка провалена, опиши что произошло в результате провала")
	parts = append(parts, "- ⚠️ ОБЯЗАТЕЛЬНО объясняй что такое DC простыми словами, чтобы игрок понимал механику")
	parts = append(parts, "- ⚠️ НЕ показывай игроку технические сообщения типа 'Выполняется проверка Мудрости (WIS) (DC 10)', 'Оценивается результат...', 'Бросок d20: /roll d20' - это внутренние процессы, которые игроку не нужны")
	parts = append(parts, "- ВСЕГДА описывай ситуацию естественным языком, как настоящий DM, без технических деталей")
	parts = append(parts, "- Пример ПРАВИЛЬНОГО ответа: 'Ты пытаешься вспомнить информацию о кристаллах. Сделай проверку Мудрости. Тебе нужно выбросить на кубике d20 число, которое вместе с твоим модификатором Мудрости будет не меньше 14 (это называется DC - сложность проверки). Используй команду /roll d20.'")
	parts = append(parts, "- Пример НЕПРАВИЛЬНОГО ответа: 'Выполняется проверка Мудрости (WIS) (DC 14). Бросок d20: /roll d20. Оценивается результат проверки характеристики.'")
	parts = append(parts, "")
	parts = append(parts, "⚠️ КРИТИЧЕСКИ ВАЖНО - Когда НЕ использовать проверки:")
	parts = append(parts, "- НЕ используй проверки для простых действий, которые всегда успешны или не требуют проверки:")
	parts = append(parts, "   * Взять предмет, поднять предмет с земли")
	parts = append(parts, "   * Открыть обычную дверь (не запертую)")
	parts = append(parts, "   * Пройти по дороге, тропинке")
	parts = append(parts, "   * Подняться по ступеням, лестнице (если нет препятствий)")
	parts = append(parts, "   * Войти в здание, храм (если дверь открыта)")
	parts = append(parts, "   * Сесть, встать, лечь")
	parts = append(parts, "   * Говорить, кричать")
	parts = append(parts, "   * Идти, бежать (если нет препятствий)")
	parts = append(parts, "   * Найти местных жителей, подойти к NPC (если они видны)")
	parts = append(parts, "   * Выслушать информацию от NPC (если они готовы говорить)")
	parts = append(parts, "   * Отправиться в путь, начать движение")
	parts = append(parts, "   * Приблизиться к объекту, подойти к чему-то")
	parts = append(parts, "   * Осветить путь фонарем, зажечь факел")
	parts = append(parts, "- ⚠️ КРИТИЧЕСКИ ВАЖНО: НЕ используй проверки для абстрактных понятий:")
	parts = append(parts, "   * 'Проверить удачу перед походом' - НЕ нужно, просто опиши начало пути")
	parts = append(parts, "   * 'Проверить удачливость перед погружением' - НЕ нужно, просто опиши погружение")
	parts = append(parts, "   * 'Проверить реакцию на падение' - НЕ нужно, просто опиши что произошло при падении")
	parts = append(parts, "   * 'Проверить решимость' - НЕ нужно, просто опиши действие")
	parts = append(parts, "- ⚠️ КРИТИЧЕСКИ ВАЖНО: НЕ проси игрока бросать кубики для каждого действия - это делает игру неинтересной")
	parts = append(parts, "- ⚠️ КРИТИЧЕСКИ ВАЖНО: Если действие простое и не требует проверки, просто опиши результат естественным языком БЕЗ запроса проверки")
	parts = append(parts, "")
	parts = append(parts, "Когда использовать проверки навыков:")
	parts = append(parts, "- Используй проверки ТОЛЬКО для действий, которые имеют неопределенный исход и важны для сюжета:")
	parts = append(parts, "   * Открыть замок, взломать дверь")
	parts = append(parts, "   * Заметить ловушку, скрытую дверь, секретный проход")
	parts = append(parts, "   * Убедить NPC, обмануть, запугать")
	parts = append(parts, "   * Вспомнить важную информацию (знания, история, магия)")
	parts = append(parts, "   * Найти скрытые предметы, разгадать загадку")
	parts = append(parts, "   * Расшифровать текст, прочитать древний язык")
	parts = append(parts, "   * Избежать ловушки, успеть среагировать на опасность")
	parts = append(parts, "- ⚠️ КРИТИЧЕСКИ ВАЖНО: НЕ злоупотребляй проверками - используй их только когда это действительно важно для сюжета или игрового процесса")
	parts = append(parts, "- ⚠️ КРИТИЧЕСКИ ВАЖНО: Если действие простое и не требует проверки, просто опиши результат естественным языком без запроса проверки")

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
