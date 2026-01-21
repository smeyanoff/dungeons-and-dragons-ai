package inventory

import (
	"context"
	"fmt"

	"dungeons-and-dragons-ai/internal/game/domain/inventory"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/pkg/logger"
)

// AddItemUseCase обрабатывает явное добавление предмета в инвентарь игроком
type AddItemUseCase struct {
	sessionRepo   session.Repository
	inventoryRepo InventoryRepositoryWithSave
}

// InventoryRepositoryWithSave расширяет InventoryRepository для сохранения
type InventoryRepositoryWithSave interface {
	InventoryRepository
	Save(ctx context.Context, inv *inventory.Inventory) error
}

// NewAddItemUseCase создает новый use case для добавления предмета
func NewAddItemUseCase(
	sessionRepo session.Repository,
	inventoryRepo InventoryRepositoryWithSave,
) *AddItemUseCase {
	return &AddItemUseCase{
		sessionRepo:   sessionRepo,
		inventoryRepo: inventoryRepo,
	}
}

// AddItemRequest запрос на добавление предмета
type AddItemRequest struct {
	ChatID   int64
	TgUserID int64
	ItemName string
	Quantity int
	ItemType inventory.ItemType
}

// Execute добавляет предмет в инвентарь персонажа
func (uc *AddItemUseCase) Execute(ctx context.Context, req AddItemRequest) (string, error) {
	// Получаем сессию
	gs, err := uc.sessionRepo.GetByChatID(ctx, req.ChatID)
	if err != nil {
		return "", fmt.Errorf("failed to get session: %w", err)
	}

	if gs == nil {
		return "Игра не начата. Используйте /newgame для начала новой игры.", nil
	}

	// Находим игрока
	player := gs.FindPlayerByTgUserID(req.TgUserID)
	if player == nil {
		player = gs.GetFirstPlayer()
	}

	if player == nil {
		return "Персонаж не создан. Используйте /createcharacter для создания персонажа.", nil
	}

	if player.CharacterID == 0 {
		return "Персонаж не создан. Используйте /createcharacter для создания персонажа.", nil
	}

	// Валидация названия предмета
	itemName := req.ItemName
	if itemName == "" {
		return "Укажите название предмета. Формат: /pickup <название> [количество]", nil
	}

	// Определяем количество
	quantity := req.Quantity
	if quantity <= 0 {
		quantity = 1
	}

	// Определяем тип предмета
	itemType := req.ItemType
	if itemType == "" {
		// Пытаемся определить тип по названию
		itemType = detectItemType(itemName)
	}

	// Получаем инвентарь
	inv, err := uc.inventoryRepo.GetByCharacterID(ctx, player.CharacterID)
	if err != nil {
		return "", fmt.Errorf("failed to get inventory: %w", err)
	}

	// Определяем вес по типу предмета
	weight := getWeightByType(itemType)

	// Определяем описание
	description := fmt.Sprintf("Предмет типа %s", itemType)

	// Добавляем предмет
	if err := inv.AddItem(itemName, description, weight, quantity, itemType); err != nil {
		if err.Error() == "инвентарь переполнен" {
			return fmt.Sprintf("❌ Инвентарь переполнен! Невозможно добавить %s (x%d).\n\n💼 Текущий вес: %.2f кг / %.0f кг",
				itemName, quantity, inv.GetTotalWeight(), inventory.MaxWeight), nil
		}
		return "", fmt.Errorf("failed to add item: %w", err)
	}

	// Сохраняем инвентарь
	if err := uc.inventoryRepo.Save(ctx, inv); err != nil {
		return "", fmt.Errorf("failed to save inventory: %w", err)
	}

	logger.Info("Item added to inventory",
		logger.Int64("chat_id", req.ChatID),
		logger.Uint("character_id", player.CharacterID),
		logger.String("item_name", itemName),
		logger.Int("quantity", quantity),
		logger.String("item_type", string(itemType)),
	)

	// Формируем ответ
	message := fmt.Sprintf(`✅ Предмет добавлен в инвентарь!

📦 %s (x%d)
📊 Тип: %s
⚖️ Вес: %.2f кг

💼 Инвентарь: %.2f кг / %.0f кг`,
		itemName, quantity, itemType, weight*float64(quantity), inv.GetTotalWeight(), inventory.MaxWeight)

	return message, nil
}

// detectItemType пытается определить тип предмета по названию
func detectItemType(itemName string) inventory.ItemType {
	name := " " + itemName + " "

	// Оружие
	weaponKeywords := []string{" меч ", " кинжал ", " лук ", " стрела ", " арбалет ", " булава ", " топор ", " копье ", " оружие "}
	for _, keyword := range weaponKeywords {
		if contains(name, keyword) {
			return inventory.ItemTypeWeapon
		}
	}

	// Броня
	armorKeywords := []string{" доспех ", " броня ", " щит ", " шлем ", " латы ", " кольчуга ", " кожа "}
	for _, keyword := range armorKeywords {
		if contains(name, keyword) {
			return inventory.ItemTypeArmor
		}
	}

	// Зелье
	potionKeywords := []string{" зелье ", " эликсир ", " отвар ", " напиток ", " флакон "}
	for _, keyword := range potionKeywords {
		if contains(name, keyword) {
			return inventory.ItemTypePotion
		}
	}

	// Инструмент
	toolKeywords := []string{" ключ ", " инструмент ", " веревка ", " факел ", " кирка ", " лопата "}
	for _, keyword := range toolKeywords {
		if contains(name, keyword) {
			return inventory.ItemTypeTool
		}
	}

	// Расходник
	consumableKeywords := []string{" еда ", " хлеб ", " яблоко ", " мясо ", " рыба ", " вода "}
	for _, keyword := range consumableKeywords {
		if contains(name, keyword) {
			return inventory.ItemTypeConsumable
		}
	}

	// По умолчанию - разное
	return inventory.ItemTypeMisc
}

// getWeightByType возвращает вес по типу предмета
func getWeightByType(itemType inventory.ItemType) float64 {
	switch itemType {
	case inventory.ItemTypeWeapon:
		return 2.0
	case inventory.ItemTypeArmor:
		return 5.0
	case inventory.ItemTypePotion:
		return 0.5
	case inventory.ItemTypeTool:
		return 1.5
	case inventory.ItemTypeConsumable:
		return 0.3
	default:
		return 1.0
	}
}

// contains проверяет, содержит ли строка подстроку (case-insensitive)
func contains(s, substr string) bool {
	// Простая проверка с учетом регистра для русского языка
	sLower := toLower(s)
	substrLower := toLower(substr)
	return containsStr(sLower, substrLower)
}

// toLower преобразует строку к нижнему регистру (использует strings.ToLower)
func toLower(s string) string {
	// Используем стандартную библиотеку для корректной работы с UTF-8
	result := make([]rune, 0, len(s))
	for _, r := range s {
		if r >= 'А' && r <= 'Я' {
			// Русские заглавные буквы (А-Я) -> строчные (а-я)
			result = append(result, r+32)
		} else if r == 'Ё' {
			// Ё -> ё
			result = append(result, 'ё')
		} else if r >= 'A' && r <= 'Z' {
			// Английские заглавные буквы (A-Z) -> строчные (a-z)
			result = append(result, r+32)
		} else {
			result = append(result, r)
		}
	}
	return string(result)
}

// containsStr проверяет, содержит ли строка подстроку
func containsStr(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
