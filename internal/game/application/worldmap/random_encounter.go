package worldmap

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"dungeons-and-dragons-ai/internal/game/domain/event"
	"dungeons-and-dragons-ai/internal/game/domain/session"
	"dungeons-and-dragons-ai/internal/game/domain/world"
	ragdomain "dungeons-and-dragons-ai/internal/rag/domain"

	"github.com/google/uuid"
)

// rng — генератор для броска шанса случайной встречи. Не криптостойкий и не
// используется для игровых бросков кубиков (за это отвечает internal/game/domain/dice) —
// только для решения "происходит ли встреча" при переходе между локациями.
var rng = rand.New(rand.NewSource(time.Now().UnixNano()))

const (
	// randomEncounterBaseChancePercent — базовый шанс встречи при переходе (в процентах).
	randomEncounterBaseChancePercent = 15
	// randomEncounterChancePerHourPercent — дополнительный шанс за каждый час в пути.
	randomEncounterChancePerHourPercent = 5
	// randomEncounterMaxChancePercent — потолок шанса встречи, чтобы долгие переходы не были гарантированной встречей.
	randomEncounterMaxChancePercent = 60
)

// randomEncounterChance считает шанс встречи (0-100) в зависимости от времени в пути.
// Мгновенное перемещение (портал, 0 часов) встреч не порождает.
func randomEncounterChance(travelHours int) int {
	if travelHours <= 0 {
		return 0
	}
	chance := randomEncounterBaseChancePercent + travelHours*randomEncounterChancePerHourPercent
	if chance > randomEncounterMaxChancePercent {
		chance = randomEncounterMaxChancePercent
	}
	return chance
}

// maybeGenerateRandomEncounter с некоторым шансом порождает случайную встречу на пути
// между локациями. Встреча привязывается к целевой локации (to) как активное
// world.WorldEvent типа WorldEventTypeRandomEncounter — она разрешается той же
// инфраструктурой веток/проверок, что и обычные события локаций
// (см. isLocationEventType и разрешение через resolveLocationEventFromCheck
// в player_action), и автоматически отменяется при уходе из локации без
// разрешения (resolveLocationEventsOnLeave).
//
// Возвращает текст встречи для добавления в сообщение о перемещении, либо
// пустую строку, если встреча не произошла.
func (uc *MoveToLocationUseCase) maybeGenerateRandomEncounter(
	ctx context.Context,
	gs *session.GameSession,
	from, to *world.Location,
	travelHours int,
) string {
	if uc.worldEventRepo == nil || to == nil {
		return ""
	}

	chance := randomEncounterChance(travelHours)
	if chance <= 0 {
		return ""
	}

	// Не накладываем встречу поверх уже неразрешённого события в целевой локации.
	existing, err := uc.worldEventRepo.GetByLocationID(ctx, to.ID)
	if err != nil {
		return ""
	}
	for i := range existing {
		if isLocationEventType(existing[i].Type) && existing[i].Status == world.WorldEventStatusActive {
			return ""
		}
	}

	if rng.Intn(100) >= chance {
		return ""
	}

	fromName := to.Name
	if from != nil {
		fromName = from.Name
	}

	locationID := to.ID
	now := time.Now()
	name := fmt.Sprintf("Случайная встреча на пути в %s", to.Name)
	description := fmt.Sprintf(
		"На пути из %s в %s вам преграждает дорогу неизвестная опасность. Что вы предпримете?",
		fromName, to.Name,
	)
	branches := []world.LocationEventBranch{
		{
			ID:             "negotiate",
			Name:           "Договориться",
			Description:    "Попытаться решить дело миром",
			RequiredAction: "договориться мирно",
			SuccessRate:    55,
			Consequences:   "Можно разойтись без стычки или получить полезные сведения",
			Reward:         "информация или свободный проход",
		},
		{
			ID:             "flee",
			Name:           "Уйти от встречи",
			Description:    "Свернуть с пути или отступить, не вступая в контакт",
			RequiredAction: "уклониться от встречи",
			SuccessRate:    65,
			Consequences:   "Позволяет избежать риска, но встреча может повториться позже",
			Reward:         "",
		},
		{
			ID:             "fight",
			Name:           "Принять бой",
			Description:    "Встретить опасность с оружием наготове",
			RequiredAction: "вступить в бой",
			SuccessRate:    50,
			Consequences:   "Победа приносит трофеи, поражение — урон и потерю времени",
			Reward:         "трофеи противника",
		},
	}

	ev := &world.WorldEvent{
		WorldID:            gs.World.ID,
		Type:               world.WorldEventTypeRandomEncounter,
		Status:             world.WorldEventStatusActive,
		Name:               name,
		Description:        description,
		Metadata:           buildRandomEncounterMetadata(description, branches),
		RequiredLocationID: &locationID,
		ActivatedAt:        &now,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if err := uc.worldEventRepo.Save(ctx, ev); err != nil {
		return ""
	}

	if uc.eventRepo != nil {
		storyItem := &event.StoryEvent{
			GameSessionID: gs.ID,
			LocationID:    &locationID,
			AuthorType:    event.AuthorTypeDM,
			Content:       description,
			CreatedAt:     now,
		}
		_ = uc.eventRepo.Save(ctx, storyItem)
	}

	if uc.indexDocUC != nil {
		doc := ragdomain.Document{
			ID:         uuid.New().String(),
			Source:     ragdomain.SourceEvent,
			SessionID:  gs.ID,
			LocationID: &locationID,
			Text:       description,
			Timestamp:  now,
		}
		_ = uc.indexDocUC.Execute(ctx, doc)
	}

	return "\n\n⚔️ " + description
}

func buildRandomEncounterMetadata(hook string, branches []world.LocationEventBranch) []byte {
	options := make([]string, len(branches))
	for i, branch := range branches {
		options[i] = branch.Name
	}

	meta := world.LocationEventMetadata{
		Hook:      hook,
		Options:   options,
		Status:    "pending",
		Branches:  branches,
		EventType: string(world.WorldEventTypeRandomEncounter),
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		return nil
	}
	return raw
}
