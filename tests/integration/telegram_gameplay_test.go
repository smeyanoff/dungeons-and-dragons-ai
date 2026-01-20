package integration

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	achievementapp "dungeons-and-dragons-ai/internal/game/application/achievement"
	characterapp "dungeons-and-dragons-ai/internal/game/application/character"
	ratingapp "dungeons-and-dragons-ai/internal/game/application/rating"
	spellapp "dungeons-and-dragons-ai/internal/game/application/spell"
	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/rating"
	questdomain "dungeons-and-dragons-ai/internal/game/domain/quest"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	worlddomain "dungeons-and-dragons-ai/internal/game/domain/world"
	"dungeons-and-dragons-ai/internal/game/infrastructure/persistence"
	llminfrastructure "dungeons-and-dragons-ai/internal/llm/infrastructure"
)

// telegramGameplayConfig расширяет testConfig для Telegram gameplay тестов.
type telegramGameplayConfig struct {
	*testConfig
	rateLimiter *llminfrastructure.SimpleLLMRateLimiter
}

func setupTelegramGameplayTest(t *testing.T) *telegramGameplayConfig {
	cfg := setupIntegrationTest(t)
	// Минимум 2 секунды между LLM запросами (чтобы не бомбить модель).
	rateLimiter := llminfrastructure.NewSimpleLLMRateLimiter(2 * time.Second)
	return &telegramGameplayConfig{testConfig: cfg, rateLimiter: rateLimiter}
}

func (cfg *telegramGameplayConfig) waitForRateLimit(ctx context.Context) error {
	return cfg.rateLimiter.Wait(ctx)
}

