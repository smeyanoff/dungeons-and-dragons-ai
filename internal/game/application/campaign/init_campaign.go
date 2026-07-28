package campaign

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	jsonrepair "dungeons-and-dragons-ai/internal/game/application/jsonrepair"
	"dungeons-and-dragons-ai/internal/game/domain/item"
	"dungeons-and-dragons-ai/internal/game/domain/location"
	"dungeons-and-dragons-ai/internal/game/domain/npc"
	"dungeons-and-dragons-ai/internal/game/domain/quest"
	"dungeons-and-dragons-ai/internal/game/domain/world"
	"dungeons-and-dragons-ai/internal/llm/domain"
	"dungeons-and-dragons-ai/pkg/logger"
)

type WorldRepository interface {
	Save(ctx context.Context, w *world.World) error
}

// CampaignFactRepository — репозиторий устойчивых фактов кампании (память "кто есть кто",
// не привязанная к локации). Опционален: если не задан, посев фактов при генерации мира пропускается.
type CampaignFactRepository interface {
	Save(ctx context.Context, fact *world.CampaignFact) error
}

type InitCampaignUseCase struct {
	llm              domain.LLM
	worldRepo        WorldRepository
	campaignFactRepo CampaignFactRepository
}

func NewInitCampaignUseCase(llm domain.LLM, worldRepo WorldRepository) *InitCampaignUseCase {
	return &InitCampaignUseCase{
		llm:       llm,
		worldRepo: worldRepo,
	}
}

// SetCampaignFactRepository устанавливает репозиторий для посева фактов идентичности NPC
// (родство/принадлежность) в память кампании сразу при генерации мира.
func (uc *InitCampaignUseCase) SetCampaignFactRepository(repo CampaignFactRepository) {
	uc.campaignFactRepo = repo
}

// seedNPCIdentityFacts сохраняет факт "кто есть кто" для каждого NPC с заполненным Relation,
// сгенерированный вместе с миром. Ошибки записи не прерывают создание кампании — это
// дополнительная память, а не критический путь.
func (uc *InitCampaignUseCase) seedNPCIdentityFacts(ctx context.Context, worldID uint, locations []LocationDTO) {
	if uc.campaignFactRepo == nil || worldID == 0 {
		return
	}
	for _, locDTO := range locations {
		for _, npcDTO := range locDTO.NPCs {
			name := strings.TrimSpace(npcDTO.Name)
			relation := strings.TrimSpace(npcDTO.Relation)
			if name == "" || relation == "" {
				continue
			}
			fact := &world.CampaignFact{
				WorldID:  worldID,
				Category: world.FactCategoryNPCIdentity,
				Text:     fmt.Sprintf("%s — %s", name, relation),
			}
			if err := uc.campaignFactRepo.Save(ctx, fact); err != nil {
				logger.Warn("Failed to seed NPC identity fact",
					logger.String("npc_name", name),
					logger.Uint("world_id", worldID),
					logger.ErrorField(err),
				)
			}
		}
	}
}

const (
// Важно: при генерации мира не задаем max_tokens, чтобы не провоцировать
// остановки модели и обрезанные JSON ответы.
)

func getEnvInt(name string, defaultValue int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return defaultValue
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return defaultValue
	}
	return v
}

