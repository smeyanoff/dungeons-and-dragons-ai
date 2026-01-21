package context

import (
	"context"
	"fmt"
	"os"
	"strconv"
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
	config        ragContextConfig
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
		config:        loadRAGContextConfig(),
	}
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
			if len(recentEvents) > maxRecentEvents {
				startIdx = len(recentEvents) - maxRecentEvents
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
				parts = append(parts, fmt.Sprintf("%s: %s", authorPrefix, truncateText(e.Content, maxEventChars)))
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
			parts = append(parts, "📋 Проверки навыков в бою:")
			parts = append(parts, "• Используй 'request_ability_check' для действий, требующих проверки (см. общие инструкции ниже)")
			parts = append(parts, "• Следуй тем же правилам, что и вне боя")
			logger.Debug("Added combat information to context",
				logger.Uint("session_id", gs.ID),
				logger.Int("participants_count", len(activeCombat.Participants)),
			)
		}
	}

	// Добавляем общие инструкции по использованию ability check tools (вне боя)
	parts = append(parts, "\n--- Инструкции по проверкам навыков (ABILITY CHECKS) ---")
	parts = append(parts, "")
	parts = append(parts, "🚨 КРИТИЧЕСКИ ВАЖНО - ПРОЧИТАЙ ПЕРВЫМ:")
	parts = append(parts, "• ⚠️ НЕ проси игрока бросать кубики для простых действий (расспросы, разговоры, получение информации)")
	parts = append(parts, "• ⚠️ НЕ проси проверки для описательных действий: 'прислушаться', 'осмотреть', 'почувствовать', 'заметить', 'увидеть', 'услышать', 'послушать', 'ощутить' - просто опиши результат БЕЗ проверки")
	parts = append(parts, "• ⚠️ НЕ проси проверки для атмосферных действий: 'осмотреть окрестности', 'почувствовать ветер', 'уловить детали', 'послушать звуки', 'ощутить атмосферу' - просто опиши результат БЕЗ проверки")
	parts = append(parts, "• ⚠️ НЕ проси проверки для абстрактных понятий: 'интуиция', 'реакция', 'удача', 'решимость', 'присутствие' - просто опиши результат БЕЗ проверки")
	parts = append(parts, "• ⚠️ КРИТИЧЕСКИ ВАЖНО: Если описываешь локацию или атмосферу - НЕ предлагай проверки в конце описания")
	parts = append(parts, "• ⚠️ КРИТИЧЕСКИ ВАЖНО: НЕ используй фразы 'Используй команду /roll d20, если захочешь...' для описательных или атмосферных действий")
	parts = append(parts, "• ⚠️ Для простых действий просто опиши результат естественным языком БЕЗ проверки")
	parts = append(parts, "• ⚠️ Для случайных событий используй инструмент 'roll_dice' - НЕ проси игрока бросать кубики")
	parts = append(parts, "• ⚠️ Проверки навыков используй ТОЛЬКО для конкретных действий с неопределенным исходом (замки, ловушки, важные убеждения)")
	parts = append(parts, "• ⚠️ После успешной проверки НЕ проси новую для продолжения той же задачи - используй результат и продолжай сюжет")
	parts = append(parts, "")
	parts = append(parts, "📋 ОБЩИЕ ПРАВИЛА:")
	parts = append(parts, "• Отвечай естественным языком, как настоящий DM")
	parts = append(parts, "• НЕ показывай технические сообщения ('Выполняется проверка...', 'Оценивается результат...')")
	parts = append(parts, "• ВСЕГДА указывай команду: 'Используй команду /roll d20' (только когда действительно нужна проверка)")
	parts = append(parts, "• ВСЕГДА показывай результат: '✅ Успех!' или '❌ Провал!' в начале описания (только после проверки)")
	parts = append(parts, "")
	parts = append(parts, "🎯 ГЛАВНОЕ ПРАВИЛО:")
	parts = append(parts, "• ⚠️ КРИТИЧЕСКИ ВАЖНО: НЕ проси игрока бросать кубики для каждого действия - это делает игру неинтересной")
	parts = append(parts, "• ⚠️ КРИТИЧЕСКИ ВАЖНО: Если описываешь локацию, атмосферу или окружение - просто опиши что видит/слышит/чувствует игрок БЕЗ предложения проверок")
	parts = append(parts, "• ⚠️ КРИТИЧЕСКИ ВАЖНО: НЕ заканчивай описание локации фразой 'Используй команду /roll d20, если захочешь...' - это ЗАПРЕЩЕНО")
	parts = append(parts, "• Используй проверки ТОЛЬКО когда действие имеет неопределенный исход и важно для сюжета")
	parts = append(parts, "• Для большинства действий просто опиши результат БЕЗ проверки")
	parts = append(parts, "• Для случайных событий (погода, встреча, случайный результат) используй инструмент 'roll_dice' - НЕ проси игрока бросать кубики")
	parts = append(parts, "")
	parts = append(parts, "❌ Когда НЕ использовать проверки (КРИТИЧЕСКИ ВАЖНО!):")
	parts = append(parts, "• ⚠️ Простые вопросы: 'где я?', 'что здесь?', 'куда идти?' - просто опиши локацию БЕЗ проверки")
	parts = append(parts, "• ⚠️ Простое перемещение: 'пойти', 'идти', 'двигаться', 'пойти вглубь', 'пойти дальше', 'пойти в лес' - просто опиши результат перемещения БЕЗ проверки")
	parts = append(parts, "• ⚠️ Расспросы, вопросы NPC, получение информации: 'спросить', 'расспросить', 'узнать', 'где', 'как', 'кто' - просто опиши ответ БЕЗ проверки")
	parts = append(parts, "• ⚠️ Разговоры, беседы с NPC - просто опиши диалог БЕЗ проверки")
	parts = append(parts, "• ⚠️ Получение информации от NPC - просто дай информацию БЕЗ проверки")
	parts = append(parts, "• ⚠️ Описательные действия: 'прислушаться', 'осмотреть', 'почувствовать', 'заметить', 'увидеть', 'услышать', 'послушать', 'ощутить', 'почувствовать интуицию', 'уловить звуки', 'увидеть признаки' - просто опиши результат БЕЗ проверки")
	parts = append(parts, "• ⚠️ Абстрактные понятия: 'проверить удачу', 'проверить реакцию', 'проверить решимость', 'прислушаться к интуиции', 'почувствовать присутствие' - просто опиши результат БЕЗ проверки")
	parts = append(parts, "• Простые действия: взять предмет, открыть обычную дверь, пройти по дороге, подняться по лестнице, войти в здание, сесть/встать, говорить, идти, бежать")
	parts = append(parts, "• Если действие простое, описательное или информационное - просто опиши результат естественным языком БЕЗ проверки")
	parts = append(parts, "")
	parts = append(parts, "📝 ПРИМЕРЫ ПРАВИЛЬНОГО ИСПОЛЬЗОВАНИЯ:")
	parts = append(parts, "• Игрок: 'где я?' → DM: 'Ты просыпаешься в небольшом лесу...' (БЕЗ проверки)")
	parts = append(parts, "• Игрок: 'пойти вглубь леса' → DM: 'Ты углубляешься в лес...' (БЕЗ проверки)")
	parts = append(parts, "• Игрок: 'прислушаться к звукам' → DM: 'Ты слышишь шелест листьев...' (БЕЗ проверки)")
	parts = append(parts, "• Игрок: 'осмотреть окрестности' → DM: 'Вокруг тебя виднеются...' (БЕЗ проверки)")
	parts = append(parts, "")
	parts = append(parts, "📝 ПРИМЕРЫ НЕПРАВИЛЬНОГО ИСПОЛЬЗОВАНИЯ (ЗАПРЕЩЕНО!):")
	parts = append(parts, "• Игрок: 'где я?' → DM: 'Используй команду /roll d20, чтобы...' (НЕПРАВИЛЬНО!)")
	parts = append(parts, "• Игрок: 'пойти вглубь леса' → DM: 'Используй команду /roll d20, если захочешь...' (НЕПРАВИЛЬНО!)")
	parts = append(parts, "• Игрок: 'прислушаться' → DM: 'Используй команду /roll d20, чтобы уловить звуки...' (НЕПРАВИЛЬНО!)")
	parts = append(parts, "")
	parts = append(parts, "🎲 Для случайных событий:")
	parts = append(parts, "• Если нужно определить случайное событие (погода, встреча с NPC, случайный результат), используй инструмент 'roll_dice'")
	parts = append(parts, "• НЕ проси игрока бросать кубики для случайных событий - бросай кубики сам через инструмент")
	parts = append(parts, "• Примеры: 'определить погоду', 'встретить ли NPC', 'какой результат из нескольких вариантов'")
	parts = append(parts, "")
	parts = append(parts, "✅ Когда использовать проверки навыков (request_ability_check):")
	parts = append(parts, "• ТОЛЬКО для конкретных действий с неопределенным исходом:")
	parts = append(parts, "  - Открыть замок, взломать запертую дверь → 'dexterity' или 'strength'")
	parts = append(parts, "  - Заметить ловушку, скрытую дверь, секретный проход → 'intelligence' или 'wisdom'")
	parts = append(parts, "  - Убедить/обмануть/запугать NPC в ВАЖНОЙ ситуации (не просто разговор!) → 'charisma'")
	parts = append(parts, "  - Вспомнить важную информацию, расшифровать древний текст → 'intelligence'")
	parts = append(parts, "  - Проверка восприятия для поиска скрытых вещей → 'wisdom'")
	parts = append(parts, "• ⚠️ ВАЖНО: Для обычных разговоров и расспросов НЕ используй проверки - просто дай информацию")
	parts = append(parts, "")
	parts = append(parts, "📍 ПРЕДОПРЕДЕЛЕННЫЕ ПРОВЕРКИ В ЛОКАЦИЯХ (КРИТИЧЕСКИ ВАЖНО!):")
	parts = append(parts, "• ⚠️ Предопределенные проверки можно использовать ТОЛЬКО когда игрок находится в указанном месте (см. LocationHint в описании проверки)")
	parts = append(parts, "• ⚠️ НЕ используй предопределенные проверки просто так - они должны срабатывать только когда игрок находится в конкретном месте локации")
	parts = append(parts, "• ⚠️ НЕ используй предопределенные проверки при простом перемещении в локацию - дождись, пока игрок окажется в конкретном месте")
	parts = append(parts, "• ⚠️ КРИТИЧЕСКИ ВАЖНО: Если игрок говорит 'пойти вглубь леса' или просто перемещается - НЕ используй предопределенные проверки сразу")
	parts = append(parts, "• ⚠️ КРИТИЧЕСКИ ВАЖНО: Предопределенные проверки срабатывают ТОЛЬКО когда игрок явно находится в указанном месте (например, 'глубоко в чаще леса', 'в северной части зала')")
	parts = append(parts, "• ⚠️ Пример: Если LocationHint = 'глубоко в чаще леса', проверку можно использовать ТОЛЬКО когда игрок явно находится 'глубоко в чаще', а НЕ просто 'идет в лес'")
	parts = append(parts, "• Пример ПРАВИЛЬНОГО использования:")
	parts = append(parts, "  - Игрок: 'подхожу к северной части храма, возле разрушенной колонны'")
	parts = append(parts, "  - DM: 'Ты подходишь к разрушенной колонне. Сделай проверку Мудрости (DC 12) - услышать тихий шепот духа. Используй команду /roll d20.'")
	parts = append(parts, "  - Игрок: 'осматриваю колонну внимательнее'")
	parts = append(parts, "  - DM: 'Ты внимательно осматриваешь колонну. Сделай проверку Мудрости (DC 12) - услышать тихий шепот духа. Используй команду /roll d20.'")
	parts = append(parts, "• Пример НЕПРАВИЛЬНОГО использования:")
	parts = append(parts, "  - Игрок: 'пойду на болото' (просто перемещение)")
	parts = append(parts, "  - DM: 'Используй команду /roll d20 для проверки Мудрости' (НЕПРАВИЛЬНО - игрок еще не в указанном месте!)")
	parts = append(parts, "  - Игрок: 'вхожу в храм' (просто вход)")
	parts = append(parts, "  - DM: 'Используй команду /roll d20 для проверки Ловкости' (НЕПРАВИЛЬНО - игрок еще не у зыбкой грязи!)")
	parts = append(parts, "• ⚠️ Если игрок просто перемещается в локацию, НЕ используй предопределенные проверки сразу - дождись, пока игрок окажется в конкретном месте")
	parts = append(parts, "• ⚠️ Если LocationHint указывает 'в северной части храма, возле разрушенной колонны', проверку можно использовать ТОЛЬКО когда игрок находится именно там")
	parts = append(parts, "• ⚠️ Если LocationHint указывает 'по краю болота, окружающему храм', проверку можно использовать ТОЛЬКО когда игрок находится именно там")
	parts = append(parts, "")
	parts = append(parts, "🔄 ПРАВИЛО ОДНОЙ ПРОВЕРКИ:")
	parts = append(parts, "• Проверка для конкретной задачи выполняется ТОЛЬКО ОДИН РАЗ")
	parts = append(parts, "• Если провалена, игрок НЕ МОЖЕТ повторить для той же задачи")
	parts = append(parts, "• Для повторной попытки нужен ДРУГОЙ подход (другой навык, дополнительная информация)")
	parts = append(parts, "• Примеры ОДНОЙ задачи: 'прочитать записи' и 'изучить оставшиеся записи' - это ОДНА задача")
	parts = append(parts, "• ⚠️ КРИТИЧЕСКИ ВАЖНО: После успешной проверки НЕ проси новую для продолжения той же задачи")
	parts = append(parts, "• ⚠️ Если игрок уже выполнил проверку и получил результат - используй этот результат, НЕ проси новую проверку")
	parts = append(parts, "• ⚠️ Пример НЕПРАВИЛЬНО: Игрок выполнил проверку мудрости (успех) → ДМ просит новую проверку для 'продолжения' - ЭТО ЗАПРЕЩЕНО")
	parts = append(parts, "• ⚠️ Пример ПРАВИЛЬНО: Игрок выполнил проверку мудрости (успех) → ДМ описывает результат и продолжает сюжет БЕЗ новых проверок")
	parts = append(parts, "")
	parts = append(parts, "⚙️ ПРОЦЕСС ИСПОЛЬЗОВАНИЯ:")
	parts = append(parts, "")
	parts = append(parts, "ШАГ 0 - ПЕРЕД запросом проверки (КРИТИЧЕСКИ ВАЖНО!):")
	parts = append(parts, "• ВСЕГДА проверь историю игры ('Последние сообщения') ПЕРЕД запросом проверки")
	parts = append(parts, "• Если в истории есть результат броска ('🎲 Бросок d20: **<число>**'), НЕ запрашивай новую проверку")
	parts = append(parts, "• Если есть результат проверки для похожей задачи, НЕ запрашивай повторную проверку")
	parts = append(parts, "• Вместо этого СРАЗУ обработай существующий результат:")
	parts = append(parts, "  - Извлеки число из результата броска")
	parts = append(parts, "  - Определи характеристику и DC (на основе контекста)")
	parts = append(parts, "  - Вызови 'evaluate_check' с правильными параметрами")
	parts = append(parts, "  - Покажи результат игроку (✅ Успех! или ❌ Провал!)")
	parts = append(parts, "• ⚠️ НИКОГДА не проси бросить кубик снова, если результат уже есть в истории")
	parts = append(parts, "")
	parts = append(parts, "ШАГ 1 - Если в истории НЕТ результата броска:")
	parts = append(parts, "• Определи характеристику и DC:")
	parts = append(parts, "  - Легко: DC 10-12")
	parts = append(parts, "  - Средне: DC 13-15")
	parts = append(parts, "  - Сложно: DC 16-18")
	parts = append(parts, "  - Очень сложно: DC 19-20")
	parts = append(parts, "")
	parts = append(parts, "ШАГ 2 - Вызови 'request_ability_check':")
	parts = append(parts, "• ОБЯЗАТЕЛЬНО укажи параметры: ability, dc, reason, stakes (все обязательны)")
	parts = append(parts, "• Если инструмент вернул 'already_checked: true':")
	parts = append(parts, "  - НЕ проси игрока бросить кубик снова")
	parts = append(parts, "  - Сообщи игроку естественным языком, что проверка уже была выполнена")
	parts = append(parts, "  - Предложи попробовать другой подход")
	parts = append(parts, "")
	parts = append(parts, "ШАГ 3 - Если инструмент НЕ вернул 'already_checked':")
	parts = append(parts, "• Опиши ситуацию естественным языком")
	parts = append(parts, "• ОБЯЗАТЕЛЬНО укажи команду: 'Используй команду /roll d20'")
	parts = append(parts, "• Объясни что такое DC простыми словами")
	parts = append(parts, "• Пример: 'Ты пытаешься вспомнить информацию о кристаллах. Сделай проверку Мудрости. Тебе нужно выбросить на кубике d20 число, которое вместе с твоим модификатором Мудрости будет не меньше 14 (DC 14). Используй команду /roll d20.'")
	parts = append(parts, "")
	parts = append(parts, "ШАГ 4 - После запроса броска (КРИТИЧЕСКИ ВАЖНО!):")
	parts = append(parts, "• СРАЗУ в следующем ответе АВТОМАТИЧЕСКИ проверь историю игры")
	parts = append(parts, "• НЕ ЖДИ сообщения от игрока - если в истории есть результат броска, СРАЗУ обработай его")
	parts = append(parts, "• Результаты сохраняются как: 'Игрок бросил кубик: 🎲 Бросок d20: **<число>**'")
	parts = append(parts, "• Если есть несколько результатов, используй ПОСЛЕДНИЙ")
	parts = append(parts, "")
	parts = append(parts, "ШАГ 5 - Если в истории есть результат броска:")
	parts = append(parts, "• Извлеки число из результата (например, из '🎲 Бросок d20: **14**' → 14)")
	parts = append(parts, "• Добавь модификатор из 'request_ability_check' (поле 'modifier')")
	parts = append(parts, "• Вызови 'evaluate_check' с параметрами:")
	parts = append(parts, "  - ability: та же характеристика, что в 'request_ability_check'")
	parts = append(parts, "  - dc: та же DC, что в 'request_ability_check'")
	parts = append(parts, "  - roll_result: итоговый результат (бросок + модификатор)")
	parts = append(parts, "• Покажи результат игроку:")
	parts = append(parts, "  - ВСЕГДА начинай с '✅ Успех!' или '❌ Провал!'")
	parts = append(parts, "  - Опиши что произошло естественным языком")
	parts = append(parts, "  - Если провал, предложи другой подход (другой навык, дополнительная информация, помощь NPC)")
	parts = append(parts, "  - НИКОГДА не предлагай повторить проверку после провала")
	parts = append(parts, "")
	parts = append(parts, "⚠️ КРИТИЧЕСКИ ВАЖНО:")
	parts = append(parts, "• НЕ вызывай 'request_ability_check' БЕЗ указания dc, reason и stakes")
	parts = append(parts, "• НЕ описывай результат проверки до броска игрока")
	parts = append(parts, "• НЕ описывай результат без использования 'evaluate_check'")
	parts = append(parts, "• НЕ ЖДИ сообщения от игрока - проверь историю автоматически")
	parts = append(parts, "• ВСЕГДА явно показывай успех или провал (✅ Успех! или ❌ Провал!)")
	parts = append(parts, "• НЕ показывай технические сообщения игроку")

	// Используем RAG для поиска релевантных событий с таймаутом
	// Используем сообщение игрока как запрос для поиска
	logger.Debug("Retrieving RAG context",
		logger.Uint("session_id", gs.ID),
		logger.String("query", playerMessage),
	)
	ragCtx, ragCancel := context.WithTimeout(ctx, 15*time.Second)
	defer ragCancel()
	ragDocs, err := b.retrieveUC.Execute(ragCtx, gs.ID, playerMessage, maxRAGDocs)
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
		totalChars := 0
		for i, doc := range ragDocs {
			if i >= maxRAGDocs || totalChars >= maxRAGTotalChars {
				break
			}
			docText := truncateText(doc.Text, maxRAGDocChars)
			if totalChars+len(docText) > maxRAGTotalChars {
				docText = truncateText(docText, maxRAGTotalChars-totalChars)
			}
			totalChars += len(docText)
			parts = append(parts, fmt.Sprintf("[%d] %s", i+1, docText))
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
	maxRAGDocs       int
	maxRAGDocChars   int
	maxRAGTotalChars int
}

func loadRAGContextConfig() ragContextConfig {
	return ragContextConfig{
		maxRecentEvents:  getEnvInt("RAG_MAX_RECENT_EVENTS", 8),
		maxEventChars:    getEnvInt("RAG_MAX_EVENT_CHARS", 800),
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
