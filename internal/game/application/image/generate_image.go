package image

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
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
	SystemPrompt   string // Системный промпт (стиль художника)
	UserPrompt     string // Пользовательский промпт (что нарисовать)
	Type           string // Тип изображения: "location", "npc", "item", "character", "custom"
	EntityID       uint   // ID сущности (локации, NPC, предмета)
	ForceRegenerate bool  // Принудительная регенерация (игнорировать кэш)
	UserID         int64  // ID пользователя для проверки лимитов
	SkipLimitCheck bool   // Пропустить проверку лимита (для Premium пользователей)
}

// GenerateImageResponse ответ на запрос генерации изображения
type GenerateImageResponse struct {
	ImagePath string // Путь к сохраненному изображению
	FileID    string // File ID из GigaChat API (для кэширования в API)
	FromCache bool   // Было ли изображение взято из кэша
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

	// Генерируем имя файла на основе типа и ID сущности (или хэша промпта)
	var filename string
	if req.EntityID > 0 {
		filename = fmt.Sprintf("%s_%d.jpg", req.Type, req.EntityID)
	} else {
		// Для кастомных изображений используем хэш промпта
		hash := md5.Sum([]byte(req.UserPrompt))
		filename = fmt.Sprintf("%s_%s_%s.jpg", req.Type, hex.EncodeToString(hash[:8]), time.Now().Format("20060102"))
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
					ImagePath: imagePath,
					FromCache: true,
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

	// Скачиваем изображение
	imageData, err := uc.imageGenerator.DownloadImage(ctx, fileID)
	if err != nil {
		return nil, fmt.Errorf("failed to download image: %w", err)
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
		ImagePath: imagePath,
		FileID:    fileID,
		FromCache: false,
	}, nil
}

// GenerateFilename генерирует имя файла для изображения
func GenerateFilename(imageType string, entityID uint) string {
	if entityID > 0 {
		return fmt.Sprintf("%s_%d.jpg", imageType, entityID)
	}
	return fmt.Sprintf("%s_%d.jpg", imageType, time.Now().Unix())
}