func (uc *InitCampaignUseCase) Execute(
	ctx context.Context,
	worldTheme string,
) (*world.World, error) {
	// Шаг 1: Генерация главного квеста
	mainQuest, err := uc.generateMainQuest(ctx, worldTheme)
	if err != nil {
		return nil, fmt.Errorf("failed to generate main quest: %w", err)
	}

	// Шаг 2: Генерация списка локаций (базовая информация, без проверок)
	locations, err := uc.generateLocations(ctx, worldTheme, mainQuest.Title)
	if err != nil {
		return nil, fmt.Errorf("failed to generate locations: %w", err)
	}

	// Шаг 3: Генерация деталей для каждой локации (NPC)
	for i := range locations {
		npcs, err := uc.generateLocationNPCs(ctx, locations[i].Name, locations[i].Description)
		if err != nil {
			logger.Warn("Failed to generate NPCs for location",
				logger.String("location", locations[i].Name),
				logger.ErrorField(err),
			)
			// Продолжаем без NPC для этой локации
		} else {
			locations[i].NPCs = npcs
		}
	}

	// Шаг 4: Генерация связей между локациями
	connections, err := uc.generateConnections(ctx, locations, mainQuest.Items)
	if err != nil {
		logger.Warn("Failed to generate connections",
			logger.ErrorField(err),
		)
		// Продолжаем без связей
	} else {
		// Применяем связи к локациям
		for i, loc := range locations {
			if conns, ok := connections[loc.Name]; ok {
				locations[i].Connections = conns
			}
		}
	}

	// Создаем мир из сгенерированных данных
	w, err := uc.buildWorld(ctx, mainQuest, locations)
	if err != nil {
		return nil, err
	}

	// Шаг 4.5: Сохраняем идентичность NPC (родство/принадлежность) в память кампании сразу
	// при создании мира — до того, как DM успеет сымпровизировать и позже себе противоречить.
	uc.seedNPCIdentityFacts(ctx, w.ID, locations)

	// Шаг 5: Playbook для каждой локации (hook → 2–4 сцены → критический выбор → последствия; связь с NPC и квестовыми предметами).
	// Включается переменной окружения ENABLE_PLAYBOOK_GENERATION=true (в тестах по умолчанию выключено).
	if os.Getenv("ENABLE_PLAYBOOK_GENERATION") == "true" {
		for i := range w.Locations {
			npcNames := make([]string, 0, len(w.Locations[i].NPCs))
			for _, n := range w.Locations[i].NPCs {
				npcNames = append(npcNames, n.Name)
			}
			var questItemNames []string
			if w.MainQuest != nil {
				for _, it := range w.MainQuest.Items {
					questItemNames = append(questItemNames, it.Name)
				}
			}
			scenario, err := uc.generateLocationPlaybook(ctx, w.Locations[i].Name, w.Locations[i].Description, npcNames, questItemNames)
			if err != nil {
				logger.Warn("Failed to generate location playbook",
					logger.String("location", w.Locations[i].Name),
					logger.ErrorField(err),
				)
				continue
			}
			if scenario != nil {
				if err := w.Locations[i].SetScenario(scenario); err != nil {
					logger.Warn("Failed to set location scenario", logger.String("location", w.Locations[i].Name), logger.ErrorField(err))
					continue
				}
			}
		}
		if err := uc.worldRepo.Save(ctx, w); err != nil {
			logger.Warn("Failed to save world after playbook generation", logger.ErrorField(err))
		}
	}
	return w, nil
}

// generateMainQuest генерирует главный квест
func (uc *InitCampaignUseCase) generateMainQuest(ctx context.Context, worldTheme string) (*QuestDTO, error) {
	return uc.generateMainQuestWithRetry(ctx, worldTheme, 0)
}

// generateMainQuestWithRetry генерирует главный квест с retry механизмом
func (uc *InitCampaignUseCase) generateMainQuestWithRetry(ctx context.Context, worldTheme string, attempt int) (*QuestDTO, error) {
	const maxRetries = 3 // Увеличиваем количество retry для борьбы с deadline exceeded
	if attempt > maxRetries {
		logger.Warn("Failed to generate valid main quest, using fallback",
			logger.Int("attempts", maxRetries+1),
		)
		return fallbackMainQuest(worldTheme), nil
	}

	// Добавляем exponential backoff для retry
	if attempt > 0 {
		backoff := time.Duration(attempt) * 5 * time.Second // 5s, 10s, 15s
		if backoff > 30*time.Second {
			backoff = 30 * time.Second
		}
		logger.Info("Retrying main quest generation",
			logger.Int("attempt", attempt),
			logger.Duration("backoff", backoff),
		)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}
	}

	prompt := GenerateMainQuestPrompt(worldTheme)
	if attempt > 0 {
		// Более строгий промпт для retry
		prompt = GenerateMainQuestPromptStrict(worldTheme)
	}

	llmCtx, llmCancel := context.WithTimeout(ctx, 90*time.Second)
	defer llmCancel()

	logger.Info("Generating main quest",
		logger.Int("attempt", attempt),
		logger.Int("prompt_length", len(prompt)),
		logger.Duration("timeout", 90*time.Second),
	)

	startTime := time.Now()
	raw, err := uc.llm.Generate(llmCtx, prompt)
	duration := time.Since(startTime)

	if err != nil {
		logger.Error("Main quest generation failed",
			logger.ErrorField(err),
			logger.Int("attempt", attempt),
			logger.Duration("duration", duration),
		)
		return nil, err
	}

	logger.Info("Main quest generation completed",
		logger.Int("attempt", attempt),
		logger.Duration("duration", duration),
		logger.Int("response_length", len(raw)),
	)

	cleaned := cleanJSONResponse(raw)
	if !json.Valid([]byte(cleaned)) {
		// Для ужесточения контрактов: repair только на первой попытке и только для потенциально truncated JSON
		if attempt == 0 && looksLikeTruncatedJSON(cleaned) {
			logger.Warn("LLM response looks like truncated JSON, attempting minimal repair",
				logger.Int("attempt", attempt),
				logger.Int("response_length", len(cleaned)),
			)
			cleaned = tryRepairTruncatedJSON(cleaned)
			if !json.Valid([]byte(cleaned)) {
				logger.Warn("Failed to repair truncated JSON, will retry with stricter prompt",
					logger.Int("attempt", attempt),
				)
				return uc.generateMainQuestWithRetry(ctx, worldTheme, attempt+1)
			} else {
				logger.Info("Successfully repaired truncated JSON",
					logger.Int("attempt", attempt),
				)
			}
		} else {
			logger.Warn("LLM response is not valid JSON, retrying with stricter prompt",
				logger.Int("attempt", attempt),
				logger.Int("response_length", len(cleaned)),
			)
			return uc.generateMainQuestWithRetry(ctx, worldTheme, attempt+1)
		}
	}

	var quest QuestDTO
	if err := decodeStrictJSON(cleaned, &quest); err != nil {
		logger.Warn("Failed to parse main quest JSON",
			logger.Int("attempt", attempt),
			logger.ErrorField(err),
			logger.Int("response_length", len(raw)),
			logger.String("cleaned_preview", cleaned[:min(500, len(cleaned))]),
		)
		return uc.generateMainQuestWithRetry(ctx, worldTheme, attempt+1)
	}

	if quest.Title == "" || quest.Description == "" {
		logger.Warn("Main quest validation failed - title or description is empty",
			logger.Int("attempt", attempt),
			logger.String("title", quest.Title),
			logger.String("description", quest.Description),
		)
		if attempt >= maxRetries {
			logger.Warn("Main quest validation failed after max retries, using fallback",
				logger.Int("attempts", maxRetries+1),
			)
			return fallbackMainQuest(worldTheme), nil
		}
		return uc.generateMainQuestWithRetry(ctx, worldTheme, attempt+1)
	}

	return &quest, nil
}

