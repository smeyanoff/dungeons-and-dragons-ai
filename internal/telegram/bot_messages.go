package telegram

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"dungeons-and-dragons-ai/pkg/logger"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// sendMessage отправляет сообщение с проверкой ошибок, логированием и retry механизмом
func (b *Bot) sendMessage(msg tgbotapi.MessageConfig) error {
	return b.sendMessageWithRetry(context.Background(), msg, 0)
}

// sendMessageWithRetry отправляет сообщение с retry механизмом и exponential backoff
func (b *Bot) sendMessageWithRetry(ctx context.Context, msg tgbotapi.MessageConfig, attempt int) error {
	const maxRetries = 3
	const initialBackoff = 100 * time.Millisecond
	const maxBackoff = 5 * time.Second

	msg.Text = sanitizeTelegramOutput(msg.Text, msg.ChatID, "sendMessage")

	b.circuitOpenMu.RLock()
	circuitOpen := b.circuitOpen
	b.circuitOpenMu.RUnlock()

	if circuitOpen {
		b.errorCountMu.Lock()
		timeSinceLastError := time.Since(b.lastErrorTime)
		if timeSinceLastError > 30*time.Second {
			b.circuitOpenMu.Lock()
			b.circuitOpen = false
			b.circuitOpenMu.Unlock()
			b.errorCount = 0
		}
		b.errorCountMu.Unlock()

		if b.circuitOpen {
			return fmt.Errorf("circuit breaker is open, too many errors")
		}
	}

	_, err := b.api.Send(msg)
	if err == nil {
		b.errorCountMu.Lock()
		b.errorCount = 0
		b.errorCountMu.Unlock()
		b.circuitOpenMu.Lock()
		b.circuitOpen = false
		b.circuitOpenMu.Unlock()
		return nil
	}

	b.errorCountMu.Lock()
	b.errorCount++
	b.lastErrorTime = time.Now()
	errorCount := b.errorCount
	b.errorCountMu.Unlock()

	if errorCount >= 10 {
		b.circuitOpenMu.Lock()
		b.circuitOpen = true
		b.circuitOpenMu.Unlock()
		logger.Warn("Telegram API circuit breaker opened due to too many errors",
			logger.Int("consecutive_errors", errorCount),
		)
	}

	errStr := err.Error()
	shouldRetry := false
	if strings.Contains(errStr, "unexpected EOF") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "connection") ||
		strings.Contains(errStr, "network") {
		shouldRetry = true
	}

	if !shouldRetry || attempt >= maxRetries {
		if attempt >= maxRetries {
			logger.Warn("Failed to send message after max retries",
				logger.ErrorField(sanitizeTelegramError(err)),
				logger.Int64("chat_id", msg.ChatID),
				logger.Int("attempts", attempt+1),
			)
		}
		return err
	}

	const maxSafeShift = 30
	safeAttempt := attempt
	if safeAttempt < 0 {
		safeAttempt = 0
	} else if safeAttempt > maxSafeShift {
		safeAttempt = maxSafeShift
	}
	// #nosec G115 - safeAttempt ограничен до maxSafeShift=30 (безопасно для int64/time.Duration)
	backoff := initialBackoff * time.Duration(1<<uint(safeAttempt))
	if backoff > maxBackoff {
		backoff = maxBackoff
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(backoff):
	}

	return b.sendMessageWithRetry(ctx, msg, attempt+1)
}

// sanitizeUTF8 очищает строку от невалидных UTF-8 последовательностей.
func sanitizeUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}

	var result strings.Builder
	result.Grow(len(s))

	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		if r == utf8.RuneError && size == 1 {
			result.WriteRune('\uFFFD')
			s = s[1:]
		} else {
			result.WriteRune(r)
			s = s[size:]
		}
	}

	return result.String()
}

var (
	toolCallBlockRe   = regexp.MustCompile(`(?s)<tool_call>.*?</tool_call>`)
	toolResultBlockRe = regexp.MustCompile(`(?s)<tool_result>.*?</tool_result>`)
	codeFenceRe       = regexp.MustCompile("(?s)```.*?```")
	jsonObjectRe      = regexp.MustCompile(`(?s)\{[^{}]*\"[^\"]+\"\s*:\s*[^{}]*\}`)
)

