package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const CorrelationIDHeader = "X-Correlation-ID"

type correlationIDKey struct{}

func CorrelationIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(correlationIDKey{}).(string)
	return value
}

func ContextWithCorrelationID(ctx context.Context, correlationID string) context.Context {
	return context.WithValue(ctx, correlationIDKey{}, strings.TrimSpace(correlationID))
}

func NewCorrelationID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes[:])
}

var metrics = struct {
	sync.Mutex
	httpRequests     map[string]int
	outboxMessages   map[string]int
	consumerMessages map[string]int
}{
	httpRequests:     map[string]int{},
	outboxMessages:   map[string]int{},
	consumerMessages: map[string]int{},
}

func RecordHTTPRequest(service string, status int) {
	metrics.Lock()
	defer metrics.Unlock()
	key := fmt.Sprintf(`service="%s",status="%s"`, labelValue(service), strconv.Itoa(status))
	metrics.httpRequests[key]++
}

func RecordOutboxMessage(routingKey, state string) {
	metrics.Lock()
	defer metrics.Unlock()
	key := fmt.Sprintf(`routing_key="%s",state="%s"`, labelValue(routingKey), labelValue(state))
	metrics.outboxMessages[key]++
}

func RecordConsumerMessage(consumer, routingKey, state string) {
	metrics.Lock()
	defer metrics.Unlock()
	key := fmt.Sprintf(`consumer="%s",routing_key="%s",state="%s"`, labelValue(consumer), labelValue(routingKey), labelValue(state))
	metrics.consumerMessages[key]++
}

func MetricsText() string {
	metrics.Lock()
	defer metrics.Unlock()
	lines := []string{
		"# HELP cityevents_http_requests_total Total HTTP requests handled by service.",
		"# TYPE cityevents_http_requests_total counter",
	}
	appendMetric := func(name string, values map[string]int) {
		keys := make([]string, 0, len(values))
		for key := range values {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			lines = append(lines, fmt.Sprintf("%s{%s} %d", name, key, values[key]))
		}
	}
	appendMetric("cityevents_http_requests_total", metrics.httpRequests)
	lines = append(lines,
		"# HELP cityevents_outbox_messages_total Outbox relay messages by routing key and state.",
		"# TYPE cityevents_outbox_messages_total counter",
	)
	appendMetric("cityevents_outbox_messages_total", metrics.outboxMessages)
	lines = append(lines,
		"# HELP cityevents_consumer_messages_total Consumer messages by consumer, routing key, and state.",
		"# TYPE cityevents_consumer_messages_total counter",
	)
	appendMetric("cityevents_consumer_messages_total", metrics.consumerMessages)
	return strings.Join(lines, "\n") + "\n"
}

func labelValue(value string) string {
	return strings.NewReplacer(`\`, `\\`, "\n", `\n`, `"`, `\"`).Replace(value)
}
