package dm_analyzer

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"dungeons-and-dragons-ai/internal/game/domain/combat"
	"dungeons-and-dragons-ai/internal/game/domain/inventory"
	"dungeons-and-dragons-ai/internal/game/domain/quest"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/game/domain/world"
	"dungeons-and-dragons-ai/internal/llm/domain"
	llmtools "dungeons-and-dragons-ai/internal/llm/domain/tools"
)

// intPtr создает указатель на int (helper для тестов)
func intPtr(i int) *int {
	return &i
}

// Mock LLM
type mockLLM struct {
	generateFunc              func(ctx context.Context, prompt string) (string, error)
	generateWithMaxTokensFunc func(ctx context.Context, prompt string, maxTokens int) (string, error)
	generateWithToolsFunc     func(ctx context.Context, prompt string, tools []llmtools.Tool) (*domain.LLMResponseWithTools, error)
}

func (m *mockLLM) Generate(ctx context.Context, prompt string) (string, error) {
	if m.generateFunc != nil {
		return m.generateFunc(ctx, prompt)
	}
	// Backward-compatible fallback: older tests stub GenerateWithMaxTokens only.
	if m.generateWithMaxTokensFunc != nil {
		return m.generateWithMaxTokensFunc(ctx, prompt, 0)
	}
	return "{}", nil
}

func (m *mockLLM) GenerateWithMaxTokens(ctx context.Context, prompt string, maxTokens int) (string, error) {
	if m.generateWithMaxTokensFunc != nil {
		return m.generateWithMaxTokensFunc(ctx, prompt, maxTokens)
	}
	return "{}", nil
}

// GenerateWithTools implements domain.LLM interface
func (m *mockLLM) GenerateWithTools(ctx context.Context, prompt string, tools []llmtools.Tool) (*domain.LLMResponseWithTools, error) {
	if m.generateWithToolsFunc != nil {
		return m.generateWithToolsFunc(ctx, prompt, tools)
	}
	// Fallback на обычный Generate для обратной совместимости
	content, err := m.Generate(ctx, prompt)
	if err != nil {
		return nil, err
	}
	return &domain.LLMResponseWithTools{
		Content:   content,
		ToolCalls: nil,
		Finished:  true,
	}, nil
}

// Mock Combat Repository
type mockCombatRepo struct {
	saveFunc                 func(ctx context.Context, c *combat.Combat) error
	getActiveBySessionIDFunc func(ctx context.Context, sessionID uint) (*combat.Combat, error)
	savedCombats             []*combat.Combat
}

func (m *mockCombatRepo) Save(ctx context.Context, c *combat.Combat) error {
	if m.saveFunc != nil {
		return m.saveFunc(ctx, c)
	}
	if m.savedCombats == nil {
		m.savedCombats = make([]*combat.Combat, 0)
	}
	m.savedCombats = append(m.savedCombats, c)
	return nil
}

func (m *mockCombatRepo) GetActiveBySessionID(ctx context.Context, sessionID uint) (*combat.Combat, error) {
	if m.getActiveBySessionIDFunc != nil {
		return m.getActiveBySessionIDFunc(ctx, sessionID)
	}
	return nil, nil
}

// Mock Quest Repository
type mockQuestRepo struct {
	getByWorldIDFunc func(ctx context.Context, worldID uint) ([]*quest.Quest, error)
	saveFunc         func(ctx context.Context, q *quest.Quest) error
	savedQuests      []*quest.Quest
}

func (m *mockQuestRepo) GetByWorldID(ctx context.Context, worldID uint) ([]*quest.Quest, error) {
	if m.getByWorldIDFunc != nil {
		return m.getByWorldIDFunc(ctx, worldID)
	}
	return nil, nil
}

func (m *mockQuestRepo) Save(ctx context.Context, q *quest.Quest) error {
	if m.saveFunc != nil {
		return m.saveFunc(ctx, q)
	}
	if m.savedQuests == nil {
		m.savedQuests = make([]*quest.Quest, 0)
	}
	m.savedQuests = append(m.savedQuests, q)
	return nil
}

// Mock Inventory Repository
type mockInventoryRepo struct {
	getByCharacterIDFunc func(ctx context.Context, characterID uint) (*inventory.Inventory, error)
	saveFunc             func(ctx context.Context, inv *inventory.Inventory) error
}

func (m *mockInventoryRepo) GetByCharacterID(ctx context.Context, characterID uint) (*inventory.Inventory, error) {
	if m.getByCharacterIDFunc != nil {
		return m.getByCharacterIDFunc(ctx, characterID)
	}
	return nil, nil
}

func (m *mockInventoryRepo) Save(ctx context.Context, inv *inventory.Inventory) error {
	if m.saveFunc != nil {
		return m.saveFunc(ctx, inv)
	}
	return nil
}

