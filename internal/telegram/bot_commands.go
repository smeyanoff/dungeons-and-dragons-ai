package telegram

import (
	"context"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// isKnownCommand проверяет, является ли строка известной командой
func (b *Bot) isKnownCommand(command string) bool {
	knownCommands := map[string]bool{
		"start":           true,
		"help":            true,
		"newgame":         true,
		"endgame":         true,
		"createcharacter": true,
		"character":       true,
		"history":         true,
		"inventory":       true,
		"roll":            true,
		"quests":          true,
		"daily":           true,
		"map":             true,
		"achievements":    true,
		"party":           true,
		"dismiss":         true,
		"preferences":     true,
		"set_style":       true,
		"set_detail":      true,
		"set_language":    true,
		"toggle_stats":    true,
		"leaderboard":     true,
		"spells":          true,
		"cast":            true,
		"image":           true,
		"autoimage":       true,
		"subscribe":       true,
		"subscription":    true,
		"flee":            true,
		"run":             true,
		"feedback":        true,
		"attack":          true,
		"pickup":          true,
		"battlefield":     true,
		"abilities":       true,
		"wait_until":      true,
		"time":            true,
		"progress":        true,
		"move_to_location": true,
		"cooperative":     true,
		"join":            true,
		"leave":           true,
		"coopstatus":      true,
		"summary":         true,
		"resume":          true,
	}
	return knownCommands[strings.ToLower(command)]
}

func (b *Bot) handleCommand(ctx context.Context, chatID int64, command, args string, tgUserID int64, from *tgbotapi.User) error {
	switch command {
	case "start":
		return b.handleStart(ctx, chatID)
	case "help":
		return b.handleHelp(ctx, chatID)
	case "newgame":
		return b.handleNewGame(ctx, chatID, tgUserID, args)
	case "endgame":
		return b.handleEndGame(ctx, chatID)
	case "createcharacter":
		return b.handleCreateCharacter(ctx, chatID, args)
	case "character":
		return b.handleShowCharacter(ctx, chatID, tgUserID)
	case "history":
		return b.handleHistory(ctx, chatID, args)
	case "inventory":
		return b.handleInventory(ctx, chatID, tgUserID)
	case "roll":
		return b.handleRoll(ctx, chatID, args)
	case "quests":
		return b.handleQuests(ctx, chatID)
	case "daily":
		return b.handleDaily(ctx, chatID, tgUserID)
	case "map":
		return b.handleMap(ctx, chatID)
	case "achievements":
		return b.handleAchievements(ctx, chatID, tgUserID)
	case "party":
		return b.handleParty(ctx, chatID, tgUserID)
	case "dismiss":
		return b.handleDismiss(ctx, chatID, tgUserID, args)
	case "preferences":
		return b.handlePreferences(ctx, chatID, tgUserID)
	case "set_style":
		return b.handleSetStyle(ctx, chatID, tgUserID, args)
	case "set_detail":
		return b.handleSetDetail(ctx, chatID, tgUserID, args)
	case "set_language":
		return b.handleSetLanguage(ctx, chatID, tgUserID, args)
	case "toggle_stats":
		return b.handleToggleStats(ctx, chatID, tgUserID)
	case "leaderboard":
		return b.handleLeaderboard(ctx, chatID, tgUserID, args)
	case "spells":
		return b.handleSpells(ctx, chatID, tgUserID)
	case "cast":
		return b.handleCast(ctx, chatID, tgUserID, args)
	case "image":
		return b.handleImage(ctx, chatID, tgUserID, args)
	case "autoimage":
		return b.handleToggleAutoImage(ctx, chatID, tgUserID, args)
	case "subscribe":
		return b.handleSubscribe(ctx, chatID, tgUserID, args)
	case "subscription":
		return b.handleSubscription(ctx, chatID, tgUserID)
	case "flee", "run":
		return b.handleFlee(ctx, chatID)
	case "feedback":
		return b.handleFeedback(ctx, chatID, args, tgUserID, from)
	case "attack":
		return b.handleAttack(ctx, chatID, args)
	case "pickup":
		return b.handlePickup(ctx, chatID, args, tgUserID)
	case "battlefield":
		return b.handleBattlefield(ctx, chatID, args)
	case "abilities":
		return b.handleAbilities(ctx, chatID, args)
	case "wait_until", "time":
		return b.handleWaitUntil(ctx, chatID, args)
	case "progress":
		return b.handleProgress(ctx, chatID, tgUserID)
	case "move_to_location":
		return b.handleMoveToLocationCommand(ctx, chatID, args, tgUserID)
	case "cooperative":
		return b.handleCooperative(ctx, chatID, tgUserID, args)
	case "join":
		return b.handleJoin(ctx, chatID, tgUserID)
	case "leave":
		return b.handleLeave(ctx, chatID, tgUserID)
	case "coopstatus":
		return b.handleCoopStatus(ctx, chatID)
	case "summary", "resume":
		return b.handleSessionSummary(ctx, chatID, tgUserID)
	default:
		msg := tgbotapi.NewMessage(chatID, "Неизвестная команда. Используйте /help для списка команд")
		return b.sendMessage(msg)
	}
}

func (b *Bot) handleStart(ctx context.Context, chatID int64) error {
	return b.handleHelp(ctx, chatID)
}

func (b *Bot) handleHelp(ctx context.Context, chatID int64) error {
	text := `🎲 Добро пожаловать в Dungeons & Dragons AI!

Я ваш Dungeon Master. Используйте команды:

🎮 Основные команды:
/newgame <тема> - начать новую игру
/endgame - завершить текущую игру
/help - показать эту справку

👤 Персонаж:
/createcharacter - создать персонажа (интерактивно или через команду)
/character - посмотреть информацию о персонаже
/progress - посмотреть прогресс и статистику персонажа

🎒 Инвентарь и предметы:
/inventory - посмотреть инвентарь
/pickup <предмет> [количество] - подобрать предмет в инвентарь

⚔️ Бой:
/attack - атаковать противника (во время боя)
/battlefield [format] - показать поле боя (format: table/compact/detailed)

🎲 Игра:
/roll <выражение> - бросить кубик (например: /roll d20, /roll 2d6+3)
/summary или /resume - краткое резюме сессии (где мы и что дальше)
/history - посмотреть историю игры
/quests - посмотреть активные квесты
/map - посмотреть карту мира
/wait_until <время> - изменить время суток (morning/noon/afternoon/evening/night/midnight)
/flee - попытаться выйти из боя (во время боя)
/abilities [filter] - показать способности персонажа (filter: all/spells/feats/class)
/spells - показать заклинания персонажа
/cast <название> [цель] - использовать заклинание (например: /cast Огненный снаряд)
/feedback <текст> - отправить отзыв о игре

👥 Отряд:
/party - посмотреть состав вашего отряда (лидер + компаньоны)
/dismiss <имя> - уволить компаньона из отряда (например: /dismiss Алара)

⚙️ Персонализация:
/preferences - посмотреть текущие настройки стиля повествования
/set_style <стиль> - изменить стиль (balanced/dark/light/detailed/minimalist)
/set_detail <уровень> - изменить детализация (low/medium/high)
/set_language <язык> - изменить язык (ru/en)
/toggle_stats - переключить отображение статистики

💡 После начала игры просто пишите мне, что хотите сделать, и я буду описывать что происходит!`

	return b.sendLongMessage(chatID, text)
}