// generateLocations генерирует список локаций
func (uc *InitCampaignUseCase) generateLocations(ctx context.Context, worldTheme, mainQuestTitle string) ([]LocationDTO, error) {
	return uc.generateLocationsWithRetry(ctx, worldTheme, mainQuestTitle, 0)
}

// generateLocationsWithRetry генерирует список локаций с retry механизмом
func (uc *InitCampaignUseCase) generateLocationsWithRetry(ctx context.Context, worldTheme, mainQuestTitle string, attempt int) ([]LocationDTO, error) {
	const maxRetries = 1 // Уменьшаем количество retry для ужесточения контрактов
	if attempt > maxRetries {
		logger.Warn("Failed to generate valid locations, using fallback",
			logger.Int("attempts", maxRetries+1),
		)
		return fallbackLocations(worldTheme), nil
	}

	prompt := GenerateLocationsPrompt(worldTheme, mainQuestTitle)
	if attempt > 0 {
		// Более строгий промпт для retry
		prompt = GenerateLocationsPromptStrict(worldTheme, mainQuestTitle)
	}

	llmCtx, llmCancel := context.WithTimeout(ctx, 90*time.Second)
	defer llmCancel()

	logger.Debug("Generating locations",
		logger.Int("prompt_length", len(prompt)),
	)
	raw, err := uc.llm.Generate(llmCtx, prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM error: %w", err)
	}

	cleaned := cleanJSONResponse(raw)
	if !json.Valid([]byte(cleaned)) {
		// Для ужесточения контрактов: repair только на первой попытке и только для потенциально truncated JSON
		if attempt == 0 && looksLikeTruncatedJSON(cleaned) {
			logger.Warn("LLM response for locations looks like truncated JSON, attempting minimal repair",
				logger.Int("attempt", attempt),
				logger.Int("response_length", len(cleaned)),
			)
			cleaned = tryRepairTruncatedJSON(cleaned)
			if !json.Valid([]byte(cleaned)) {
				logger.Warn("Failed to repair truncated JSON for locations, will retry with stricter prompt",
					logger.Int("attempt", attempt),
				)
				return uc.generateLocationsWithRetry(ctx, worldTheme, mainQuestTitle, attempt+1)
			} else {
				logger.Info("Successfully repaired truncated JSON for locations",
					logger.Int("attempt", attempt),
				)
			}
		} else {
			logger.Warn("LLM response for locations is not valid JSON, retrying with stricter prompt",
				logger.Int("attempt", attempt),
				logger.Int("response_length", len(cleaned)),
			)
			return uc.generateLocationsWithRetry(ctx, worldTheme, mainQuestTitle, attempt+1)
		}
	}

	var response struct {
		Locations []LocationDTO `json:"locations"`
	}
	if err := decodeStrictJSON(cleaned, &response); err != nil {
		logger.Warn("Failed to parse locations JSON",
			logger.Int("attempt", attempt),
			logger.ErrorField(err),
			logger.Int("response_length", len(raw)),
			logger.String("cleaned_preview", cleaned[:min(500, len(cleaned))]),
		)
		return uc.generateLocationsWithRetry(ctx, worldTheme, mainQuestTitle, attempt+1)
	}

	if len(response.Locations) == 0 {
		logger.Warn("No locations generated, retrying",
			logger.Int("attempt", attempt),
		)
		if attempt >= maxRetries {
			logger.Warn("No locations generated after max retries, using fallback",
				logger.Int("attempts", maxRetries+1),
			)
			return fallbackLocations(worldTheme), nil
		}
		return uc.generateLocationsWithRetry(ctx, worldTheme, mainQuestTitle, attempt+1)
	}

	return response.Locations, nil
}

