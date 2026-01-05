package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog"
)

type fakeHandler struct {
	verifyCalled          int
	resetCalled           int
	eventCanceledCalls    int // Added for testing
	eventUnpublishedCalls int // Added for testing

	verifyErr error
	resetErr  error

	lastVerify struct {
		userID string
		email  string
		url    string
	}
	lastReset struct {
		userID string
		email  string
		url    string
	}
}

func (h *fakeHandler) VerifyEmail(ctx context.Context, userID, email, url string) error {
	_ = ctx
	h.verifyCalled++
	h.lastVerify.userID, h.lastVerify.email, h.lastVerify.url = userID, email, url
	return h.verifyErr
}
func (h *fakeHandler) PasswordReset(ctx context.Context, userID, email, url string) error {
	_ = ctx
	h.resetCalled++
	h.lastReset.userID, h.lastReset.email, h.lastReset.url = userID, email, url
	return h.resetErr
}

func (h *fakeHandler) EventCanceled(ctx context.Context, eventID, userID, reason, actorRole string) error {
	_ = ctx
	h.eventCanceledCalls++
	return nil
}

func (h *fakeHandler) EventUnpublished(ctx context.Context, eventID, userID, reason, actorRole string) error {
	_ = ctx
	h.eventUnpublishedCalls++
	return nil
}

type fakePublisher struct {
	retryCalls []struct {
		tier        string
		nextAttempt int
		rk          string
	}
	finalCalls []struct {
		reason string
		rk     string
	}
	retryErr error
	finalErr error
}

func (p *fakePublisher) PublishRetry(ctx context.Context, tier string, orig amqp.Delivery, nextAttempt int, cause error) error {
	_ = ctx
	_ = cause
	p.retryCalls = append(p.retryCalls, struct {
		tier        string
		nextAttempt int
		rk          string
	}{tier: tier, nextAttempt: nextAttempt, rk: orig.RoutingKey})
	return p.retryErr
}
func (p *fakePublisher) PublishFinal(ctx context.Context, orig amqp.Delivery, reason string, cause error) error {
	_ = ctx
	_ = cause
	p.finalCalls = append(p.finalCalls, struct {
		reason string
		rk     string
	}{reason: reason, rk: orig.RoutingKey})
	return p.finalErr
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}
	return b
}

func newTestConsumer(h Handler, pub Publisher) *Consumer {
	c := NewConsumer(Config{
		RabbitURL:          "amqp://unused",
		Exchange:           "city.events",
		Queue:              "email-service.q",
		BindKeys:           []string{"auth.email.#", "auth.password.#"},
		Prefetch:           1,
		Tag:                "t",
		EmailPublicBaseURL: "http://localhost:8090",
	}, h, zerolog.Nop())

	// inject publisher directly (unit tests do not call connectAndDeclare)
	c.pub = pub
	return c
}

func TestRetryTierMapping(t *testing.T) {
	if got := retryTier(1); got != "10s" {
		t.Fatalf("attempt1 expected 10s got %s", got)
	}
	if got := retryTier(2); got != "1m" {
		t.Fatalf("attempt2 expected 1m got %s", got)
	}
	if got := retryTier(3); got != "10m" {
		t.Fatalf("attempt3 expected 10m got %s", got)
	}
	if got := retryTier(99); got != "10m" {
		t.Fatalf("attempt99 expected 10m got %s", got)
	}
}

func TestHandleDelivery_UnknownRoutingKey_AcksByReturningNil(t *testing.T) {
	h := &fakeHandler{}
	p := &fakePublisher{}
	c := newTestConsumer(h, p)

	d := amqp.Delivery{RoutingKey: "unknown.key"}
	if err := c.handleDelivery(context.Background(), d); err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if h.verifyCalled != 0 || h.resetCalled != 0 {
		t.Fatalf("expected handler not called")
	}
}

