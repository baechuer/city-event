package notify

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

type Sender interface {
	SendVerifyEmail(ctx context.Context, toEmail, url string) error
	SendPasswordReset(ctx context.Context, toEmail, url string) error
	SendEventCanceled(ctx context.Context, toEmail, eventID, reason string) error
	SendEventUnpublished(ctx context.Context, toEmail, eventID, reason string) error
}

type permanentMarker interface{ Permanent() bool }

type IdempotencyStore interface {
	// Seen returns true if key already marked as sent.
	Seen(ctx context.Context, key string) (bool, error)

	// MarkSent marks key as sent with TTL (idempotent).
	// If it already exists, treat as success.
	MarkSent(ctx context.Context, key string, ttl time.Duration) error
}

type UserResolver interface {
	GetEmail(ctx context.Context, userID string) (string, error)
}

type Service struct {
	sender   Sender
	resolver UserResolver
	idem     IdempotencyStore // nil => disabled
	ttl      time.Duration
	lg       zerolog.Logger
}

func NewService(sender Sender, resolver UserResolver, idem IdempotencyStore, ttl time.Duration, lg zerolog.Logger) *Service {
	return &Service{
		sender:   sender,
		resolver: resolver,
		idem:     idem,
		ttl:      ttl,
		lg:       lg.With().Str("component", "notify_service").Logger(),
	}
}

func (s *Service) VerifyEmail(ctx context.Context, userID, email, link string) error {
	s.lg.Info().Str("user_id", userID).Str("email", email).Msg("NotifyService.VerifyEmail called")
	token := tokenFromLink(link)
	key := fmt.Sprintf("email:verify:%s", tokenOrFallback(token, link))

	if s.idem != nil {
		seen, e := s.idem.Seen(ctx, key)
		if e != nil {
			return e
		}
		if seen {
			s.lg.Info().Str("email", email).Str("token", token).Msg("idempotent skip (already sent)")
			return nil
		}
	}

	err := s.sender.SendVerifyEmail(ctx, email, link)
	if err != nil {
		return err
	}

	if s.idem != nil {
		if e := s.idem.MarkSent(ctx, key, s.ttl); e != nil {
			s.lg.Warn().Err(e).Str("key", key).Msg("idempotency mark failed (send already succeeded)")
			return nil
		}
	}

	s.lg.Info().Str("email", email).Str("token", token).Msg("verify email sent")
	return nil

}

func (s *Service) PasswordReset(ctx context.Context, userID, email, link string) error {
	token := tokenFromLink(link)
	key := fmt.Sprintf("email:reset:%s", tokenOrFallback(token, link))
	if s.idem != nil {
		seen, e := s.idem.Seen(ctx, key)
		if e != nil {
			return e
		}
		if seen {
			s.lg.Info().Str("email", email).Str("token", token).Msg("idempotent skip (already sent)")
			return nil
		}
	}
	err := s.sender.SendPasswordReset(ctx, email, link)
	if err != nil {
		return err
	}

	if s.idem != nil {
		if e := s.idem.MarkSent(ctx, key, s.ttl); e != nil {
			s.lg.Warn().Err(e).Str("key", key).Msg("idempotency mark failed (send already succeeded)")
			return nil
		}
	}

	s.lg.Info().Str("email", email).Str("token", token).Msg("password reset email sent")
	return nil
}

// --- helpers

func tokenFromLink(link string) string {
	link = strings.TrimSpace(link)
	if link == "" {
		return ""
	}
	u, err := url.Parse(link)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(u.Query().Get("token"))
}

func tokenOrFallback(token, link string) string {
	if token != "" {
		return token
	}
	// fallback: stable-ish key; you can hash if you want
	return link
}

// isNonRetriable is an optional helper if you want to further subdivide logic at the upper layer; there's no strong dependency here.
// Kept in case you want to quickly enter DLQ when the sender returns a PermanentError.
func isNonRetriable(err error) bool {
	if err == nil {
		return false
	}
	var pm permanentMarker
	if errors.As(err, &pm) && pm.Permanent() {
		return true
	}
	return false
}
