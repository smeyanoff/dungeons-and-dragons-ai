package persistence

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"dungeons-and-dragons-ai/internal/game/domain/player"
	"dungeons-and-dragons-ai/internal/game/domain/session"
)

type GameSessionRepository struct {
	db *gorm.DB
}

func NewGameSessionRepository(db *gorm.DB) *GameSessionRepository {
	return &GameSessionRepository{db: db}
}

func (r *GameSessionRepository) GetByChatID(
	ctx context.Context,
	chatID int64,
) (*session.GameSession, error) {

	var gs session.GameSession

	// Используем Unscoped() чтобы найти сессию даже если она была soft deleted
	// Это нужно для проверки завершенных сессий перед созданием новой
	err := r.db.WithContext(ctx).
		Unscoped().
		Preload("World").
		Preload("World.MainQuest").
		Preload("World.MainQuest.Items").
		Preload("World.Locations").
		Preload("World.Locations.NPCs").
		Preload("World.Locations.Monsters").
		Preload("World.Locations.Connections").
		Preload("World.Events").
		Preload("Players").
		Preload("Players.Character").
		Preload("Players.Character.Stats").
		Where("chat_id = ?", chatID).
		First(&gs).Error

	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &gs, nil
}

func (r *GameSessionRepository) Save(
	ctx context.Context,
	gs *session.GameSession,
) error {

	return r.db.WithContext(ctx).
		Transaction(func(tx *gorm.DB) error {
			return tx.Session(&gorm.Session{FullSaveAssociations: true}).
				Save(gs).Error
		})
}

// Delete удаляет сессию по chat_id (hard delete)
// Используется для удаления завершенных сессий перед созданием новой
// Удаляет связанных игроков перед удалением сессии (решает проблему foreign key constraint)
func (r *GameSessionRepository) Delete(
	ctx context.Context,
	chatID int64,
) error {
	// Используем транзакцию для атомарного удаления сессии и связанных данных
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Сначала находим сессию для получения её ID
		var gs session.GameSession
		if err := tx.Unscoped().Where("chat_id = ?", chatID).First(&gs).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				// Сессия не найдена - это не ошибка, возможно уже удалена
				return nil
			}
			return err
		}

		// Удаляем связанных игроков (hard delete)
		// Это необходимо для избежания foreign key constraint violation
		if err := tx.Unscoped().Where("game_session_id = ?", gs.ID).
			Delete(&player.Player{}).Error; err != nil {
			return fmt.Errorf("failed to delete players: %w", err)
		}

		// Теперь удаляем саму сессию (hard delete)
		// Используем Unscoped().Delete() для физического удаления записи
		// Это необходимо, чтобы избежать duplicate key при создании новой сессии
		if err := tx.Unscoped().Where("chat_id = ?", chatID).
			Delete(&session.GameSession{}).Error; err != nil {
			return fmt.Errorf("failed to delete session: %w", err)
		}

		return nil
	})
}
