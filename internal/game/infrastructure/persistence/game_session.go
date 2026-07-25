package persistence

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"dungeons-and-dragons-ai/internal/game/domain/combat"
	"dungeons-and-dragons-ai/internal/game/domain/event"
	"dungeons-and-dragons-ai/internal/game/domain/player"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/game/domain/world"
)

type GameSessionRepository struct {
	db *gorm.DB
}

func NewGameSessionRepository(db *gorm.DB) *GameSessionRepository {
	return &GameSessionRepository{db: db}
}

// CountActiveGamesByTgUserID подсчитывает количество активных игр для пользователя
// Активная игра - это сессия со статусом "active", в которой есть игрок с указанным TgUserID
func (r *GameSessionRepository) CountActiveGamesByTgUserID(
	ctx context.Context,
	tgUserID int64,
) (int, error) {
	var count int64

	// Подсчитываем активные сессии, где есть игрок с указанным TgUserID
	err := r.db.WithContext(ctx).
		Model(&session.GameSession{}).
		Joins("INNER JOIN players ON players.game_session_id = game_sessions.id").
		Where("game_sessions.state = ? AND players.tg_user_id = ?", session.StateActive, tgUserID).
		Distinct("game_sessions.id").
		Count(&count).Error

	if err != nil {
		return 0, fmt.Errorf("failed to count active games: %w", err)
	}

	return int(count), nil
}

// CountSavesByTgUserID подсчитывает количество сохранений для пользователя
// Сохранение - это завершенная сессия (state = "done"), которую можно загрузить позже
// В текущей реализации это количество завершенных сессий пользователя
func (r *GameSessionRepository) CountSavesByTgUserID(
	ctx context.Context,
	tgUserID int64,
) (int, error) {
	var count int64

	// Подсчитываем завершенные сессии, где есть игрок с указанным TgUserID
	err := r.db.WithContext(ctx).
		Model(&session.GameSession{}).
		Joins("INNER JOIN players ON players.game_session_id = game_sessions.id").
		Where("game_sessions.state = ? AND players.tg_user_id = ?", session.StateDone, tgUserID).
		Distinct("game_sessions.id").
		Count(&count).Error

	if err != nil {
		return 0, fmt.Errorf("failed to count saves: %w", err)
	}

	return int(count), nil
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

		// Удаляем связанные цели сессии (hard delete)
		if err := tx.Unscoped().Where("game_session_id = ?", gs.ID).
			Delete(&session.SessionGoal{}).Error; err != nil {
			return fmt.Errorf("failed to delete session goals: %w", err)
		}

		// Удаляем историю событий сессии (hard delete) — иначе она остаётся
		// осиротевшей в БД после завершения игры (не читается новой сессией,
		// т.к. RAG/history всегда фильтруют по game_session_id, но накапливается мёртвым весом).
		if err := tx.Unscoped().Where("game_session_id = ?", gs.ID).
			Delete(&event.StoryEvent{}).Error; err != nil {
			return fmt.Errorf("failed to delete story events: %w", err)
		}

		// Удаляем бои сессии: сначала участников (FK на combat_id), затем сами бои.
		var combatIDs []uint
		if err := tx.Unscoped().Model(&combat.Combat{}).
			Where("game_session_id = ?", gs.ID).
			Pluck("id", &combatIDs).Error; err != nil {
			return fmt.Errorf("failed to list combats: %w", err)
		}
		if len(combatIDs) > 0 {
			if err := tx.Unscoped().Where("combat_id IN ?", combatIDs).
				Delete(&combat.CombatParticipant{}).Error; err != nil {
				return fmt.Errorf("failed to delete combat participants: %w", err)
			}
			if err := tx.Unscoped().Where("game_session_id = ?", gs.ID).
				Delete(&combat.Combat{}).Error; err != nil {
				return fmt.Errorf("failed to delete combats: %w", err)
			}
		}

		// Удаляем факты и мировые события кампании по world_id — они привязаны к миру
		// завершённой игры, а не к game_session_id, и без этого остаются осиротевшими.
		if gs.WorldID != 0 {
			if err := tx.Unscoped().Where("world_id = ?", gs.WorldID).
				Delete(&world.CampaignFact{}).Error; err != nil {
				return fmt.Errorf("failed to delete campaign facts: %w", err)
			}
			if err := tx.Unscoped().Where("world_id = ?", gs.WorldID).
				Delete(&world.WorldEvent{}).Error; err != nil {
				return fmt.Errorf("failed to delete world events: %w", err)
			}
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
