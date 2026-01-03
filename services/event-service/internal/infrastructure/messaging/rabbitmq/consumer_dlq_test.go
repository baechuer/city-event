//go:build integration

package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/baechuer/real-time-ressys/services/event-service/internal/application/event"
	"github.com/baechuer/real-time-ressys/services/event-service/internal/domain"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// mockServiceForDLQ is a simple mock of event.Service to control errors
type mockServiceForDLQ struct {
	mock.Mock
}

func (m *mockServiceForDLQ) IncrementParticipantCount(ctx context.Context, eventID uuid.UUID) error {
	args := m.Called(ctx, eventID)
	return args.Error(0)
}

func (m *mockServiceForDLQ) DecrementParticipantCount(ctx context.Context, eventID uuid.UUID) error {
	args := m.Called(ctx, eventID)
	return args.Error(0)
}

// Ensure mockServiceForDLQ implements the necessary interface methods if required by Consumer.
// The consumer depends on *event.Service struct directly in the current implementation?
// Ah, NewConsumer takes *event.Service. We cannot mock a struct easily without interfaces.
// The consumer.go:
// type Consumer struct {
//     service  *event.Service
// }
// This is tight coupling. To test we need to control the Service behavior.
// Ideally, Consumer should depend on an interface like `ParticipantCounter`.
// Since we cannot refactor the main code deeply right now, we might need a workaround or just verify using the REAL Logic but simulating DB failure?
// Simulating DB failure is hard in integration test.
//
// OPTION A: Refactor Consumer to use an interface.
// OPTION B: Use REAL service but mock the REPO inside it?
// The event.New takes a Repo interface. We CAN pass a mock Repo to event.New!
// This is the correct way.

type mockFailingRepo struct {
	// Embed the real interface methods (or stub them)
}

func (m *mockFailingRepo) Create(ctx context.Context, e *domain.Event) error { return nil }
func (m *mockFailingRepo) GetByID(ctx context.Context, id string) (*domain.Event, error) {
	return nil, nil
}
func (m *mockFailingRepo) Update(ctx context.Context, e *domain.Event) error { return nil }
func (m *mockFailingRepo) ListByOwner(ctx context.Context, o string, status string, p, ps int) ([]*domain.Event, int, error) {
	return nil, 0, nil
}
func (m *mockFailingRepo) ListPublicTimeKeyset(ctx context.Context, f event.ListFilter, hasCursor bool, afterStart time.Time, afterID string) ([]*domain.Event, error) {
	return nil, nil
}
func (m *mockFailingRepo) ListPublicRelevanceKeyset(ctx context.Context, f event.ListFilter, hasCursor bool, afterRank float64, afterStart time.Time, afterID string) ([]*domain.Event, []float64, error) {
	return nil, nil, nil
}
func (m *mockFailingRepo) WithTx(ctx context.Context, fn func(r event.TxEventRepo) error) error {
	return fn(m)
}

// Transaction methods
func (m *mockFailingRepo) GetByIDForUpdate(ctx context.Context, id string) (*domain.Event, error) {
	return nil, nil
}
func (m *mockFailingRepo) InsertOutbox(ctx context.Context, msg event.OutboxMessage) error {
	return nil
}

// The target methods for consumer
func (m *mockFailingRepo) IncrementParticipantCount(ctx context.Context, eventID uuid.UUID) error {
	return errors.New("simulated transient error")
}
func (m *mockFailingRepo) DecrementParticipantCount(ctx context.Context, eventID uuid.UUID) error {
	return errors.New("simulated transient error")
}
func (m *mockFailingRepo) GetCitySuggestions(ctx context.Context, query string, limit int) ([]string, error) {
	return []string{}, nil
}
func (m *mockFailingRepo) GetByIDs(ctx context.Context, ids []string) ([]*domain.Event, error) {
	return []*domain.Event{}, nil
}

