# CODE_REVIEW — Dungeons & Dragons AI Bot (Telegram + AI DM + RAG)

**Последнее обновление:** 2026-01-21  
**Зачем файл:** держим **короткий** список **активных** рисков/дефектов + конкретные action items (что сделать / где / как проверить).  
**Историю “что уже починили” здесь не храним** — только актуальное.

---

## ✅ Состояние (на 2026-01-21)

- **Тесты**: `go test ./...` ✅ PASS; интеграционные прогоны дают сигналы/проблемы (см. `TESTING_REPORT.md`, `FEEDBACK.md`).
- **Docker (prod)**: `dnd-bot-prod`, `dnd-postgres-prod`, `dnd-qdrant-prod` ✅ healthy.
  В логах `dnd-bot-prod` зафиксированы: **invalid JSON + failed repair/retry**, **GigaChat 429 rate limit**, **image download 403 Permission denied**, **hard truncation контекста промпта**.

---

## ✅ Активные риски

### P0 — качество геймплея: проверки нельзя "починить промптом" ✅ (частично решено)
- **Симптом**: DM начинает **спамить проверками**; игроки ожидают "редко и значимо" (см. `FEEDBACK.md`).
- **Решено**: Введены guardrails (budget/cooldown/anti-trivial) + обязательные stakes/reason. Требуется доработка analyzer-first логики.
- **Action items**:
  - Полностью перенести "решение о проверке" в код (tools/анализатор), а не в промпт.
- **Как проверить**:
  - `tests/integration/ability_check_guardrails_test.go`
  - Логи: отсутствие "цепочек" проверок на тривиальные действия.

### P1 — утечки служебного/инструментального текста в player-facing ответы
- **Симптом**: в тексте появляются намёки на tools/JSON/“ожидаемый ввод /roll…” (см. `FEEDBACK.md`).
- **Action items**:
  - Добавить слой “output sanitizer” для Telegram: запрещённые паттерны (JSON/инструкции/tools) либо вынести в отдельный internal‑канал.
  - Добавить тесты, что ответы игроку не содержат служебных маркеров.
- **Как проверить**:
  - Интеграционные Telegram‑тесты (stubbed/real) + лог‑алерты при срабатывании sanitizer.

### P1 — потеря контекста из-за hard truncation промпта ✅ (решено)
- **Симптом (prod‑логи)**: `Game context truncated for prompt ... hard_truncated=true`, при этом вырезаются блоки вроде **персонажа игрока / истории / анализа**.
- **Решено**: Введена приоритизация блоков контекста с логированием удалённых блоков.
- **Метрики**: доля hard_truncated, какие блоки удаляются.
- **Как проверить**:
  - Логи `hard_truncated=false` для типичного геймплея; регресс‑тест на сохранение "must-have" блоков.

### P1 — невалидный JSON от LLM в `InitCampaign`/генерации мира ✅ (улучшено)
- **Симптом (prod‑логи + `TESTING_REPORT.md`)**: `LLM response ... is not valid JSON, attempting to repair` → `Failed to repair ...` (locations/connections/predefined checks).
- **Решено**: Более строгие JSON-контракты на ретраях, меньше repair/fallback.
- **Action items**:
  - Дальнейшее ужесточение контракта: схема/валидация, structured output, ограничение на длину.
- **Как проверить**:
  - `make test-telegram-real` (при наличии creds) + отсутствие repair/fallback в логах для InitCampaign.

### P1 — DM Analyzer: truncated JSON ответы от LLM ✅ (исправлено)
- **Симптом (prod‑логи)**: `{"combat_detected":false,"enemies":[],"quest_completed":false,"quest_failed":false,"quest_title":"","experience_gained":0,"experience_reason":"","items_received":[],"location_visited":{"name":"Сер�` - ответ обрывается.
- **Решено**: Добавлено раннее обнаружение truncated JSON + автоматический repair перед основной валидацией. Увеличен таймаут DM analyzer с 30s до 60s.
- **Action items**:
  - Мониторить логи на отсутствие "Raw LLM response" с обрезанными ответами.
- **Как проверить**:
  - В логах DM analyzer отсутствие truncated ответов; успешный анализ действий игроков.

