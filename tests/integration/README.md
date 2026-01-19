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
make test-integration-gameplay     # Тесты игрового процесса (как пользователь)

# Из корня проекта напрямую
go test -v ./tests/integration/...

# С указанием конкретного теста
go test -v ./tests/integration/... -run TestGameFlow_CompleteScenario
go test -v ./tests/integration/... -run TestTelegramGameplay

# С таймаутом (тесты могут занимать время из-за LLM запросов)
go test -v ./tests/integration/... -timeout 30m
```

**Важно:** Результаты тестов автоматически записываются в `TESTING_REPORT.md` (проблемы) и `FEEDBACK.md` (странное поведение LLM).

## Что тестируется

### TestGameFlow_CompleteScenario
Полный сценарий игры:
1. Создание новой игры
2. Создание персонажа
3. Игровое действие (исследование)
4. Просмотр инвентаря
5. Подбор предмета
6. Бросок кубика
7. Просмотр квестов
8. Просмотр ежедневных заданий
9. Просмотр карты
10. Просмотр истории
11. Просмотр достижений
12. Просмотр заклинаний
13. Завершение игры

### TestCombatMechanics
Боевые механики:
- Инициация боя через действие
- Атака через команду

### TestCharacterCreation
Создание персонажей разных рас и классов:
- Elf Wizard
- Human Fighter
- Dwarf Cleric
- Orc Rogue
- Halfling Ranger

### TestInventoryOperations
Операции с инвентарем:
- Просмотр пустого инвентаря
- Подбор предмета

### TestDiceRolling
Броски кубиков:
- d20
- 2d6
- d20+5
- 2d6+3
- d100

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

## Примечания

- Тесты требуют реального подключения к GigaChat API
- Тесты могут занимать значительное время из-за LLM запросов
- Тесты автоматически очищают данные после выполнения
- Если контейнеры не запущены, тесты будут пропущены с сообщением

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
