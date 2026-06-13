#!/usr/bin/env bash

set -euo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/common.sh"

apply=false
id=""
limit=10
reset_attempts=false

usage() {
  cat <<'EOF'
Usage: ./scripts/requeue-dead-outbox.sh [options]

Dry-runs or requeues terminal DEAD outbox rows back to FAILED so the relay can
try publishing them again after an operator has fixed the underlying issue.

Options:
  --id ID            Requeue one exact outbox row.
  --limit N          Maximum rows to inspect/requeue when --id is omitted. Default: 10
  --reset-attempts   Reset attempts to 0 during --apply. Default keeps attempts.
  --apply            Mutate rows. Without this, the script is dry-run only.
  -h, --help         Show this help

Safety:
  --apply is allowed in GitHub Actions. On a local workstation it also requires:
    CITYEVENTS_ALLOW_LOCAL_REPLAY=true
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --id)
      [[ $# -ge 2 ]] || die "--id requires a value"
      id="$2"
      shift 2
      ;;
    --limit)
      [[ $# -ge 2 ]] || die "--limit requires a value"
      limit="$2"
      shift 2
      ;;
    --reset-attempts)
      reset_attempts=true
      shift
      ;;
    --apply)
      apply=true
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "unknown argument: $1"
      ;;
  esac
done

[[ "$limit" =~ ^[0-9]+$ ]] || die "--limit must be an integer"
(( limit > 0 )) || die "--limit must be greater than zero"

cd "$REPO_ROOT"

if [[ "$apply" == true && "${GITHUB_ACTIONS:-}" != "true" && "${CITYEVENTS_ALLOW_LOCAL_REPLAY:-}" != "true" ]]; then
  die "--apply mutates Postgres. Set CITYEVENTS_ALLOW_LOCAL_REPLAY=true locally, or run in GitHub Actions."
fi

sql_escape() {
  printf '%s' "$1" | sed "s/'/''/g"
}

where_clause="status = 'DEAD'"
if [[ -n "$id" ]]; then
  escaped_id="$(sql_escape "$id")"
  where_clause="$where_clause and id = '$escaped_id'"
fi

postgres_exec() {
  local sql="$1"
  run_docker compose exec -T postgres \
    psql -U cityevents -d cityevents -v ON_ERROR_STOP=1 -c "$sql"
}

preview_sql="select id, aggregate_type, aggregate_id, routing_key, attempts, created_at, left(last_error, 180) as last_error from outbox_messages where $where_clause order by created_at asc limit $limit;"

if [[ "$apply" != true ]]; then
  log "Dry-run: matching DEAD outbox rows"
  postgres_exec "$preview_sql"
  cat <<'EOF'

No rows were changed. Rerun with --apply after fixing the root cause.
Use --reset-attempts only when the cause is known to be fixed and a fresh retry
budget is intentional.
EOF
  exit 0
fi

reset_sql="m.attempts"
if [[ "$reset_attempts" == true ]]; then
  reset_sql="0"
fi

log "Requeue DEAD outbox rows"
postgres_exec "
with target as (
  select id
  from outbox_messages
  where $where_clause
  order by created_at asc
  limit $limit
  for update skip locked
)
update outbox_messages m
set status = 'FAILED',
    attempts = $reset_sql,
    available_at = now(),
    last_error = left('operator requeued from DEAD at ' || now() || '; previous error: ' || m.last_error, 500)
from target
where m.id = target.id
returning m.id, m.aggregate_type, m.aggregate_id, m.routing_key, m.status, m.attempts, m.available_at;
"
