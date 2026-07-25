---
name: read-dnd-bot-logs
description: Read, query, and filter dnd-bot's LLM request/response logs (prompt/response, tokens, duration, errors, per-session "branches", DM stats) via the built-in monitoring HTTP API (internal/monitoring/server.go). Use for "read the logs", "check LLM errors", "show recent DM requests", "what did GigaChat answer for chat X", "monitoring stats", "check output-leak/RAG-failure counters".
---

Paths below are relative to the repo root (`dungeons-and-dragons-ai/`).

The running `dnd-bot` process (dev or prod) exposes a separate, unauthenticated
HTTP API for LLM monitoring — every call the bot makes to GigaChat (DM
responses, tool calls, the `dm_analyzer` structural-analysis calls) is
persisted to the `llm_logs` table and readable through this API. It is a
**second HTTP server** on its own port, distinct from the bot's own
`:8080/health` — see `internal/monitoring/server.go` and
`cmd/bot/main.go:501-517`.

The driver is `.claude/skills/read-dnd-bot-logs/logs.sh`, a `curl`+`jq`
wrapper around the JSON API. All commands below were run against a live
local `dnd-bot-prod` deployment (docker-compose.prod.yml) in this session.

## Prerequisites

- `curl`, `jq`, `python3` (used only to URL-encode `from`/`to` timestamps).
- The bot must already be running somewhere reachable. It almost always
  already is — check first:

```bash
docker ps --format '{{.Names}}\t{{.Status}}\t{{.Ports}}' | grep dnd
# dnd-bot-prod        Up 13 minutes (healthy)   0.0.0.0:8081->8081/tcp
# dnd-postgres-prod    Up 13 minutes (healthy)   0.0.0.0:5432->5432/tcp
# dnd-qdrant-prod      Up 13 minutes (healthy)   0.0.0.0:6334->6333/tcp, ...
```

If nothing is running, start it the normal project way (see CLAUDE.md /
Makefile — not re-verified in this session): `make deploy` for the prod
stack, or `make docker-up && go run ./cmd/bot` for a local dev run (needs
`TELEGRAM_BOT_TOKEN` + `GIGACHAT_CLIENT_ID/SECRET` in `.env`; the bot logs
"Application will run in monitoring-only mode" and keeps the monitoring
server up even if the Telegram token is invalid/missing — see
`cmd/bot/main.go:547-556`). The monitoring port is `MONITORING_PORT`
(default `8081`, see `.env.example`).

## Run (agent path)

```bash
DND_MONITORING_URL=http://localhost:8081  # default; export to override
.claude/skills/read-dnd-bot-logs/logs.sh <command> [args]
```

Commands (all confirmed working against the live endpoint):

| Command | What it does |
|---|---|
| `stats [FROM] [TO]` | Aggregate counters for a time window (`FROM`/`TO` are RFC3339; default: last 7 days) |
| `recent [LIMIT=100]` | Most recent logs, newest first (server clamps `LIMIT` silently to `[1,1000]`) |
| `chat CHAT_ID [LIMIT]` | Logs for one Telegram `chat_id` |
| `user TG_USER_ID [LIMIT]` | Logs for one Telegram `tg_user_id` |
| `session SESSION_ID [LIMIT]` | Logs for one `game_session_id` |
| `errors [LIMIT=1000]` | Logs with a non-empty `error` field (client-side filter — see Gotchas) |
| `branches [CHAT_ID] [LIMIT=100]` | Per-session rollups: request/token/error counts, first/last seen |
| `show LOG_ID` | Full record for one log id, including the complete prompt and response text |
| `metrics` | Counters: `rag_failure_count`, `rag_empty_result_count`, `output_leak_count`, `telegram_polling_error_count` |

Example run against the live deployment (data present at the time):

