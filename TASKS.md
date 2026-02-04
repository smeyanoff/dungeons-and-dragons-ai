# Задачи команды разработки (Sprint Backlog)

**Последнее обновление:** 2026-02-02
**Текущий спринт:** Февраль 2026 — исправление критических инфраструктурных проблем

**Правило приоритета:** P0 ошибки → P1 ошибки → P2 улучшения геймплея (монетизация не берем).

---

## 🎯 Цель спринта

- **Исправление критических блокеров**: GigaChat TLS, Database migrations, Runtime panics
- **Стабилизация core механик**: DM Analyzer JSON, Combat detection
- **Улучшение тестовой инфраструктуры**

---

## 🚨 P0 — Критические ошибки (блокеры)

### 1. GigaChat TLS Certificate *(БЛОКИРУЕТ ВСЕ LLM)*
**Симптом:** `tls: failed to verify certificate: x509: certificate signed by unknown authority`
**Влияние:** 100% LLM запросов fail → невозможно создать игру

**Action items:**
- [x] Добавить `GIGACHAT_SKIP_TLS_VERIFY=true` или настроить сертификаты
- [x] Добавить fallback на stub content при TLS failures

**Проверка:** Успешные запросы к GigaChat API без TLS ошибок

---

### 2. Database Migration Failure *(БЛОКИРУЕТ COOPERATIVE)*
**Симптом:** `relation "session_goals" does not exist`
**Влияние:** Cooperative режим и сессионные цели недоступны

**Action items:**
- [x] Выполнить `go run scripts/migrate.go` для pending миграций
- [x] Добавить database health checks при старте приложения
- [x] Автоматизировать миграции в CI/CD pipeline

**Проверка:** Все таблицы существуют, cooperative режим работает

---

### 3. Runtime Panics: Nil Pointer *(CRASH В PRODUCTION)*
**Симптом:** `panic: runtime error: invalid memory address or nil pointer dereference`
**Влияние:** Тесты падают, возможные crashes в production

**Action items:**
- [x] Добавить nil checks в cooperative mode логике
- [x] Defensive programming для session/player initialization
- [x] Улучшить error handling для missing entities

**Проверка:** Все тесты проходят без runtime panics

---

### 4. DM Analyzer: JSON Parsing *(ВЛИЯЕТ НА ГЕЙМПЛЕЙ)*
**Симптом:** JSON обрывается (`"npc_met":n`), 6 retry + fallback
**Влияние:** Location events не генерируются, пропуск триггеров

**Action items:**
- [x] Увеличить token limit analyzer (4096 → 8192)
- [x] Улучшить JSON validation и repair logic
- [x] Оптимизировать prompt для complete responses

**Проверка:** Analyzer возвращает valid JSON в >95% случаев

---

### 5. Combat Detection: False Negatives *(ВЛИЯЕТ НА БОЙ)*
**Симптом:** `combat_detected=false` при явных атаках ("гоблин атакует")
**Влияние:** Бой не начинается автоматически

**Action items:**
- [x] Расширить `detectsCombatMarker` русскими ключевыми словами (атакует, нападает, бьёт)
- [x] Улучшить prompt с примерами combat на русском
- [x] Добавить fallback enemy extraction

**Проверка:** Combat detection accuracy >95%

---

## ⚠️ P1 — Стабилизация инфраструктуры

### 6. Test Infrastructure: Database Isolation
**Симптом:** `record not found` при поиске sessions/players в тестах
**Влияние:** Интеграционные тесты не работают корректно

**Action items:**
- [x] Улучшить test data setup и cleanup
- [x] Добавить transaction isolation
- [x] Реализовать test database fixtures

**Проверка:** Все integration тесты проходят

---

### 7. /battlefield нестабильность
**Симптом:** Команда не отображает информацию о бое или падает
**Влияние:** Нет прозрачности боевой системы

**Action items:**
- [x] Добавить defensive checks для отсутствующих данных боя
- [x] Логирование ошибок для диагностики

**Проверка:** `/battlefield` корректно показывает состояние боя

---

### 8. DM Analyzer: Улучшение Prompt
**Симптом:** Недостаточно качественные JSON ответы
**Влияние:** Увеличивает retry и fallback случаи

**Action items:**
- [x] Оптимизировать JSON prompt для DM Analyzer
- [x] Фикс GigaChat token expiry (единицы измерения)
- [x] Валидация полей (защита от пустых enemies при combat_detected=true)

**Проверка:** Стабильные полные JSON ответы

---

## 📋 P2 — Игровые улучшения (backlog)

### Гибридный бой
- Авто + ручные решения в бою
- Динамический выбор тактики

### Динамические квесты
- Квесты реагируют на действия игрока
- Ветвление сюжета

### Система репутации NPC
- Отношения с NPC влияют на квесты и диалоги

---

## ✅ Завершено в текущем спринте

| Задача | Результат | Дата |
|--------|-----------|------|
| GigaChat Function Calling | Тулзы в `functions` в запросе | 2026-01 |
| Cooldown система проверок | Guardrails: budget/cooldown/anti-trivial | 2026-01 |
| Прогресс персонажа | Визуализация роста и статистика | 2026-01 |
| JSON Stability Crisis | Улучшен repair + pre-validation | 2026-01 |
| LLM Context Deadline | Увеличены timeouts (30s → 60s) | 2026-01 |
| Image download 403 | X-Client-ID header + retry | 2026-01 |
| GigaChat TLS Certificate | Skip TLS env + TLS fallback stub | 2026-02 |
| Database migrations | Session goals table + health checks | 2026-02 |
| Runtime panics | Defensive nil checks in coop flow | 2026-02 |
| DM Analyzer JSON parsing | 8192 tokens + JSON repair/prompt | 2026-02 |
| Combat detection | Expanded markers + prompt + fallback | 2026-02 |
| Test infra DB isolation | Migrations + cleanup | 2026-02 |
| /battlefield stability | Defensive checks + logging | 2026-02 |
| DM Analyzer prompt quality | Schema validation + token expiry fix | 2026-02 |

---

## 📈 Ключевые метрики

| Метрика | Цель | Критичность |
|---------|------|-------------|
| GigaChat TLS errors | 0 | P0 |
| Migration errors | 0 | P0 |
| Runtime panics | 0 | P0 |
| DM Analyzer valid JSON | >95% | P1 |
| Combat detection accuracy | >95% | P1 |
| Checks per 100 messages | <5 | P1 |

---

## 📝 Workflow обновления задач

1. Взяли задачу → отметить **⏳ In Progress**, добавить исполнителя
2. Завершили → перенести в "Завершено", обновить CODE_REVIEW.md и PRODUCT_IDEAS.md
