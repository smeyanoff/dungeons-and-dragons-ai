package gigachat

// Модели GigaChat API (значения поля "model" в запросах /chat/completions и /embeddings).
// Список — по документации GigaChat (https://developers.sber.ru/docs/ru/gigachat/models);
// Sber периодически добавляет новые модели, поэтому список стоит обновлять по мере необходимости.
// Валидация по этому списку — мягкая (предупреждение в логах, не блокировка): неизвестное,
// но валидное на стороне API имя модели не должно останавливать запуск бота.

// ChatModel — имя модели для генерации текста (chat completions).
type ChatModel string

const (
	ModelGigaChat     ChatModel = "GigaChat"      // Базовая модель (по умолчанию)
	ModelGigaChatPlus ChatModel = "GigaChat-Plus" // Расширенный контекст
	ModelGigaChatPro  ChatModel = "GigaChat-Pro"  // Более сложные задачи, выше качество
	ModelGigaChatMax  ChatModel = "GigaChat-Max"  // Топовая модель по качеству
	ModelGigaChat2    ChatModel = "GigaChat-2"    // Новое поколение моделей
	ModelGigaChat2Pro ChatModel = "GigaChat-2-Pro"
	ModelGigaChat2Max ChatModel = "GigaChat-2-Max"
)

// KnownChatModels возвращает список известных (задокументированных) chat-моделей GigaChat.
func KnownChatModels() []ChatModel {
	return []ChatModel{
		ModelGigaChat,
		ModelGigaChatPlus,
		ModelGigaChatPro,
		ModelGigaChatMax,
		ModelGigaChat2,
		ModelGigaChat2Pro,
		ModelGigaChat2Max,
	}
}

// IsKnownChatModel проверяет, входит ли имя модели в список задокументированных.
// false не означает ошибку — API может поддерживать модели, отсутствующие в этом списке.
func IsKnownChatModel(name string) bool {
	for _, m := range KnownChatModels() {
		if string(m) == name {
			return true
		}
	}
	return false
}

// EmbeddingModel — имя модели для эмбеддингов (/embeddings).
type EmbeddingModel string

const (
	ModelEmbeddings      EmbeddingModel = "Embeddings"      // Базовая модель эмбеддингов (по умолчанию)
	ModelEmbeddingsGigaR EmbeddingModel = "EmbeddingsGigaR" // Модель эмбеддингов для RAG/поиска по документам
)

// KnownEmbeddingModels возвращает список известных (задокументированных) моделей эмбеддингов GigaChat.
func KnownEmbeddingModels() []EmbeddingModel {
	return []EmbeddingModel{
		ModelEmbeddings,
		ModelEmbeddingsGigaR,
	}
}

// IsKnownEmbeddingModel проверяет, входит ли имя модели эмбеддингов в список задокументированных.
func IsKnownEmbeddingModel(name string) bool {
	for _, m := range KnownEmbeddingModels() {
		if string(m) == name {
			return true
		}
	}
	return false
}

// embeddingDimensions — размерность вектора для каждой известной модели эмбеддингов
// (по документации GigaChat и по фактическому ответу /embeddings). Используется, чтобы
// коллекция Qdrant создавалась с правильным размером вектора: несовпадение размерности
// не ошибка конфигурации, а runtime-ошибка Qdrant ("Vector dimension error"), которая
// проявляется только при первом поиске/индексации, а не при старте бота.
var embeddingDimensions = map[EmbeddingModel]int{
	ModelEmbeddings:      1024,
	ModelEmbeddingsGigaR: 2560,
}

// DefaultEmbeddingDimension — размерность по умолчанию для неизвестных/недокументированных моделей.
const DefaultEmbeddingDimension = 1024

// EmbeddingDimension возвращает размерность вектора для модели эмбеддингов.
// Для неизвестной модели возвращает DefaultEmbeddingDimension — это не блокирует запуск,
// но при реальном несовпадении размерности Qdrant вернет ошибку при первом поиске.
func EmbeddingDimension(model EmbeddingModel) int {
	if dim, ok := embeddingDimensions[model]; ok {
		return dim
	}
	return DefaultEmbeddingDimension
}
