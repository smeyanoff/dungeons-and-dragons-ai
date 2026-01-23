# Обратная связь от пользователей (summary)

**Последнее обновление:** Январь 2026

> Только **актуальные нерешенные проблемы** и сильные сигналы.
> План/фичи — в `PRODUCT_IDEAS.md`, статусы/спринт — в `TASKS.md`.

---

## Главный продуктовый сигнал (P0): проверки нельзя "починить промптом"
- При детальных инструкциях DM спамит проверками.
- Ожидание игроков: проверки **редкие и значимые**.
- Нужен контроль через **tools + runtime guardrails** (budget/cooldown/anti-trivial + обязательные reason/stakes).

---

## Актуальные проблемы (P1)

### ❌ Утечки "внутренней кухни" tools в player-facing текст (РЕЦИДИВ)
- **Новая проблема**: Ловушки просачиваются в DM текст ("Ловушка в замке древних теней...")
- **Симптом**: Игрок получает предупреждения о ловушках ДО того, как в них угодит
- **Влияние**: Ломает погружение, показывает внутреннюю механику игры
- **Нужно**: Ловушки активируются только при действиях игрока → тогда проверка навыка

### ❌ `/battlefield` нестабилен/не работает
- Нет прозрачности боя → сложнее играть и отлаживать поведение.

### ✅ UX проверок навыков (улучшено)
- Было: нужно писать "/roll d20", несоответствия в описании (восприятие vs мудрость).
- Решение: "/roll" без аргументов бросает d20; DM продолжает повествование автоматически.
- Результат: более удобный UX проверок.

### ❌ Утечка информации о ловушках в DM текст
- **Пример**: "Ловушка в замке древних теней Осторожно! В локации замок древних теней тебя подстерегает опасная ловушка..."
- **Проблема**: Игрок знает о ловушке заранее, нет surprise момента
- **Ожидание**: Ловушки должны активироваться при действиях игрока, тогда предлагать проверку

### ❌ Блокировка после провала проверки
- **Пример**: После провала проверки (d20=4+2=6) система пишет "проверка уже была выполнена"
- **Проблема**: Игра останавливается, игрок не знает что делать дальше
- **Ожидание**: После провала DM должен автоматически продолжить повествование с последствиями

### ❌ Блокировка после успешной проверки
- **Пример**: После успешной проверки (d20=17+0=17) система все равно пишет "проверка уже выполнена"
- **Проблема**: Даже успех блокирует игру, DM не продолжает повествование
- **Ожидание**: После успеха DM должен автоматически описать положительные последствия и продолжить

---

## Сильные сигналы качества (держать курс)
- Атмосферные описания, связная генерация мира.
- Бой в целом работает (инициатива/атаки/урон/победа).
- История/RAG помогает удерживать контекст (когда индексация стабильна).

---

## Инженерные алерты (влияют на UX/стабильность)
- **Утечка ловушек в DM текст**: analyzer просачивает внутреннюю информацию игроку
- **Блокировка после провала проверки**: pending check остается активным, прерывая игру
- **Блокировка после успешной проверки**: даже success не resolve pending check
- **Невалидный JSON от LLM** (особенно `InitCampaign`) → ретраи/repair/fallback, деградация мира.
- **DM Analyzer возвращает `{}` или битые поля** (напр. `combat_detected=true`, но `enemy.name=""`) → пропуски триггеров, бой может не стартовать.
- **RAG индексация**: редкие таймауты (событие в БД есть, но в индексе нет) → "провалы памяти".
- **GigaChat token expiry** выглядит неверным (подозрение: единицы `expires_in`) → риск некорректной ротации токенов.
- **Image download 403** → нестабильные изображения; нужен текстовый fallback.

---

## Последний сводный сигнал из интеграционных прогонов (2026-01-21)
- Часто **пустой JSON `{}` из DM Analyzer** → непредсказуемость и пропуски событий/боёв.
- Регулярные **утечки служебного текста** (проверки/ожидание `/roll`/tool-контент) → ломает погружение.
- Частые **JSON-repair/fallback** при генерации мира → нестабильность контракта.

## Новая обратная связь (2026-01-22)
- **NPC компаньоны**: Хотелось бы, чтобы NPC могли присоединяться к отряду
- **Частая генерация изображений**: DM слишком часто использует генерацию изображений
- **Блокировка после провала проверки**: После провала система блокирует дальнейшую игру вместо естественного продолжения
- **Блокировка после успешной проверки**: Даже после успеха (17) система блокирует игру, DM не продолжает повествование

## Актуальные проблемы из прогона 2026-01-22

### ❌ DM Analyzer возвращает невалидный JSON
- **Симптом**: LLM возвращает усеченный JSON: `{"combat_detected":false,"enemies":[],"quest_completed":false,"quest_failed":false,"quest_title":"","experience_gained":0,"experience_reason":"","items_received":[],"location_visited":null,"npc_met":n`
- **Результат**: JSON обрывается посередине поля `npc_met`, вызывая 6 retry попыток и fallback
- **Влияние на UX**: Location events при первом посещении могут не генерироваться, пропуск важных игровых триггеров
- **Тест**: Воспроизводится в `TestTelegramGameplay_BotSimulation_LocationEvent_FirstVisit`
- **Метрики**: `analyzer_json_empty_json count=6`, высокая частота fallback-сценариев
- **Рекомендация**: Улучшить prompt для DM Analyzer, добавить более строгую валидацию JSON перед отправкой в LLM
## Обратная связь от интеграционных тестов (2026-01-22 14:56:03)

