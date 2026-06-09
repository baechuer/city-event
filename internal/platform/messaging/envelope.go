package messaging

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const (
	EventExchange               = "cityevents.events"
	DeadLetterExchange          = "cityevents.dlx"
	FeedQueue                   = "cityevents.feed.projection"
	FeedDeadLetterQueue         = "cityevents.feed.projection.dlq"
	NotificationQueue           = "cityevents.notification.delivery"
	NotificationDeadLetterQueue = "cityevents.notification.delivery.dlq"
)

type Envelope struct {
	MessageID     string         `json:"messageId"`
	RoutingKey    string         `json:"routingKey"`
	AggregateType string         `json:"aggregateType"`
	AggregateID   string         `json:"aggregateId"`
	OccurredAt    time.Time      `json:"occurredAt"`
	CorrelationID string         `json:"correlationId,omitempty"`
	Payload       map[string]any `json:"payload"`
}

func NewEnvelope(messageID, routingKey, aggregateType, aggregateID, correlationID string, occurredAt time.Time, payload map[string]any) (Envelope, error) {
	envelope := Envelope{
		MessageID:     strings.TrimSpace(messageID),
		RoutingKey:    strings.TrimSpace(routingKey),
		AggregateType: strings.TrimSpace(aggregateType),
		AggregateID:   strings.TrimSpace(aggregateID),
		OccurredAt:    occurredAt.UTC(),
		CorrelationID: strings.TrimSpace(correlationID),
		Payload:       payload,
	}
	if err := envelope.Validate(); err != nil {
		return Envelope{}, err
	}
	return envelope, nil
}

func (e Envelope) Validate() error {
	if e.MessageID == "" {
		return errors.New("message id is required")
	}
	if e.RoutingKey == "" {
		return errors.New("routing key is required")
	}
	if e.AggregateType == "" {
		return errors.New("aggregate type is required")
	}
	if e.AggregateID == "" {
		return errors.New("aggregate id is required")
	}
	if e.OccurredAt.IsZero() {
		return errors.New("occurred at is required")
	}
	if e.Payload == nil {
		return errors.New("payload is required")
	}
	return nil
}

func (e Envelope) MarshalJSONBytes() ([]byte, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(e)
}

func DecodeEnvelope(body []byte) (Envelope, error) {
	var envelope Envelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return Envelope{}, err
	}
	if err := envelope.Validate(); err != nil {
		return Envelope{}, err
	}
	return envelope, nil
}