// generateLocationNPCs генерирует NPC для локации
func (uc *InitCampaignUseCase) generateLocationNPCs(ctx context.Context, locationName, locationDescription string) ([]NPCDTO, error) {
	return uc.generateLocationNPCsWithRetry(ctx, locationName, locationDescription, 0)
}

// generateLocationNPCsWithRetry генерирует NPC для локации с retry механизмом
func (uc *InitCampaignUseCase) generateLocationNPCsWithRetry(ctx context.Context, locationName, locationDescription string, attempt int) ([]NPCDTO, error) {
	const maxRetries = 1 // Уменьшаем количество retry для ужесточения контрактов
	if attempt > maxRetries {
		logger.Warn("Failed to generate valid NPCs, using empty fallback",
			logger.Int("attempts", maxRetries+1),
			logger.String("location", locationName),
		)
		return []NPCDTO{}, nil
	}

	prompt := GenerateLocationNPCsPrompt(locationName, locationDescription)
	if attempt > 0 {
		prompt = GenerateLocationNPCsPromptStrict(locationName, locationDescription)
	}

	llmCtx, llmCancel := context.WithTimeout(ctx, 20*time.Second)
	defer llmCancel()

	logger.Debug("Generating NPCs for location",
		logger.String("location_name", locationName),
		logger.Int("prompt_length", len(prompt)),
	)
	raw, err := uc.llm.Generate(llmCtx, prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM error: %w", err)
	}

	cleaned := cleanJSONResponse(raw)
	if !json.Valid([]byte(cleaned)) {
		// Для ужесточения контрактов: repair только на первой попытке и только для потенциально truncated JSON
		if attempt == 0 && looksLikeTruncatedJSON(cleaned) {
			logger.Warn("LLM response for NPCs looks like truncated JSON, attempting minimal repair",
				logger.Int("attempt", attempt),
				logger.String("location", locationName),
			)
			cleaned = tryRepairTruncatedJSON(cleaned)
			if !json.Valid([]byte(cleaned)) {
				logger.Warn("Failed to repair truncated JSON for NPCs, will retry with stricter prompt",
					logger.Int("attempt", attempt),
				)
				return uc.generateLocationNPCsWithRetry(ctx, locationName, locationDescription, attempt+1)
			} else {
				logger.Info("Successfully repaired truncated JSON for NPCs",
					logger.Int("attempt", attempt),
				)
			}
		} else {
			logger.Warn("LLM response for NPCs is not valid JSON, retrying with stricter prompt",
				logger.Int("attempt", attempt),
			)
			return uc.generateLocationNPCsWithRetry(ctx, locationName, locationDescription, attempt+1)
		}
	}

	var response struct {
		NPCs []NPCDTO `json:"npcs"`
	}
	if err := decodeStrictJSON(cleaned, &response); err != nil {
		logger.Warn("Failed to parse NPCs JSON",
			logger.Int("attempt", attempt),
			logger.ErrorField(err),
		)
		return uc.generateLocationNPCsWithRetry(ctx, locationName, locationDescription, attempt+1)
	}

	return response.NPCs, nil
}

// generateConnections генерирует связи между локациями. questItems — предметы главного
// квеста, которыми (изредка) можно заблокировать отдельные связи через required_item.
func (uc *InitCampaignUseCase) generateConnections(ctx context.Context, locations []LocationDTO, questItems []ItemDTO) (map[string][]ConnectionDTO, error) {
	connections, err := uc.generateConnectionsWithRetry(ctx, locations, questItems, 0)
	if err != nil {
		return nil, err
	}
	return sanitizeRequiredItems(connections, questItems), nil
}

