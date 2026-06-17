# Тесты с реальной LLM (GigaChat)

## Настройка для запуска тестов с реальной LLM

### 1. Получение GigaChat credentials

1. Зарегистрируйтесь на [GigaChat API](https://developers.sber.ru/portal/products/gigachat-api)
2. Получите `client_id` и `client_secret`
3. Установите переменные окружения:

```bash
export GIGACHAT_CLIENT_ID="your_client_id_here"
export GIGACHAT_CLIENT_SECRET="your_client_secret_here"
```

### 2. Опциональные настройки

```bash
export GIGACHAT_MODEL="GigaChat"                    # Модель (по умолчанию GigaChat)
export GIGACHAT_SKIP_TLS_VERIFY="true"              # Для тестов на localhost (по умолчанию false)
export GIGACHAT_AUTH_URL="https://ngw.devices.sberbank.ru:9443"  # URL авторизации
export GIGACHAT_API_URL="https://gigachat.devices.sberbank.ru/api/v1"  # API URL
export GIGACHAT_SCOPE="GIGACHAT_API_PERS"           # Scope доступа
```

### 3. Запуск тестов

```bash
# Запуск всех тестов с реальной LLM
make test-telegram-real

# Или конкретные тесты
go test -v ./tests/integration/... -run "TestLLM_RealIntegration_"

# С кастомными настройками rate limiting
LLM_TEST_MIN_DELAY_MS=3000 go test -v ./tests/integration/... -run "TestLLM_RealIntegration_"
```

## Доступные тесты с реальной LLM

### TestLLM_RealIntegration_CombatAnalysis
- Тестирует анализ боя DM Analyzer с реальными ответами LLM
- Проверяет корректность распознавания комбат-ситуаций
- Валидирует парсинг врагов, опыта, предметов
- Проверяет обработку усеченного JSON

### TestLLM_RealIntegration_RateLimit
- Тестирует rate limiting механизмы
- Проверяет корректность задержек между запросами
- Мониторит 429 ошибки от API

### TestTelegramGameplay_CoreMechanics_RealLLM
- Комплексный end-to-end тест всех основных механик
- Симулирует полный пользовательский journey
- Проверяет создание игры, персонажа, исследование, бой, инвентарь, квесты

## Результаты тестирования

Результаты записываются в:
- `TESTING_REPORT.md` - проблемы и статусы
- `FEEDBACK.md` - обратная связь по качеству LLM ответов

## Важные замечания

1. **Стоимость**: Тесты с реальной LLM потребляют API квоты GigaChat
2. **Скорость**: Тесты работают медленнее из-за rate limiting (2 секунды между запросами)
3. **Надежность**: Возможны сетевые ошибки или временные проблемы с API
4. **Rate limiting**: Автоматически применяется для предотвращения превышения лимитов

## Troubleshooting

### Ошибка авторизации
```
tls: failed to verify certificate
```
**Решение**: Установите `GIGACHAT_SKIP_TLS_VERIFY=true`

### Rate limit errors
```
429 Too Many Requests
```
**Решение**: Увеличьте `LLM_TEST_MIN_DELAY_MS` или подождите

### Empty credentials
```
GIGACHAT_CLIENT_ID и GIGACHAT_CLIENT_SECRET не установлены
```
**Решение**: Установите переменные окружения как описано выше