package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog"
)

// Handler is the app-layer contract that MQ consumer calls.
type Handler interface {
	VerifyEmail(ctx context.Context, userID, email, url string) error
	PasswordReset(ctx context.Context, userID, email, url string) error
	EventCanceled(ctx context.Context, eventID, userID, reason, prevStatus string) error
}

// Publisher is the MQ publish contract used by Consumer.
// It is an interface so unit tests can inject a fake publisher without real AMQP channels.
type Publisher interface {
	PublishRetry(ctx context.Context, tier string, orig amqp.Delivery, nextAttempt int, cause error) error
	PublishFinal(ctx context.Context, orig amqp.Delivery, reason string, cause error) error
}

type Config struct {
	RabbitURL string
	Exchange  string
	Queue     string
	BindKeys  []string
	Prefetch  int
	Tag       string

	// NEW: used to rewrite outgoing email links (not MQ payload)
	EmailPublicBaseURL string // e.g. http://localhost:8090
}

const (
	// queues
	qDLQ      = "email-service.dlq"
	qRetry10s = "email-service.retry.10s"
	qRetry1m  = "email-service.retry.1m"
	qRetry10m = "email-service.retry.10m"

	defaultMaxAttempts = 5
)

type Consumer struct {
	url      string
	exchange string
	queue    string
	bindKeys []string
	prefetch int
	tag      string

	publicBase string // http://localhost:8090

	lg      zerolog.Logger
	handler Handler

	mu      sync.Mutex
	running bool
	doneCh  chan struct{}

	// AMQP resources
	conn      *amqp.Connection
	chConsume *amqp.Channel
	chPublish *amqp.Channel

	deliveries <-chan amqp.Delivery
	pub        Publisher
}

func NewConsumer(cfg Config, h Handler, lg zerolog.Logger) *Consumer {
	return &Consumer{
		url:        cfg.RabbitURL,
		exchange:   cfg.Exchange,
		queue:      cfg.Queue,
		bindKeys:   cfg.BindKeys,
		prefetch:   cfg.Prefetch,
		tag:        cfg.Tag,
		publicBase: strings.TrimRight(cfg.EmailPublicBaseURL, "/"),
		handler:    h,
		lg:         lg.With().Str("component", "rabbitmq_consumer").Logger(),
	}
}

func (c *Consumer) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return nil
	}
	if c.handler == nil {
		return fmt.Errorf("nil handler")
	}

	c.doneCh = make(chan struct{})
	c.running = true
	go c.run(ctx)
	return nil
}

func (c *Consumer) Stop(ctx context.Context) error {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return nil
	}
	doneCh := c.doneCh
	c.running = false
	c.mu.Unlock()

	c.closeConn()

	select {
	case <-doneCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Consumer) run(ctx context.Context) {
	defer func() {
		c.mu.Lock()
		doneCh := c.doneCh
		c.doneCh = nil
		c.running = false
		c.mu.Unlock()

		if doneCh != nil {
			close(doneCh)
		}
	}()

	backoff := 1 * time.Second
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-ctx.Done():
			c.lg.Info().Msg("consumer supervisor exiting (ctx cancelled)")
			return
		default:
		}

		if !c.isRunning() {
			c.lg.Info().Msg("consumer supervisor exiting (stopped)")
			return
		}

		err := c.connectAndDeclare()
		if err != nil {
			if isPreconditionFailed(err) {
				c.lg.Error().Err(err).Msg("FATAL: topology precondition failed. Delete and recreate MQ resources, then restart.")
				return
			}

			c.lg.Error().Err(err).Dur("backoff", backoff).Msg("connectAndDeclare failed; retrying")
			if !sleepOrDone(ctx, backoff) {
				return
			}
			backoff = minDur(backoff*2, maxBackoff)
			continue
		}

		backoff = 1 * time.Second
		c.consumeLoop(ctx)

		select {
		case <-ctx.Done():
			return
		default:
		}

		c.lg.Warn().Dur("backoff", backoff).Msg("deliveries closed; reconnecting")
		c.closeConn()

		if !sleepOrDone(ctx, backoff) {
			return
		}
		backoff = minDur(backoff*2, maxBackoff)
	}
}

func (c *Consumer) isRunning() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

