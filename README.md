# Dungeons & Dragons AI - Telegram Bot

Telegram-бот для игры в Dungeons & Dragons, где Dungeon Master — это AI (GigaChat, Sber).

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

```bash
git clone <repository-url>
cd dungeons-and-dragons-ai
cp .env.example .env
```

Отредактируйте `.env` и укажите:
- `TELEGRAM_BOT_TOKEN` — токен вашего Telegram бота (получить у [@BotFather](https://t.me/BotFather))
- `GIGACHAT_CLIENT_ID` / `GIGACHAT_CLIENT_SECRET` — credentials GigaChat API

```bash
# Инфраструктура (PostgreSQL + Qdrant)
docker compose -f build/docker-compose.yml up -d

# Бот (нужен TELEGRAM_BOT_TOKEN)
docker compose -f build/docker-compose.yml --profile bot up -d

# Логи
docker compose -f build/docker-compose.yml --profile bot logs -f bot

# Остановка (добавьте -v для удаления данных, включая БД)
docker compose -f build/docker-compose.yml down
```

## Локальная разработка

```bash
go mod download

export TELEGRAM_BOT_TOKEN="your_token"
export DATABASE_URL="postgres://user:password@localhost:5432/dnd?sslmode=disable"
export GIGACHAT_CLIENT_ID="your_client_id"
export GIGACHAT_CLIENT_SECRET="your_client_secret"

# Зависимости (PostgreSQL + Qdrant) через Docker, либо локально (Postgres 16+, Qdrant v1.7+)
docker compose -f build/docker-compose.yml up -d postgres qdrant

go run ./cmd/bot
```

Hot reload: `air` (требует установки). Тесты и линт — см. `CLAUDE.md` (`go test ./...`, `golangci-lint run`).

## Переменные окружения

Полный список — в `.env.example`. Основные:

**Обязательные:**
- `TELEGRAM_BOT_TOKEN`, `GIGACHAT_CLIENT_ID`, `GIGACHAT_CLIENT_SECRET`

**Опциональные:**
- `DATABASE_URL` — подключение к PostgreSQL (по умолчанию — значение из docker-compose)
- `GIGACHAT_MODEL` — модель для генерации текста DM (по умолчанию `GigaChat`).
  Доступные: `GigaChat`, `GigaChat-Plus`, `GigaChat-Pro`, `GigaChat-Max`,
  `GigaChat-2`, `GigaChat-2-Pro`, `GigaChat-2-Max` (см. `pkg/gigachat/models.go`)
- `GIGACHAT_ANALYZER_MODEL` — отдельная модель для структурных LLM-вызовов
  `dm_analyzer` (бой/квесты/предметы, pre-check проверок навыков) — не творческая
  генерация, поэтому имеет смысл использовать модель дешевле `GIGACHAT_MODEL`
  (по умолчанию `GigaChat-2`)
- `GIGACHAT_EMBEDDINGS_MODEL` — модель для эмбеддингов/RAG (по умолчанию `Embeddings`;
  доступна также `EmbeddingsGigaR`)
- `QDRANT_HOST` (по умолчанию `qdrant` в Docker, `localhost` локально), `QDRANT_PORT` (`6334`)

## Использование

1. Найдите бота в Telegram, отправьте `/start`
2. `/newgame <тема>` — создать новую игру (например, `/newgame фэнтези мир с драконами`)
3. После создания игры пишите боту, что хотите сделать — AI DM отвечает

## Архитектура

Проект следует Clean Architecture, ручной DI (`cmd/bot/main.go`). Подробности — в `CLAUDE.md`.

```
cmd/bot/            # точка входа, ручной DI
internal/game/       # доменная логика D&D-кампании (domain/application/infrastructure)
internal/llm/        # абстракция над LLM (GigaChat-адаптер, monitored-декоратор)
internal/rag/        # RAG для контекстного поиска (Qdrant + GigaChat embeddings)
internal/telegram/   # presentation layer — Telegram bot handlers
internal/monitoring/ # HTTP API для LLM-логов (llm_logs), см. .claude/skills/read-dnd-bot-logs
pkg/gigachat/        # низкоуровневый HTTP-клиент GigaChat API
pkg/logger/          # обёртка над zap
pkg/version/         # версия/коммит/build time
k8s/                 # Kubernetes-манифесты
scripts/             # деплой, бэкап/восстановление
build/               # Dockerfile, docker-compose.{yml,prod.yml,...}
```

## Версионирование

Semantic Versioning, подробности — [VERSIONING.md](VERSIONING.md). Проверка: `curl http://localhost:8080/version`.

## Production деплой

Подробное руководство (Docker Compose, Kubernetes, бэкапы, мониторинг, troubleshooting) —
[DEPLOY.md](DEPLOY.md). Коротко:

```bash
cp .env.example .env   # заполните значения
make deploy             # build + up прод-стека
make prod-ps            # статус
make prod-logs           # логи
```

## CI/CD

`.github/workflows/`: автотесты и сборка образов при каждом push (`ci.yml`), security scanning
(`make security-scan`: gosec/trivy/semgrep/trufflehog/snyk), автоматический деплой при push в
`main`/теге `v*` (`deploy.yml`, `release.yml`) — см. [DEPLOY.md](DEPLOY.md#cicd).

## Лицензия

См. файл [LICENSE](LICENSE)