func TestConsumer_DLQ_Retry(t *testing.T) {
	if os.Getenv("TEST_INTEGRATION") == "" {
		t.Skip("Skipping integration test (TEST_INTEGRATION not set)")
	}

	rabbitURL := os.Getenv("RABBIT_URL")
	if rabbitURL == "" {
		rabbitURL = "amqp://guest:guest@localhost:5672/"
	}

	// 1. Setup Mock Service with Failing Repo
	failingRepo := &mockFailingRepo{}
	svc := event.New(failingRepo, nil, nil, 0, 0)

	// Since NewConsumer is setting up the queues, we need to be careful not to conflict with running services.
	// But in test env, we are okay.
	// Use a unique exchange name for test safety? NewConsumer declares "events.exchange" hardcoded?
	// The NewConsumer code allows passing exchange name.
	exchangeName := "test.events.dlq.exchange"

	// 2. Init Consumer
	consumer, err := NewConsumer(rabbitURL, exchangeName, svc)
	require.NoError(t, err)
	defer consumer.Close()

	// Since manual Start() consumes, we might just want to consume manually from retry dict?
	// Or we can let the consumer run and inspect the queues.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go consumer.Start(ctx)
	// Give it time to bind
	time.Sleep(1 * time.Second)

	// 3. Publish message that will FAIL
	ch, err := consumer.conn.Channel()
	require.NoError(t, err)
	defer ch.Close()

	// Need to bind retry queue to default exchange? No, consumer declares that.
	// Need to verify message goes to Retry Queue.

	eventID := uuid.New().String()
	joinMsg := map[string]string{
		"event_id": eventID,
		"user_id":  uuid.New().String(),
	}
	body, _ := json.Marshal(joinMsg)

	// Publish to main exchange
	err = ch.PublishWithContext(ctx,
		exchangeName,
		"join.created",
		false,
		false,
		amqp.Publishing{
			ContentType: "application/json",
			Body:        body,
			MessageId:   uuid.New().String(),
		},
	)
	require.NoError(t, err)

	// 4. Assert Message moves to Retry Queue
	// We consume from "event-service.join-events.retry"
	// Note: QueueName is hardcoded in NewConsumer as "event-service.join-events.retry"
	// This might conflict with real service if running on same RabbitMQ.
	// Ideally we parameterize Queue Name. But for now assuming test env isolation or accepting interference.
	retryQueueName := "event-service.join-events.retry"

	// Wait for retry (consumer handles it immediately on failure)
	t.Log("Waiting for message to appear in Retry Queue...")

	var delivery amqp.Delivery
	assert.Eventually(t, func() bool {
		d, ok, err := ch.Get(retryQueueName, false) // autoAck=false
		if err != nil || !ok {
			return false
		}
		delivery = d
		return true
	}, 5*time.Second, 100*time.Millisecond, "Message should appear in retry queue")

	if delivery.Body != nil {
		// Check headers
		retryCount, ok := delivery.Headers["x-retry-count"].(int32)
		assert.True(t, ok)
		assert.Equal(t, int32(1), retryCount, "Should be retry attempt 1")

		val, ok := delivery.Headers["x-original-routing-key"].(string)
		assert.True(t, ok)
		assert.Equal(t, "join.created", val)

		// Clean up by Acking so it doesn't stay
		// delivery.Ack(false)
		// But wait, if we Ack it, it's gone. If we Nack or let it expire (TTL), it goes back to Main Exchange!
		// The test verified it GOT to the retry queue.
		// That satisfies "Retry Logic".
	}

	// 5. Verify DLQ Transition (Optional/Time dependent)
	// To verify DLQ, we'd need to let it loop 3 times.
	// 5s TTL * 3 = 15s. Plus processing.
	// We can manually injecting a message with header x-retry-count=2 to DLQ test directly.

	t.Log("Testing DLQ Logic direct injection...")
	dlqHeaders := amqp.Table{
		"x-retry-count":          int32(3), // Max reached
		"x-original-routing-key": "join.created",
	}

	err = ch.PublishWithContext(ctx,
		exchangeName,
		"join.created",
		false,
		false,
		amqp.Publishing{
			ContentType: "application/json",
			Body:        body,
			MessageId:   uuid.New().String() + "-dlq-test",
			Headers:     dlqHeaders,
		},
	)
	require.NoError(t, err)

	// Expect it to go to DLQ immediately because consumer sees retry count >= 3
	dlqName := "event-service.join-events.dlq"

	assert.Eventually(t, func() bool {
		d, ok, err := ch.Get(dlqName, true) // autoAck=true to remove
		if err != nil || !ok {
			return false
		}
		return string(d.Body) == string(body)
	}, 5*time.Second, 100*time.Millisecond, "Message should appear in DLQ")
}
