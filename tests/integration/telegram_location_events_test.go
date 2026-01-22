package integration

import (
	"fmt"
	"testing"
	"time"

	characterapp "dungeons-and-dragons-ai/internal/game/application/character"
	locationeventapp "dungeons-and-dragons-ai/internal/game/application/location_event"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	worlddomain "dungeons-and-dragons-ai/internal/game/domain/world"
	"dungeons-and-dragons-ai/internal/game/infrastructure/persistence"
)

// TestTelegramLocationEventsGeneration проверяет генерацию событий в локациях при первом посещении
func TestTelegramLocationEventsGeneration(t *testing.T) {
	cfg := setupInfraOnlyIntegrationTest(t)
	if cfg == nil {
		return
	}
	defer cleanupTest(t, &testConfig{db: cfg.db, chatID: cfg.chatID, tgUserID: cfg.tgUserID})

	ctx := cfg.ctx
	chatID := cfg.chatID

	// Очищаем события для тестовых location ID
	eventRepo := persistence.NewWorldEventRepository(cfg.db)
	for _, locationID := range []uint{101, 102, 103} {
		events, err := eventRepo.GetByLocationID(ctx, locationID)
		if err == nil {
			for _, event := range events {
				cfg.db.Delete(&event) // Удаляем существующие события
			}
		}
	}

	// Создаем deterministic world+session
	w := worlddomain.New("Test World (Location Events)")
	w.Description = "Deterministic test world for location events"

	// Создаем несколько локаций
	locations := []worlddomain.Location{
		{Name: "Forest Clearing", Description: "A peaceful clearing in the forest"},
		{Name: "Ancient Temple", Description: "An old temple with mysterious runes"},
		{Name: "Mountain Cave", Description: "A dark cave high in the mountains"},
	}
	w.Locations = locations

	if err := cfg.worldRepo.Save(ctx, w); err != nil {
		t.Fatalf("Не удалось сохранить тестовый мир: %v", err)
	}
	gs := &session.GameSession{ChatID: chatID, State: session.StateActive, World: *w, WorldID: w.ID}
	if err := cfg.sessionRepo.Save(ctx, gs); err != nil {
		t.Fatalf("Не удалось сохранить сессию: %v", err)
	}

	// Создаем персонажа
	createCharacterUC := characterapp.NewCreateCharacterUseCase(cfg.sessionRepo, cfg.playerRepo)
	_, err := createCharacterUC.Execute(ctx, newCharacterRequest(chatID))
	if err != nil {
		t.Fatalf("Не удалось создать персонажа: %v", err)
	}

	// Создаем генератор событий локаций
	locationEventGenerator := locationeventapp.NewLocationEventGenerator(eventRepo)

	// Тестируем генерацию событий для каждой локации при первом посещении
	for i, location := range locations {
		t.Run(fmt.Sprintf("Location_%d_%s", i+1, location.Name), func(t *testing.T) {
			req := locationeventapp.GenerateLocationEventRequest{
				WorldID:      w.ID,
				LocationID:   uint(100 + i + 1), // Используем уникальный ID локации для каждого теста
				LocationName: location.Name,
				IsFirstVisit: true, // Первый визит
			}

			response, err := locationEventGenerator.Execute(ctx, req)
			if err != nil {
				t.Fatalf("Ошибка генерации события для локации %s: %v", location.Name, err)
			}

			t.Logf("Generated event for %s: ID=%d, Type=%s", location.Name, response.Event.ID, response.Event.Type)

			// Проверяем, что событие было сгенерировано
			if response == nil || response.Event == nil {
				// Проверим, есть ли существующие события для этой локации
				existingEvents, checkErr := eventRepo.GetByLocationID(ctx, req.LocationID)
				if checkErr != nil {
					t.Fatalf("Ошибка проверки существующих событий: %v", checkErr)
				}
				t.Fatalf("Ожидалось сгенерированное событие для локации %s (LocationID: %d), существующие события: %d", location.Name, req.LocationID, len(existingEvents))
			}

			// Проверяем основные поля события
			event := response.Event
			if event.Name == "" {
				t.Fatal("Имя события не должно быть пустым")
			}

			if event.Description == "" {
				t.Fatal("Описание события не должно быть пустым")
			}

			if event.Type == "" {
				t.Fatal("Тип события не должен быть пустым")
			}

			// Проверяем, что событие связано с правильной локацией
			expectedLocationID := uint(100 + i + 1)
			if event.RequiredLocationID == nil || *event.RequiredLocationID != expectedLocationID {
				t.Fatalf("Событие должно быть связано с локацией ID %d, получено %v", expectedLocationID, event.RequiredLocationID)
			}

			// Проверяем тип события (должен быть одним из допустимых)
			validTypes := []worlddomain.WorldEventType{
				worlddomain.WorldEventTypeLocationNPC,
				worlddomain.WorldEventTypeLocationItem,
				worlddomain.WorldEventTypeLocationTrap,
				worlddomain.WorldEventTypeLocationPuzzle,
				worlddomain.WorldEventTypeLocationEncounter,
			}

			isValidType := false
			for _, validType := range validTypes {
				if event.Type == validType {
					isValidType = true
					break
				}
			}

			if !isValidType {
				t.Fatalf("Недопустимый тип события: %s", event.Type)
			}

			// Проверяем, что событие сохранено в БД (ищем по ID напрямую)
			var savedEvent worlddomain.WorldEvent
			err = cfg.db.Where("id = ?", event.ID).First(&savedEvent).Error
			if err != nil {
				t.Fatalf("Ошибка получения события из БД по ID %d: %v", event.ID, err)
			}

			// Проверяем основные поля
			if savedEvent.Type != event.Type {
				t.Fatalf("Тип события в БД не совпадает: ожидалось %s, получено %s", event.Type, savedEvent.Type)
			}

			if savedEvent.Name != event.Name {
				t.Fatalf("Имя события в БД не совпадает: ожидалось %s, получено %s", event.Name, savedEvent.Name)
			}

			// Проверяем RequiredLocationID
			if savedEvent.RequiredLocationID == nil {
				t.Fatal("RequiredLocationID не сохранен в БД")
			}

			if *savedEvent.RequiredLocationID != expectedLocationID {
				t.Fatalf("RequiredLocationID в БД не совпадает: ожидалось %d, получено %d", expectedLocationID, *savedEvent.RequiredLocationID)
			}
		})
	}
}

