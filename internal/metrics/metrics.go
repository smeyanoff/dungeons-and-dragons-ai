package metrics

import "sync/atomic"

// Счётчики для мониторинга (402 GigaChat, RAG-failures, утечки системного текста в DM).

var (
	ragFailureCount  int64 // неудачная индексация или поиск RAG
	outputLeakCount  int64 // срабатывание output-guard (удалён tool/internal текст из ответа игроку)
	injectionAttempt int64 // срабатывание input-guard (заблокирована попытка взлома сессии/DM)
)

// IncrementRAGFailure увеличивает счётчик неудач RAG и возвращает новое значение.
func IncrementRAGFailure() int64 {
	return atomic.AddInt64(&ragFailureCount, 1)
}

// IncrementOutputLeak увеличивает счётчик срабатываний output-guard и возвращает новое значение.
func IncrementOutputLeak() int64 {
	return atomic.AddInt64(&outputLeakCount, 1)
}

// RAGFailureCount возвращает текущее значение счётчика неудач RAG.
func RAGFailureCount() int64 {
	return atomic.LoadInt64(&ragFailureCount)
}

// OutputLeakCount возвращает текущее значение счётчика срабатываний output-guard.
func OutputLeakCount() int64 {
	return atomic.LoadInt64(&outputLeakCount)
}

// IncrementInjectionAttempt увеличивает счётчик заблокированных попыток взлома
// сессии/DM (input-guard) и возвращает новое значение.
func IncrementInjectionAttempt() int64 {
	return atomic.AddInt64(&injectionAttempt, 1)
}

// InjectionAttemptCount возвращает текущее значение счётчика заблокированных
// попыток взлома сессии/DM.
func InjectionAttemptCount() int64 {
	return atomic.LoadInt64(&injectionAttempt)
}
