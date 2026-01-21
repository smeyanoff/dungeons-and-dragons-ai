package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	mapapp "dungeons-and-dragons-ai/internal/game/application/worldmap"
	"dungeons-and-dragons-ai/internal/game/infrastructure/persistence"
	telegrambot "dungeons-and-dragons-ai/internal/telegram"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type tgCapturedCall struct {
	Method      string
	ChatID      int64
	MessageID   int
	Text        string
	ReplyMarkup string
}

type fakeTelegramAPI struct {
	mu           sync.Mutex
	nextMsgID    int
	calls        []tgCapturedCall
	lastMessages map[int64]int // chat_id -> last message id
}

func newFakeTelegramAPI() *fakeTelegramAPI {
	return &fakeTelegramAPI{
		nextMsgID:    1,
		lastMessages: map[int64]int{},
	}
}

func (f *fakeTelegramAPI) handler(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		// Path format: /bot<TOKEN>/<method>
		path := strings.TrimPrefix(r.URL.Path, "/")
		parts := strings.Split(path, "/")
		method := ""
		if len(parts) >= 2 {
			method = parts[1]
		}
		if method == "" {
			http.Error(w, "bad path", http.StatusBadRequest)
			return
		}

		_ = r.ParseMultipartForm(32 << 20) // ok for both form & multipart

		get := func(key string) string {
			if r.MultipartForm != nil && r.MultipartForm.Value != nil {
				if v := r.MultipartForm.Value[key]; len(v) > 0 {
					return v[0]
				}
			}
			if v := r.FormValue(key); v != "" {
				return v
			}
			return ""
		}

		var chatID int64
		if chatIDStr := get("chat_id"); chatIDStr != "" {
			if parsed, err := strconv.ParseInt(chatIDStr, 10, 64); err == nil {
				chatID = parsed
			}
		}
		text := get("text")
		replyMarkup := get("reply_markup")

		var messageID int
		if midStr := get("message_id"); midStr != "" {
			if parsed, err := strconv.Atoi(midStr); err == nil {
				messageID = parsed
			}
		}

		f.mu.Lock()
		defer f.mu.Unlock()

		switch method {
		case "getMe":
			f.writeJSON(w, map[string]any{
				"ok": true,
				"result": map[string]any{
					"id":         999001,
					"is_bot":     true,
					"first_name": "TestBot",
					"username":   "test_bot",
				},
			})
			return
		case "setMyCommands", "answerCallbackQuery", "deleteMessage":
			// Minimal OK response.
			f.writeJSON(w, map[string]any{"ok": true, "result": true})
			return
		case "sendMessage":
			f.nextMsgID++
			messageID = f.nextMsgID
			f.lastMessages[chatID] = messageID
			f.calls = append(f.calls, tgCapturedCall{
				Method:      method,
				ChatID:      chatID,
				MessageID:   messageID,
				Text:        text,
				ReplyMarkup: replyMarkup,
			})

			f.writeJSON(w, map[string]any{
				"ok": true,
				"result": map[string]any{
					"message_id": messageID,
					"chat":       map[string]any{"id": chatID, "type": "private"},
					"text":       text,
				},
			})
			return
		case "editMessageText":
			// editMessageText uses message_id from the request; keep it as-is.
			if messageID == 0 {
				messageID = f.lastMessages[chatID]
			}
			f.calls = append(f.calls, tgCapturedCall{
				Method:    method,
				ChatID:    chatID,
				MessageID: messageID,
				Text:      text,
			})
			f.writeJSON(w, map[string]any{
				"ok": true,
				"result": map[string]any{
					"message_id": messageID,
					"chat":       map[string]any{"id": chatID, "type": "private"},
					"text":       text,
				},
			})
			return
		case "sendPhoto":
			// We don't validate file upload; just acknowledge.
			f.nextMsgID++
			messageID = f.nextMsgID
			f.lastMessages[chatID] = messageID
			f.calls = append(f.calls, tgCapturedCall{
				Method:    method,
				ChatID:    chatID,
				MessageID: messageID,
				Text:      get("caption"),
			})
			f.writeJSON(w, map[string]any{
				"ok": true,
				"result": map[string]any{
					"message_id": messageID,
					"chat":       map[string]any{"id": chatID, "type": "private"},
				},
			})
			return
		default:
			// Unknown method - still respond OK to not make bot brittle.
			f.calls = append(f.calls, tgCapturedCall{
				Method:      method,
				ChatID:      chatID,
				MessageID:   messageID,
				Text:        text,
				ReplyMarkup: replyMarkup,
			})
			f.writeJSON(w, map[string]any{"ok": true, "result": true})
			return
		}
	}
}

