package domain

import "context"

// Usage содержит информацию о расходе токенов.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

type usageKey struct{}

// WithUsage добавляет в context указатель на Usage, который можно заполнить.
func WithUsage(ctx context.Context) (context.Context, *Usage) {
	usage := &Usage{}
	return context.WithValue(ctx, usageKey{}, usage), usage
}

// UsageFromContext извлекает Usage из context, если он был добавлен.
func UsageFromContext(ctx context.Context) (*Usage, bool) {
	usage, ok := ctx.Value(usageKey{}).(*Usage)
	return usage, ok
}