### P1 — LocationEvent создаётся, но не попадает в следующий DM prompt (integration gap)
- **Симптом (`TESTING_REPORT.md`)**: событие есть в `world_events`, но его нет в следующем prompt; тест сейчас делает `t.Skip`.
- **Action items**:
  - Подключить `world_events` в контекст (history/RAG/контекст‑билдер), чтобы DM видел событие на следующем ходу.
  - Проиндексировать location events в RAG как StoryEvent или отдельный doc‑тип.
  - После фикса — перевести тест из `Skip` в assert (чтобы регресс ловился).
- **Как проверить**:
  - `tests/integration/telegram_location_event_simulation_test.go` должен стать PASS без `Skip`.

### P2 — изображения: 403 Permission denied при скачивании + таймауты генерации ✅ (исправлено)
- **Симптом (prod‑логи + `TESTING_REPORT.md`)**:
  - `gigachat image download error status 403: {"message":"Permission denied"}`
  - `Failed to generate world map image ... context deadline exceeded` (до исправления)
- **Решение**:
  - Добавлен консистентный X-Client-ID header при генерации и скачивании изображений (согласно GigaChat API docs).
  - Добавлена retry логика в `GenerateImage` через `doRequest` для обработки 429 ошибок.
  - Уменьшен concurrency limit GigaChat с 5 до 2 для снижения rate limiting.
  - Увеличен таймаут генерации изображений с 90s до 120s.
- **Action items**:
  - Мониторить прод-логи на отсутствие 403 и таймаутов после применения фикса.
  - Добавить явный feature-flag "images off" при системных 403, если проблема сохранится.
  - Уменьшить шум логов (один warn + счетчик), улучшить degrade: всегда текстовый fallback.
- **Как проверить**:
  - В прод‑логах нет повторяющихся 403/таймаутов; генерация карты не ломает `/newgame` UX.

### P2 — GigaChat rate limiting (429) влияет на latency/UX ✅ (улучшено)
- **Симптом (prod‑логи)**: `Rate limited (429), retry attempt ...`.
- **Решено**: Уменьшен concurrency limit с 5 до 2, добавлен jitter в backoff, улучшена retry логика с semaphore.
- **Action items**:
  - Мониторить метрики rate limiting; при необходимости добавить очередь/дедупликацию.
  - Метрики: RPS, 429 rate, p95 latency.
- **Как проверить**:
  - Падение доли 429 и tail latency в логах/мониторинге.

### P2 — RAG: провалы индексации/поиска → “провалы памяти”
- **Симптом (сигнал `FEEDBACK.md`)**: событие в БД есть, но в индексе/контексте нет (редкие таймауты/пропуски).
- **Action items**:
  - Логировать и алертить ошибки индексации (IndexDocument) + ретраи/очередь.
  - Добавить “graceful fallback”: если RAG пуст, подтягивать историю из БД (минимальный срез).
- **Как проверить**:
  - В логах нет “тихих” пропусков; интеграционные сценарии не теряют ключевые факты.

### P2 — DM Analyzer: пустые/битые поля (например, combat_detected=true, но enemy.name="")
- **Симптом (`TESTING_REPORT.md`)**: модель возвращает врага без имени → пайплайн пытается стартовать бой и падает внутри (“need at least 2 participants”).
- **Action items**:
  - Валидация анализа: минимальные требования для боя (имя/HP/сторона) + fallback (“combat_detected=false” или автозаполнение).
  - Тесты на “битого” врага/пустой анализ.
- **Как проверить**:
  - `tests/integration/telegram_real_llm_user_journey_test.go` без предупреждений про “Skipping enemy without name”.

### P2 — GigaChat auth: подозрение на неверную обработку `expires_in`
- **Симптом (сигнал `FEEDBACK.md`)**: риск некорректной ротации токена из-за единиц измерения `expires_in`.
- **Action items**:
  - Проверить контракт GigaChat auth и расчёт TTL; добавить тест на “почти истёкший токен”.
- **Как проверить**:
  - В прод‑логах нет внезапных auth‑ошибок/шторма обновления токенов.

### P2 — `/battlefield` продуктово нестабилен ✅ (решено)
- **Симптом**: игроки жалуются на нестабильность/непрозрачность (см. `FEEDBACK.md`).
- **Решено**: `/battlefield` сделан детерминированным выводом для дебага боя (без LLM).
- **Как проверить**:
  - `tests/integration/telegram_battlefield_test.go` + ручная проверка в Telegram.