// generateConnectionsWithRetry генерирует связи между локациями с retry механизмом
func (uc *InitCampaignUseCase) generateConnectionsWithRetry(ctx context.Context, locations []LocationDTO, questItems []ItemDTO, attempt int) (map[string][]ConnectionDTO, error) {
	const maxRetries = 1 // Уменьшаем количество retry для ужесточения контрактов
	if attempt > maxRetries {
		logger.Warn("Failed to generate valid connections, using fallback",
			logger.Int("attempts", maxRetries+1),
		)
		return fallbackConnections(locations), nil
	}

	questItemNames := make([]string, 0, len(questItems))
	for _, it := range questItems {
		questItemNames = append(questItemNames, it.Name)
	}

	prompt := GenerateConnectionsPrompt(locations, questItemNames)
	if attempt > 0 {
		prompt = GenerateConnectionsPromptStrict(locations, questItemNames)
	}

	llmCtx, llmCancel := context.WithTimeout(ctx, 90*time.Second)
	defer llmCancel()

	logger.Debug("Generating location connections",
		logger.Int("prompt_length", len(prompt)),
	)
	raw, err := uc.llm.Generate(llmCtx, prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM error: %w", err)
	}

	cleaned := cleanJSONResponse(raw)
	if !json.Valid([]byte(cleaned)) {
		// Для ужесточения контрактов: repair только на первой попытке и только для потенциально truncated JSON
		if attempt == 0 && looksLikeTruncatedJSON(cleaned) {
			logger.Warn("LLM response for connections looks like truncated JSON, attempting minimal repair",
				logger.Int("attempt", attempt),
				logger.Int("response_length", len(cleaned)),
			)
			cleaned = tryRepairTruncatedJSON(cleaned)
			if !json.Valid([]byte(cleaned)) {
				logger.Warn("Failed to repair truncated JSON for connections, will retry with stricter prompt",
					logger.Int("attempt", attempt),
				)
				return uc.generateConnectionsWithRetry(ctx, locations, questItems, attempt+1)
			} else {
				logger.Info("Successfully repaired truncated JSON for connections",
					logger.Int("attempt", attempt),
				)
			}
		} else {
			logger.Warn("LLM response for connections is not valid JSON, retrying with stricter prompt",
				logger.Int("attempt", attempt),
				logger.Int("response_length", len(cleaned)),
			)
			return uc.generateConnectionsWithRetry(ctx, locations, questItems, attempt+1)
		}
	}

	var response struct {
		Connections map[string][]ConnectionDTO `json:"connections"`
	}
	if err := decodeStrictJSON(cleaned, &response); err != nil {
		logger.Warn("Failed to parse connections JSON",
			logger.Int("attempt", attempt),
			logger.ErrorField(err),
		)
		if attempt >= maxRetries {
			logger.Warn("Failed to parse connections after max retries, using fallback",
				logger.Int("attempts", maxRetries+1),
			)
			return fallbackConnections(locations), nil
		}
		return uc.generateConnectionsWithRetry(ctx, locations, questItems, attempt+1)
	}

	return response.Connections, nil
}

// playbookResponse — DTO для ответа LLM по playbook локации.
type playbookResponse struct {
	Hook                  string   `json:"hook"`
	KeyEvents             []string `json:"key_events"`
	CriticalChoice        string   `json:"critical_choice"`
	PossibleOutcomes      []string `json:"possible_outcomes"`
	RelatedNPCNames       []string `json:"related_npc_names"`
	RelatedQuestItemNames []string `json:"related_quest_item_names"`
}

// generateLocationPlaybook генерирует playbook локации (hook → 2–4 сцены → критический выбор → последствия) и связь с NPC/квестовыми предметами.
func (uc *InitCampaignUseCase) generateLocationPlaybook(
	ctx context.Context,
	locName, locDesc string,
	npcNames, questItemNames []string,
) (*world.LocationScenario, error) {
	prompt := GenerateLocationPlaybookPrompt(locName, locDesc, npcNames, questItemNames)
	genCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	raw, err := uc.llm.Generate(genCtx, prompt)
	if err != nil {
		return nil, err
	}
	cleaned := cleanJSONResponse(raw)
	if !json.Valid([]byte(cleaned)) {
		return nil, fmt.Errorf("invalid JSON from LLM")
	}
	var resp playbookResponse
	if err := json.Unmarshal([]byte(cleaned), &resp); err != nil {
		return nil, err
	}
	scenario := &world.LocationScenario{
		ID:                    fmt.Sprintf("playbook_%s", strings.ReplaceAll(locName, " ", "_")),
		Title:                 locName,
		Description:           locDesc,
		Objective:             "",
		Hook:                  resp.Hook,
		KeyEvents:             resp.KeyEvents,
		CriticalChoice:        resp.CriticalChoice,
		PossibleOutcomes:      resp.PossibleOutcomes,
		Reward:                "",
		Status:                "not_started",
		CreatedAt:             time.Now().Format(time.RFC3339),
		RelatedNPCNames:       resp.RelatedNPCNames,
		RelatedQuestItemNames: resp.RelatedQuestItemNames,
	}
	return scenario, nil
}

// buildWorld создает мир из сгенерированных данных
func (uc *InitCampaignUseCase) buildWorld(
	ctx context.Context,
	mainQuest *QuestDTO,
	locations []LocationDTO,
) (*world.World, error) {
	// 1️⃣ создаём мир
	w := world.New(mainQuest.Title)

	// 2️⃣ главный квест
	q := quest.New(mainQuest.Title, mainQuest.Description)
	for _, it := range mainQuest.Items {
		q.AddItem(item.New(it.Name, it.Purpose))
	}
	w.SetMainQuest(q)

	// 3️⃣ локации и NPC (без связей пока)
	for _, locDTO := range locations {
		loc := location.New(locDTO.Name, locDTO.Description)
		for _, npcDTO := range locDTO.NPCs {
			loc.AddNPC(npc.New(npcDTO.Name, npcDTO.Role, normalizeAttitude(npcDTO.Attitude)))
		}

		// Добавляем локацию без предопределенных проверок
		w.AddLocation(loc)
	}

	// 4️⃣ сохраняем мир (чтобы получить ID для локаций)
	dbCtx, dbCancel := context.WithTimeout(ctx, 10*time.Second)
	defer dbCancel()
	if err := uc.worldRepo.Save(dbCtx, w); err != nil {
		return nil, err
	}

	// 5️⃣ добавляем связи между локациями
	locationNameToID := make(map[string]uint)
	for i := range w.Locations {
		locationNameToID[w.Locations[i].Name] = w.Locations[i].ID
	}

	for i, locDTO := range locations {
		fromID, exists := locationNameToID[locDTO.Name]
		if !exists || i >= len(w.Locations) {
			continue
		}

		for _, connDTO := range locDTO.Connections {
			toID, exists := locationNameToID[connDTO.ToLocation]
			if !exists {
				continue
			}

			connection := world.LocationConnection{
				FromLocationID:   fromID,
				ToLocationID:     toID,
				Direction:        connDTO.Direction,
				Description:      connDTO.Description,
				RequiredItemName: connDTO.RequiredItem,
			}
			w.Locations[i].Connections = append(w.Locations[i].Connections, connection)
		}
	}

	// 6️⃣ сохраняем мир снова с обновленными связями
	if err := uc.worldRepo.Save(dbCtx, w); err != nil {
		return nil, err
	}

	return w, nil
}

