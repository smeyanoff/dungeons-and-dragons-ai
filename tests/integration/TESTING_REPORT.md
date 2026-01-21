
## Проблемы, найденные при интеграционном тестировании (2026-01-19 20:07:01)

1. Не удалось создать игру: failed to generate main quest: LLM error: network error: Post "https://ngw.devices.sberbank.ru:9443/api/v2/oauth": tls: failed to verify certificate: x509: certificate signed by unknown authority
2. Не удалось создать персонажа: game session not found, use /newgame first
3. Не удалось получить ежедневные задания: game session not found, use /newgame first
4. Не удалось получить квесты: game session not found, use /newgame first

---

## Проблемы, найденные при интеграционном тестировании (2026-01-19 20:11:50)

1. Не удалось создать игру: failed to generate main quest: LLM error: network error: Post "https://ngw.devices.sberbank.ru:9443/api/v2/oauth": tls: failed to verify certificate: x509: certificate signed by unknown authority
2. Не удалось создать персонажа: game session not found, use /newgame first
3. Не удалось получить ежедневные задания: game session not found, use /newgame first
4. Не удалось получить квесты: game session not found, use /newgame first

---

## Проблемы, найденные при интеграционном тестировании (2026-01-19 20:14:17)

1. Не удалось создать игру: failed to generate main quest: LLM error: network error: Post "https://ngw.devices.sberbank.ru:9443/api/v2/oauth": tls: failed to verify certificate: x509: certificate signed by unknown authority
2. Не удалось создать персонажа: game session not found, use /newgame first
3. Не удалось получить ежедневные задания: game session not found, use /newgame first
4. Не удалось получить квесты: game session not found, use /newgame first

---

## Проблемы, найденные при интеграционном тестировании (2026-01-19 20:16:34)

1. Не удалось получить ежедневные задания: failed to get daily quests: ERROR: relation "daily_quests" does not exist (SQLSTATE 42P01)

---

## Проблемы, найденные при интеграционном тестировании (2026-01-19 20:20:00)

1. Не удалось создать игру: failed to generate main quest: LLM error: network error: Post "https://ngw.devices.sberbank.ru:9443/api/v2/oauth": tls: failed to verify certificate: x509: certificate signed by unknown authority
2. Не удалось обработать действие: failed to generate DM response: failed to generate response with tools: failed to generate initial response: network error: Post "https://ngw.devices.sberbank.ru:9443/api/v2/oauth": tls: failed to verify certificate: x509: certificate signed by unknown authority

---

## Проблемы, найденные при интеграционном тестировании (2026-01-19 20:26:40)

1. Ежедневные задания не содержат ожидаемых типов

---

## Проблемы, найденные при интеграционном тестировании (2026-01-19 20:26:51)

1. Список заклинаний не содержит ожидаемых элементов

---

## Проблемы, найденные при интеграционном тестировании (2026-01-20 23:50:19)

1. /newgame: ERROR: column "current_location_id" of relation "game_sessions" does not exist (SQLSTATE 42703)
2. После /newgame не найдена активная сессия: err=<nil>, session_nil=true
3. После /createcharacter не найден персонаж в сессии
4. После /map не удалось найти сообщение с inline-кнопками навигации (map_to_*)

---

## Проблемы, найденные при интеграционном тестировании (2026-01-20 23:50:43)

1. Не удалось сохранить сессию: ERROR: column "current_location_id" of relation "game_sessions" does not exist (SQLSTATE 42703)
2. Не удалось создать персонажа: game session not found, use /newgame first
3. Не удалось получить ежедневные задания: game session not found, use /newgame first
4. Не удалось получить квесты: game session not found, use /newgame first

---

## Проблемы, найденные при интеграционном тестировании (2026-01-21 01:23:31)

1. Не удалось получить ежедневные задания: character not created, use /createcharacter first

---

## Проблемы, найденные при интеграционном тестировании (2026-01-21 01:27:12)

1. Не удалось получить ежедневные задания: character not created, use /createcharacter first
2. Не удалось получить ежедневные задания после действия: character not created, use /createcharacter first
3. Не удалось получить ежедневные задания после боя: character not created, use /createcharacter first

---

## Проблемы, найденные при интеграционном тестировании (2026-01-21 01:28:03)

1. Не удалось получить ежедневные задания: character not created, use /createcharacter first

---

## Проблемы, найденные при интеграционном тестировании (2026-01-21 01:29:03)

1. Не удалось получить ежедневные задания: character not created, use /createcharacter first

---

## Проблемы, найденные при интеграционном тестировании (2026-01-21 14:28:34)

1. LocationEvent: событие локации создано (world_event_id=6, name="Находка в Cave"), но не найдено в следующем DM prompt (возможно, оно не попадает в контекст/историю/RAG)
2. LocationEvent: событие локации есть в world_events (id=6), но не найдено ни в одном StoryEvent (history) — игрок/DM могут его не увидеть

---

## Проблемы, найденные при интеграционном тестировании (2026-01-21 14:39:48)

1. После /map не удалось найти сообщение с inline-кнопками навигации (map_to_*)

---

## Проблемы, найденные при интеграционном тестировании (2026-01-21 14:41:23)

1. LocationEvent: событие локации создано (world_event_id=9, name="Встреча в Cave"), но не найдено в следующем DM prompt (возможно, оно не попадает в контекст/историю/RAG)
2. LocationEvent: событие локации есть в world_events (id=9), но не найдено ни в одном StoryEvent (history) — игрок/DM могут его не увидеть

---

## Результаты тестирования основных механик (2026-01-21 22:58:38)

### ✅ Stub-тесты (без реального LLM)
Все stub-тесты прошли успешно:
- TestTelegramGameplay_BotSimulation_AbilityCheckOneTap
- TestTelegramGameplay_BotSimulation_UserJourney
- TestTelegramGameplay_BotSimulation_LocationEvent_FirstVisit
- TestTelegramGameplay_BotSimulation_ToolFirstAbilityCheckFlow
- TestTelegramGameplay_BotSimulation_UserJourney_StubbedLLM

**Выявлено:** 0 проблем в stub-тестах

### ⚠️ Тесты с реальными ответами LLM
Тесты с реальными ответами LLM пропущены из-за отсутствия GigaChat credentials:
- TestTelegramGameplay_CompleteFlow
- TestTelegramGameplay_CombatFlow
- TestTelegramGameplay_RealLLM_PendingAbilityCheck_ManualAndRoll
- TestTelegramGameplay_RealLLM_UserJourney_MainMechanics
- TestTelegramGameplay_ComprehensiveUserJourney_StubbedLLM

**Причина:** GIGACHAT_CLIENT_ID и GIGACHAT_CLIENT_SECRET не установлены

### 📝 Создан новый тест
Написан TestTelegramGameplay_ComprehensiveUserJourney_StubbedLLM - комплексный тест, проверяющий полный пользовательский journey:
- /help
- /newgame (с реальным LLM)
- /createcharacter
- Игровые действия
- Pending ability checks (/roll d20)
- Боевая система (/battlefield, /attack)
- Просмотр всех команд (/inventory, /quests, /daily, /achievements, /spells, /map, /history)
- Проверка на утечки tool-текста
- /endgame

**Статус:** Требует GigaChat credentials для полного тестирования

---
