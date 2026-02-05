package telegram

import (
	"context"
	"fmt"
	"strings"

	"dungeons-and-dragons-ai/internal/game/domain/player"
	"dungeons-and-dragons-ai/internal/game/domain/session"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (b *Bot) handleParty(ctx context.Context, chatID int64, tgUserID int64) error {
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

	// Находим игрока
	player := gs.FindPlayerByTgUserID(tgUserID)
	if player == nil {
		player = gs.GetFirstPlayer()
		if player == nil {
			msg := tgbotapi.NewMessage(chatID, "Персонаж не создан. Используйте /createcharacter для создания персонажа.")
			return b.sendMessage(msg)
		}
	}

	var result strings.Builder
	result.WriteString("👥 Ваш отряд\n\n")

	// Информация об игроке
	result.WriteString(fmt.Sprintf("🎯 **Лидер:** %s (%s %d уровня)\n", player.Character.Name, player.Character.Class, player.Character.Level))
	result.WriteString(fmt.Sprintf("   Здоровье: %d/%d HP\n\n", player.Character.HP, player.Character.MaxHP))

	// Информация о компаньонах
	if len(gs.Companions) > 0 {
		result.WriteString("🛡️ **Компаньоны:**\n")
		for i, companion := range gs.Companions {
			status := "❤️"
			if companion.HP <= 0 {
				status = "💀"
			} else if companion.HP < companion.MaxHP/2 {
				status = "💔"
			}

			result.WriteString(fmt.Sprintf("%d. %s %s (%s %d ур)\n", i+1, status, companion.Name, companion.Class, companion.Level))
			result.WriteString(fmt.Sprintf("   Здоровье: %d/%d HP, Защита: %d\n", companion.HP, companion.MaxHP, companion.AC))
		}
	} else {
		result.WriteString("🛡️ **Компаньоны:** Нет компаньонов\n")
	}

	// Статистика отряда
	totalHP := player.Character.HP
	totalMaxHP := player.Character.MaxHP
	for _, companion := range gs.Companions {
		if companion.HP > 0 {
			totalHP += companion.HP
			totalMaxHP += companion.MaxHP
		}
	}

	result.WriteString(fmt.Sprintf("\n📊 **Общая сила отряда:** %d/%d HP\n", totalHP, totalMaxHP))
	result.WriteString(fmt.Sprintf("👤 **Количество участников:** %d\n", 1+len(gs.Companions)))

	return b.sendLongMessage(chatID, result.String())
}

func (b *Bot) handleDismiss(ctx context.Context, chatID int64, tgUserID int64, args string) error {
	if args == "" {
		msg := tgbotapi.NewMessage(chatID, "Укажите имя компаньона для увольнения. Пример: /dismiss Алара")
		return b.sendMessage(msg)
	}

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

	// Ищем компаньона по имени
	var foundCompanion *session.Companion
	var companionIndex = -1
	for i, companion := range gs.Companions {
		if strings.EqualFold(companion.Name, args) {
			foundCompanion = &gs.Companions[i]
			companionIndex = i
			break
		}
	}

	if foundCompanion == nil {
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Компаньон '%s' не найден в вашем отряде.", args))
		return b.sendMessage(msg)
	}

	// Удаляем компаньона
	gs.Companions = append(gs.Companions[:companionIndex], gs.Companions[companionIndex+1:]...)

	// Сохраняем сессию
	if err := b.sessionRepo.Save(ctx, gs); err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при сохранении сессии: %v", err))
		return b.sendMessage(errorMsg)
	}

	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("✅ Компаньон %s покинул ваш отряд.", foundCompanion.Name))
	return b.sendMessage(msg)
}

func (b *Bot) handlePreferences(ctx context.Context, chatID int64, tgUserID int64) error {
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

	// Находим игрока
	player := gs.FindPlayerByTgUserID(tgUserID)
	if player == nil {
		player = gs.GetFirstPlayer()
		if player == nil {
			msg := tgbotapi.NewMessage(chatID, "Персонаж не создан. Используйте /createcharacter для создания персонажа.")
			return b.sendMessage(msg)
		}
	}

	// Получаем текущие настройки
	prefs := player.GetPreferences()

	var result strings.Builder
	result.WriteString("⚙️ Настройки персонализации\n\n")
	result.WriteString("Текущие настройки:\n")
	result.WriteString(fmt.Sprintf("📖 Стиль повествования: %s\n", getNarrativeStyleName(prefs.NarrativeStyle)))
	result.WriteString(fmt.Sprintf("📝 Уровень детализации: %s\n", getDetailLevelName(prefs.DetailLevel)))
	result.WriteString(fmt.Sprintf("🌐 Язык: %s\n", prefs.Language))
	result.WriteString(fmt.Sprintf("📊 Показывать статистику: %s\n\n", boolToEmoji(prefs.ShowStats)))

	result.WriteString("Для изменения настроек используйте:\n")
	result.WriteString("/set_style <balanced|dark|light|detailed|minimalist>\n")
	result.WriteString("/set_detail <low|medium|high>\n")
	result.WriteString("/set_language <ru|en>\n")
	result.WriteString("/toggle_stats\n")

	return b.sendLongMessage(chatID, result.String())
}

// Вспомогательные функции для отображения настроек
func getNarrativeStyleName(style player.NarrativeStyle) string {
	switch style {
	case player.NarrativeStyleDark:
		return "Темный 🌑"
	case player.NarrativeStyleLight:
		return "Светлый ☀️"
	case player.NarrativeStyleDetailed:
		return "Детализированный 📖"
	case player.NarrativeStyleMinimalist:
		return "Минималистичный 📄"
	default:
		return "Сбалансированный ⚖️"
	}
}

