package telegram

import (
	"context"
	"fmt"
	"strings"

	characterapp "dungeons-and-dragons-ai/internal/game/application/character"
	"dungeons-and-dragons-ai/internal/game/domain/character"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (b *Bot) handleCreateCharacter(ctx context.Context, chatID int64, args string) error {
	// Парсим аргументы: /createcharacter имя раса класс
	parts := strings.Fields(args)

	var name string
	var race character.Race
	var class character.Class

	if len(parts) >= 1 && parts[0] != "" {
		name = parts[0]
	} else {
		// Интерактивное создание через кнопки
		return b.showCharacterCreationMenu(ctx, chatID)
	}

	if len(parts) >= 2 {
		race = character.Race(strings.ToLower(parts[1]))
	} else {
		race = character.RaceHuman // по умолчанию
	}

	if len(parts) >= 3 {
		class = character.Class(strings.ToLower(parts[2]))
	} else {
		class = character.ClassFighter // по умолчанию
	}

	// Валидация расы
	validRaces := map[character.Race]bool{
		character.RaceHuman:    true,
		character.RaceElf:      true,
		character.RaceDwarf:    true,
		character.RaceOrc:      true,
		character.RaceHalfling: true,
	}
	if !validRaces[race] {
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Неизвестная раса: %s. Доступные: human, elf, dwarf, orc, halfling", race))
		return b.sendMessage(msg)
	}

	// Валидация класса
	validClasses := map[character.Class]bool{
		character.ClassFighter: true,
		character.ClassWizard:  true,
		character.ClassRogue:   true,
		character.ClassCleric:  true,
		character.ClassRanger:  true,
	}
	if !validClasses[class] {
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Неизвестный класс: %s. Доступные: fighter, wizard, rogue, cleric, ranger", class))
		return b.sendMessage(msg)
	}

	// Создаем персонажа
	req := characterapp.CreateCharacterRequest{
		ChatID: chatID,
		Name:   name,
		Race:   race,
		Class:  class,
	}

	player, err := b.createCharacterUC.Execute(ctx, req)
	if err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при создании персонажа: %v", err))
		return b.sendMessage(errorMsg)
	}

	// Формируем сообщение с информацией о персонаже
	charText := fmt.Sprintf(`✅ Персонаж создан!

👤 Имя: %s
🏛️ Раса: %s
⚔️ Класс: %s
📊 Уровень: %d
❤️ HP: %d/%d

📈 Характеристики:
💪 Сила: %d
🏃 Ловкость: %d
🛡️ Телосложение: %d
🧠 Интеллект: %d
🔮 Мудрость: %d
💬 Харизма: %d`,
		player.Character.Name,
		player.Character.Race,
		player.Character.Class,
		player.Character.Level,
		player.Character.HP,
		player.Character.MaxHP,
		player.Character.Stats.Strength,
		player.Character.Stats.Dexterity,
		player.Character.Stats.Constitution,
		player.Character.Stats.Intelligence,
		player.Character.Stats.Wisdom,
		player.Character.Stats.Charisma,
	)

	return b.sendLongMessage(chatID, charText)
}

func (b *Bot) showCharacterCreationMenu(ctx context.Context, chatID int64) error {
	text := `🎭 Создание персонажа

Используйте команду:
/createcharacter <имя> <раса> <класс>

Пример: /createcharacter Гендальф elf wizard

Или начните создание через кнопки:`

	msg := tgbotapi.NewMessage(chatID, text)

	// Создаем кнопки для выбора расы
	raceButtons := [][]tgbotapi.InlineKeyboardButton{
		{
			tgbotapi.NewInlineKeyboardButtonData("👤 Человек", "race_human"),
			tgbotapi.NewInlineKeyboardButtonData("🧝 Эльф", "race_elf"),
		},
		{
			tgbotapi.NewInlineKeyboardButtonData("⛏️ Дварф", "race_dwarf"),
			tgbotapi.NewInlineKeyboardButtonData("👹 Орк", "race_orc"),
		},
		{
			tgbotapi.NewInlineKeyboardButtonData("🧙 Хоббит", "race_halfling"),
		},
	}

	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(raceButtons...)

	return b.sendMessage(msg)
}
