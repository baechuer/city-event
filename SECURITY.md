# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability, please report it by emailing the project maintainers directly. Do not create public GitHub issues for security vulnerabilities.

## Secret Management

### Required Environment Variables

The following secrets **MUST** be set before running the application:

| Variable | Description | How to Generate |
|----------|-------------|-----------------|
| `JWT_SECRET` | Signing key for access/refresh tokens | `openssl rand -base64 32` |
| `INTERNAL_SECRET_KEY` | Service-to-service authentication | `openssl rand -base64 32` |
| `POSTGRES_USER` | Database username | Choose a non-default username |
| `POSTGRES_PASSWORD` | Database password | `openssl rand -base64 24` |

### Setup Instructions

1. Copy the template file:
   ```bash
   cp .env.example .env
   ```

2. Generate secure secrets:
   ```bash
   # Generate JWT secret
   echo "JWT_SECRET=$(openssl rand -base64 32)" >> .env
   
   # Generate internal key
   echo "INTERNAL_SECRET_KEY=$(openssl rand -base64 32)" >> .env
   
   # Generate DB password
   echo "POSTGRES_PASSWORD=$(openssl rand -base64 24)" >> .env
   ```

3. Review and edit `.env` with your specific values.

### Secret Rotation

#### JWT Secret Rotation

1. Add new secret as primary, keep old as secondary (not yet implemented - planned feature)
2. Wait for all access tokens to expire (15 minutes default)
3. Remove old secret

#### Database Password Rotation

1. Update password in PostgreSQL
2. Update `POSTGRES_PASSWORD` in `.env`
3. Restart all services: `docker-compose down && docker-compose up -d`

## Security Headers

All services set the following security headers:

- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- `X-XSS-Protection: 1; mode=block`
- `Referrer-Policy: strict-origin-when-cross-origin`
- `Content-Security-Policy: default-src 'self'`

## Authentication

### Token Lifecycle

- **Access Token**: 15 minutes, stored in memory only
- **Refresh Token**: 7 days, stored in HttpOnly cookie
- **Cookie Flags**: HttpOnly, Secure (in production), SameSite=Lax

### Token Revocation

- Logout revokes the current refresh token
- Password change revokes all sessions
- Admin can revoke all sessions for a user

## Internal Service Communication

Services communicate internally using:

1. **X-Internal-Secret** header for service-to-service auth
2. **X-Request-ID** header for distributed tracing

Internal endpoints (`/internal/*`) are not exposed through the BFF.

## Rate Limiting

- Redis-based rate limiting on all endpoints
- Default: 100 requests/minute per IP
- Join operations: 10 requests/minute per user

## Known Limitations

1. JWT secret rotation requires manual coordination (multi-secret support planned)
2. No automated secret scanning in CI (planned)
3. Redis does not use password in dev mode (intentional for local development)

## Checklist for Production Deployment

- [ ] All secrets generated with cryptographically secure random
- [ ] `.env` file is NOT committed to git
- [ ] Redis password is set (`REDIS_PASSWORD`)
- [ ] RabbitMQ credentials changed from default
- [ ] HTTPS/TLS enabled for all external traffic
- [ ] `Secure` cookie flag enabled (set `APP_ENV=production`)
- [ ] Database connections use SSL (`sslmode=require`)
- [ ] Firewall rules restrict inter-service traffic
