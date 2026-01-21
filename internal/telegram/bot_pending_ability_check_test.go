package telegram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"dungeons-and-dragons-ai/internal/game/domain/session"
)

type mockSessionRepo struct {
	mu       sync.Mutex
	sessions map[int64]*session.GameSession
	saves    int
}

func (m *mockSessionRepo) GetByChatID(ctx context.Context, chatID int64) (*session.GameSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[chatID], nil
}

func (m *mockSessionRepo) Save(ctx context.Context, gs *session.GameSession) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sessions == nil {
		m.sessions = make(map[int64]*session.GameSession)
	}
	m.sessions[gs.ChatID] = gs
	m.saves++
	return nil
}

func (m *mockSessionRepo) Delete(ctx context.Context, chatID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, chatID)
	return nil
}

type tgCall struct {
	Method      string
	ChatID      int64
	Text        string
	ReplyMarkup string
}

type fakeTelegramAPI struct {
	mu    sync.Mutex
	calls []tgCall
}

func (f *fakeTelegramAPI) handler(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
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

		_ = r.ParseMultipartForm(32 << 20)
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
		case "setMyCommands":
			f.writeJSON(w, map[string]any{"ok": true, "result": true})
			return
		case "sendMessage":
			f.mu.Lock()
			f.calls = append(f.calls, tgCall{
				Method:      method,
				ChatID:      chatID,
				Text:        get("text"),
				ReplyMarkup: get("reply_markup"),
			})
			f.mu.Unlock()
			f.writeJSON(w, map[string]any{
				"ok": true,
				"result": map[string]any{
					"message_id": 1,
					"chat":       map[string]any{"id": chatID, "type": "private"},
					"text":       get("text"),
				},
			})
			return
		default:
			f.writeJSON(w, map[string]any{"ok": true, "result": true})
			return
		}
	}
}

func (f *fakeTelegramAPI) writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (f *fakeTelegramAPI) snapshotCalls() []tgCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]tgCall, len(f.calls))
	copy(out, f.calls)
	return out
}

func TestMaybeSendPendingAbilityCheckPrompt_SendsOnceAndMarksNotified(t *testing.T) {
	ctx := context.Background()
	chatID := int64(1001)
	checkID := "check_ability_1"

	repo := &mockSessionRepo{
		sessions: map[int64]*session.GameSession{
			chatID: {
				ChatID: chatID,
				State:  session.StateActive,
			},
		},
	}

	gs, _ := repo.GetByChatID(ctx, chatID)
	gs.SetPendingAbilityCheck(checkID, "wisdom", 12)
	if err := repo.Save(ctx, gs); err != nil {
		t.Fatalf("Save session: %v", err)
	}

	fakeAPI := &fakeTelegramAPI{}
	srv := httptest.NewServer(fakeAPI.handler(t))
	t.Cleanup(srv.Close)
	apiEndpointFmt := strings.TrimRight(srv.URL, "/") + "/bot%s/%s"

	bot, err := NewBotWithAPIEndpoint(
		"TEST_TOKEN",
		apiEndpointFmt,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		repo,
		nil,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewBotWithAPIEndpoint: %v", err)
	}

	if err := bot.maybeSendPendingAbilityCheckPrompt(ctx, chatID); err != nil {
		t.Fatalf("maybeSendPendingAbilityCheckPrompt: %v", err)
	}

	calls := fakeAPI.snapshotCalls()
	found := false
	for _, call := range calls {
		if call.Method == "sendMessage" && call.ChatID == chatID && strings.Contains(call.Text, "🎲 Проверка") {
			if !strings.Contains(call.ReplyMarkup, "ability_roll_"+checkID) {
				t.Fatalf("reply_markup does not include ability_roll_%s: %s", checkID, call.ReplyMarkup)
			}
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected pending ability check prompt to be sent")
	}

	gs, _ = repo.GetByChatID(ctx, chatID)
	if gs == nil || !gs.PendingAbilityCheckNotified {
		t.Fatalf("expected pending ability check to be marked as notified")
	}

	callCount := len(fakeAPI.snapshotCalls())
	if err := bot.maybeSendPendingAbilityCheckPrompt(ctx, chatID); err != nil {
		t.Fatalf("second maybeSendPendingAbilityCheckPrompt: %v", err)
	}
	if len(fakeAPI.snapshotCalls()) != callCount {
		t.Fatalf("expected no additional sendMessage after notified")
	}
}
