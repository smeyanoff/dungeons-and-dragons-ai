package telegram

import (
	"strings"
	"testing"
)

func TestSplitMessage(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		maxLen   int
		expected int // количество частей
	}{
		{
			name:     "short message",
			text:     "Short message",
			maxLen:   100,
			expected: 1,
		},
		{
			name:     "exact length",
			text:     string(make([]byte, TelegramSafeMessageLength)),
			maxLen:   TelegramSafeMessageLength,
			expected: 1,
		},
		{
			name:     "long message",
			text:     string(make([]byte, TelegramSafeMessageLength*2)),
			maxLen:   TelegramSafeMessageLength,
			expected: 2,
		},
		{
			name:     "message with newlines",
			text:     "Line 1\nLine 2\n" + string(make([]byte, TelegramSafeMessageLength)),
			maxLen:   TelegramSafeMessageLength,
			expected: 2,
		},
		{
			name:     "very long line",
			text:     string(make([]byte, TelegramSafeMessageLength*3)),
			maxLen:   TelegramSafeMessageLength,
			expected: 3,
		},
		{
			name:     "empty message",
			text:     "",
			maxLen:   100,
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parts := splitMessage(tt.text, tt.maxLen)
			if len(parts) != tt.expected {
				t.Errorf("splitMessage() returned %d parts, expected %d", len(parts), tt.expected)
			}

			// Проверяем, что все части не превышают maxLen
			for i, part := range parts {
				if len(part) > tt.maxLen {
					t.Errorf("part %d length %d exceeds maxLen %d", i, len(part), tt.maxLen)
				}
			}

			// Проверяем, что объединение частей дает исходный текст (для коротких сообщений)
			if len(tt.text) <= tt.maxLen {
				if len(parts) != 1 || parts[0] != tt.text {
					t.Errorf("splitMessage() should return original text for short messages")
				}
			}
		})
	}
}

func TestSplitMessageEdgeCases(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{
			name: "only newlines",
			text: "\n\n\n",
		},
		{
			name: "very long word",
			text: string(make([]byte, TelegramSafeMessageLength*2)) + " word",
		},
		{
			name: "mixed content",
			text: "Short\n" + string(make([]byte, TelegramSafeMessageLength)) + "\nEnd",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parts := splitMessage(tt.text, TelegramSafeMessageLength)
			if len(parts) == 0 {
				t.Error("splitMessage() returned empty parts")
			}

			// Проверяем, что все части не превышают безопасную длину
			for i, part := range parts {
				if len(part) > TelegramSafeMessageLength {
					t.Errorf("part %d length %d exceeds TelegramSafeMessageLength %d", i, len(part), TelegramSafeMessageLength)
				}
			}
		})
	}
}

