package dm_tools

import (
	"context"
	"errors"
	"testing"
)

// mockImageGenerationService мок для ImageGenerationService
type mockImageGenerationService struct {
	generateImageFunc func(ctx context.Context, req GenerateImageRequest) (*GenerateImageResponse, error)
}

func (m *mockImageGenerationService) GenerateImage(ctx context.Context, req GenerateImageRequest) (*GenerateImageResponse, error) {
	if m.generateImageFunc != nil {
		return m.generateImageFunc(ctx, req)
	}
	return &GenerateImageResponse{
		ImagePath: "/test/image.png",
		FileID:    "test_file_id",
		FromCache: false,
	}, nil
}

// mockSubscriptionChecker мок для SubscriptionChecker
type mockSubscriptionChecker struct {
	isPremiumFunc func(ctx context.Context, tgUserID int64) (bool, error)
}

func (m *mockSubscriptionChecker) IsPremium(ctx context.Context, tgUserID int64) (bool, error) {
	if m.isPremiumFunc != nil {
		return m.isPremiumFunc(ctx, tgUserID)
	}
	return false, nil
}

// TestGenerateImageTool_PremiumUserSkipsLimit проверяет, что Premium пользователи пропускают проверку лимита (CODE-6)
func TestGenerateImageTool_PremiumUserSkipsLimit(t *testing.T) {
	ctx := context.Background()
	chatID := int64(123)
	userID := int64(456)

	var receivedRequest GenerateImageRequest
	var skipLimitCheckValue bool

	mockImageService := &mockImageGenerationService{
		generateImageFunc: func(ctx context.Context, req GenerateImageRequest) (*GenerateImageResponse, error) {
			receivedRequest = req
			skipLimitCheckValue = req.SkipLimitCheck
			return &GenerateImageResponse{
				ImagePath: "/test/image.png",
				FileID:    "test_file_id",
				FromCache: false,
			}, nil
		},
	}

	mockSubscriptionChecker := &mockSubscriptionChecker{
		isPremiumFunc: func(ctx context.Context, tgUserID int64) (bool, error) {
			if tgUserID == userID {
				return true, nil // Premium пользователь
			}
			return false, nil
		},
	}

	tool := NewGenerateImageTool(mockImageService, chatID, userID, mockSubscriptionChecker)

	args := map[string]interface{}{
		"description": "Тестовое изображение",
	}

	result, err := tool.Execute(ctx, args)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if result == nil {
		t.Fatal("Execute() result = nil, want non-nil")
	}

	// Проверяем, что SkipLimitCheck установлен в true для Premium пользователя
	if !skipLimitCheckValue {
		t.Error("Execute() SkipLimitCheck = false, want true for Premium user")
	}

	// Проверяем, что userID передан корректно
	if receivedRequest.UserID != userID {
		t.Errorf("Execute() UserID = %d, want %d", receivedRequest.UserID, userID)
	}
}

// TestGenerateImageTool_FreeUserHasLimit проверяет, что Free пользователи имеют проверку лимита (CODE-6)
func TestGenerateImageTool_FreeUserHasLimit(t *testing.T) {
	ctx := context.Background()
	chatID := int64(123)
	userID := int64(456)

	var receivedRequest GenerateImageRequest
	var skipLimitCheckValue bool

	mockImageService := &mockImageGenerationService{
		generateImageFunc: func(ctx context.Context, req GenerateImageRequest) (*GenerateImageResponse, error) {
			receivedRequest = req
			skipLimitCheckValue = req.SkipLimitCheck
			return &GenerateImageResponse{
				ImagePath: "/test/image.png",
				FileID:    "test_file_id",
				FromCache: false,
			}, nil
		},
	}

	mockSubscriptionChecker := &mockSubscriptionChecker{
		isPremiumFunc: func(ctx context.Context, tgUserID int64) (bool, error) {
			return false, nil // Free пользователь
		},
	}

	tool := NewGenerateImageTool(mockImageService, chatID, userID, mockSubscriptionChecker)

	args := map[string]interface{}{
		"description": "Тестовое изображение",
	}

	result, err := tool.Execute(ctx, args)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if result == nil {
		t.Fatal("Execute() result = nil, want non-nil")
	}

	// Проверяем, что SkipLimitCheck установлен в false для Free пользователя
	if skipLimitCheckValue {
		t.Error("Execute() SkipLimitCheck = true, want false for Free user")
	}

	// Проверяем, что userID передан корректно
	if receivedRequest.UserID != userID {
		t.Errorf("Execute() UserID = %d, want %d", receivedRequest.UserID, userID)
	}
}