// TestTelegramLocationEventsCooldown проверяет cooldown между генерацией событий
func TestTelegramLocationEventsCooldown(t *testing.T) {
	cfg := setupInfraOnlyIntegrationTest(t)
	if cfg == nil {
		return
	}
	defer cleanupTest(t, &testConfig{db: cfg.db, chatID: cfg.chatID, tgUserID: cfg.tgUserID})

	ctx := cfg.ctx

	eventRepo := persistence.NewWorldEventRepository(cfg.db)
	// Очищаем события для location ID 200
	events, err := eventRepo.GetByLocationID(ctx, 200)
	if err != nil {
		t.Logf("Warning: could not get events for cleanup: %v", err)
	}
	if err == nil {
		for _, event := range events {
			cfg.db.Delete(&event)
		}
	}

	locationEventGenerator := locationeventapp.NewLocationEventGenerator(eventRepo)

	// Создаем deterministic world
	w := worlddomain.New("Test World (Cooldown)")
	w.Description = "Deterministic test world for cooldown testing"
	w.Locations = []worlddomain.Location{{Name: "Test Location", Description: "Test location for cooldown"}}
	if err := cfg.worldRepo.Save(ctx, w); err != nil {
		t.Fatalf("Не удалось сохранить тестовый мир: %v", err)
	}

	// Первая генерация события (должна сработать)
	req1 := locationeventapp.GenerateLocationEventRequest{
		WorldID:      w.ID,
		LocationID:   200,
		LocationName: "Test Location",
		IsFirstVisit: true,
	}

	response1, err := locationEventGenerator.Execute(ctx, req1)
	if err != nil {
		t.Fatalf("Ошибка первой генерации события: %v", err)
	}

	if response1 == nil {
		t.Fatal("Ожидалось первое событие")
	}

	// Вторая генерация сразу после первой (должна вернуть nil из-за cooldown)
	req2 := locationeventapp.GenerateLocationEventRequest{
		WorldID:      w.ID,
		LocationID:   200,
		LocationName: "Test Location",
		IsFirstVisit: false, // Не первый визит
	}

	response2, err := locationEventGenerator.Execute(ctx, req2)
	if err != nil {
		t.Fatalf("Ошибка второй генерации события: %v", err)
	}

	if response2 != nil {
		t.Fatal("Ожидалось nil из-за cooldown, но получено событие")
	}

	// Проверяем, что в БД только одно событие для этой локации
	events5, err5 := eventRepo.GetByLocationID(ctx, 200)
	if err5 != nil {
		t.Fatalf("Ошибка получения событий из БД: %v", err5)
	}
	if len(events5) != 1 {
		t.Fatalf("Ожидалось 1 событие в БД, получено %d", len(events5))
	}

	if len(events) != 1 {
		t.Fatalf("Ожидалось 1 событие в БД, получено %d", len(events))
	}
}

