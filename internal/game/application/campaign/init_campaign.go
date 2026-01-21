package campaign

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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

type InitCampaignUseCase struct {
	llm       domain.LLM
	worldRepo WorldRepository
}

func NewInitCampaignUseCase(llm domain.LLM, worldRepo WorldRepository) *InitCampaignUseCase {
	return &InitCampaignUseCase{
		llm:       llm,
		worldRepo: worldRepo,
	}
}

const (
	// Важно: в GigaChat без явного max_tokens сервер может применять небольшой дефолт,
	// из-за чего JSON часто "обрезается" (особенно locations/connections).
	// Эти лимиты задают ВЕРХНЮЮ границу ответа; модель может остановиться раньше.
	maxTokensMainQuest   = 2500
	maxTokensLocations   = 2500
	maxTokensNPCs        = 1600
	maxTokensChecks      = 1800
	maxTokensConnections = 3200
)

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

	// Шаг 3: Генерация предопределенных проверок для каждой локации
	for i := range locations {
		checks, err := uc.generateLocationPredefinedChecks(ctx, locations[i].Name, locations[i].Description)
		if err != nil {
			logger.Warn("Failed to generate predefined checks for location",
				logger.String("location", locations[i].Name),
				logger.ErrorField(err),
			)
			// Продолжаем без проверок для этой локации
		} else {
			locations[i].PredefinedChecks = checks
		}
	}

	// Шаг 4: Генерация деталей для каждой локации (NPC)
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

	// Шаг 5: Генерация связей между локациями
	connections, err := uc.generateConnections(ctx, locations)
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
	return uc.buildWorld(ctx, mainQuest, locations)
}

// generateMainQuest генерирует главный квест
func (uc *InitCampaignUseCase) generateMainQuest(ctx context.Context, worldTheme string) (*QuestDTO, error) {
	return uc.generateMainQuestWithRetry(ctx, worldTheme, 0)
}

