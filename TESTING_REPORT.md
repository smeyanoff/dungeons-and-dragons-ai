# Отчет о тестировании D&D AI Bot

**Последний прогон:** 2026-02-03 (Telegram gameplay e2e, mock Telegram API; stubbed + real LLM)

## Как запускать

- **Stubbed (стабильно, без реального LLM/RAG)**: `make test-telegram-stub`
- **Real LLM (одна кампания, RAG включен)**:

```bash
set -a && source .env && set +a
go test -v -count=1 -timeout 60m ./tests/integration/... -run 'TestTelegramGameplay_RealLLM_SingleCampaign_ToFirstCombat'
```

Перезапуск контейнеров — только через `make deploy`.

## Итог (актуально)

- ✅ `TestTelegramGameplay_BotSimulation_UserJourney_StubbedLLM` — PASS
- ✅ `TestTelegramGameplay_RealLLM_SingleCampaign_ToFirstCombat` — PASS (~25s)
- ⚠️ Тесты, требующие GigaChat creds в env процесса, будут SKIP, если не подхватить `.env`.

## Одна тестовая кампания (покрытие)

Сценарий, который должен проходить «от появления в мире до первого боя в другой локации»:

- `/newgame <theme>` → проверка ответа DM на создание мира
- `/createcharacter <name race class>`
- **Действие игрока** → **player-facing prompt**: `🎲 Нужна проверка ... (DC ...)` + `/roll`
- `/roll d20` → pending ability check очищается
- `/map` → callback `map_to_*` → `CurrentLocationID` меняется (переход в другую локацию)
- **Первый бой в новой локации** → `/battlefield` → `/attack`

## Ключевые проверки (что именно валидируем)

- **UX ability check**: игрок видит подсказку с `DC` и `/roll` сразу после действия, без «тишины».
- **Состояние сессии**: pending check очищается после `/roll`.
- **Навигация**: callback из `/map` действительно переключает локацию.
- **Monitoring промптов/тулзов**: в `llm_logs` есть записи по `chat_id`, и среди них присутствуют запросы с tools (combat/inventory/etc). Проверка навыка — analyzer-first: `request_ability_check` не регистрируется как tool для DM и не должен встречаться в `ToolsCalls`; сигнал флоу — player-facing prompt "🎲 Нужна проверка ... DC ... /roll" сразу после действия игрока.

## Исправления, сделанные по итогам тестирования

1. **Cleanup интеграционных тестов (FK session_goals)** ✅
   - Исправлено удаление `session_goals`: колонка **`game_session_id`** (а не `session_id`).

2. **Ability check prompt в Telegram gameplay** ✅
   - При решении анализатора «нужна проверка» теперь **сразу возвращаем** игроку prompt `🎲 Нужна проверка ... (DC ...)` и откладываем дальнейшую генерацию DM-ответа до `/roll`.

## Удаленные/почищенные тесты (как битые/неактуальные)

- `tests/integration/telegram_session_goals_test.go` — panic / некорректная подготовка данных (не создавалась сессия).
- `tests/integration/telegram_tool_first_ability_check_flow_test.go` — дублировал сценарий и рассинхронизировался с текущим UX (после фикса покрытие обеспечено stubbed user journey).
- `tests/integration/location_event_payload_test.go` — ожидания по payload/metadata устарели (ложные FAIL).
- `tests/integration/telegram_location_events_test.go` — флейковый/ошибочный тест (ложные FAIL).

## История проблем (кратко)

Сводка по категориям по прошлым прогонам (2026-02-03):

- **LLM/сеть**: GigaChat 402 при генерации main quest (/newgame для сессионных целей и cooperative режима).
- **Команды/UX**: бот не ответил на /help; после /battlefield не найдено сообщение с «Поле боя»; после /map не найдены inline-кнопки навигации (map_to_*); инвентарь не отображает информацию; карта мира без кнопок навигации.
- **Состояние**: персонаж не создан после /createcharacter (сессионные цели / cooperative); pending ability check не очищен после /roll d20; первый игрок не создан в cooperative режиме; в llm_logs есть записи, но нет запросов с tools и не найден вызов request_ability_check.

---

**request_ability_check в llm_logs (закрыто 2026-07-23):** ранее тест ожидал tool-вызов
`request_ability_check` в логах (tool-first ability check). Текущий флоу — analyzer-first:
проверка решается отдельным LLM-анализом действия (`needs_ability_check` в JSON), а не
вызовом tool DM — `request_ability_check` структурно не регистрируется как tool для DM.
Тест `TestTelegramGameplay_RealLLM_SingleCampaign_ToFirstCombat` обновлён — сигналом флоу
служит player-facing prompt ability check (см. «Ключевые проверки» выше).

Для анализа конкретных запросов/ответов/тулзов по свежим прогонам используйте
`.claude/skills/read-dnd-bot-logs` (JSON API поверх `llm_logs`) вместо статичных дампов здесь.