func sanitizeTelegramOutput(text string, chatID int64, source string) string {
	original := sanitizeUTF8(text)
	sanitized := original
	leakDetected := false

	var (
		strippedToolCall   bool
		strippedToolResult bool
		strippedCodeFence  bool
		strippedJSON       bool
		strippedLines      bool
	)

	var changed bool
	sanitized, changed = replaceAndFlag(toolCallBlockRe, sanitized)
	strippedToolCall = changed
	leakDetected = leakDetected || changed
	sanitized, changed = replaceAndFlag(toolResultBlockRe, sanitized)
	strippedToolResult = changed
	leakDetected = leakDetected || changed
	sanitized, changed = replaceAndFlag(codeFenceRe, sanitized)
	strippedCodeFence = changed
	leakDetected = leakDetected || changed
	sanitized, changed = replaceAndFlag(jsonObjectRe, sanitized)
	strippedJSON = changed
	leakDetected = leakDetected || changed

	sanitized = strings.ReplaceAll(sanitized, "```", "")

	sanitized, changed = stripSuspiciousLines(sanitized)
	strippedLines = changed
	leakDetected = leakDetected || changed

	sanitized = collapseBlankLines(sanitized)
	if sanitized == "" {
		sanitized = "..."
	}

	if leakDetected {
		logger.Warn("Output guard stripped internal artifacts",
			logger.Int64("chat_id", chatID),
			logger.String("source", source),
			logger.Bool("stripped_tool_call", strippedToolCall),
			logger.Bool("stripped_tool_result", strippedToolResult),
			logger.Bool("stripped_code_fence", strippedCodeFence),
			logger.Bool("stripped_json", strippedJSON),
			logger.Bool("stripped_suspicious_lines", strippedLines),
			logger.Int("original_len", len(original)),
			logger.Int("sanitized_len", len(sanitized)),
		)
	}

	return sanitized
}

func replaceAndFlag(re *regexp.Regexp, input string) (string, bool) {
	if !re.MatchString(input) {
		return input, false
	}
	return re.ReplaceAllString(input, ""), true
}

func stripSuspiciousLines(text string) (string, bool) {
	lines := strings.Split(text, "\n")
	kept := make([]string, 0, len(lines))
	removed := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		if strings.Contains(lower, "tool_call") ||
			strings.Contains(lower, "tool_result") ||
			strings.Contains(lower, "function_call") ||
			strings.Contains(trimmed, "<tool_call>") ||
			strings.Contains(trimmed, "</tool_call>") ||
			strings.Contains(trimmed, "<tool_result>") ||
			strings.Contains(trimmed, "</tool_result>") ||
			looksLikeJSONLine(trimmed) {
			removed = true
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n"), removed
}

func looksLikeJSONLine(trimmed string) bool {
	if trimmed == "" {
		return false
	}
	if !(strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") ||
		strings.HasSuffix(trimmed, "}") || strings.HasSuffix(trimmed, "]")) {
		return false
	}
	return strings.Contains(trimmed, `":`)
}

func collapseBlankLines(text string) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	blank := false
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			if blank {
				continue
			}
			blank = true
			out = append(out, "")
			continue
		}
		blank = false
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// sanitizeTelegramError редактирует потенциальные секреты (Telegram Bot Token) из ошибок.
func sanitizeTelegramError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	redacted := redactTelegramBotToken(msg)
	if redacted == msg {
		return err
	}
	return fmt.Errorf("%s", redacted)
}

// redactTelegramBotToken заменяет `...api.telegram.org/bot<token>/...` на `...api.telegram.org/bot***/...`.
func redactTelegramBotToken(s string) string {
	const marker = "api.telegram.org/bot"
	idx := strings.Index(s, marker)
	if idx == -1 {
		return s
	}
	start := idx + len(marker)
	end := strings.Index(s[start:], "/")
	if end == -1 {
		return s[:start] + "***"
	}
	end = start + end
	return s[:start] + "***" + s[end:]
}

