package main

import (
	"log"
	"os"

	"dungeons-and-dragons-ai/internal/game/domain/achievement"
	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/combat"
	"dungeons-and-dragons-ai/internal/game/domain/event"
	"dungeons-and-dragons-ai/internal/game/domain/feedback"
	"dungeons-and-dragons-ai/internal/game/domain/inventory"
	"dungeons-and-dragons-ai/internal/game/domain/item"
	llmlogdomain "dungeons-and-dragons-ai/internal/game/domain/llm_log"
	"dungeons-and-dragons-ai/internal/game/domain/player"
	"dungeons-and-dragons-ai/internal/game/domain/quest"
	"dungeons-and-dragons-ai/internal/game/domain/rating"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/game/domain/spell"
	"dungeons-and-dragons-ai/internal/game/domain/subscription"
	"dungeons-and-dragons-ai/internal/game/domain/world"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormLogger "gorm.io/gorm/logger"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5432/dnd_test?sslmode=disable"
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormLogger.Default.LogMode(gormLogger.Info),
	})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	log.Println("Running migrations...")

	err = db.AutoMigrate(
		&session.GameSession{},
		&world.World{},
		&world.Location{},
		&world.LocationConnection{},
		&world.NPC{},
		&world.Monster{},
		&world.WorldEvent{},
		&player.Player{},
		&character.Character{},
		&character.Stats{},
		&event.StoryEvent{},
		&inventory.Inventory{},
		&inventory.InventoryItem{},
		&llmlogdomain.LLMLog{},
		&combat.Combat{},
		&combat.CombatParticipant{},
		&item.Item{},
		&quest.Quest{},
		&quest.DailyQuest{},
		&quest.DailyQuestProgress{},
		&quest.DailyQuestStreak{},
		&feedback.Feedback{},
		&achievement.Achievement{},
		&rating.PlayerRating{},
		&achievement.PlayerAchievement{},
		&achievement.AchievementProgress{},
		&spell.Spell{},
		&spell.CharacterSpell{},
		&subscription.Subscription{},
	)
	if err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	log.Println("Migrations completed successfully!")
}
