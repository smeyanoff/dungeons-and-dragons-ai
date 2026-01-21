# Задачи команды разработки (Sprint Backlog)

**Последнее обновление:** 2026-01-21
**Завершенный спринт:** Январь 2026 — analyzer-first проверки (без участия DM), стабилизация JSON/контекста/RAG ✅
**Текущий спринт:** Февраль 2026 — новый функционал и оптимизации

**Правило приоритета:** P0 баги → P1 риски → P2 улучшения.
**Главный фокус:** уйти от запроса проверок DM-ом, вынести в tools и анализатор DM-а.

---

## 🎯 Цель спринта

- **Analyzer-first ability checks**: анализатор решает "нужна ли проверка" и создаёт pending check **без участия LLM-DM**.
- **Text-only UX**: игрок взаимодействует с проверками только сообщениями (`/roll`), без кнопок и без "запросов" от DM.
- **Стабилизация**: меньше repair/fallback, утечек текста, потери контекста.

---

## ✅ Выполненные задачи

### P0 — Критично для игрового баланса ✅

- **Analyzer-first checks**: ✅ Перенести решение о проверках из LLM-DM в код (tools/анализатор). DM только ведёт художественный текст.
- **Guardrails против спама**: ✅ budget/cooldown/anti-trivial + обязательные reason/stakes для каждой проверки.
- **DM не просит /roll**: ✅ никаких "кинь проверку", "нужен бросок" в тексте сцен. Только pending check → `/roll`.

### P1 — Качество и стабильность ✅

- **Output sanitizer**: ✅ Убрать утечки tool-текста/JSON/инструкций из player-facing сообщений.
- **Context truncation fix**: ✅ Приоритизация блоков (pin персонаж/локация/бой/квест) + summarization вместо удаления.
- **JSON contracts**: ✅ Ужесточить валидацию для InitCampaign, меньше repair/retry/fallback.
- **Location events integration**: ✅ Подключить world_events в DM prompt (history/RAG), убрать t.Skip из теста.

### P2 — Улучшения инфраструктуры ✅

- **Image downloads**: ✅ Исправить 403 Permission denied (X-Client-ID header + retry логика).
- **Rate limiting**: ✅ Оптимизировать GigaChat (concurrency limits + jitter-backoff + метрики).
- **RAG reliability**: ✅ Логирование ошибок индексации + graceful fallback на историю из БД.
- **DM Analyzer validation**: ✅ Валидация анализа боя (имя/HP/сторона) + fallback на "combat_detected=false".
- **Battlefield stability**: ✅ Детерминированный вывод без LLM, стабильный формат для дебага.

---

## 📝 Правило обновления задач

- Взяли задачу → **⏳ In Progress**, исполнитель, ссылка на PR.
- Завершили → удалить из списка, обновить `CODE_REVIEW.md` (снять риск) и `PRODUCT_IDEAS.md` (документировать UX).