// GenerateOpeningMessage генерирует вступительное сообщение от DM: описание мира и главного квеста.
// Вызывается сразу после создания мира. При ошибке вызывающий код должен показать статичное приветствие.
func (uc *InitCampaignUseCase) GenerateOpeningMessage(ctx context.Context, w *world.World) (string, error) {
	if w == nil || w.MainQuest == nil {
		return "", fmt.Errorf("world or main quest is nil")
	}

	worldDesc := w.Description
	if worldDesc == "" || worldDesc == w.Name {
		worldDesc = w.Name + " — мир приключений."
	}

	var locList string
	if len(w.Locations) > 0 {
		names := make([]string, 0, len(w.Locations))
		for _, loc := range w.Locations {
			names = append(names, loc.Name)
		}
		locList = strings.Join(names, ", ")
	}

	prompt := fmt.Sprintf(`Ты — Мастер подземелий (Dungeon Master) в D&D 5e.

Только что создан мир. Опиши его игроку и представь главный квест — одним вступительным сообщением от лица DM.

Данные мира:
- Название мира: %s
- Описание/атмосфера: %s
- Главный квест: «%s» — %s
- Локации мира: %s

Требования:
- Ответь на русском языке.
- 2–4 коротких абзаца: сначала атмосфера мира, затем представь главный квест так, чтобы игроку было понятно, с чего начать.
- Пиши от первого лица как DM (обращайся к игроку «ты»).
- Не проси бросать кубики и не добавляй проверки — только описание и постановка квеста.
- Без технических пометок, без JSON, без markdown-блоков кода.`,
		w.Name,
		worldDesc,
		w.MainQuest.Title,
		w.MainQuest.Description,
		locList,
	)

	genCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	msg, err := uc.llm.Generate(genCtx, prompt)
	if err != nil {
		logger.Warn("Failed to generate opening message from DM",
			logger.ErrorField(err),
			logger.String("world_name", w.Name),
		)
		return "", err
	}

	msg = strings.TrimSpace(msg)
	if msg == "" {
		return "", fmt.Errorf("empty opening message from LLM")
	}

	return msg, nil
}

// normalizeAttitude приводит значение attitude, полученное от LLM, к одному из допустимых
// значений ("hostile", "wary", "neutral", "friendly"). Неизвестное или пустое значение
// трактуется как "neutral", чтобы NPC без явно заданного отношения не становились
// непреднамеренно враждебными или дружелюбными.
func normalizeAttitude(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "hostile":
		return "hostile"
	case "wary":
		return "wary"
	case "friendly":
		return "friendly"
	default:
		return "neutral"
	}
}

// cleanJSONResponse очищает ответ LLM от markdown блоков кода
func cleanJSONResponse(raw string) string {
	return jsonrepair.Clean(raw)
}

func decodeStrictJSON(input string, target interface{}) error {
	dec := json.NewDecoder(strings.NewReader(input))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("extra data after JSON")
	}
	return nil
}

func fallbackMainQuest(worldTheme string) *QuestDTO {
	title := fmt.Sprintf("Тайна мира: %s", strings.TrimSpace(worldTheme))
	if strings.TrimSpace(worldTheme) == "" {
		title = "Тайна мира"
	}
	return &QuestDTO{
		Title:       title,
		Description: "В мире появилась угроза, которую нужно остановить. Найдите ключи, раскройте древнюю тайну и восстановите равновесие.",
		Items: []ItemDTO{
			{Name: "Древний ключ", Purpose: "Открывает доступ к забытому святилищу"},
			{Name: "Карта путей", Purpose: "Указывает путь к важным локациям"},
		},
	}
}