func (c *Consumer) connectAndDeclare() error {
	c.closeConn()

	conn, err := amqp.Dial(c.url)
	if err != nil {
		return fmt.Errorf("rabbitmq dial: %w", err)
	}

	chConsume, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("consume channel: %w", err)
	}

	chPublish, err := conn.Channel()
	if err != nil {
		_ = chConsume.Close()
		_ = conn.Close()
		return fmt.Errorf("publish channel: %w", err)
	}

	// ---- Declare exchanges ----
	if err := chConsume.ExchangeDeclare(c.exchange, "topic", true, false, false, false, nil); err != nil {
		c.closeAll(conn, chConsume, chPublish)
		return fmt.Errorf("main exchange declare: %w", err)
	}

	for _, ex := range []string{DLX10sExchange, DLX1mExchange, DLX10mExchange, DLXFinalExchange} {
		if err := chConsume.ExchangeDeclare(ex, "topic", true, false, false, false, nil); err != nil {
			c.closeAll(conn, chConsume, chPublish)
			return fmt.Errorf("dlx exchange declare (%s): %w", ex, err)
		}
	}

	// ---- Declare queues ----
	mainArgs := amqp.Table{
		"x-dead-letter-exchange":    DLXFinalExchange,
		"x-dead-letter-routing-key": rkFinalDLQ,
	}
	if _, err := chConsume.QueueDeclare(c.queue, true, false, false, false, mainArgs); err != nil {
		c.closeAll(conn, chConsume, chPublish)
		return fmt.Errorf("main queue declare: %w", err)
	}

	for _, key := range c.bindKeys {
		k := strings.TrimSpace(key)
		if k == "" {
			continue
		}
		if err := chConsume.QueueBind(c.queue, k, c.exchange, false, nil); err != nil {
			c.closeAll(conn, chConsume, chPublish)
			return fmt.Errorf("main queue bind (%s): %w", k, err)
		}
	}

	if _, err := chConsume.QueueDeclare(qDLQ, true, false, false, false, nil); err != nil {
		c.closeAll(conn, chConsume, chPublish)
		return fmt.Errorf("dlq queue declare: %w", err)
	}
	if err := chConsume.QueueBind(qDLQ, rkFinalDLQ, DLXFinalExchange, false, nil); err != nil {
		c.closeAll(conn, chConsume, chPublish)
		return fmt.Errorf("dlq queue bind: %w", err)
	}

	if err := declareRetryQueue(chConsume, qRetry10s, DLX10sExchange, 10*time.Second, c.exchange); err != nil {
		c.closeAll(conn, chConsume, chPublish)
		return err
	}
	if err := declareRetryQueue(chConsume, qRetry1m, DLX1mExchange, 1*time.Minute, c.exchange); err != nil {
		c.closeAll(conn, chConsume, chPublish)
		return err
	}
	if err := declareRetryQueue(chConsume, qRetry10m, DLX10mExchange, 10*time.Minute, c.exchange); err != nil {
		c.closeAll(conn, chConsume, chPublish)
		return err
	}

	if c.prefetch > 0 {
		if err := chConsume.Qos(c.prefetch, 0, false); err != nil {
			c.closeAll(conn, chConsume, chPublish)
			return fmt.Errorf("qos: %w", err)
		}
	}

	dlv, err := chConsume.Consume(c.queue, c.tag, false, false, false, false, nil)
	if err != nil {
		c.closeAll(conn, chConsume, chPublish)
		return fmt.Errorf("consume: %w", err)
	}

	pub, err := NewRetryPublisher(chPublish, c.lg)
	if err != nil {
		c.closeAll(conn, chConsume, chPublish)
		return fmt.Errorf("retry publisher: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.chConsume = chConsume
	c.chPublish = chPublish
	c.deliveries = dlv
	c.pub = pub
	c.mu.Unlock()

	c.lg.Info().
		Str("exchange", c.exchange).
		Str("queue", c.queue).
		Strs("bind_keys", c.bindKeys).
		Int("prefetch", c.prefetch).
		Str("public_base", c.publicBase).
		Msg("rabbitmq consumer ready (separate consume/publish channels; confirm+mandatory enabled)")

	return nil
}

func declareRetryQueue(ch *amqp.Channel, qName, tierExchange string, ttl time.Duration, mainExchange string) error {
	args := amqp.Table{
		"x-message-ttl":          int64(ttl / time.Millisecond),
		"x-dead-letter-exchange": mainExchange,
	}
	if _, err := ch.QueueDeclare(qName, true, false, false, false, args); err != nil {
		return fmt.Errorf("retry queue declare (%s): %w", qName, err)
	}
	if err := ch.QueueBind(qName, "#", tierExchange, false, nil); err != nil {
		return fmt.Errorf("retry queue bind (%s): %w", qName, err)
	}
	return nil
}

func (c *Consumer) consumeLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			c.lg.Info().Msg("consume loop context cancelled")
			return

		case d, ok := <-c.deliveries:
			if !ok {
				c.lg.Warn().Msg("deliveries channel closed")
				return
			}

			start := time.Now()
			err := c.handleDelivery(ctx, d)

			if err == nil {
				_ = d.Ack(false)
				c.lg.Info().Str("routing_key", d.RoutingKey).Dur("took", time.Since(start)).Msg("message processed")
				continue
			}

			var rerr *requeueError
			if errors.As(err, &rerr) && rerr.requeue {
				_ = d.Nack(false, true)
				c.lg.Warn().Err(err).Str("routing_key", d.RoutingKey).Msg("handle failed; requeue=true")
				continue
			}

			_ = d.Nack(false, false)
			c.lg.Error().Err(err).Str("routing_key", d.RoutingKey).Msg("handle failed; nack requeue=false (safety DLX -> final DLQ)")
		}
	}
}

