# Logger Package

Централизованный структурированный логгер на основе [zap](https://github.com/uber-go/zap).

## Использование

### Инициализация

Логгер автоматически инициализируется из переменных окружения при запуске приложения:

```go
import "dungeons-and-dragons-ai/pkg/logger"

func main() {
    if err := logger.InitFromEnv(); err != nil {
        // обработка ошибки
    }
    defer logger.Sync()
    
    // Использование логгера
    logger.Info("Application started")
}
```

### Переменные окружения

- `LOG_LEVEL` - уровень логирования: `debug`, `info`, `warn`, `error` (по умолчанию: `info`)
- `LOG_DEVELOPMENT` - режим разработки: `true`/`false` (по умолчанию: `false`)

### Примеры использования

```go
// Простое логирование
logger.Info("User logged in")
logger.Error("Failed to connect to database")

// С полями
logger.Info("Processing request",
    logger.String("user_id", "123"),
    logger.Int("request_id", 456),
    logger.Duration("duration", time.Since(start)),
)

// С ошибкой
logger.Error("Failed to save data",
    logger.ErrorField(err),
    logger.String("table", "users"),
)

// Debug логирование
logger.Debug("Cache hit",
    logger.String("key", cacheKey),
)

// Warning
logger.Warn("Rate limit approaching",
    logger.Int("current", 90),
    logger.Int("limit", 100),
)
```

### Уровни логирования

- **DEBUG** - детальная информация для отладки
- **INFO** - общая информация о работе приложения
- **WARN** - предупреждения о потенциальных проблемах
- **ERROR** - ошибки, требующие внимания

### Структурированные поля

Все логи структурированы и содержат:
- `timestamp` - время события в формате ISO8601
- `level` - уровень логирования
- `caller` - место в коде, откуда вызван лог
- `message` - текстовое сообщение
- Дополнительные поля, переданные при вызове

### Пример вывода

```json
{
  "timestamp": "2025-01-16T20:30:14.979Z",
  "level": "info",
  "caller": "cmd/bot/main.go:239",
  "message": "Starting bot",
  "version": "1.0.0",
  "commit": "abc123",
  "buildTime": "2025-01-16T15:57:02Z"
}
```