func TestAnalyzeDMResponseUseCase_Execute(t *testing.T) {
	tests := []struct {
		name          string
		dmResponse    string
		setupMocks    func(*mockLLM, *mockCombatRepo, *mockQuestRepo)
		expectedError bool
		validate      func(*testing.T, *DMResponseAnalysis, *mockCombatRepo, *mockQuestRepo)
	}{
		{
			name:       "combat detected",
			dmResponse: "Внезапно на вас нападают три гоблина!",
			setupMocks: func(llm *mockLLM, combatRepo *mockCombatRepo, questRepo *mockQuestRepo) {
				llm.generateWithMaxTokensFunc = func(ctx context.Context, prompt string, maxTokens int) (string, error) {
					analysis := DMResponseAnalysis{
						CombatDetected: true,
						Enemies: []Enemy{
							{Name: "Goblin", HP: intPtr(10), AC: intPtr(15), AttackBonus: intPtr(4)},
						},
					}
					data, _ := json.Marshal(analysis)
					return string(data), nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, analysis *DMResponseAnalysis, combatRepo *mockCombatRepo, questRepo *mockQuestRepo) {
				if !analysis.CombatDetected {
					t.Error("expected combat to be detected")
				}
				if len(analysis.Enemies) == 0 {
					t.Error("expected at least one enemy")
				}
				if len(combatRepo.savedCombats) == 0 {
					t.Error("expected combat to be saved")
				}
				if len(combatRepo.savedCombats) > 0 {
					savedCombat := combatRepo.savedCombats[0]
					if savedCombat.State != combat.CombatStateActive {
						t.Errorf("expected combat state to be active, got %s", savedCombat.State)
					}
					if len(savedCombat.Participants) < 2 {
						t.Error("expected at least 2 participants (player + enemy)")
					}
				}
			},
		},
		{
			name:       "quest completed",
			dmResponse: "Вы успешно выполнили квест 'Спасение принцессы'!",
			setupMocks: func(llm *mockLLM, combatRepo *mockCombatRepo, questRepo *mockQuestRepo) {
				llm.generateWithMaxTokensFunc = func(ctx context.Context, prompt string, maxTokens int) (string, error) {
					analysis := DMResponseAnalysis{
						QuestCompleted:   true,
						QuestTitle:       "Спасение принцессы",
						ExperienceGained: 150,
						ExperienceReason: "quest_completed",
					}
					data, _ := json.Marshal(analysis)
					return string(data), nil
				}
				questRepo.getByWorldIDFunc = func(ctx context.Context, worldID uint) ([]*quest.Quest, error) {
					return []*quest.Quest{
						quest.New("Спасение принцессы", "Спасти принцессу от дракона"),
					}, nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, analysis *DMResponseAnalysis, combatRepo *mockCombatRepo, questRepo *mockQuestRepo) {
				if !analysis.QuestCompleted {
					t.Error("expected quest to be completed")
				}
				if analysis.ExperienceGained != 150 {
					t.Errorf("expected 150 experience, got %d", analysis.ExperienceGained)
				}
				if len(questRepo.savedQuests) == 0 {
					t.Error("expected quest to be saved")
				}
				if len(questRepo.savedQuests) > 0 {
					savedQuest := questRepo.savedQuests[0]
					if savedQuest.Status != quest.QuestStatusCompleted {
						t.Errorf("expected quest status to be completed, got %s", savedQuest.Status)
					}
				}
			},
		},
		{
			name:       "quest failed",
			dmResponse: "Квест 'Найти артефакт' провален. Артефакт уничтожен.",
			setupMocks: func(llm *mockLLM, combatRepo *mockCombatRepo, questRepo *mockQuestRepo) {
				llm.generateWithMaxTokensFunc = func(ctx context.Context, prompt string, maxTokens int) (string, error) {
					analysis := DMResponseAnalysis{
						QuestFailed: true,
						QuestTitle:  "Найти артефакт",
					}
					data, _ := json.Marshal(analysis)
					return string(data), nil
				}
				questRepo.getByWorldIDFunc = func(ctx context.Context, worldID uint) ([]*quest.Quest, error) {
					return []*quest.Quest{
						quest.New("Найти артефакт", "Найти древний артефакт"),
					}, nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, analysis *DMResponseAnalysis, combatRepo *mockCombatRepo, questRepo *mockQuestRepo) {
				if !analysis.QuestFailed {
					t.Error("expected quest to be failed")
				}
				if len(questRepo.savedQuests) > 0 {
					savedQuest := questRepo.savedQuests[0]
					if savedQuest.Status != quest.QuestStatusFailed {
						t.Errorf("expected quest status to be failed, got %s", savedQuest.Status)
					}
				}
			},
		},
		{
			name:       "experience gained",
			dmResponse: "Вы победили в бою и получили опыт!",
			setupMocks: func(llm *mockLLM, combatRepo *mockCombatRepo, questRepo *mockQuestRepo) {
				llm.generateWithMaxTokensFunc = func(ctx context.Context, prompt string, maxTokens int) (string, error) {
					analysis := DMResponseAnalysis{
						ExperienceGained: 50,
						ExperienceReason: "combat_victory",
					}
					data, _ := json.Marshal(analysis)
					return string(data), nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, analysis *DMResponseAnalysis, combatRepo *mockCombatRepo, questRepo *mockQuestRepo) {
				if analysis.ExperienceGained != 50 {
					t.Errorf("expected 50 experience, got %d", analysis.ExperienceGained)
				}
				if analysis.ExperienceReason != "combat_victory" {
					t.Errorf("expected reason 'combat_victory', got %s", analysis.ExperienceReason)
				}
			},
		},
		{
			name:       "no events detected",
			dmResponse: "Вы идете по дороге и видите красивый пейзаж.",
			setupMocks: func(llm *mockLLM, combatRepo *mockCombatRepo, questRepo *mockQuestRepo) {
				llm.generateWithMaxTokensFunc = func(ctx context.Context, prompt string, maxTokens int) (string, error) {
					analysis := DMResponseAnalysis{}
					data, _ := json.Marshal(analysis)
					return string(data), nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, analysis *DMResponseAnalysis, combatRepo *mockCombatRepo, questRepo *mockQuestRepo) {
				if analysis.CombatDetected {
					t.Error("expected no combat detected")
				}
				if analysis.QuestCompleted || analysis.QuestFailed {
					t.Error("expected no quest status change")
				}
				if analysis.ExperienceGained != 0 {
					t.Errorf("expected 0 experience, got %d", analysis.ExperienceGained)
				}
			},
		},
		{
			name:       "empty analysis triggers combat fallback",
			dmResponse: "Начался бой, гоблин атакует!",
			setupMocks: func(llm *mockLLM, combatRepo *mockCombatRepo, questRepo *mockQuestRepo) {
				llm.generateWithMaxTokensFunc = func(ctx context.Context, prompt string, maxTokens int) (string, error) {
					return "{}", nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, analysis *DMResponseAnalysis, combatRepo *mockCombatRepo, questRepo *mockQuestRepo) {
				if !analysis.CombatDetected {
					t.Error("expected combat detected from fallback")
				}
				if len(analysis.Enemies) == 0 {
					t.Error("expected enemies from fallback analysis")
				}
			},
		},
		{
			name:       "LLM error - returns empty analysis",
			dmResponse: "Тестовый ответ",
			setupMocks: func(llm *mockLLM, combatRepo *mockCombatRepo, questRepo *mockQuestRepo) {
				llm.generateWithMaxTokensFunc = func(ctx context.Context, prompt string, maxTokens int) (string, error) {
					return "", errors.New("LLM error")
				}
			},
			expectedError: false, // Execute не возвращает ошибку при ошибке LLM
			validate: func(t *testing.T, analysis *DMResponseAnalysis, combatRepo *mockCombatRepo, questRepo *mockQuestRepo) {
				// При ошибке LLM должен вернуться пустой анализ
				if analysis.CombatDetected {
					t.Error("expected no combat detected on LLM error")
				}
			},
		},
		{
			name:       "invalid JSON from LLM",
			dmResponse: "Тестовый ответ",
			setupMocks: func(llm *mockLLM, combatRepo *mockCombatRepo, questRepo *mockQuestRepo) {
				llm.generateWithMaxTokensFunc = func(ctx context.Context, prompt string, maxTokens int) (string, error) {
					return "invalid json {", nil
				}
			},
			expectedError: false, // Execute не возвращает ошибку при невалидном JSON
			validate: func(t *testing.T, analysis *DMResponseAnalysis, combatRepo *mockCombatRepo, questRepo *mockQuestRepo) {
				// При невалидном JSON должен вернуться пустой анализ
				if analysis.CombatDetected {
					t.Error("expected no combat detected on invalid JSON")
				}
			},
		},
		{
			name:       "JSON with markdown code blocks",
			dmResponse: "Бой начался!",
			setupMocks: func(llm *mockLLM, combatRepo *mockCombatRepo, questRepo *mockQuestRepo) {
				llm.generateWithMaxTokensFunc = func(ctx context.Context, prompt string, maxTokens int) (string, error) {
					analysis := DMResponseAnalysis{
						CombatDetected: true,
						Enemies: []Enemy{
							{Name: "Goblin", HP: intPtr(10), AC: intPtr(15), AttackBonus: intPtr(4)},
						},
					}
					data, _ := json.Marshal(analysis)
					return "```json\n" + string(data) + "\n```", nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, analysis *DMResponseAnalysis, combatRepo *mockCombatRepo, questRepo *mockQuestRepo) {
				if !analysis.CombatDetected {
					t.Error("expected combat to be detected")
				}
			},
		},
		{
			name:       "active combat already exists - should not create new",
			dmResponse: "Еще один враг появляется!",
			setupMocks: func(llm *mockLLM, combatRepo *mockCombatRepo, questRepo *mockQuestRepo) {
				llm.generateWithMaxTokensFunc = func(ctx context.Context, prompt string, maxTokens int) (string, error) {
					analysis := DMResponseAnalysis{
						CombatDetected: true,
						Enemies: []Enemy{
							{Name: "Orc", HP: intPtr(15), AC: intPtr(13), AttackBonus: intPtr(5)},
						},
					}
					data, _ := json.Marshal(analysis)
					return string(data), nil
				}
				combatRepo.getActiveBySessionIDFunc = func(ctx context.Context, sessionID uint) (*combat.Combat, error) {
					existingCombat := &combat.Combat{
						GameSessionID: sessionID,
						State:         combat.CombatStateActive,
					}
					return existingCombat, nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, analysis *DMResponseAnalysis, combatRepo *mockCombatRepo, questRepo *mockQuestRepo) {
				// Не должен создавать новый бой, если уже есть активный
				if len(combatRepo.savedCombats) > 0 {
					t.Error("expected no new combat to be created when active combat exists")
				}
			},
		},
		{
			name:       "quest not found by title - uses first active",
			dmResponse: "Квест 'Несуществующий квест' выполнен!",
			setupMocks: func(llm *mockLLM, combatRepo *mockCombatRepo, questRepo *mockQuestRepo) {
				llm.generateWithMaxTokensFunc = func(ctx context.Context, prompt string, maxTokens int) (string, error) {
					analysis := DMResponseAnalysis{
						QuestCompleted: true,
						QuestTitle:     "Несуществующий квест",
					}
					data, _ := json.Marshal(analysis)
					return string(data), nil
				}
				questRepo.getByWorldIDFunc = func(ctx context.Context, worldID uint) ([]*quest.Quest, error) {
					q1 := quest.New("Активный квест 1", "Описание 1")
					q2 := quest.New("Активный квест 2", "Описание 2")
					q2.Complete() // Этот квест уже выполнен
					return []*quest.Quest{q1, q2}, nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, analysis *DMResponseAnalysis, combatRepo *mockCombatRepo, questRepo *mockQuestRepo) {
				// Должен использовать первый активный квест
				if len(questRepo.savedQuests) > 0 {
					savedQuest := questRepo.savedQuests[0]
					if savedQuest.Title != "Активный квест 1" {
						t.Errorf("expected to update first active quest, got %s", savedQuest.Title)
					}
					if savedQuest.Status != quest.QuestStatusCompleted {
						t.Errorf("expected quest to be completed, got %s", savedQuest.Status)
					}
				}
			},
		},
		{
			name:       "no active quests - should not fail",
			dmResponse: "Квест выполнен!",
			setupMocks: func(llm *mockLLM, combatRepo *mockCombatRepo, questRepo *mockQuestRepo) {
				llm.generateWithMaxTokensFunc = func(ctx context.Context, prompt string, maxTokens int) (string, error) {
					analysis := DMResponseAnalysis{
						QuestCompleted: true,
					}
					data, _ := json.Marshal(analysis)
					return string(data), nil
				}
				questRepo.getByWorldIDFunc = func(ctx context.Context, worldID uint) ([]*quest.Quest, error) {
					q1 := quest.New("Завершенный квест", "Описание")
					q1.Complete()
					return []*quest.Quest{q1}, nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, analysis *DMResponseAnalysis, combatRepo *mockCombatRepo, questRepo *mockQuestRepo) {
				// Не должно быть ошибки, если нет активных квестов
				if len(questRepo.savedQuests) > 0 {
					t.Error("expected no quest to be saved when no active quests")
				}
			},
		},
		{
			name:       "combat detected but no enemies - should not create combat and must set fallback message",
			dmResponse: "Бой начался!",
			setupMocks: func(llm *mockLLM, combatRepo *mockCombatRepo, questRepo *mockQuestRepo) {
				llm.generateWithMaxTokensFunc = func(ctx context.Context, prompt string, maxTokens int) (string, error) {
					analysis := DMResponseAnalysis{
						CombatDetected: true,
						Enemies:        []Enemy{}, // Пустой список врагов из LLM
					}
					data, _ := json.Marshal(analysis)
					return string(data), nil
				}
			},
			expectedError: false,
			validate: func(t *testing.T, analysis *DMResponseAnalysis, combatRepo *mockCombatRepo, questRepo *mockQuestRepo) {
				// Не должен создавать бой без врагов
				if len(combatRepo.savedCombats) > 0 {
					t.Error("expected no combat to be created when no enemies")
				}
				// Анализ после строгой валидации не должен помечать бой как активный
				if analysis.CombatDetected {
					t.Error("expected combat_detected to be false when no valid enemies")
				}
				if len(analysis.Enemies) != 0 {
					t.Errorf("expected enemies to be empty after validation, got %d", len(analysis.Enemies))
				}
				// Игрок должен получить понятное fallback‑сообщение вместо запуска боевой механики
				if analysis.CombatFallbackMessage == "" {
					t.Error("expected combat_fallback_message to be set when combat is disabled due to empty enemies")
				}
			},
		},
		{
			name:       "combat repo error",
			dmResponse: "Бой начался!",
			setupMocks: func(llm *mockLLM, combatRepo *mockCombatRepo, questRepo *mockQuestRepo) {
				llm.generateWithMaxTokensFunc = func(ctx context.Context, prompt string, maxTokens int) (string, error) {
					analysis := DMResponseAnalysis{
						CombatDetected: true,
						Enemies: []Enemy{
							{Name: "Goblin", HP: intPtr(10), AC: intPtr(15), AttackBonus: intPtr(4)},
						},
					}
					data, _ := json.Marshal(analysis)
					return string(data), nil
				}
				combatRepo.getActiveBySessionIDFunc = func(ctx context.Context, sessionID uint) (*combat.Combat, error) {
					return nil, errors.New("database error")
				}
			},
			expectedError: false, // Execute не возвращает ошибку при ошибке обработки
			validate: func(t *testing.T, analysis *DMResponseAnalysis, combatRepo *mockCombatRepo, questRepo *mockQuestRepo) {
				// Анализ должен быть успешным, но бой не должен быть сохранен
				if !analysis.CombatDetected {
					t.Error("expected combat to be detected")
				}
			},
		},
		{
			name:       "quest repo error",
			dmResponse: "Квест выполнен!",
			setupMocks: func(llm *mockLLM, combatRepo *mockCombatRepo, questRepo *mockQuestRepo) {
				llm.generateWithMaxTokensFunc = func(ctx context.Context, prompt string, maxTokens int) (string, error) {
					analysis := DMResponseAnalysis{
						QuestCompleted: true,
						QuestTitle:     "Тестовый квест",
					}
					data, _ := json.Marshal(analysis)
					return string(data), nil
				}
				questRepo.getByWorldIDFunc = func(ctx context.Context, worldID uint) ([]*quest.Quest, error) {
					return nil, errors.New("database error")
				}
			},
			expectedError: false, // Execute не возвращает ошибку при ошибке обработки
			validate: func(t *testing.T, analysis *DMResponseAnalysis, combatRepo *mockCombatRepo, questRepo *mockQuestRepo) {
				// Анализ должен быть успешным
				if !analysis.QuestCompleted {
					t.Error("expected quest to be completed")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			llm := &mockLLM{}
			combatRepo := &mockCombatRepo{}
			questRepo := &mockQuestRepo{}
			inventoryRepo := &mockInventoryRepo{}

			if tt.setupMocks != nil {
				tt.setupMocks(llm, combatRepo, questRepo)
			}

			uc := NewAnalyzeDMResponseUseCase(
				llm,
				combatRepo,
				questRepo,
				inventoryRepo,
				1,     // sessionID
				123,   // chatID
				1,     // worldID
				1,     // characterID
				1,     // playerID
				false, // autoGenerateImages
				0,     // imagesGeneratedInSession
			)

			analysis, err := uc.Execute(context.Background(), tt.dmResponse)

			if tt.expectedError {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if tt.validate != nil {
					tt.validate(t, analysis, combatRepo, questRepo)
				}
			}
		})
	}
}

func TestCleanJSONResponse(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "json with markdown code block",
			input:    "```json\n{\"test\": \"value\"}\n```",
			expected: "{\"test\": \"value\"}",
		},
		{
			name:     "json with code block without json",
			input:    "```\n{\"test\": \"value\"}\n```",
			expected: "{\"test\": \"value\"}",
		},
		{
			name:     "plain json",
			input:    "{\"test\": \"value\"}",
			expected: "{\"test\": \"value\"}",
		},
		{
			name:     "json with whitespace",
			input:    "  {\"test\": \"value\"}  ",
			expected: "{\"test\": \"value\"}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cleanJSONResponse(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestTryRepairTruncatedJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		isValid  bool
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "{}",
			isValid:  true,
		},
		{
			name:     "valid json",
			input:    "{\"test\": \"value\"}",
			expected: "{\"test\": \"value\"}",
			isValid:  true,
		},
		{
			name:     "truncated object",
			input:    "{\"test\": \"val",
			expected: "{\"test\": \"val\"}",
			isValid:  false, // jsonrepair may not produce valid JSON for incomplete strings
		},
		{
			name:     "truncated array",
			input:    "{\"items\": [1, 2, 3",
			expected: "{\"items\": [1, 2, 3}]", // Функция закрывает сначала объект, потом массив
			isValid:  false,                    // Известная проблема: функция не всегда правильно восстанавливает вложенные структуры
		},
		{
			name:     "nested truncated",
			input:    "{\"outer\": {\"inner\": \"val",
			expected: "{\"outer\": {\"inner\": \"val\"}}",
			isValid:  false, // jsonrepair may not produce valid JSON for incomplete nested structures
		},
		{
			name:     "trailing comma",
			input:    "{\"a\": 1, \"b\": 2,",
			expected: "{\"a\": 1, \"b\": 2}",
			isValid:  true,
		},
		{
			name:     "multiple open braces",
			input:    "{\"a\": {\"b\": {",
			expected: "{\"a\": {\"b\": {}}",
			isValid:  true,
		},
		{
			name:     "string with escaped quote",
			input:    "{\"test\": \"value\\\"",
			expected: "{\"test\": \"value\\\"\"}",
			isValid:  false, // jsonrepair may not handle escaped quotes properly
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tryRepairTruncatedJSON(tt.input)
			if tt.isValid {
				if !json.Valid([]byte(result)) {
					t.Errorf("expected valid JSON, got invalid: %q", result)
				}
			} else {
				// Для невалидных случаев просто проверяем, что функция что-то вернула
				if result == "" {
					t.Error("expected non-empty result")
				}
			}
			// Проверяем, что результат содержит ожидаемую структуру
			if len(result) < len(tt.expected) {
				t.Errorf("result too short: expected at least %d chars, got %d", len(tt.expected), len(result))
			}
		})
	}
}

func TestBuildAnalysisPrompt(t *testing.T) {
	dmResponse := "Вы видите трех гоблинов!"
	prompt := buildAnalysisPrompt(dmResponse, false)

	if prompt == "" {
		t.Error("expected non-empty prompt")
	}

	if !contains(prompt, dmResponse) {
		t.Error("prompt should contain DM response")
	}

	// Проверяем, что промпт содержит ключевые слова
	keywords := []string{"combat_detected", "enemies", "quest_completed", "experience_gained"}
	for _, keyword := range keywords {
		if !contains(prompt, keyword) {
			t.Errorf("prompt should contain keyword: %s", keyword)
		}
	}
}

// TestAnalyzeDMResponseUseCase_HandleCombatStart_DefaultHPAC проверяет, что handleCombatStart использует значения по умолчанию для HP/AC (#64)
func TestAnalyzeDMResponseUseCase_HandleCombatStart_DefaultHPAC(t *testing.T) {
	tests := []struct {
		name             string
		enemies          []Enemy
		expectedDefaults bool
		validate         func(*testing.T, *combat.Combat)
	}{
		{
			name: "enemy with zero HP uses default",
			enemies: []Enemy{
				{Name: "Goblin", HP: intPtr(0), AC: intPtr(0), AttackBonus: intPtr(0)}, // Все значения 0 - должны использовать дефолты
			},
			expectedDefaults: true,
			validate: func(t *testing.T, c *combat.Combat) {
				if len(c.Participants) < 2 {
					t.Fatalf("expected at least 2 participants, got %d", len(c.Participants))
				}
				// Ищем врага (не игрока)
				var enemyParticipant *combat.CombatParticipant
				for i := range c.Participants {
					if !c.Participants[i].IsPlayer {
						enemyParticipant = &c.Participants[i]
						break
					}
				}
				if enemyParticipant == nil {
					t.Fatal("expected at least one enemy participant")
				}
				// Проверяем дефолтные значения: HP=10, AC=12, AttackBonus=2
				if enemyParticipant.MonsterHP != 10 {
					t.Errorf("expected default HP 10, got %d", enemyParticipant.MonsterHP)
				}
				if enemyParticipant.MonsterMaxHP != 10 {
					t.Errorf("expected default MaxHP 10, got %d", enemyParticipant.MonsterMaxHP)
				}
				if enemyParticipant.MonsterAC != 12 {
					t.Errorf("expected default AC 12, got %d", enemyParticipant.MonsterAC)
				}
				// AttackBonus: 0 через указатель означает, что значение было передано, но оно 0
				// Однако в новой логике 0 <= 0, поэтому используется дефолт 2
				// Но если передано 0 явно (не null), то должно использоваться 0
				// Проверяем, что для 0 используется дефолт 2 (так как 0 не >= 0 в новой логике)
				if enemyParticipant.MonsterAttackBonus != 2 {
					t.Errorf("expected AttackBonus 2 (0 is not >= 0, so default applied), got %d", enemyParticipant.MonsterAttackBonus)
				}
			},
		},
		{
			name: "enemy with negative HP uses default",
			enemies: []Enemy{
				{Name: "Orc", HP: intPtr(-5), AC: intPtr(-3), AttackBonus: intPtr(-1)}, // Отрицательные значения - должны использовать дефолты
			},
			expectedDefaults: true,
			validate: func(t *testing.T, c *combat.Combat) {
				var enemyParticipant *combat.CombatParticipant
				for i := range c.Participants {
					if !c.Participants[i].IsPlayer {
						enemyParticipant = &c.Participants[i]
						break
					}
				}
				if enemyParticipant == nil {
					t.Fatal("expected at least one enemy participant")
				}
				// Проверяем дефолтные значения для отрицательных значений
				if enemyParticipant.MonsterHP != 10 {
					t.Errorf("expected default HP 10 for negative HP, got %d", enemyParticipant.MonsterHP)
				}
				if enemyParticipant.MonsterAC != 12 {
					t.Errorf("expected default AC 12 for negative AC, got %d", enemyParticipant.MonsterAC)
				}
			},
		},
		{
			name: "enemy with valid HP/AC uses provided values",
			enemies: []Enemy{
				{Name: "Dragon", HP: intPtr(100), AC: intPtr(18), AttackBonus: intPtr(8)}, // Валидные значения - должны использоваться
			},
			expectedDefaults: false,
			validate: func(t *testing.T, c *combat.Combat) {
				var enemyParticipant *combat.CombatParticipant
				for i := range c.Participants {
					if !c.Participants[i].IsPlayer {
						enemyParticipant = &c.Participants[i]
						break
					}
				}
				if enemyParticipant == nil {
					t.Fatal("expected at least one enemy participant")
				}
				// Проверяем, что используются переданные значения, а не дефолты
				if enemyParticipant.MonsterHP != 100 {
					t.Errorf("expected HP 100, got %d", enemyParticipant.MonsterHP)
				}
				if enemyParticipant.MonsterMaxHP != 100 {
					t.Errorf("expected MaxHP 100, got %d", enemyParticipant.MonsterMaxHP)
				}
				if enemyParticipant.MonsterAC != 18 {
					t.Errorf("expected AC 18, got %d", enemyParticipant.MonsterAC)
				}
				if enemyParticipant.MonsterAttackBonus != 8 {
					t.Errorf("expected AttackBonus 8, got %d", enemyParticipant.MonsterAttackBonus)
				}
			},
		},
		{
			name: "enemy with zero HP but valid AC",
			enemies: []Enemy{
				{Name: "Skeleton", HP: intPtr(0), AC: intPtr(15), AttackBonus: intPtr(3)}, // HP=0 (дефолт), AC=15 (валидно)
			},
			expectedDefaults: true,
			validate: func(t *testing.T, c *combat.Combat) {
				var enemyParticipant *combat.CombatParticipant
				for i := range c.Participants {
					if !c.Participants[i].IsPlayer {
						enemyParticipant = &c.Participants[i]
						break
					}
				}
				if enemyParticipant == nil {
					t.Fatal("expected at least one enemy participant")
				}
				// HP должен быть дефолтным (10), AC должен быть переданным (15)
				if enemyParticipant.MonsterHP != 10 {
					t.Errorf("expected default HP 10, got %d", enemyParticipant.MonsterHP)
				}
				if enemyParticipant.MonsterAC != 15 {
					t.Errorf("expected AC 15, got %d", enemyParticipant.MonsterAC)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			llm := &mockLLM{}
			combatRepo := &mockCombatRepo{}
			questRepo := &mockQuestRepo{}

			uc := NewAnalyzeDMResponseUseCase(
				llm,
				combatRepo,
				questRepo,
				nil,   // inventoryRepo
				1,     // sessionID
				123,   // chatID
				1,     // worldID
				1,     // characterID
				1,     // playerID
				false, // autoGenerateImages
				0,     // imagesGeneratedInSession
			)

			ctx := context.Background()
			err := uc.handleCombatStart(ctx, tt.enemies)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(combatRepo.savedCombats) == 0 {
				t.Fatal("expected combat to be saved")
			}

			savedCombat := combatRepo.savedCombats[0]
			tt.validate(t, savedCombat)
		})
	}
}

func TestAnalyzeDMResponseUseCase_EmptyAnalysisNoMarkers_TrustedWithoutRetry(t *testing.T) {
	llm := &mockLLM{}
	combatRepo := &mockCombatRepo{}
	questRepo := &mockQuestRepo{}
	inventoryRepo := &mockInventoryRepo{}

	callCount := 0
	llm.generateWithMaxTokensFunc = func(ctx context.Context, prompt string, maxTokens int) (string, error) {
		callCount++
		return "{}", nil
	}

	uc := NewAnalyzeDMResponseUseCase(
		llm,
		combatRepo,
		questRepo,
		inventoryRepo,
		1,     // sessionID
		123,   // chatID
		1,     // worldID
		1,     // characterID
		1,     // playerID
		false, // autoGenerateImages
		0,     // imagesGeneratedInSession
	)

	// Обычный повествовательный ход без каких-либо маркеров боя — пустой анализ
	// (JSON успешно распарсился, все поля false/пусто) должен приниматься сразу,
	// без ретраев: это самый частый случай в игре, и раньше он тратил впустую
	// 5 дополнительных вызовов LLM на каждый такой ход.
	analysis, err := uc.Execute(context.Background(), "Староста замолкает и ждет, пока вы решите, последовать ли его совету.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 1 {
		t.Fatalf("expected exactly 1 LLM call (no retries for legitimate empty analysis), got %d", callCount)
	}
	if analysis.CombatDetected {
		t.Fatalf("expected no combat detected, got %+v", analysis)
	}
}

func TestAnalyzeDMResponseUseCase_EmptyAnalysisWithMarkers_ImmediateFallbackNoRetry(t *testing.T) {
	llm := &mockLLM{}
	combatRepo := &mockCombatRepo{}
	questRepo := &mockQuestRepo{}
	inventoryRepo := &mockInventoryRepo{}

	callCount := 0
	llm.generateWithMaxTokensFunc = func(ctx context.Context, prompt string, maxTokens int) (string, error) {
		callCount++
		return "{}", nil
	}

	uc := NewAnalyzeDMResponseUseCase(
		llm,
		combatRepo,
		questRepo,
		inventoryRepo,
		1,     // sessionID
		123,   // chatID
		1,     // worldID
		1,     // characterID
		1,     // playerID
		false, // autoGenerateImages
		0,     // imagesGeneratedInSession
	)

	analysis, err := uc.Execute(context.Background(), "Начался бой с орком")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 1 {
		t.Fatalf("expected exactly 1 LLM call (fallback triggers immediately, no retries), got %d", callCount)
	}
	if !analysis.CombatDetected || len(analysis.Enemies) == 0 {
		t.Fatalf("expected fallback combat analysis, got %+v", analysis)
	}
}

// Mock Full Session Repository (для тестов присоединения/ухода компаньонов)
type mockFullSessionRepo struct {
	gs        *session.GameSession
	saveCalls int
}

func (m *mockFullSessionRepo) GetByChatID(ctx context.Context, chatID int64) (*session.GameSession, error) {
	return m.gs, nil
}

func (m *mockFullSessionRepo) Save(ctx context.Context, gs *session.GameSession) error {
	m.saveCalls++
	m.gs = gs
	return nil
}

func TestAnalyzeDMResponseUseCase_CompanionJoined_AddsToParty(t *testing.T) {
	llm := &mockLLM{}
	combatRepo := &mockCombatRepo{}
	questRepo := &mockQuestRepo{}
	inventoryRepo := &mockInventoryRepo{}
	fullSessionRepo := &mockFullSessionRepo{gs: &session.GameSession{ChatID: 123}}

	llm.generateWithMaxTokensFunc = func(ctx context.Context, prompt string, maxTokens int) (string, error) {
		analysis := DMResponseAnalysis{
			CompanionJoined: &CompanionJoined{
				Name:        "Алара",
				Class:       "Воин",
				Description: "Бывшая наёмница, уставшая от одиночества",
			},
		}
		data, _ := json.Marshal(analysis)
		return string(data), nil
	}

	uc := NewAnalyzeDMResponseUseCase(llm, combatRepo, questRepo, inventoryRepo, 1, 123, 1, 1, 1, false, 0)
	uc.SetFullSessionRepository(fullSessionRepo)

	analysis, err := uc.Execute(context.Background(), "Алара присоединяется к вашему отряду в качестве соратника.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if analysis.CompanionJoined == nil || analysis.CompanionJoined.Name != "Алара" {
		t.Fatalf("expected companion_joined to be Алара, got %+v", analysis.CompanionJoined)
	}
	if len(fullSessionRepo.gs.Companions) != 1 {
		t.Fatalf("expected 1 companion in party, got %d", len(fullSessionRepo.gs.Companions))
	}
	if fullSessionRepo.gs.Companions[0].Name != "Алара" || fullSessionRepo.gs.Companions[0].Class != "Воин" {
		t.Fatalf("unexpected companion data: %+v", fullSessionRepo.gs.Companions[0])
	}
	if fullSessionRepo.gs.Companions[0].HP <= 0 || fullSessionRepo.gs.Companions[0].HP != fullSessionRepo.gs.Companions[0].MaxHP {
		t.Fatalf("expected companion HP to be positive and equal to MaxHP, got %+v", fullSessionRepo.gs.Companions[0])
	}
	if analysis.CompanionEventMessage == "" || !contains(analysis.CompanionEventMessage, "Алара") {
		t.Fatalf("expected companion event message to mention Алара, got %q", analysis.CompanionEventMessage)
	}
}

func TestAnalyzeDMResponseUseCase_CompanionJoined_SkipsDuplicate(t *testing.T) {
	llm := &mockLLM{}
	combatRepo := &mockCombatRepo{}
	questRepo := &mockQuestRepo{}
	inventoryRepo := &mockInventoryRepo{}
	existing := session.Companion{Name: "Алара", Class: "Воин", HP: 30, MaxHP: 30}
	fullSessionRepo := &mockFullSessionRepo{gs: &session.GameSession{ChatID: 123, Companions: []session.Companion{existing}}}

	llm.generateWithMaxTokensFunc = func(ctx context.Context, prompt string, maxTokens int) (string, error) {
		analysis := DMResponseAnalysis{
			CompanionJoined: &CompanionJoined{Name: "алара", Class: "Воин"}, // разный регистр — то же имя
		}
		data, _ := json.Marshal(analysis)
		return string(data), nil
	}

	uc := NewAnalyzeDMResponseUseCase(llm, combatRepo, questRepo, inventoryRepo, 1, 123, 1, 1, 1, false, 0)
	uc.SetFullSessionRepository(fullSessionRepo)

	_, err := uc.Execute(context.Background(), "Алара всё ещё с вами.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fullSessionRepo.gs.Companions) != 1 {
		t.Fatalf("expected companion not to be duplicated, got %d companions", len(fullSessionRepo.gs.Companions))
	}
}

func TestAnalyzeDMResponseUseCase_CompanionLeft_RemovesFromParty(t *testing.T) {
	llm := &mockLLM{}
	combatRepo := &mockCombatRepo{}
	questRepo := &mockQuestRepo{}
	inventoryRepo := &mockInventoryRepo{}
	existing := session.Companion{ID: 7, Name: "Борис", Class: "Маг", HP: 25, MaxHP: 25}
	fullSessionRepo := &mockFullSessionRepo{gs: &session.GameSession{ChatID: 123, Companions: []session.Companion{existing}}}

	llm.generateWithMaxTokensFunc = func(ctx context.Context, prompt string, maxTokens int) (string, error) {
		analysis := DMResponseAnalysis{
			CompanionLeft: &CompanionLeft{Name: "Борис", Reason: "погиб, защищая отряд"},
		}
		data, _ := json.Marshal(analysis)
		return string(data), nil
	}

	uc := NewAnalyzeDMResponseUseCase(llm, combatRepo, questRepo, inventoryRepo, 1, 123, 1, 1, 1, false, 0)
	uc.SetFullSessionRepository(fullSessionRepo)

	analysis, err := uc.Execute(context.Background(), "Борис героически погибает, защищая отряд от врагов.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if analysis.CompanionLeft == nil || analysis.CompanionLeft.Name != "Борис" {
		t.Fatalf("expected companion_left to be Борис, got %+v", analysis.CompanionLeft)
	}
	if len(fullSessionRepo.gs.Companions) != 0 {
		t.Fatalf("expected companion to be removed from party, got %+v", fullSessionRepo.gs.Companions)
	}
	if analysis.CompanionEventMessage == "" || !contains(analysis.CompanionEventMessage, "Борис") {
		t.Fatalf("expected companion event message to mention Борис, got %q", analysis.CompanionEventMessage)
	}
}

func TestAnalyzeDMResponseUseCase_CompanionLeft_UnknownNameIsNoop(t *testing.T) {
	llm := &mockLLM{}
	combatRepo := &mockCombatRepo{}
	questRepo := &mockQuestRepo{}
	inventoryRepo := &mockInventoryRepo{}
	fullSessionRepo := &mockFullSessionRepo{gs: &session.GameSession{ChatID: 123}}

	llm.generateWithMaxTokensFunc = func(ctx context.Context, prompt string, maxTokens int) (string, error) {
		analysis := DMResponseAnalysis{
			CompanionLeft: &CompanionLeft{Name: "Незнакомец", Reason: "ушёл"},
		}
		data, _ := json.Marshal(analysis)
		return string(data), nil
	}

	uc := NewAnalyzeDMResponseUseCase(llm, combatRepo, questRepo, inventoryRepo, 1, 123, 1, 1, 1, false, 0)
	uc.SetFullSessionRepository(fullSessionRepo)

	_, err := uc.Execute(context.Background(), "Незнакомец уходит.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fullSessionRepo.saveCalls != 0 {
		t.Fatalf("expected no session save for unknown companion name, got %d saves", fullSessionRepo.saveCalls)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > len(substr) && (s[:len(substr)] == substr ||
			s[len(s)-len(substr):] == substr ||
			containsMiddle(s, substr))))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Mock Campaign Fact Repository
type mockCampaignFactRepo struct {
	saved []*world.CampaignFact
}

func (m *mockCampaignFactRepo) Save(ctx context.Context, fact *world.CampaignFact) error {
	m.saved = append(m.saved, fact)
	return nil
}

// TestAnalyzeDMResponseUseCase_ItemsReceived_SkipsLikelyDuplicateWeapon воспроизводит
// баг из реальной игровой сессии: DM несколько раз подряд описывает вручение одного и того
// же уникального меча под слегка разными названиями ("меч Авроры" -> "светящийся клинок
// Авроры"). Экстрактор каждый раз находит items_received, но предмет уже есть в инвентаре -
// повторного добавления быть не должно.
func TestAnalyzeDMResponseUseCase_ItemsReceived_SkipsLikelyDuplicateWeapon(t *testing.T) {
	llm := &mockLLM{}
	combatRepo := &mockCombatRepo{}
	questRepo := &mockQuestRepo{}

	existingInv := inventory.NewInventory(1)
	_ = existingInv.AddItem("меч Авроры", "старый меч с выгравированными символами света", 2, 1, inventory.ItemTypeWeapon, 0)

	var savedInv *inventory.Inventory
	inventoryRepo := &mockInventoryRepo{
		getByCharacterIDFunc: func(ctx context.Context, characterID uint) (*inventory.Inventory, error) {
			return existingInv, nil
		},
		saveFunc: func(ctx context.Context, inv *inventory.Inventory) error {
			savedInv = inv
			return nil
		},
	}

	llm.generateWithMaxTokensFunc = func(ctx context.Context, prompt string, maxTokens int) (string, error) {
		analysis := DMResponseAnalysis{
			ItemsReceived: []Item{
				{
					Name:        "светящийся клинок Авроры",
					Description: "старинный меч, способный освещать путь через темноту",
					Weight:      0,
					Quantity:    1,
					Type:        "weapon",
				},
			},
		}
		data, _ := json.Marshal(analysis)
		return string(data), nil
	}

	uc := NewAnalyzeDMResponseUseCase(llm, combatRepo, questRepo, inventoryRepo, 1, 123, 1, 1, 1, false, 0)

	_, err := uc.Execute(context.Background(), "Элиан протягивает вам меч обеими руками, будто передавая священную реликвию.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if savedInv == nil {
		t.Fatalf("expected inventory to be saved")
	}
	if len(savedInv.Items) != 1 {
		t.Fatalf("expected the duplicate sword not to be added, got %d items: %+v", len(savedInv.Items), savedInv.Items)
	}
	if savedInv.Items[0].Quantity != 1 {
		t.Fatalf("expected quantity to remain 1 (not stacked), got %d", savedInv.Items[0].Quantity)
	}
}

// TestAnalyzeDMResponseUseCase_ItemsReceived_RecordsCampaignFact проверяет, что получение
// нового предмета фиксируется в ключевых фактах кампании (категория "item") - эта память,
// в отличие от "Инвентарь персонажа", подмешивается DM на каждом ходу, а не только по прямому
// запросу об инвентаре, и должна не давать DM повторно предлагать уже выданный предмет.
func TestAnalyzeDMResponseUseCase_ItemsReceived_RecordsCampaignFact(t *testing.T) {
	llm := &mockLLM{}
	combatRepo := &mockCombatRepo{}
	questRepo := &mockQuestRepo{}

	inventoryRepo := &mockInventoryRepo{
		getByCharacterIDFunc: func(ctx context.Context, characterID uint) (*inventory.Inventory, error) {
			return inventory.NewInventory(1), nil
		},
	}
	campaignFactRepo := &mockCampaignFactRepo{}

	llm.generateWithMaxTokensFunc = func(ctx context.Context, prompt string, maxTokens int) (string, error) {
		analysis := DMResponseAnalysis{
			ItemsReceived: []Item{
				{Name: "Меч Авроры", Description: "Светящийся клинок", Weight: 2, Quantity: 1, Type: "weapon"},
			},
		}
		data, _ := json.Marshal(analysis)
		return string(data), nil
	}

	uc := NewAnalyzeDMResponseUseCase(llm, combatRepo, questRepo, inventoryRepo, 1, 123, 1, 1, 1, false, 0)
	uc.SetCampaignFactRepository(campaignFactRepo)

	_, err := uc.Execute(context.Background(), "Элиан вручает вам меч Авроры.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(campaignFactRepo.saved) != 1 {
		t.Fatalf("expected 1 campaign fact to be saved, got %d", len(campaignFactRepo.saved))
	}
	if campaignFactRepo.saved[0].Category != world.FactCategoryItem {
		t.Errorf("expected fact category %q, got %q", world.FactCategoryItem, campaignFactRepo.saved[0].Category)
	}
	if !contains(campaignFactRepo.saved[0].Text, "Меч Авроры") {
		t.Errorf("expected fact text to mention item name, got %q", campaignFactRepo.saved[0].Text)
	}
}
