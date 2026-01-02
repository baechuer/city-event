package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/baechuer/real-time-ressys/services/join-service/internal/domain"
	"github.com/google/uuid"
)

type JoinService struct {
	repo  domain.JoinRepository
	cache domain.CacheRepository
}

func NewJoinService(repo domain.JoinRepository, cache domain.CacheRepository) *JoinService {
	return &JoinService{repo: repo, cache: cache}
}

func isPrivileged(role string) bool {
	r := strings.ToLower(strings.TrimSpace(role))
	return r == "admin" || r == "moderator"
}

func (s *JoinService) requireOrganizerOrAdmin(ctx context.Context, eventID uuid.UUID, requesterID uuid.UUID, role string) error {
	if isPrivileged(role) {
		return nil
	}
	owner, err := s.repo.GetEventOwnerID(ctx, eventID)
	if err != nil {
		return err
	}
	if owner != requesterID {
		return domain.ErrForbidden
	}
	return nil
}

func (s *JoinService) Join(ctx context.Context, traceID, idempotencyKey string, eventID, userID uuid.UUID) (string, error) {
	// cache fast-fail stays
	if s.cache != nil {
		capacity, err := s.cache.GetEventCapacity(ctx, eventID)
		if err == nil {
			if capacity < 0 {
				return "", domain.ErrEventClosed
			}
		} else if !errors.Is(err, domain.ErrCacheMiss) {
			// ignore redis errors
		}
	}
	status, err := s.repo.JoinEvent(ctx, traceID, idempotencyKey, eventID, userID)
	if err != nil {
		return "", err
	}
	return string(status), nil
}

func (s *JoinService) GetMyParticipation(ctx context.Context, userID, eventID uuid.UUID) (domain.JoinRecord, error) {
	// Simple pass-through to repo
	return s.repo.GetByEventAndUser(ctx, eventID, userID)
}

func (s *JoinService) Cancel(ctx context.Context, traceID, idempotencyKey string, eventID, userID uuid.UUID) error {
	return s.repo.CancelJoin(ctx, traceID, idempotencyKey, eventID, userID)
}

// Reads
func (s *JoinService) ListMyJoins(ctx context.Context, userID uuid.UUID, statuses []domain.JoinStatus, from, to *time.Time, limit int, cursor *domain.KeysetCursor) ([]domain.JoinRecord, *domain.KeysetCursor, error) {
	return s.repo.ListMyJoins(ctx, userID, statuses, from, to, limit, cursor)
}

func (s *JoinService) ListParticipants(ctx context.Context, eventID uuid.UUID, requesterID uuid.UUID, role string, limit int, cursor *domain.KeysetCursor) ([]domain.JoinRecord, *domain.KeysetCursor, error) {
	if err := s.requireOrganizerOrAdmin(ctx, eventID, requesterID, role); err != nil {
		return nil, nil, err
	}
	return s.repo.ListParticipants(ctx, eventID, limit, cursor)
}

func (s *JoinService) ListWaitlist(ctx context.Context, eventID uuid.UUID, requesterID uuid.UUID, role string, limit int, cursor *domain.KeysetCursor) ([]domain.JoinRecord, *domain.KeysetCursor, error) {
	if err := s.requireOrganizerOrAdmin(ctx, eventID, requesterID, role); err != nil {
		return nil, nil, err
	}
	return s.repo.ListWaitlist(ctx, eventID, limit, cursor)
}

func (s *JoinService) GetStats(ctx context.Context, eventID uuid.UUID, requesterID uuid.UUID, role string) (domain.EventStats, error) {
	if err := s.requireOrganizerOrAdmin(ctx, eventID, requesterID, role); err != nil {
		return domain.EventStats{}, err
	}
	return s.repo.GetStats(ctx, eventID)
}

// Moderation
func (s *JoinService) Kick(ctx context.Context, traceID string, eventID, targetUserID, actorID uuid.UUID, role string, reason string) error {
	if err := s.requireOrganizerOrAdmin(ctx, eventID, actorID, role); err != nil {
		return err
	}
	return s.repo.Kick(ctx, traceID, eventID, targetUserID, actorID, reason)
}

func (s *JoinService) Ban(ctx context.Context, traceID string, eventID, targetUserID, actorID uuid.UUID, role string, reason string, expiresAt *time.Time) error {
	if err := s.requireOrganizerOrAdmin(ctx, eventID, actorID, role); err != nil {
		return err
	}
	return s.repo.Ban(ctx, traceID, eventID, targetUserID, actorID, reason, expiresAt)
}

func (s *JoinService) Unban(ctx context.Context, traceID string, eventID, targetUserID, actorID uuid.UUID, role string) error {
	if err := s.requireOrganizerOrAdmin(ctx, eventID, actorID, role); err != nil {
		return err
	}
	return s.repo.Unban(ctx, traceID, eventID, targetUserID, actorID)
}