// TestTelegramGameplay_CompleteFlow тестирует основной поток игры (с реальными ответами LLM).
func TestTelegramGameplay_CompleteFlow(t *testing.T) {
	cfg := setupTelegramGameplayTest(t)
	defer cleanupTest(t, cfg.testConfig)

	ctx := cfg.ctx
	chatID := cfg.chatID
	tgUserID := cfg.tgUserID

	var problems []string
	var llmFeedback []string

	// 1) New game
	t.Run("1. Создание новой игры", func(t *testing.T) {
		existingSession, err := cfg.sessionRepo.GetByChatID(ctx, chatID)
		if err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось проверить существующую сессию: %v", err))
			t.Fatalf("Не удалось проверить существующую сессию: %v", err)
		}
		if existingSession != nil {
			if err := cfg.sessionRepo.Delete(ctx, chatID); err != nil {
				problems = append(problems, fmt.Sprintf("Не удалось удалить существующую сессию: %v", err))
				t.Fatalf("Не удалось удалить существующую сессию: %v", err)
			}
		}

		if err := cfg.waitForRateLimit(ctx); err != nil {
			problems = append(problems, fmt.Sprintf("Ошибка rate limiter: %v", err))
		}

		world, err := cfg.initCampaignUC.Execute(ctx, "классическое фэнтези с магией и драконами")
		if err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось создать игру: %v", err))
			t.Fatalf("Не удалось создать игру: %v", err)
		}
		if world == nil || len(world.Locations) == 0 {
			problems = append(problems, "Мир создан без локаций")
			t.Fatal("Мир создан без локаций")
		}

		gs := &session.GameSession{ChatID: chatID, State: session.StateActive, World: *world, WorldID: world.ID}
		if err := cfg.sessionRepo.Save(ctx, gs); err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось сохранить сессию: %v", err))
			t.Fatalf("Не удалось сохранить сессию: %v", err)
		}

		t.Logf("✅ Игра создана: мир ID=%d, локаций=%d", world.ID, len(world.Locations))
	})

	// 2) Create character
	t.Run("2. Создание персонажа", func(t *testing.T) {
		req := characterapp.CreateCharacterRequest{ChatID: chatID, Name: "ТестовыйГерой", Race: character.RaceElf, Class: character.ClassWizard}
		player, err := cfg.createCharacterUC.Execute(ctx, req)
		if err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось создать персонажа: %v", err))
			t.Fatalf("Не удалось создать персонажа: %v", err)
		}
		if player == nil || player.Character.Stats.Strength == 0 {
			problems = append(problems, "Персонаж создан без характеристик")
			t.Error("Персонаж создан без характеристик")
		}
	})

	// 3) Player action
	t.Run("3. Исследование начальной локации", func(t *testing.T) {
		if err := cfg.waitForRateLimit(ctx); err != nil {
			problems = append(problems, fmt.Sprintf("Ошибка rate limiter: %v", err))
		}
		response, err := cfg.handleActionUC.Execute(ctx, chatID, "Осматриваю комнату, в которой нахожусь")
		if err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось обработать действие: %v", err))
			t.Fatalf("Не удалось обработать действие: %v", err)
		}
		if response == "" {
			problems = append(problems, "Ответ DM пуст")
			t.Fatal("Ответ DM пуст")
		}
		if len([]rune(response)) < 50 {
			llmFeedback = append(llmFeedback, fmt.Sprintf("Ответ DM слишком короткий (%d символов): %s", len([]rune(response)), response))
		}
	})

	// 4) Systems snapshot
	t.Run("4. Проверка систем", func(t *testing.T) {
		if _, err := cfg.getInventoryUC.Execute(ctx, chatID, tgUserID); err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось получить инвентарь: %v", err))
		}
		if _, err := cfg.getDailyQuestsUC.Execute(ctx, chatID, tgUserID); err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось получить ежедневные задания: %v", err))
		}
		if _, err := cfg.getQuestsUC.Execute(ctx, chatID); err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось получить квесты: %v", err))
		}
		if _, err := cfg.getMapUC.Execute(ctx, chatID); err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось получить карту: %v", err))
		}
		req := achievementapp.GetAchievementsRequest{ChatID: chatID, TgUserID: tgUserID}
		if _, err := cfg.getAchievementsUC.Execute(ctx, req); err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось получить достижения: %v", err))
		}
		sreq := spellapp.GetSpellsRequest{ChatID: chatID, TgUserID: tgUserID}
		if _, err := cfg.getSpellsUC.Execute(ctx, sreq); err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось получить заклинания: %v", err))
		}
		if _, err := cfg.getHistoryUC.Execute(ctx, chatID, 10); err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось получить историю: %v", err))
		}
	})

	if len(problems) > 0 {
		writeToTestingReport(problems)
	}
	if len(llmFeedback) > 0 {
		writeToFeedback(llmFeedback)
	}
}

