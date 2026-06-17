package logger

import (
	"errors"
	"testing"
)

func TestSanitizeErrorRedactsTelegramToken(t *testing.T) {
	inputErr := errors.New("request failed: https://api.telegram.org/bot123456:ABC_DEF-123/getUpdates")
	sanitized := sanitizeError(inputErr)
	if sanitized == nil {
		t.Fatal("expected sanitized error, got nil")
	}

	expected := "request failed: https://api.telegram.org/bot***/getUpdates"
	if sanitized.Error() != expected {
		t.Fatalf("expected %q, got %q", expected, sanitized.Error())
	}
}

func TestSanitizeErrorNoChange(t *testing.T) {
	inputErr := errors.New("regular error")
	sanitized := sanitizeError(inputErr)
	if sanitized.Error() != inputErr.Error() {
		t.Fatalf("expected %q, got %q", inputErr.Error(), sanitized.Error())
	}
}
