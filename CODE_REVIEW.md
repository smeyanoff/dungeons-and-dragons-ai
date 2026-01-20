# CODE_REVIEW — Dungeons & Dragons AI Bot (Telegram + AI DM + RAG)

**Последнее обновление:** 2026-01-21  
**Цель файла:** короткий список **актуальных** технических рисков/дефектов + конкретные action items для команды.

---

## ✅ Текущее состояние (по факту логов/тестов)

- **Unit suite**: зелёная (`make test`), включая `-race` (см. `TESTING_REPORT.md`).
- **Интеграционные тесты** (`tests/integration`): требуют Postgres+Qdrant; по логам есть **нестабильности**/падения (см. `test_results.log`).
- **Docker compose (dev)**: сейчас **не в “здоровом” состоянии**:
  - `dnd-qdrant`: **unhealthy**, при этом процесс стартует и слушает 6333/6334.
  - `dnd-bot`: **crash-loop / restarting** (фатальная ошибка при инициализации Telegram Bot, token пустой).

---

## 🔴 P0 — Блокеры (чинить в первую очередь)

### 1) Docker: Qdrant помечается `unhealthy`, ломает `depends_on: service_healthy`
- **Симптом:** `dnd-qdrant` = `unhealthy`, из-за чего цепочка `depends_on` может ломать старт `bot`.
- **Причина (вероятно):** dev healthcheck дергает `http://localhost:6333/health` (см. `build/docker-compose.yml`), но для `qdrant/qdrant:v1.16.2` endpoint может отличаться (или требовать другой путь/метод), из‑за чего сервис “жив”, но `unhealthy`.
- **Что сделать:**
  - Починить dev healthcheck (валидный endpoint для `qdrant/qdrant:v1.16.2`) или сменить стратегию readiness (порт + простой запрос).
  - В `build/docker-compose.prod.yml` healthcheck qdrant сейчас “подозрительный” (фактически может всегда быть OK из-за `|| exit 0`) — привести к честной проверке (фейлится, если qdrant мертв/порт закрыт).
- **Источник:** `TESTING_REPORT.md` + docker compose `ps/logs` (2026‑01‑21).

### 2) Docker: `bot` падает при пустом `TELEGRAM_BOT_TOKEN`
- **Симптом:** `Failed to create bot: Not Found`, контейнер `dnd-bot` постоянно рестартится; compose предупреждает про пустые env.
- **Наблюдение по коду:** `cmd/bot/main.go` делает fail-fast при пустом токене; в docker это превращается в restart-loop при `restart: unless-stopped/always`.
- **Что сделать:**
  - Явная валидация env на старте + понятное сообщение в логах и завершение **без crash-loop** (или выключение сервиса через profile, если нет токена).
  - Обновить документацию запуска (что без токена сервис не стартует).
- **Источник:** docker compose `logs` (2026‑01‑21).

### 3) Интеграционные тесты: `duplicate key` по `game_sessions.chat_id`
- **Симптом:** `duplicate key value violates unique constraint "idx_game_sessions_chat_id"` → падают сценарии создания игры/персонажа.
- **Что сделать:**
  - Сделать тесты изолированными: **уникальные `chat_id` на каждый тест-кейс** (вместо одного константного), либо очищать БД через `TRUNCATE ... CASCADE`, либо поднимать чистую БД/схему на прогон.
  - Проверить `Delete()` и каскады/связанные таблицы (чтобы cleanup действительно убирал данные).
- **Источник:** `test_results.log`.

---

## 🟠 P1 — Высокий приоритет (качество, стабильность, UX)

### 1) GigaChat token expiry выглядит сломанным (сек/мс)
- **Симптом:** логи вида `expires in 1767... seconds (at year 58022...)`.
- **Гипотеза:** API отдаёт `expires_at` в **миллисекундах**, а код трактует как seconds (`time.Unix(token.ExpiresAt, 0)`).
- **Что сделать:** нормализовать `expires_at` (ms→s) и добавить sanity-check (если дата слишком далеко — считать неверной и править).
- **Источник:** `FEEDBACK.md`, `test_results.log`, код `pkg/gigachat/auth.go`.

### 2) LLM нестабилен по JSON → флейки/падения генерации кампании
- **Симптом:** “not valid JSON”, repair иногда не спасает, `failed to generate valid ... after 2 attempts`.
- **Что сделать:**
  - Усилить контракт: строгие схемы, запрет markdown code fences, больше ретраев/repair, fallback на детерминированный мир там, где возможно.
  - Вынести критичные генерации (world/locations/connections) в более устойчивый пайплайн.
- **Источник:** `test_results.log`, `TESTING_REPORT.md`, `FEEDBACK.md`.

### 3) DM Analyzer часто возвращает `{}` или обрывает JSON
- **Симптом:** `[DM Analyzer] Raw LLM response ...: {}` / ответы с ```json и незакрытыми полями.
- **Что сделать:** привести ответ анализатора к строгому JSON (без markdown), добавить валидацию и fallback-парсер/repair именно для анализатора.
- **Источник:** `test_results.log`, `FEEDBACK.md`.

### 4) Игровой UX: “технические” сообщения tools видны игроку
- **Что сделать:** санитизация финального ответа DM + разделение “internal/tool output” vs “player-facing narrative”.
- **Источник:** `FEEDBACK.md`.

### 5) `/battlefield` нестабилен (по фидбеку)
- **Что сделать:** завести воспроизводимый сценарий/тест, проверить обработчики команды и состояния боя.
- **Источник:** `FEEDBACK.md`.

---

## 🟡 P2 — Средний приоритет (техдолг/операционка)

### 1) Postgres в docker логах шумит `database "dnd_user" does not exist`
- **Причина:** `pg_isready -U dnd_user` без `-d` по умолчанию проверяет БД с именем пользователя.
- **Что сделать:** в dev healthcheck указать `-d ${POSTGRES_DB:-dnd}` (и/или отдельного пользователя/БД). В prod compose это уже выглядит исправленным.
- **Источник:** docker compose `logs` (2026‑01‑21).

### 2) Qdrant client/server version warning (clientVersion=Unknown)
- **Риск:** потенциальная несовместимость клиента/сервера или некорректная детекция версии.
- **Что сделать:** обновить клиент, либо осознанно настроить `SkipCompatibilityCheck=true` (если приемлемо) + зафиксировать версии.
- **Источник:** `test_results.log`.

### 3) `PROBLEMS.md` сейчас пустой
- **Что сделать:** либо заполнить актуальными infra/security пунктами, либо убрать из “обязательных источников” (чтобы не вводил в заблуждение).

---

## 📎 Источники (что было проверено)

- `TESTING_REPORT.md` (обновление 2026‑01‑20, + заметки 2026‑01‑21)
- `FEEDBACK.md` (Январь 2026)
- `test_results.log` (интеграционный прогон с ошибками)
- Docker compose `ps/logs` (2026‑01‑21)
