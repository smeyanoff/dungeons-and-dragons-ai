package integration

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	characterapp "dungeons-and-dragons-ai/internal/game/application/character"
	"dungeons-and-dragons-ai/internal/game/application/dice"
	dmtools "dungeons-and-dragons-ai/internal/game/application/dm_tools"
	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/event"
	questdomain "dungeons-and-dragons-ai/internal/game/domain/quest"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	worlddomain "dungeons-and-dragons-ai/internal/game/domain/world"
	telegrambot "dungeons-and-dragons-ai/internal/telegram"
)

func TestAbilityCheck_Guardrails_AlreadyPending(t *testing.T) {
	cfg := setupInfraOnlyIntegrationTest(t)
	if cfg == nil {
		return
	}
	defer cleanupTest(t, &testConfig{db: cfg.db})

	ctx := cfg.ctx
	chatID := cfg.chatID

	// Prepare deterministic world + session + character
	q := &questdomain.Quest{Title: "Test Quest (Guardrails)", Description: "Test quest for guardrails"}
	w := worlddomain.New("Test World (Guardrails)")
	w.Description = "Deterministic test world for ability check guardrails"
	w.SetMainQuest(q)
	w.Locations = []worlddomain.Location{{Name: "Start", Description: "Start location"}}
	if err := cfg.worldRepo.Save(ctx, w); err != nil {
		t.Fatalf("Не удалось сохранить тестовый мир: %v", err)
	}
	gs := &session.GameSession{ChatID: chatID, State: session.StateActive, World: *w, WorldID: w.ID}
	if err := cfg.sessionRepo.Save(ctx, gs); err != nil {
		t.Fatalf("Не удалось сохранить сессию: %v", err)
	}
	createCharacterUC := characterapp.NewCreateCharacterUseCase(cfg.sessionRepo, cfg.playerRepo)
	if _, err := createCharacterUC.Execute(ctx, characterapp.CreateCharacterRequest{
		ChatID: chatID, Name: "Герой", Race: character.RaceHuman, Class: character.ClassFighter,
	}); err != nil {
		t.Fatalf("Не удалось создать персонажа: %v", err)
	}

	// Set existing pending check
	gs, _ = cfg.sessionRepo.GetByChatID(ctx, chatID)
	gs.SetPendingAbilityCheck("pending_1", "strength", 12)
	if err := cfg.sessionRepo.Save(ctx, gs); err != nil {
		t.Fatalf("Не удалось сохранить pending check: %v", err)
	}

	tool := dmtools.NewRequestAbilityCheckTool(cfg.sessionRepo, cfg.eventRepo, chatID)
	out, err := tool.Execute(ctx, map[string]interface{}{
		"ability": "wisdom",
		"dc":      10,
		"reason":  "пытается вспомнить детали",
		"stakes":  "низкие ставки: просто дополнительная информация",
	})
	if err != nil {
		t.Fatalf("tool.Execute: %v", err)
	}
	m, ok := out.(map[string]interface{})
	if !ok {
		t.Fatalf("ожидали map[string]interface{}, получили %T", out)
	}
	if v, ok := m["already_pending"].(bool); !ok || !v {
		t.Fatalf("ожидали already_pending=true, получили: %#v", m["already_pending"])
	}

	// Must not overwrite existing pending check
	gs, _ = cfg.sessionRepo.GetByChatID(ctx, chatID)
	if gs.PendingAbilityCheckID != "pending_1" || gs.PendingAbilityCheckAbility != "strength" || gs.PendingAbilityCheckDC != 12 {
		t.Fatalf("pending check был перезаписан (id=%s ability=%s dc=%d)", gs.PendingAbilityCheckID, gs.PendingAbilityCheckAbility, gs.PendingAbilityCheckDC)
	}
}

