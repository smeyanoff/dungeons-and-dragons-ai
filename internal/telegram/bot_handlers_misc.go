package telegram

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	achievementapp "dungeons-and-dragons-ai/internal/game/application/achievement"
	imageapp "dungeons-and-dragons-ai/internal/game/application/image"
	dm_tools "dungeons-and-dragons-ai/internal/game/application/dm_tools"
	ratingapp "dungeons-and-dragons-ai/internal/game/application/rating"
	sessionapp "dungeons-and-dragons-ai/internal/game/application/session"
	spellapp "dungeons-and-dragons-ai/internal/game/application/spell"
	subscriptionapp "dungeons-and-dragons-ai/internal/game/application/subscription"
	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/combat"
	"dungeons-and-dragons-ai/internal/game/domain/player"
	"dungeons-and-dragons-ai/internal/game/domain/rating"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/game/domain/subscription"
	"dungeons-and-dragons-ai/internal/game/infrastructure/persistence"
	"dungeons-and-dragons-ai/pkg/logger"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

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

// handleSessionSummary обрабатывает команды /summary и /resume — краткое резюме сессии (2–3 предложения: где мы и что дальше).
func (b *Bot) handleSessionSummary(ctx context.Context, chatID int64, tgUserID int64) error {
	if b.sessionRepo == nil {
		msg := tgbotapi.NewMessage(chatID, "Ошибка: репозиторий сессий недоступен.")
		return b.sendMessage(msg)
	}
	gs, err := b.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil {
		logger.Error("Failed to get session for summary", logger.ErrorField(err), logger.Int64("chat_id", chatID))
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка: %v", err))
		return b.sendMessage(errorMsg)
	}
	if gs == nil || !gs.IsActive() {
		msg := tgbotapi.NewMessage(chatID, "Игра не начата. Используйте /newgame для начала новой игры.")
		return b.sendMessage(msg)
	}

	var parts []string
	// Текущая локация
	locName := ""
	if gs.CurrentLocationID != nil {
		for i := range gs.World.Locations {
			if gs.World.Locations[i].ID == *gs.CurrentLocationID {
				locName = gs.World.Locations[i].Name
				break
			}
		}
	}
	if locName != "" {
		parts = append(parts, fmt.Sprintf("Вы находитесь в локации «%s».", locName))
	} else {
		parts = append(parts, "Вы в мире «"+gs.World.Name+"».")
	}
	// Активный бой
	if b.combatRepo != nil {
		activeCombat, _ := b.combatRepo.GetActiveBySessionID(ctx, gs.ID)
		if activeCombat != nil && activeCombat.IsActive() {
			parts = append(parts, "Сейчас идёт бой.")
		}
	}
	// Квест
	if gs.World.MainQuest != nil && strings.TrimSpace(gs.World.MainQuest.Title) != "" {
		parts = append(parts, fmt.Sprintf("Активный квест: %s.", gs.World.MainQuest.Title))
	}
	// Что дальше
	if len(parts) < 3 {
		parts = append(parts, "Напишите, что делаете дальше — DM продолжит историю.")
	}

	summary := strings.Join(parts, " ")
	msg := tgbotapi.NewMessage(chatID, "📋 Резюме сессии:\n\n"+summary)
	return b.sendMessage(msg)
}
