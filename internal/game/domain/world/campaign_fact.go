package world

import "time"

// CampaignFactCategory классифицирует устойчивый факт кампании.
type CampaignFactCategory string

const (
	FactCategoryReputation   CampaignFactCategory = "reputation"   // Репутация игрока/группы
	FactCategoryQuest        CampaignFactCategory = "quest"         // Веха главного или побочного квеста
	FactCategoryDecision     CampaignFactCategory = "decision"      // Решение игрока с долгими последствиями
	FactCategoryRelationship CampaignFactCategory = "relationship" // Отношения с NPC/фракцией
)

// CampaignFact — курируемый устойчивый факт кампании (не привязан к конкретной локации).
// В отличие от StoryEvent/RAG-документов, это небольшой список ключевых фактов,
// которые должны быть видны DM независимо от того, в какой локации сейчас находится игрок.
type CampaignFact struct {
	ID        uint                 `gorm:"primaryKey"`
	WorldID   uint                 `gorm:"index"`
	Category  CampaignFactCategory `gorm:"type:varchar(32);not null"`
	Text      string               `gorm:"type:text;not null"`
	CreatedAt time.Time
}
