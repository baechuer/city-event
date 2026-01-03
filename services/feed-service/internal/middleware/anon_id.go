package middleware

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type anonIDKey struct{}

// AnonID middleware - HMAC-signed cookie for anonymous user tracking
// Cookie format: <anon_id>.<exp_unix>.<sig>
func AnonID(secret string, ttl time.Duration, secureCookie bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var anonID string

			if c, err := r.Cookie("anon_id"); err == nil {
				if id, ok := verifyAnonCookie(secret, c.Value); ok {
					anonID = id
				}
			}

			if anonID == "" {
				anonID = uuid.NewString()
				exp := time.Now().Add(ttl).Unix()
				cookieVal := signAnonCookie(secret, anonID, exp)

				cookie := &http.Cookie{
					Name:     "anon_id",
					Value:    cookieVal,
					Path:     "/",
					MaxAge:   int(ttl.Seconds()),
					HttpOnly: true,
					SameSite: http.SameSiteLaxMode,
					Secure:   secureCookie,
				}
				http.SetCookie(w, cookie)
			}

			ctx := context.WithValue(r.Context(), anonIDKey{}, anonID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AnonIDFromContext extracts the anon_id from context
func AnonIDFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(anonIDKey{}).(string)
	return v, ok && v != ""
}

// signAnonCookie creates a signed cookie value
func signAnonCookie(secret, anonID string, exp int64) string {
	payload := fmt.Sprintf("%s.%d", anonID, exp)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return payload + "." + sig
}

// verifyAnonCookie validates a signed cookie value
func verifyAnonCookie(secret, cookie string) (string, bool) {
	parts := strings.SplitN(cookie, ".", 3)
	if len(parts) != 3 {
		return "", false
	}

	anonID, expStr, _ := parts[0], parts[1], parts[2]

	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil {
		return "", false
	}

	if time.Now().Unix() > exp {
		return "", false // expired
	}

	expected := signAnonCookie(secret, anonID, exp)
	if !hmac.Equal([]byte(cookie), []byte(expected)) {
		return "", false
	}

	return anonID, true
}