// sendPhoto отправляет фото через Telegram Bot API.
// Важно: caption также проходит через output guard (если задан).
func (b *Bot) sendPhoto(ctx context.Context, chatID int64, photoPath string, caption string) error {
	_ = ctx // оставляем ctx для единообразия сигнатур и будущего timeout/cancel

	photo := tgbotapi.NewPhoto(chatID, tgbotapi.FilePath(photoPath))
	if caption != "" {
		photo.Caption = sanitizeTelegramOutput(caption, chatID, "sendPhoto")
	}

	_, err := b.api.Send(photo)
	if err != nil {
		return fmt.Errorf("failed to send photo: %w", err)
	}

	return nil
}

// sendLongMessage разбивает длинные сообщения на части и отправляет их.
func (b *Bot) sendLongMessage(chatID int64, text string) error {
	text = sanitizeTelegramOutput(text, chatID, "sendLongMessage")

	if len(text) <= TelegramMaxMessageLength {
		msg := tgbotapi.NewMessage(chatID, text)
		return b.sendMessage(msg)
	}

	parts := splitMessage(text, TelegramSafeMessageLength)

	var lastErr error
	for i, part := range parts {
		part = sanitizeUTF8(part)
		msg := tgbotapi.NewMessage(chatID, part)
		if len(parts) > 1 {
			indicator := fmt.Sprintf("(%d/%d)\n", i+1, len(parts))
			if len(indicator)+len(part) > TelegramMaxMessageLength {
				maxPartLen := TelegramMaxMessageLength - len(indicator)
				if maxPartLen > 0 {
					part = part[:maxPartLen]
				} else {
					indicator = ""
				}
			}
			msg.Text = indicator + part
		}
		if err := b.sendMessage(msg); err != nil {
			lastErr = err
			logger.Error("Failed to send message part",
				logger.ErrorField(err),
				logger.Int64("chat_id", chatID),
				logger.Int("part", i+1),
				logger.Int("total_parts", len(parts)),
			)
		}
	}

	return lastErr
}

// splitMessage разбивает текст на части заданной максимальной длины.
func splitMessage(text string, maxLen int) []string {
	if len(text) <= maxLen {
		return []string{text}
	}

	var parts []string
	current := ""

	lines := strings.Split(text, "\n")
	for _, line := range lines {
		if len(current)+len(line)+1 > maxLen && current != "" {
			parts = append(parts, current)
			current = ""
		}

		if len(line) > maxLen {
			words := strings.Fields(line)
			if len(words) == 0 {
				for i := 0; i < len(line); i += maxLen {
					end := i + maxLen
					if end > len(line) {
						end = len(line)
					}
					parts = append(parts, line[i:end])
				}
				current = ""
			} else {
				for _, word := range words {
					if len(word) > maxLen {
						if current != "" {
							parts = append(parts, current)
							current = ""
						}
						for i := 0; i < len(word); i += maxLen {
							end := i + maxLen
							if end > len(word) {
								end = len(word)
							}
							parts = append(parts, word[i:end])
						}
					} else {
						if len(current)+len(word)+1 > maxLen && current != "" {
							parts = append(parts, current)
							current = ""
						}
						if current != "" {
							current += " "
						}
						current += word
					}
				}
			}
		} else {
			if current != "" {
				current += "\n"
			}
			current += line
		}
	}

	if current != "" {
		parts = append(parts, current)
	}

	return parts
}

// editMessage редактирует сообщение, обрабатывая ошибку "message is not modified" как не критичную.
func (b *Bot) editMessage(edit tgbotapi.EditMessageTextConfig, chatID int64, fallbackText string) error {
	edit.Text = sanitizeTelegramOutput(edit.Text, chatID, "editMessage")
	if fallbackText != "" {
		fallbackText = sanitizeTelegramOutput(fallbackText, chatID, "editMessageFallback")
	}
	_, err := b.api.Send(edit)
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "message is not modified") ||
			strings.Contains(errStr, "message_not_modified") {
			logger.Debug("Message is not modified (expected behavior)",
				logger.String("error", errStr),
				logger.Int64("chat_id", chatID),
			)
			return nil
		}

		logger.Warn("Failed to edit message",
			logger.ErrorField(err),
			logger.Int64("chat_id", chatID),
		)
		return b.sendLongMessage(chatID, fallbackText)
	}
	return nil
}