// TestTelegramLocationEventsNotFirstVisit проверяет, что события не генерируются при повторном посещении
func TestTelegramLocationEventsNotFirstVisit(t *testing.T) {
	cfg := setupInfraOnlyIntegrationTest(t)
	if cfg == nil {
		return
	}
	defer cleanupTest(t, &testConfig{db: cfg.db, chatID: cfg.chatID, tgUserID: cfg.tgUserID})

	ctx := cfg.ctx

	eventRepo := persistence.NewWorldEventRepository(cfg.db)
	// Очищаем события для location ID 300
	events2, err2 := eventRepo.GetByLocationID(ctx, 300)
	if err2 == nil {
		for _, event := range events2 {
			cfg.db.Delete(&event)
		}
	}

	locationEventGenerator := locationeventapp.NewLocationEventGenerator(eventRepo)

	// Создаем deterministic world
	w := worlddomain.New("Test World (Not First)")
	w.Description = "Deterministic test world for not first visit testing"
	w.Locations = []worlddomain.Location{{Name: "Test Location", Description: "Test location for not first visit"}}
	if err := cfg.worldRepo.Save(ctx, w); err != nil {
		t.Fatalf("Не удалось сохранить тестовый мир: %v", err)
	}

	// Попытка генерации события при не первом посещении
	req := locationeventapp.GenerateLocationEventRequest{
		WorldID:      w.ID,
		LocationID:   300,
		LocationName: "Test Location",
		IsFirstVisit: false, // Не первый визит
	}

	response, err := locationEventGenerator.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Ошибка генерации события: %v", err)
	}

	if response != nil {
		t.Fatal("Ожидалось nil при не первом посещении, но получено событие")
	}

	// Проверяем, что в БД нет событий
	events, err := eventRepo.GetByLocationID(ctx, 300)
	if err != nil {
		t.Fatalf("Ошибка получения событий из БД: %v", err)
	}

	if len(events) != 0 {
		t.Fatalf("Ожидалось 0 событий в БД, получено %d", len(events))
	}
}

