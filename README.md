# Dungeons & Dragons AI - Telegram Bot

Telegram-бот для игры в Dungeons & Dragons, где Dungeon Master - это AI (GigaChat).

## Возможности

- 🎲 Создание игровых кампаний через AI
- 🗺️ Генерация миров, локаций, NPC и квестов
- 💬 Интерактивная игра через Telegram
- 🎮 AI Dungeon Master отвечает на действия игроков
- 🔍 RAG (Retrieval-Augmented Generation) для контекстного поиска

## Требования

- Go 1.25+ (для локальной разработки)
- Docker и Docker Compose (для запуска через Docker)
- GigaChat API credentials
- Telegram Bot Token

## Быстрый старт с Docker

### 1. Клонируйте репозиторий

```bash
git clone <repository-url>
cd dungeons-and-dragons-ai
```

### 2. Настройте переменные окружения

Скопируйте пример файла окружения и заполните необходимые значения:

```bash
cp .env.example .env
```

Отредактируйте `.env` файл и укажите:
- `TELEGRAM_BOT_TOKEN` - токен вашего Telegram бота (получить у [@BotFather](https://t.me/BotFather))
- `GIGACHAT_CLIENT_ID` - Client ID для GigaChat API
- `GIGACHAT_CLIENT_SECRET` - Client Secret для GigaChat API

### 3. Запустите проект

```bash
# Инфраструктура (PostgreSQL + Qdrant)
docker compose -f build/docker-compose.yml up -d

# Бот (нужен TELEGRAM_BOT_TOKEN)
docker compose -f build/docker-compose.yml --profile bot up -d
```

Это запустит:
- PostgreSQL базу данных
- Qdrant векторное хранилище
- Telegram бота (только с профилем `bot`)

### 4. Проверьте логи

```bash
docker compose -f build/docker-compose.yml --profile bot logs -f bot
```

### 5. Остановка

```bash
docker compose -f build/docker-compose.yml down
```

Для удаления всех данных (включая базу данных):

```bash
docker compose -f build/docker-compose.yml down -v
```

## Локальная разработка

### 1. Установите зависимости

```bash
go mod download
```

### 2. Настройте переменные окружения

Создайте `.env` файл на основе `.env.example` или установите переменные окружения:

```bash
export TELEGRAM_BOT_TOKEN="your_token"
export DATABASE_URL="postgres://user:password@localhost:5432/dnd?sslmode=disable"
export GIGACHAT_CLIENT_ID="your_client_id"
export GIGACHAT_CLIENT_SECRET="your_client_secret"
```

### 3. Запустите зависимости (PostgreSQL и Qdrant)

Через Docker Compose (только зависимости):

```bash
docker compose -f build/docker-compose.yml up -d postgres qdrant
```

Или установите локально:
- PostgreSQL 16+
- Qdrant v1.7+

### 4. Запустите бота

```bash
go run ./cmd/bot
```

## Переменные окружения

Все переменные окружения описаны в файле `.env.example`. Основные:

### Обязательные

- `TELEGRAM_BOT_TOKEN` - токен Telegram бота
- `GIGACHAT_CLIENT_ID` - Client ID для GigaChat API
- `GIGACHAT_CLIENT_SECRET` - Client Secret для GigaChat API

### Опциональные

- `DATABASE_URL` - URL подключения к PostgreSQL (по умолчанию используется значение из docker-compose)
- `GIGACHAT_MODEL` - модель GigaChat для генерации текста DM (по умолчанию: `GigaChat`).
  Доступные модели: `GigaChat`, `GigaChat-Plus`, `GigaChat-Pro`, `GigaChat-Max`,
  `GigaChat-2`, `GigaChat-2-Pro`, `GigaChat-2-Max` (см. `pkg/gigachat/models.go`)
- `GIGACHAT_ANALYZER_MODEL` - отдельная модель для "проверочных" LLM-вызовов
  (`dm_analyzer`: определение боя/квестов/предметов, pre-check проверок навыков) —
  структурная классификация/JSON, а не творческая генерация, поэтому имеет смысл
  использовать модель дешевле `GIGACHAT_MODEL` (по умолчанию: `GigaChat-2`)
- `GIGACHAT_EMBEDDINGS_MODEL` - модель GigaChat для эмбеддингов/RAG (по умолчанию: `Embeddings`).
  Доступные модели: `Embeddings`, `EmbeddingsGigaR`
- `QDRANT_HOST` - хост Qdrant (по умолчанию: `qdrant` в Docker, `localhost` локально)
- `QDRANT_PORT` - порт Qdrant (по умолчанию: `6334`)

## Использование

1. Найдите вашего бота в Telegram
2. Отправьте `/start` для начала
3. Используйте `/newgame <тема>` для создания новой игры
   - Например: `/newgame фэнтези мир с драконами`
4. После создания игры просто пишите боту, что хотите сделать, и AI DM будет отвечать!

## Команды

- `/start` - Начало работы с ботом
- `/newgame <тема>` - Создать новую игру с указанной тематикой

## Архитектура

Проект следует Clean Architecture:

- `cmd/bot` - Точка входа приложения
- `internal/game` - Доменная логика игры
  - `domain` - Доменные модели
  - `application` - Use cases
  - `infrastructure` - Реализации (persistence, context)
- `internal/llm` - Абстракция LLM
- `internal/rag` - RAG для контекстного поиска
- `internal/telegram` - Telegram bot handler
- `pkg/gigachat` - GigaChat API client

## Docker Compose Services

- **postgres** - PostgreSQL 16 база данных
- **qdrant** - Qdrant векторное хранилище
- **bot** - Telegram бот приложение

## Версионирование

Проект использует Semantic Versioning. Подробности см. [VERSIONING.md](VERSIONING.md)

Проверка версии:
```bash
curl http://localhost:8080/version
```

## Production деплой

Для деплоя в production см. подробное руководство: [DEPLOY.md](DEPLOY.md)

### Быстрый старт для production

```bash
# 1. Настройте переменные окружения
cp .env.example .env
# Отредактируйте .env и заполните все значения

# 2. Деплой
make deploy

# 3. Проверка статуса
make prod-ps
make prod-logs
```

### Бэкапы

```bash
# Создание бэкапа
make backup

# Восстановление из бэкапа
make restore POSTGRES_BACKUP=backups/postgres_xxx.sql.gz QDRANT_BACKUP=backups/qdrant_xxx.tar.gz
```

## Разработка

```bash
# Запуск с hot reload (требует установки air)
air

# Тестирование
go test ./...

# Линтинг
golangci-lint run

# Сборка Docker образа
docker build -t dnd-bot .

# Просмотр логов
docker-compose logs -f bot

# Перезапуск бота
docker-compose restart bot
```

## Структура проекта

```
.
├── cmd/bot/              # Точка входа приложения
├── internal/
│   ├── game/            # Доменная логика игры
│   ├── llm/             # Абстракция LLM
│   ├── rag/             # RAG для контекстного поиска
│   └── telegram/        # Telegram bot handler
├── pkg/gigachat/        # GigaChat API client
├── k8s/                 # Kubernetes манифесты
├── scripts/             # Скрипты для деплоя и бэкапов
├── docker-compose.yml   # Development конфигурация
├── docker-compose.prod.yml  # Production конфигурация
└── DEPLOY.md            # Подробное руководство по деплою
```

## CI/CD

Проект включает автоматизированный CI/CD через GitHub Actions:
- Автоматические тесты при каждом push
- Сборка Docker образов
- Security scanning
- Автоматический деплой (настраивается)

## Лицензия

См. файл [LICENSE](LICENSE)
