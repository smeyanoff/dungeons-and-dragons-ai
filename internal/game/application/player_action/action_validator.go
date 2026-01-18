package player_action

import (
	"context"
	"fmt"
	"strings"

	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/inventory"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/pkg/logger"
)

// ActionValidator валидирует действия игрока перед отправкой в LLM
type ActionValidator struct {
	inventoryRepo InventoryRepository
}

// InventoryRepository интерфейс для работы с инвентарем (общий для ActionValidator и HandleActionUseCase)
type InventoryRepository interface {
	GetByCharacterID(ctx context.Context, characterID uint) (*inventory.Inventory, error)
	Save(ctx context.Context, inv *inventory.Inventory) error
}

func NewActionValidator(inventoryRepo InventoryRepository) *ActionValidator {
	return &ActionValidator{
		inventoryRepo: inventoryRepo,
	}
}

// ValidationResult результат валидации действия
type ValidationResult struct {
	Valid   bool
	Message string
}

// Validate проверяет возможность выполнения действия
func (v *ActionValidator) Validate(
	ctx context.Context,
	gs *session.GameSession,
	playerMessage string,
) (*ValidationResult, error) {
	player := gs.GetFirstPlayer()
	if player == nil {
		return &ValidationResult{
			Valid:   true, // Разрешаем действие, если нет персонажа (для обратной совместимости)
			Message: "",
		}, nil
	}

	char := player.Character

	// Проверяем, жив ли персонаж
	if char.Status == character.StatusDead {
		return &ValidationResult{
			Valid:   false,
			Message: "Ваш персонаж мертв. Вы не можете выполнять действия.",
		}, nil
	}

	// Нормализуем сообщение для анализа
	normalizedMessage := strings.ToLower(strings.TrimSpace(playerMessage))

	// Проверяем использование предметов из инвентаря
	itemCheck := v.checkItemUsage(ctx, char.ID, normalizedMessage)
	if !itemCheck.Valid {
		return itemCheck, nil
	}

	// Проверяем требования к характеристикам
	statsCheck := v.checkStatsRequirements(char.Stats, normalizedMessage)
	if !statsCheck.Valid {
		return statsCheck, nil
	}

	return &ValidationResult{
		Valid:   true,
		Message: "",
	}, nil
}

// checkItemUsage проверяет наличие предметов, упомянутых в действии
func (v *ActionValidator) checkItemUsage(
	ctx context.Context,
	characterID uint,
	message string,
) *ValidationResult {
	if v.inventoryRepo == nil {
		// Если репозиторий не настроен, пропускаем проверку
		return &ValidationResult{Valid: true, Message: ""}
	}

	inv, err := v.inventoryRepo.GetByCharacterID(ctx, characterID)
	if err != nil {
		// Если не удалось получить инвентарь, логируем и пропускаем проверку
		logger.Warn("Failed to get inventory for validation",
			logger.ErrorField(err),
			logger.Uint("character_id", characterID),
		)
		return &ValidationResult{Valid: true, Message: ""}
	}

	if inv == nil || len(inv.Items) == 0 {
		// Если инвентарь пуст, проверяем только использование предметов
		return v.checkEmptyInventoryItemUsage(message)
	}

	// Проверяем наличие упомянутых предметов
	mentionedItems := v.extractItemNames(message)
	if len(mentionedItems) == 0 {
		return &ValidationResult{Valid: true, Message: ""}
	}

	// Проверяем наличие каждого предмета в инвентаре
	for _, itemName := range mentionedItems {
		hasItem := false
		for _, invItem := range inv.Items {
			if strings.Contains(strings.ToLower(invItem.Name), itemName) {
				hasItem = true
				break
			}
		}

		if !hasItem {
			return &ValidationResult{
				Valid:   false,
				Message: fmt.Sprintf("У вас нет предмета '%s' в инвентаре.", itemName),
			}
		}
	}

	return &ValidationResult{Valid: true, Message: ""}
}

// checkEmptyInventoryItemUsage проверяет использование предметов при пустом инвентаре
func (v *ActionValidator) checkEmptyInventoryItemUsage(message string) *ValidationResult {
	// Список действий, которые могут требовать предметы
	itemActions := []string{
		"использовать", "применить", "выпить", "съесть", "надеть", "одеть",
		"взять", "бросить", "стрелять", "атаковать мечом", "атаковать оружием",
	}

	for _, action := range itemActions {
		if strings.Contains(message, action) {
			// Извлекаем название предмета после действия
			parts := strings.SplitAfter(message, action)
			if len(parts) > 1 {
				itemPart := strings.TrimSpace(parts[1])
				if len(itemPart) > 0 {
					return &ValidationResult{
						Valid:   false,
						Message: fmt.Sprintf("У вас нет предметов в инвентаре для действия '%s'.", action),
					}
				}
			}
		}
	}

	return &ValidationResult{Valid: true, Message: ""}
}