func getDetailLevelName(level player.DetailLevel) string {
	switch level {
	case player.DetailLevelLow:
		return "Низкий 📉"
	case player.DetailLevelHigh:
		return "Высокий 📈"
	default:
		return "Средний 📊"
	}
}

func boolToEmoji(b bool) string {
	if b {
		return "✅"
	}
	return "❌"
}

func (b *Bot) handleSetStyle(ctx context.Context, chatID int64, tgUserID int64, args string) error {
	if args == "" {
		msg := tgbotapi.NewMessage(chatID, "Укажите стиль: balanced, dark, light, detailed, minimalist")
		return b.sendMessage(msg)
	}

	var newStyle player.NarrativeStyle
	switch strings.ToLower(args) {
	case "balanced":
		newStyle = player.NarrativeStyleBalanced
	case "dark":
		newStyle = player.NarrativeStyleDark
	case "light":
		newStyle = player.NarrativeStyleLight
	case "detailed":
		newStyle = player.NarrativeStyleDetailed
	case "minimalist":
		newStyle = player.NarrativeStyleMinimalist
	default:
		msg := tgbotapi.NewMessage(chatID, "Неверный стиль. Используйте: balanced, dark, light, detailed, minimalist")
		return b.sendMessage(msg)
	}

	return b.updatePlayerPreference(ctx, chatID, tgUserID, func(prefs *player.UserPreferences) {
		prefs.NarrativeStyle = newStyle
	}, func(prefs player.UserPreferences) string {
		return fmt.Sprintf("✅ Стиль повествования изменен на: %s", getNarrativeStyleName(newStyle))
	})
}

func (b *Bot) handleSetDetail(ctx context.Context, chatID int64, tgUserID int64, args string) error {
	if args == "" {
		msg := tgbotapi.NewMessage(chatID, "Укажите уровень детализации: low, medium, high")
		return b.sendMessage(msg)
	}

	var newLevel player.DetailLevel
	switch strings.ToLower(args) {
	case "low":
		newLevel = player.DetailLevelLow
	case "medium":
		newLevel = player.DetailLevelMedium
	case "high":
		newLevel = player.DetailLevelHigh
	default:
		msg := tgbotapi.NewMessage(chatID, "Неверный уровень. Используйте: low, medium, high")
		return b.sendMessage(msg)
	}

	return b.updatePlayerPreference(ctx, chatID, tgUserID, func(prefs *player.UserPreferences) {
		prefs.DetailLevel = newLevel
	}, func(prefs player.UserPreferences) string {
		return fmt.Sprintf("✅ Уровень детализации изменен на: %s", getDetailLevelName(newLevel))
	})
}

func (b *Bot) handleSetLanguage(ctx context.Context, chatID int64, tgUserID int64, args string) error {
	if args == "" {
		msg := tgbotapi.NewMessage(chatID, "Укажите язык: ru, en")
		return b.sendMessage(msg)
	}

	language := strings.ToLower(args)
	if language != "ru" && language != "en" {
		msg := tgbotapi.NewMessage(chatID, "Неверный язык. Используйте: ru, en")
		return b.sendMessage(msg)
	}

	return b.updatePlayerPreference(ctx, chatID, tgUserID, func(prefs *player.UserPreferences) {
		prefs.Language = language
	}, func(prefs player.UserPreferences) string {
		return fmt.Sprintf("✅ Язык изменен на: %s", strings.ToUpper(language))
	})
}

func (b *Bot) handleToggleStats(ctx context.Context, chatID int64, tgUserID int64) error {
	return b.updatePlayerPreference(ctx, chatID, tgUserID, func(prefs *player.UserPreferences) {
		prefs.ShowStats = !prefs.ShowStats
	}, func(prefs player.UserPreferences) string {
		return fmt.Sprintf("✅ Отображение статистики: %s", boolToEmoji(prefs.ShowStats))
	})
}

// updatePlayerPreference обновляет настройки игрока
func (b *Bot) updatePlayerPreference(ctx context.Context, chatID int64, tgUserID int64, updateFunc func(*player.UserPreferences), messageFunc func(player.UserPreferences) string) error {
	// Получаем сессию
	gs, err := b.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при получении сессии: %v", err))
		return b.sendMessage(errorMsg)
	}
	if gs == nil {
		msg := tgbotapi.NewMessage(chatID, "Игра не начата.")
		return b.sendMessage(msg)
	}

	// Находим игрока
	player := gs.FindPlayerByTgUserID(tgUserID)
	if player == nil {
		player = gs.GetFirstPlayer()
		if player == nil {
			msg := tgbotapi.NewMessage(chatID, "Персонаж не создан.")
			return b.sendMessage(msg)
		}
	}

	// Получаем текущие настройки
	prefs := player.GetPreferences()

	// Применяем изменения
	updateFunc(&prefs)

	// Сохраняем настройки
	if err := player.SetPreferences(prefs); err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при сохранении настроек: %v", err))
		return b.sendMessage(errorMsg)
	}

	// Сохраняем сессию
	if err := b.sessionRepo.Save(ctx, gs); err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при сохранении сессии: %v", err))
		return b.sendMessage(errorMsg)
	}

	msg := tgbotapi.NewMessage(chatID, messageFunc(prefs))
	return b.sendMessage(msg)
}