func fallbackLocations(worldTheme string) []LocationDTO {
	theme := strings.TrimSpace(worldTheme)
	if theme == "" {
		theme = "фэнтезийный мир"
	}
	return []LocationDTO{
		{Name: "Деревня У развилки", Description: fmt.Sprintf("Мирная деревня у дороги в %s: таверна, рынок, жители.", theme)},
		{Name: "Древний лес", Description: "Таинственный лес с шепчущими деревьями и скрытыми тропами."},
		{Name: "Забытые руины", Description: "Разрушенные руины, где хранятся ответы на главный квест."},
	}
}

// sanitizeRequiredItems отбрасывает required_item, придуманные LLM и не совпадающие ни с
// одним реальным предметом главного квеста — иначе партия могла бы получить путь,
// заблокированный несуществующим предметом, который никогда не попадёт в инвентарь.
func sanitizeRequiredItems(connections map[string][]ConnectionDTO, questItems []ItemDTO) map[string][]ConnectionDTO {
	known := make(map[string]bool, len(questItems))
	for _, it := range questItems {
		known[strings.ToLower(strings.TrimSpace(it.Name))] = true
	}

	for from, conns := range connections {
		for i := range conns {
			req := strings.TrimSpace(conns[i].RequiredItem)
			if req == "" {
				continue
			}
			if !known[strings.ToLower(req)] {
				logger.Warn("Dropping hallucinated required_item on generated connection",
					logger.String("from", from),
					logger.String("to", conns[i].ToLocation),
					logger.String("required_item", req),
				)
				conns[i].RequiredItem = ""
			}
		}
	}
	return connections
}

func fallbackConnections(locations []LocationDTO) map[string][]ConnectionDTO {
	connections := make(map[string][]ConnectionDTO)
	if len(locations) == 0 {
		return connections
	}

	for i := 0; i < len(locations)-1; i++ {
		from := locations[i].Name
		to := locations[i+1].Name
		connections[from] = append(connections[from], ConnectionDTO{
			ToLocation:  to,
			Direction:   "path",
			Description: "Тропа между локациями",
		})
		connections[to] = append(connections[to], ConnectionDTO{
			ToLocation:  from,
			Direction:   "path",
			Description: "Тропа обратно",
		})
	}
	return connections
}

// looksLikeTruncatedJSON проверяет, выглядит ли JSON как обрезанный (незакрытые скобки, незавершенные строки)
func looksLikeTruncatedJSON(jsonStr string) bool {
	jsonStr = strings.TrimSpace(jsonStr)
	if jsonStr == "" {
		return false
	}

	openBraces := strings.Count(jsonStr, "{") - strings.Count(jsonStr, "}")
	openBrackets := strings.Count(jsonStr, "[") - strings.Count(jsonStr, "]")

	// Если есть незакрытые скобки, вероятно truncated
	if openBraces > 0 || openBrackets > 0 {
		return true
	}

	// Если заканчивается на незавершенную строку (нечетное количество кавычек)
	quoteCount := strings.Count(jsonStr, `"`)
	if quoteCount%2 != 0 {
		return true
	}

	// Если заканчивается на запятую или двоеточие
	lastChar := jsonStr[len(jsonStr)-1]
	if lastChar == ',' || lastChar == ':' {
		return true
	}

	return false
}

