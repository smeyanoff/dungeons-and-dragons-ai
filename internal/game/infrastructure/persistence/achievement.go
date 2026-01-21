package persistence

import (
	"context"

	"gorm.io/gorm"

	"dungeons-and-dragons-ai/internal/game/domain/achievement"
)

type AchievementRepository struct {
	db *gorm.DB
}

func NewAchievementRepository(db *gorm.DB) *AchievementRepository {
	return &AchievementRepository{db: db}
}

// GetByCode получает достижение по коду
func (r *AchievementRepository) GetByCode(ctx context.Context, code string) (*achievement.Achievement, error) {
	var a achievement.Achievement
	err := r.db.WithContext(ctx).
		Where("code = ?", code).
		First(&a).Error

	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &a, nil
}

// GetAll получает все достижения
func (r *AchievementRepository) GetAll(ctx context.Context) ([]*achievement.Achievement, error) {
	var achievements []*achievement.Achievement
	err := r.db.WithContext(ctx).
		Find(&achievements).Error

	if err != nil {
		return nil, err
	}

	return achievements, nil
}

// GetByType получает достижения по типу
func (r *AchievementRepository) GetByType(ctx context.Context, achievementType achievement.AchievementType) ([]*achievement.Achievement, error) {
	var achievements []*achievement.Achievement
	err := r.db.WithContext(ctx).
		Where("type = ?", achievementType).
		Find(&achievements).Error

	if err != nil {
		return nil, err
	}

	return achievements, nil
}

// GetPlayerAchievements получает все достижения игрока
func (r *AchievementRepository) GetPlayerAchievements(ctx context.Context, playerID uint) ([]*achievement.PlayerAchievement, error) {
	var playerAchievements []*achievement.PlayerAchievement
	err := r.db.WithContext(ctx).
		Preload("Achievement").
		Where("player_id = ?", playerID).
		Order("earned_at DESC").
		Find(&playerAchievements).Error

	if err != nil {
		return nil, err
	}

	return playerAchievements, nil
}

// GetPlayerAchievementByCode получает достижение игрока по коду достижения
func (r *AchievementRepository) GetPlayerAchievementByCode(ctx context.Context, playerID uint, code string) (*achievement.PlayerAchievement, error) {
	var pa achievement.PlayerAchievement
	err := r.db.WithContext(ctx).
		Preload("Achievement").
		Joins("JOIN achievements ON achievements.id = player_achievements.achievement_id").
		Where("player_achievements.player_id = ? AND achievements.code = ?", playerID, code).
		First(&pa).Error

	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &pa, nil
}

// SavePlayerAchievement сохраняет достижение игрока
func (r *AchievementRepository) SavePlayerAchievement(ctx context.Context, pa *achievement.PlayerAchievement) error {
	return r.db.WithContext(ctx).
		Session(&gorm.Session{FullSaveAssociations: true}).
		Save(pa).Error
}

// GetAchievementProgress получает прогресс достижения для игрока
func (r *AchievementRepository) GetAchievementProgress(ctx context.Context, playerID uint, achievementID uint) (*achievement.AchievementProgress, error) {
	var progress achievement.AchievementProgress
	err := r.db.WithContext(ctx).
		Where("player_id = ? AND achievement_id = ?", playerID, achievementID).
		First(&progress).Error

	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &progress, nil
}

// SaveAchievementProgress сохраняет прогресс достижения
func (r *AchievementRepository) SaveAchievementProgress(ctx context.Context, progress *achievement.AchievementProgress) error {
	return r.db.WithContext(ctx).
		Save(progress).Error
}

// GetAchievementProgressByRequirementKey получает прогресс достижения для игрока по ключу требования
// Возвращает текущее значение прогресса (0 если прогресса нет)
func (r *AchievementRepository) GetAchievementProgressByRequirementKey(ctx context.Context, playerID uint, requirementKey string) (int, error) {
	// Получаем достижение по ключу требования
	allAchievements, err := r.GetAll(ctx)
	if err != nil {
		return 0, err
	}

	// Находим достижение с указанным ключом
	var targetAchievement *achievement.Achievement
	for _, a := range allAchievements {
		if a.RequirementKey == requirementKey {
			targetAchievement = a
			break
		}
	}

	if targetAchievement == nil {
		// Достижения с таким ключом нет, возвращаем 0
		return 0, nil
	}

	// Получаем прогресс по этому достижению
	progress, err := r.GetAchievementProgress(ctx, playerID, targetAchievement.ID)
	if err != nil {
		return 0, err
	}

	if progress == nil {
		return 0, nil
	}

	return progress.CurrentValue, nil
}

// Save сохраняет достижение
func (r *AchievementRepository) Save(ctx context.Context, a *achievement.Achievement) error {
	return r.db.WithContext(ctx).
		Save(a).Error
}