func TestAbilityCheck_Guardrails_AlreadyChecked(t *testing.T) {
	cfg := setupInfraOnlyIntegrationTest(t)
	if cfg == nil {
		return
	}
	defer cleanupTest(t, &testConfig{db: cfg.db})

	ctx := cfg.ctx
	chatID := cfg.chatID

	// Prepare deterministic world + session + character
	q := &questdomain.Quest{Title: "Test Quest (AlreadyChecked)", Description: "Test quest for already_checked"}
	w := worlddomain.New("Test World (AlreadyChecked)")
	w.Description = "Deterministic test world for already_checked guardrail"
	w.SetMainQuest(q)
	w.Locations = []worlddomain.Location{{Name: "Start", Description: "Start location"}}
	if err := cfg.worldRepo.Save(ctx, w); err != nil {
		t.Fatalf("Не удалось сохранить тестовый мир: %v", err)
	}
	gs := &session.GameSession{ChatID: chatID, State: session.StateActive, World: *w, WorldID: w.ID}
	if err := cfg.sessionRepo.Save(ctx, gs); err != nil {
		t.Fatalf("Не удалось сохранить сессию: %v", err)
	}
	createCharacterUC := characterapp.NewCreateCharacterUseCase(cfg.sessionRepo, cfg.playerRepo)
	if _, err := createCharacterUC.Execute(ctx, characterapp.CreateCharacterRequest{
		ChatID: chatID, Name: "Герой", Race: character.RaceElf, Class: character.ClassWizard,
	}); err != nil {
		t.Fatalf("Не удалось создать персонажа: %v", err)
	}

	gs, _ = cfg.sessionRepo.GetByChatID(ctx, chatID)
	evt := &event.StoryEvent{
		GameSessionID: gs.ID,
		AuthorType:    event.AuthorTypeDM,
		Content:       "wisdom ✅ успех: ты прошел проверку",
		CreatedAt:     time.Now().Add(-10 * time.Minute),
	}
	if err := cfg.eventRepo.Save(ctx, evt); err != nil {
		t.Fatalf("Не удалось сохранить тестовое событие: %v", err)
	}

	tool := dmtools.NewRequestAbilityCheckTool(cfg.sessionRepo, cfg.eventRepo, chatID)
	out, err := tool.Execute(ctx, map[string]interface{}{
		"ability": "wisdom",
		"dc":      12,
		"reason":  "пытается распознать знаки",
		"stakes":  "средние ставки: можно упустить важную деталь",
	})
	if err != nil {
		t.Fatalf("tool.Execute: %v", err)
	}
	m, ok := out.(map[string]interface{})
	if !ok {
		t.Fatalf("ожидали map[string]interface{}, получили %T", out)
	}
	if v, ok := m["already_checked"].(bool); !ok || !v {
		t.Fatalf("ожидали already_checked=true, получили: %#v", m["already_checked"])
	}
	if warn, _ := m["warning"].(string); strings.TrimSpace(warn) == "" {
		t.Fatalf("ожидали непустой warning при already_checked")
	}
}