// tryRepairTruncatedJSON пытается восстановить обрезанный JSON
func tryRepairTruncatedJSON(jsonStr string) string {
	jsonStr = strings.TrimSpace(jsonStr)
	if jsonStr == "" {
		return "{}"
	}

	repaired := jsonrepair.Repair(jsonStr)
	if json.Valid([]byte(repaired)) {
		return repaired
	}
	jsonStr = repaired

	// Сначала пытаемся найти последний валидный объект в массиве или map
	// Это более агрессивная стратегия - обрезаем до последнего полного объекта
	if strings.Contains(jsonStr, `"locations"`) || strings.Contains(jsonStr, `"npcs"`) {
		// Ищем последний закрытый объект в массиве
		lastObjectEnd := findLastCompleteObject(jsonStr)
		if lastObjectEnd > 0 {
			// Обрезаем до последнего валидного объекта и закрываем структуры
			truncated := jsonStr[:lastObjectEnd]
			truncated = strings.TrimRight(truncated, " \n\r\t,")

			// Подсчитываем незакрытые структуры
			openBraces := strings.Count(truncated, "{") - strings.Count(truncated, "}")
			openBrackets := strings.Count(truncated, "[") - strings.Count(truncated, "]")

			// Закрываем структуры
			for i := 0; i < openBrackets; i++ {
				truncated += "]"
			}
			for i := 0; i < openBraces; i++ {
				truncated += "}"
			}

			if json.Valid([]byte(truncated)) {
				return truncated
			}
		}
	}

	// Для connections (map структура) ищем последний валидный ключ-значение
	if strings.Contains(jsonStr, `"connections"`) {
		lastConnectionEnd := findLastCompleteConnection(jsonStr)
		if lastConnectionEnd > 0 {
			truncated := jsonStr[:lastConnectionEnd]
			truncated = strings.TrimRight(truncated, " \n\r\t,")

			// Подсчитываем незакрытые структуры
			openBraces := strings.Count(truncated, "{") - strings.Count(truncated, "}")
			openBrackets := strings.Count(truncated, "[") - strings.Count(truncated, "]")

			// Закрываем структуры
			for i := 0; i < openBrackets; i++ {
				truncated += "]"
			}
			for i := 0; i < openBraces; i++ {
				truncated += "}"
			}

			if json.Valid([]byte(truncated)) {
				return truncated
			}
		}
	}

	// Стандартная логика восстановления
	openBraces := 0
	openBrackets := 0
	inString := false
	escapeNext := false
	lastValidObjectEnd := -1
	lastQuotePos := -1

	for i := 0; i < len(jsonStr); i++ {
		char := jsonStr[i]

		if escapeNext {
			escapeNext = false
			continue
		}

		if char == '\\' {
			escapeNext = true
			continue
		}

		if char == '"' && !escapeNext {
			inString = !inString
			if inString {
				lastQuotePos = i
			} else if lastQuotePos >= 0 {
				// Закрыли строку - это валидная позиция
				lastValidObjectEnd = i
			}
			continue
		}

		if inString {
			continue
		}

		switch char {
		case '{':
			openBraces++
		case '}':
			openBraces--
			if openBraces == 0 && openBrackets == 0 {
				lastValidObjectEnd = i
			}
		case '[':
			openBrackets++
		case ']':
			openBrackets--
			if openBraces == 0 && openBrackets == 0 {
				lastValidObjectEnd = i
			}
		}
	}

	result := jsonStr

	// Если JSON обрезан в середине строки
	if inString {
		if lastValidObjectEnd > 0 {
			// Обрезаем до последнего валидного объекта
			result = jsonStr[:lastValidObjectEnd+1]
		} else if lastQuotePos > 0 {
			// Обрезаем до начала незавершенной строки
			result = jsonStr[:lastQuotePos+1]
		} else {
			// Просто закрываем строку
			result = jsonStr + "\""
		}
	}

	// Удаляем trailing comma перед закрытием структур
	result = strings.TrimRight(result, " \n\r\t,")

	// Подсчитываем незакрытые структуры
	openBraces = strings.Count(result, "{") - strings.Count(result, "}")
	openBrackets = strings.Count(result, "[") - strings.Count(result, "]")

	// Если все еще в строке, закрываем её
	if inString && !strings.HasSuffix(result, "\"") {
		result += "\""
	}

	// Закрываем незакрытые структуры
	if openBraces > 0 || openBrackets > 0 {
		for i := 0; i < openBrackets; i++ {
			result += "]"
		}
		for i := 0; i < openBraces; i++ {
			result += "}"
		}
	}

	return result
}

// findLastCompleteObject находит позицию конца последнего полного объекта в массиве
func findLastCompleteObject(jsonStr string) int {
	// Ищем последний закрытый объект в массиве
	// Паттерн: }, или }\n или }\n\n или }, \n
	patterns := []string{`},\n`, `},\r\n`, `}\n`, `}\r`, `}, `, `}\n\n`, `}\r\n\r\n`}

	maxPos := -1
	bestPattern := ""
	for _, pattern := range patterns {
		pos := strings.LastIndex(jsonStr, pattern)
		if pos > maxPos {
			maxPos = pos
			bestPattern = pattern
		}
	}

	// Если нашли, возвращаем позицию после закрывающей скобки объекта
	if maxPos > 0 && bestPattern != "" {
		// Находим закрывающую скобку объекта
		bracePos := maxPos
		for i := maxPos; i >= 0 && i > maxPos-50; i-- {
			if jsonStr[i] == '}' {
				bracePos = i
				break
			}
		}

		// Проверяем, что это действительно конец объекта (не в строке)
		// Простая проверка: перед } должна быть " или число или }
		if bracePos > 0 {
			// Возвращаем позицию после закрывающей скобки
			return bracePos + 1
		}
	}

	return -1
}

// findLastCompleteConnection находит позицию конца последнего полного connection в map
func findLastCompleteConnection(jsonStr string) int {
	// Для connections структура: "connections": { "location_name": [ {...}, {...} ] }
	// Ищем последний закрытый массив в map: ], или ]\n
	patterns := []string{`],\n`, `],\r\n`, `]\n`, `]\r`, `], `}

	maxPos := -1
	bestPattern := ""
	for _, pattern := range patterns {
		pos := strings.LastIndex(jsonStr, pattern)
		if pos > maxPos {
			maxPos = pos
			bestPattern = pattern
		}
	}

	// Если нашли, возвращаем позицию после закрывающей скобки массива
	if maxPos > 0 && bestPattern != "" {
		// Находим закрывающую скобку массива
		bracketPos := maxPos
		for i := maxPos; i >= 0 && i > maxPos-50; i-- {
			if jsonStr[i] == ']' {
				bracketPos = i
				break
			}
		}

		if bracketPos > 0 {
			return bracketPos + 1
		}
	}

	return -1
}

// min возвращает минимум двух чисел
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
