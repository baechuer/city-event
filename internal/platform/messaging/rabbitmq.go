package messaging

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/baechuer/cityevents/internal/platform/observability"
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	RetryCountHeader       = "x-cityevents-retry-count"
	RetryDelayMillisHeader = "x-cityevents-retry-delay-ms"
	LastErrorHeader        = "x-cityevents-last-error"
	OriginalRoutingHeader  = "x-cityevents-original-routing-key"
	DeadLetterReasonHeader = "x-cityevents-dead-letter-reason"
	MaxConsumerRetries     = 5
)

var consumerRetryDelays = []time.Duration{
	5 * time.Second,
	30 * time.Second,
	2 * time.Minute,
	10 * time.Minute,
	30 * time.Minute,
}

type FailedDeliveryAction string

const (
	FailedDeliveryRetried      FailedDeliveryAction = "retried"
	FailedDeliveryDeadLettered FailedDeliveryAction = "dead_lettered"
)

type retryPolicy struct {
	queue                string
	retryRoutingKey      string
	deadLetterRoutingKey string
	eventBindings        []string
}

var retryPolicies = map[string]retryPolicy{
	FeedQueue: {
		queue:                FeedQueue,
		retryRoutingKey:      FeedQueue + ".retry",
		deadLetterRoutingKey: FeedDeadLetterQueue,
		eventBindings:        []string{"event.*", "join.*"},
	},
	NotificationQueue: {
		queue:                NotificationQueue,
		retryRoutingKey:      NotificationQueue + ".retry",
		deadLetterRoutingKey: NotificationDeadLetterQueue,
		eventBindings:        []string{"join.confirmed", "join.waitlisted", "join.promoted", "join.canceled", "event.canceled"},
	},
}

func DeclareTopology(ch *amqp.Channel) error {
	if err := ch.ExchangeDeclare(EventExchange, "topic", true, false, false, false, nil); err != nil {
		return err
	}
	if err := ch.ExchangeDeclare(DeadLetterExchange, "topic", true, false, false, false, nil); err != nil {
		return err
	}
	if err := ch.ExchangeDeclare(RetryExchange, "direct", true, false, false, false, nil); err != nil {
		return err
	}
	if err := ch.ExchangeDeclare(RetryReturnExchange, "direct", true, false, false, false, nil); err != nil {
		return err
	}
	for _, policy := range []retryPolicy{retryPolicies[FeedQueue], retryPolicies[NotificationQueue]} {
		if err := declareConsumerTopology(ch, policy); err != nil {
			return err
		}
	}
	return nil
}

func declareConsumerTopology(ch *amqp.Channel, policy retryPolicy) error {
	if _, err := ch.QueueDeclare(policy.queue, true, false, false, false, amqp.Table{
		"x-dead-letter-exchange": DeadLetterExchange,
	}); err != nil {
		return err
	}
	if _, err := ch.QueueDeclare(policy.deadLetterRoutingKey, true, false, false, false, nil); err != nil {
		return err
	}
	for _, binding := range policy.eventBindings {
		if err := ch.QueueBind(policy.queue, binding, EventExchange, false, nil); err != nil {
			return err
		}
	}
	if err := ch.QueueBind(policy.queue, policy.queue, RetryReturnExchange, false, nil); err != nil {
		return err
	}
	for _, binding := range deadLetterBindings(policy) {
		if err := ch.QueueBind(policy.deadLetterRoutingKey, binding, DeadLetterExchange, false, nil); err != nil {
			return err
		}
	}
	for attempt, delay := range consumerRetryDelays {
		queueName := retryQueueName(policy.queue, attempt+1)
		if _, err := ch.QueueDeclare(queueName, true, false, false, false, amqp.Table{
			"x-message-ttl":             int64(delay / time.Millisecond),
			"x-dead-letter-exchange":    RetryReturnExchange,
			"x-dead-letter-routing-key": policy.queue,
		}); err != nil {
			return err
		}
		if err := ch.QueueBind(queueName, retryRoutingKey(policy.queue, attempt+1), RetryExchange, false, nil); err != nil {
			return err
		}
	}
	return nil
}

func deadLetterBindings(policy retryPolicy) []string {
	seen := map[string]struct{}{}
	bindings := make([]string, 0, len(policy.eventBindings)+2)
	for _, binding := range append([]string{policy.deadLetterRoutingKey, policy.queue}, policy.eventBindings...) {
		if _, ok := seen[binding]; ok {
			continue
		}
		seen[binding] = struct{}{}
		bindings = append(bindings, binding)
	}
	return bindings
}