// generateMainQuestWithRetry генерирует главный квест с retry механизмом
func (uc *InitCampaignUseCase) generateMainQuestWithRetry(ctx context.Context, worldTheme string, attempt int) (*QuestDTO, error) {
	const maxRetries = 2
	if attempt > maxRetries {
		logger.Warn("Failed to generate valid main quest, using fallback",
			logger.Int("attempts", maxRetries+1),
		)
		return fallbackMainQuest(worldTheme), nil
	}

	prompt := GenerateMainQuestPrompt(worldTheme)
	if attempt > 0 {
		// Более строгий промпт для retry
		prompt = GenerateMainQuestPromptStrict(worldTheme)
	}

	llmCtx, llmCancel := context.WithTimeout(ctx, 30*time.Second)
	defer llmCancel()

	// Явно поднимаем max_tokens, чтобы избежать обрезания JSON дефолтами провайдера.
	logger.Debug("Generating main quest",
		logger.Int("prompt_length", len(prompt)),
		logger.Int("max_tokens", maxTokensMainQuest),
	)
	raw, err := uc.llm.GenerateWithMaxTokens(llmCtx, prompt, maxTokensMainQuest)
	if err != nil {
		return nil, fmt.Errorf("LLM error: %w", err)
	}

	cleaned := cleanJSONResponse(raw)
	if !json.Valid([]byte(cleaned)) {
		logger.Warn("LLM response is not valid JSON, attempting to repair",
			logger.Int("attempt", attempt),
			logger.Int("response_length", len(cleaned)),
		)
		if len(cleaned) > 0 {
			preview := cleaned
			if len(cleaned) > 300 {
				preview = cleaned[:300] + "..."
			}
			logger.Debug("JSON preview", logger.String("preview", preview))
		}

		cleaned = tryRepairTruncatedJSON(cleaned)
		if !json.Valid([]byte(cleaned)) {
			logger.Warn("Failed to repair JSON, retrying",
				logger.Int("attempt", attempt),
				logger.String("raw_preview", raw[:min(500, len(raw))]),
			)
			return uc.generateMainQuestWithRetry(ctx, worldTheme, attempt+1)
		} else {
			logger.Info("Successfully repaired truncated JSON",
				logger.Int("attempt", attempt),
			)
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
	const maxRetries = 2
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

	llmCtx, llmCancel := context.WithTimeout(ctx, 30*time.Second)
	defer llmCancel()

	// Явно поднимаем max_tokens, чтобы избежать обрезания JSON дефолтами провайдера.
	logger.Debug("Generating locations",
		logger.Int("prompt_length", len(prompt)),
		logger.Int("max_tokens", maxTokensLocations),
	)
	raw, err := uc.llm.GenerateWithMaxTokens(llmCtx, prompt, maxTokensLocations)
	if err != nil {
		return nil, fmt.Errorf("LLM error: %w", err)
	}

	cleaned := cleanJSONResponse(raw)
	if !json.Valid([]byte(cleaned)) {
		logger.Warn("LLM response for locations is not valid JSON, attempting to repair",
			logger.Int("attempt", attempt),
			logger.Int("response_length", len(cleaned)),
			logger.String("raw_preview", raw[:min(500, len(raw))]),
		)
		// Логируем последние символы для диагностики
		if len(cleaned) > 200 {
			logger.Debug("End of response",
				logger.String("last_200_chars", cleaned[len(cleaned)-200:]),
			)
		}

		cleaned = tryRepairTruncatedJSON(cleaned)
		if !json.Valid([]byte(cleaned)) {
			logger.Warn("Failed to repair JSON for locations, retrying",
				logger.Int("attempt", attempt),
				logger.String("repaired_preview", cleaned[:min(500, len(cleaned))]),
			)
			return uc.generateLocationsWithRetry(ctx, worldTheme, mainQuestTitle, attempt+1)
		} else {
			logger.Info("Successfully repaired truncated JSON for locations",
				logger.Int("attempt", attempt),
			)
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
	const maxRetries = 2
	if attempt > maxRetries {
		logger.Warn("Failed to generate valid NPCs, using empty fallback",
			logger.Int("attempts", maxRetries+1),
			logger.String("location", locationName),
		)
		return []NPCDTO{}, nil
	}

	prompt := GenerateLocationNPCsPrompt(locationName, locationDescription)

	llmCtx, llmCancel := context.WithTimeout(ctx, 20*time.Second)
	defer llmCancel()

	// Явно поднимаем max_tokens, чтобы избежать обрезания JSON дефолтами провайдера.
	logger.Debug("Generating NPCs for location",
		logger.String("location_name", locationName),
		logger.Int("prompt_length", len(prompt)),
		logger.Int("max_tokens", maxTokensNPCs),
	)
	raw, err := uc.llm.GenerateWithMaxTokens(llmCtx, prompt, maxTokensNPCs)
	if err != nil {
		return nil, fmt.Errorf("LLM error: %w", err)
	}

	cleaned := cleanJSONResponse(raw)
	if !json.Valid([]byte(cleaned)) {
		logger.Warn("LLM response for NPCs is not valid JSON, attempting to repair",
			logger.Int("attempt", attempt),
			logger.String("location", locationName),
		)
		cleaned = tryRepairTruncatedJSON(cleaned)
		if !json.Valid([]byte(cleaned)) {
			logger.Warn("Failed to repair JSON for NPCs, retrying",
				logger.Int("attempt", attempt),
			)
			return uc.generateLocationNPCsWithRetry(ctx, locationName, locationDescription, attempt+1)
		} else {
			logger.Info("Successfully repaired truncated JSON for NPCs",
				logger.Int("attempt", attempt),
			)
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

// generateLocationPredefinedChecks генерирует предопределенные проверки для локации
func (uc *InitCampaignUseCase) generateLocationPredefinedChecks(ctx context.Context, locationName, locationDescription string) ([]PredefinedCheckDTO, error) {
	return uc.generateLocationPredefinedChecksWithRetry(ctx, locationName, locationDescription, 0)
}

// generateLocationPredefinedChecksWithRetry генерирует предопределенные проверки для локации с retry механизмом
func (uc *InitCampaignUseCase) generateLocationPredefinedChecksWithRetry(ctx context.Context, locationName, locationDescription string, attempt int) ([]PredefinedCheckDTO, error) {
	const maxRetries = 2
	if attempt > maxRetries {
		logger.Warn("Failed to generate valid predefined checks, using empty fallback",
			logger.Int("attempts", maxRetries+1),
			logger.String("location", locationName),
		)
		return []PredefinedCheckDTO{}, nil
	}

	prompt := GenerateLocationPredefinedChecksPrompt(locationName, locationDescription)

	llmCtx, llmCancel := context.WithTimeout(ctx, 20*time.Second)
	defer llmCancel()

	// Явно поднимаем max_tokens, чтобы избежать обрезания JSON дефолтами провайдера.
	logger.Debug("Generating predefined checks",
		logger.String("location_name", locationName),
		logger.Int("prompt_length", len(prompt)),
		logger.Int("max_tokens", maxTokensChecks),
	)
	raw, err := uc.llm.GenerateWithMaxTokens(llmCtx, prompt, maxTokensChecks)
	if err != nil {
		return nil, fmt.Errorf("LLM error: %w", err)
	}

	cleaned := cleanJSONResponse(raw)
	if !json.Valid([]byte(cleaned)) {
		logger.Warn("LLM response for predefined checks is not valid JSON, attempting to repair",
			logger.Int("attempt", attempt),
			logger.String("location", locationName),
		)
		cleaned = tryRepairTruncatedJSON(cleaned)
		if !json.Valid([]byte(cleaned)) {
			logger.Warn("Failed to repair JSON for predefined checks, retrying",
				logger.Int("attempt", attempt),
			)
			return uc.generateLocationPredefinedChecksWithRetry(ctx, locationName, locationDescription, attempt+1)
		} else {
			logger.Info("Successfully repaired truncated JSON for predefined checks",
				logger.Int("attempt", attempt),
			)
		}
	}

	var response struct {
		PredefinedChecks []PredefinedCheckDTO `json:"predefined_checks"`
	}
	if err := decodeStrictJSON(cleaned, &response); err != nil {
		logger.Warn("Failed to parse predefined checks JSON",
			logger.Int("attempt", attempt),
			logger.ErrorField(err),
		)
		return uc.generateLocationPredefinedChecksWithRetry(ctx, locationName, locationDescription, attempt+1)
	}

	return response.PredefinedChecks, nil
}

// generateConnections генерирует связи между локациями
func (uc *InitCampaignUseCase) generateConnections(ctx context.Context, locations []LocationDTO) (map[string][]ConnectionDTO, error) {
	return uc.generateConnectionsWithRetry(ctx, locations, 0)
}

// generateConnectionsWithRetry генерирует связи между локациями с retry механизмом
func (uc *InitCampaignUseCase) generateConnectionsWithRetry(ctx context.Context, locations []LocationDTO, attempt int) (map[string][]ConnectionDTO, error) {
	const maxRetries = 2
	if attempt > maxRetries {
		logger.Warn("Failed to generate valid connections, using fallback",
			logger.Int("attempts", maxRetries+1),
		)
		return fallbackConnections(locations), nil
	}

	prompt := GenerateConnectionsPrompt(locations)

	llmCtx, llmCancel := context.WithTimeout(ctx, 30*time.Second)
	defer llmCancel()

	// Явно поднимаем max_tokens, чтобы избежать обрезания JSON дефолтами провайдера.
	logger.Debug("Generating location connections",
		logger.Int("prompt_length", len(prompt)),
		logger.Int("max_tokens", maxTokensConnections),
	)
	raw, err := uc.llm.GenerateWithMaxTokens(llmCtx, prompt, maxTokensConnections)
	if err != nil {
		return nil, fmt.Errorf("LLM error: %w", err)
	}

	cleaned := cleanJSONResponse(raw)
	if !json.Valid([]byte(cleaned)) {
		logger.Warn("LLM response for connections is not valid JSON, attempting to repair",
			logger.Int("attempt", attempt),
			logger.Int("response_length", len(cleaned)),
			logger.String("raw_preview", raw[:min(500, len(raw))]),
		)
		// Логируем последние символы для диагностики
		if len(cleaned) > 200 {
			logger.Debug("End of response",
				logger.String("last_200_chars", cleaned[len(cleaned)-200:]),
			)
		}

		cleaned = tryRepairTruncatedJSON(cleaned)
		if !json.Valid([]byte(cleaned)) {
			logger.Warn("Failed to repair JSON for connections, retrying",
				logger.Int("attempt", attempt),
				logger.String("repaired_preview", cleaned[:min(500, len(cleaned))]),
			)
			return uc.generateConnectionsWithRetry(ctx, locations, attempt+1)
		} else {
			logger.Info("Successfully repaired truncated JSON for connections",
				logger.Int("attempt", attempt),
			)
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
		return uc.generateConnectionsWithRetry(ctx, locations, attempt+1)
	}

	return response.Connections, nil
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
			loc.AddNPC(npc.New(npcDTO.Name, npcDTO.Role))
		}

		// Конвертируем предопределенные проверки из DTO в world.PredefinedCheck
		var predefinedChecks []world.PredefinedCheck
		for _, checkDTO := range locDTO.PredefinedChecks {
			predefinedChecks = append(predefinedChecks, world.PredefinedCheck{
				Ability:      checkDTO.Ability,
				DC:           checkDTO.DC,
				Description:  checkDTO.Description,
				LocationHint: checkDTO.LocationHint,
			})
		}

		// Конвертируем NPCs из location.NPC в world.NPC
		var worldNPCs []world.NPC
		for _, npc := range loc.NPCs {
			worldNPCs = append(worldNPCs, world.NPC{
				Name:        npc.Name,
				Role:        npc.Role,
				Personality: "",
			})
		}

		// Используем новый метод AddLocationWithChecks для добавления локации с проверками
		w.AddLocationWithChecks(locDTO.Name, locDTO.Description, worldNPCs, predefinedChecks)
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
				FromLocationID: fromID,
				ToLocationID:   toID,
				Direction:      connDTO.Direction,
				Description:    connDTO.Description,
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
		{Name: "Городские ворота", Description: fmt.Sprintf("Вход в %s и отправная точка приключения.", theme)},
		{Name: "Древний лес", Description: "Таинственный лес с шепчущими деревьями и скрытыми тропами."},
		{Name: "Забытые руины", Description: "Разрушенные руины, где хранятся ответы на главный квест."},
	}
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