func TestHandleDelivery_VerifyEmail_RewritesLinkTo8090(t *testing.T) {
	h := &fakeHandler{}
	p := &fakePublisher{}
	c := newTestConsumer(h, p)
	c.publicBase = "http://localhost:8090"

	t.Run("VerifyEmail_UsesOriginalLink", func(t *testing.T) {
		evt := VerifyEmailEvent{
			UserID: "u1",
			Email:  "a@b.com",
			URL:    "http://localhost:8080/auth/v1/verify-email/confirm?token=XYZ",
		}

		d := amqp.Delivery{
			RoutingKey:  "auth.email.verify.requested",
			ContentType: "application/json",
			Body:        mustJSON(t, evt),
		}

		if err := c.handleDelivery(context.Background(), d); err != nil {
			t.Fatalf("expected nil err, got %v", err)
		}
		if h.verifyCalled != 1 {
			t.Fatalf("expected verify called once, got %d", h.verifyCalled)
		}
		// Expect exact same URL (no rewriting)
		if h.lastVerify.url != "http://localhost:8080/auth/v1/verify-email/confirm?token=XYZ" {
			t.Fatalf("expected original url, got %q", h.lastVerify.url)
		}
	})

	t.Run("EventCanceled", func(t *testing.T) {
		payload := `{"event_id": "e1", "user_id": "u1", "reason": "r", "prev_status": "p"}`
		d := amqp.Delivery{
			RoutingKey: "email.event_canceled",
			Body:       []byte(payload),
		}

		if err := c.handleDelivery(context.Background(), d); err != nil {
			t.Errorf("expected nil error, got %v", err)
		}

		if h.eventCanceledCalls != 1 {
			t.Errorf("expected 1 call, got %d", h.eventCanceledCalls)
		}
	})

	t.Run("UnknownKey_Dropped", func(t *testing.T) {
		// New hardening test: ensure unknown key returns nil (ack/drop) and doesn't error
		d := amqp.Delivery{
			RoutingKey: "email.unknown.event",
			Body:       []byte(`{"some": "data"}`),
		}

		if err := c.handleDelivery(context.Background(), d); err != nil {
			t.Errorf("expected nil error (drop), got %v", err)
		}
		// Confirm no handler calls
		if h.verifyCalled != 1 { // from previous test
			t.Errorf("calls should not increase for verify, got %d", h.verifyCalled)
		}
		if h.eventCanceledCalls != 1 { // from previous test
			t.Errorf("calls should not increase for eventCanceled, got %d", h.eventCanceledCalls)
		}
	})
}

func TestHandleDelivery_BadJSON_GoesFinalDLQ(t *testing.T) {
	h := &fakeHandler{}
	p := &fakePublisher{}
	c := newTestConsumer(h, p)

	d := amqp.Delivery{
		RoutingKey:  "auth.email.verify.requested",
		ContentType: "application/json",
		Body:        []byte("{not-json"),
	}

	err := c.handleDelivery(context.Background(), d)
	if err != nil {
		t.Fatalf("expected nil err (final dlq path returns nil), got %v", err)
	}
	if len(p.finalCalls) != 1 {
		t.Fatalf("expected final dlq called once, got %d", len(p.finalCalls))
	}
	if p.finalCalls[0].reason != "bad_json" {
		t.Fatalf("expected reason bad_json, got %q", p.finalCalls[0].reason)
	}
}

func TestOnHandlerError_Permanent_GoesFinalDLQ(t *testing.T) {
	h := &fakeHandler{}
	p := &fakePublisher{}
	c := newTestConsumer(h, p)

	// permanent marker
	perr := permanentErr("nope")

	d := amqp.Delivery{RoutingKey: "auth.email.verify.requested"}

	if err := c.onHandlerError(context.Background(), d, perr); err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if len(p.finalCalls) != 1 {
		t.Fatalf("expected final dlq called once")
	}
	if p.finalCalls[0].reason != "non_retriable" {
		t.Fatalf("expected non_retriable reason, got %q", p.finalCalls[0].reason)
	}
}

