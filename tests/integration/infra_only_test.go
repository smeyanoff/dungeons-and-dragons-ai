package integration

import (
	"context"
	"strings"
	"testing"

	"dungeons-and-dragons-ai/internal/game/domain/inventory"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/game/domain/spell"
	worlddomain "dungeons-and-dragons-ai/internal/game/domain/world"
	"dungeons-and-dragons-ai/internal/game/infrastructure/persistence"
	"dungeons-and-dragons-ai/pkg/logger"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// infraOnlyConfig — минимальная конфигурация для интеграционных тестов,
// которые не требуют LLM/Qdrant (например, ability check guardrails, callback flows).
type infraOnlyConfig struct {
	db            *gorm.DB
	ctx           context.Context
	chatID        int64
	tgUserID      int64
	sessionRepo   session.Repository
	worldRepo     *persistence.WorldRepository
	playerRepo    *persistence.PlayerRepository
	eventRepo     *persistence.GameEventRepository
	inventoryRepo *persistence.InventoryRepository
	spellRepo     *persistence.SpellRepository
}

func setupInfraOnlyIntegrationTest(t *testing.T) *infraOnlyConfig {
	t.Helper()

	ctx := context.Background()

	// Инициализация логгера (не критично).
	if err := logger.InitFromEnv(); err != nil {
		t.Logf("Не удалось инициализировать логгер: %v", err)
	}

	// Подключение к БД (используем те же env+fallback, что и в setupIntegrationTest).
	dbDSN := getEnv("DATABASE_URL", "postgres://dnd_user:dnd_password@localhost:5432/dnd?sslmode=disable")
	if strings.Contains(dbDSN, "@postgres:") {
		dbDSN = strings.Replace(dbDSN, "@postgres:", "@localhost:", 1)
	}

	db, err := gorm.Open(postgres.Open(dbDSN), &gorm.Config{})
	if err != nil {
		t.Skipf("Контейнеры не запущены/БД недоступна (DATABASE_URL=%q): %v", dbDSN, err)
		return nil
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Skipf("Не удалось получить sql.DB: %v", err)
		return nil
	}
	if err := sqlDB.Ping(); err != nil {
		t.Skipf("Контейнеры не запущены/БД недоступна (ping failed): %v", err)
		return nil
	}

	// Гарантируем наличие новых колонок в game_sessions перед тестами.
	if err := db.AutoMigrate(&session.GameSession{}); err != nil {
		t.Fatalf("Не удалось выполнить AutoMigrate для game_sessions: %v", err)
	}
	if err := db.AutoMigrate(&worlddomain.WorldEvent{}); err != nil {
		t.Fatalf("Не удалось выполнить AutoMigrate для world_events: %v", err)
	}
	if err := db.AutoMigrate(&inventory.Inventory{}, &inventory.InventoryItem{}); err != nil {
		t.Fatalf("Не удалось выполнить AutoMigrate для inventories: %v", err)
	}
	if err := db.AutoMigrate(&spell.Spell{}, &spell.CharacterSpell{}); err != nil {
		t.Fatalf("Не удалось выполнить AutoMigrate для spells: %v", err)
	}

	chatID, tgUserID := generateTestIDs(t)

	spellRepo := persistence.NewSpellRepository(db)
	if err := spellRepo.InitDefaultSpells(ctx); err != nil {
		t.Fatalf("Не удалось инициализировать заклинания по умолчанию: %v", err)
	}

	return &infraOnlyConfig{
		db:            db,
		ctx:           ctx,
		chatID:        chatID,
		tgUserID:      tgUserID,
		sessionRepo:   persistence.NewGameSessionRepository(db),
		worldRepo:     persistence.NewWorldRepository(db),
		playerRepo:    persistence.NewPlayerRepository(db),
		eventRepo:     persistence.NewGameEventRepository(db),
		inventoryRepo: persistence.NewInventoryRepository(db),
		spellRepo:     spellRepo,
	}
}
