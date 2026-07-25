package dm_tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"dungeons-and-dragons-ai/internal/game/domain/event"
	"dungeons-and-dragons-ai/pkg/logger"
)

// FollowupEventRepository интерфейс для сохранения событий дополнительных сообщений
type FollowupEventRepository interface {
	Save(ctx context.Context, e *event.StoryEvent) error
}

// SendFollowupMessageTool позволяет DM отправить дополнительное сообщение после обработки основного действия
// DM вызывает этот инструмент и знает, что к нему придет еще один запрос для генерации дополнительного сообщения
type SendFollowupMessageTool struct {
	eventRepo FollowupEventRepository
	sessionID uint
	chatID    int64
}

// NewSendFollowupMessageTool создает новый инструмент для отправки дополнительных сообщений
func NewSendFollowupMessageTool(eventRepo FollowupEventRepository, sessionID uint, chatID int64) *SendFollowupMessageTool {
	return &SendFollowupMessageTool{
		eventRepo: eventRepo,
		sessionID: sessionID,
		chatID:    chatID,
	}
}

func (t *SendFollowupMessageTool) Name() string {
	return "send_followup_message"
}

func (t *SendFollowupMessageTool) Description() string {
	return `Отправить дополнительное сообщение после обработки основного действия игрока.
Используй этот инструмент, когда нужно отправить дополнительное описание или информацию:
- Игрок открыл сундук → отправь основное сообщение, затем вызови этот инструмент для описания содержимого
- Игрок вступил в бой → отправь основное сообщение, затем вызови этот инструмент для описания первого хода врага
- Игрок использовал заклинание → отправь основное сообщение, затем вызови этот инструмент для описания эффекта

ВАЖНО: После вызова этого инструмента к тебе придет ДОПОЛНИТЕЛЬНЫЙ запрос для генерации сообщения.
В этом запросе будет указан message_type и context для генерации сообщения.

Параметры:
- message_type: тип сообщения (например, "item_description", "combat_turn", "spell_effect", "location_detail", "npc_dialogue")
- context: контекст для генерации сообщения (например, описание предмета, имя врага, название заклинания)

После вызова этого инструмента ты получишь запрос для генерации дополнительного сообщения.`
}

func (t *SendFollowupMessageTool) Parameters() json.RawMessage {
	properties := JSONSchemaProperties{
		"message_type": JSONSchemaParam{
			Type:        "string",
			Description: "Тип сообщения: 'item_description', 'combat_turn', 'spell_effect', 'location_detail', 'npc_dialogue', или другой подходящий тип",
			Required:    true,
		},
		"context": JSONSchemaParam{
			Type:        "string",
			Description: "Контекст для генерации сообщения (например, описание предмета, имя врага, название заклинания, описание локации)",
			Required:    true,
		},
	}
	return BuildJSONSchema(properties, []string{"message_type", "context"})
}

func (t *SendFollowupMessageTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	// Извлекаем параметры
	messageType, ok := args["message_type"].(string)
	if !ok || messageType == "" {
		return nil, fmt.Errorf("message_type is required and must be a string")
	}

	contextStr, ok := args["context"].(string)
	if !ok || contextStr == "" {
		return nil, fmt.Errorf("context is required and must be a string")
	}

	logger.Info("SendFollowupMessageTool: executing",
		logger.Uint("session_id", t.sessionID),
		logger.String("message_type", messageType),
		logger.Int64("chat_id", t.chatID),
	)

	// Создаем событие-метку для указания, что нужно сгенерировать дополнительное сообщение
	// Это событие будет использовано для создания запроса к DM для генерации дополнительного сообщения
	// Сохраняем информацию в Content в структурированном формате для последующего парсинга
	followupEvent := &event.StoryEvent{
		GameSessionID: t.sessionID,
		AuthorType:    event.AuthorTypeDM,
		Content:       fmt.Sprintf("[FOLLOWUP_REQUEST] message_type=%s context=%s", messageType, contextStr),
		CreatedAt:     time.Now(),
	}

	// Сохраняем событие в БД
	if err := t.eventRepo.Save(ctx, followupEvent); err != nil {
		logger.Error("SendFollowupMessageTool: failed to save followup event",
			logger.Uint("session_id", t.sessionID),
			logger.ErrorField(err),
		)
		return nil, fmt.Errorf("failed to save followup event: %w", err)
	}

	logger.Info("SendFollowupMessageTool: followup event saved",
		logger.Uint("session_id", t.sessionID),
		logger.String("message_type", messageType),
		logger.Uint("event_id", followupEvent.ID),
	)

	// Возвращаем результат с информацией о том, что запрос на дополнительное сообщение создан
	result := map[string]interface{}{
		"message_type": messageType,
		"context":      contextStr,
		"status":       "followup_request_created",
		"message":      fmt.Sprintf("Запрос на дополнительное сообщение создан. Тип: %s", messageType),
	}

	return result, nil
}
