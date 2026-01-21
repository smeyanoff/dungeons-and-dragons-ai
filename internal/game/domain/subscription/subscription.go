package subscription

import (
	"time"
)

// Plan представляет тарифный план подписки
type Plan string

const (
	PlanFree    Plan = "free"    // Бесплатный тариф
	PlanPremium Plan = "premium" // Премиум тариф (₽299/мес)
	PlanPro     Plan = "pro"     // Про тариф (₽599/мес)
)

// PlanDetails содержит детали тарифного плана
type PlanDetails struct {
	Plan               Plan
	Name               string
	Price              int  // Цена в рублях в месяц
	MaxActiveGames     int  // Максимальное количество активных игр (0 = безлимит)
	MaxMessagesPerDay  int  // Максимальное количество сообщений в день (0 = безлимит)
	MaxImagesPerDay    int  // Максимальное количество изображений в день (0 = безлимит)
	MaxSaves           int  // Максимальное количество сохранений (0 = безлимит)
	MaxInventorySlots  int  // Максимальное количество слотов в инвентаре
	MaxPlayersPerGame  int  // Максимальное количество игроков в игре (0 = безлимит)
	PriorityProcessing bool // Приоритетная обработка запросов
	APIAccess          bool // Доступ к API
	CustomMods         bool // Доступ к кастомным модам
	ExclusiveWorlds    bool // Доступ к эксклюзивным мирам
	PrioritySupport    bool // Приоритетная поддержка
	Analytics          bool // Доступ к аналитике
	ExportHistory      bool // Экспорт истории
}

// GetPlanDetails возвращает детали тарифного плана
func GetPlanDetails(plan Plan) PlanDetails {
	switch plan {
	case PlanPremium:
		return PlanDetails{
			Plan:               PlanPremium,
			Name:               "Premium",
			Price:              299,
			MaxActiveGames:     0, // безлимит
			MaxMessagesPerDay:  0, // безлимит
			MaxImagesPerDay:    0, // безлимит
			MaxSaves:           10,
			MaxInventorySlots:  50, // базовый + расширенный
			MaxPlayersPerGame:  1,
			PriorityProcessing: true,
			APIAccess:          false,
			CustomMods:         false,
			ExclusiveWorlds:    true,
			PrioritySupport:    true,
			Analytics:          true,
			ExportHistory:      true,
		}
	case PlanPro:
		return PlanDetails{
			Plan:               PlanPro,
			Name:               "Pro",
			Price:              599,
			MaxActiveGames:     0, // безлимит
			MaxMessagesPerDay:  0, // безлимит
			MaxImagesPerDay:    0, // безлимит
			MaxSaves:           0, // безлимит
			MaxInventorySlots:  70,
			MaxPlayersPerGame:  8,
			PriorityProcessing: true,
			APIAccess:          true,
			CustomMods:         true,
			ExclusiveWorlds:    true,
			PrioritySupport:    true,
			Analytics:          true,
			ExportHistory:      true,
		}
	default: // PlanFree
		return PlanDetails{
			Plan:               PlanFree,
			Name:               "Free",
			Price:              0,
			MaxActiveGames:     1,
			MaxMessagesPerDay:  50,
			MaxImagesPerDay:    5,
			MaxSaves:           1,
			MaxInventorySlots:  30,
			MaxPlayersPerGame:  1,
			PriorityProcessing: false,
			APIAccess:          false,
			CustomMods:         false,
			ExclusiveWorlds:    false,
			PrioritySupport:    false,
			Analytics:          false,
			ExportHistory:      false,
		}
	}
}

// Subscription представляет подписку пользователя
type Subscription struct {
	ID       uint  `gorm:"primaryKey"`
	TgUserID int64 `gorm:"uniqueIndex;not null"` // Telegram User ID
	Plan     Plan  `gorm:"type:varchar(16);not null;default:'free'"`

	// Статус подписки
	Status Status `gorm:"type:varchar(16);not null;default:'active'"`

	// Даты подписки
	StartedAt  *time.Time `gorm:"type:timestamp"`
	ExpiresAt  *time.Time `gorm:"type:timestamp"` // null для бессрочных подписок
	CanceledAt *time.Time `gorm:"type:timestamp"` // Дата отмены (для автоотмены)

	// Платежная информация (опционально, для интеграции с платежными системами)
	PaymentID       string `gorm:"type:varchar(128)"` // ID платежа в платежной системе
	PaymentProvider string `gorm:"type:varchar(32)"`  // Провайдер (yookassa, stripe)

	// Метаданные
	AutoRenew bool `gorm:"default:false"` // Автопродление подписки
	TrialDays int  `gorm:"default:0"`     // Дни пробного периода
	TrialUsed bool `gorm:"default:false"` // Использован ли пробный период

	CreatedAt time.Time
	UpdatedAt time.Time
}

// Status представляет статус подписки
type Status string

const (
	StatusActive   Status = "active"   // Активная подписка
	StatusExpired  Status = "expired"  // Истекшая подписка
	StatusCanceled Status = "canceled" // Отмененная подписка
	StatusTrial    Status = "trial"    // Пробный период
)

// IsActive проверяет, активна ли подписка
func (s *Subscription) IsActive() bool {
	if s.Status != StatusActive && s.Status != StatusTrial {
		return false
	}

	// Если есть дата истечения, проверяем её
	if s.ExpiresAt != nil {
		return time.Now().Before(*s.ExpiresAt)
	}

	// Если нет даты истечения, подписка бессрочная (активна)
	return true
}

// GetPlan возвращает текущий план подписки
func (s *Subscription) GetPlan() Plan {
	if !s.IsActive() {
		return PlanFree
	}
	return s.Plan
}

// GetPlanDetails возвращает детали текущего плана подписки
func (s *Subscription) GetPlanDetails() PlanDetails {
	return GetPlanDetails(s.GetPlan())
}

// CanUseFeature проверяет, может ли пользователь использовать функцию
func (s *Subscription) CanUseFeature(feature string) bool {
	plan := s.GetPlan()
	details := GetPlanDetails(plan)

	switch feature {
	case "priority_processing":
		return details.PriorityProcessing
	case "api_access":
		return details.APIAccess
	case "custom_mods":
		return details.CustomMods
	case "exclusive_worlds":
		return details.ExclusiveWorlds
	case "priority_support":
		return details.PrioritySupport
	case "analytics":
		return details.Analytics
	case "export_history":
		return details.ExportHistory
	default:
		return false
	}
}

// DaysRemaining возвращает количество оставшихся дней подписки (или -1 для бессрочных)
func (s *Subscription) DaysRemaining() int {
	if s.ExpiresAt == nil {
		return -1 // бессрочная
	}

	now := time.Now()
	if now.After(*s.ExpiresAt) {
		return 0 // истекла
	}

	diff := s.ExpiresAt.Sub(now)
	return int(diff.Hours() / 24)
}
