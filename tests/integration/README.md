# Интеграционные тесты

Интеграционные тесты для проверки основных механик игры D&D AI Bot.

## Одна тестовая кампания

Основной сценарий: **от появления в мире до первого боя в другой локации** (как игра через Telegram, с моком Telegram API).

- **Stub** (без реального LLM/RAG): `make test-telegram-stub` — стабильно, только Postgres.
- **Real LLM** (одна кампания, RAG включен): загрузите `.env`, затем:
  ```bash
  set -a && source .env && set +a
  go test -v -count=1 -timeout 60m ./tests/integration/... -run 'TestTelegramGameplay_RealLLM_SingleCampaign_ToFirstCombat'
  ```
  или: `make test-telegram-real` (переменные окружения должны быть в shell, например из `.env`).
- **Cassette (record/replay)** — тот же сценарий, но без сети и credentials после однократной записи;
  см. раздел «Cassette-тестирование (record/replay)» ниже.

**Перезапуск контейнеров — только через `make deploy`.**

## Требования

1. Запущенные контейнеры PostgreSQL и Qdrant:
   ```bash
   make docker-up
   ```

2. Переменные окружения для GigaChat (для real-LLM тестов):
   ```bash
   export GIGACHAT_CLIENT_ID="your_client_id"
   export GIGACHAT_CLIENT_SECRET="your_client_secret"
   ```
   Рекомендуется использовать `.env`: `set -a && source .env && set +a`.

## Запуск тестов

```bash
# Одна тестовая кампания (рекомендуется)
make test-telegram-stub   # без LLM
make test-telegram-real  # с реальным LLM (одна кампания)

# Все Telegram gameplay (stub + real одна кампания)
make test-telegram

# Расширенный набор real-LLM тестов
make test-telegram-real-all

# Все интеграционные тесты
make test-integration
```

**Важно:** Результаты тестов автоматически записываются в `TESTING_REPORT.md` (проблемы и анализ запросов/ответов/тулзов) и `FEEDBACK.md` (странное поведение LLM).

## Что тестируется (одна кампания)

- `/newgame <theme>` → ответ DM на создание мира
- `/createcharacter <name race class>`
- Действие игрока → player-facing prompt `🎲 Нужна проверка ... (DC ...)` + `/roll`
- `/roll d20` → pending ability check очищается
- `/map` → callback навигации → смена локации
- Первый бой в новой локации → `/battlefield` → `/attack`
- Анализ `llm_logs`: запросы с tools (combat/inventory/etc). Проверка навыка — analyzer-first
  (`needs_ability_check` в JSON-анализе действия игрока), `request_ability_check` не регистрируется
  как tool для DM и не встречается в `llm_logs`; сигнал флоу — player-facing prompt выше.

### TestTelegramGameplay_BotSimulation_UserJourney_StubbedLLM

Стабильный сценарий без реального LLM/RAG: тот же поток с stubbed миром и ответами.

### Другие тесты (опционально)

- `TestTelegramGameplay_CompleteFlow`, `TestTelegramGameplay_CombatFlow`,
  `TestTelegramGameplay_DailyQuests`, `TestTelegramGameplay_SpellSystem` — use-case уровень,
  без fake Telegram.
- `TestLLM_RealIntegration_CombatAnalysis`, `TestLLM_RealIntegration_RateLimit`,
  `TestTelegramGameplay_CoreMechanics_RealLLM` — точечные real-LLM тесты (анализ боя,
  rate limiting, комплексный journey); входят в `make test-telegram-real-all`.

## Cassette-тестирование (record/replay)

Позволяет прогонять real-LLM тесты **локально/в CI без сети и GigaChat credentials**, воспроизводя
реальные ответы модели, записанные один раз. Механизм — `tests/integration/llm_cassette_test.go`;
кассетируются оба внешних вызова: LLM (`Generate`/`GenerateWithMaxTokens`/`GenerateWithTools`) и
эмбеддер RAG (`Embed`) — DM-промпт почти всегда включает RAG-контекст, поэтому без кассетирования
эмбеддингов текст prompt разошёлся бы между записью и воспроизведением. Сам векторный поиск (Qdrant)
не кассетируется — выполняется реально локально (`make docker-up` всё ещё нужен), но за счёт
кассетированных эмбеддингов возвращает тот же результат.

```bash
# 1) Записать кассету (нужны реальные GIGACHAT credentials в .env, make docker-up)
set -a && source .env && set +a
make test-telegram-record CASSETTE=tests/integration/cassettes/single_campaign.json \
  RUN=TestTelegramGameplay_RealLLM_SingleCampaign_ToFirstCombat

# 2) Воспроизвести офлайн (credentials НЕ нужны — можно даже unset их перед запуском)
make test-telegram-replay CASSETTE=tests/integration/cassettes/single_campaign.json \
  RUN=TestTelegramGameplay_RealLLM_SingleCampaign_ToFirstCombat
```

Кассеты — обычные JSON-файлы, коммитятся в git (`tests/integration/cassettes/`), чтобы другой
разработчик или CI гонял replay без единого сетевого запроса.

Если после изменения кода промпт стал другим (например, поменялся `RAGContextBuilder` или добавился
новый tool), replay упадёт с понятной ошибкой `cassette miss: ...` — это сигнал пересоздать кассету
через `make test-telegram-record` заново.

## Примечания

- `make test-telegram-stub` не требует реального LLM и Qdrant (нужен Postgres).
- `make test-telegram-real` требует GigaChat credentials в env и может занимать время (rate limit по умолчанию 2500ms; `LLM_TEST_MIN_DELAY_MS`).
- `make test-telegram-replay` не требует GigaChat credentials вообще, но требует поднятый Qdrant (`make docker-up`).
- Перезапуск контейнеров — только `make deploy`.

## Troubleshooting

### Ошибка подключения к БД
Убедитесь, что контейнеры запущены: `make docker-up`, `docker ps`.

### Ошибка подключения к Qdrant
Проверьте: `curl http://localhost:6334/healthz`.

### Пропуск тестов из-за отсутствия GigaChat credentials
Подхватите `.env`: `set -a && source .env && set +a` перед запуском тестов.

### `tls: failed to verify certificate: x509: certificate signed by unknown authority`
На хосте (macOS/Linux без сертификатов Сбербанка) добавьте в `.env`:
`GIGACHAT_SKIP_TLS_VERIFY=true`. Внутри Docker-образа сертификаты Сбербанка уже установлены
(см. `build/Dockerfile`), поэтому запуск через `make test-integration-gameplay-docker` или
`docker exec dnd-bot-prod sh -c "cd /root && go test -v -timeout 60m ./tests/integration/... -run 'TestTelegramGameplay'"`
не требует этой переменной.