func (f *fakeTelegramAPI) writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (f *fakeTelegramAPI) snapshotCalls() []tgCapturedCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]tgCapturedCall, len(f.calls))
	copy(out, f.calls)
	return out
}

func makeMessageUpdate(chatID, tgUserID int64, text string) tgbotapi.Update {
	return tgbotapi.Update{
		UpdateID: int(time.Now().UnixNano() % 1_000_000),
		Message: &tgbotapi.Message{
			MessageID: int(time.Now().UnixNano() % 1_000_000),
			Chat:      &tgbotapi.Chat{ID: chatID, Type: "private"},
			From:      &tgbotapi.User{ID: tgUserID, UserName: "tester"},
			Text:      text,
		},
	}
}

func makeCallbackUpdate(chatID, tgUserID int64, messageID int, data string) tgbotapi.Update {
	return tgbotapi.Update{
		UpdateID: int(time.Now().UnixNano() % 1_000_000),
		CallbackQuery: &tgbotapi.CallbackQuery{
			ID:   fmt.Sprintf("cb_%d", time.Now().UnixNano()),
			Data: data,
			From: &tgbotapi.User{ID: tgUserID, UserName: "tester"},
			Message: &tgbotapi.Message{
				MessageID: messageID,
				Chat:      &tgbotapi.Chat{ID: chatID, Type: "private"},
			},
		},
	}
}

func extractFirstCallbackData(replyMarkup string) (string, bool) {
	if strings.TrimSpace(replyMarkup) == "" {
		return "", false
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(replyMarkup), &raw); err != nil {
		return "", false
	}
	rows, ok := raw["inline_keyboard"].([]any)
	if !ok {
		return "", false
	}
	for _, row := range rows {
		btns, ok := row.([]any)
		if !ok {
			continue
		}
		for _, b := range btns {
			btn, ok := b.(map[string]any)
			if !ok {
				continue
			}
			if cd, ok := btn["callback_data"].(string); ok && cd != "" {
				return cd, true
			}
		}
	}
	return "", false
}

func findToolLeak(calls []tgCapturedCall, chatID int64) string {
	for _, call := range calls {
		if call.ChatID != chatID {
			continue
		}
		if call.Method != "sendMessage" && call.Method != "editMessageText" {
			continue
		}
		text := strings.TrimSpace(call.Text)
		if text == "" || text == "🤔 Думаю..." {
			continue
		}
		if strings.Contains(text, "<tool_call") ||
			strings.Contains(text, "request_ability_check") ||
			strings.Contains(text, "evaluate_check") ||
			strings.Contains(text, "\"tool_calls\"") ||
			strings.Contains(text, "tool_call") ||
			strings.Contains(text, "already_checked") {
			return text
		}
	}
	return ""
}

