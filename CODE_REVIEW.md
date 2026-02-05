# CODE_REVIEW — Dungeons & Dragons AI Bot

**Последнее обновление:** 2026-02-05
**Цель:** короткий список активных рисков + action items для команды.

**Статус:** P0 блокеров нет. P1 по стабильности/архитектуре закрыты (tests без `time.Sleep`, `bot.go` < 1500 LOC, output‑guard, RAG fallback, visited/навигация, 402 fallback). Остаются P2 улучшения: контроль медиа (реже, без автокарты), тонкая настройка частоты изображений.

---

## 🚨 P0 — Критичные проблемы

*Нет активных критических проблем*

---

## ⚠️ P1 — Важные проблемы

### 1. Остаточные флейковые тесты
**Симптом:** было — интеграционные тесты использовали `time.Sleep` для синхронизации  
**Влияние:** было — нестабильность CI/CD, ложные срабатывания
**Файлы:** 
- `tests/integration/*.go` (множественные использования)
- `internal/game/infrastructure/persistence/*_test.go` (unit тесты)

**Action items:**
- [x] Заменить `time.Sleep` в интеграционных тестах на proper synchronization
- [x] Добавить deterministic test helpers для unit тестов

**Проверка:** `go test ./...` проходит стабильно

---

### 2. Telegram Bot все еще большой
**Симптом:** было — `internal/telegram/bot.go` содержал тысячи строк кода  
**Влияние:** Сложность поддержки, тестирования, риск конфликтов при разработке
**Файл:** `internal/telegram/bot.go`

**Action items:**
- [x] Дальнейшее разделение bot.go на более мелкие модули (факт: `bot.go` ~700 LOC)
- [ ] Выделить middleware и utility функции (опционально, если снова начнёт разрастаться)
- [x] Проверить соответствие роутинга команд и списка распознаваемых команд (`isKnownCommand` vs `handleCommand`)

**Проверка:** Основной bot.go файл < 1500 строк

---

## 📊 Ключевые продуктовые сигналы

### Основные UX проблемы (из FEEDBACK.md):
- **Утечки internal/tool текста** в сообщения DM — игроки видят системные инструкции
- **Слишком много изображений** — медиа появляется "само по себе" слишком часто  
- **Провалы памяти** — таймауты/сбои в RAG приводят к потере контекста
- **Навигация блокируется проверками** — после перемещения запросы попадают под cooldown
- **Visited-состояние локаций** — нет отметки посещённых локаций для ориентации

### Что работает хорошо:
- ✅ **Guardrails для проверок** — budget/cooldown/anti-trivial + reason/stakes
- ✅ **Атмосферные описания** и связность мира
- ✅ **RAG помогает удерживать контекст** когда стабилен

**Мониторить:** утечки системного текста, частота генерации изображений, RAG failures

---

## 🧪 Новые проблемы из интеграционного тестирования

### GigaChat 402 ошибки в cooperative режиме
**Симптом:** `failed to generate main quest: gigachat error status 402`
**Влияние:** Блокирует создание новых игр в cooperative режиме
**Файлы:** Тесты cooperative режима

**Action items:**
- [ ] Исследовать лимиты/квоты GigaChat API и условия использования
- [x] Добавить fallback для случаев исчерпания лимитов (typed `PaymentRequiredError` + упрощённый режим без блокировки старта)
- [ ] Мониторинг использования API квот

---

## ⚠️ P1 — UX регрессии (влияет на игроков)

### Утечки системного текста *(UX КРИТИЧНО)*
**Проблема:** Игроки видят internal/tool инструкции в сообщениях DM
**Влияние:** Ломает immersion, показывает "кухню" системы
**Action items:**
- [x] Единый output‑guard перед отправкой в Telegram: жёсткая фильтрация internal/tool/JSON артефактов из финального текста DM
- [x] Логировать утечки, но не показывать игроку
- [x] Добавить validation/тесты на output DM перед отправкой

---

### Контроль медиа генерации *(UX УЛУЧШЕНИЕ)*
**Проблема:** Слишком много изображений генерируется "само по себе"
**Влияние:** Медиа появляется слишком часто, отвлекает от игры
**Action items:**
- [ ] Правила генерации/показа изображений (реже, только по событию/запросу)
- [ ] Лимиты на сессию для медиа контента
- [ ] Убрать автогенерацию карты как часть базового UX

---

### Навигационные улучшения *(UX УЛУЧШЕНИЕ)*
**Проблема:** Навигация блокируется проверками, нет visited-состояния локаций
**Action items:**
- [ ] Исключить навигационные запросы из cooldown/анти-повтор
- [ ] Добавить отметку посещённых локаций
- [ ] OOC/мета-запросы должны обрабатываться отдельно
- [ ] При перемещении закрывать `pending ability check` (иначе навигация “залипает” после смены локации)

---

## ✅ Решено (архив основных достижений)

| Проблема | Решение | Дата |
|----------|---------|------|
| **P0 Combat race condition** | Copy return + concurrent tests | 2026-02 |
| **P0 Ignored critical errors** | Log session/RAG failures + fallback paths | 2026-02 |
| **P0 Goroutine leaks** | Managed log worker + cache Close | 2026-02 |
| **P1 LLM architecture coupling** | Shared tool interfaces in llm domain | 2026-02 |
| **P1 Telegram bot monolith** | Split callbacks/commands/feedback/health/messages | 2026-02 |
| **P1 Basic command instability** | Integration test coverage + LLM fallback | 2026-02 |
| DM Analyzer: Truncated JSON | 8192 tokens + repair/validation | 2026-02 |
| Combat Detection: False Negatives | Expanded markers + prompt | 2026-02 |
| GigaChat TLS Certificate | Skip TLS env + TLS fallback stub | 2026-02 |
| Database Migrations | Session goals + health checks | 2026-02 |
| Runtime Panics (nil pointer) | Defensive nil checks в cooperative mode | 2026-02 |

---

## 📈 Метрики мониторинга

| Метрика | Текущее состояние | Цель | Критичность |
|---------|-------------------|------|-------------|
| P0 проблемы | 0 ✅ | 0 | P0 |
| P1 проблемы | 0 ✅ | 0 | P1 |
| Flaky tests (time.Sleep) | 0 ✅ | 0 | P1 |
| Large files (>1000 LOC) | bot.go: 696 ✅ | <3 | P1 |
| Integration tests pass rate | >95% ✅ | >95% | P1 |
| System text leaks в DM | 0 (guarded + tests) ✅ | 0 | P1 |
| GigaChat 402 errors | могут происходить, но есть fallback ✅ | <1% | P1 |

---

## 🔧 Технический долг (низкий приоритет)

- **Дублирование кода:** `truncateRunes` в 2 местах
- **Глобальное состояние:** `pkg/logger` глобальная переменная  
- **Неправильная структура pkg:** бизнес-логика в `pkg/gigachat/`
- **Отсутствие валидации:** Telegram callback data без проверок
- **Небезопасное TLS:** `InsecureSkipVerify` в production коде
