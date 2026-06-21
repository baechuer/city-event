#!/usr/bin/env bash

set -euo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/common.sh"

cd "$REPO_ROOT"
setup_go_cache

run_id="$(date -u '+%Y%m%dT%H%M%SZ')-$RANDOM"
run_dir="$REPO_ROOT/tmp/route-correctness/$run_id"
mkdir -p "$run_dir"
summary_file="$run_dir/summary.md"
routes_file="$run_dir/routes.tsv"
metrics_file="$run_dir/metrics.tsv"
go_test_log="$run_dir/go-test.log"

printf 'route_family\troute_scope\tevidence_file\trequired_marker\tstatus\n' >"$routes_file"
printf 'metric\tvalue\n' >"$metrics_file"

gate_failures=0
route_count=0

record_route() {
  local family="$1"
  local scope="$2"
  local file="$3"
  local marker="$4"
  local status="$5"
  printf '%s\t%s\t%s\t%s\t%s\n' "$family" "$scope" "$file" "$marker" "$status" >>"$routes_file"
  route_count=$((route_count + 1))
  if [[ "$status" != "PASS" ]]; then
    gate_failures=$((gate_failures + 1))
  fi
}

require_route_marker() {
  local family="$1"
  local scope="$2"
  local file="$3"
  local marker="$4"
  if grep -Fq "$marker" "$file"; then
    record_route "$family" "$scope" "$file" "$marker" "PASS"
  else
    record_route "$family" "$scope" "$file" "$marker" "FAIL"
  fi
}

log "Run route correctness Go tests"
if run_go test \
  ./internal/platform/httpapi \
  ./internal/services/auth \
  ./internal/services/eventregistration \
  ./internal/services/feed \
  ./internal/services/gateway \
  ./internal/services/media \
  >"$go_test_log" 2>&1; then
  go_test_status="PASS"
else
  go_test_status="FAIL"
  gate_failures=$((gate_failures + 1))
fi

require_route_marker "auth" "register creates user, validates input, rejects duplicate email" "internal/services/auth/http_test.go" "TestAuthHandlersRegisterValidationAndDuplicate"
require_route_marker "auth" "login returns token and hides invalid email/password distinction" "internal/services/auth/http_test.go" "TestAuthHandlersLoginFailures"
require_route_marker "auth" "refresh rotates HttpOnly refresh cookie and rejects CSRF/reuse" "internal/services/auth/http_test.go" "TestAuthHandlersRefreshRotatesCookieAndRejectsReuse"
require_route_marker "auth" "logout revokes access token and requires CSRF when cookies are present" "internal/services/auth/http_test.go" "TestAuthHandlersLogoutWithRefreshCookieRequiresCSRF"
require_route_marker "auth" "admin-only role update" "internal/services/auth/http_test.go" "TestAuthHandlersAdminCanUpdateRoles"

require_route_marker "events" "create/list/detail/join/cancel workflow" "internal/services/eventregistration/http_test.go" "TestEventHandlersWorkflow"
require_route_marker "events" "create/update authorization and validation" "internal/services/eventregistration/http_test.go" "TestEventHandlersValidationAndAuthorization"
require_route_marker "events" "revoked token is rejected by auth introspection" "internal/services/eventregistration/http_test.go" "TestEventHandlersRejectTokenDeniedByAuthService"
require_route_marker "events" "stale organizer role is refreshed from auth service" "internal/services/eventregistration/http_test.go" "TestEventHandlersUseCurrentRoleFromAuthService"
require_route_marker "events" "organizer/admin can moderate attendee registrations" "internal/services/eventregistration/http_test.go" "TestEventHandlersOrganizerAndAdminCanCancelAttendee"
require_route_marker "events" "public list excludes canceled events and detail includes viewer status" "internal/services/eventregistration/http_test.go" "TestEventHandlersListExcludesCanceledAndDetailIncludesViewerStatus"

require_route_marker "feed" "feed list/detail returns projected events" "internal/services/feed/http_test.go" "TestFeedHandlersListAndDetail"
require_route_marker "feed" "feed safe errors for invalid query and missing detail" "internal/services/feed/http_test.go" "TestFeedHandlersErrors"

