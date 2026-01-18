package dm_tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"dungeons-and-dragons-ai/pkg/logger"
)

// ImageGenerationService интерфейс для генерации изображений (разрывает циклическую зависимость)
type ImageGenerationService interface {
	// GenerateImage генерирует изображение по запросу
	// Request должен иметь поля: SystemPrompt, UserPrompt, Type, EntityID, ForceRegenerate, UserID, SkipLimitCheck
	GenerateImage(ctx context.Context, req GenerateImageRequest) (*GenerateImageResponse, error)
}

// GenerateImageRequest запрос на генерацию изображения (локальный тип для dm_tools)
type GenerateImageRequest struct {
	SystemPrompt   string
	UserPrompt     string
	Type           string
	EntityID       uint
	ForceRegenerate bool
	UserID         int64
	SkipLimitCheck bool
}

// GenerateImageResponse ответ на запрос генерации изображения (локальный тип для dm_tools)
type GenerateImageResponse struct {
	ImagePath string
	FileID    string
	FromCache bool
}

// GenerateImageTool позволяет DM генерировать изображения через GigaChat API
type GenerateImageTool struct {
	imageService ImageGenerationService
	chatID       int64
	userID       int64
}

// NewGenerateImageTool создает новый инструмент для генерации изображений
func NewGenerateImageTool(imageService ImageGenerationService, chatID int64, userID int64) *GenerateImageTool {
	return &GenerateImageTool{
		imageService: imageService,
		chatID:       chatID,
		userID:       userID,
	}
}

func (t *GenerateImageTool) Name() string {
	return "generate_image"
}

func (t *GenerateImageTool) Description() string {
	return `Генерирует изображение через AI по текстовому описанию. Используй этот инструмент, когда хочешь визуализировать что-то в игре:
- Локации (леса, города, подземелья)
- Персонажей (NPC, монстры, игровые персонажи)
- Предметы (оружие, магические артефакты, сокровища)
- События (магические эффекты, битвы, церемонии)
- Окружающую среду (погоду, архитектуру, природу)

Описание должно быть детальным и атмосферным, в стиле фэнтези и D&D. Изображения автоматически кэшируются для повторного использования.`
}

func (t *GenerateImageTool) Parameters() json.RawMessage {
	properties := JSONSchemaProperties{
		"description": {
			Type:        "string",
			Description: "Детальное описание того, что нужно нарисовать (на русском языке). Пример: 'Древний эльфийский лес с магическими рунами на деревьях, туман, атмосферное освещение'",
			Required:    true,
		},
		"image_type": {
			Type:        "string",
			Description: "Тип изображения: 'location' (локация), 'npc' (персонаж/NPC), 'item' (предмет), 'character' (персонаж игрока), 'event' (событие), 'environment' (окружающая среда), 'custom' (другое)",
			Required:    false,
			Enum:        []interface{}{"location", "npc", "item", "character", "event", "environment", "custom"},
		},
		"entity_id": {
			Type:        "integer",
			Description: "ID сущности (локации, NPC, предмета) для привязки изображения. Если не указан, изображение будет считаться кастомным.",
			Required:    false,
		},
		"force_regenerate": {
			Type:        "boolean",
			Description: "Принудительно регенерировать изображение, даже если оно уже есть в кэше (по умолчанию false)",
			Required:    false,
		},
	}

	required := []string{"description"}
	return BuildJSONSchema(properties, required)
}

func (t *GenerateImageTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	logger.Info("GenerateImageTool: executing",
		logger.Int64("chat_id", t.chatID),
		logger.Int64("user_id", t.userID),
	)

	// Извлекаем описание (обязательный параметр)
	description, ok := args["description"].(string)
	if !ok || description == "" {
		return nil, fmt.Errorf("description is required and must be a non-empty string")
	}

	// Извлекаем тип изображения (опционально)
	imageType := "custom"
	if it, ok := args["image_type"].(string); ok && it != "" {
		imageType = it
	}

	// Извлекаем entity_id (опционально)
	var entityID uint = 0
	if eid, ok := args["entity_id"].(float64); ok && eid > 0 {
		entityID = uint(eid)
	}

	// Извлекаем force_regenerate (опционально)
	forceRegenerate := false
	if fr, ok := args["force_regenerate"].(bool); ok {
		forceRegenerate = fr
	}

	// Формируем системный промпт в стиле фэнтези/D&D
	systemPrompt := "Ты — талантливый художник в стиле фэнтези и Dungeons & Dragons. Создавай детализированные, атмосферные и захватывающие изображения в стиле классического фэнтези-арта."
	
	// Формируем пользовательский промпт
	userPrompt := fmt.Sprintf("Нарисуй %s", description)

	logger.Info("GenerateImageTool: generating image",
		logger.String("type", imageType),
		logger.String("description", description),
		logger.Uint("entity_id", entityID),
		logger.Bool("force_regenerate", forceRegenerate),
	)

	// Генерируем изображение
	req := GenerateImageRequest{
		SystemPrompt:    systemPrompt,
		UserPrompt:      userPrompt,
		Type:            imageType,
		EntityID:        entityID,
		ForceRegenerate: forceRegenerate,
		UserID:          t.userID,
		SkipLimitCheck:  false, // TODO: Проверять Premium статус для снятия лимита
	}

	resp, err := t.imageService.GenerateImage(ctx, req)
	if err != nil {
		logger.Error("GenerateImageTool: failed to generate image",
			logger.ErrorField(err),
			logger.String("description", description),
		)
		return map[string]interface{}{
			"success":      false,
			"error":        err.Error(),
			"description":  description,
		}, nil // Возвращаем ошибку как часть результата, не прерывая выполнение
	}

	// Получаем относительный путь к изображению для возврата в результат
	// Абсолютный путь используется для отправки через Telegram
	imagePath := resp.ImagePath
	if absPath, err := filepath.Abs(imagePath); err == nil {
		imagePath = absPath
	}

	// Проверяем, существует ли файл
	fileExists := false
	if _, err := os.Stat(resp.ImagePath); err == nil {
		fileExists = true
	}

	result := map[string]interface{}{
		"success":      true,
		"description":  description,
		"image_type":   imageType,
		"image_path":   imagePath,
		"file_id":      resp.FileID,
		"from_cache":   resp.FromCache,
		"file_exists":  fileExists,
		"message":      fmt.Sprintf("Изображение успешно сгенерировано: %s", description),
	}

	if resp.FromCache {
		result["message"] = fmt.Sprintf("Изображение получено из кэша: %s", description)
	}

	logger.Info("GenerateImageTool: image generated successfully",
		logger.String("image_path", imagePath),
		logger.String("file_id", resp.FileID),
		logger.Bool("from_cache", resp.FromCache),
	)

	return result, nil
}
