package domain

import (
	"time"

	"github.com/google/uuid"
)

// CalculateActionPolicy determines what actions a user can take on an event.
// It follows the rules defined in PHASE_4_PLAN.md.
func CalculateActionPolicy(event *Event, part *Participation, userID uuid.UUID, userRole string, now time.Time, isDegraded bool) ActionPolicy {
	// 1. Auth Gate
	if userID == uuid.Nil {
		return ActionPolicy{
			CanJoin:   false,
			CanCancel: false,
			Reason:    "auth_required",
		}
	}

	// 2. Degraded Gate
	if isDegraded {
		return ActionPolicy{
			CanJoin:   false,
			CanCancel: false,
			Reason:    "participation_unavailable",
		}
	}

	// 3. Status Checks
	status := StatusNone
	if part != nil {
		status = part.Status
	}

	// 4. Owner / Admin Logic
	isOwner := event.OwnerID == userID
	isAdminOrMod := userRole == "admin" || userRole == "moderator"

	canEdit := isOwner
	canCancelEvent := (isOwner || isAdminOrMod) && event.StartTime.After(now)
	canUnpublish := (isOwner || isAdminOrMod) && event.Status == EventStatusPublished && event.StartTime.After(now)

	// Can Cancel Participation?
	canCancel := !isOwner && (status == StatusActive || status == StatusWaitlisted) && event.StartTime.After(now)

	// Can Join?
	canJoin := false
	reason := ""

	if isOwner {
		canJoin = false
		reason = "is_organizer"
	} else {
		switch status {
		case StatusActive, StatusWaitlisted:
			canJoin = false
			reason = "already_joined"
		case StatusRejected:
			canJoin = false
			reason = "banned_or_rejected"
		default:
			// Not joined, canceled, or expired -> Can attempt join
			if event.EndTime.Before(now) {
				canJoin = false
				reason = "event_ended"
			} else if event.Capacity < 0 {
				canJoin = false
				reason = "event_closed"
			} else {
				canJoin = true
			}
		}
	}

	return ActionPolicy{
		CanJoin:        canJoin,
		CanCancel:      canCancel,
		CanCancelEvent: canCancelEvent,
		CanUnpublish:   canUnpublish,
		CanEdit:        canEdit,
		Reason:         reason,
	}
}