// TestTelegramBotSimulation_UserJourney прогоняет основной пользовательский сценарий "как в Telegram":
// команды, сообщения игрока, callbacks (навигация по карте).
func TestTelegramGameplay_BotSimulation_UserJourney(t *testing.T) {
	cfg := setupTelegramGameplayTest(t)
	defer cleanupTest(t, cfg.testConfig)

	ctx := cfg.ctx
	chatID := cfg.chatID
	tgUserID := cfg.tgUserID

	var problems []string
	var llmFeedback []string

	// Fake Telegram API server
	fakeAPI := newFakeTelegramAPI()
	srv := httptest.NewServer(fakeAPI.handler(t))
	defer srv.Close()

	apiEndpointFmt := strings.TrimRight(srv.URL, "/") + "/bot%s/%s"

	// Repos for bot-only features
	combatRepo := persistence.NewCombatRepository(cfg.db)
	feedbackRepo := persistence.NewFeedbackRepository(cfg.db)
	worldEventRepo := persistence.NewWorldEventRepository(cfg.db)
	moveToLocationUC := mapapp.NewMoveToLocationUseCase(cfg.sessionRepo, worldEventRepo, nil, nil)

	// IMPORTANT: to avoid extra (costly) model calls, we pass generateImageUC=nil and indexDocUC=nil in bot.
	bot, err := telegrambot.NewBotWithAPIEndpoint(
		"TEST_TOKEN",
		apiEndpointFmt,
		cfg.initCampaignUC,
		cfg.handleActionUC,
		cfg.createCharacterUC,
		cfg.getHistoryUC,
		cfg.getInventoryUC,
		cfg.addItemUC,
		cfg.handleCombatUC,
		cfg.rollDiceUC,
		cfg.getQuestsUC,
		cfg.getDailyQuestsUC,
		cfg.checkDailyProgressUC,
		cfg.getMapUC,
		moveToLocationUC,
		cfg.getAchievementsUC,
		cfg.getSpellsUC,
		cfg.useSpellUC,
		nil, // generateImageUC
		nil, // getSubscriptionUC
		nil, // checkLimitsUC
		nil, // getLeaderboardUC
		nil, // updateRatingUC
		nil, // performAbilityCheckUC
		cfg.sessionRepo,
		combatRepo,
		feedbackRepo,
		nil, // eventRepo (skip extra DB writes from /roll)
		nil, // indexDocUC (skip embeddings calls from /roll)
	)
	if err != nil {
		t.Fatalf("Не удалось создать Telegram bot (fake API): %v", err)
	}

	// 0) /help
	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/help")); err != nil {
		problems = append(problems, fmt.Sprintf("/help: %v", err))
		t.Errorf("/help: %v", err)
	}

	// 1) /newgame (LLM)
	if err := cfg.waitForRateLimit(ctx); err != nil {
		problems = append(problems, fmt.Sprintf("Ошибка rate limiter (before /newgame): %v", err))
	}
	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/newgame классическое фэнтези")); err != nil {
		problems = append(problems, fmt.Sprintf("/newgame: %v", err))
		t.Errorf("/newgame: %v", err)
	}

	gs, err := cfg.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil || gs == nil || !gs.IsActive() {
		problems = append(problems, fmt.Sprintf("После /newgame не найдена активная сессия: err=%v, session_nil=%v", err, gs == nil))
		t.Errorf("После /newgame не найдена активная сессия: err=%v session=%v", err, gs)
	}
	if gs != nil && gs.CurrentLocationID == nil {
		// Это критично для /map + navigation callbacks.
		problems = append(problems, "После /newgame не установлена CurrentLocationID (навигация по карте может не работать)")
	}
	hasConnections := false
	if gs != nil {
		for _, loc := range gs.World.Locations {
			if len(loc.Connections) > 0 {
				hasConnections = true
				break
			}
		}
	}

	// 2) /createcharacter
	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/createcharacter ТестовыйГерой elf wizard")); err != nil {
		problems = append(problems, fmt.Sprintf("/createcharacter: %v", err))
		t.Errorf("/createcharacter: %v", err)
	}

	gs, _ = cfg.sessionRepo.GetByChatID(ctx, chatID)
	if gs == nil || gs.GetFirstPlayer() == nil || gs.GetFirstPlayer().Character.Name == "" {
		problems = append(problems, "После /createcharacter не найден персонаж в сессии")
		t.Errorf("После /createcharacter не найден персонаж в сессии")
	}

	// 3) Player action (LLM)
	if err := cfg.waitForRateLimit(ctx); err != nil {
		problems = append(problems, fmt.Sprintf("Ошибка rate limiter (before player action): %v", err))
	}
	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "Осматриваю комнату и ищу следы недавней драки")); err != nil {
		problems = append(problems, fmt.Sprintf("player action: %v", err))
		t.Errorf("player action: %v", err)
	}

	// Heuristic feedback: DM response should be meaningful.
	calls := fakeAPI.snapshotCalls()
	var dmText string
	for i := len(calls) - 1; i >= 0; i-- {
		if calls[i].Method == "editMessageText" && strings.TrimSpace(calls[i].Text) != "" && calls[i].Text != "🤔 Думаю..." {
			dmText = calls[i].Text
			break
		}
	}
	if dmText != "" && len([]rune(dmText)) < 80 {
		llmFeedback = append(llmFeedback, fmt.Sprintf("Ответ DM слишком короткий (%d символов): %s", len([]rune(dmText)), dmText))
	}

	// 4) /inventory
	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/inventory")); err != nil {
		problems = append(problems, fmt.Sprintf("/inventory: %v", err))
		t.Errorf("/inventory: %v", err)
	}

	// 5) /map + callback navigation
	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/map")); err != nil {
		problems = append(problems, fmt.Sprintf("/map: %v", err))
		t.Errorf("/map: %v", err)
	}

	// Find nav message with inline keyboard.
	calls = fakeAPI.snapshotCalls()
	navMsgID := 0
	navReplyMarkup := ""
	for _, c := range calls {
		if c.Method == "sendMessage" && strings.Contains(c.Text, "Куда идём дальше") && strings.TrimSpace(c.ReplyMarkup) != "" {
			navMsgID = c.MessageID
			navReplyMarkup = c.ReplyMarkup
		}
	}
	cbData, ok := extractFirstCallbackData(navReplyMarkup)
	if hasConnections && (navMsgID == 0 || !ok) {
		problems = append(problems, "После /map не удалось найти сообщение с inline-кнопками навигации (map_to_*)")
	} else if navMsgID != 0 && ok {
		if err := bot.HandleUpdate(ctx, makeCallbackUpdate(chatID, tgUserID, navMsgID, cbData)); err != nil {
			problems = append(problems, fmt.Sprintf("map navigation callback (%s): %v", cbData, err))
			t.Errorf("map navigation callback: %v", err)
		} else {
			// Check that we edited nav message with move confirmation.
			calls = fakeAPI.snapshotCalls()
			moved := false
			for _, c := range calls {
				if c.Method == "editMessageText" && c.MessageID == navMsgID && strings.Contains(c.Text, "Вы переместились") {
					moved = true
					break
				}
			}
			if !moved {
				problems = append(problems, "Навигация по карте не отредактировала сообщение с подтверждением перемещения (ожидали 'Вы переместились')")
			}
		}
	}

	// 6) /daily + /quests + /history + /roll + /endgame
	for _, cmd := range []string{"/daily", "/quests", "/history", "/roll d20", "/endgame"} {
		if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, cmd)); err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", cmd, err))
			t.Errorf("%s: %v", cmd, err)
		}
	}

	if len(problems) > 0 {
		writeToTestingReport(problems)
	}

	// Проверяем, что в player-facing ответах нет технических маркеров tools/JSON.
	if leak := findToolLeak(fakeAPI.snapshotCalls(), chatID); leak != "" {
		t.Fatalf("Обнаружен tool-текст в player-facing сообщении: %s", leak)
	}
	if len(llmFeedback) > 0 {
		writeToFeedback(llmFeedback)
	}
}