// InitDefaultAchievements инициализирует базовые достижения в БД
func (r *AchievementRepository) InitDefaultAchievements(ctx context.Context) error {
	defaultAchievements := []*achievement.Achievement{
		// Combat достижения
		{
			Code:             "first_combat",
			Title:            "Первый бой",
			Description:      "Участвуйте в первом бою",
			Type:             achievement.AchievementTypeCombat,
			Rarity:           achievement.RarityCommon,
			RequirementValue: 1,
			RequirementKey:   "combat_participated",
			ExperienceReward: 50,
			GoldReward:       10,
			Icon:             "⚔️",
			Category:         "Бой",
			IsHidden:         false,
			IsRepeatable:     false,
		},
		{
			Code:             "combat_victor_10",
			Title:            "Победитель",
			Description:      "Победите в 10 боях",
			Type:             achievement.AchievementTypeCombat,
			Rarity:           achievement.RarityUncommon,
			RequirementValue: 10,
			RequirementKey:   "combat_wins",
			ExperienceReward: 200,
			GoldReward:       50,
			Icon:             "🏆",
			Category:         "Бой",
			IsHidden:         false,
			IsRepeatable:     false,
		},
		{
			Code:             "combat_victor_100",
			Title:            "Ветеран битв",
			Description:      "Победите в 100 боях",
			Type:             achievement.AchievementTypeCombat,
			Rarity:           achievement.RarityRare,
			RequirementValue: 100,
			RequirementKey:   "combat_wins",
			ExperienceReward: 1000,
			GoldReward:       500,
			Icon:             "⚔️💀",
			Category:         "Бой",
			IsHidden:         false,
			IsRepeatable:     false,
		},
		// Quest достижения
		{
			Code:             "first_quest",
			Title:            "Первый квест",
			Description:      "Завершите первый квест",
			Type:             achievement.AchievementTypeQuest,
			Rarity:           achievement.RarityCommon,
			RequirementValue: 1,
			RequirementKey:   "quests_completed",
			ExperienceReward: 100,
			GoldReward:       20,
			Icon:             "📜",
			Category:         "Квесты",
			IsHidden:         false,
			IsRepeatable:     false,
		},
		{
			Code:             "quest_master_10",
			Title:            "Мастер квестов",
			Description:      "Завершите 10 квестов",
			Type:             achievement.AchievementTypeQuest,
			Rarity:           achievement.RarityUncommon,
			RequirementValue: 10,
			RequirementKey:   "quests_completed",
			ExperienceReward: 500,
			GoldReward:       200,
			Icon:             "📜✨",
			Category:         "Квесты",
			IsHidden:         false,
			IsRepeatable:     false,
		},
		// Progress достижения
		{
			Code:             "level_5",
			Title:            "Опытный воин",
			Description:      "Достигните 5 уровня",
			Type:             achievement.AchievementTypeProgress,
			Rarity:           achievement.RarityCommon,
			RequirementValue: 5,
			RequirementKey:   "character_level",
			ExperienceReward: 100,
			GoldReward:       50,
			Icon:             "⭐",
			Category:         "Прогресс",
			IsHidden:         false,
			IsRepeatable:     false,
		},
		{
			Code:             "level_10",
			Title:            "Мастер",
			Description:      "Достигните 10 уровня",
			Type:             achievement.AchievementTypeProgress,
			Rarity:           achievement.RarityRare,
			RequirementValue: 10,
			RequirementKey:   "character_level",
			ExperienceReward: 500,
			GoldReward:       250,
			Icon:             "⭐⭐",
			Category:         "Прогресс",
			IsHidden:         false,
			IsRepeatable:     false,
		},
		{
			Code:             "explorer",
			Title:            "Исследователь",
			Description:      "Посетите 5 различных локаций",
			Type:             achievement.AchievementTypeExploration,
			Rarity:           achievement.RarityUncommon,
			RequirementValue: 5,
			RequirementKey:   "locations_visited",
			ExperienceReward: 150,
			GoldReward:       30,
			Icon:             "🗺️",
			Category:         "Исследование",
			IsHidden:         false,
			IsRepeatable:     false,
		},
		// Collection достижения
		{
			Code:             "collector",
			Title:            "Коллекционер",
			Description:      "Соберите 20 различных предметов",
			Type:             achievement.AchievementTypeCollection,
			Rarity:           achievement.RarityUncommon,
			RequirementValue: 20,
			RequirementKey:   "items_collected",
			ExperienceReward: 300,
			GoldReward:       100,
			Icon:             "🎒",
			Category:         "Коллекции",
			IsHidden:         false,
			IsRepeatable:     false,
		},
	}

	for _, a := range defaultAchievements {
		// Проверяем, существует ли уже достижение с таким кодом
		existing, err := r.GetByCode(ctx, a.Code)
		if err != nil {
			return err
		}
		if existing == nil {
			// Создаем новое достижение
			if err := r.Save(ctx, a); err != nil {
				return err
			}
		}
	}

	return nil
}
