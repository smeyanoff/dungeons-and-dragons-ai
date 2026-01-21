# Отчет о тестировании D&D AI Bot

**Последнее обновление:** 2026-01-21

## ✅ Статус

- **`make test`** (=`go test ./...`): ✅ PASS
- **`make test-integration-gameplay`**: ✅ PASS  
  - LLM‑зависимые тесты **SKIP**, если не заданы `GIGACHAT_CLIENT_ID`/`GIGACHAT_CLIENT_SECRET`.

## 🧪 Добавлено/обновлено тестами

- `TestMaybeSendPendingAbilityCheckPrompt_SendsOnceAndMarksNotified`: отправка one‑tap prompt с inline‑кнопкой `ability_roll_*`, пометка `PendingAbilityCheckNotified`, без повторной отправки.  
  Файл: `internal/telegram/bot_pending_ability_check_test.go`.
- `TestTelegramGameplay_BotSimulation_ToolFirstAbilityCheckFlow`: полный UX “tool-first ability check” как в Telegram: сообщение игрока → `request_ability_check` → inline‑кнопка → callback → очистка pending + запись в историю + edit сообщения.  
  Файл: `tests/integration/telegram_tool_first_ability_check_flow_test.go`.

## ▶️ Как запускать тесты

```bash
make test
make test-integration
make test-integration-gameplay
```

## 🧯 Проблемы, найденные при интеграционном прогоне (2026-01-21)

1. **Location events не “доезжают” до DM/игрока**: событие локации создаётся в `world_events`, но не находится ни в следующем DM prompt, ни в `story_events` (history) — похоже, оно не попадает в контекст/историю/RAG.  
   Покрыто тестом: `tests/integration/telegram_location_event_simulation_test.go` (не падает, но пишет проблему).

2. **`/map` без inline-навигации**: после команды `/map` в bot-simulation не удалось найти сообщение с inline‑кнопками навигации (`map_to_*`).

3. **Qdrant client/server version warning в логах**: встречается `clientVersion=Unknown` и предупреждение о несовместимости версий (хотя подключение работает). Это шумит и может скрывать реальные проблемы.

## 🧪 Добавлено тестами (2026-01-21)

- `TestTelegramGameplay_BotSimulation_LocationEvent_FirstVisit`: проверка, что “first visit” может создавать событие локации, и сигнализация (в отчёт) если событие не видно в контексте/истории.  
  Файл: `tests/integration/telegram_location_event_simulation_test.go`.

## Проблемы, найденные при интеграционном тестировании (2026-01-21 14:43:55)

1. LocationEvent: событие локации создано (world_event_id=10, name="Встреча в Cave"), но не найдено в следующем DM prompt (возможно, оно не попадает в контекст/историю/RAG)
2. LocationEvent: событие локации есть в world_events (id=10), но не найдено ни в одном StoryEvent (history) — игрок/DM могут его не увидеть

---

## Проблемы, найденные при интеграционном тестировании (2026-01-21 15:07:53)

1. LocationEvent: событие локации создано (world_event_id=12, name="Находка в Cave"), но не найдено в следующем DM prompt (возможно, оно не попадает в контекст/историю/RAG)
2. LocationEvent: событие локации есть в world_events (id=12), но не найдено ни в одном StoryEvent (history) — игрок/DM могут его не увидеть

---

## Проблемы, найденные при интеграционном тестировании (2026-01-21 15:13:45)

1. LocationEvent: событие локации создано (world_event_id=14, name="Встреча в Cave"), но не найдено в следующем DM prompt (возможно, оно не попадает в контекст/историю/RAG)
2. LocationEvent: событие локации есть в world_events (id=14), но не найдено ни в одном StoryEvent (history) — игрок/DM могут его не увидеть

---

## Обновления автотестов и сигналы качества (2026-01-21)

1. **Добавлен стабильный Telegram gameplay e2e без реального LLM**: `TestTelegramGameplay_BotSimulation_UserJourney_StubbedLLM` (не требует `GIGACHAT_*`, не делает сетевых вызовов к LLM/RAG).  
   Цель: покрыть базовый “как пользователь в Telegram” сценарий: `/newgame` → `/createcharacter` → player action → one-tap ability check → `/map` → `/history` → `/endgame`.

2. **Включён rate limit для LLM вызовов в интеграционных тестах**: все реальные LLM обращения теперь автоматически ограничены (по умолчанию 2.5s между вызовами, настраивается `LLM_TEST_MIN_DELAY_MS`).  
   Цель: не DDOSить модель при полном прогоне `make test-integration`.

3. **Потенциальная проблема UX**: если в `/history` попадают tool‑маркеры (`request_ability_check`, `evaluate_check`, JSON/tool_call), это считается player-facing утечкой и должно быть исправлено (тесты это фейлят/репортят).

