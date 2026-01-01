package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/baechuer/real-time-ressys/services/auth-service/internal/application/auth"
)

const (
	DefaultExchange = "city.events"

	// Minimum window to wait for Return / Confirm.
	publishWait = 150 * time.Millisecond
)

type Publisher struct {
	url      string
	exchange string

	mu sync.Mutex

	conn *amqp.Connection
	ch   *amqp.Channel

	confirmCh <-chan amqp.Confirmation
	returnCh  <-chan amqp.Return
}

func NewPublisher(url string) (*Publisher, error) {
	p := &Publisher{
		url:      url,
		exchange: DefaultExchange,
	}
	if err := p.connect(); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *Publisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.ch != nil {
		_ = p.ch.Close()
		p.ch = nil
	}
	if p.conn != nil {
		_ = p.conn.Close()
		p.conn = nil
	}
	return nil
}

// ---- auth.EventPublisher ----

func (p *Publisher) PublishVerifyEmail(ctx context.Context, evt auth.VerifyEmailEvent) error {
	return p.publishJSON(ctx, "auth.email.verify.requested", evt)
}

func (p *Publisher) PublishPasswordReset(ctx context.Context, evt auth.PasswordResetEvent) error {
	return p.publishJSON(ctx, "auth.password.reset.requested", evt)
}

// ---- internal ----

func (p *Publisher) connect() error {
	conn, err := amqp.Dial(p.url)
	if err != nil {
		return fmt.Errorf("rabbitmq dial: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("rabbitmq channel: %w", err)
	}

	// Declare topic exchange (idempotent).
	if err := ch.ExchangeDeclare(
		p.exchange,
		"topic",
		true,  // durable
		false, // auto-delete
		false,
		false,
		nil,
	); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return fmt.Errorf("exchange declare: %w", err)
	}

	// Enable confirm mode.
	if err := ch.Confirm(false); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return fmt.Errorf("confirm mode: %w", err)
	}

	p.confirmCh = ch.NotifyPublish(make(chan amqp.Confirmation, 1))
	p.returnCh = ch.NotifyReturn(make(chan amqp.Return, 1))

	p.conn = conn
	p.ch = ch
	return nil
}

func (p *Publisher) ensureConnected() error {
	if p.conn != nil && !p.conn.IsClosed() && p.ch != nil {
		return nil
	}
	return p.connect()
}

func (p *Publisher) publishJSON(ctx context.Context, routingKey string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	// Ensure there is a deadline to avoid blocking forever.
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.ensureConnected(); err != nil {
		return err
	}

	// Drain any stale confirm / return messages to avoid mixing results.
drain:
	for {
		select {
		case <-p.confirmCh:
		case <-p.returnCh:
		default:
			break drain
		}
	}

	// mandatory = true
	if err := p.ch.PublishWithContext(
		ctx,
		p.exchange,
		routingKey,
		true,  // mandatory
		false, // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Timestamp:    time.Now(),
			Body:         body,
		},
	); err != nil {
		// Publish call itself failed (channel/connection level error).
		p.resetConn()
		return fmt.Errorf("publish failed: %w", err)
	}

	// Wait for Return / Confirm / Timeout.
	select {
	case ret := <-p.returnCh:
		// No queue is bound for this routing key.
		return fmt.Errorf(
			"rabbitmq unroutable: key=%s code=%d text=%s",
			routingKey, ret.ReplyCode, ret.ReplyText,
		)

	case conf := <-p.confirmCh:
		fmt.Printf("DEBUG: Ack received. Tag=%d Ack=%v\n", conf.DeliveryTag, conf.Ack)
		// Mandatory delivery: Return usually arrives before Ack.
		// Check returnCh non-blockingly to avoid race if both are ready.
		// Wait a tiny bit to ensure the Return frame is processed if it exists.
		time.Sleep(1000 * time.Millisecond)
		select {
		case ret := <-p.returnCh:
			fmt.Printf("DEBUG: Return captured after sleep. Code=%d\n", ret.ReplyCode)
			return fmt.Errorf(
				"rabbitmq unroutable: key=%s code=%d text=%s",
				routingKey, ret.ReplyCode, ret.ReplyText,
			)
		default:
			fmt.Println("DEBUG: No Return captured after sleep.")
		}

		if !conf.Ack {
			return fmt.Errorf("rabbitmq nack: key=%s deliveryTag=%d", routingKey, conf.DeliveryTag)
		}
		// Ack means the broker has accepted the message (and persisted it for durable queues).
		return nil

	case <-time.After(publishWait):
		fmt.Printf("DEBUG: publish timeout. key=%s\n", routingKey)
		// With confirm+mandatory, this usually only happens when the broker is extremely slow or unhealthy.
		return fmt.Errorf("rabbitmq publish timeout: key=%s", routingKey)

	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *Publisher) resetConn() {
	if p.ch != nil {
		_ = p.ch.Close()
		p.ch = nil
	}
	if p.conn != nil {
		_ = p.conn.Close()
		p.conn = nil
	}
}