// TestAbilityCheck_Guardrails_AlreadyChecked_DifferentLocation воспроизводит баг:
// проверка характеристики, выполненная в одной локации, не должна блокировать
// такую же проверку после перехода в другую локацию (сцена в этой модели = локация,
// см. GameSession.IsAbilityCheckRepeatedInScene / ClearSceneAbilityCheckHistory).
func TestAbilityCheck_Guardrails_AlreadyChecked_DifferentLocation(t *testing.T) {
	cfg := setupInfraOnlyIntegrationTest(t)
	if cfg == nil {
		return
	}
	defer cleanupTest(t, &testConfig{db: cfg.db})

	ctx := cfg.ctx
	chatID := cfg.chatID

	q := &questdomain.Quest{Title: "Test Quest (AlreadyChecked/DiffLocation)", Description: "Test quest for already_checked across locations"}
	w := worlddomain.New("Test World (AlreadyChecked/DiffLocation)")
	w.Description = "Deterministic test world for already_checked guardrail across locations"
	w.SetMainQuest(q)
	w.Locations = []worlddomain.Location{
		{Name: "Start", Description: "Start location"},
		{Name: "Other", Description: "Other location"},
	}
	if err := cfg.worldRepo.Save(ctx, w); err != nil {
		t.Fatalf("Не удалось сохранить тестовый мир: %v", err)
	}
	startLocationID := w.Locations[0].ID
	otherLocationID := w.Locations[1].ID

	gs := &session.GameSession{ChatID: chatID, State: session.StateActive, World: *w, WorldID: w.ID, CurrentLocationID: &otherLocationID}
	if err := cfg.sessionRepo.Save(ctx, gs); err != nil {
		t.Fatalf("Не удалось сохранить сессию: %v", err)
	}
	createCharacterUC := characterapp.NewCreateCharacterUseCase(cfg.sessionRepo, cfg.playerRepo)
	if _, err := createCharacterUC.Execute(ctx, characterapp.CreateCharacterRequest{
		ChatID: chatID, Name: "Герой", Race: character.RaceElf, Class: character.ClassWizard,
	}); err != nil {
		t.Fatalf("Не удалось создать персонажа: %v", err)
	}

	gs, _ = cfg.sessionRepo.GetByChatID(ctx, chatID)
	gs.CurrentLocationID = &otherLocationID
	if err := cfg.sessionRepo.Save(ctx, gs); err != nil {
		t.Fatalf("Не удалось обновить текущую локацию сессии: %v", err)
	}

	// Проверка интеллекта была выполнена в стартовой локации...
	evt := &event.StoryEvent{
		GameSessionID: gs.ID,
		LocationID:    &startLocationID,
		AuthorType:    event.AuthorTypeDM,
		Content:       "intelligence ✅ успех: ты прошел проверку",
		CreatedAt:     time.Now().Add(-10 * time.Minute),
	}
	if err := cfg.eventRepo.Save(ctx, evt); err != nil {
		t.Fatalf("Не удалось сохранить тестовое событие: %v", err)
	}

	// ...а теперь игрок в другой локации и запрашивает такую же проверку.
	tool := dmtools.NewRequestAbilityCheckTool(cfg.sessionRepo, cfg.eventRepo, chatID)
	out, err := tool.Execute(ctx, map[string]interface{}{
		"ability": "intelligence",
		"dc":      12,
		"reason":  "пытается вспомнить легенду об этом месте",
		"stakes":  "средние ставки: можно упустить важную деталь",
	})
	if err != nil {
		t.Fatalf("tool.Execute: %v", err)
	}
	m, ok := out.(map[string]interface{})
	if !ok {
		t.Fatalf("ожидали map[string]interface{}, получили %T", out)
	}
	if v, ok := m["already_checked"].(bool); ok && v {
		t.Fatalf("проверка в другой локации не должна блокироваться как already_checked: %#v", m)
	}
	if v, ok := m["cooldown"].(bool); ok && v {
		t.Fatalf("проверка в другой локации не должна блокироваться как cooldown: %#v", m)
	}
}

func TestAbilityCheck_Guardrails_BudgetExceeded(t *testing.T) {
	cfg := setupInfraOnlyIntegrationTest(t)
	if cfg == nil {
		return
	}
	defer cleanupTest(t, &testConfig{db: cfg.db})

	ctx := cfg.ctx
	chatID := cfg.chatID

	// Prepare deterministic world + session + character
	q := &questdomain.Quest{Title: "Test Quest (BudgetExceeded)", Description: "Test quest for ability check budget"}
	w := worlddomain.New("Test World (BudgetExceeded)")
	w.Description = "Deterministic test world for ability check budget"
	w.SetMainQuest(q)
	w.Locations = []worlddomain.Location{{Name: "Start", Description: "Start location"}}
	if err := cfg.worldRepo.Save(ctx, w); err != nil {
		t.Fatalf("Не удалось сохранить тестовый мир: %v", err)
	}
	gs := &session.GameSession{ChatID: chatID, State: session.StateActive, World: *w, WorldID: w.ID}
	if err := cfg.sessionRepo.Save(ctx, gs); err != nil {
		t.Fatalf("Не удалось сохранить сессию: %v", err)
	}
	createCharacterUC := characterapp.NewCreateCharacterUseCase(cfg.sessionRepo, cfg.playerRepo)
	if _, err := createCharacterUC.Execute(ctx, characterapp.CreateCharacterRequest{
		ChatID: chatID, Name: "Герой", Race: character.RaceElf, Class: character.ClassRogue,
	}); err != nil {
		t.Fatalf("Не удалось создать персонажа: %v", err)
	}

	gs, _ = cfg.sessionRepo.GetByChatID(ctx, chatID)
	// Create recent ability check events to exhaust budget
	for i := 0; i < 3; i++ {
		evt := &event.StoryEvent{
			GameSessionID: gs.ID,
			AuthorType:    event.AuthorTypeDM,
			// Важно: события должны считаться "ability check events" для бюджета,
			// но не должны триггерить early-return already_checked для запрашиваемой характеристики.
			// Поэтому используем другую характеристику (Сила), а ниже просим dexterity.
			Content:   "🎲 Проверка Сила (DC 12): d20=10 +2 = 12. Успех.",
			CreatedAt: time.Now().Add(time.Duration(-i) * time.Minute),
		}
		if err := cfg.eventRepo.Save(ctx, evt); err != nil {
			t.Fatalf("Не удалось сохранить тестовое событие: %v", err)
		}
	}

	tool := dmtools.NewRequestAbilityCheckTool(cfg.sessionRepo, cfg.eventRepo, chatID)
	out, err := tool.Execute(ctx, map[string]interface{}{
		"ability": "dexterity",
		"dc":      14,
		"reason":  "пытается перепрыгнуть ловушку",
		"stakes":  "средние ставки: можно получить урон",
	})
	if err != nil {
		t.Fatalf("tool.Execute: %v", err)
	}
	m, ok := out.(map[string]interface{})
	if !ok {
		t.Fatalf("ожидали map[string]interface{}, получили %T", out)
	}
	if v, ok := m["budget_exceeded"].(bool); !ok || !v {
		t.Fatalf("ожидали budget_exceeded=true, получили: %#v", m["budget_exceeded"])
	}
}

