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
	httpRequests         map[string]int
	httpRequestDuration  map[string]durationMetric
	rateLimitedRequests  map[string]int
	rateLimitStoreErrors map[string]int
	outboxMessages       map[string]int
	consumerMessages     map[string]int
}{
	httpRequests:         map[string]int{},
	httpRequestDuration:  map[string]durationMetric{},
	rateLimitedRequests:  map[string]int{},
	rateLimitStoreErrors: map[string]int{},
	outboxMessages:       map[string]int{},
	consumerMessages:     map[string]int{},
}

type durationMetric struct {
	count   int
	sum     float64
	buckets map[float64]int
}

var httpDurationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

func RecordHTTPRequest(service, method, path string, status int, duration time.Duration) {
	metrics.Lock()
	defer metrics.Unlock()
	key := fmt.Sprintf(`service="%s",method="%s",path="%s",status="%s"`, labelValue(service), labelValue(method), labelValue(path), strconv.Itoa(status))
	metrics.httpRequests[key]++

	seconds := duration.Seconds()
	histogram := metrics.httpRequestDuration[key]
	if histogram.buckets == nil {
		histogram.buckets = map[float64]int{}
	}
	histogram.count++
	histogram.sum += seconds
	for _, bucket := range httpDurationBuckets {
		if seconds <= bucket {
			histogram.buckets[bucket]++
		}
	}
	metrics.httpRequestDuration[key] = histogram
}

func RecordRateLimitedRequest(service, scope string) {
	metrics.Lock()
	defer metrics.Unlock()
	key := fmt.Sprintf(`service="%s",scope="%s"`, labelValue(service), labelValue(scope))
	metrics.rateLimitedRequests[key]++
}

func RecordRateLimitStoreError(service, backend, mode string) {
	metrics.Lock()
	defer metrics.Unlock()
	key := fmt.Sprintf(`service="%s",backend="%s",mode="%s"`, labelValue(service), labelValue(backend), labelValue(mode))
	metrics.rateLimitStoreErrors[key]++
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
		"# HELP cityevents_http_request_duration_seconds HTTP request duration histogram by service, method, path, and status.",
		"# TYPE cityevents_http_request_duration_seconds histogram",
	)
	appendDurationMetric(&lines, "cityevents_http_request_duration_seconds", metrics.httpRequestDuration)
	lines = append(lines,
		"# HELP cityevents_rate_limited_requests_total Total HTTP requests rejected by rate limiting.",
		"# TYPE cityevents_rate_limited_requests_total counter",
	)
	appendMetric("cityevents_rate_limited_requests_total", metrics.rateLimitedRequests)
	lines = append(lines,
		"# HELP cityevents_rate_limit_store_errors_total Total rate-limit store errors by backend and configured failure mode.",
		"# TYPE cityevents_rate_limit_store_errors_total counter",
	)
	appendMetric("cityevents_rate_limit_store_errors_total", metrics.rateLimitStoreErrors)
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

func appendDurationMetric(lines *[]string, name string, values map[string]durationMetric) {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		metric := values[key]
		for _, bucket := range httpDurationBuckets {
			*lines = append(*lines, fmt.Sprintf(`%s_bucket{%s,le="%s"} %d`, name, key, bucketLabel(bucket), metric.buckets[bucket]))
		}
		*lines = append(*lines, fmt.Sprintf(`%s_bucket{%s,le="+Inf"} %d`, name, key, metric.count))
		*lines = append(*lines, fmt.Sprintf("%s_sum{%s} %s", name, key, strconv.FormatFloat(metric.sum, 'f', -1, 64)))
		*lines = append(*lines, fmt.Sprintf("%s_count{%s} %d", name, key, metric.count))
	}
}

func bucketLabel(bucket float64) string {
	return strconv.FormatFloat(bucket, 'f', -1, 64)
}

func labelValue(value string) string {
	return strings.NewReplacer(`\`, `\\`, "\n", `\n`, `"`, `\"`).Replace(value)
}
