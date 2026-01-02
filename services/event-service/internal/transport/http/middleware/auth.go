package middleware

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/baechuer/real-time-ressys/services/event-service/internal/transport/http/response"
)

type ctxKey string

const (
	ctxUserID ctxKey = "user_id"
	ctxRole   ctxKey = "role"
	ctxVer    ctxKey = "ver"
)

type Claims struct {
	UserID string `json:"uid"`
	Role   string `json:"role"`
	Ver    int64  `json:"ver"`
	jwt.RegisteredClaims
}

type TokenVersionChecker interface {
	GetTokenVersion(ctx context.Context, userID string) (int64, error)
}

type AuthMiddleware struct {
	secret       []byte
	issuer       string
	versionCheck TokenVersionChecker
}

func NewAuth(secret, issuer string, versionCheck TokenVersionChecker) *AuthMiddleware {
	return &AuthMiddleware{
		secret:       []byte(secret),
		issuer:       issuer,
		versionCheck: versionCheck,
	}
}

func (a *AuthMiddleware) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, role, ver, err := a.parse(r)
		if err != nil {
			log.Printf("auth parse error: %v", err)
			// auth-service style error body
			response.Fail(
				w,
				http.StatusUnauthorized,
				"unauthorized",
				"unauthorized",
				map[string]string{"reason": err.Error()},
				response.RequestIDFromRequest(r),
			)
			return
		}

		// Check revocation via version
		if a.versionCheck != nil {
			currentVer, err := a.versionCheck.GetTokenVersion(r.Context(), uid)
			if err == nil && currentVer > ver {
				response.Fail(
					w,
					http.StatusUnauthorized,
					"token_revoked",
					"token version obsolete",
					nil,
					response.RequestIDFromRequest(r),
				)
				return
			}
			// if err != nil (Redis down) or currentVer == 0 (not in cache), we allow (fail open)
			// to avoid complete service outage if redis blips.
			// "Secure by default" would fail here, but availability usually wins in this project scope.
		}

		ctx := context.WithValue(r.Context(), ctxUserID, uid)
		ctx = context.WithValue(ctx, ctxRole, role)
		ctx = context.WithValue(ctx, ctxVer, ver)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a *AuthMiddleware) parse(r *http.Request) (string, string, int64, error) {
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	log.Printf("Authorization header received: [%s]", h)
	if !strings.HasPrefix(h, "Bearer ") {
		return "", "", 0, errors.New("missing bearer token")
	}
	raw := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))

	claims := &Claims{}
	tok, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, errors.New("unexpected signing method")
		}
		return a.secret, nil
	}, jwt.WithLeeway(30*time.Second))
	if err != nil {
		return "", "", 0, err
	}
	if !tok.Valid {
		return "", "", 0, errors.New("invalid token")
	}

	if a.issuer != "" && claims.Issuer != a.issuer {
		return "", "", 0, errors.New("invalid issuer")
	}
	if strings.TrimSpace(claims.UserID) == "" {
		return "", "", 0, errors.New("missing uid")
	}
	role := strings.TrimSpace(claims.Role)
	if role == "" {
		role = "user"
	}
	return claims.UserID, role, claims.Ver, nil
}

func UserID(r *http.Request) string {
	if v, ok := r.Context().Value(ctxUserID).(string); ok {
		return v
	}
	return ""
}

func Role(r *http.Request) string {
	if v, ok := r.Context().Value(ctxRole).(string); ok {
		return v
	}
	return ""
}

func Ver(r *http.Request) int64 {
	if v, ok := r.Context().Value(ctxVer).(int64); ok {
		return v
	}
	return 0
}
