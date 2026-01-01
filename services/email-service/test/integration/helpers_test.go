//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Mailpit API response structs
type MailpitMessages struct {
	Total    int              `json:"total"`
	Messages []MailpitMessage `json:"messages"`
}

type MailpitMessage struct {
	ID      string    `json:"ID"`
	Subject string    `json:"Subject"`
	To      []Address `json:"To"`
}

type Address struct {
	Name  string `json:"Name"`
	Email string `json:"Address"`
}

func deleteAllEmails(t *testing.T) {
	req, _ := http.NewRequest("DELETE", "http://localhost:8025/api/v1/messages", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Logf("failed to clear emails (maybe mailpit not ready?): %v", err)
		return
	}
	defer resp.Body.Close()
}

func waitForEmail(t *testing.T, subject, to string, timeout time.Duration) {
	start := time.Now()
	for time.Since(start) < timeout {
		resp, err := http.Get("http://localhost:8025/api/v1/messages")
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}

		var result MailpitMessages
		json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()

		for _, msg := range result.Messages {
			if msg.Subject == subject {
				for _, recp := range msg.To {
					if recp.Email == to {
						return // Found it!
					}
				}
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for email to %s with subject %q", to, subject)
}

func publishEvent(t *testing.T, rabbitURL, exchange, key string, body interface{}) {
	conn, err := amqp.Dial(rabbitURL)
	if err != nil {
		t.Fatalf("failed to connect to rabbitmq for publishing: %v", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("failed to open channel: %v", err)
	}
	defer ch.Close()

	// Ensure exchange exists (though service should declare it, we might race if we publish too fast, but usually service starts first)
	// We'll rely on the service `NewApp` having declared it, OR we declare passive.
	// For robustness, let's just publish.

	bytes, _ := json.Marshal(body)
	err = ch.PublishWithContext(context.Background(),
		exchange,
		key,
		false, false,
		amqp.Publishing{
			ContentType: "application/json",
			Body:        bytes,
		},
	)
	if err != nil {
		t.Fatalf("failed to publish event: %v", err)
	}
}

func waitForQueue(t *testing.T, rabbitURL, queueName string) {
	conn, err := amqp.Dial(rabbitURL)
	if err != nil {
		t.Fatalf("waitForQueue: failed to connect to rabbitmq: %v", err)
	}
	defer conn.Close()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		// Channel closes on error, so we must recreate it if it died.
		ch, err := conn.Channel()
		if err != nil {
			t.Logf("waitForQueue: open channel failed: %v", err)
			time.Sleep(500 * time.Millisecond)
			continue
		}

		// Use Active Declare to force it if missing (idempotent if matches)
		_, err = ch.QueueDeclare(queueName, true, false, false, false, nil)
		if err != nil {
			t.Logf("waitForQueue: active declare failed: %v", err)
			ch.Close()
			time.Sleep(500 * time.Millisecond)
			continue
		}

		// Force binding to ensure routing works (Consumer should do this, but it seems flaky?)
		// We bind to "city.events" with "auth.email.#" (standard) or just "#" for test.
		err = ch.QueueBind(queueName, "auth.email.#", "city.events", false, nil)
		if err != nil {
			t.Logf("waitForQueue: bind failed: %v", err)
			ch.Close()
			time.Sleep(500 * time.Millisecond)
			continue
		}

		ch.Close() // Close immediately

		return // Queue exists and bound!
	}
	t.Fatalf("waitForQueue: timed out waiting for queue %q to exist", queueName)
}
