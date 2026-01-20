package integration

import (
	"strings"
	"testing"
	"time"

	dmtools "dungeons-and-dragons-ai/internal/game/application/dm_tools"
	characterapp "dungeons-and-dragons-ai/internal/game/application/character"
	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/event"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	questdomain "dungeons-and-dragons-ai/internal/game/domain/quest"
	worlddomain "dungeons-and-dragons-ai/internal/game/domain/world"
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
		Content:   "dm: wisdom",
		CreatedAt: time.Now().Add(-30 * time.Second),
	}
	if err := cfg.eventRepo.Save(ctx, evt); err != nil {
		t.Fatalf("Не удалось сохранить тестовое событие: %v", err)
	}

	tool := dmtools.NewRequestAbilityCheckTool(cfg.sessionRepo, cfg.eventRepo, chatID)
	out, err := tool.Execute(ctx, map[string]interface{}{
		"ability": "wisdom",
		"dc":      12,
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