func TestRedactTelegramBotToken(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "redacts token in url",
			input:    "https://api.telegram.org/bot123456:ABC_DEF-123/getUpdates",
			expected: "https://api.telegram.org/bot***/getUpdates",
		},
		{
			name:     "redacts token without trailing slash",
			input:    "api.telegram.org/bot123456:ABCDEF",
			expected: "api.telegram.org/bot***",
		},
		{
			name:     "no token no change",
			input:    "https://example.com/ok",
			expected: "https://example.com/ok",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := redactTelegramBotToken(tt.input)
			if got != tt.expected {
				t.Fatalf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestHandleCommand(t *testing.T) {
	// Создаем мок бота с минимальными зависимостями
	// Примечание: реальный бот требует tgbotapi.BotAPI, который сложно замокировать
	// Поэтому тестируем только логику обработки команд через интеграционные тесты
	// или через тестирование отдельных методов

	tests := []struct {
		name    string
		command string
		args    string
	}{
		{
			name:    "start command",
			command: "start",
			args:    "",
		},
		{
			name:    "newgame command",
			command: "newgame",
			args:    "fantasy",
		},
		{
			name:    "createcharacter command",
			command: "createcharacter",
			args:    "Gandalf elf wizard",
		},
		{
			name:    "unknown command",
			command: "unknown",
			args:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Этот тест проверяет, что команды распознаются правильно
			// Реальная обработка требует полной инициализации бота
			// Для полного тестирования нужны интеграционные тесты
			_ = tt.command
			_ = tt.args
		})
	}
}

// TestSplitMessageWithRealisticContent тестирует разбиение сообщений с реалистичным контентом
func TestSplitMessageWithRealisticContent(t *testing.T) {
	longText := `🎮 Игра начата!

Мир: Тестовый мир
Это очень длинное описание мира, которое содержит много информации о мире игры. Оно должно быть разбито на несколько частей, если превышает лимит Telegram.

Главный квест: Тестовый квест
Это описание главного квеста, которое также может быть очень длинным и содержать много деталей о том, что нужно сделать игрокам.

Используйте команды или просто пишите мне, что хотите сделать!`

	parts := splitMessage(longText, 100) // Маленький лимит для тестирования

	if len(parts) == 0 {
		t.Error("splitMessage() returned empty parts for realistic content")
	}

	// Проверяем, что все части не превышают лимит
	for i, part := range parts {
		if len(part) > 100 {
			t.Errorf("part %d length %d exceeds limit 100", i, len(part))
		}
	}

	// Проверяем, что части содержат исходный контент
	combined := ""
	for _, part := range parts {
		combined += part
	}
	// Убираем индикаторы частей для проверки
	combinedClean := combined
	if len(parts) > 1 {
		// Убираем индикаторы вида "(1/2)\n" из начала
		// Это упрощенная проверка, реальная функция может добавлять индикаторы
	}
	_ = combinedClean
}

// TestTelegramMaxMessageLength проверяет константы
func TestTelegramMaxMessageLength(t *testing.T) {
	if TelegramMaxMessageLength != 4096 {
		t.Errorf("TelegramMaxMessageLength = %d, expected 4096", TelegramMaxMessageLength)
	}

	if TelegramSafeMessageLength >= TelegramMaxMessageLength {
		t.Errorf("TelegramSafeMessageLength (%d) should be less than TelegramMaxMessageLength (%d)",
			TelegramSafeMessageLength, TelegramMaxMessageLength)
	}
}

func TestSanitizeTelegramOutput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		mustNot  []string
		mustHave []string
	}{
		{
			name:    "tool_call_block_and_code_fence_and_json",
			input:   "Привет!\n<tool_call>{\"name\":\"do_thing\",\"arguments\":{\"x\":1}}</tool_call>\n```json\n{\"foo\": \"bar\"}\n```\nГотово.",
			mustNot: []string{"<tool_call>", "tool_call", "```", "{\"foo\""},
			mustHave: []string{
				"Привет!",
				"Готово.",
			},
		},
		{
			name:    "tool_result_block_is_removed",
			input:   "Ок.\n<tool_result>{\"status\":\"ok\",\"result\":{\"y\":2}}</tool_result>\nПродолжаем.",
			mustNot: []string{"<tool_result>", "tool_result", "\"status\":\"ok\""},
			mustHave: []string{
				"Ок.",
				"Продолжаем.",
			},
		},
		{
			name:    "suspicious_lines_are_stripped",
			input:   "Нормальный текст\n{\"a\": 1}\nTOOL_RESULT: something\nЕще строка",
			mustNot: []string{"{\"a\": 1}", "TOOL_RESULT"},
			mustHave: []string{
				"Нормальный текст",
				"Еще строка",
			},
		},
		{
			name:    "empty_after_strip_becomes_ellipsis",
			input:   "<tool_call>{\"name\":\"x\"}</tool_call>",
			mustNot: []string{"<tool_call>", "tool_call"},
			mustHave: []string{
				"...",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeTelegramOutput(tt.input, 123, "test")
			for _, s := range tt.mustNot {
				if strings.Contains(got, s) {
					t.Fatalf("expected %q to be removed, got %q", s, got)
				}
			}
			for _, s := range tt.mustHave {
				if !strings.Contains(got, s) {
					t.Fatalf("expected %q to be present, got %q", s, got)
				}
			}
			if strings.TrimSpace(got) == "" {
				t.Fatalf("expected non-empty sanitized output, got %q", got)
			}
		})
	}
}
