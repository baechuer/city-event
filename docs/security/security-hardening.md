# Security Hardening

## Purpose

This document tracks the security hardening work needed before CityEvents can
move beyond a portfolio/staging candidate.

## Current Controls

- Gateway verifies JWT access tokens before protected operations.
- Access tokens are short-lived and held in frontend memory.
- Refresh tokens are rotating, HttpOnly cookies.
- Refresh/logout are CSRF-protected.
- RBAC separates user, organizer, and admin operations.
- Redis-backed rate limiting protects auth, mutation, and read scopes.
- Ingress manifests require TLS redirection.

## Added Hardening Gates

`.github/workflows/security.yml` adds:

- `govulncheck ./...` for Go vulnerability reachability.
- production-only `npm audit` for frontend dependency risk.
- CodeQL analysis for Go and JavaScript/TypeScript.

These are CI security gates, not a complete security program.

The Go toolchain is pinned to `1.25.11` so `govulncheck` does not report the
standard-library vulnerabilities present in earlier Go `1.25.x` patch releases.

## Remaining Work

- Add secret scanning in the repository settings or a dedicated scanner.
- Add container image scanning after image push is implemented.
- Add OWASP ZAP baseline scanning against a staging deployment.
- Add immutable audit records for privileged role and event-management actions.
- Replace example Kubernetes secrets with External Secrets, Sealed Secrets, or a
  cloud secret manager.
- Run a focused ASVS checklist before public deployment.

## Acceptance Rubric

Security hardening is considered tracked when:

- Security workflow exists and is documented.
- Auth, RBAC, refresh, CSRF, and rate-limit controls are recorded.
- Remaining security work is explicit and claim-bounded.

Security hardening is considered production evidence only when:

- The security workflow passes on the deploy commit.
- Secret scanning and image scanning are active.
- Staging DAST produces reviewed output.
- Production secrets are managed outside Git.

## Claim Boundary

Safe now:

```text
Added CI security gates and documented the JWT, refresh-token, CSRF, RBAC, rate-limit, and TLS security boundaries.
```

Not safe yet:

```text
Production security reviewed.
```
