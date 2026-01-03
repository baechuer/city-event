package event

import (
	"strings"
	"time"

	"github.com/baechuer/real-time-ressys/services/event-service/internal/domain"
)

type Clock interface{ Now() time.Time }

type Service struct {
	repo  EventRepo
	cache Cache
	clock Clock

	// Config for TTLs
	ttlDetails time.Duration
	ttlList    time.Duration
}

func New(
	repo EventRepo,
	clock Clock,
	cache Cache,
	ttlDetails, ttlList time.Duration,
) *Service {
	// Defaults if 0
	if ttlDetails == 0 {
		ttlDetails = 5 * time.Minute
	}
	if ttlList == 0 {
		ttlList = 15 * time.Second
	}

	return &Service{
		repo:       repo,
		cache:      cache,
		clock:      clock,
		ttlDetails: ttlDetails,
		ttlList:    ttlList,
	}
}

func isUser(role string) bool      { return role == "user" }
func isModerator(role string) bool { return role == "moderator" }
func isAdmin(role string) bool     { return role == "admin" }

func canCreate(actorRole string) bool {
	actorRole = strings.TrimSpace(actorRole)
	if actorRole == "" {
		return false
	}
	return actorRole == "user" || actorRole == "organizer" || isModerator(actorRole) || isAdmin(actorRole)
}

var _ = domain.AppError{}

func canManage(actorID, actorRole, ownerID string) bool {
	// Admin and Moderator can manage any event
	if actorRole == "admin" || actorRole == "moderator" {
		return true
	}
	return strings.TrimSpace(actorID) != "" && actorID == ownerID
}
func canEdit(ownerID, actorID, actorRole string) bool {
	// keep legacy name used across handlers
	return canManage(actorID, actorRole, ownerID)
}
