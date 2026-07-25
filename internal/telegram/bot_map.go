package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	mapapp "dungeons-and-dragons-ai/internal/game/application/worldmap"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/game/domain/world"
	"dungeons-and-dragons-ai/pkg/logger"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (b *Bot) handleMap(ctx context.Context, chatID int64) error {
	// Получаем сессию (нужно для текущей локации и картинки карты)
	gs, err := b.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при получении сессии: %v", err))
		return b.sendMessage(errorMsg)
	}
	if gs == nil {
		msg := tgbotapi.NewMessage(chatID, "Игра не начата. Используйте /newgame для начала новой игры.")
		return b.sendMessage(msg)
	}

	// Если есть красивая карта-изображение — показываем её
	if gs.MapImagePath != "" {
		if err := b.sendPhoto(ctx, chatID, gs.MapImagePath, "🗺️ Карта мира"); err != nil {
			logger.Warn("Failed to send map image",
				logger.ErrorField(err),
				logger.Int64("chat_id", chatID),
			)
		}
	}

	mapText, err := b.getMapUC.Execute(ctx, chatID)
	if err != nil {
		errorMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ошибка при получении карты: %v", err))
		return b.sendMessage(errorMsg)
	}

	// 1) Отправляем текст карты (может быть длинным)
	if err := b.sendLongMessage(chatID, mapText); err != nil {
		return err
	}

	// 2) Отправляем кнопки навигации отдельным коротким сообщением
	markup := b.buildMapNavigationKeyboard(gs)
	if markup == nil {
		return nil
	}
	navMsg := tgbotapi.NewMessage(chatID, "🧭 Куда идём дальше?")
	navMsg.ReplyMarkup = markup
	return b.sendMessage(navMsg)
}

func (b *Bot) buildMapNavigationKeyboard(gs *session.GameSession) *tgbotapi.InlineKeyboardMarkup {
	if gs == nil || len(gs.World.Locations) == 0 {
		return nil
	}

	// Быстрый доступ к локациям
	locationMap := make(map[uint]*world.Location, len(gs.World.Locations))
	for i := range gs.World.Locations {
		loc := &gs.World.Locations[i]
		locationMap[loc.ID] = loc
	}

	// Текущая локация
	var current *world.Location
	if gs.CurrentLocationID != nil {
		current = locationMap[*gs.CurrentLocationID]
	}
	if current == nil || len(current.Connections) == 0 {
		// Если текущая локация не задана или без связей, ищем первую с доступными связями
		for i := range gs.World.Locations {
			if len(gs.World.Locations[i].Connections) > 0 {
				current = &gs.World.Locations[i]
				break
			}
		}
	}
	if current == nil || len(current.Connections) == 0 {
		return nil
	}

	var rows [][]tgbotapi.InlineKeyboardButton
	var row []tgbotapi.InlineKeyboardButton

	// Кнопки по направлениям
	for _, conn := range current.Connections {
		toName := "???"
		visitedMark := ""
		if toLoc := locationMap[conn.ToLocationID]; toLoc != nil {
			toName = toLoc.Name
			if toLoc.Visited {
				visitedMark = "📍 "
			}
		}
		dirSym := mapDirectionSymbol(conn.Direction)
		btnText := fmt.Sprintf("%s %s%s", dirSym, visitedMark, toName)
		btnData := fmt.Sprintf("map_to_%d", conn.ToLocationID)
		row = append(row, tgbotapi.NewInlineKeyboardButtonData(btnText, btnData))
		if len(row) == 2 {
			rows = append(rows, row)
			row = nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}

	m := tgbotapi.NewInlineKeyboardMarkup(rows...)
	return &m
}

func mapDirectionSymbol(direction string) string {
	switch strings.ToLower(direction) {
	case "north", "n":
		return "⬆️"
	case "south", "s":
		return "⬇️"
	case "east", "e":
		return "➡️"
	case "west", "w":
		return "⬅️"
	case "up", "u":
		return "⬆️⬆️"
	case "down", "d":
		return "⬇️⬇️"
	case "portal":
		return "🌀"
	case "path", "road":
		return "🛤️"
	default:
		return "→"
	}
}

func (b *Bot) handleMoveToLocationCommand(ctx context.Context, chatID int64, args string, tgUserID int64) error {
	args = strings.TrimSpace(args)
	if args == "" {
		msg := tgbotapi.NewMessage(chatID, "Использование: /move_to_location <id_локации>\nПример: /move_to_location 2")
		return b.sendMessage(msg)
	}

	if b.moveToLocationUC == nil {
		return b.sendMessage(tgbotapi.NewMessage(chatID, "Навигация по карте временно недоступна."))
	}

	locID, err := strconv.ParseUint(args, 10, 64)
	if err != nil || locID == 0 {
		msg := tgbotapi.NewMessage(chatID, "Некорректный id локации. Пример: /move_to_location 2")
		return b.sendMessage(msg)
	}

	gsForContext, _ := b.sessionRepo.GetByChatID(ctx, chatID)
	ctxWithIDs := context.WithValue(ctx, "chat_id", chatID)
	if tgUserID != 0 {
		ctxWithIDs = context.WithValue(ctxWithIDs, "tg_user_id", tgUserID)
	}
	if gsForContext != nil {
		ctxWithIDs = context.WithValue(ctxWithIDs, "session_id", gsForContext.ID)
	}

	toLocationID := uint(locID)
	resp, err := b.moveToLocationUC.Execute(ctxWithIDs, mapapp.MoveToLocationRequest{
		ChatID:       chatID,
		ToLocationID: &toLocationID,
	})
	if err != nil {
		return b.sendMessage(tgbotapi.NewMessage(chatID, fmt.Sprintf("Не удалось переместиться: %v", err)))
	}

	if err := b.sendLongMessage(chatID, resp.Message); err != nil {
		return err
	}

	gs, _ := b.sessionRepo.GetByChatID(ctx, chatID)
	if markup := b.buildMapNavigationKeyboard(gs); markup != nil {
		navMsg := tgbotapi.NewMessage(chatID, "🧭 Куда идём дальше?")
		navMsg.ReplyMarkup = markup
		if err := b.sendMessage(navMsg); err != nil {
			return err
		}
	}

	return nil
}
