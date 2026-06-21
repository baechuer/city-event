#!/usr/bin/env bash

set -euo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/common.sh"

cd "$REPO_ROOT"
setup_go_cache

run_id="$(date -u '+%Y%m%dT%H%M%SZ')-$RANDOM"
run_dir="$REPO_ROOT/tmp/security-error-evidence/$run_id"
mkdir -p "$run_dir"
summary_file="$run_dir/summary.md"
gates_file="$run_dir/security-gates.tsv"
metrics_file="$run_dir/metrics.tsv"
go_test_log="$run_dir/go-test.log"

printf 'security_area\tgate\tevidence_file\trequired_marker\tstatus\n' >"$gates_file"
printf 'metric\tvalue\n' >"$metrics_file"

gate_failures=0
gate_count=0

record_gate() {
  local area="$1"
  local gate="$2"
  local file="$3"
  local marker="$4"
  local status="$5"
  printf '%s\t%s\t%s\t%s\t%s\n' "$area" "$gate" "$file" "$marker" "$status" >>"$gates_file"
  gate_count=$((gate_count + 1))
  if [[ "$status" != "PASS" ]]; then
    gate_failures=$((gate_failures + 1))
  fi
}

require_security_marker() {
  local area="$1"
  local gate="$2"
  local file="$3"
  local marker="$4"
  if grep -Fq "$marker" "$file"; then
    record_gate "$area" "$gate" "$file" "$marker" "PASS"
  else
    record_gate "$area" "$gate" "$file" "$marker" "FAIL"
  fi
}

log "Run security and error Go tests"
if run_go test \
  ./internal/platform/authn \
  ./internal/platform/config \
  ./internal/platform/httpapi \
  ./internal/platform/identity \
  ./internal/services/auth \
  ./internal/services/eventregistration \
  ./internal/services/gateway \
  ./internal/services/media \
  >"$go_test_log" 2>&1; then
  go_test_status="PASS"
else
  go_test_status="FAIL"
  gate_failures=$((gate_failures + 1))
fi

require_security_marker "auth-public-errors" "unknown and wrong-password login use the same safe public error" "internal/services/auth/http_test.go" "TestAuthHandlersLoginFailures"
require_security_marker "auth-public-errors" "request body size limit returns request_too_large source error" "internal/platform/httpapi/json_test.go" "TestDecodeJSONLimitedRejectsLargeBody"
require_security_marker "auth-public-errors" "unknown fields and trailing JSON are rejected" "internal/platform/httpapi/json_test.go" "TestDecodeJSONLimitedRejectsUnknownFieldsAndTrailingJSON"

require_security_marker "session" "refresh token is HttpOnly and refresh rotates cookie" "internal/services/auth/http_test.go" "TestAuthHandlersWorkflow"
require_security_marker "session" "refresh requires CSRF and reuse revokes token family" "internal/services/auth/http_test.go" "TestAuthHandlersRefreshRotatesCookieAndRejectsReuse"
require_security_marker "session" "logout with refresh cookie requires CSRF and clears cookies" "internal/services/auth/http_test.go" "TestAuthHandlersLogoutWithRefreshCookieRequiresCSRF"
require_security_marker "session" "revoked access token is rejected by auth service" "internal/services/auth/http_test.go" "revoked me status"
require_security_marker "session" "revocation cache writes through and falls back to Postgres" "internal/services/auth/revocation_cache_test.go" "TestCachedRevocationRepositoryWritesThroughAndFallsBack"
require_security_marker "session" "revocation cache outage falls back to durable store" "internal/services/auth/revocation_cache_test.go" "TestCachedRevocationRepositoryIgnoresCacheOutage"

require_security_marker "rbac" "admin can update role and user cannot" "internal/services/auth/http_test.go" "TestAuthHandlersAdminCanUpdateRoles"
require_security_marker "rbac" "event create/update requires current role and ownership" "internal/services/eventregistration/http_test.go" "TestEventHandlersValidationAndAuthorization"
require_security_marker "rbac" "event service rejects revoked tokens from introspection" "internal/services/eventregistration/http_test.go" "TestEventHandlersRejectTokenDeniedByAuthService"
require_security_marker "rbac" "event service refreshes stale token role from auth service" "internal/services/eventregistration/http_test.go" "TestEventHandlersUseCurrentRoleFromAuthService"
require_security_marker "rbac" "organizer/admin can cancel attendee registration" "internal/services/eventregistration/http_test.go" "TestEventHandlersOrganizerAndAdminCanCancelAttendee"
require_security_marker "rbac" "media service rejects revoked tokens from introspection" "internal/services/media/http_test.go" "TestMediaHandlersRejectTokenDeniedByAuthService"
require_security_marker "rbac" "media service refreshes stale token role from auth service" "internal/services/media/http_test.go" "TestMediaHandlersUseCurrentRoleFromAuthService"
require_security_marker "rbac" "media upload checks event ownership and admin override" "internal/services/media/http_test.go" "TestMediaHandlersAuthorizeUploadIntentByEventOwnership"
require_security_marker "rbac" "media event authorizer rejects non-owner and plain user" "internal/services/media/event_authorizer_test.go" "TestHTTPEventAuthorizerRejectsNonOwnerAndPlainUser"