// TestTelegramGameplay_CombatFlow проверяет базовый боевой сценарий (LLM + боевой usecase).
func TestTelegramGameplay_CombatFlow(t *testing.T) {
	cfg := setupTelegramGameplayTest(t)
	defer cleanupTest(t, cfg.testConfig)

	ctx := cfg.ctx
	chatID := cfg.chatID

	var problems []string
	var llmFeedback []string

	if existingSession, err := cfg.sessionRepo.GetByChatID(ctx, chatID); err == nil && existingSession != nil {
		_ = cfg.sessionRepo.Delete(ctx, chatID)
	}

	if err := cfg.waitForRateLimit(ctx); err != nil {
		problems = append(problems, fmt.Sprintf("Ошибка rate limiter: %v", err))
	}
	world, err := cfg.initCampaignUC.Execute(ctx, "боевое фэнтези с гоблинами")
	if err != nil {
		problems = append(problems, fmt.Sprintf("Не удалось создать игру: %v", err))
		t.Fatalf("Не удалось создать игру: %v", err)
	}
	gs := &session.GameSession{ChatID: chatID, State: session.StateActive, World: *world, WorldID: world.ID}
	if err := cfg.sessionRepo.Save(ctx, gs); err != nil {
		problems = append(problems, fmt.Sprintf("Не удалось сохранить сессию: %v", err))
		t.Fatalf("Не удалось сохранить сессию: %v", err)
	}

	_, err = cfg.createCharacterUC.Execute(ctx, characterapp.CreateCharacterRequest{ChatID: chatID, Name: "ВоинТест", Race: character.RaceHuman, Class: character.ClassFighter})
	if err != nil {
		problems = append(problems, fmt.Sprintf("Не удалось создать персонажа: %v", err))
		t.Fatalf("Не удалось создать персонажа: %v", err)
	}

	if err := cfg.waitForRateLimit(ctx); err != nil {
		problems = append(problems, fmt.Sprintf("Ошибка rate limiter: %v", err))
	}
	resp, err := cfg.handleActionUC.Execute(ctx, chatID, "Вижу гоблина и атакую его мечом")
	if err != nil {
		problems = append(problems, fmt.Sprintf("Не удалось обработать боевое действие: %v", err))
		t.Fatalf("Не удалось обработать боевое действие: %v", err)
	}
	if resp == "" {
		problems = append(problems, "Ответ DM пуст при инициации боя")
		t.Fatal("Ответ DM пуст")
	}
	lower := strings.ToLower(resp)
	if !(strings.Contains(lower, "бой") || strings.Contains(lower, "атака") || strings.Contains(lower, "урон") || strings.Contains(lower, "гоблин")) {
		llmFeedback = append(llmFeedback, fmt.Sprintf("Ответ DM при инициации боя не содержит боевых элементов: %s", resp))
	}

	if len(problems) > 0 {
		writeToTestingReport(problems)
	}
	if len(llmFeedback) > 0 {
		writeToFeedback(llmFeedback)
	}
}

// TestTelegramGameplay_SpellSystem тестирует систему заклинаний без LLM (детерминированно).
func TestTelegramGameplay_SpellSystem(t *testing.T) {
	cfg := setupTelegramGameplayTest(t)
	defer cleanupTest(t, cfg.testConfig)

	ctx := cfg.ctx
	chatID := cfg.chatID
	tgUserID := cfg.tgUserID

	var problems []string

	if existingSession, err := cfg.sessionRepo.GetByChatID(ctx, chatID); err == nil && existingSession != nil {
		_ = cfg.sessionRepo.Delete(ctx, chatID)
	}

	worldRepo := persistence.NewWorldRepository(cfg.db)
	q := &questdomain.Quest{Title: "Test Quest", Description: "Test quest"}
	w := worlddomain.New("Test World (Spells)")
	w.Description = "Deterministic test world for spell system"
	w.SetMainQuest(q)
	w.Locations = []worlddomain.Location{{Name: "Start", Description: "Start location"}}
	if err := worldRepo.Save(ctx, w); err != nil {
		problems = append(problems, fmt.Sprintf("Не удалось сохранить тестовый мир: %v", err))
		t.Fatalf("Не удалось сохранить тестовый мир: %v", err)
	}

	gs := &session.GameSession{ChatID: chatID, State: session.StateActive, World: *w, WorldID: w.ID}
	if err := cfg.sessionRepo.Save(ctx, gs); err != nil {
		problems = append(problems, fmt.Sprintf("Не удалось сохранить сессию: %v", err))
		t.Fatalf("Не удалось сохранить сессию: %v", err)
	}

	_, err := cfg.createCharacterUC.Execute(ctx, characterapp.CreateCharacterRequest{ChatID: chatID, Name: "МагТест", Race: character.RaceElf, Class: character.ClassWizard})
	if err != nil {
		problems = append(problems, fmt.Sprintf("Не удалось создать персонажа: %v", err))
		t.Fatalf("Не удалось создать персонажа: %v", err)
	}

	t.Run("Проверка доступных заклинаний", func(t *testing.T) {
		spellsText, err := cfg.getSpellsUC.Execute(ctx, spellapp.GetSpellsRequest{ChatID: chatID, TgUserID: tgUserID})
		if err != nil {
			problems = append(problems, fmt.Sprintf("Не удалось получить заклинания: %v", err))
			t.Errorf("Не удалось получить заклинания: %v", err)
			return
		}
		if spellsText == "" {
			problems = append(problems, "Текст заклинаний пуст")
			t.Error("Текст заклинаний пуст")
		}
	})

	t.Run("Использование заклинания", func(t *testing.T) {
		_, err := cfg.useSpellUC.Execute(ctx, spellapp.UseSpellRequest{ChatID: chatID, TgUserID: tgUserID, SpellName: "Магическая стрела"})
		// Это может быть нормально, если нет цели/не в бою.
		if err != nil {
			t.Logf("⚠️ Не удалось использовать заклинание (может быть нормально): %v", err)
		}
	})

	if len(problems) > 0 {
		writeToTestingReport(problems)
	}
}

