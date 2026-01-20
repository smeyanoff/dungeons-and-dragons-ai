# Отчет о тестировании D&D AI Bot

**Последнее обновление:** 2026-01-21

## ✅ Статус

- **`go test ./...`**: ✅ PASS (включая `tests/integration`)

## 🧪 Добавлено/обновлено тестами

### Проверки характеристик (P2.1/P2.2)

- `RequestAbilityCheckTool`: pending‑guard, cooldown, already_checked.
- `PerformAbilityCheckUseCase`: ошибки, успешный сценарий, индексирование в RAG, очистка pending.

### Telegram gameplay (как реальный пользователь)

- `TestTelegramGameplay_BotSimulation_AbilityCheckOneTap`: симуляция inline-кнопки `ability_roll_*` (one‑tap бросок) → событие в историю → pending очищен.

### Guardrails + payload событий локаций

- `TestAbilityCheck_Guardrails_*`: pending / already_checked / cooldown без участия LLM (детерминированно).
- `TestLocationEvent_PayloadMetadata_Structure`: проверка, что `world_events.metadata` хранит структурированный JSON (`hook/options/suggested_checks/stakes/status`).

Файлы:
- `tests/integration/telegram_ability_check_simulation_test.go`
- `tests/integration/ability_check_guardrails_test.go`
- `tests/integration/location_event_payload_test.go`

Файлы:
- `internal/game/application/dm_tools/character_tool_test.go`
- `internal/game/application/ability_check/perform_ability_check_test.go`

## 🐛 Исправления/обновления по результатам прогона

- `tests/integration/game_mechanics_test.go`:
  - включен `AutoMigrate` для `game_sessions` и `world_events` (устранено падение на volume со старой схемой: отсутствовал `world_events.metadata`);
  - генерация **уникального `chat_id`** на тест и `tgUserID == chatID` (приватный чат), иначе daily quests не находили персонажа;
  - обновлён вызов `player_action.NewHandleActionUseCase` под актуальную сигнатуру (добавлены `AnalyzePlayerActionUseCase` и `LocationEventGenerator`);
  - убран шумный `DELETE FROM game_sessions ...` в `TestCharacterCreation`, который давал FK-ошибки из-за `players`.
- `tests/integration/infra_only_test.go`: добавлен “infra‑only” setup (Postgres-only) для быстрых и детерминированных интеграционных проверок без LLM/Qdrant.

## ▶️ Как запускать тесты

```bash
go test ./...
```

Примечание: интеграционные тесты используют инфраструктуру проекта (Postgres/Qdrant) при наличии.

Альтернатива через Makefile:

```bash
make test-telegram
make test-integration
```