1. Combat detection - goblin attack: Incorrect combat detection (expected true, got false)
2. Combat detection - goblin attack: Combat detected but no enemies parsed
3. Combat with multiple enemies: Incorrect combat detection (expected true, got false)
4. Combat with multiple enemies: Combat detected but no enemies parsed

---

## Обратная связь от интеграционных тестов (2026-01-22 14:56:20)

1. No rate limiting detected in rapid requests test

---

## Обратная связь от интеграционных тестов (2026-01-22 14:58:16)

1. Combat with multiple enemies: Incorrect combat detection (expected true, got false)
2. Combat with multiple enemies: Combat detected but no enemies parsed

---

## Обратная связь от интеграционных тестов (2026-01-22 14:58:36)

1. Combat detection - goblin attack: Incorrect combat detection (expected true, got false)
2. Combat detection - goblin attack: Combat detected but no enemies parsed
3. Combat with multiple enemies: Incorrect combat detection (expected true, got false)
4. Combat with multiple enemies: Combat detected but no enemies parsed

---

## Обратная связь от интеграционных тестов (2026-01-22 14:59:21)

1. Combat detection - goblin attack: Incorrect combat detection (expected true, got false)
2. Combat detection - goblin attack: Combat detected but no enemies parsed

---

## Обратная связь от интеграционных тестов (2026-01-22 14:01:36)

1. Combat detection - goblin attack: Incorrect combat detection (expected true, got false)
2. Combat detection - goblin attack: Combat detected but no enemies parsed
3. Combat with multiple enemies: Incorrect combat detection (expected true, got false)
4. Combat with multiple enemies: Combat detected but no enemies parsed

---

## Обратная связь от интеграционных тестов (2026-01-22 14:02:03)

1. Combat detection - goblin attack: Incorrect combat detection (expected true, got false)
2. Combat detection - goblin attack: Combat detected but no enemies parsed
3. Combat with multiple enemies: Incorrect combat detection (expected true, got false)
4. Combat with multiple enemies: Combat detected but no enemies parsed

---

## Комплексное тестирование всех механик (2026-01-23)

### 🤖 Наблюдения за поведением LLM в stub-тестах

**DM Analyzer проблемы с JSON парсингом:**
- **Симптом**: LLM возвращает усеченный JSON: `{"combat_detected":false,"enemies":[],"quest_completed":false,"quest_failed":false,"quest_title":"","experience_gained":0,"experience_reason":"","items_received":[],"location_visited":null,"npc_met":n`
- **Результат**: JSON обрывается посередине поля `npc_met`, вызывая 6 retry попыток и fallback
- **Влияние**: Location events при первом посещении могут не генерироваться, пропуск важных игровых триггеров
- **Частота**: `analyzer_json_empty_json count=6`, высокая частота fallback-сценариев
- **Рекомендация**: Улучшить prompt для DM Analyzer, добавить более строгую валидацию JSON перед отправкой в LLM

### ⚠️ Технические проблемы инфраструктуры

**Контейнеры и база данных:**
- PostgreSQL и Qdrant корректно запускаются и работают
- Все тесты компилируются после исправления API signatures
- Rate limiting работает корректно (2500ms между запросами)
- Stub-тесты проходят успешно для большинства функционала

**API изменения:**
- Требовалось обновление сигнатур функций после изменений в коде
- `NewBotWithAPIEndpoint` требует `playerRepo` параметр
- `NewMoveToLocationUseCase` требует LLM и дополнительные репозитории
- `NewAnalyzeDMResponseUseCase` требует `autoGenerateImages` флаг

### 📊 Метрики производительности

**Время выполнения:**
- Stub-тесты: ~1-2 секунды
- Real LLM тесты: корректно пропускаются без credentials
- Компиляция: успешна после исправления ошибок

**Надежность:**
- Все LLM-dependent тесты корректно SKIP при отсутствии credentials
- Infrastructure тесты стабильны
- Rate limiting предотвращает перегрузку API

### 🎯 Рекомендации по улучшению LLM поведения

1. **DM Analyzer JSON validation**: Добавить pre-validation JSON перед отправкой в LLM
2. **Prompt engineering**: Улучшить инструкции для генерации complete JSON responses
3. **Error handling**: Улучшить fallback логику для частично поврежденных JSON
4. **Testing**: Добавить больше тестов для edge cases в LLM responses

---

## Заключение комплексного тестирования

**Положительные результаты:**
- ✅ Все реализованные механики имеют тестовое покрытие
- ✅ Infrastructure стабильна (PostgreSQL, Qdrant, rate limiting)
- ✅ API contracts обновлены и работают корректно
- ✅ LLM-dependent тесты gracefully skip без credentials

**Области для улучшения:**
- 🔄 DM Analyzer JSON parsing reliability
- 🔄 LLM prompt optimization для consistent responses
- 🔄 Error handling для malformed LLM outputs

**Следующие шаги:**
- Запуск с реальными GIGACHAT credentials для полного end-to-end тестирования
- Мониторинг LLM метрик в production
- Расширение тестового покрытия для edge cases

---
