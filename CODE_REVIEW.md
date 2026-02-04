# CODE_REVIEW — Dungeons & Dragons AI Bot

**Последнее обновление:** 2026-02-02
**Цель:** короткий список активных рисков + action items для команды.

---

## 🚨 P0 — Критичные проблемы

### 1. GigaChat TLS Certificate
**Симптом:** `tls: failed to verify certificate: x509: certificate signed by unknown authority`
**Влияние:** 100% LLM запросов fail → невозможно создать игру/персонажа

**Action items:**
- [x] Добавить `GIGACHAT_SKIP_TLS_VERIFY=true` или настроить правильные сертификаты
- [x] Добавить fallback на stub content при TLS failures для graceful degradation

**Проверка:** Успешные запросы к GigaChat API без TLS ошибок

---

### 2. Database Migrations
**Симптом:** `relation "session_goals" does not exist`
**Влияние:** Cooperative режим и сессионные цели недоступны

**Action items:**
- [x] Автоматизировать миграции в CI/CD pipeline
- [x] Добавить health check на старте: проверка существования всех таблиц

**Проверка:** Все таблицы существуют, `go run scripts/migrate.go` выполнен

---

### 3. Runtime Panics (nil pointer)
**Симптом:** `panic: runtime error: invalid memory address or nil pointer dereference`
**Влияние:** Crash в production, падение тестов cooperative mode

**Action items:**
- [x] Добавить nil checks в cooperative mode логике
- [x] Defensive programming: проверки на nil перед использованием session/player

**Проверка:** Все тесты проходят без panic

---

## ⚠️ P1 — Важные проблемы

### 4. DM Analyzer: Truncated JSON
**Симптом:** JSON обрывается (`"npc_met":n`), 6 retry + fallback
**Влияние:** Location events не генерируются, пропуск игровых триггеров

**Action items:**
- [x] Увеличить token limit analyzer (4096 → 8192)
- [x] Улучшить JSON repair/validation logic

**Проверка:** Analyzer возвращает valid JSON в >95% случаев

---

### 5. Combat Detection: False Negatives
**Симптом:** `combat_detected=false` при явных атаках ("гоблин атакует")
**Влияние:** Бой не стартует автоматически

**Action items:**
- [x] Расширить `detectsCombatMarker` русскими ключевыми словами (атакует, нападает, бьёт)
- [x] Улучшить prompt с примерами combat на русском

**Проверка:** Combat detection accuracy >95%

---

### 6. /battlefield нестабильность
**Симптом:** Команда не отображает информацию о бое или падает
**Влияние:** Нет прозрачности боевой системы

**Action items:**
- [x] Добавить defensive checks для отсутствующих данных боя
- [x] Логирование ошибок для диагностики

**Проверка:** `/battlefield` корректно показывает состояние боя

---

## 📊 Ключевой продуктовый сигнал

**DM спамит проверками** — пользователи ожидают редкие и значимые проверки.

**Статус:** ✅ Реализованы guardrails (budget/cooldown/anti-trivial + reason/stakes)

**Мониторить:** среднее количество проверок за сессию, негативный feedback

---

## ✅ Решено (архив)

| Проблема | Решение | Дата |
|----------|---------|------|
| DM спамит проверками | Guardrails: budget/cooldown/anti-trivial + reason/stakes | 2026-02 |
| GigaChat TLS Certificate | Skip TLS env + TLS fallback stub | 2026-02 |
| Database Migrations | Session goals + health checks | 2026-02 |
| Runtime Panics | Defensive nil checks in coop flow | 2026-02 |
| DM Analyzer: Truncated JSON | 8192 tokens + repair/validation | 2026-02 |
| Combat Detection: False Negatives | Expanded markers + prompt | 2026-02 |
| /battlefield нестабильность | Defensive checks + logging | 2026-02 |
| Прогресс персонажа | Визуализация роста и статистика сессий | 2026-01 |
| Утечка информации о ловушках | Analyzer-first + guardrails в промпт | 2026-01 |
| Автопродолжение после провала проверки | DM получает результат в контексте | 2026-01 |
| Блокировка после успешной проверки | Исправлена логика resolve pending check | 2026-01 |
| Частая генерация изображений | Rate limiting (3/сессию) | 2026-01 |
| JSON Stability Crisis | Улучшен repair + pre-validation | 2026-01 |
| LLM Context Deadline Exceeded | Увеличены timeouts (30s → 60s) | 2026-01 |
| GigaChat Function Calling | Тулзы в `functions` в `chat/completions` | 2026-01 |
| Telegram Polling EOF | Exponential backoff с jitter | 2026-01 |
| Image download 403 | X-Client-ID header + retry | 2026-01 |

---

## 📈 Метрики мониторинга

| Метрика | Цель | Критичность |
|---------|------|-------------|
| GigaChat TLS errors | 0 | P0 |
| Migration errors | 0 | P0 |
| Runtime panics | 0 | P0 |
| DM Analyzer valid JSON | >95% | P1 |
| Combat detection accuracy | >95% | P1 |
| Integration tests pass rate | >90% | P1 |