// TestGenerateImageTool_SubscriptionCheckError проверяет обработку ошибки при проверке подписки (CODE-6)
func TestGenerateImageTool_SubscriptionCheckError(t *testing.T) {
	ctx := context.Background()
	chatID := int64(123)
	userID := int64(456)

	var receivedRequest GenerateImageRequest
	var skipLimitCheckValue bool

	mockImageService := &mockImageGenerationService{
		generateImageFunc: func(ctx context.Context, req GenerateImageRequest) (*GenerateImageResponse, error) {
			receivedRequest = req
			skipLimitCheckValue = req.SkipLimitCheck
			return &GenerateImageResponse{
				ImagePath: "/test/image.png",
				FileID:    "test_file_id",
				FromCache: false,
			}, nil
		},
	}

	mockSubscriptionChecker := &mockSubscriptionChecker{
		isPremiumFunc: func(ctx context.Context, tgUserID int64) (bool, error) {
			return false, errors.New("database error") // Ошибка при проверке
		},
	}

	tool := NewGenerateImageTool(mockImageService, chatID, userID, mockSubscriptionChecker)

	args := map[string]interface{}{
		"description": "Тестовое изображение",
	}

	result, err := tool.Execute(ctx, args)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil (should continue with limit check on error)", err)
	}

	if result == nil {
		t.Fatal("Execute() result = nil, want non-nil")
	}

	// При ошибке проверки подписки должен использоваться лимит (SkipLimitCheck = false)
	if skipLimitCheckValue {
		t.Error("Execute() SkipLimitCheck = true, want false when subscription check fails")
	}

	// Проверяем, что userID передан корректно
	if receivedRequest.UserID != userID {
		t.Errorf("Execute() UserID = %d, want %d", receivedRequest.UserID, userID)
	}
}

// TestGenerateImageTool_NoSubscriptionChecker проверяет работу без SubscriptionChecker (CODE-6)
func TestGenerateImageTool_NoSubscriptionChecker(t *testing.T) {
	ctx := context.Background()
	chatID := int64(123)
	userID := int64(456)

	var receivedRequest GenerateImageRequest
	var skipLimitCheckValue bool

	mockImageService := &mockImageGenerationService{
		generateImageFunc: func(ctx context.Context, req GenerateImageRequest) (*GenerateImageResponse, error) {
			receivedRequest = req
			skipLimitCheckValue = req.SkipLimitCheck
			return &GenerateImageResponse{
				ImagePath: "/test/image.png",
				FileID:    "test_file_id",
				FromCache: false,
			}, nil
		},
	}

	// Создаем tool без SubscriptionChecker (nil)
	tool := NewGenerateImageTool(mockImageService, chatID, userID, nil)

	args := map[string]interface{}{
		"description": "Тестовое изображение",
	}

	result, err := tool.Execute(ctx, args)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if result == nil {
		t.Fatal("Execute() result = nil, want non-nil")
	}

	// Без SubscriptionChecker должен использоваться лимит (SkipLimitCheck = false)
	if skipLimitCheckValue {
		t.Error("Execute() SkipLimitCheck = true, want false when no subscription checker")
	}

	// Проверяем, что userID передан корректно
	if receivedRequest.UserID != userID {
		t.Errorf("Execute() UserID = %d, want %d", receivedRequest.UserID, userID)
	}
}
