package audit

import (
	"context"

	"github.com/baechuer/real-time-ressys/services/join-service/internal/domain"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// Logger provides structured audit logging for business events
type Logger struct {
	log zerolog.Logger
}

// New creates a new audit logger
func New(log zerolog.Logger) *Logger {
	return &Logger{
		log: log.With().Bool("audit", true).Logger(),
	}
}

// JoinCreated logs when a user joins an event
func (l *Logger) JoinCreated(ctx context.Context, eventID, userID uuid.UUID, status domain.JoinStatus, idempotencyKey string) {
	l.log.Info().
		Str("action", "join_created").
		Str("event_id", eventID.String()).
		Str("user_id", userID.String()).
		Str("status", string(status)).
		Str("idempotency_key", idempotencyKey).
		Str("trace_id", getTraceID(ctx)).
		Msg("User joined event")
}

// JoinCanceled logs when a user cancels their participation
func (l *Logger) JoinCanceled(ctx context.Context, eventID, userID uuid.UUID, idempotencyKey string) {
	l.log.Info().
		Str("action", "join_canceled").
		Str("event_id", eventID.String()).
		Str("user_id", userID.String()).
		Str("idempotency_key", idempotencyKey).
		Str("trace_id", getTraceID(ctx)).
		Msg("User canceled participation")
}

// Promoted logs when a user is promoted from waitlist to active
func (l *Logger) Promoted(ctx context.Context, eventID, userID uuid.UUID) {
	l.log.Info().
		Str("action", "promoted").
		Str("event_id", eventID.String()).
		Str("user_id", userID.String()).
		Str("trace_id", getTraceID(ctx)).
		Msg("User promoted from waitlist")
}

// Kicked logs when a user is kicked from an event
func (l *Logger) Kicked(ctx context.Context, eventID, targetID, actorID uuid.UUID, reason string) {
	l.log.Warn().
		Str("action", "kicked").
		Str("event_id", eventID.String()).
		Str("target_user_id", targetID.String()).
		Str("actor_user_id", actorID.String()).
		Str("reason", reason).
		Str("trace_id", getTraceID(ctx)).
		Msg("User kicked from event")
}

// Banned logs when a user is banned from an event
func (l *Logger) Banned(ctx context.Context, eventID, targetID, actorID uuid.UUID, reason string) {
	l.log.Warn().
		Str("action", "banned").
		Str("event_id", eventID.String()).
		Str("target_user_id", targetID.String()).
		Str("actor_user_id", actorID.String()).
		Str("reason", reason).
		Str("trace_id", getTraceID(ctx)).
		Msg("User banned from event")
}

// Unbanned logs when a user is unbanned from an event
func (l *Logger) Unbanned(ctx context.Context, eventID, targetID, actorID uuid.UUID) {
	l.log.Info().
		Str("action", "unbanned").
		Str("event_id", eventID.String()).
		Str("target_user_id", targetID.String()).
		Str("actor_user_id", actorID.String()).
		Str("trace_id", getTraceID(ctx)).
		Msg("User unbanned from event")
}

// OutboxMessageSent logs when an outbox message is successfully published
func (l *Logger) OutboxMessageSent(ctx context.Context, messageID, routingKey string) {
	l.log.Debug().
		Str("action", "outbox_sent").
		Str("message_id", messageID).
		Str("routing_key", routingKey).
		Msg("Outbox message sent")
}

// OutboxMessageDead logs when an outbox message is moved to dead status
func (l *Logger) OutboxMessageDead(ctx context.Context, messageID, routingKey string, retries int) {
	l.log.Error().
		Str("action", "outbox_dead").
		Str("message_id", messageID).
		Str("routing_key", routingKey).
		Int("retries", retries).
		Msg("Outbox message moved to dead status")
}

// getTraceID extracts trace ID from context if available
func getTraceID(ctx context.Context) string {
	if v := ctx.Value("trace_id"); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	// Try to get from request ID as fallback
	if v := ctx.Value("request_id"); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
