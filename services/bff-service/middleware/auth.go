package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type contextKey string

const (
	UserIDKey      contextKey = "user_id"
	BearerTokenKey contextKey = "bearer_token"
)

func Auth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				next.ServeHTTP(w, r)
				return
			}

			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
				next.ServeHTTP(w, r)
				return
			}

			tokenStr := parts[1]
			claims := &jwt.MapClaims{}

			token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
				return []byte(secret), nil
			})

			if err != nil || !token.Valid {
				next.ServeHTTP(w, r)
				return
			}

			// Based on auth-service, it uses 'uid' or 'sub'
			uidStr, _ := (*claims)["uid"].(string)
			if uidStr == "" {
				uidStr, _ = (*claims)["sub"].(string)
			}

			if uidStr != "" {
				if uid, err := uuid.Parse(uidStr); err == nil {
					ctx := context.WithValue(r.Context(), UserIDKey, uid)
					ctx = context.WithValue(ctx, BearerTokenKey, authHeader)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

func GetUserID(ctx context.Context) uuid.UUID {
	id, ok := ctx.Value(UserIDKey).(uuid.UUID)
	if !ok {
		return uuid.Nil
	}
	return id
}

func GetBearerToken(ctx context.Context) string {
	token, ok := ctx.Value(BearerTokenKey).(string)
	if !ok {
		return ""
	}
	return token
}
