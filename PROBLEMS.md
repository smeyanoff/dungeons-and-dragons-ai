# Активные проблемы разработки

**Обновлено:** 2026-02-09  
Детали и action items — в **CODE_REVIEW.md**. Статусы задач — в **TASKS.md**.

---

## P0 — Критичные

*Нет активных.*

---

## P1 — Важные (задачи по CODE_REVIEW и логам контейнера)

| # | Проблема | Статус |
|---|----------|--------|
| 1 | Combat analyzer: пустые враги в JSON при наличии боя в тексте DM → бой отключается | **Открыта** |
| 2 | RAG: found_docs=6, docs_added=0 — документы не попадают в контекст | **Открыта** |
| 3 | Тест request_ability_check: ожидание tool в llm_logs при analyzer-first флоу | **Открыта** (уточнить/ослабить тест или док) |
| 4 | Output-guard: маркеры 🎯 perform_* в ответе DM — проверить, что не уходят игроку | **Проверить** |

**Решённые ранее:** Flaky tests, размер bot.go, GigaChat 402, утечки системного текста, RAG fallback, навигация/visited — см. архив ниже.

---

## P2 — Улучшения (бэклог)

- Мониторинг квот GigaChat и RAG-failures (TASKS.md P2.7).
- Контроль медиа: реже, по запросу/событию (уже в спринте/архиве).

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
