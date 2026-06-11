package messaging

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/baechuer/cityevents/internal/platform/observability"
	amqp "github.com/rabbitmq/amqp091-go"
)

func DeclareTopology(ch *amqp.Channel) error {
	if err := ch.ExchangeDeclare(EventExchange, "topic", true, false, false, false, nil); err != nil {
		return err
	}
	if err := ch.ExchangeDeclare(DeadLetterExchange, "topic", true, false, false, false, nil); err != nil {
		return err
	}
	if _, err := ch.QueueDeclare(FeedQueue, true, false, false, false, amqp.Table{
		"x-dead-letter-exchange": DeadLetterExchange,
	}); err != nil {
		return err
	}
	if _, err := ch.QueueDeclare(FeedDeadLetterQueue, true, false, false, false, nil); err != nil {
		return err
	}
	if _, err := ch.QueueDeclare(NotificationQueue, true, false, false, false, amqp.Table{
		"x-dead-letter-exchange": DeadLetterExchange,
	}); err != nil {
		return err
	}
	if _, err := ch.QueueDeclare(NotificationDeadLetterQueue, true, false, false, false, nil); err != nil {
		return err
	}
	if err := ch.QueueBind(FeedQueue, "event.*", EventExchange, false, nil); err != nil {
		return err
	}
	if err := ch.QueueBind(FeedQueue, "join.*", EventExchange, false, nil); err != nil {
		return err
	}
	for _, routingKey := range []string{"join.confirmed", "join.waitlisted", "join.promoted", "join.canceled", "event.canceled"} {
		if err := ch.QueueBind(NotificationQueue, routingKey, EventExchange, false, nil); err != nil {
			return err
		}
	}
	if err := ch.QueueBind(FeedDeadLetterQueue, "#", DeadLetterExchange, false, nil); err != nil {
		return err
	}
	if err := ch.QueueBind(NotificationDeadLetterQueue, "#", DeadLetterExchange, false, nil); err != nil {
		return err
	}
	return nil
}

type ConfirmingPublisher struct {
	ch       *amqp.Channel
	confirms <-chan amqp.Confirmation
}

func NewConfirmingPublisher(conn *amqp.Connection) (*ConfirmingPublisher, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, err
	}
	if err := DeclareTopology(ch); err != nil {
		_ = ch.Close()
		return nil, err
	}
	if err := ch.Confirm(false); err != nil {
		_ = ch.Close()
		return nil, err
	}
	return &ConfirmingPublisher{
		ch:       ch,
		confirms: ch.NotifyPublish(make(chan amqp.Confirmation, 1)),
	}, nil
}

func (p *ConfirmingPublisher) Close() error {
	return p.ch.Close()
}

func (p *ConfirmingPublisher) Publish(ctx context.Context, routingKey string, envelope Envelope) error {
	body, err := envelope.MarshalJSONBytes()
	if err != nil {
		return err
	}
	headers := traceHeadersFromContext(ctx)
	if err := p.ch.PublishWithContext(ctx, EventExchange, routingKey, false, false, amqp.Publishing{
		DeliveryMode:  amqp.Persistent,
		ContentType:   "application/json",
		Headers:       headers,
		MessageId:     envelope.MessageID,
		CorrelationId: envelope.CorrelationID,
		Timestamp:     envelope.OccurredAt,
		Body:          body,
	}); err != nil {
		return err
	}

	select {
	case confirm, ok := <-p.confirms:
		if !ok {
			return errors.New("publisher confirms channel closed")
		}
		if !confirm.Ack {
			return errors.New("rabbitmq publish was negatively acknowledged")
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(10 * time.Second):
		return errors.New("publisher confirm timeout")
	}
}

func traceHeadersFromContext(ctx context.Context) amqp.Table {
	trace := observability.TraceContextFromContext(ctx)
	if trace.TraceParent == "" {
		return nil
	}
	headers := amqp.Table{
		observability.TraceParentHeader: trace.TraceParent,
	}
	if trace.TraceState != "" {
		headers[observability.TraceStateHeader] = trace.TraceState
	}
	return headers
}

func ContextWithAMQPTraceHeaders(ctx context.Context, headers amqp.Table) context.Context {
	traceParent, _ := headers[observability.TraceParentHeader].(string)
	traceState, _ := headers[observability.TraceStateHeader].(string)
	trace := observability.TraceContext{
		TraceParent: strings.TrimSpace(traceParent),
		TraceState:  strings.TrimSpace(traceState),
	}
	if !observability.ValidTraceParent(trace.TraceParent) {
		return ctx
	}
	return observability.ContextWithTraceContext(ctx, trace)
}
