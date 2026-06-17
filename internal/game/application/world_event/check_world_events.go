package world_event

import (
	"context"
	"fmt"

	"dungeons-and-dragons-ai/internal/game/domain/world"
)

// WorldEventRepository интерфейс репозитория мировых событий
type WorldEventRepository interface {
	GetScheduledByWorldID(ctx context.Context, worldID uint) ([]world.WorldEvent, error)
	GetActiveByWorldID(ctx context.Context, worldID uint) ([]world.WorldEvent, error)
	Save(ctx context.Context, e *world.WorldEvent) error
}

// CheckWorldEventsUseCase проверяет и активирует мировые события
type CheckWorldEventsUseCase struct {
	eventRepo WorldEventRepository
}

func NewCheckWorldEventsUseCase(eventRepo WorldEventRepository) *CheckWorldEventsUseCase {
	return &CheckWorldEventsUseCase{
		eventRepo: eventRepo,
	}
}

// CheckWorldEventsRequest запрос на проверку мировых событий
type CheckWorldEventsRequest struct {
	WorldID           uint
	CurrentLocationID *uint // Текущая локация игрока (может быть nil)
}

// CheckWorldEventsResponse ответ с информацией об активированных событиях
type CheckWorldEventsResponse struct {
	ActivatedEvents []world.WorldEvent
	ActiveEvents    []world.WorldEvent
}

// Execute проверяет запланированные события и активирует те, которые соответствуют условиям
func (uc *CheckWorldEventsUseCase) Execute(
	ctx context.Context,
	req CheckWorldEventsRequest,
	w *world.World,
) (*CheckWorldEventsResponse, error) {
	// Получаем все запланированные события для мира
	scheduledEvents, err := uc.eventRepo.GetScheduledByWorldID(ctx, req.WorldID)
	if err != nil {
		return nil, fmt.Errorf("failed to get scheduled events: %w", err)
	}

	// Получаем текущие активные события
	activeEvents, err := uc.eventRepo.GetActiveByWorldID(ctx, req.WorldID)
	if err != nil {
		return nil, fmt.Errorf("failed to get active events: %w", err)
	}

	// Вычисляем параметры текущего состояния мира
	dayOfWeek := w.GetDayOfWeek()
	timeOfDay := w.TimeOfDay
	weather := w.Weather

	// Проверяем каждое запланированное событие
	activatedEvents := make([]world.WorldEvent, 0)
	for i := range scheduledEvents {
		event := &scheduledEvents[i]

		// Проверяем, может ли событие быть активировано
		if event.CanActivate(dayOfWeek, timeOfDay, weather, req.CurrentLocationID) {
			// Активируем событие
			event.Activate()

			// Сохраняем активированное событие
			if err := uc.eventRepo.Save(ctx, event); err != nil {
				// Логируем ошибку, но продолжаем обработку других событий
				fmt.Printf("Failed to save activated event %d: %v\n", event.ID, err)
				continue
			}

			activatedEvents = append(activatedEvents, *event)
		}
	}

	return &CheckWorldEventsResponse{
		ActivatedEvents: activatedEvents,
		ActiveEvents:    activeEvents,
	}, nil
}

// GetActiveEventsDescription формирует описание активных событий для включения в контекст DM
func (uc *CheckWorldEventsUseCase) GetActiveEventsDescription(events []world.WorldEvent) string {
	if len(events) == 0 {
		return ""
	}

	description := "\n\nАктивные события в мире:\n"
	for _, event := range events {
		description += fmt.Sprintf("- %s (%s): %s\n", event.Name, event.Type, event.Description)
	}

	return description
}
