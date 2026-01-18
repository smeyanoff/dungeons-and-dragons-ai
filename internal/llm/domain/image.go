package domain

import (
	"context"
)

// ImageGenerator интерфейс для генерации изображений через LLM API
type ImageGenerator interface {
	// GenerateImage генерирует изображение и возвращает file_id
	GenerateImage(ctx context.Context, systemPrompt string, userPrompt string) (string, error)
	// DownloadImage скачивает изображение по file_id и возвращает бинарные данные
	DownloadImage(ctx context.Context, fileID string) ([]byte, error)
}
