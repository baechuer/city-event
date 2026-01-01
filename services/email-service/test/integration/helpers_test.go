//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
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
	apiPort := os.Getenv("MAILPIT_API_PORT")
	if apiPort == "" {
		apiPort = "8025"
	}
	req, _ := http.NewRequest("DELETE", fmt.Sprintf("http://localhost:%s/api/v1/messages", apiPort), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Logf("failed to clear emails (maybe mailpit not ready?): %v", err)
		return
	}
	defer resp.Body.Close()
}

func waitForEmail(t *testing.T, subject, to string, timeout time.Duration) {
	apiPort := os.Getenv("MAILPIT_API_PORT")
	if apiPort == "" {
		apiPort = "8025"
	}
	start := time.Now()
	for time.Since(start) < timeout {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%s/api/v1/messages", apiPort))
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
	deadline := time.Now().Add(30 * time.Second)

	for time.Now().Before(deadline) {
		conn, err := amqp.Dial(rabbitURL)
		if err != nil {
			t.Logf("waitForQueue: dial failed: %v", err)
			time.Sleep(500 * time.Millisecond)
			continue
		}

		// Channel closes on error, so we must recreate it if it died.
		ch, err := conn.Channel()
		if err != nil {
			t.Logf("waitForQueue: open channel failed: %v", err)
			time.Sleep(500 * time.Millisecond)
			continue
		}

		_, err = ch.QueueDeclarePassive(queueName, true, false, false, false, nil)
		ch.Close() // Close immediately

		if err == nil {
			// Found the queue!
			// Now ensure the binding exists to avoid race where queue exists but binding doesn't.
			// Re-open channel as it was closed by QueueDeclarePassive
			ch, err = conn.Channel()
			if err != nil {
				t.Logf("waitForQueue: failed to re-open channel for binding: %v", err)
				time.Sleep(500 * time.Millisecond)
				continue
			}
			_ = ch.QueueBind(queueName, "auth.email.#", "city.events", false, nil)
			ch.Close()
			conn.Close()
			return
		}

		t.Logf("waitForQueue: %v", err)
		conn.Close() // Close connection if check failed
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("waitForQueue: timed out waiting for queue %q to exist", queueName)
}
