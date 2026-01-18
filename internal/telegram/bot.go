package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"dungeons-and-dragons-ai/internal/game/application/campaign"
	characterapp "dungeons-and-dragons-ai/internal/game/application/character"
	combatapp "dungeons-and-dragons-ai/internal/game/application/combat"
	"dungeons-and-dragons-ai/internal/game/application/dice"
	"dungeons-and-dragons-ai/internal/game/application/history"
	inventoryapp "dungeons-and-dragons-ai/internal/game/application/inventory"
	"dungeons-and-dragons-ai/internal/game/application/player_action"
	questapp "dungeons-and-dragons-ai/internal/game/application/quest"
	mapapp 	"dungeons-and-dragons-ai/internal/game/application/worldmap"
	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/combat"
	"dungeons-and-dragons-ai/internal/game/domain/feedback"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/pkg/logger"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	// TelegramMaxMessageLength максимальная длина сообщения в Telegram
	TelegramMaxMessageLength = 4096
	// TelegramSafeMessageLength безопасная длина для разбиения сообщений (с запасом для форматирования)
	TelegramSafeMessageLength = 4000
)

type Bot struct {
	api               *tgbotapi.BotAPI
	initCampaignUC    *campaign.InitCampaignUseCase
	handleActionUC    *player_action.HandleActionUseCase
	createCharacterUC *characterapp.CreateCharacterUseCase
	getHistoryUC      *history.GetHistoryUseCase
	getInventoryUC    *inventoryapp.GetInventoryUseCase
	addItemUC         *inventoryapp.AddItemUseCase
	handleCombatUC    *combatapp.HandleCombatUseCase
	rollDiceUC        *dice.RollDiceUseCase
	getQuestsUC       *questapp.GetQuestsUseCase
	getMapUC          *mapapp.GetMapUseCase
	sessionRepo       session.Repository
	combatRepo        CombatRepository
	feedbackRepo      FeedbackRepository
}

// FeedbackRepository интерфейс для работы с фидбеком
type FeedbackRepository interface {
	Save(ctx context.Context, fb *feedback.Feedback) error
}

// CombatRepository интерфейс для работы с боем
type CombatRepository interface {
	GetActiveBySessionID(ctx context.Context, sessionID uint) (*combat.Combat, error)
	Save(ctx context.Context, c *combat.Combat) error
}

func NewBot(
	token string,
	initCampaignUC *campaign.InitCampaignUseCase,
	handleActionUC *player_action.HandleActionUseCase,
	createCharacterUC *characterapp.CreateCharacterUseCase,
	getHistoryUC *history.GetHistoryUseCase,
	getInventoryUC *inventoryapp.GetInventoryUseCase,
	addItemUC *inventoryapp.AddItemUseCase,
	handleCombatUC *combatapp.HandleCombatUseCase,
	rollDiceUC *dice.RollDiceUseCase,
	getQuestsUC *questapp.GetQuestsUseCase,
	getMapUC *mapapp.GetMapUseCase,
	sessionRepo session.Repository,
	combatRepo CombatRepository,
	feedbackRepo FeedbackRepository,
) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("failed to create bot: %w", err)
	}

	bot := &Bot{
		api:               api,
		initCampaignUC:    initCampaignUC,
		handleActionUC:    handleActionUC,
		createCharacterUC: createCharacterUC,
		getHistoryUC:      getHistoryUC,
		getInventoryUC:    getInventoryUC,
		addItemUC:         addItemUC,
		handleCombatUC:    handleCombatUC,
		rollDiceUC:        rollDiceUC,
		getQuestsUC:       getQuestsUC,
		getMapUC:          getMapUC,
		sessionRepo:       sessionRepo,
		combatRepo:        combatRepo,
		feedbackRepo:      feedbackRepo,
	}

	// Настраиваем Bot Commands Menu для отображения команд в интерфейсе Telegram
	if err := bot.setupBotCommands(); err != nil {
		logger.Warn("Failed to setup bot commands menu",
			logger.ErrorField(err),
		)
		// Не возвращаем ошибку, так как это не критично для работы бота
	}

	return bot, nil
}