func TestAbilityCheck_RollWithoutPendingDoesNotResolve(t *testing.T) {
	cfg := setupInfraOnlyIntegrationTest(t)
	if cfg == nil {
		return
	}
	defer cleanupTest(t, &testConfig{db: cfg.db})

	ctx := cfg.ctx
	chatID := cfg.chatID
	tgUserID := cfg.tgUserID

	// Prepare deterministic world + session
	q := &questdomain.Quest{Title: "Test Quest (RollWithoutPending)", Description: "Test quest for /roll without pending check"}
	w := worlddomain.New("Test World (RollWithoutPending)")
	w.Description = "Deterministic test world for /roll without pending ability check"
	w.SetMainQuest(q)
	w.Locations = []worlddomain.Location{{Name: "Start", Description: "Start location"}}
	if err := cfg.worldRepo.Save(ctx, w); err != nil {
		t.Fatalf("Не удалось сохранить тестовый мир: %v", err)
	}

	gs := &session.GameSession{ChatID: chatID, State: session.StateActive, World: *w, WorldID: w.ID}
	if err := cfg.sessionRepo.Save(ctx, gs); err != nil {
		t.Fatalf("Не удалось сохранить сессию: %v", err)
	}

	// Fake Telegram API server
	fakeAPI := newFakeTelegramAPI()
	srv := httptest.NewServer(fakeAPI.handler(t))
	defer srv.Close()
	apiEndpointFmt := strings.TrimRight(srv.URL, "/") + "/bot%s/%s"

	rollDiceUC := dice.NewRollDiceUseCase()

	bot, err := telegrambot.NewBotWithAPIEndpoint(
		"TEST_TOKEN",
		apiEndpointFmt,
		nil, // initCampaignUC
		nil, // handleActionUC
		nil, // createCharacterUC
		nil, // getHistoryUC
		nil, // getInventoryUC
		nil, // addItemUC
		nil, // handleCombatUC
		rollDiceUC,
		nil, // getQuestsUC
		nil, // getDailyQuestsUC
		nil, // checkDailyProgressUC
		nil, // getMapUC
		nil, // moveToLocationUC
		nil, // getAchievementsUC
		nil, // getSpellsUC
		nil, // useSpellUC
		nil, // generateImageUC
		nil, // getSubscriptionUC
		nil, // checkLimitsUC
		nil, // getLeaderboardUC
		nil, // updateRatingUC
		nil, // performAbilityCheckUC
		cfg.sessionRepo,
		nil, // playerRepo
		nil, // combatRepo
		nil, // feedbackRepo
		nil, // eventRepo
		nil, // indexDocUC
		nil, // deleteSessionDataUC
	)
	if err != nil {
		t.Fatalf("Не удалось создать Telegram bot (fake API): %v", err)
	}

	if err := bot.HandleUpdate(ctx, makeMessageUpdate(chatID, tgUserID, "/roll d20")); err != nil {
		t.Fatalf("/roll d20: %v", err)
	}

	gs, err = cfg.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil || gs == nil {
		t.Fatalf("Не удалось получить сессию после /roll: %v", err)
	}
	if gs.HasPendingAbilityCheck() {
		t.Fatalf("Не ожидали pending ability check после /roll без pending")
	}

	calls := fakeAPI.snapshotCalls()
	found := false
	for _, c := range calls {
		if c.Method == "sendMessage" && c.ChatID == chatID && strings.Contains(c.Text, "/roll") && strings.Contains(c.Text, "Мастер") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Ожидали сообщение о том, что /roll работает только по запросу Мастера")
	}
}