```bash
$ .claude/skills/read-dnd-bot-logs/logs.sh stats
{
  "total_requests": 25,
  "total_errors": 0,
  "average_duration_ms": 2770,
  "total_tokens": 36402,
  "total_tool_calls": 0,
  "total_problems": 0
}

$ .claude/skills/read-dnd-bot-logs/logs.sh branches
[
  {
    "session_id": 2,
    "chat_id": 698225384,
    "tg_user_id": 698225384,
    "total_requests": 13,
    "total_errors": 0,
    "total_tokens": 31471,
    "total_tool_calls": 0,
    "first_seen": "2026-07-25T11:43:30.7029+03:00",
    "last_seen": "2026-07-25T11:45:19.164533+03:00"
  }
]

$ .claude/skills/read-dnd-bot-logs/logs.sh recent 1
[
  {
    "id": 25,
    "created_at": "2026-07-25T11:45:19.164533+03:00",
    "chat_id": 698225384,
    "tg_user_id": 698225384,
    "session_id": 2,
    "model": "GigaChat",
    "prompt": "Ты анализируешь ответ Dungeon Master в игре D&D ...",
    "max_tokens": null,
    "response": "...",
    "response_id": null,
    "duration_ms": 812,
    "tokens_used": 1180,
    "has_tools": false
  }
]
```

`show LOG_ID` / `chat` / `user` / `session` records can additionally carry
`status_code`, `error`, `tools_calls` (raw JSON string) and
`tools_calls_count` — all `omitempty`, so they're absent on the happy path
(see `internal/game/domain/llm_log/llm_log.go`).

On a missing id, `show` exits non-zero (curl's `-f` turns the 404 into curl
exit code `22`):

```bash
$ .claude/skills/read-dnd-bot-logs/logs.sh show 999999; echo "exit=$?"
exit=22
```

## Human path (HTML dashboard)

The same server renders an HTML dashboard at the same port: `/` (stats
cards + refresh button), `/logs?chat_id=&tg_user_id=&session_id=&limit=`,
`/log/<id>`, `/errors?limit=`, `/branches?chat_id=&tg_user_id=&limit=`.
Useful for a human skimming visually; the JSON API above is what an agent
should use.

## Gotchas

- **This is a second server, not `/health`.** `:8080/health` is the bot's
  own liveness check; the monitoring API lives on `MONITORING_PORT`
  (`:8081` by default) and has its own root — hitting `:8081/health`
  404s, there is no such route on this server.
- **No auth, CORS wide open** (`Access-Control-Allow-Origin: *`,
  `internal/monitoring/server.go:44-60`). Every prompt/response this API
  serves is the *real* DM conversation text, including the player's
  Telegram `chat_id`/`tg_user_id`. Treat output from this endpoint as
  sensitive — don't paste full dumps into shared/public places.
- **There's no `/api/errors` JSON endpoint** — only the HTML `/errors`
  page filters server-side (`GetWithErrors`). The `errors` driver command
  works around this by pulling `/api/logs` and filtering on the `error`
  field client-side.
- **`limit` out-of-range doesn't error, it silently falls back to the
  default** (100 for logs/branches, 50 for the HTML errors page) — a
  `limit=99999` request quietly returns the default-sized page, not an
  error and not "all rows."
- **Prod Postgres and the default integration-test DSN can collide.**
  `docker-compose.prod.yml` binds Postgres to host port `5432`, the exact
  same port `tests/integration`'s default `DATABASE_URL` targets
  (`postgres://dnd_user:dnd_password@localhost:5432/dnd`). Integration
  test setup calls `resetIntegrationDatabase()`, which runs `TRUNCATE ...
  RESTART IDENTITY CASCADE` on every table — including `llm_logs` — before
  each test. **Observed directly in this session:** `stats` reported
  `total_requests: 25` with real conversation data, then minutes later
  (no container restart, `dnd-bot-prod` uptime continuous) the same query
  returned `total_requests: 0` and `recent`/`branches` came back empty.
  Nothing this skill's driver does is destructive, so the likely cause is
  some other process on the same host running the integration suite
  against the live prod database. **Never run `make test-integration` /
  `go test ./tests/integration/...` (or `make docker-up`, which is a
  separate compose project but can still resolve to the same
  `localhost:5432`) on a machine where a `*-prod` Postgres container is
  bound to `5432`** — it will silently wipe live data.

## Troubleshooting

| Symptom | Fix |
|---|---|
| `curl: (7) Failed to connect ... 8081` | Nothing is listening on the monitoring port; check `docker ps` / `MONITORING_PORT` and start the bot (see Prerequisites). |
| Every command returns `[]` / all-zero `stats` | Not a bug in the API — either genuinely no LLM calls happened yet, or (see Gotchas) the table was truncated by a concurrent integration-test run. |
| `show <id>` exits 22 with no output | `curl -f` treats the API's 404 as a hard failure; the id doesn't exist — try `recent` to find valid ids. |
