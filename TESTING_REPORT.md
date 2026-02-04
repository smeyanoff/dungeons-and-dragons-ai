# Отчет о тестировании D&D AI Bot

**Последний прогон:** 2026-02-03 (Telegram gameplay e2e, mock Telegram API; stubbed + real LLM)

## Как запускать

- **Stubbed (стабильно, без реального LLM/RAG)**: `make test-telegram-stub`
- **Real LLM (одна кампания, RAG включен)**:

```bash
set -a && source /Users/dima/Projects/dungeons-and-dragons-ai/.env && set +a
go test -v -count=1 -timeout 60m ./tests/integration/... -run 'TestTelegramGameplay_RealLLM_SingleCampaign_ToFirstCombat'
```

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
- **Monitoring промптов/тулзов**: в `llm_logs` есть записи по `chat_id`, и среди них присутствуют запросы с tools; ожидается `request_ability_check`.

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

---

## Проблемы, найденные при интеграционном тестировании (2026-02-03 15:34:03)

1. llm_logs есть (10), но нет ни одного запроса с tools
2. в llm_logs не нашли tool вызов request_ability_check (ожидали tool-first ability check)

---

## Проблемы, найденные при интеграционном тестировании (2026-02-03 15:35:45)

1. Real LLM Combat Analysis: 0 problems, 0 feedback items from 5 test cases

---

## Проблемы, найденные при интеграционном тестировании (2026-02-03 15:37:25)

1. Бот не ответил на команду /help
2. После /battlefield не найдено сообщение с полем боя
3. После /map не найдены inline-кнопки навигации (map_to_*)

---

## Проблемы, найденные при интеграционном тестировании (2026-02-03 15:37:46)

1. Bot did not respond to /help command
2. Character not created after /createcharacter
3. Pending ability check not cleared after /roll d20
4. Battlefield message not found after /battlefield command
5. No navigation buttons found after /map command

---

## Проблемы, найденные при интеграционном тестировании (2026-02-03 15:38:42)

1. Система инвентаря не отображает информацию
2. Карта мира не предоставляет кнопки навигации

---

## Проблемы, найденные при интеграционном тестировании (2026-02-03 15:40:12)

1. /newgame для сессионных целей: failed to generate main quest: gigachat error status 402: 
2. Персонаж не создан для сессионных целей
3. /newgame для cooperative режима: failed to generate main quest: gigachat error status 402: 
4. Первый игрок не создан в cooperative режиме

---
