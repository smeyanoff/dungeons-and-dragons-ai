# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project overview

Telegram-бот для игры в D&D, где Dungeon Master — это LLM (GigaChat, Sber). Go 1.25. Clean Architecture, ручной DI (без фреймворка) — всё связывается вручную в `cmd/bot/main.go`.

## Commands

```bash
go run ./cmd/bot                # запуск локально (нужны env, см. .env.example)
go build ./...
go test ./...                   # юнит-тесты
go test ./internal/game/application/dm_analyzer/ -run TestName -v   # один тест / пакет
golangci-lint run                # линт (используется в CI, конфиг по умолчанию — .golangci.yml нет)

make docker-up                  # поднять Postgres + Qdrant для локальной разработки/тестов
make docker-down

make test                       # go test ./...
make test-integration           # ./tests/integration/... (нужны контейнеры)
make test-telegram-stub         # gameplay-тесты без реального LLM (только Postgres) — стабильны, гонять чаще всего
make test-telegram-real         # одна кампания с реальным GigaChat (может SKIP без credentials)
make test-telegram-real-all     # полный набор real-LLM тестов
make test-telegram              # stub + real (одна кампания)

make deploy                     # деплой через build/docker-compose.prod.yml (единственный способ перезапуска прод-контейнеров)
make backup / make restore
make security-scan               # gosec + trivy + semgrep + trufflehog + snyk (по отдельности тоже есть таргеты)
```

Интеграционные тесты — обычный пакет `integration` (без build tags), живут в `tests/integration/`. Перед real-LLM тестами нужно подхватить `.env`: `set -a && source .env && set +a`. Rate-limit для real-LLM тестов регулируется `LLM_TEST_MIN_DELAY_MS` (по умолчанию 2500ms) — см. `tests/integration/README.md`.

## Architecture

Три bounded context'а под `internal/`, каждый по Clean Architecture (`domain` / `application` / `infrastructure`):

- **`internal/game`** — доменная логика D&D-кампании. `domain/*` — модели без зависимостей (character, combat, world, quest, session, spell, inventory, item, npc, rating, achievement, subscription, feedback, llm_log, event, location, dice). `application/*` — use cases, по одному пакету на фичу (campaign, player_action, combat, character, dm_analyzer, dm_tools, history, image, inventory, location_event, quest, rating, session, spell, subscription, world_event, worldmap, ability_check, achievement, dice, jsonrepair). `infrastructure/persistence` — GORM-репозитории (Postgres); `infrastructure/context` — построение контекста для промпта DM (`SimpleContextBuilder`, обёрнутый в `RAGContextBuilder`); `infrastructure/cache` — кэш ответов DM.
- **`internal/llm`** — абстракция над LLM. `domain.LLM` — интерфейс (`Generate`, `GenerateWithMaxTokens`, `GenerateWithTools`) и `domain/tools` — схема tool calling. `infrastructure/gigachat.go` — адаптер к `pkg/gigachat`; `infrastructure/monitored_llm.go` — декоратор, логирующий каждый запрос/ответ в `llm_log` (через `persistence.LLMLogRepository`) — используется для дебага поведения DM и заполнения `TESTING_REPORT.md`/`FEEDBACK.md`.
- **`internal/rag`** — RAG для контекстного поиска. `application` — `IndexDocument` / `RetrieveContext` use cases; `infrastructure/embeddings` — эмбеддинги через GigaChat; `infrastructure/vectorstore` — клиент Qdrant.
- **`internal/telegram`** — единственный презентационный слой; `bot.go` + `bot_*.go` — хендлеры разбиты по темам (commands, gameplay, character, map, callbacks, feedback, preferences, health, status). Раньше это был один огромный файл — теперь разнесён специально, не собирать обратно.
- **`pkg/gigachat`** — низкоуровневый HTTP-клиент Sber GigaChat API (auth/токены, chat completion, embeddings, генерация изображений). Не содержит доменной логики.
- **`pkg/logger`** — обёртка над zap, инициализируется в `main()` до всего остального (`logger.InitFromEnv()`).
- **`pkg/version`** — версия/коммит/build time, прокидываются через ldflags при сборке (см. `/version` HTTP endpoint).

### DI и точка входа

`cmd/bot/main.go` — единственное место, где всё связывается вручную: репозитории → use cases → `telegram.Bot`. Никакого DI-контейнера нет; при добавлении нового use case его нужно явно завести здесь (создать репозиторий/зависимости, передать в конструктор, при необходимости — адаптер-обёртку для несовпадающих интерфейсов между пакетами, см. `dailyQuestProgressAdapterForPlayerAction`, `locationEventRepoAdapter` и т.п. прямо в `main.go`). Порядок инициализации важен: логгер → env/config → GigaChat client → LLM (+ monitored-обёртка) → Qdrant/RAG → репозитории → use cases (многие друг на друга ссылаются, напр. `handleActionUC` собирает почти всё) → `telegram.Bot`.

### DM-флоу (ядро игры)

Действие игрока обрабатывается в `player_action.HandleActionUseCase` — он контекстно самый крупный use case, комбинирующий: построение контекста (RAG + история + инвентарь + бой + world events), вызов LLM с tools, `dm_analyzer` (анализ ответа DM: обнаружение боя/проверок навыков/JSON-репар через `jsonrepair`), обновление опыта/достижений/квестов/рейтинга, кэш ответов (`dmcache`), генерацию изображений по лимитам подписки. При работе с игровым флоу ожидай, что изменение в одном месте (напр. формат ответа LLM) может требовать правок в analyzer, prompt-builder и integration-тестах одновременно.

### Тестирование

- Юнит-тесты — рядом с кодом (`*_test.go` в том же пакете).
- Интеграционные — `tests/integration/`, пакет `integration`, без build tags; общий сетап в `test_support_integration_test.go` (`setupIntegrationTest`/`cleanupTest`). Многие тесты гоняют полный сценарий через фейковый Telegram API (`telegram_bot_simulation_test.go` и другие `telegram_*` файлы) — от `/newgame` до первого боя.
- Известные open issues и приоритеты — `PROBLEMS.md` (P0/P1/P2) и `CODE_REVIEW.md`; поведенческие баги LLM/DM — `FEEDBACK.md`; статус задач — `TASKS.md`. Стоит свериться с ними перед тем, как чинить что-то в DM-флоу или combat — велик шанс, что проблема уже описана и есть решение/контекст.

### Прочее

- `pkg/version` версия отдаётся на `/version`, health check — через глобальную переменную `bot` в `main.go`.
- Секреты токена Telegram могут попадать в stdlib `log` от `go-telegram-bot-api` — в `main.go` стоит редактирующий `io.Writer` (`newRedactingWriter`), который вырезает токен из URL перед выводом; не убирать при рефакторинге логирования.
- Продакшн — Docker Compose (`build/docker-compose.prod.yml`) + k8s-манифесты в `k8s/`; подробности — `DEPLOY.md`.
