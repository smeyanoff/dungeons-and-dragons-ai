package ability_check

import (
	"context"
	"fmt"
	"time"

	"dungeons-and-dragons-ai/internal/game/domain/character"
	"dungeons-and-dragons-ai/internal/game/domain/dice"
	"dungeons-and-dragons-ai/internal/game/domain/event"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/rag/application"
	ragdomain "dungeons-and-dragons-ai/internal/rag/domain"
	"dungeons-and-dragons-ai/pkg/logger"

	"github.com/google/uuid"
)

type SessionRepository interface {
	GetByChatID(ctx context.Context, chatID int64) (*session.GameSession, error)
	Save(ctx context.Context, gs *session.GameSession) error
}

type EventRepository interface {
	Save(ctx context.Context, e *event.StoryEvent) error
}

type PerformAbilityCheckUseCase struct {
	sessionRepo SessionRepository
	eventRepo   EventRepository
	indexDocUC  *application.IndexDocument
}

type PerformAbilityCheckResult struct {
	Message         string
	Ability         string
	DC              int    // Адаптивный DC
	BaseDC          int    // Базовый DC
	DifficultyDesc  string // Описание сложности
	BaseRoll        int
	Modifier        int
	Total           int
	Success         bool
	CharacterName   string
}

func NewPerformAbilityCheckUseCase(
	sessionRepo SessionRepository,
	eventRepo EventRepository,
	indexDocUC *application.IndexDocument,
) *PerformAbilityCheckUseCase {
	return &PerformAbilityCheckUseCase{
		sessionRepo: sessionRepo,
		eventRepo:   eventRepo,
		indexDocUC:  indexDocUC,
	}
}

func (uc *PerformAbilityCheckUseCase) Execute(ctx context.Context, chatID int64) (*PerformAbilityCheckResult, error) {
	return uc.execute(ctx, chatID, nil)
}

// ExecuteWithBaseRoll выполняет проверку с ручным значением d20.
// baseRoll должен быть в диапазоне 1..20.
func (uc *PerformAbilityCheckUseCase) ExecuteWithBaseRoll(ctx context.Context, chatID int64, baseRoll int) (*PerformAbilityCheckResult, error) {
	if baseRoll < 1 || baseRoll > 20 {
		return nil, fmt.Errorf("base roll must be between 1 and 20")
	}
	return uc.execute(ctx, chatID, &baseRoll)
}

func (uc *PerformAbilityCheckUseCase) execute(ctx context.Context, chatID int64, baseRollOverride *int) (*PerformAbilityCheckResult, error) {
	gs, err := uc.sessionRepo.GetByChatID(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}
	if gs == nil {
		return nil, fmt.Errorf("session not found")
	}
	if !gs.HasPendingAbilityCheck() {
		return nil, fmt.Errorf("no pending ability check")
	}

	// Проверяем cooldown для типа способности
	const cooldownDuration = 30 * time.Second // 30 секунд cooldown
	ability := gs.PendingAbilityCheckAbility
	if onCooldown, remainingTime := gs.IsAbilityOnCooldown(ability, cooldownDuration); onCooldown {
		return nil, fmt.Errorf("проверка способности '%s' на cooldown. Осталось: %.0f сек", ability, remainingTime.Seconds())
	}

	player := gs.GetFirstPlayer()
	if player == nil {
		return nil, fmt.Errorf("player not found")
	}

	baseDC := gs.PendingAbilityCheckDC

	// Применяем адаптивную сложность
	dc := gs.GetAdaptiveDC(baseDC)

	abilityName, abilityValue := resolveAbility(player.Character.Stats, ability)
	modifier := dice.CalculateModifier(abilityValue)

	baseRoll := 0
	total := 0
	if baseRollOverride != nil {
		baseRoll = *baseRollOverride
		total = baseRoll + modifier
	} else {
		rollResult, err := dice.RollWithModifier("d20", modifier)
		if err != nil {
			return nil, fmt.Errorf("failed to roll dice: %w", err)
		}
		if len(rollResult.Rolls) > 0 {
			baseRoll = rollResult.Rolls[0]
		}
		total = rollResult.Total
	}
	success := total >= dc

	// Записываем результат в статистику адаптивной сложности
	gs.RecordAbilityCheckResult(success)

	outcome := "Провал"
	if success {
		outcome = "Успех"
	}

	difficultyDesc := gs.GetDifficultyDescription()
	message := fmt.Sprintf("🎲 Проверка %s (DC %d, сложность: %s): d20=%d %+d = %d. %s.",
		abilityName, dc, difficultyDesc, baseRoll, modifier, total, outcome)

	// Сохраняем событие и индексируем в RAG
	eventItem := &event.StoryEvent{
		GameSessionID: gs.ID,
		AuthorType:    event.AuthorTypeDM,
		Content:       message,
		CreatedAt:     time.Now(),
	}
	if err := uc.eventRepo.Save(ctx, eventItem); err != nil {
		logger.Error("Failed to save ability check event",
			logger.ErrorField(err),
			logger.Uint("session_id", gs.ID),
		)
	} else if uc.indexDocUC != nil {
		doc := ragdomain.Document{
			ID:        uuid.New().String(),
			Source:    ragdomain.SourceEvent,
			SessionID: gs.ID,
			Text:      message,
			Timestamp: time.Now(),
		}
		if err := uc.indexDocUC.Execute(ctx, doc); err != nil {
			logger.Warn("Failed to index ability check event",
				logger.ErrorField(err),
				logger.Uint("session_id", gs.ID),
			)
		}
	}

	// Устанавливаем cooldown для типа способности
	gs.SetAbilityCooldown(ability)

	gs.ClearPendingAbilityCheck()
	if err := uc.sessionRepo.Save(ctx, gs); err != nil {
		logger.Error("Failed to clear pending ability check",
			logger.ErrorField(err),
			logger.Uint("session_id", gs.ID),
		)
	}

	return &PerformAbilityCheckResult{
		Message:        message,
		Ability:        ability,
		DC:             dc,
		BaseDC:         baseDC,
		DifficultyDesc: difficultyDesc,
		BaseRoll:       baseRoll,
		Modifier:       modifier,
		Total:          total,
		Success:        success,
		CharacterName:  player.Character.Name,
	}, nil
}

func resolveAbility(stats character.Stats, ability string) (string, int) {
	switch ability {
	case "strength":
		return "Сила (STR)", stats.Strength
	case "dexterity":
		return "Ловкость (DEX)", stats.Dexterity
	case "constitution":
		return "Телосложение (CON)", stats.Constitution
	case "intelligence":
		return "Интеллект (INT)", stats.Intelligence
	case "wisdom":
		return "Мудрость (WIS)", stats.Wisdom
	case "charisma":
		return "Харизма (CHA)", stats.Charisma
	default:
		return "Неизвестная характеристика", 10
	}
}
