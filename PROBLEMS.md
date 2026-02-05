# Активные проблемы разработки

**Обновлено:** 2026-02-05  
Детали и action items — в **CODE_REVIEW.md**. Статусы задач — в **TASKS.md**.

---

## P0 — Критичные

*Нет активных.*

---

## P1 — Важные

Все пункты спринта по P1 выполнены (output-guard, навигация/visited, RAG fallback, 402 fallback, flaky tests, декомпозиция bot.go). Остаётся только мониторинг и опциональный рефакторинг (см. CODE_REVIEW.md).

| # | Проблема | Статус |
|---|----------|--------|
| 1 | Flaky tests (time.Sleep) | РЕШЕНА |
| 2 | Telegram Bot размер (bot.go) | РЕШЕНА (~696 LOC) |
| 3 | GigaChat 402 в cooperative | РЕШЕНА (fallback) |
| 4 | Утечки системного текста в DM | РЕШЕНА (output-guard) |
| 5 | Провалы RAG / потеря контекста | РЕШЕНА (fallback режим) |
| 6 | Навигация / visited | РЕШЕНА (pending clear, visited markers) |

---

## P2 — Улучшения (бэклог)

- Контроль медиа: реже, по запросу/событию, без автокарты по умолчанию.
- Мониторинг квот GigaChat и RAG-failures.

---

## Решено (архив)

| Проблема | Дата |
|----------|------|
| P0 Race Condition в Combat / Игнорирование ошибок / Утечка горутин | 2026-02 |
| P1 Clean Architecture / Базовые команды бота | 2026-02 |
| GigaChat TLS Certificate / Database Migrations / Runtime Panics (nil) | 2026-02 |
| DM Analyzer JSON / Combat Detection | 2026-02 |
| /battlefield нестабильность / Mini-events / DM дублирование | 2026-02 |
| GigaChat context deadline / Утечка ловушек / Автопродолжение после провала | 2026-01 |
| Telegram Polling EOF / Image download 403 / Частая генерация изображений | 2026-01 |