// ---- payloads ----
type VerifyEmailEvent struct {
	UserID string `json:"UserID"`
	Email  string `json:"Email"`
	URL    string `json:"URL"`
}

type PasswordResetEvent struct {
	UserID string `json:"UserID"`
	Email  string `json:"Email"`
	URL    string `json:"URL"`
}

func (c *Consumer) handleDelivery(ctx context.Context, d amqp.Delivery) error {
	rk := strings.TrimSpace(d.RoutingKey)

	switch rk {
	case "auth.email.verify.requested":
		var evt VerifyEmailEvent
		if err := json.Unmarshal(d.Body, &evt); err != nil {
			return c.toFinalDLQ(ctx, d, "bad_json", err)
		}

		// ✅ Rewrite email link to 8090 (don't change MQ payload)
		link := c.rewriteURL(rk, evt.URL)

		if err := c.handler.VerifyEmail(ctx, evt.UserID, evt.Email, link); err != nil {
			return c.onHandlerError(ctx, d, err)
		}
		return nil

	case "auth.password.reset.requested":
		var evt PasswordResetEvent
		if err := json.Unmarshal(d.Body, &evt); err != nil {
			return c.toFinalDLQ(ctx, d, "bad_json", err)
		}

		link := c.rewriteURL(rk, evt.URL)

		if err := c.handler.PasswordReset(ctx, evt.UserID, evt.Email, link); err != nil {
			return c.onHandlerError(ctx, d, err)
		}
		return nil

	case "email.event_canceled":
		// Payload from join-service:
		// {"event_id": "...", "user_id": "...", "reason": "...", "prev_status": "..."}

		// We define a struct for this or use map. Let's use a struct.
		// Since we don't have contracts package imported here, and I don't want to break imports,
		// I will define a local struct or map. Local struct is safer.
		type EventCanceledPayload struct {
			EventID    string `json:"event_id"`
			UserID     string `json:"user_id"`
			Reason     string `json:"reason"`
			PrevStatus string `json:"prev_status"`
		}
		var evt EventCanceledPayload
		if err := json.Unmarshal(d.Body, &evt); err != nil {
			return c.toFinalDLQ(ctx, d, "bad_json", err)
		}

		// Validation
		if evt.EventID == "" || evt.UserID == "" {
			c.lg.Warn().Msg("email.event_canceled missing fields; dropping")
			return nil
		}

		if err := c.handler.EventCanceled(ctx, evt.EventID, evt.UserID, evt.Reason, evt.PrevStatus); err != nil {
			return c.onHandlerError(ctx, d, err)
		}
		return nil

	default:
		// HARDENING: Drop (Ack) unknown messages to prevent DLQ flooding (DoS risk).
		// We do NOT log the body, only the routing key (sanitized).
		safeRK := truncateString(rk, 100) // Prevent log flooding with massive keys
		c.lg.Warn().
			Str("routing_key", safeRK).
			Str("decision", "drop_ack").
			Msg("unknown routing key; dropping to prevent head-of-line blocking")
		return nil
	}
}