require_route_marker "media" "upload intent, uploaded transition, and detail workflow" "internal/services/media/http_test.go" "TestMediaHandlersWorkflow"
require_route_marker "media" "upload authorization uses event ownership and admin override" "internal/services/media/http_test.go" "TestMediaHandlersAuthorizeUploadIntentByEventOwnership"
require_route_marker "media" "revoked token is rejected by auth introspection" "internal/services/media/http_test.go" "TestMediaHandlersRejectTokenDeniedByAuthService"
require_route_marker "media" "stale organizer role is refreshed from auth service" "internal/services/media/http_test.go" "TestMediaHandlersUseCurrentRoleFromAuthService"
require_route_marker "media" "validation rejects invalid, unauthenticated, spoofed, and missing cases" "internal/services/media/http_test.go" "TestMediaHandlersValidation"

require_route_marker "gateway" "protected event routes validate bearer token and strip spoofed identity headers" "internal/services/gateway/http_test.go" "TestGatewayValidatesTokenAndStripsSpoofedIdentityHeaders"
require_route_marker "gateway" "protected event routes reject missing token" "internal/services/gateway/http_test.go" "TestGatewayRejectsProtectedEventRoutesWithoutToken"
require_route_marker "gateway" "public event list bypasses auth" "internal/services/gateway/http_test.go" "TestGatewayAllowsPublicEventListWithoutAuth"
require_route_marker "gateway" "gateway owns public CORS headers" "internal/services/gateway/http_test.go" "TestGatewayOwnsPublicCORSHeaders"
require_route_marker "gateway" "invalid token is rejected before proxying" "internal/services/gateway/http_test.go" "TestGatewayRejectsInvalidTokenBeforeProxying"
require_route_marker "gateway" "trace context reaches auth and upstream services" "internal/services/gateway/http_test.go" "TestGatewayPropagatesTraceContextToAuthAndUpstream"

require_route_marker "platform-http" "metrics endpoint requires bearer token" "internal/platform/httpapi/router_test.go" "TestMetricsRequiresBearerTokenWhenConfigured"
require_route_marker "platform-http" "rate limiting emits public 429 response" "internal/platform/httpapi/router_test.go" "TestRateLimitRejectsRepeatedRequests"
require_route_marker "platform-http" "outbox and consumer metrics are exported" "internal/platform/httpapi/router_test.go" "TestOutboxAndConsumerMetrics"

passed_routes="$(awk -F '\t' 'NR > 1 && $5 == "PASS" { count++ } END { print count + 0 }' "$routes_file")"
failed_routes="$(awk -F '\t' 'NR > 1 && $5 == "FAIL" { count++ } END { print count + 0 }' "$routes_file")"

{
  printf 'route_marker_count\t%s\n' "$route_count"
  printf 'route_marker_passed\t%s\n' "$passed_routes"
  printf 'route_marker_failed\t%s\n' "$failed_routes"
  printf 'go_test_status\t%s\n' "$go_test_status"
  printf 'gate_failures\t%s\n' "$gate_failures"
  printf 'result_dir\t%s\n' "$run_dir"
} >>"$metrics_file"

{
  cat <<EOF
# Route Correctness Evidence Summary

- Run ID: $run_id
- Route markers checked: $route_count
- Route markers passed: $passed_routes
- Route markers failed: $failed_routes
- Targeted Go test status: $go_test_status
- Gate failures: $gate_failures
- Routes TSV: $routes_file
- Metrics TSV: $metrics_file
- Go test log: $go_test_log
- Result files: $run_dir

## Scope

This artifact maps public route families to concrete tests and markers. It is a
route-correctness evidence index; it does not replace load, browser E2E, or
failure-injection evidence.

## Route Evidence

| Route Family | Route Scope | Evidence File | Marker | Status |
| --- | --- | --- | --- | --- |
EOF
  tail -n +2 "$routes_file" | awk -F '\t' '{ printf "| %s | %s | `%s` | `%s` | %s |\n", $1, $2, $3, $4, $5 }'
} >"$summary_file"

cat "$summary_file"
echo
if (( gate_failures > 0 )); then
  echo "Targeted Go test output:" >&2
  cat "$go_test_log" >&2 || true
  die "route correctness evidence failed with $gate_failures gate failure(s). See $summary_file"
fi
echo "Route correctness evidence passed."
