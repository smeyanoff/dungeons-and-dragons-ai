package infrastructure

import (
	"context"
	"fmt"

	"dungeons-and-dragons-ai/pkg/gigachat"
)

// ImageGenerator интерфейс для генерации изображений
type ImageGenerator interface {
	// GenerateImage генерирует изображение и возвращает file_id
	GenerateImage(ctx context.Context, systemPrompt string, userPrompt string) (string, error)
	// DownloadImage скачивает изображение по file_id и возвращает бинарные данные
	DownloadImage(ctx context.Context, fileID string) ([]byte, error)
}

// GigachatImageGenerator реализация ImageGenerator через GigaChat API
type GigachatImageGenerator struct {
	client *gigachat.Client
	model  string
}

// NewGigachatImageGenerator создает новый GigachatImageGenerator
func NewGigachatImageGenerator(client *gigachat.Client, model string) ImageGenerator {
	return &GigachatImageGenerator{
		client: client,
		model:  model,
	}
}

// GenerateImage генерирует изображение через GigaChat API
func (g *GigachatImageGenerator) GenerateImage(ctx context.Context, systemPrompt string, userPrompt string) (string, error) {
	fileID, err := g.client.GenerateImage(ctx, g.model, systemPrompt, userPrompt)
	if err != nil {
		return "", fmt.Errorf("failed to generate image: %w", err)
	}
	return fileID, nil
}

// DownloadImage скачивает изображение по file_id
func (g *GigachatImageGenerator) DownloadImage(ctx context.Context, fileID string) ([]byte, error) {
	imageData, err := g.client.DownloadImage(ctx, fileID)
	if err != nil {
		return nil, fmt.Errorf("failed to download image: %w", err)
	}
	return imageData, nil
}
