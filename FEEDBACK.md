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

### ✅ Утечки "внутренней кухни" tools в player-facing текст (исправлено)
- Были служебные строки ("выполняется проверка…", "/roll…", tool-аргументы/JSON) в ответах.
- Решение: analyzer-first проверки без участия DM, убрали утечки из LLM контекста.
- Результат: только художественный текст + понятный итог проверок.

### ❌ `/battlefield` нестабилен/не работает
- Нет прозрачности боя → сложнее играть и отлаживать поведение.

### ✅ UX проверок навыков (улучшено)
- Было: нужно писать "/roll d20", несоответствия в описании (восприятие vs мудрость).
- Решение: "/roll" без аргументов бросает d20; DM продолжает повествование автоматически.
- Результат: более удобный UX проверок.

---

## Сильные сигналы качества (держать курс)
- Атмосферные описания, связная генерация мира.
- Бой в целом работает (инициатива/атаки/урон/победа).
- История/RAG помогает удерживать контекст (когда индексация стабильна).

---

## Инженерные алерты (влияют на UX/стабильность)
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