require_security_marker "gateway-boundary" "gateway validates token before proxying and strips spoofed identity headers" "internal/services/gateway/http_test.go" "TestGatewayValidatesTokenAndStripsSpoofedIdentityHeaders"
require_security_marker "gateway-boundary" "gateway rejects protected routes without token before upstream call" "internal/services/gateway/http_test.go" "TestGatewayRejectsProtectedEventRoutesWithoutToken"
require_security_marker "gateway-boundary" "gateway rejects invalid tokens before upstream call" "internal/services/gateway/http_test.go" "TestGatewayRejectsInvalidTokenBeforeProxying"
require_security_marker "gateway-boundary" "public route bypass does not attach identity headers" "internal/services/gateway/http_test.go" "TestGatewayAllowsPublicEventListWithoutAuth"

require_security_marker "cors" "CORS preflight excludes identity headers and allows CSRF/correlation/trace headers" "internal/platform/httpapi/router_test.go" "TestCORSPreflight"
require_security_marker "cors" "wildcard CORS is not treated as credentialed origin" "internal/platform/httpapi/router_test.go" "TestCORSDoesNotTreatWildcardAsCredentialedOrigin"
require_security_marker "cors" "gateway owns public CORS headers from proxied services" "internal/services/gateway/http_test.go" "TestGatewayOwnsPublicCORSHeaders"

require_security_marker "rate-limit" "repeated requests return 429 and increment rate-limit metric" "internal/platform/httpapi/router_test.go" "TestRateLimitRejectsRepeatedRequests"
require_security_marker "rate-limit" "untrusted X-Forwarded-For is ignored" "internal/platform/httpapi/router_test.go" "TestRateLimitIgnoresForwardedForFromUntrustedPeer"
require_security_marker "rate-limit" "trusted X-Forwarded-For is honored" "internal/platform/httpapi/router_test.go" "TestRateLimitUsesForwardedForFromTrustedPeer"
require_security_marker "rate-limit" "authenticated requests are keyed by verified user" "internal/platform/httpapi/router_test.go" "TestRateLimitKeysAuthenticatedRequestsByVerifiedUser"
require_security_marker "rate-limit" "Redis-backed rate limit store is shared across routers" "internal/platform/httpapi/router_test.go" "TestRateLimitUsesSharedStoreAcrossRouters"
require_security_marker "rate-limit" "rate-limit store fail-open mode emits metric" "internal/platform/httpapi/router_test.go" "TestRateLimitStoreErrorFailOpen"
require_security_marker "rate-limit" "rate-limit store fail-closed mode emits metric" "internal/platform/httpapi/router_test.go" "TestRateLimitStoreErrorFailClosed"
require_security_marker "rate-limit" "health and preflight skip rate limiting" "internal/platform/httpapi/router_test.go" "TestRateLimitSkipsHealthAndPreflight"

require_security_marker "telemetry" "metrics endpoint requires bearer token when configured" "internal/platform/httpapi/router_test.go" "TestMetricsRequiresBearerTokenWhenConfigured"
require_security_marker "telemetry" "non-local config requires metrics bearer token" "internal/platform/config/config_test.go" "TestValidateRequiresMetricsTokenOutsideLocalAndTest"

require_security_marker "jwt" "JWT verification rejects unexpected algorithm header" "internal/platform/authn/token_test.go" "TestTokenManagerVerifyRejectsUnexpectedHeader"
require_security_marker "jwt" "JWT verification rejects missing token type header" "internal/platform/authn/token_test.go" "TestTokenManagerVerifyRejectsMissingTypeHeader"
require_security_marker "jwt" "auth introspection rejects invalid or unavailable responses" "internal/platform/authn/introspection_test.go" "TestHTTPIntrospectorRejectsInvalidOrUnavailableResponses"

passed_gates="$(awk -F '\t' 'NR > 1 && $5 == "PASS" { count++ } END { print count + 0 }' "$gates_file")"
failed_gates="$(awk -F '\t' 'NR > 1 && $5 == "FAIL" { count++ } END { print count + 0 }' "$gates_file")"

{
  printf 'security_gate_count\t%s\n' "$gate_count"
  printf 'security_gate_passed\t%s\n' "$passed_gates"
  printf 'security_gate_failed\t%s\n' "$failed_gates"
  printf 'go_test_status\t%s\n' "$go_test_status"
  printf 'gate_failures\t%s\n' "$gate_failures"
  printf 'result_dir\t%s\n' "$run_dir"
} >>"$metrics_file"

{
  cat <<EOF
# Security And Error Evidence Summary

- Run ID: $run_id
- Security gates checked: $gate_count
- Security gates passed: $passed_gates
- Security gates failed: $failed_gates
- Targeted Go test status: $go_test_status
- Gate failures: $gate_failures
- Gates TSV: $gates_file
- Metrics TSV: $metrics_file
- Go test log: $go_test_log
- Result files: $run_dir

## Scope

This artifact maps security and safe-error requirements to concrete tests and
markers. It proves CI-visible coverage for API boundary behavior; it does not
prove external penetration testing, live WAF behavior, or production hardening.

## Security Gates

| Security Area | Gate | Evidence File | Marker | Status |
| --- | --- | --- | --- | --- |
EOF
  tail -n +2 "$gates_file" | awk -F '\t' '{ printf "| %s | %s | `%s` | `%s` | %s |\n", $1, $2, $3, $4, $5 }'
} >"$summary_file"

cat "$summary_file"
echo
if (( gate_failures > 0 )); then
  echo "Targeted Go test output:" >&2
  cat "$go_test_log" >&2 || true
  die "security/error evidence failed with $gate_failures gate failure(s). See $summary_file"
fi
echo "Security/error evidence passed."