func ConsumerRetryDelays() []time.Duration {
	delays := make([]time.Duration, len(consumerRetryDelays))
	copy(delays, consumerRetryDelays)
	return delays
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

func PublishFailedDelivery(ctx context.Context, ch *amqp.Channel, confirms <-chan amqp.Confirmation, queue string, delivery amqp.Delivery, cause error) (FailedDeliveryAction, error) {
	policy, ok := retryPolicies[queue]
	if !ok {
		return "", fmt.Errorf("no retry policy for queue %q", queue)
	}
	retryCount := DeliveryRetryCount(delivery.Headers)
	if retryCount >= MaxConsumerRetries {
		headers := failureHeaders(delivery.Headers, retryCount, 0, cause)
		headers[DeadLetterReasonHeader] = "max_retries_exhausted"
		if err := publishWithConfirm(ctx, ch, confirms, DeadLetterExchange, policy.deadLetterRoutingKey, delivery, headers); err != nil {
			return "", err
		}
		return FailedDeliveryDeadLettered, nil
	}

	nextRetry := retryCount + 1
	delay := consumerRetryDelays[nextRetry-1]
	headers := failureHeaders(delivery.Headers, nextRetry, delay, cause)
	if err := publishWithConfirm(ctx, ch, confirms, RetryExchange, retryRoutingKey(queue, nextRetry), delivery, headers); err != nil {
		return "", err
	}
	return FailedDeliveryRetried, nil
}

func DeliveryRetryCount(headers amqp.Table) int {
	if headers == nil {
		return 0
	}
	switch value := headers[RetryCountHeader].(type) {
	case int:
		return nonNegative(value)
	case int8:
		return nonNegative(int(value))
	case int16:
		return nonNegative(int(value))
	case int32:
		return nonNegative(int(value))
	case int64:
		return nonNegative(int(value))
	case uint8:
		return int(value)
	case uint16:
		return int(value)
	case uint32:
		return int(value)
	case uint64:
		return int(value)
	case string:
		var parsed int
		if _, err := fmt.Sscanf(value, "%d", &parsed); err == nil {
			return nonNegative(parsed)
		}
	}
	return 0
}

func nonNegative(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

func DeliveryRoutingKey(delivery amqp.Delivery) string {
	if value, ok := delivery.Headers[OriginalRoutingHeader].(string); ok {
		if routed := strings.TrimSpace(value); routed != "" {
			return routed
		}
	}
	return delivery.RoutingKey
}

func retryQueueName(queue string, attempt int) string {
	return fmt.Sprintf("%s.retry.%d", queue, attempt)
}

func retryRoutingKey(queue string, attempt int) string {
	policy, ok := retryPolicies[queue]
	if !ok {
		return fmt.Sprintf("%s.retry.%d", queue, attempt)
	}
	return fmt.Sprintf("%s.%d", policy.retryRoutingKey, attempt)
}

func failureHeaders(headers amqp.Table, retryCount int, delay time.Duration, cause error) amqp.Table {
	out := amqp.Table{}
	for key, value := range headers {
		out[key] = value
	}
	out[RetryCountHeader] = int32(retryCount)
	out[RetryDelayMillisHeader] = int64(delay / time.Millisecond)
	if _, ok := out[OriginalRoutingHeader]; !ok {
		out[OriginalRoutingHeader] = ""
	}
	if cause != nil {
		out[LastErrorHeader] = trimHeaderError(cause)
	}
	return out
}

func trimHeaderError(err error) string {
	message := strings.TrimSpace(err.Error())
	if len(message) > 500 {
		return message[:500]
	}
	return message
}

func publishWithConfirm(ctx context.Context, ch *amqp.Channel, confirms <-chan amqp.Confirmation, exchange string, routingKey string, delivery amqp.Delivery, headers amqp.Table) error {
	if headers == nil {
		headers = amqp.Table{}
	}
	if headers[OriginalRoutingHeader] == "" {
		headers[OriginalRoutingHeader] = delivery.RoutingKey
	}
	if err := ch.PublishWithContext(ctx, exchange, routingKey, false, false, amqp.Publishing{
		DeliveryMode:  amqp.Persistent,
		ContentType:   delivery.ContentType,
		Headers:       headers,
		MessageId:     delivery.MessageId,
		CorrelationId: delivery.CorrelationId,
		Timestamp:     time.Now().UTC(),
		Body:          delivery.Body,
	}); err != nil {
		return err
	}
	if confirms == nil {
		return nil
	}
	select {
	case confirm, ok := <-confirms:
		if !ok {
			return errors.New("publisher confirms channel closed")
		}
		if !confirm.Ack {
			return errors.New("failed delivery publish was negatively acknowledged")
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(10 * time.Second):
		return errors.New("failed delivery publish confirm timeout")
	}
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