// TestTelegramLocationEventsMaxPerLocationWindow проверяет ограничение на количество событий в окне времени
func TestTelegramLocationEventsMaxPerLocationWindow(t *testing.T) {
	cfg := setupInfraOnlyIntegrationTest(t)
	if cfg == nil {
		return
	}
	defer cleanupTest(t, &testConfig{db: cfg.db, chatID: cfg.chatID, tgUserID: cfg.tgUserID})

	ctx := cfg.ctx

	eventRepo := persistence.NewWorldEventRepository(cfg.db)
	// Очищаем события для location ID 400
	events3, err3 := eventRepo.GetByLocationID(ctx, 400)
	if err3 == nil {
		for _, event := range events3 {
			cfg.db.Delete(&event)
		}
	}

	locationEventGenerator := locationeventapp.NewLocationEventGenerator(eventRepo)

	// Создаем deterministic world
	w := worlddomain.New("Test World (Max Events)")
	w.Description = "Deterministic test world for max events testing"
	w.Locations = []worlddomain.Location{{Name: "Test Location", Description: "Test location for max events"}}
	if err := cfg.worldRepo.Save(ctx, w); err != nil {
		t.Fatalf("Не удалось сохранить тестовый мир: %v", err)
	}

	// Создаем максимальное количество событий вручную
	now := time.Now()
	for i := 0; i < 3; i++ { // maxEventsPerLocationPerWindow = 3
		event := &worlddomain.WorldEvent{
			WorldID:            w.ID,
			Type:               worlddomain.WorldEventTypeLocationNPC,
			Status:             worlddomain.WorldEventStatusActive,
			Name:               fmt.Sprintf("Test Event %d", i+1),
			Description:        fmt.Sprintf("Test event description %d", i+1),
			RequiredLocationID: &[]uint{400}[0],
			ActivatedAt:        &now,
			CreatedAt:          now.Add(time.Duration(i) * time.Minute),
			UpdatedAt:          now.Add(time.Duration(i) * time.Minute),
		}

		if err := eventRepo.Save(ctx, event); err != nil {
			t.Fatalf("Не удалось сохранить тестовое событие %d: %v", i+1, err)
		}
	}

	// Попытка генерации нового события (должна вернуть nil из-за превышения лимита)
	req := locationeventapp.GenerateLocationEventRequest{
		WorldID:      w.ID,
		LocationID:   400,
		LocationName: "Test Location",
		IsFirstVisit: true,
	}

	response, err := locationEventGenerator.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Ошибка генерации события: %v", err)
	}

	if response != nil {
		t.Fatal("Ожидалось nil из-за превышения лимита событий, но получено событие")
	}

	// Проверяем, что в БД все еще 3 события
	events, err := eventRepo.GetByLocationID(ctx, 400)
	if err != nil {
		t.Fatalf("Ошибка получения событий из БД: %v", err)
	}

	if len(events) != 3 {
		t.Fatalf("Ожидалось 3 события в БД, получено %d", len(events))
	}
}

// TestTelegramLocationEventsEventTypes проверяет разнообразие типов генерируемых событий
func TestTelegramLocationEventsEventTypes(t *testing.T) {
	cfg := setupInfraOnlyIntegrationTest(t)
	if cfg == nil {
		return
	}
	defer cleanupTest(t, &testConfig{db: cfg.db, chatID: cfg.chatID, tgUserID: cfg.tgUserID})

	ctx := cfg.ctx

	eventRepo := persistence.NewWorldEventRepository(cfg.db)
	// Очищаем события для location ID 500+
	for i := 500; i < 510; i++ {
		events4, err4 := eventRepo.GetByLocationID(ctx, uint(i))
		if err4 == nil {
			for _, event := range events4 {
				cfg.db.Delete(&event)
			}
		}
	}

	locationEventGenerator := locationeventapp.NewLocationEventGenerator(eventRepo)

	// Создаем deterministic world
	w := worlddomain.New("Test World (Event Types)")
	w.Description = "Deterministic test world for event types testing"
	w.Locations = []worlddomain.Location{{Name: "Test Location", Description: "Test location for event types"}}
	if err := cfg.worldRepo.Save(ctx, w); err != nil {
		t.Fatalf("Не удалось сохранить тестовый мир: %v", err)
	}

	// Генерируем несколько событий для проверки разнообразия типов
	generatedTypes := make(map[worlddomain.WorldEventType]bool)

	for i := 0; i < 10; i++ {
		req := locationeventapp.GenerateLocationEventRequest{
			WorldID:      w.ID,
			LocationID:   uint(500 + i + 1), // Разные локации для обхода cooldown
			LocationName: fmt.Sprintf("Test Location %d", i+1),
			IsFirstVisit: true,
		}

		response, err := locationEventGenerator.Execute(ctx, req)
		if err != nil {
			t.Fatalf("Ошибка генерации события %d: %v", i+1, err)
		}

		if response != nil {
			generatedTypes[response.Event.Type] = true
		}
	}

	// Проверяем, что сгенерированы разные типы событий
	expectedTypes := []worlddomain.WorldEventType{
		worlddomain.WorldEventTypeLocationNPC,
		worlddomain.WorldEventTypeLocationItem,
		worlddomain.WorldEventTypeLocationTrap,
		worlddomain.WorldEventTypeLocationPuzzle,
	}

	generatedCount := 0
	for _, expectedType := range expectedTypes {
		if generatedTypes[expectedType] {
			generatedCount++
		}
	}

	// Должны быть сгенерированы хотя бы 2-3 разных типа из 5 возможных
	if generatedCount < 2 {
		t.Fatalf("Ожидалось генерация хотя бы 2 разных типов событий, получено %d", generatedCount)
	}
}
