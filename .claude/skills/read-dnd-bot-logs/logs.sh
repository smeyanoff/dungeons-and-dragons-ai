#!/usr/bin/env bash
# Driver for the dnd-bot LLM monitoring/logs endpoint (internal/monitoring/server.go).
# Talks to the JSON API exposed by the already-running bot process (dev or prod).
#
# Usage: logs.sh <command> [args...]
#   stats   [FROM_RFC3339] [TO_RFC3339]   Aggregate stats (default: last 7 days)
#   recent  [LIMIT=100]                   Most recent logs, newest first (max 1000)
#   chat    CHAT_ID [LIMIT=100]           Logs for one Telegram chat_id
#   user    TG_USER_ID [LIMIT=100]        Logs for one Telegram tg_user_id
#   session SESSION_ID [LIMIT=100]        Logs for one game_session_id
#   errors  [LIMIT=50]                    Only logs with a non-empty error field (max 500)
#   branches [CHAT_ID] [LIMIT=100]        Per-session aggregates (requests/tokens/errors), optionally filtered by chat_id
#   show    LOG_ID                        Full record for one log id (includes full prompt/response)
#   metrics                                Counters: rag_failure/empty, output_leak, telegram_polling_error
#
# Env:
#   DND_MONITORING_URL   Base URL of the monitoring server (default: http://localhost:8081)
#
# Requires: curl, jq

set -euo pipefail

BASE_URL="${DND_MONITORING_URL:-http://localhost:8081}"

fail() { echo "error: $*" >&2; exit 1; }

need_jq() { command -v jq >/dev/null 2>&1 || fail "jq is required"; }

get() {
  # get <path+query>
  curl -sf "${BASE_URL}${1}"
}

cmd="${1:-}"
shift || true

case "$cmd" in
  stats)
    from="${1:-}"; to="${2:-}"
    q=""
    [ -n "$from" ] && q="?from=$(python3 -c "import urllib.parse,sys;print(urllib.parse.quote(sys.argv[1]))" "$from")"
    if [ -n "$to" ]; then
      sep="&"; [ -z "$q" ] && sep="?"
      q="${q}${sep}to=$(python3 -c "import urllib.parse,sys;print(urllib.parse.quote(sys.argv[1]))" "$to")"
    fi
    need_jq
    get "/api/stats${q}" | jq .
    ;;

  recent)
    limit="${1:-100}"
    need_jq
    get "/api/logs?limit=${limit}" | jq .
    ;;

  chat)
    [ -n "${1:-}" ] || fail "usage: logs.sh chat CHAT_ID [LIMIT]"
    chat_id="$1"; limit="${2:-100}"
    need_jq
    get "/api/logs?chat_id=${chat_id}&limit=${limit}" | jq .
    ;;

  user)
    [ -n "${1:-}" ] || fail "usage: logs.sh user TG_USER_ID [LIMIT]"
    tg_user_id="$1"; limit="${2:-100}"
    need_jq
    get "/api/logs?tg_user_id=${tg_user_id}&limit=${limit}" | jq .
    ;;

  session)
    [ -n "${1:-}" ] || fail "usage: logs.sh session SESSION_ID [LIMIT]"
    session_id="$1"; limit="${2:-100}"
    need_jq
    get "/api/logs?session_id=${session_id}&limit=${limit}" | jq .
    ;;

  errors)
    # No dedicated /api/errors JSON endpoint exists (only the HTML /errors page uses
    # GetWithErrors server-side) -- filter client-side over the JSON /api/logs feed instead.
    limit="${1:-1000}"
    need_jq
    get "/api/logs?limit=${limit}" | jq '[.[] | select(.error != null and .error != "")]'
    ;;

  branches)
    chat_id=""; limit="100"
    if [ $# -ge 1 ] && [[ "${1:-}" =~ ^[0-9-]+$ ]]; then chat_id="$1"; shift || true; fi
    limit="${1:-100}"
    q="?limit=${limit}"
    [ -n "$chat_id" ] && q="${q}&chat_id=${chat_id}"
    need_jq
    get "/api/branches${q}" | jq .
    ;;

  show)
    [ -n "${1:-}" ] || fail "usage: logs.sh show LOG_ID"
    id="$1"
    need_jq
    get "/api/log/${id}" | jq .
    ;;

  metrics)
    need_jq
    get "/api/metrics" | jq .
    ;;

  *)
    fail "unknown command: '${cmd}'. See header of $0 for usage."
    ;;
esac
