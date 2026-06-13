//go:build integration

package messaging

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

func TestPublishFailedDeliveryRoutesRetryAndDLQ(t *testing.T) {
	rabbitURL := os.Getenv("PHASE4_TEST_RABBITMQ_URL")
	if rabbitURL == "" {
		rabbitURL = "amqp://cityevents:cityevents@localhost:5672/"
	}
	conn, err := amqp.Dial(rabbitURL)
	if err != nil {
		t.Fatalf("connect rabbitmq: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("channel: %v", err)
	}
	t.Cleanup(func() { _ = ch.Close() })
	if err := DeclareTopology(ch); err != nil {
		t.Fatalf("declare topology: %v", err)
	}
	purgeFeedRetryTopology(t, ch)
	if err := ch.Confirm(false); err != nil {
		t.Fatalf("confirm mode: %v", err)
	}
	confirms := ch.NotifyPublish(make(chan amqp.Confirmation, 1))

	baseDelivery := amqp.Delivery{
		RoutingKey:    "join.confirmed",
		ContentType:   "application/json",
		MessageId:     "message-retry",
		CorrelationId: "corr-retry",
		Body:          []byte(`{"messageId":"message-retry"}`),
		Headers:       amqp.Table{},
	}
	action, err := PublishFailedDelivery(context.Background(), ch, confirms, FeedQueue, baseDelivery, errors.New("handler failed"))
	if err != nil {
		t.Fatalf("publish retry: %v", err)
	}
	if action != FailedDeliveryRetried {
		t.Fatalf("retry action = %s, want %s", action, FailedDeliveryRetried)
	}
	retryDelivery := getQueueDelivery(t, ch, retryQueueName(FeedQueue, 1))
	if got := DeliveryRetryCount(retryDelivery.Headers); got != 1 {
		t.Fatalf("retry count = %d, want 1", got)
	}
	if got := DeliveryRoutingKey(retryDelivery); got != "join.confirmed" {
		t.Fatalf("original routing key = %s, want join.confirmed", got)
	}

	deadDelivery := baseDelivery
	deadDelivery.MessageId = "message-dead"
	deadDelivery.Headers = amqp.Table{
		RetryCountHeader:      int32(MaxConsumerRetries),
		OriginalRoutingHeader: "join.confirmed",
	}
	action, err = PublishFailedDelivery(context.Background(), ch, confirms, FeedQueue, deadDelivery, errors.New("poison message"))
	if err != nil {
		t.Fatalf("publish dlq: %v", err)
	}
	if action != FailedDeliveryDeadLettered {
		t.Fatalf("dead-letter action = %s, want %s", action, FailedDeliveryDeadLettered)
	}
	dlqDelivery := getQueueDelivery(t, ch, FeedDeadLetterQueue)
	if dlqDelivery.MessageId != "message-dead" {
		t.Fatalf("dlq message id = %s, want message-dead", dlqDelivery.MessageId)
	}
	if got, _ := dlqDelivery.Headers[DeadLetterReasonHeader].(string); got != "max_retries_exhausted" {
		t.Fatalf("dead-letter reason = %q, want max_retries_exhausted", got)
	}
}

func purgeFeedRetryTopology(t *testing.T, ch *amqp.Channel) {
	t.Helper()
	for _, queue := range []string{FeedQueue, FeedDeadLetterQueue} {
		if _, err := ch.QueuePurge(queue, false); err != nil {
			t.Fatalf("purge %s: %v", queue, err)
		}
	}
	for attempt := 1; attempt <= MaxConsumerRetries; attempt++ {
		queue := retryQueueName(FeedQueue, attempt)
		if _, err := ch.QueuePurge(queue, false); err != nil {
			t.Fatalf("purge %s: %v", queue, err)
		}
	}
}

func getQueueDelivery(t *testing.T, ch *amqp.Channel, queue string) amqp.Delivery {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		delivery, ok, err := ch.Get(queue, true)
		if err != nil {
			t.Fatalf("get %s: %v", queue, err)
		}
		if ok {
			return delivery
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", queue)
	return amqp.Delivery{}
}
