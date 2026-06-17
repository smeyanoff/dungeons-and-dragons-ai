# CODE_REVIEW — D&D AI Bot

**Обновлено:** 2026-02-09  
**Назначение:** активные риски и задачи для команды. Детали решённого — в TASKS.md и PROBLEMS.md.

---

## Статус

- **P0:** нет блокеров.
- **P1:** спринт 2026-02 доставлен (OOC, контроль медиа, combat guard). См. TASKS.md архив.
- **В бэклоге:** TASKS.md P2 доставлен (2026-02-09): playbook, пояснимость проверок, лимит/события медиа, резюме сессии, метрика checks per 100 msg, мониторинг.

---

## P0 — Критичные

*Нет активных.*

---

## P1 — Остаточное

- **Опционально:** при росте `internal/telegram/bot.go` (>1000 LOC) вынести middleware/утилиты.
- **Мониторинг:** доставлен P2.7 (счётчики 402 в GigaChat GetMetrics, RAG-failures и output-leak в internal/metrics, /api/metrics).

---

## P2 — Бэклог

P1 доставлен (2026-02-09): OOC, контроль медиа, combat guard. Остальное в **TASKS.md** P2.

| Область | Статус |
|---------|--------|
| **Медиа** | P1.2 + P2.3, P2.4 доставлены (лимит на сессию, мягкое сообщение, значимое событие: локация/NPC/конец боя) |
| **Combat guard** | P1.3 доставлен (fallback при пустом списке врагов) |
| **OOC** | P1.1 доставлен (isOOCMetaQuery, ответ без cooldown/проверок) |
| **Навигация** | Уже учтено (вне cooldown). |

**Из FEEDBACK.md:** сохранять атмосферные описания; проверки — редкие и со stakes; OOC-запросы обрабатывать отдельно.

---

## Решено (архив)

| Проблема | Решение |
|----------|---------|
| P0 Combat race / goroutine leaks | Copy return, fallback, managed worker |
| P1 LLM coupling / flaky tests | Shared tools, split bot, sync без Sleep |
| Утечки DM / RAG / 402 / visited | Output-guard, RAG fallback, PaymentRequiredError, visited, clear pending on move |
| DM Analyzer JSON, Combat, TLS, nil panic | 8192 tokens + repair; markers; skip TLS env; nil checks |

---

## Метрики

| Метрика | Цель | Сейчас |
|---------|------|--------|
| P0/P1 активные | 0 | 0 ✅ |
| bot.go LOC | <1500 | ~696 ✅ |
| Flaky tests | 0 | 0 ✅ |
| System text leaks | 0 | guarded + tests ✅ |
| GigaChat 402 | fallback | typed error + упрощённый режим ✅ |

---

## Техдолг (низкий приоритет)

- Дублирование: `truncateRunes` в двух местах.
- Глобальное состояние: `pkg/logger`.
- Бизнес-логика в `pkg/gigachat/`.
- Telegram callback data без валидации.
- `InsecureSkipVerify` в production.
