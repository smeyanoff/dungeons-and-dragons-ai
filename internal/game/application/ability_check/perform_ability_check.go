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
	Message       string
	Ability       string
	DC            int
	BaseRoll      int
	Modifier      int
	Total         int
	Success       bool
	CharacterName string
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

	player := gs.GetFirstPlayer()
	if player == nil {
		return nil, fmt.Errorf("player not found")
	}

	ability := gs.PendingAbilityCheckAbility
	dc := gs.PendingAbilityCheckDC

	abilityName, abilityValue := resolveAbility(player.Character.Stats, ability)
	modifier := dice.CalculateModifier(abilityValue)

	rollResult, err := dice.RollWithModifier("d20", modifier)
	if err != nil {
		return nil, fmt.Errorf("failed to roll dice: %w", err)
	}

	baseRoll := 0
	if len(rollResult.Rolls) > 0 {
		baseRoll = rollResult.Rolls[0]
	}
	total := rollResult.Total
	success := total >= dc

	outcome := "Провал"
	if success {
		outcome = "Успех"
	}

	message := fmt.Sprintf("🎲 Проверка %s (DC %d): d20=%d %+d = %d. %s.",
		abilityName, dc, baseRoll, modifier, total, outcome)

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

	gs.ClearPendingAbilityCheck()
	if err := uc.sessionRepo.Save(ctx, gs); err != nil {
		logger.Error("Failed to clear pending ability check",
			logger.ErrorField(err),
			logger.Uint("session_id", gs.ID),
		)
	}

	return &PerformAbilityCheckResult{
		Message:       message,
		Ability:       ability,
		DC:            dc,
		BaseRoll:      baseRoll,
		Modifier:      modifier,
		Total:         total,
		Success:       success,
		CharacterName: player.Character.Name,
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
