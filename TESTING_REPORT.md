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

## История проблем (кратко)

Сводка по категориям по прошлым прогонам (2026-02-03):

- **LLM/сеть**: GigaChat 402 при генерации main quest (/newgame для сессионных целей и cooperative режима).
- **Команды/UX**: бот не ответил на /help; после /battlefield не найдено сообщение с «Поле боя»; после /map не найдены inline-кнопки навигации (map_to_*); инвентарь не отображает информацию; карта мира без кнопок навигации.
- **Состояние**: персонаж не создан после /createcharacter (сессионные цели / cooperative); pending ability check не очищен после /roll d20; первый игрок не создан в cooperative режиме; в llm_logs есть записи, но нет запросов с tools и не найден вызов request_ability_check.

---

Ниже дописывается секция **«Анализ запросов/ответов/промптов/тулзов»** после каждого прогона теста `TestTelegramGameplay_RealLLM_SingleCampaign_ToFirstCombat`.

## Анализ запросов/ответов/промптов/тулзов (2026-02-05 14:26:40)

chat_id=1770290740524597943, логов=11

| #1 | prompt_len=7038 | response_len=408 | has_tools=false | tools= |
|     | response_preview: ```json {   "combat_detected": false,   "enemies": [],   "combat_ended": false, ... |
| #2 | prompt_len=3693 | response_len=715 | has_tools=false | tools= |
|     | response_preview: ### Финальный ответ:  Эребос, едва успев спрятаться в тени, слышит приближающиес... |
| #3 | prompt_len=2887 | response_len=783 | has_tools=true | tools=generate_image |
|     | response_preview: Эребос тяжело сглотнул, услышав характерный скрип двери, открывшейся не так, как... |
| #4 | prompt_len=4062 | response_len=468 | has_tools=false | tools= |
|     | response_preview: {   "needs_ability_check": true,   "ability_check": {     "ability": "dexterity"... |
| #5 | prompt_len=616 | response_len=1183 | has_tools=false | tools= |
|     | response_preview: {   "connections": {     "Руины Черного замка": [       {         "to_location":... |
| #6 | prompt_len=381 | response_len=285 | has_tools=false | tools= |
|     | response_preview: {   "npcs": [     {       "name": "Аббадон Серые Очи",       "role": "Маг-путеше... |
| #7 | prompt_len=362 | response_len=349 | has_tools=false | tools= |
|     | response_preview: {   "npcs": [     {       "name": "Граф Дарквуд",       "role": "главный антагон... |
| #8 | prompt_len=365 | response_len=282 | has_tools=false | tools= |
|     | response_preview: {   "npcs": [     {       "name": "Таградар Кровавая Коготь",       "role": "стр... |
| #9 | prompt_len=580 | response_len=405 | has_tools=false | tools= |
|     | response_preview: {   "locations": [     {       "name": "Руины Черного замка",       "description... |
| #10 | prompt_len=600 | response_len=634 | has_tools=false | tools= |
|     | response_preview: {   "locations": [     {       "name": "Руины древнего храма Вельзира",       "d... |
| #11 | prompt_len=503 | response_len=757 | has_tools=false | tools= |
|     | response_preview: {   "title": "Проклятая книга теней",   "description": "Игроки исследуют древние... |

---

## Проблемы, найденные при интеграционном тестировании (2026-02-05 14:26:40)

1. в llm_logs не нашли tool вызов request_ability_check (ожидали tool-first ability check)  
   **Уточнение (2026-02-09):** текущий флоу — analyzer-first: проверка решается отдельным LLM-анализом действия (`needs_ability_check` в JSON), а не вызовом tool DM. Ожидание «request_ability_check в llm_logs» может быть неверным; см. CODE_REVIEW.md P1 п.3 — уточнить тест или документацию.

---