// TestTelegramGameplay_RatingSystem тестирует систему рейтингов через Telegram (без LLM-assert'ов).
func TestTelegramGameplay_RatingSystem(t *testing.T) {
	cfg := setupTelegramGameplayTest(t)
	defer cleanupTest(t, cfg.testConfig)

	ctx := cfg.ctx
	chatID := cfg.chatID
	tgUserID := cfg.tgUserID

	var problems []string

	if existingSession, err := cfg.sessionRepo.GetByChatID(ctx, chatID); err == nil && existingSession != nil {
		_ = cfg.sessionRepo.Delete(ctx, chatID)
	}

	if err := cfg.waitForRateLimit(ctx); err != nil {
		problems = append(problems, fmt.Sprintf("Ошибка rate limiter: %v", err))
	}
	world, err := cfg.initCampaignUC.Execute(ctx, "тест рейтингов")
	if err != nil {
		problems = append(problems, fmt.Sprintf("Не удалось создать игру: %v", err))
		t.Fatalf("Не удалось создать игру: %v", err)
	}
	gs := &session.GameSession{ChatID: chatID, State: session.StateActive, World: *world, WorldID: world.ID}
	if err := cfg.sessionRepo.Save(ctx, gs); err != nil {
		problems = append(problems, fmt.Sprintf("Не удалось сохранить сессию: %v", err))
		t.Fatalf("Не удалось сохранить сессию: %v", err)
	}

	player, err := cfg.createCharacterUC.Execute(ctx, characterapp.CreateCharacterRequest{ChatID: chatID, Name: "РейтинговыйГерой", Race: character.RaceHuman, Class: character.ClassFighter})
	if err != nil {
		problems = append(problems, fmt.Sprintf("Не удалось создать персонажа: %v", err))
		t.Fatalf("Не удалось создать персонажа: %v", err)
	}
	_ = player

	addXPReq := characterapp.AddExperienceRequest{ChatID: chatID, Amount: 500, Reason: "test"}
	_, _, err = cfg.addExperienceUC.Execute(ctx, addXPReq)
	if err != nil {
		problems = append(problems, fmt.Sprintf("Не удалось добавить опыт: %v", err))
		t.Fatalf("Не удалось добавить опыт: %v", err)
	}

	if err := cfg.updateRatingUC.Execute(ctx, ratingapp.UpdateRatingRequest{TgUserID: tgUserID, ChatID: chatID}); err != nil {
		problems = append(problems, fmt.Sprintf("Не удалось обновить рейтинг: %v", err))
		t.Fatalf("Не удалось обновить рейтинг: %v", err)
	}

	_, err = cfg.getLeaderboardUC.Execute(ctx, ratingapp.GetLeaderboardRequest{MetricType: rating.MetricTypeExperience, Limit: 10, TgUserID: tgUserID})
	if err != nil {
		problems = append(problems, fmt.Sprintf("Не удалось получить лидерборд: %v", err))
		t.Fatalf("Не удалось получить лидерборд: %v", err)
	}

	if len(problems) > 0 {
		writeToTestingReport(problems)
	}
}
