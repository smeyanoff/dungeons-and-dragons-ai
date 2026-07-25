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
- Анализ `llm_logs`: запросы с tools, вызов `request_ability_check`

### TestTelegramGameplay_BotSimulation_UserJourney_StubbedLLM

Стабильный сценарий без реального LLM/RAG: тот же поток с stubbed миром и ответами.

### Другие тесты (опционально)

- `TestTelegramGameplay_CompleteFlow`, `TestTelegramGameplay_CombatFlow` — use-case уровень, без fake Telegram.
- `make test-telegram-real-all` — полный набор real-LLM тестов.

## Примечания

- `make test-telegram-stub` не требует реального LLM и Qdrant (нужен Postgres).
- `make test-telegram-real` требует GigaChat credentials в env и может занимать время (rate limit по умолчанию 2500ms; `LLM_TEST_MIN_DELAY_MS`).
- Перезапуск контейнеров — только `make deploy`.

## Troubleshooting

### Ошибка подключения к БД
Убедитесь, что контейнеры запущены: `make docker-up`, `docker ps`.

### Ошибка подключения к Qdrant
Проверьте: `curl http://localhost:6334/healthz`.

### Пропуск тестов из-за отсутствия GigaChat credentials
Подхватите `.env`: `set -a && source .env && set +a` перед запуском тестов.
