package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestCalculateActionPolicy(t *testing.T) {
	now := time.Now()
	eventID := uuid.New()
	userID := uuid.New()

	futureEvent := &Event{
		ID:        eventID,
		StartTime: now.Add(1 * time.Hour),
		EndTime:   now.Add(2 * time.Hour),
		Capacity:  10,
	}

	pastEvent := &Event{
		ID:        eventID,
		StartTime: now.Add(-2 * time.Hour),
		EndTime:   now.Add(-1 * time.Hour),
		Capacity:  10,
	}

	t.Run("Auth Required", func(t *testing.T) {
		policy := CalculateActionPolicy(futureEvent, nil, uuid.Nil, now, false)
		assert.False(t, policy.CanJoin)
		assert.Equal(t, "auth_required", policy.Reason)
	})

	t.Run("Degraded Mode", func(t *testing.T) {
		policy := CalculateActionPolicy(futureEvent, nil, userID, now, true)
		assert.False(t, policy.CanJoin)
		assert.Equal(t, "participation_unavailable", policy.Reason)
	})

	t.Run("Can Join Future Event", func(t *testing.T) {
		policy := CalculateActionPolicy(futureEvent, nil, userID, now, false)
		assert.True(t, policy.CanJoin)
		assert.False(t, policy.CanCancel)
	})

	t.Run("Already Active", func(t *testing.T) {
		part := &Participation{Status: StatusActive}
		policy := CalculateActionPolicy(futureEvent, part, userID, now, false)
		assert.False(t, policy.CanJoin)
		assert.True(t, policy.CanCancel)
		assert.Equal(t, "already_joined", policy.Reason)
	})

	t.Run("Event Ended", func(t *testing.T) {
		policy := CalculateActionPolicy(pastEvent, nil, userID, now, false)
		assert.False(t, policy.CanJoin)
		assert.Equal(t, "event_ended", policy.Reason)
	})

	t.Run("Event Closed", func(t *testing.T) {
		closedEvent := &Event{Capacity: -1, EndTime: now.Add(1 * time.Hour)}
		policy := CalculateActionPolicy(closedEvent, nil, userID, now, false)
		assert.False(t, policy.CanJoin)
		assert.Equal(t, "event_closed", policy.Reason)
	})
}