// rewriteURL converts auth-service link -> email-service web page link.
func (c *Consumer) rewriteURL(routingKey, original string) string {
	base := strings.TrimRight(c.publicBase, "/")
	if base == "" {
		// fallback
		base = "http://localhost:8090"
	}

	token := extractToken(original)
	if token == "" {
		return original
	}

	var path string
	switch routingKey {
	case "auth.email.verify.requested":
		path = "/verify"
	case "auth.password.reset.requested":
		path = "/reset"
	default:
		return original
	}

	return fmt.Sprintf("%s%s?token=%s", base, path, url.QueryEscape(token))
}

func extractToken(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(u.Query().Get("token"))
}

func (c *Consumer) onHandlerError(ctx context.Context, d amqp.Delivery, err error) error {
	if isNonRetriable(err) {
		return c.toFinalDLQ(ctx, d, "non_retriable", err)
	}

	attempt := getAttempt(d.Headers)
	maxAttempts := envInt("EMAIL_MAX_ATTEMPTS", defaultMaxAttempts)
	if attempt >= maxAttempts {
		return c.toFinalDLQ(ctx, d, "max_attempts_exceeded", err)
	}

	nextAttempt := attempt + 1
	tier := retryTier(nextAttempt)

	if c.pub == nil {
		return requeue(fmt.Errorf("nil retry publisher"))
	}
	if pubErr := c.pub.PublishRetry(ctx, tier, d, nextAttempt, err); pubErr != nil {
		return requeue(fmt.Errorf("republish retry failed: %w", pubErr))
	}

	c.lg.Warn().
		Int("attempt", nextAttempt).
		Str("routing_key", d.RoutingKey).
		Str("tier", tier).
		Msg("retriable failure: republished to retry tier (confirm+mandatory enabled)")

	return nil
}

func (c *Consumer) toFinalDLQ(ctx context.Context, d amqp.Delivery, reason string, cause error) error {
	if c.pub == nil {
		return requeue(fmt.Errorf("nil retry publisher"))
	}
	if pubErr := c.pub.PublishFinal(ctx, d, reason, cause); pubErr != nil {
		return requeue(fmt.Errorf("republish dlq failed: %w", pubErr))
	}
	c.lg.Error().Str("reason", reason).Err(cause).Msg("sent to final DLQ")
	return nil
}

func retryTier(nextAttempt int) string {
	switch {
	case nextAttempt <= 1:
		return "10s"
	case nextAttempt == 2:
		return "1m"
	default:
		return "10m"
	}
}

func getAttempt(h amqp.Table) int {
	if h == nil {
		return 0
	}
	v, ok := h["x-attempt"]
	if !ok {
		return 0
	}
	switch t := v.(type) {
	case int:
		return t
	case int32:
		return int(t)
	case int64:
		return int(t)
	case float64:
		return int(t)
	case string:
		n, _ := strconv.Atoi(t)
		return n
	default:
		return 0
	}
}

func isNonRetriable(err error) bool {
	var per interface{ Permanent() bool }
	if errors.As(err, &per) && per.Permanent() {
		return true
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	return false
}

type requeueError struct {
	err     error
	requeue bool
}

func (e *requeueError) Error() string { return e.err.Error() }
func (e *requeueError) Unwrap() error { return e.err }

func requeue(err error) error { return &requeueError{err: err, requeue: true} }

func envInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, e := strconv.Atoi(v)
	if e != nil || n <= 0 {
		return def
	}
	return n
}

func sleepOrDone(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func minDur(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func (c *Consumer) closeAll(conn *amqp.Connection, a *amqp.Channel, b *amqp.Channel) {
	if b != nil {
		_ = b.Close()
	}
	if a != nil {
		_ = a.Close()
	}
	if conn != nil {
		_ = conn.Close()
	}
}

func (c *Consumer) closeConn() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.chPublish != nil {
		_ = c.chPublish.Close()
		c.chPublish = nil
	}
	if c.chConsume != nil {
		_ = c.chConsume.Close()
		c.chConsume = nil
	}
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}

	c.deliveries = nil
	c.pub = nil
}

func truncateString(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

func isPreconditionFailed(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToUpper(err.Error())
	return strings.Contains(msg, "PRECONDITION_FAILED") || strings.Contains(msg, "INEQUIVALENT ARG")
}
