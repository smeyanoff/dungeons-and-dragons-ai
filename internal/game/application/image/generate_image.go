package image

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"dungeons-and-dragons-ai/internal/llm/domain"
	"dungeons-and-dragons-ai/pkg/logger"
)

// ImageGenerationUseCase use case для генерации изображений
type ImageGenerationUseCase struct {
	imageGenerator domain.ImageGenerator
	storage        ImageStorage
	limiter        ImageGenerationLimiter // Опциональный лимитер
}

// NewImageGenerationUseCase создает новый ImageGenerationUseCase
func NewImageGenerationUseCase(
	imageGenerator domain.ImageGenerator,
	storage ImageStorage,
) *ImageGenerationUseCase {
	return &ImageGenerationUseCase{
		imageGenerator: imageGenerator,
		storage:        storage,
	}
}

// SetLimiter устанавливает лимитер для генерации изображений
func (uc *ImageGenerationUseCase) SetLimiter(limiter ImageGenerationLimiter) {
	uc.limiter = limiter
}

// GenerateImageRequest запрос на генерацию изображения
type GenerateImageRequest struct {
	SystemPrompt    string // Системный промпт (стиль художника)
	UserPrompt      string // Пользовательский промпт (что нарисовать)
	Type            string // Тип изображения: "location", "npc", "item", "character", "custom"
	EntityID        uint   // ID сущности (локации, NPC, предмета)
	EntityName      string // Уникальное имя сущности для кэширования (используется когда EntityID = 0)
	ForceRegenerate bool   // Принудительная регенерация (игнорировать кэш)
	UserID          int64  // ID пользователя для проверки лимитов
	SkipLimitCheck  bool   // Пропустить проверку лимита (для Premium пользователей)
}

// GenerateImageResponse ответ на запрос генерации изображения
type GenerateImageResponse struct {
	ImagePath  string // Путь к сохраненному изображению (пустой если не удалось скачать)
	FileID     string // File ID из GigaChat API (всегда возвращается)
	FromCache  bool   // Было ли изображение взято из кэша
	Downloaded bool   // Удалось ли скачать и сохранить файл
}

// Execute генерирует изображение, скачивает его и сохраняет локально
func (uc *ImageGenerationUseCase) Execute(ctx context.Context, req GenerateImageRequest) (*GenerateImageResponse, error) {
	// Проверяем лимит генерации (если требуется и лимитер настроен)
	if !req.SkipLimitCheck && uc.limiter != nil && req.UserID > 0 {
		canGenerate, err := uc.limiter.CheckLimit(ctx, req.UserID)
		if err != nil {
			return nil, fmt.Errorf("failed to check limit: %w", err)
		}
		if !canGenerate {
			remaining, _ := uc.limiter.GetRemainingQuota(ctx, req.UserID)
			return nil, fmt.Errorf("daily image generation limit reached (remaining: %d)", remaining)
		}
	}

	// Генерируем имя файла на основе типа и уникального идентификатора сущности
	var filename string
	if req.EntityID > 0 {
		// Используем EntityID если он доступен
		filename = fmt.Sprintf("%s_%d.jpg", req.Type, req.EntityID)
	} else if req.EntityName != "" {
		// Используем EntityName для создания уникального идентификатора
		// Нормализуем имя (убираем пробелы, приводим к нижнему регистру)
		normalizedName := strings.ToLower(strings.ReplaceAll(req.EntityName, " ", "_"))
		normalizedName = strings.ReplaceAll(normalizedName, "'", "")
		normalizedName = strings.ReplaceAll(normalizedName, "\"", "")
		// Ограничиваем длину имени файла (максимум 100 символов)
		if len(normalizedName) > 100 {
			hash := sha256.Sum256([]byte(normalizedName))
			normalizedName = hex.EncodeToString(hash[:8]) // 16 символов
		}
		filename = fmt.Sprintf("%s_%s.jpg", req.Type, normalizedName)
	} else {
		// Для полностью кастомных изображений используем SHA256 хэш промпта
		hash := sha256.Sum256([]byte(req.UserPrompt))
		filename = fmt.Sprintf("%s_%s.jpg", req.Type, hex.EncodeToString(hash[:16]))
	}

	// Проверяем кэш, если не требуется принудительная регенерация
	if !req.ForceRegenerate {
		if uc.storage.Exists(ctx, filename) {
			imagePath, err := uc.storage.Get(ctx, filename)
			if err == nil {
				logger.Info("Image retrieved from cache",
					logger.String("filename", filename),
					logger.String("type", req.Type),
				)
				return &GenerateImageResponse{
					ImagePath:  imagePath,
					FileID:     "", // Кэшированные изображения не имеют file_id
					FromCache:  true,
					Downloaded: true,
				}, nil
			}
		}
	}

	// Генерируем изображение через GigaChat API
	logger.Info("Generating image via GigaChat API",
		logger.String("type", req.Type),
		logger.String("prompt", req.UserPrompt),
	)

	fileID, err := uc.imageGenerator.GenerateImage(ctx, req.SystemPrompt, req.UserPrompt)
	if err != nil {
		return nil, fmt.Errorf("failed to generate image: %w", err)
	}

	logger.Info("Image generated, downloading",
		logger.String("file_id", fileID),
		logger.String("type", req.Type),
	)

	// Скачиваем изображение с отдельным таймаутом (изображения могут быть большими)
	downloadCtx, downloadCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer downloadCancel()

	logger.Info("Downloading image file",
		logger.String("file_id", fileID))

	imageData, err := uc.imageGenerator.DownloadImage(downloadCtx, fileID)
	if err != nil {
		logger.Warn("Failed to download image, but file_id is valid",
			logger.String("file_id", fileID),
			logger.ErrorField(err))
		// Возвращаем результат с file_id, но без данных файла
		// Это позволит использовать file_id для повторных попыток скачивания
		return &GenerateImageResponse{
			ImagePath:  "",
			FileID:     fileID,
			FromCache:  false,
			Downloaded: false,
		}, nil
	}

	logger.Info("Image downloaded, saving",
		logger.String("file_id", fileID),
		logger.Int("size_bytes", len(imageData)),
	)

	// Сохраняем изображение локально
	imagePath, err := uc.storage.Save(ctx, imageData, filename)
	if err != nil {
		return nil, fmt.Errorf("failed to save image: %w", err)
	}

	logger.Info("Image saved successfully",
		logger.String("path", imagePath),
		logger.String("filename", filename),
	)

	// Записываем факт генерации в лимитер (если настроен)
	if !req.SkipLimitCheck && uc.limiter != nil && req.UserID > 0 {
		if err := uc.limiter.RecordGeneration(ctx, req.UserID); err != nil {
			logger.Warn("Failed to record image generation",
				logger.ErrorField(err),
				logger.Int64("user_id", req.UserID),
			)
		}
	}

	return &GenerateImageResponse{
		ImagePath:  imagePath,
		FileID:     fileID,
		FromCache:  false,
		Downloaded: true,
	}, nil
}

// GenerateFilename генерирует имя файла для изображения
func GenerateFilename(imageType string, entityID uint) string {
	if entityID > 0 {
		return fmt.Sprintf("%s_%d.jpg", imageType, entityID)
	}
	return fmt.Sprintf("%s_%d.jpg", imageType, time.Now().Unix())
}
