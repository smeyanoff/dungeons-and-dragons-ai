package image

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// ImageStorage интерфейс для хранения изображений
type ImageStorage interface {
	// Save сохраняет изображение и возвращает путь к файлу
	Save(ctx context.Context, imageData []byte, filename string) (string, error)
	// Get получает путь к изображению по имени файла
	Get(ctx context.Context, filename string) (string, error)
	// Exists проверяет существование изображения
	Exists(ctx context.Context, filename string) bool
	// Delete удаляет изображение
	Delete(ctx context.Context, filename string) error
}

// LocalImageStorage реализация ImageStorage для локального хранения
type LocalImageStorage struct {
	basePath string
}

// NewLocalImageStorage создает новый LocalImageStorage
func NewLocalImageStorage(basePath string) (ImageStorage, error) {
	// Создаем директорию, если её нет
	// Используем 0750 вместо 0755 для более строгих прав доступа (только владелец и группа)
	if err := os.MkdirAll(basePath, 0750); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	return &LocalImageStorage{
		basePath: basePath,
	}, nil
}

// Save сохраняет изображение локально
func (s *LocalImageStorage) Save(ctx context.Context, imageData []byte, filename string) (string, error) {
	filePath := filepath.Join(s.basePath, filename)

	// Используем 0600 вместо 0644 для более строгих прав доступа (только владелец)
	if err := os.WriteFile(filePath, imageData, 0600); err != nil {
		return "", fmt.Errorf("failed to save image: %w", err)
	}

	return filePath, nil
}

// Get получает путь к изображению
func (s *LocalImageStorage) Get(ctx context.Context, filename string) (string, error) {
	filePath := filepath.Join(s.basePath, filename)

	if !s.Exists(ctx, filename) {
		return "", fmt.Errorf("image not found: %s", filename)
	}

	return filePath, nil
}

// Exists проверяет существование изображения
func (s *LocalImageStorage) Exists(ctx context.Context, filename string) bool {
	filePath := filepath.Join(s.basePath, filename)
	_, err := os.Stat(filePath)
	return err == nil
}

// Delete удаляет изображение
func (s *LocalImageStorage) Delete(ctx context.Context, filename string) error {
	filePath := filepath.Join(s.basePath, filename)

	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete image: %w", err)
	}

	return nil
}