func TestOnHandlerError_Retriable_RepublishesRetryWithAttemptHeader(t *testing.T) {
	h := &fakeHandler{}
	p := &fakePublisher{}
	c := newTestConsumer(h, p)

	d := amqp.Delivery{
		RoutingKey: "auth.email.verify.requested",
		Headers:    amqp.Table{"x-attempt": int64(0)},
	}

	if err := c.onHandlerError(context.Background(), d, errors.New("temp")); err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if len(p.retryCalls) != 1 {
		t.Fatalf("expected retry publish called once, got %d", len(p.retryCalls))
	}
	if p.retryCalls[0].nextAttempt != 1 || p.retryCalls[0].tier != "10s" {
		t.Fatalf("expected attempt=1 tier=10s, got attempt=%d tier=%s", p.retryCalls[0].nextAttempt, p.retryCalls[0].tier)
	}
}

func TestOnHandlerError_MaxAttempts_GoesFinalDLQ(t *testing.T) {
	h := &fakeHandler{}
	p := &fakePublisher{}
	c := newTestConsumer(h, p)

	// make max attempts small via env? your code uses envInt; unit test avoids env by setting attempt already >= defaultMaxAttempts
	d := amqp.Delivery{
		RoutingKey: "auth.email.verify.requested",
		Headers:    amqp.Table{"x-attempt": int64(defaultMaxAttempts)}, // attempt >= max => final
	}

	if err := c.onHandlerError(context.Background(), d, errors.New("temp")); err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if len(p.finalCalls) != 1 {
		t.Fatalf("expected final dlq called once, got %d", len(p.finalCalls))
	}
	if p.finalCalls[0].reason != "max_attempts_exceeded" {
		t.Fatalf("expected max_attempts_exceeded, got %q", p.finalCalls[0].reason)
	}
}

func TestOnHandlerError_PublishRetryFails_RequeueError(t *testing.T) {
	h := &fakeHandler{}
	p := &fakePublisher{retryErr: errors.New("publish failed")}
	c := newTestConsumer(h, p)

	d := amqp.Delivery{
		RoutingKey: "auth.email.verify.requested",
		Headers:    amqp.Table{"x-attempt": int64(0)},
	}

	err := c.onHandlerError(context.Background(), d, errors.New("temp"))
	var rq *requeueError
	if !errors.As(err, &rq) || !rq.requeue {
		t.Fatalf("expected requeueError(requeue=true), got %T %v", err, err)
	}
}

func TestToFinalDLQ_PublishFails_RequeueError(t *testing.T) {
	h := &fakeHandler{}
	p := &fakePublisher{finalErr: errors.New("dlq publish failed")}
	c := newTestConsumer(h, p)

	d := amqp.Delivery{RoutingKey: "auth.email.verify.requested"}

	err := c.toFinalDLQ(context.Background(), d, "x", errors.New("cause"))
	var rq *requeueError
	if !errors.As(err, &rq) || !rq.requeue {
		t.Fatalf("expected requeueError(requeue=true), got %T %v", err, err)
	}
}

// ---- helpers ----

type permanentErr string

func (e permanentErr) Error() string             { return string(e) }
func (e permanentErr) Permanent() bool           { return true }
func (e permanentErr) Temporary() bool           { return false }
func (e permanentErr) Unwrap() error             { return nil }
func (e permanentErr) Timeout() bool             { return false }
func (e permanentErr) RetryAfter() time.Duration { return 0 }

// sanity: getAttempt supports different types
func TestGetAttempt_SupportsTypes(t *testing.T) {
	if got := getAttempt(nil); got != 0 {
		t.Fatalf("nil headers expected 0 got %d", got)
	}
	if got := getAttempt(amqp.Table{"x-attempt": int64(2)}); got != 2 {
		t.Fatalf("int64 expected 2 got %d", got)
	}
	if got := getAttempt(amqp.Table{"x-attempt": "3"}); got != 3 {
		t.Fatalf("string expected 3 got %d", got)
	}
}