// setupBotCommands настраивает Bot Commands Menu в Telegram
func (b *Bot) setupBotCommands() error {
	commands := []tgbotapi.BotCommand{
		{Command: "start", Description: "Начать работу с ботом"},
		{Command: "help", Description: "Показать справку по командам"},
		{Command: "newgame", Description: "Начать новую игру"},
		{Command: "createcharacter", Description: "Создать персонажа"},
		{Command: "character", Description: "Информация о персонаже"},
		{Command: "inventory", Description: "Посмотреть инвентарь"},
		{Command: "pickup", Description: "Подобрать предмет"},
		{Command: "attack", Description: "Атаковать противника"},
		{Command: "battlefield", Description: "Показать поле боя"},
		{Command: "abilities", Description: "Способности персонажа"},
		{Command: "spells", Description: "Заклинания персонажа"},
		{Command: "roll", Description: "Бросить кубик"},
		{Command: "history", Description: "История игры"},
		{Command: "quests", Description: "Активные квесты"},
		{Command: "map", Description: "Карта мира"},
		{Command: "flee", Description: "Попытаться выйти из боя"},
		{Command: "feedback", Description: "Отправить отзыв о игре"},
		{Command: "endgame", Description: "Завершить игру"},
	}

	cmd := tgbotapi.NewSetMyCommands(commands...)
	_, err := b.api.Request(cmd)
	if err != nil {
		return fmt.Errorf("failed to set bot commands: %w", err)
	}

	logger.Info("Bot commands menu configured successfully",
		logger.Int("commands_count", len(commands)),
	)

	return nil
}

func (b *Bot) Start(ctx context.Context) error {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	logger.Info("Bot started",
		logger.String("username", b.api.Self.UserName),
		logger.Int64("bot_id", int64(b.api.Self.ID)),
	)

	for {
		select {
		case <-ctx.Done():
			logger.Info("Bot stopping",
				logger.ErrorField(ctx.Err()),
			)
			return ctx.Err()
		case update := <-updates:
			if err := b.handleUpdate(ctx, update); err != nil {
				logger.Error("Error handling update",
					logger.ErrorField(err),
					logger.Int("update_id", update.UpdateID),
				)
			}
		}
	}
}

func (b *Bot) handleUpdate(ctx context.Context, update tgbotapi.Update) error {
	// Обработка callback query (кнопки)
	if update.CallbackQuery != nil {
		logger.Debug("Handling callback query",
			logger.String("data", update.CallbackQuery.Data),
			logger.Int64("chat_id", update.CallbackQuery.Message.Chat.ID),
		)
		return b.handleCallbackQuery(ctx, update.CallbackQuery)
	}

	if update.Message == nil {
		return nil
	}

	chatID := update.Message.Chat.ID
	text := update.Message.Text
	userID := update.Message.From.ID

	// Команды
	if update.Message.IsCommand() {
		logger.Info("Handling command",
			logger.String("command", update.Message.Command()),
			logger.String("args", update.Message.CommandArguments()),
			logger.Int64("chat_id", chatID),
			logger.Int64("user_id", int64(userID)),
		)
		// Для команды /feedback передаем также информацию о пользователе для метаданных
		return b.handleCommand(ctx, chatID, update.Message.Command(), update.Message.CommandArguments(), int64(userID), update.Message.From)
	}

	// Обычные сообщения - действия игрока
	logger.Debug("Handling player action",
		logger.Int64("chat_id", chatID),
		logger.Int64("user_id", int64(userID)),
		logger.Int("message_length", len(text)),
	)
	return b.handlePlayerAction(ctx, chatID, text)
}