func TestAbilityCheck_Guardrails_Cooldown(t *testing.T) {
	cfg := setupInfraOnlyIntegrationTest(t)
	if cfg == nil {
		return
	}
	defer cleanupTest(t, &testConfig{db: cfg.db})

	ctx := cfg.ctx
	chatID := cfg.chatID

	// Prepare deterministic world + session + character
	q := &questdomain.Quest{Title: "Test Quest (Cooldown)", Description: "Test quest for cooldown"}
	w := worlddomain.New("Test World (Cooldown)")
	w.Description = "Deterministic test world for cooldown guardrail"
	w.SetMainQuest(q)
	w.Locations = []worlddomain.Location{{Name: "Start", Description: "Start location"}}
	if err := cfg.worldRepo.Save(ctx, w); err != nil {
		t.Fatalf("Не удалось сохранить тестовый мир: %v", err)
	}
	gs := &session.GameSession{ChatID: chatID, State: session.StateActive, World: *w, WorldID: w.ID}
	if err := cfg.sessionRepo.Save(ctx, gs); err != nil {
		t.Fatalf("Не удалось сохранить сессию: %v", err)
	}
	createCharacterUC := characterapp.NewCreateCharacterUseCase(cfg.sessionRepo, cfg.playerRepo)
	if _, err := createCharacterUC.Execute(ctx, characterapp.CreateCharacterRequest{
		ChatID: chatID, Name: "Герой", Race: character.RaceHuman, Class: character.ClassRogue,
	}); err != nil {
		t.Fatalf("Не удалось создать персонажа: %v", err)
	}

	gs, _ = cfg.sessionRepo.GetByChatID(ctx, chatID)
	evt := &event.StoryEvent{
		GameSessionID: gs.ID,
		AuthorType:    event.AuthorTypeDM,
		// Важно: чтобы hasResult было false (иначе попадём в already_checked).
		// Но для cooldown нужен checkContext - слова "проверка", "d20" или "бросок"
		Content:   "dm проверка wisdom d20",
		CreatedAt: time.Now().Add(-30 * time.Second),
	}
	if err := cfg.eventRepo.Save(ctx, evt); err != nil {
		t.Fatalf("Не удалось сохранить тестовое событие: %v", err)
	}

	tool := dmtools.NewRequestAbilityCheckTool(cfg.sessionRepo, cfg.eventRepo, chatID)
	out, err := tool.Execute(ctx, map[string]interface{}{
		"ability": "wisdom",
		"dc":      12,
		"reason":  "пытается заметить ловушку",
		"stakes":  "средние ставки: можно наступить на ловушку",
	})
	if err != nil {
		t.Fatalf("tool.Execute: %v", err)
	}
	m, ok := out.(map[string]interface{})
	if !ok {
		t.Fatalf("ожидали map[string]interface{}, получили %T", out)
	}
	if v, ok := m["cooldown"].(bool); !ok || !v {
		t.Fatalf("ожидали cooldown=true, получили: %#v", m["cooldown"])
	}
	if warn, _ := m["warning"].(string); strings.TrimSpace(warn) == "" {
		t.Fatalf("ожидали непустой warning при cooldown")
	}
}
