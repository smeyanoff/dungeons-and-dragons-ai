# Инструкции по запуску интеграционных тестов

## ⚠️ Важно: TLS сертификаты

**Для тестов на хосте (macOS/Linux без сертификатов Сбербанка):**
- Добавьте `GIGACHAT_SKIP_TLS_VERIFY=true` в `.env` файл
- Или экспортируйте: `export GIGACHAT_SKIP_TLS_VERIFY=true`

**Для тестов внутри контейнера (рекомендуется):**
- Сертификаты Сбербанка уже настроены в Docker образе
- Используйте: `make test-integration-gameplay-docker`
- `GIGACHAT_SKIP_TLS_VERIFY` не требуется

## Быстрый старт

1. **Убедитесь, что контейнеры запущены:**
   ```bash
   make docker-up
   ```

2. **Установите переменные окружения для GigaChat:**
   
   **Вариант А: Использование .env файла (рекомендуется)**
   ```bash
   # Добавьте в .env файл:
   GIGACHAT_CLIENT_ID="your_client_id"
   GIGACHAT_CLIENT_SECRET="your_client_secret"
   # Для тестов на хосте (без сертификатов):
   GIGACHAT_SKIP_TLS_VERIFY=true
   ```
   
   **Вариант Б: Экспорт переменных**
   ```bash
   export GIGACHAT_CLIENT_ID="your_client_id"
   export GIGACHAT_CLIENT_SECRET="your_client_secret"
   # Для тестов на хосте (без сертификатов):
   export GIGACHAT_SKIP_TLS_VERIFY=true
   ```

3. **Запустите тесты игрового процесса:**
   
   **Стабильный Telegram e2e без реального LLM (рекомендуется для CI/быстрых прогонов):**
   ```bash
   make test-telegram-stub
   ```

   **На хосте с реальным LLM (требует GIGACHAT_SKIP_TLS_VERIFY=true):**
   ```bash
   make test-telegram-real
   ```
   
   **Внутри контейнера (где сертификаты уже настроены):**
   ```bash
   # Тесты будут использовать сертификаты из контейнера
   docker exec -it dnd-bot-prod sh -c "cd /root && go test -v -timeout 60m ./tests/integration/... -run 'TestTelegramGameplay'"
   ```

## Rate limit для реального LLM (важно)

Интеграционные тесты автоматически ограничивают частоту обращений к LLM, чтобы не DDOSить модель.

- По умолчанию: **2500ms** между LLM запросами
- Настройка:

```bash
LLM_TEST_MIN_DELAY_MS=4000 make test-telegram-real
```

## Что тестируется

### TestTelegramGameplay_CompleteFlow
Полный игровой процесс как реальный пользователь:
- Создание игры и персонажа
- Игровые действия (исследование, подбор предметов)
- Просмотр всех систем (инвентарь, квесты, заклинания, достижения, карта, история)
- Ежедневные задания

### TestTelegramGameplay_CombatFlow
Боевая система:
- Инициация боя через действие
- Проверка статуса боя
- Атака через команду
- Проверка HP после боя

### TestTelegramGameplay_DailyQuests
Система ежедневных заданий:
- Получение заданий
- Проверка прогресса

### TestTelegramGameplay_SpellSystem
Система заклинаний:
- Просмотр заклинаний
- Использование заклинаний

## Автоматическая запись результатов

Тесты автоматически записывают результаты в:

1. **TESTING_REPORT.md** - найденные проблемы и ошибки
2. **FEEDBACK.md** - странное поведение LLM (короткие ответы, отсутствие ожидаемых элементов)

## Время выполнения

- `make test-telegram-stub`: обычно **быстро** (без LLM).
- `make test-telegram-real`: может занять **30-60 минут** из-за реальных запросов к LLM.

## Troubleshooting

### Контейнеры не запущены
```bash
make docker-up
docker ps  # Проверьте, что postgres и qdrant запущены
```

### Ошибка подключения к БД
Проверьте, что PostgreSQL доступен:
```bash
docker ps | grep postgres
```

### Ошибка подключения к Qdrant
Проверьте, что Qdrant доступен:
```bash
curl http://localhost:6334/health
```

### Отсутствуют GigaChat credentials
Тесты требуют реальные credentials. Если они не установлены, тесты будут пропущены.

### Проблемы с TLS сертификатом
Если тесты падают с ошибкой `tls: failed to verify certificate: x509: certificate signed by unknown authority`:

**Решение 1: Использовать GIGACHAT_SKIP_TLS_VERIFY (для тестов на хосте)**
```bash
# Добавьте в .env файл:
GIGACHAT_SKIP_TLS_VERIFY=true

# Или экспортируйте переменную:
export GIGACHAT_SKIP_TLS_VERIFY=true
```

**Решение 2: Запускать тесты внутри контейнера (где сертификаты настроены)**
```bash
# В контейнере сертификаты Сбербанка уже установлены
docker exec -it dnd-bot-prod sh -c "cd /root && go test -v -timeout 60m ./tests/integration/... -run 'TestTelegramGameplay'"
```

**Примечание:** В Docker образе сертификаты Сбербанка уже установлены (см. `build/Dockerfile`), поэтому тесты внутри контейнера работают без `GIGACHAT_SKIP_TLS_VERIFY`.

## Запуск отдельных тестов

```bash
# Только полный игровой процесс
go test -v ./tests/integration/... -run TestTelegramGameplay_CompleteFlow

# Только боевая система
go test -v ./tests/integration/... -run TestTelegramGameplay_CombatFlow

# Только ежедневные задания
go test -v ./tests/integration/... -run TestTelegramGameplay_DailyQuests

# Только система заклинаний
go test -v ./tests/integration/... -run TestTelegramGameplay_SpellSystem
```

## Что проверяется

### Функциональность
- ✅ Все команды работают без ошибок
- ✅ Данные сохраняются в БД
- ✅ Состояние игры корректно обновляется

### Качество LLM ответов
- ✅ Ответы не пустые
- ✅ Ответы содержат ожидаемые элементы (описание локации, боевые элементы и т.д.)
- ✅ Ответы достаточно подробные

### Интеграция систем
- ✅ Ежедневные задания создаются и отслеживаются
- ✅ Боевая система корректно инициируется
- ✅ Заклинания доступны для магических классов
- ✅ Достижения отображаются

## После запуска тестов

1. Проверьте **TESTING_REPORT.md** на наличие проблем
2. Проверьте **FEEDBACK.md** на странное поведение LLM
3. Исправьте найденные проблемы
4. Запустите тесты снова для проверки исправлений