// extractItemNames извлекает названия предметов из сообщения
func (v *ActionValidator) extractItemNames(message string) []string {
	// Простая эвристика для извлечения предметов
	// Ищем слова после ключевых глаголов использования
	verbs := []string{"использовать", "применить", "выпить", "съесть", "надеть", "одеть", "бросить", "стрелять"}
	var items []string

	for _, verb := range verbs {
		if strings.Contains(message, verb) {
			// Пытаемся извлечь слово после глагола
			parts := strings.SplitAfter(message, verb)
			if len(parts) > 1 {
				itemPart := strings.TrimSpace(parts[1])
				// Берем первое слово или несколько слов (до предлога или союза)
				words := strings.Fields(itemPart)
				stopWords := map[string]bool{
					"и": true, "или": true, "с": true, "в": true, "на": true,
					"для": true, "от": true, "к": true, "а": true, "но": true,
				}
				if len(words) > 0 {
					var itemWords []string
					for _, word := range words {
						if stopWords[strings.ToLower(word)] {
							break
						}
						itemWords = append(itemWords, word)
						if len(itemWords) >= 2 { // Берем максимум 2 слова
							break
						}
					}
					if len(itemWords) > 0 {
						items = append(items, strings.Join(itemWords, " "))
					}
				}
			}
		}
	}

	return items
}

// checkStatsRequirements проверяет требования к характеристикам для действия
func (v *ActionValidator) checkStatsRequirements(
	stats character.Stats,
	message string,
) *ValidationResult {
	// Исключаем метафорические действия (не проверяем их)
	metaphoricalPhrases := []string{
		"поднять глаза", "поднять взгляд", "поднять взор", "поднять голову",
		"поднять настроение", "поднять дух", "поднять бровь", "поднять брови",
		"поднять руку", "поднять руки", // Поднять руку может быть как физическим, так и метафорическим
	}

	// Проверяем, не является ли действие метафорическим
	for _, phrase := range metaphoricalPhrases {
		if strings.Contains(message, phrase) {
			// Это метафорическое действие, не проверяем характеристики
			return &ValidationResult{Valid: true, Message: ""}
		}
	}

	// Проверяем физические действия, требующие силы
	// Более специфичные паттерны для физических действий
	strengthActionPatterns := []struct {
		verb        string
		requiresObj bool // Требуется ли физический объект после глагола
	}{
		{"поднять", true},  // "поднять меч", "поднять камень" - требует объект
		{"поднимаю", true},
		{"толкнуть", true},  // "толкнуть дверь", "толкнуть камень"
		{"толкаю", true},
		{"сломать", true},   // "сломать дверь", "сломать палку"
		{"ломаю", true},
		{"перетащить", true}, // "перетащить ящик"
		{"тащу", true},      // "тащу ящик"
		{"нести", true},     // "нести сумку"
		{"держать", true},   // "держать меч", "держать щит"
	}

	for _, pattern := range strengthActionPatterns {
		if strings.Contains(message, pattern.verb) {
			// Проверяем, что после глагола есть существительное (объект действия)
			// Извлекаем текст после глагола
			parts := strings.SplitAfter(message, pattern.verb)
			if len(parts) > 1 {
				afterVerb := strings.TrimSpace(parts[1])
				// Проверяем, что после глагола есть хотя бы одно слово (объект)
				if len(afterVerb) > 0 && len(strings.Fields(afterVerb)) > 0 {
					// Исключаем метафорические объекты
					metaphoricalObjects := []string{"глаза", "взгляд", "взор", "настроение", "дух", "бровь", "брови"}
					firstWord := strings.ToLower(strings.Fields(afterVerb)[0])
					isMetaphorical := false
					for _, obj := range metaphoricalObjects {
						if firstWord == obj {
							isMetaphorical = true
							break
						}
					}
					if !isMetaphorical {
						// Это физическое действие с реальным объектом, проверяем силу
						if stats.Strength < 10 {
							return &ValidationResult{
								Valid:   false,
								Message: fmt.Sprintf("Ваша сила (%d) недостаточна для этого действия. Требуется минимум 10.", stats.Strength),
							}
						}
					}
				}
			}
		}
	}

	// Проверяем действия, требующие ловкости
	dexterityActions := []string{
		"забраться", "лазить", "балансировать", "красться", "тихо",
		"пролезть", "прыгнуть", "перепрыгнуть",
	}

	for _, action := range dexterityActions {
		if strings.Contains(message, action) {
			if stats.Dexterity < 10 {
				return &ValidationResult{
					Valid:   false,
					Message: fmt.Sprintf("Ваша ловкость (%d) недостаточна для этого действия. Требуется минимум 10.", stats.Dexterity),
				}
			}
		}
	}

	return &ValidationResult{Valid: true, Message: ""}
}
