# Отчет о тестировании D&D AI Bot

**Последнее обновление:** 2026-01-21

## ✅ Статус прогонов

- **`make test`** (=`go test ./...`): ✅ PASS (2026-01-21)  
- **`make test-integration`**: ✅ PASS (2026-01-21)  
  Примечание: LLM-зависимые тесты корректно **SKIP**, если не заданы `GIGACHAT_CLIENT_ID/GIGACHAT_CLIENT_SECRET`.
- **`make test-telegram-stub`**: ✅ PASS (2026-01-21)  
- **`make test-telegram-real`**: ✅ PASS (2026-01-21)  
- **`make test-integration-gameplay-docker`**: ⚠️ не запускалось в этом прогоне (требует запущенный `dnd-bot-prod`).

## 🧪 Добавлено/обновлено тестами

- `TestGetMapUseCase_CacheBehavior`: базовые проверки кэша карты (`getCachedMap`/`setCachedMap`).  
  Файл: `internal/game/application/worldmap/get_map_test.go`.
- `TestAnalyzeDMResponseUseCase_EmptyAnalysisRetriesThenUsesNonEmpty` и `...RetriesThenFallback`: ретраи DM Analyzer при пустом JSON, проверка fallback.  
  Файл: `internal/game/application/dm_analyzer/analyze_dm_response_test.go`.
- `TestTelegramGameplay_RealLLM_UserJourney_MainMechanics`: сквозной сценарий “как пользователь в Telegram” с реальным LLM для `/newgame`+действия игрока, плюс детерминированные проверки `/roll` (pending check) и `/battlefield`+`/attack`.  
  Файл: `tests/integration/telegram_real_llm_user_journey_test.go`.
- `TestTelegramBattlefieldCommand`: переведён на infra-only (больше не зависит от GigaChat creds), чтобы стабильно исполняться в `make test-integration`.  
  Файл: `tests/integration/telegram_battlefield_test.go`.

## 🧯 Проблемы, найденные при прогоне

1. **Location events не попадают в следующий DM prompt**  
   `TestTelegramGameplay_BotSimulation_LocationEvent_FirstVisit`: событие создано в `world_events`, но не найдено в следующем DM prompt.  
   Файл: `tests/integration/telegram_location_event_simulation_test.go`.

2. **GigaChat Image: download 403 (Permission denied)**  
   В `TestTelegramGameplay_CompleteFlow` генерация изображения локации завершается ошибкой скачивания: `gigachat image download error status 403: {"status":403,"message":"Permission denied"}`.  
   Это не падает тестом (есть fallback), но ломает “красивый UX” и шумит в логах.

3. **InitCampaign: невалидный JSON от LLM → repair/retry/fallback (connections/NPCs)**  
   В прогоне real LLM видно: `LLM response ... is not valid JSON, attempting to repair`, далее ретраи, а для `connections` — `Failed to generate valid connections, using fallback`.  
   Это может ухудшать качество мира и детерминизм (и потенциально ломать `/map` навигацию).

4. **DM Analyzer: combat_detected=true, но enemy.name пустой → бой не стартует**  
   В `TestTelegramGameplay_RealLLM_UserJourney_MainMechanics` анализ содержит врага с `name=""`, после чего лог: `Skipping enemy without name ...` и `failed to start combat: need at least 2 participants for combat`.  
   Это баг/дырка в контракте: модель может вернуть “битого” врага, а пайплайн стартует бой и падает внутри (хотя тест продолжает жить, т.к. бой в тесте создаётся детерминированно для проверки `/battlefield`).

## 📝 Примечания

- В интеграционном прогоне наблюдалась ошибка генерации изображения локации: `gigachat image download error status 403`.  
  Не ломает тесты напрямую, но шумит в логах.


## Проблемы, найденные при интеграционном тестировании (2026-01-21 21:31:54)

1. LocationEvent: событие локации создано (world_event_id=25, location_id=373, name="Встреча в Cave"), но не найдено в следующем DM prompt — вероятно, не подключено к контексту/RAG/истории

---

## Проблемы, найденные при интеграционном тестировании (2026-01-21 21:32:23)

1. LocationEvent: событие локации создано (world_event_id=27, location_id=386, name="Встреча в Cave"), но не найдено в следующем DM prompt — вероятно, не подключено к контексту/RAG/истории

---

## Проблемы, найденные при интеграционном тестировании (2026-01-21 21:32:49)

1. LocationEvent: событие локации создано (world_event_id=29, location_id=399, name="Встреча в Cave"), но не найдено в следующем DM prompt — вероятно, не подключено к контексту/RAG/истории

---

## Проблемы, найденные при интеграционном тестировании (2026-01-21 21:33:50)

1. LocationEvent: событие локации создано (world_event_id=31, location_id=413, name="Встреча в Cave"), но не найдено в следующем DM prompt — вероятно, не подключено к контексту/RAG/истории

---

## Проблемы, найденные при интеграционном тестировании (2026-01-21 21:35:00)

1. LocationEvent: событие локации создано (world_event_id=33, location_id=427, name="Загадка в Cave"), но не найдено в следующем DM prompt — вероятно, не подключено к контексту/RAG/истории

---
