# Активные проблемы разработки

**Обновлено:** 2026-07-25  
Детали и action items — в **CODE_REVIEW.md**. Статусы задач — в **TASKS.md**.

---

## P0 — Критичные

*Нет активных.*

---

## P1 — Важные (задачи по CODE_REVIEW и логам контейнера)

| # | Проблема | Статус |
|---|----------|--------|
| 1 | Combat analyzer: пустые враги в JSON при наличии боя в тексте DM → бой отключается | **Решена (см. TASKS.md P1.1, тест analyze_dm_response_test.go)** |
| 2 | RAG: found_docs=6, docs_added=0 — документы не попадают в контекст | **Решена** — в `qdrant.go` (`Search`) не был указан `WithPayload`, Qdrant по умолчанию не возвращает payload вместе с точками, поэтому весь текст документов приходил пустым. Добавлен `WithPayload: qdrant.NewWithPayload(true)` + метрика `rag_empty_result_count` (`/api/metrics`) на случай рецидива. |
| 3 | Тест request_ability_check: ожидание tool в llm_logs при analyzer-first флоу | **Решена** — тест `TestTelegramGameplay_RealLLM_SingleCampaign_ToFirstCombat` обновлён: убрана проверка на tool-вызов `request_ability_check` в `llm_logs` (структурно невозможна — тул не регистрируется для DM), целевой флоу (analyzer-first + player-facing prompt) уже проверялся отдельно и оставлен как единственный критерий. Документация (`TESTING_REPORT.md`, `tests/integration/README.md`) синхронизирована. |
| 4 | Output-guard: маркеры 🎯 perform_* в ответе DM — проверить, что не уходят игроку | **Решена** — в `sanitizeTelegramOutput`/`stripSuspiciousLines` (`internal/telegram/bot_messages.go`) не было защиты от голого упоминания имени tool'а в прозе (например, "🎯 perform_combat_attack"), только от `<tool_call>`/`<tool_result>`/JSON-структур. Добавлена проверка по списку реальных имён зарегистрированных tools; тесты в `bot_test.go`. |
| 5 | Telegram polling EOF: periodic `unexpected EOF` при getUpdates, растущий backoff → возможны окна, когда бот не читает апдейты | **Решена** — `configureHTTPClient`: `DisableKeepAlives: true` устраняет переиспользование протухшего keep-alive соединения после долгой обработки апдейтов между циклами polling; `CloseIdleConnections()` при сетевых ошибках — доп. страховка; счётчик `telegram_polling_error_count` в `/api/metrics` (см. `internal/telegram/bot.go`). |
| 6 | NPC-несостыковки: один и тот же NPC получал противоречивые факты между репликами DM (пример из игры, подтверждён по логам `llm_logs`: Тэсса — сначала явно "дочь старосты", несколько ходов спустя DM сам стал называть её "дочь пропавшего мастера" / "дочь кузнеца") — идентичность NPC не попадала в устойчивую память кампании, только в RAG/локальную историю | **Решена в 3 слоя** — (1) добавлена категория `npc_identity` в `CampaignFact` (`domain/world/campaign_fact.go`); (2) `save_campaign_fact` (`dm_tools/campaign_fact_tool.go`) явно просит DM сохранять идентичность NPC сразу при первом представлении; (3) `dm_analyzer` пассивно генерирует тот же факт из `npc_met`, если DM не вызвал инструмент сам. **Важный нюанс из логов:** LLM-анализатор в ~90% ответов не проставляет `npc_met.is_first_meeting` вовсе (уходит в `false` по умолчанию), из-за чего пассивная подстраховка (2)/(3) ненадёжна в проде. Поэтому добавлен четвёртый, самый надёжный слой: NPC с родством/принадлежностью (`NPCDTO.Relation`) теперь фиксируются как `npc_identity`-факты сразу при генерации мира (`InitCampaignUseCase.seedNPCIdentityFacts`, до первого хода игрока) — не зависит от поведения LLM в рантайме. |

**Решённые ранее:** Flaky tests, размер bot.go, GigaChat 402, утечки системного текста, RAG fallback, навигация/visited — см. архив ниже.

---

## P2 — Улучшения (бэклог)

- Контроль медиа: реже, по запросу/событию (уже в спринте/архиве).
- TASKS.md P2.1–P2.3: деградация RAG (частично уже покрыта fallback-веткой в `rag_context_builder.go`), контроль частоты проверок в проде, навигация/OOC без фрустрации — не тронуты в этом заходе, требуют отдельного разбора прод-логов.

---

## Решено (архив)

| Проблема | Дата |
|----------|------|
| RAG found_docs>0/docs_added=0 (отсутствовал WithPayload в Qdrant Search) / Analyzer-first vs request_ability_check тест / Output-guard: голые имена tools в прозе / Telegram polling EOF (keep-alive) | 2026-07-23 |
| P0 Race Condition в Combat / Игнорирование ошибок / Утечка горутин | 2026-02 |
| P1 Clean Architecture / Базовые команды бота | 2026-02 |
| GigaChat TLS Certificate / Database Migrations / Runtime Panics (nil) | 2026-02 |
| DM Analyzer JSON / Combat Detection | 2026-02 |
| /battlefield нестабильность / Mini-events / DM дублирование | 2026-02 |
| GigaChat context deadline / Утечка ловушек / Автопродолжение после провала | 2026-01 |
| Image download 403 / Частая генерация изображений | 2026-01 |
