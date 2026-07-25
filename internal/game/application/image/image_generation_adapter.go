package image

import (
	"context"

	"dungeons-and-dragons-ai/internal/game/application/dm_tools"
)

// ImageGenerationServiceAdapter адаптирует ImageGenerationUseCase к dm_tools.ImageGenerationService
type ImageGenerationServiceAdapter struct {
	uc *ImageGenerationUseCase
}

// NewImageGenerationServiceAdapter создает новый адаптер
func NewImageGenerationServiceAdapter(uc *ImageGenerationUseCase) dm_tools.ImageGenerationService {
	return &ImageGenerationServiceAdapter{
		uc: uc,
	}
}

// GenerateImage генерирует изображение, адаптируя запрос из dm_tools к внутреннему формату
func (a *ImageGenerationServiceAdapter) GenerateImage(ctx context.Context, req dm_tools.GenerateImageRequest) (*dm_tools.GenerateImageResponse, error) {
	// Преобразуем запрос из dm_tools в внутренний формат
	internalReq := GenerateImageRequest{
		SystemPrompt:    req.SystemPrompt,
		UserPrompt:      req.UserPrompt,
		Type:            req.Type,
		EntityID:        req.EntityID,
		ForceRegenerate: req.ForceRegenerate,
		UserID:          req.UserID,
		SkipLimitCheck:  req.SkipLimitCheck,
	}

	// Выполняем генерацию
	resp, err := a.uc.Execute(ctx, internalReq)
	if err != nil {
		return nil, err
	}

	// Преобразуем ответ из внутреннего формата в формат dm_tools
	return &dm_tools.GenerateImageResponse{
		ImagePath: resp.ImagePath,
		FileID:    resp.FileID,
		FromCache: resp.FromCache,
	}, nil
}