func (b *Bot) handleCommand(ctx context.Context, chatID int64, command, args string, tgUserID int64, from *tgbotapi.User) error {
	switch command {
	case "start":
		return b.handleStart(ctx, chatID)
	case "help":
		return b.handleHelp(ctx, chatID)
	case "newgame":
		return b.handleNewGame(ctx, chatID, args)
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
	case "map":
		return b.handleMap(ctx, chatID)
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
	case "spells":
		return b.handleSpells(ctx, chatID, args)
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

🎒 Инвентарь и предметы:
/inventory - посмотреть инвентарь
/pickup <предмет> [количество] - подобрать предмет в инвентарь

⚔️ Бой:
/attack - атаковать противника (во время боя)
/battlefield [format] - показать поле боя (format: table/compact/detailed)

🎲 Игра:
/roll <выражение> - бросить кубик (например: /roll d20, /roll 2d6+3)
/history - посмотреть историю игры
/quests - посмотреть активные квесты
/map - посмотреть карту мира
/flee - попытаться выйти из боя (во время боя)
/abilities [filter] - показать способности персонажа (filter: all/spells/feats/class)
/spells - показать заклинания персонажа
/feedback <текст> - отправить отзыв о игре

💡 После начала игры просто пишите мне, что хотите сделать, и я буду описывать что происходит!`

	return b.sendLongMessage(chatID, text)
}

func (b *Bot) handleNewGame(ctx context.Context, chatID int64, theme string) error {
	if theme == "" {
		theme = "классическое фэнтези"
	}

	// Проверяем существующую сессию
	existingSession, err := b.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil {
		return fmt.Errorf("failed to check existing session: %w", err)
	}

	if existingSession != nil {
		if existingSession.IsActive() {
			msg := tgbotapi.NewMessage(chatID, "У вас уже есть активная игра. Используйте /endgame для завершения текущей игры перед началом новой.")
			return b.sendMessage(msg)
		}
		// Если есть завершенная сессия, удаляем её перед созданием новой
		// Это предотвращает нарушение уникального индекса на chat_id
		logger.Info("Removing completed session before creating new one",
			logger.Int64("chat_id", chatID),
			logger.Uint("old_session_id", existingSession.ID),
			logger.String("old_state", string(existingSession.State)),
		)
		if err := b.sessionRepo.Delete(ctx, chatID); err != nil {
			logger.Error("Failed to delete completed session",
				logger.ErrorField(err),
				logger.Int64("chat_id", chatID),
			)
			// Не возвращаем ошибку, пытаемся продолжить создание новой игры
		}
	}

	// Отправляем сообщение о начале генерации
	logger.Info("Starting new game",
		logger.Int64("chat_id", chatID),
		logger.String("theme", theme),
	)
	msg := tgbotapi.NewMessage(chatID, "🎲 Создаю мир... Это может занять несколько секунд.")
	if err := b.sendMessage(msg); err != nil {
		logger.Error("Failed to send message",
			logger.ErrorField(err),
			logger.Int64("chat_id", chatID),
		)
	}

	// Создаём кампанию
	world, err := b.initCampaignUC.Execute(ctx, theme)
	if err != nil {
		logger.Error("Failed to create campaign",
			logger.ErrorField(err),
			logger.Int64("chat_id", chatID),
			logger.String("theme", theme),
		)
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при создании игры: %v", err))
		if sendErr := b.sendMessage(errorMsg); sendErr != nil {
			logger.Error("Failed to send error message",
				logger.ErrorField(sendErr),
				logger.Int64("chat_id", chatID),
			)
		}
		return err
	}
	logger.Info("Campaign created successfully",
		logger.Int64("chat_id", chatID),
		logger.Uint("world_id", world.ID),
		logger.String("world_name", world.Name),
	)

	// Создаём игровую сессию
	gs := &session.GameSession{
		ChatID:  chatID,
		State:   session.StateActive,
		World:   *world,
		WorldID: world.ID,
	}

	if err := b.sessionRepo.Save(ctx, gs); err != nil {
		logger.Error("Failed to save game session",
			logger.ErrorField(err),
			logger.Int64("chat_id", chatID),
			logger.Uint("world_id", world.ID),
		)
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при сохранении сессии: %v", err))
		if sendErr := b.sendMessage(errorMsg); sendErr != nil {
			logger.Error("Failed to send error message",
				logger.ErrorField(sendErr),
				logger.Int64("chat_id", chatID),
			)
		}
		return err
	}
	logger.Info("Game session saved",
		logger.Int64("chat_id", chatID),
		logger.Uint("session_id", gs.ID),
	)

	// Отправляем приветственное сообщение
	welcomeText := fmt.Sprintf(`🎮 Игра начата!

Мир: %s
%s

Главный квест: %s
%s

Используйте команды или просто пишите мне, что хотите сделать!`,
		world.Name,
		world.Description,
		world.MainQuest.Title,
		world.MainQuest.Description)

	return b.sendLongMessage(chatID, welcomeText)
}

func (b *Bot) handlePlayerAction(ctx context.Context, chatID int64, text string) error {
	// Отправляем индикатор печати
	actionMsg := tgbotapi.NewMessage(chatID, "🤔 Думаю...")
	sentMsg, err := b.api.Send(actionMsg)
	indicatorSent := err == nil // Запоминаем, был ли индикатор успешно отправлен

	if err != nil {
		// Если не удалось отправить индикатор, продолжаем выполнение
		// но логируем ошибку для диагностики
		logger.Warn("Failed to send typing indicator",
			logger.ErrorField(err),
			logger.Int64("chat_id", chatID),
		)
	}

	// Получаем ответ от DM
	logger.Debug("Processing player action",
		logger.Int64("chat_id", chatID),
		logger.Int("message_length", len(text)),
	)
	response, err := b.handleActionUC.Execute(ctx, chatID, text)
	
	if err != nil {
		logger.Error("Failed to handle player action",
			logger.ErrorField(err),
			logger.Int64("chat_id", chatID),
		)
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка: %v", err))
		if sendErr := b.sendMessage(errorMsg); sendErr != nil {
			logger.Error("Failed to send error message",
				logger.ErrorField(sendErr),
				logger.Int64("chat_id", chatID),
			)
		}
		return err
	}
	logger.Debug("Player action processed",
		logger.Int64("chat_id", chatID),
		logger.Int("response_length", len(response)),
	)

	// Обновляем сообщение с ответом
	// Если ответ слишком длинный, отправляем новое сообщение вместо редактирования
	if len(response) > TelegramMaxMessageLength {
		// Удаляем индикатор печати, если он был отправлен
		if indicatorSent {
			deleteMsg := tgbotapi.NewDeleteMessage(chatID, sentMsg.MessageID)
			if _, err := b.api.Send(deleteMsg); err != nil {
				logger.Warn("Failed to delete typing indicator",
					logger.ErrorField(err),
					logger.Int64("chat_id", chatID),
				)
			}
		}

		// Отправляем разбитое сообщение
		return b.sendLongMessage(chatID, response)
	}

	// Если индикатор не был отправлен, просто отправляем новое сообщение
	if !indicatorSent {
		return b.sendLongMessage(chatID, response)
	}

	edit := tgbotapi.NewEditMessageText(chatID, sentMsg.MessageID, response)
	return b.editMessage(edit, chatID, response)
}

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

func (b *Bot) handleCallbackQuery(ctx context.Context, query *tgbotapi.CallbackQuery) error {
	chatID := query.Message.Chat.ID
	data := query.Data

	logger.Debug("Handling callback query",
		logger.String("data", data),
		logger.Int64("chat_id", chatID),
		logger.Int64("user_id", query.From.ID),
	)

	// Парсим callback data
	// Формат: race_<race> или class_<race>_<class> или create_<name>_<race>_<class>
	if strings.HasPrefix(data, "race_") {
		// Выбор расы
		race := strings.TrimPrefix(data, "race_")
		return b.handleRaceSelection(ctx, chatID, query, race)
	} else if strings.HasPrefix(data, "class_") {
		// Выбор класса (формат: class_<race>_<class>)
		parts := strings.Split(strings.TrimPrefix(data, "class_"), "_")
		if len(parts) >= 2 {
			race := parts[0]
			class := parts[1]
			return b.handleClassSelection(ctx, chatID, query, race, class)
		}
	} else if strings.HasPrefix(data, "create_") {
		// Завершение создания персонажа (формат: create_<name>_<race>_<class>)
		parts := strings.Split(strings.TrimPrefix(data, "create_"), "_")
		if len(parts) >= 3 {
			name := parts[0]
			race := parts[1]
			class := parts[2]
			return b.handleCreateCharacterFromCallback(ctx, chatID, query, name, race, class)
		}
	}

	// Неизвестный callback
	callback := tgbotapi.NewCallback(query.ID, "Неизвестная команда")
	_, err := b.api.Request(callback)
	return err
}

func (b *Bot) handleRaceSelection(ctx context.Context, chatID int64, query *tgbotapi.CallbackQuery, race string) error {
	// Отвечаем на callback
	callback := tgbotapi.NewCallback(query.ID, fmt.Sprintf("Выбрана раса: %s", race))
	if _, err := b.api.Request(callback); err != nil {
		logger.Error("Failed to answer callback",
			logger.ErrorField(err),
		)
	}

	// Обновляем сообщение с кнопками выбора класса
	text := fmt.Sprintf(`🎭 Создание персонажа

✅ Выбрана раса: %s

Теперь выберите класс:`, race)

	edit := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID, text)

	// Создаем кнопки для выбора класса с указанием расы
	classButtons := [][]tgbotapi.InlineKeyboardButton{
		{
			tgbotapi.NewInlineKeyboardButtonData("⚔️ Воин", fmt.Sprintf("class_%s_fighter", race)),
			tgbotapi.NewInlineKeyboardButtonData("🔮 Маг", fmt.Sprintf("class_%s_wizard", race)),
		},
		{
			tgbotapi.NewInlineKeyboardButtonData("🗡️ Вор", fmt.Sprintf("class_%s_rogue", race)),
			tgbotapi.NewInlineKeyboardButtonData("✨ Жрец", fmt.Sprintf("class_%s_cleric", race)),
		},
		{
			tgbotapi.NewInlineKeyboardButtonData("🏹 Следопыт", fmt.Sprintf("class_%s_ranger", race)),
		},
		{
			tgbotapi.NewInlineKeyboardButtonData("⬅️ Назад", "race_human"), // Кнопка возврата
		},
	}

	edit.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: classButtons}
	return b.editMessage(edit, chatID, text)
}

func (b *Bot) handleClassSelection(ctx context.Context, chatID int64, query *tgbotapi.CallbackQuery, race, class string) error {
	// Отвечаем на callback
	callback := tgbotapi.NewCallback(query.ID, fmt.Sprintf("Выбран класс: %s", class))
	if _, err := b.api.Request(callback); err != nil {
		logger.Error("Failed to answer callback",
			logger.ErrorField(err),
		)
	}

	// Запрашиваем имя персонажа
	text := fmt.Sprintf(`🎭 Создание персонажа

✅ Раса: %s
✅ Класс: %s

📝 Теперь введите имя персонажа текстовым сообщением, или используйте команду:
/createcharacter <имя> %s %s

Пример: /createcharacter Гендальф %s %s`, race, class, race, class, race, class)

	edit := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID, text)

	// Используем username как дефолтное имя
	defaultName := query.From.UserName
	if defaultName == "" {
		defaultName = query.From.FirstName
	}
	if defaultName == "" {
		defaultName = "Герой"
	}

	// Кнопка для создания с дефолтным именем
	buttons := [][]tgbotapi.InlineKeyboardButton{
		{
			tgbotapi.NewInlineKeyboardButtonData(
				fmt.Sprintf("✅ Создать с именем '%s'", defaultName),
				fmt.Sprintf("create_%s_%s_%s", defaultName, race, class),
			),
		},
		{
			tgbotapi.NewInlineKeyboardButtonData("⬅️ Назад к выбору расы", "race_human"),
		},
	}

	edit.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: buttons}
	_, err := b.api.Send(edit)
	return err
}

func (b *Bot) handleCreateCharacterFromCallback(ctx context.Context, chatID int64, query *tgbotapi.CallbackQuery, name, raceStr, classStr string) error {
	// Отвечаем на callback
	callback := tgbotapi.NewCallback(query.ID, "Создаю персонажа...")
	if _, err := b.api.Request(callback); err != nil {
		logger.Error("Failed to answer callback",
			logger.ErrorField(err),
		)
	}

	// Парсим расу и класс
	race := character.Race(strings.ToLower(raceStr))
	class := character.Class(strings.ToLower(classStr))

	// Валидация расы
	validRaces := map[character.Race]bool{
		character.RaceHuman:    true,
		character.RaceElf:      true,
		character.RaceDwarf:    true,
		character.RaceOrc:      true,
		character.RaceHalfling: true,
	}
	if !validRaces[race] {
		errorMsg := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID,
			fmt.Sprintf("❌ Ошибка: Неизвестная раса: %s", race))
		return b.editMessage(errorMsg, chatID, fmt.Sprintf("❌ Ошибка: Неизвестная раса: %s", race))
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
		errorMsg := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID,
			fmt.Sprintf("❌ Ошибка: Неизвестный класс: %s", class))
		return b.editMessage(errorMsg, chatID, fmt.Sprintf("❌ Ошибка: Неизвестный класс: %s", class))
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
		errorMsg := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID,
			fmt.Sprintf("❌ Ошибка при создании персонажа: %v", err))
		if sendErr := b.editMessage(errorMsg, chatID, fmt.Sprintf("❌ Ошибка при создании персонажа: %v", err)); sendErr != nil {
			return sendErr
		}
		return err
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

	// Обновляем сообщение с результатом
	resultMsg := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID, charText)
	resultMsg.ReplyMarkup = nil // Убираем кнопки

	return b.editMessage(resultMsg, chatID, charText)
}

func (b *Bot) handleShowCharacter(ctx context.Context, chatID int64, tgUserID int64) error {
	// Получаем сессию
	gs, err := b.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}

	if gs == nil {
		msg := tgbotapi.NewMessage(chatID, "Игра не начата. Используйте /newgame для начала новой игры.")
		return b.sendMessage(msg)
	}

	// Ищем игрока по TgUserID (для приватных чатов chatID == tgUserID)
	// В групповых чатах это позволит найти правильного игрока
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

	charText := fmt.Sprintf(`👤 Персонаж: %s

🏛️ Раса: %s
⚔️ Класс: %s
📊 Уровень: %d
⭐ Опыт: %d / %d (до следующего уровня: %d)
❤️ HP: %d/%d
💀 Статус: %s

📈 Характеристики:
💪 Сила: %d
🏃 Ловкость: %d
🛡️ Телосложение: %d
🧠 Интеллект: %d
🔮 Мудрость: %d
💬 Харизма: %d`,
		char.Name,
		char.Race,
		char.Class,
		char.Level,
		char.Experience,
		char.GetExperienceToNextLevel()+char.Experience,
		expToNext,
		char.HP,
		char.MaxHP,
		char.Status,
		char.Stats.Strength,
		char.Stats.Dexterity,
		char.Stats.Constitution,
		char.Stats.Intelligence,
		char.Stats.Wisdom,
		char.Stats.Charisma,
	)

	return b.sendLongMessage(chatID, charText)
}

func (b *Bot) handleHistory(ctx context.Context, chatID int64, args string) error {
	limit := 10 // по умолчанию последние 10 событий
	if args != "" {
		// Можно добавить парсинг лимита из args
		limit = 10
	}

	historyText, err := b.getHistoryUC.Execute(ctx, chatID, limit)
	if err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при получении истории: %v", err))
		return b.sendMessage(errorMsg)
	}

	return b.sendLongMessage(chatID, historyText)
}

func (b *Bot) handleInventory(ctx context.Context, chatID int64, tgUserID int64) error {
	inventoryText, err := b.getInventoryUC.Execute(ctx, chatID, tgUserID)
	if err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при получении инвентаря: %v", err))
		return b.sendMessage(errorMsg)
	}

	return b.sendLongMessage(chatID, inventoryText)
}

func (b *Bot) handlePickup(ctx context.Context, chatID int64, args string, tgUserID int64) error {
	// Парсим аргументы: /pickup <предмет> [количество]
	parts := strings.Fields(args)
	
	if len(parts) == 0 {
		msg := tgbotapi.NewMessage(chatID, "Укажите название предмета. Формат: /pickup <предмет> [количество]\n\nПример: /pickup меч\nПример: /pickup стрела 5")
		return b.sendMessage(msg)
	}
	
	// Пытаемся определить количество (последняя часть)
	quantity := 1
	itemName := ""
	
	if len(parts) > 1 {
		// Проверяем, является ли последняя часть числом
		if qty, err := strconv.Atoi(parts[len(parts)-1]); err == nil && qty > 0 {
			// Последняя часть - количество
			quantity = qty
			// Имя - все части кроме последней
			itemName = strings.Join(parts[:len(parts)-1], " ")
		} else {
			// Последняя часть не число - все части это название предмета
			itemName = strings.Join(parts, " ")
		}
	} else {
		// Только одна часть - это название предмета
		itemName = parts[0]
	}
	
	if itemName == "" {
		msg := tgbotapi.NewMessage(chatID, "Укажите название предмета. Формат: /pickup <предмет> [количество]")
		return b.sendMessage(msg)
	}
	
	req := inventoryapp.AddItemRequest{
		ChatID:    chatID,
		TgUserID:  tgUserID,
		ItemName:  itemName,
		Quantity:  quantity,
		ItemType:  "", // Определяется автоматически по названию
	}
	
	result, err := b.addItemUC.Execute(ctx, req)
	if err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при добавлении предмета: %v", err))
		return b.sendMessage(errorMsg)
	}
	
	return b.sendLongMessage(chatID, result)
}

func (b *Bot) handleRoll(ctx context.Context, chatID int64, args string) error {
	result, err := b.rollDiceUC.Execute(ctx, args)
	if err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при броске кубика: %v\n\nИспользуйте формат: /roll d20 или /roll 2d6+3", err))
		return b.sendMessage(errorMsg)
	}

	return b.sendLongMessage(chatID, result)
}

func (b *Bot) handleQuests(ctx context.Context, chatID int64) error {
	questsText, err := b.getQuestsUC.Execute(ctx, chatID)
	if err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при получении квестов: %v", err))
		return b.sendMessage(errorMsg)
	}

	return b.sendLongMessage(chatID, questsText)
}

func (b *Bot) handleMap(ctx context.Context, chatID int64) error {
	mapText, err := b.getMapUC.Execute(ctx, chatID)
	if err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при получении карты: %v", err))
		return b.sendMessage(errorMsg)
	}

	return b.sendLongMessage(chatID, mapText)
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

	// Используем handleActionUC, который автоматически вызовет DM tool для получения поля боя
	// DM tool get_battlefield_status будет вызван через handleActionUC
	battlefieldMessage := fmt.Sprintf("Покажи поле боя в формате %s", format)
	result, err := b.handleActionUC.Execute(ctx, chatID, battlefieldMessage)
	if err != nil {
		logger.Error("Failed to get battlefield status",
			logger.ErrorField(err),
			logger.Int64("chat_id", chatID),
		)
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка: %v\n\nИспользуйте: /battlefield [table|compact|detailed]", err))
		return b.sendMessage(errorMsg)
	}

	return b.sendLongMessage(chatID, result)
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

	// Используем handleActionUC, который автоматически вызовет DM tool для получения способностей
	abilitiesMessage := fmt.Sprintf("Покажи способности персонажа, фильтр: %s", filterType)
	result, err := b.handleActionUC.Execute(ctx, chatID, abilitiesMessage)
	if err != nil {
		logger.Error("Failed to get abilities",
			logger.ErrorField(err),
			logger.Int64("chat_id", chatID),
		)
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка: %v\n\nИспользуйте: /abilities [all|spells|feats|class]", err))
		return b.sendMessage(errorMsg)
	}

	return b.sendLongMessage(chatID, result)
}

// handleSpells обрабатывает команду /spells для отображения заклинаний персонажа
func (b *Bot) handleSpells(ctx context.Context, chatID int64, args string) error {
	// Используем handleAbilities с фильтром "spells"
	return b.handleAbilities(ctx, chatID, "spells")
}

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

func (b *Bot) handleFeedback(ctx context.Context, chatID int64, args string, tgUserID int64, from *tgbotapi.User) error {
	// Проверяем, что текст фидбека указан
	feedbackText := strings.TrimSpace(args)
	if feedbackText == "" {
		msg := tgbotapi.NewMessage(chatID, "Пожалуйста, укажите ваш отзыв. Формат: /feedback <текст отзыва>\n\nПример: /feedback Отличная игра! Очень интересный DM.")
		return b.sendMessage(msg)
	}

	if b.feedbackRepo == nil {
		msg := tgbotapi.NewMessage(chatID, "Извините, система фидбека временно недоступна.")
		return b.sendMessage(msg)
	}

	// Создаем фидбек
	fb := &feedback.Feedback{
		ChatID:  chatID,
		UserID:  tgUserID,
		Message: feedbackText,
	}

	// Добавляем метаданные пользователя, если доступны
	if from != nil {
		fb.UserFirstName = from.FirstName
		fb.UserLastName = from.LastName
		fb.UserUsername = from.UserName
	}

	// Сохраняем фидбек
	if err := b.feedbackRepo.Save(ctx, fb); err != nil {
		logger.Error("Failed to save feedback",
			logger.ErrorField(err),
			logger.Int64("chat_id", chatID),
			logger.Int64("user_id", tgUserID),
		)
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при сохранении отзыва: %v", err))
		return b.sendMessage(errorMsg)
	}

	logger.Info("Feedback saved",
		logger.Int64("chat_id", chatID),
		logger.Int64("user_id", tgUserID),
		logger.Uint("feedback_id", fb.ID),
	)

	// Отправляем подтверждение пользователю
	msg := tgbotapi.NewMessage(chatID, "✅ Спасибо за ваш отзыв! Он поможет нам улучшить игру. 🎲")
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

// sendMessage отправляет сообщение с проверкой ошибок и логированием
func (b *Bot) sendMessage(msg tgbotapi.MessageConfig) error {
	_, err := b.api.Send(msg)
	if err != nil {
		logger.Error("Failed to send message",
			logger.ErrorField(err),
			logger.Int64("chat_id", msg.ChatID),
		)
		return err
	}
	return nil
}

// sendLongMessage разбивает длинные сообщения на части и отправляет их
// Telegram имеет лимит 4096 символов на сообщение
func (b *Bot) sendLongMessage(chatID int64, text string) error {
	if len(text) <= TelegramMaxMessageLength {
		msg := tgbotapi.NewMessage(chatID, text)
		return b.sendMessage(msg)
	}

	// Разбиваем на части по безопасной длине
	parts := splitMessage(text, TelegramSafeMessageLength)

	var lastErr error
	for i, part := range parts {
		msg := tgbotapi.NewMessage(chatID, part)
		if len(parts) > 1 {
			// Добавляем индикатор части для многочастных сообщений
			indicator := fmt.Sprintf("(%d/%d)\n", i+1, len(parts))
			// Проверяем, что индикатор не превышает лимит вместе с частью
			if len(indicator)+len(part) > TelegramMaxMessageLength {
				// Если индикатор слишком длинный, уменьшаем часть
				maxPartLen := TelegramMaxMessageLength - len(indicator)
				if maxPartLen > 0 {
					part = part[:maxPartLen]
				} else {
					// Если индикатор сам по себе превышает лимит, отправляем без него
					indicator = ""
				}
			}
			msg.Text = indicator + part
		}
		if err := b.sendMessage(msg); err != nil {
			lastErr = err
			logger.Error("Failed to send message part",
				logger.ErrorField(err),
				logger.Int64("chat_id", chatID),
				logger.Int("part", i+1),
				logger.Int("total_parts", len(parts)),
			)
		}
	}

	return lastErr
}

// splitMessage разбивает текст на части заданной максимальной длины
// Старается разбивать по предложениям или словам
func splitMessage(text string, maxLen int) []string {
	if len(text) <= maxLen {
		return []string{text}
	}

	var parts []string
	current := ""

	// Разбиваем по строкам
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		// Если добавление строки превысит лимит, сохраняем текущую часть
		if len(current)+len(line)+1 > maxLen && current != "" {
			parts = append(parts, current)
			current = ""
		}

		if len(line) > maxLen {
			// Строка слишком длинная, разбиваем по словам
			words := strings.Fields(line)
			if len(words) == 0 {
				// Строка без пробелов - принудительно разбиваем по maxLen
				for i := 0; i < len(line); i += maxLen {
					end := i + maxLen
					if end > len(line) {
						end = len(line)
					}
					parts = append(parts, line[i:end])
				}
				current = ""
			} else {
				for _, word := range words {
					// Если слово само по себе превышает лимит, разбиваем его принудительно
					if len(word) > maxLen {
						// Сохраняем текущую часть, если есть
						if current != "" {
							parts = append(parts, current)
							current = ""
						}
						// Разбиваем длинное слово на части
						for i := 0; i < len(word); i += maxLen {
							end := i + maxLen
							if end > len(word) {
								end = len(word)
							}
							parts = append(parts, word[i:end])
						}
					} else {
						if len(current)+len(word)+1 > maxLen && current != "" {
							parts = append(parts, current)
							current = ""
						}
						if current != "" {
							current += " "
						}
						current += word
					}
				}
			}
		} else {
			if current != "" {
				current += "\n"
			}
			current += line
		}
	}

	if current != "" {
		parts = append(parts, current)
	}

	return parts
}

// editMessage редактирует сообщение, обрабатывая ошибку "message is not modified" как не критичную
func (b *Bot) editMessage(edit tgbotapi.EditMessageTextConfig, chatID int64, fallbackText string) error {
	_, err := b.api.Send(edit)
	if err != nil {
		// Проверяем, является ли это ошибкой "message is not modified"
		// Это нормальное поведение при повторных нажатиях на кнопки
		errStr := err.Error()
		if strings.Contains(errStr, "message is not modified") ||
			strings.Contains(errStr, "message_not_modified") {
			// Это не критичная ошибка, логируем на уровне DEBUG
			logger.Debug("Message is not modified (expected behavior)",
				logger.String("error", errStr),
				logger.Int64("chat_id", chatID),
			)
			return nil // Возвращаем nil, так как это ожидаемое поведение
		}

		// Для других ошибок логируем на уровне WARN и отправляем новое сообщение
		logger.Warn("Failed to edit message",
			logger.ErrorField(err),
			logger.Int64("chat_id", chatID),
		)
		// Если редактирование не удалось, отправляем новое сообщение
		return b.sendLongMessage(chatID, fallbackText)
	}
	return nil
}
