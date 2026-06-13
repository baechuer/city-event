#!/usr/bin/env bash

set -euo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/common.sh"

apply=false
queue="cityevents.feed.projection.dlq"
count=10
api_url="${RABBITMQ_API_URL:-http://127.0.0.1:15672}"
api_user="${RABBITMQ_API_USER:-cityevents}"
api_password="${RABBITMQ_API_PASSWORD:-cityevents}"
vhost="${RABBITMQ_VHOST:-/}"
target_exchange="cityevents.events"
output_dir=""

usage() {
  cat <<'EOF'
Usage: ./scripts/copy-rabbitmq-dlq.sh [options]

Dry-runs or copies messages from a RabbitMQ DLQ back to cityevents.events.
The DLQ message is intentionally left in place; this is a safe copy-back, not a
purge or destructive move.

Options:
  --queue NAME          DLQ queue to inspect/copy. Default: cityevents.feed.projection.dlq
  --count N            Maximum messages to inspect/copy. Default: 10
  --api-url URL        RabbitMQ management URL. Default: http://127.0.0.1:15672
  --user USER          RabbitMQ management user. Default: cityevents
  --password PASSWORD  RabbitMQ management password. Default: cityevents
  --vhost VHOST        RabbitMQ vhost. Default: /
  --target-exchange X  Exchange to publish copies to. Default: cityevents.events
  --output-dir DIR     Directory for raw messages and publish bodies.
  --apply              Publish copies. Without this, the script is dry-run only.
  -h, --help           Show this help

Safety:
  --apply is allowed in GitHub Actions. On a local workstation it also requires:
    CITYEVENTS_ALLOW_LOCAL_REPLAY=true
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --queue)
      [[ $# -ge 2 ]] || die "--queue requires a value"
      queue="$2"
      shift 2
      ;;
    --count)
      [[ $# -ge 2 ]] || die "--count requires a value"
      count="$2"
      shift 2
      ;;
    --api-url)
      [[ $# -ge 2 ]] || die "--api-url requires a value"
      api_url="${2%/}"
      shift 2
      ;;
    --user)
      [[ $# -ge 2 ]] || die "--user requires a value"
      api_user="$2"
      shift 2
      ;;
    --password)
      [[ $# -ge 2 ]] || die "--password requires a value"
      api_password="$2"
      shift 2
      ;;
    --vhost)
      [[ $# -ge 2 ]] || die "--vhost requires a value"
      vhost="$2"
      shift 2
      ;;
    --target-exchange)
      [[ $# -ge 2 ]] || die "--target-exchange requires a value"
      target_exchange="$2"
      shift 2
      ;;
    --output-dir)
      [[ $# -ge 2 ]] || die "--output-dir requires a value"
      output_dir="$2"
      shift 2
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

[[ "$count" =~ ^[0-9]+$ ]] || die "--count must be an integer"
(( count > 0 )) || die "--count must be greater than zero"

cd "$REPO_ROOT"

if [[ "$apply" == true && "${GITHUB_ACTIONS:-}" != "true" && "${CITYEVENTS_ALLOW_LOCAL_REPLAY:-}" != "true" ]]; then
  die "--apply publishes RabbitMQ messages. Set CITYEVENTS_ALLOW_LOCAL_REPLAY=true locally, or run in GitHub Actions."
fi

if [[ -z "$output_dir" ]]; then
  output_dir="$REPO_ROOT/tmp/rabbitmq-dlq-copy/$(date -u '+%Y%m%dT%H%M%SZ')-$RANDOM"
elif [[ "$output_dir" != /* ]]; then
  output_dir="$REPO_ROOT/$output_dir"
fi
mkdir -p "$output_dir"

urlencode() {
  run_node -e 'process.stdout.write(encodeURIComponent(process.argv[1]));' "$1"
}

vhost_path="$(urlencode "$vhost")"
queue_path="$(urlencode "$queue")"
exchange_path="$(urlencode "$target_exchange")"
messages_file="$output_dir/messages.json"
publish_file="$output_dir/publish.ndjson"
publish_responses="$output_dir/publish-responses.ndjson"
summary_file="$output_dir/summary.md"

get_body="$(printf '{"count":%d,"ackmode":"ack_requeue_true","encoding":"auto","truncate":50000}' "$count")"

log "Read DLQ messages without removing them"
curl -fsS -u "$api_user:$api_password" \
  -H "Content-Type: application/json" \
  --data "$get_body" \
  "$api_url/api/queues/$vhost_path/$queue_path/get" >"$messages_file"

message_count="$(run_node -e 'const fs = require("fs"); const rows = JSON.parse(fs.readFileSync(process.argv[1], "utf8")); console.log(rows.length);' "$messages_file")"

run_node -e '
const fs = require("fs");
const input = process.argv[1];
const rows = JSON.parse(fs.readFileSync(input, "utf8"));
const retryHeaders = new Set([
  "x-cityevents-retry-count",
  "x-cityevents-retry-delay-ms",
  "x-cityevents-last-error",
  "x-cityevents-dead-letter-reason",
]);
for (const row of rows) {
  const properties = {...(row.properties || {})};
  const headers = {...(properties.headers || {})};
  const routingKey = String(headers["x-cityevents-original-routing-key"] || row.routing_key || "").trim();
  for (const key of retryHeaders) delete headers[key];
  properties.headers = headers;
  properties.delivery_mode = 2;
  const body = {
    properties,
    routing_key: routingKey,
    payload: row.payload || "",
    payload_encoding: row.payload_encoding || "string",
  };
  process.stdout.write(JSON.stringify(body) + "\n");
}
' "$messages_file" >"$publish_file"

{
  echo "# RabbitMQ DLQ Copy Summary"
  echo
  echo "- Captured At UTC: $(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  echo "- Queue: $queue"
  echo "- Target Exchange: $target_exchange"
  echo "- Requested Count: $count"
  echo "- Messages Read: $message_count"
  echo "- Apply: $apply"
  echo "- Output Directory: $output_dir"
  echo
  echo "## Safety"
  echo
  echo "Messages are read with ack_requeue_true, so the DLQ copy remains in place."
  echo "The script copies messages for reprocessing; it does not purge or move them."
  echo
  echo "## Publish Candidates"
  echo
  run_node -e '
const fs = require("fs");
const rows = fs.readFileSync(process.argv[1], "utf8").trim().split(/\n+/).filter(Boolean).map(JSON.parse);
if (rows.length === 0) {
  console.log("- none");
}
for (const [index, row] of rows.entries()) {
  const messageId = row.properties?.message_id || row.properties?.messageId || "";
  console.log(`- ${index + 1}: routing_key=${row.routing_key || "(empty)"} message_id=${messageId || "(empty)"}`);
}
' "$publish_file"
} >"$summary_file"

if [[ "$apply" != true ]]; then
  echo "Dry-run complete. Summary: $summary_file"
  exit 0
fi

: >"$publish_responses"
while IFS= read -r body; do
  [[ -n "$body" ]] || continue
  response="$(curl -fsS -u "$api_user:$api_password" \
    -H "Content-Type: application/json" \
    --data "$body" \
    "$api_url/api/exchanges/$vhost_path/$exchange_path/publish")"
  printf '%s\n' "$response" >>"$publish_responses"
  run_node -e 'const response = JSON.parse(process.argv[1]); if (!response.routed) process.exit(1);' "$response" ||
    die "RabbitMQ publish API returned routed=false"
done <"$publish_file"

{
  echo
  echo "## Apply Result"
  echo
  echo "Published copy responses: $publish_responses"
} >>"$summary_file"

echo "DLQ copy-back complete. Summary: $summary_file"
