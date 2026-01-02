package rabbitmq_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/baechuer/real-time-ressys/services/event-service/internal/application/event"
	"github.com/baechuer/real-time-ressys/services/event-service/internal/domain"
	"github.com/baechuer/real-time-ressys/services/event-service/internal/infrastructure/db/postgres"
	"github.com/baechuer/real-time-ressys/services/event-service/internal/infrastructure/messaging/rabbitmq"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConsumer_Integration simulates a full integration test:
// 1. Create an event in Postgres
// 2. Publish a "join.created" message to RabbitMQ
// 3. Wait for the consumer to process it
// 4. Verify participant count increased in Postgres
func TestConsumer_Integration(t *testing.T) {
	if os.Getenv("TEST_INTEGRATION") == "" {
		t.Skip("Skipping integration test (TEST_INTEGRATION not set)")
	}

	// Config
	rabbitURL := os.Getenv("RABBIT_URL")
	if rabbitURL == "" {
		rabbitURL = "amqp://guest:guest@localhost:5672/"
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/event_db?sslmode=disable"
	}
	exchangeName := "events.exchange"

	// 1. Setup DB
	db, err := sql.Open("postgres", dbURL)
	require.NoError(t, err)
	defer db.Close()

	if err := db.Ping(); err != nil {
		t.Skipf("Skipping: database not available: %v", err)
	}

	repo := postgres.New(db)

	// 2. Setup Service (with no-op components for non-critical parts)
	svc := event.New(repo, sysClock{}, &noopCache{}, 0, 0)

	// 3. Create Event
	ctx := context.Background()
	eventID := uuid.New().String()
	newEv := &domain.Event{
		ID:          eventID,
		OwnerID:     uuid.New().String(),
		Title:       "Integration Test Event",
		Description: "Testing RabbitMQ Consumer",
		City:        "Test City",
		Category:    "Tech",
		StartTime:   time.Now().Add(24 * time.Hour),
		EndTime:     time.Now().Add(26 * time.Hour),
		Capacity:    10,
		Status:      domain.StatusPublished,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	err = repo.Create(ctx, newEv)
	require.NoError(t, err)
	defer func() {
		// Cleanup
		db.Exec("DELETE FROM events WHERE id=$1", eventID)
	}()

	// 4. Setup RabbitMQ Connection & Publisher
	conn, err := amqp.Dial(rabbitURL)
	require.NoError(t, err)
	defer conn.Close()

	ch, err := conn.Channel()
	require.NoError(t, err)
	defer ch.Close()

	err = ch.ExchangeDeclare(
		exchangeName,
		"topic",
		true,
		false,
		false,
		false,
		nil,
	)
	require.NoError(t, err)

	// 5. Start Consumer
	// We create a fresh consumer instance for testing
	consumer, err := rabbitmq.NewConsumer(rabbitURL, exchangeName, svc)
	require.NoError(t, err)

	// Run consumer in background
	go consumer.Start(ctx)
	// Give it a moment to connect and bind
	time.Sleep(1 * time.Second)

	// 6. Publish "join.created" Message
	joinMsg := map[string]string{
		"event_id": eventID,
		"user_id":  uuid.New().String(),
		"join_id":  uuid.New().String(),
	}
	body, _ := json.Marshal(joinMsg)

	err = ch.PublishWithContext(ctx,
		exchangeName,
		"join.created", // Routing Key
		false,
		false,
		amqp.Publishing{
			ContentType: "application/json",
			MessageId:   uuid.New().String(),
			Body:        body,
		},
	)
	require.NoError(t, err)

	// 7. Wait & Verify
	// Poll DB for update
	assert.Eventually(t, func() bool {
		_, err := repo.GetByID(ctx, eventID)
		if err != nil {
			return false
		}
		// Participant count is not directly exposed on domain.Event in default get?
		// Wait, repo.GetByID fetches columns including active_participants?
		// Let's check postgres repo GetByID implementation.
		// It reads fields: id, owner_id, ...
		// If DB schema has active_participants but domain struct doesn't have it explicitly mapped in Scan?
		// I must double check domain.Event and repo.GetByID.

		// Assuming domain.Event DOES NOT have ActiveParticipants field yet in my codebase struct?
		// I added IncrementParticipantCount but did I update domain.Event struct?

		// Let's just query DB directly for verification to be safe.
		var count int
		err = db.QueryRow("SELECT active_participants FROM events WHERE id=$1", eventID).Scan(&count)
		if err != nil {
			return false
		}
		return count == 1
	}, 10*time.Second, 500*time.Millisecond, "Participant count should become 1")

	// 8. Test Decrement (join.canceled)
	err = ch.PublishWithContext(ctx,
		exchangeName,
		"join.canceled",
		false,
		false,
		amqp.Publishing{
			ContentType: "application/json",
			MessageId:   uuid.New().String(),
			Body:        body,
		},
	)
	require.NoError(t, err)

	assert.Eventually(t, func() bool {
		var count int
		err = db.QueryRow("SELECT active_participants FROM events WHERE id=$1", eventID).Scan(&count)
		return err == nil && count == 0
	}, 10*time.Second, 500*time.Millisecond, "Participant count should return to 0")
}

// Helpers

type sysClock struct{}

func (s sysClock) Now() time.Time { return time.Now() }

type noopCache struct{}

func (n *noopCache) Get(ctx context.Context, key string, dest any) (bool, error) { return false, nil }
func (n *noopCache) Set(ctx context.Context, key string, val any, ttl time.Duration) error {
	return nil
}
func (n *noopCache) Delete(ctx context.Context, keys ...string) error { return nil }
