# Интеграционные тесты

Интеграционные тесты для проверки основных механик игры D&D AI Bot.

## Требования

1. Запущенные контейнеры PostgreSQL и Qdrant:
   ```bash
   make docker-up
   ```

2. Переменные окружения для GigaChat:
   ```bash
   export GIGACHAT_CLIENT_ID="your_client_id"
   export GIGACHAT_CLIENT_SECRET="your_client_secret"
   ```

3. Опционально, для полного тестирования Telegram API:
   ```bash
   export TELEGRAM_BOT_TOKEN="your_bot_token"
   ```

## Запуск тестов

```bash
# Через Makefile (рекомендуется)
make test-integration              # Все интеграционные тесты
make test-telegram-stub            # Telegram e2e без реального LLM/Qdrant (только Postgres)
make test-telegram-real            # Telegram e2e с реальным LLM (GigaChat)
make test-telegram                 # Все Telegram gameplay тесты (stubbed + real; real может SKIP)

# Из корня проекта напрямую
go test -v ./tests/integration/...

# С указанием конкретного теста
go test -v ./tests/integration/... -run TestTelegramGameplay

# С таймаутом (тесты могут занимать время из-за LLM запросов)
go test -v ./tests/integration/... -timeout 30m
```

**Важно:** Результаты тестов автоматически записываются в `TESTING_REPORT.md` (проблемы) и `FEEDBACK.md` (странное поведение LLM).

## Что тестируется

### TestTelegramGameplay_CompleteFlow
Полный игровой процесс как реальный пользователь через Telegram:
1. Создание новой игры (/newgame)
2. Создание персонажа (/createcharacter)
3. Первое игровое действие (исследование)
4. Просмотр инвентаря (/inventory)
5. Исследование и подбор предмета
6. Просмотр ежедневных заданий (/daily)
7. Просмотр квестов (/quests)
8. Бросок кубика (/roll)
9. Просмотр заклинаний (/spells)
10. Просмотр достижений (/achievements)
11. Просмотр карты (/map)
12. Просмотр истории (/history)

**Особенности:**
- Тестирует реальные ответы LLM
- Автоматически записывает проблемы в TESTING_REPORT.md
- Автоматически записывает странное поведение LLM в FEEDBACK.md

### TestTelegramGameplay_CombatFlow
Боевая система как реальный пользователь:
1. Инициация боя через действие
2. Проверка статуса боя
3. Атака через команду /attack
4. Проверка HP после боя

### TestTelegramGameplay_DailyQuests
Система ежедневных заданий:
1. Получение ежедневных заданий
2. Проверка прогресса заданий

### TestTelegramGameplay_SpellSystem
Система заклинаний:
1. Просмотр заклинаний
2. Использование заклинания

### TestTelegramGameplay_BotSimulation_UserJourney_StubbedLLM
Стабильный “как пользователь в Telegram”, но без реального LLM/RAG:
- `/newgame` (stubbed JSON world)
- `/createcharacter`
- player action → tool-first → one-tap ability check (callback)
- `/map` + callback навигации
- `/history` (проверка на утечки tool-текста)
- `/endgame`

## Примечания

- `make test-telegram-stub` не требует реального LLM и не обращается к Qdrant (нужен Postgres).
- `make test-telegram-real` требует реального подключения к GigaChat API и может занимать значительное время.
- Реальные LLM вызовы в тестах ограничены rate limit’ом (по умолчанию 2500ms; `LLM_TEST_MIN_DELAY_MS`).
- Если контейнеры не запущены, часть тестов будет пропущена с сообщением.

## Troubleshooting

### Ошибка подключения к БД
Убедитесь, что контейнеры запущены:
```bash
make docker-up
docker ps
```

### Ошибка подключения к Qdrant
Проверьте, что Qdrant доступен:
```bash
curl http://localhost:6334/health
```

### Пропуск тестов из-за отсутствия GigaChat credentials
Тесты требуют реальные credentials для GigaChat. Если они не установлены, тесты будут пропущены.
