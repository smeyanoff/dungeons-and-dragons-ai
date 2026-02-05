package telegram

import (
	"context"
	"fmt"
	"strings"
	"time"

	imageapp "dungeons-and-dragons-ai/internal/game/application/image"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/pkg/logger"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (b *Bot) handleNewGame(ctx context.Context, chatID int64, tgUserID int64, theme string) error {
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
	llmCtx := context.WithValue(ctx, "chat_id", chatID)
	llmCtx = context.WithValue(llmCtx, "tg_user_id", tgUserID)
	world, err := b.initCampaignUC.Execute(llmCtx, theme)
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
		if isDuplicateChatIDError(err) {
			// На всякий случай проверяем, не существует ли активная сессия, и пробуем очистить завершенную.
			existing, getErr := b.sessionRepo.GetByChatID(ctx, chatID)
			if getErr == nil && existing != nil && existing.IsActive() {
				msg := tgbotapi.NewMessage(chatID, "У вас уже есть активная игра. Используйте /endgame для завершения текущей игры перед началом новой.")
				return b.sendMessage(msg)
			}
			if delErr := b.sessionRepo.Delete(ctx, chatID); delErr != nil {
				logger.Error("Failed to delete existing session after duplicate key error",
					logger.ErrorField(delErr),
					logger.Int64("chat_id", chatID),
				)
			} else {
				logger.Info("Deleted existing session after duplicate key error, retrying save",
					logger.Int64("chat_id", chatID),
				)
				if retryErr := b.sessionRepo.Save(ctx, gs); retryErr == nil {
					goto sessionSaved
				} else {
					err = retryErr
				}
			}
		}

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
sessionSaved:
	logger.Info("Game session saved",
		logger.Int64("chat_id", chatID),
		logger.Uint("session_id", gs.ID),
	)

	// Генерируем цели для сессии
	gs.GenerateSessionGoals()
	if err := b.sessionRepo.Save(ctx, gs); err != nil {
		logger.Error("Failed to save session goals",
			logger.ErrorField(err),
			logger.Int64("chat_id", chatID),
		)
		// Не возвращаем ошибку, продолжаем игру без целей
	}

	// Инициализируем текущую локацию (по умолчанию первая локация мира)
	// Это нужно для /map и навигации по кнопкам
	if gs.CurrentLocationID == nil && len(gs.World.Locations) > 0 {
		firstID := gs.World.Locations[0].ID
		if firstID != 0 {
			gs.CurrentLocationID = &firstID
			if err := b.sessionRepo.Save(ctx, gs); err != nil {
				logger.Warn("Failed to set initial current location",
					logger.ErrorField(err),
					logger.Int64("chat_id", chatID),
				)
			}
		}
	}

	// Генерируем красивую карту-изображение мира (если доступна генерация изображений)
	// Сохраняем путь в сессии, чтобы /map мог показать картинку
	if b.generateImageUC != nil && gs.MapImagePath == "" && len(gs.World.Locations) > 0 {
		var sb strings.Builder
		sb.WriteString("Fantasy world map, top-down, detailed, colored, parchment style, Dungeons & Dragons.\n")
		sb.WriteString(fmt.Sprintf("World: %s.\n", gs.World.Name))
		if gs.World.Description != "" {
			sb.WriteString(fmt.Sprintf("World description: %s.\n", gs.World.Description))
		}
		sb.WriteString("Locations:\n")
		for _, loc := range gs.World.Locations {
			sb.WriteString(fmt.Sprintf("- %s\n", loc.Name))
		}
		sb.WriteString("Connections (direction -> destination):\n")
		// Строим карту ID->name для удобного отображения связей
		locNameByID := map[uint]string{}
		for _, loc := range gs.World.Locations {
			locNameByID[loc.ID] = loc.Name
		}
		for _, loc := range gs.World.Locations {
			for _, conn := range loc.Connections {
				toName := locNameByID[conn.ToLocationID]
				if toName == "" {
					toName = fmt.Sprintf("Location #%d", conn.ToLocationID)
				}
				sb.WriteString(fmt.Sprintf("- %s: %s -> %s\n", loc.Name, conn.Direction, toName))
			}
		}

		imgCtx, cancel := context.WithTimeout(ctx, 120*time.Second) // увеличиваем таймаут для изображений с учетом retry
		defer cancel()
		req := imageapp.GenerateImageRequest{
			SystemPrompt:    "You are a fantasy cartographer. Create beautiful D&D-style maps with clear landmarks and readable layout. No text labels on the map itself.",
			UserPrompt:      sb.String(),
			Type:            "custom",
			EntityID:        gs.WorldID,
			ForceRegenerate: false,
			UserID:          0,
			SkipLimitCheck:  true,
		}
		resp, err := b.generateImageUC.Execute(imgCtx, req)
		if err != nil {
			logger.Warn("Failed to generate world map image",
				logger.ErrorField(err),
				logger.Int64("chat_id", chatID),
				logger.Uint("world_id", gs.WorldID),
			)
		} else if resp != nil && resp.ImagePath != "" {
			gs.MapImagePath = resp.ImagePath
			if err := b.sessionRepo.Save(ctx, gs); err != nil {
				logger.Warn("Failed to save map image path to session",
					logger.ErrorField(err),
					logger.Int64("chat_id", chatID),
				)
			}
		}
	}

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

func (b *Bot) handlePlayerAction(ctx context.Context, chatID int64, tgUserID int64, text string) error {
	// Отправляем индикатор печати
	actionMsg := tgbotapi.NewMessage(chatID, "🤔 Думаю...")
	actionMsg.Text = sanitizeTelegramOutput(actionMsg.Text, chatID, "typingIndicator")
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
		logger.Int64("tg_user_id", tgUserID),
		logger.Int("message_length", len(text)),
	)
	response, err := b.handleActionUC.Execute(ctx, chatID, tgUserID, text)

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

	// Проверяем наличие маркеров изображений в ответе и отправляем их
	imagePaths := b.extractImageMarkers(response)
	if len(imagePaths) > 0 {
		logger.Info("Sending generated images",
			logger.Int64("chat_id", chatID),
			logger.Int("image_count", len(imagePaths)),
		)
		for _, imagePath := range imagePaths {
			// Удаляем маркер из текста перед отправкой
			response = strings.ReplaceAll(response, fmt.Sprintf("[IMAGE:%s]", imagePath), "")
			// Отправляем изображение
			if err := b.sendPhoto(ctx, chatID, imagePath, ""); err != nil {
				logger.Warn("Failed to send generated image",
					logger.ErrorField(err),
					logger.Int64("chat_id", chatID),
					logger.String("image_path", imagePath),
				)
				// Не прерываем выполнение, просто логируем ошибку
			}
		}
		// Очищаем пробелы и переносы строк после удаления маркеров
		response = strings.TrimSpace(response)
	}

	// Обновляем сообщение с ответом
	// Если ответ слишком длинный, отправляем новое сообщение вместо редактирования
	var sendErr error
	if len(response) > TelegramMaxMessageLength {
		// Удаляем индикатор печати, если он был отправлен
		if indicatorSent {
			deleteMsg := tgbotapi.NewDeleteMessage(chatID, sentMsg.MessageID)
			if _, err := b.api.Request(deleteMsg); err != nil {
				logger.Warn("Failed to delete typing indicator",
					logger.ErrorField(err),
					logger.Int64("chat_id", chatID),
				)
			}
		}
		// Отправляем разбитое сообщение
		sendErr = b.sendLongMessage(chatID, response)
	} else if !indicatorSent {
		// Если индикатор не был отправлен, просто отправляем новое сообщение
		sendErr = b.sendLongMessage(chatID, response)
	} else {
		edit := tgbotapi.NewEditMessageText(chatID, sentMsg.MessageID, response)
		sendErr = b.editMessage(edit, chatID, response)
	}

	promptErr := b.maybeSendPendingAbilityCheckPrompt(ctx, chatID)
	if sendErr != nil {
		return sendErr
	}
	return promptErr
}

