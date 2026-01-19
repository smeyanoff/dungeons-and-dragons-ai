package persistence

import (
	"context"
	"time"

	"gorm.io/gorm"

	"dungeons-and-dragons-ai/internal/game/domain/quest"
)

type DailyQuestRepository struct {
	db *gorm.DB
}

func NewDailyQuestRepository(db *gorm.DB) *DailyQuestRepository {
	return &DailyQuestRepository{db: db}
}

// GetTodayQuests возвращает ежедневные задания на сегодня
func (r *DailyQuestRepository) GetTodayQuests(ctx context.Context) ([]*quest.DailyQuest, error) {
	var dailyQuests []*quest.DailyQuest
	
	// Получаем задания, созданные сегодня
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)
	
	err := r.db.WithContext(ctx).
		Where("created_at >= ? AND created_at < ?", startOfDay, endOfDay).
		Find(&dailyQuests).Error
	
	if err != nil {
		return nil, err
	}
	
	// Если заданий нет, создаем новые на сегодня
	if len(dailyQuests) == 0 {
		dailyQuests = r.createTodayQuests(ctx)
	}
	
	return dailyQuests, nil
}

// createTodayQuests создает ежедневные задания на сегодня
func (r *DailyQuestRepository) createTodayQuests(ctx context.Context) []*quest.DailyQuest {
	quests := []*quest.DailyQuest{
		quest.NewDailyQuest(
			quest.DailyQuestTypeCompleteQuest,
			"Завершить квест",
			"Завершите любой активный квест",
			1, // Целевое значение: 1 квест
			50, // Награда опытом
			10, // Награда золотом
		),
		quest.NewDailyQuest(
			quest.DailyQuestTypeWinCombat,
			"Победить в бою",
			"Одолейте врагов в бою",
			1, // Целевое значение: 1 победа
			75, // Награда опытом
			15, // Награда золотом
		),
		quest.NewDailyQuest(
			quest.DailyQuestTypeExploreLocation,
			"Исследовать локацию",
			"Посетите новую локацию в мире",
			1, // Целевое значение: 1 локация
			25, // Награда опытом
			5,  // Награда золотом
		),
	}
	
	// Сохраняем задания
	for _, q := range quests {
		if err := r.db.WithContext(ctx).Create(q).Error; err != nil {
			// Логируем ошибку, но продолжаем создание остальных
			continue
		}
	}
	
	return quests
}

// GetPlayerProgress возвращает прогресс игрока по ежедневным заданиям на сегодня
func (r *DailyQuestRepository) GetPlayerProgress(ctx context.Context, playerID uint, date time.Time) ([]*quest.DailyQuestProgress, error) {
	var progress []*quest.DailyQuestProgress
	
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)
	
	err := r.db.WithContext(ctx).
		Preload("DailyQuest").
		Where("player_id = ? AND date >= ? AND date < ?", playerID, startOfDay, endOfDay).
		Find(&progress).Error
	
	if err != nil {
		return nil, err
	}
	
	return progress, nil
}

// SaveProgress сохраняет прогресс игрока по заданию
func (r *DailyQuestRepository) SaveProgress(ctx context.Context, progress *quest.DailyQuestProgress) error {
	return r.db.WithContext(ctx).
		Session(&gorm.Session{FullSaveAssociations: true}).
		Save(progress).Error
}

// GetOrCreateProgress получает или создает прогресс игрока по заданию на сегодня
func (r *DailyQuestRepository) GetOrCreateProgress(
	ctx context.Context,
	playerID uint,
	dailyQuestID uint,
	date time.Time,
) (*quest.DailyQuestProgress, error) {
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	
	var progress quest.DailyQuestProgress
	err := r.db.WithContext(ctx).
		Preload("DailyQuest").
		Where("player_id = ? AND daily_quest_id = ? AND date >= ? AND date < ?",
			playerID, dailyQuestID, startOfDay, startOfDay.Add(24*time.Hour)).
		First(&progress).Error
	
	if err == gorm.ErrRecordNotFound {
		// Создаем новый прогресс
		var dailyQuest quest.DailyQuest
		if err := r.db.WithContext(ctx).First(&dailyQuest, dailyQuestID).Error; err != nil {
			return nil, err
		}
		
		progress = quest.DailyQuestProgress{
			PlayerID:     playerID,
			DailyQuestID: dailyQuestID,
			CurrentValue: 0,
			TargetValue:  dailyQuest.TargetValue,
			Completed:    false,
			Date:         startOfDay,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		
		if err := r.db.WithContext(ctx).Create(&progress).Error; err != nil {
			return nil, err
		}
		
		return &progress, nil
	}
	
	if err != nil {
		return nil, err
	}
	
	return &progress, nil
}

// GetStreak возвращает стрик игрока
func (r *DailyQuestRepository) GetStreak(ctx context.Context, playerID uint) (*quest.DailyQuestStreak, error) {
	var streak quest.DailyQuestStreak
	err := r.db.WithContext(ctx).
		Where("player_id = ?", playerID).
		First(&streak).Error
	
	if err == gorm.ErrRecordNotFound {
		// Создаем новый стрик
		streak = quest.DailyQuestStreak{
			PlayerID:  playerID,
			StreakDays: 0,
			LastDate:   time.Time{},
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}
		if err := r.db.WithContext(ctx).Create(&streak).Error; err != nil {
			return nil, err
		}
		return &streak, nil
	}
	
	if err != nil {
		return nil, err
	}
	
	return &streak, nil
}

// UpdateStreak обновляет стрик игрока
func (r *DailyQuestRepository) UpdateStreak(ctx context.Context, streak *quest.DailyQuestStreak) error {
	streak.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Save(streak).Error
}
