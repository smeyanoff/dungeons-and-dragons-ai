# CODE_REVIEW — Dungeons & Dragons AI Bot (Telegram + AI DM + RAG)

**Последнее обновление:** 2026-01-22 (спринт приоритетов обновлен)
**Зачем файл:** держим **короткий** список **активных** рисков/дефектов + конкретные action items (что сделать / где / где проверить).
**Историю "что уже починили" здесь не храним** — только актуальное.

---

## ✅ Состояние (на 2026-01-22)

- **Тесты**: `go test ./...` ✅ PASS; интеграционные прогоны дают сигналы/проблемы (см. `TESTING_REPORT.md`, `FEEDBACK.md`).
- **Docker (prod)**: `dnd-bot-prod`, `dnd-postgres-prod`, `dnd-qdrant-prod` ✅ healthy.
- **Проблемы в логах**: truncated JSON от DM Analyzer, частые 429 rate limits, image generation timeouts.

---

## 🚨 Активные риски (P0-P2 приоритезация)

### P0 — Качество геймплея: analyzer-first проверки ✅ (завершено)
- **Статус**: Реализован и стабилен, перенесен в выполненные задачи
- **Как проверить**: `tests/integration/ability_check_guardrails_test.go` - отсутствие спама проверками

### P1 — DM Analyzer: truncated JSON ответы от LLM
- **Симптом (prod-логи)**: `{"combat_detected":false,"enemies":[],"quest_completed":false,"quest_failed":false,"quest_title":"","experience_gained":0,"experience_reason":"","items_received"` - JSON обрывается посередине ключа
- **Проблема**: `tryRepairTruncatedJSON` не справляется с восстановлением JSON, обрезанного посередине ключа
- **Action items**:
  - Улучшить логику `tryRepairTruncatedJSON` для обработки незавершенных ключей полей
  - Добавить валидацию на незавершенные JSON структуры до отправки в LLM
  - Увеличить maxRetries для DM analyzer
- **Как проверить**:
  - Логи DM analyzer без "truncated JSON" ошибок
  - Успешный парсинг всех ответов DM analyzer

### P1 — GigaChat rate limiting (429) влияет на UX
- **Симптом**: Частые `Rate limited (429), retry attempt ...` несмотря на concurrency limit = 3
- **Проблема**: Недостаточно агрессивный backoff или слишком высокая нагрузка
- **Action items**:
  - Дополнительно уменьшить concurrency limit до 2
  - Увеличить initial backoff delay до 5-10 секунд
  - Добавить метрики RPS и 429 rate для мониторинга
- **Как проверить**:
  - Снижение количества 429 ошибок в прод-логах

### P1 — Image generation: context deadline exceeded
- **Симптом**: `Failed to generate world map image ... context deadline exceeded` после retry
- **Проблема**: Таймаут 180s недостаточен для генерации изображений с rate limiting
- **Action items**:
  - Увеличить таймаут генерации изображений до 300s
  - Улучшить логику retry для image generation (меньше retry, но с большим таймаутом)
  - Добавить graceful fallback на текстовое описание при persistent failures
- **Как проверить**:
  - Успешная генерация изображений локаций/NPC в прод-логах

### P0 — Location events integration gap (повышен до P0)
- **Симптом**: Location events создаются но не попадают в следующий DM prompt
- **Action items**:
  - Подключить `world_events` в контекст DM (history/RAG/контекст-билдер)
  - Проиндексировать location events в RAG как StoryEvent
  - Перевести тест из `t.Skip` в полноценный assert
- **Как проверить**:
  - `tests/integration/telegram_location_event_simulation_test.go` PASS без Skip

### P2 — DM Analyzer: пустые/битые поля
- **Симптом**: Возвращает `combat_detected=true` но `enemy.name=""` или пустой JSON `{}`
- **Action items**:
  - Улучшить `validateCombatAnalysis` с более строгими проверками
  - Добавить fallback логику для битых врагов
  - Увеличить валидацию JSON схемы перед парсингом
- **Как проверить**:
  - Отсутствие "Skipping enemy without name" в логах

### P2 — RAG индексация: редкие провалы памяти
- **Симптом**: Событие в БД есть, но в индексе/контексте нет
- **Action items**:
  - Добавить логирование ошибок индексации с алертами
  - Реализовать graceful fallback на историю из БД при пустом RAG
  - Улучшить retry логику для IndexDocument
- **Как проверить**:
  - Отсутствие "тихих" пропусков в интеграционных тестах

### P2 — GigaChat auth: подозрение на expires_in обработку
- **Симптом**: Риск некорректной ротации токена из-за единиц измерения
- **Action items**:
  - Проверить расчет TTL токена и единицы `expires_in`
  - Добавить тест на "почти истекший токен"
- **Как проверить**:
  - Стабильность auth в прод-логах без внезапных ошибок

---

## 📊 Метрики для мониторинга
- DM Analyzer: доля успешных парсингов JSON (цель >95%)
- GigaChat: RPS, 429 rate (<5%), p95 latency (<30s)
- Images: успешная генерация (>80%), отсутствие 403 ошибок
- RAG: успешная индексация (>95%), отсутствие "провалов памяти"