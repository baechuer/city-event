package eventregistration

import (
	"context"
	"strings"
	"time"
)

type Service struct {
	repo Repository
	now  func() time.Time
}

type CreateEventCommand struct {
	OrganizerID string
	Title       string
	Description string
	City        string
	Venue       string
	StartsAt    time.Time
	Capacity    int
}

type UpdateEventCommand struct {
	Title       *string
	Description *string
	City        *string
	Venue       *string
	StartsAt    *time.Time
	Capacity    *int
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

func (s *Service) CreateEvent(ctx context.Context, cmd CreateEventCommand) (EventDetail, error) {
	event, err := NewEvent(cmd.OrganizerID, cmd.Title, cmd.Description, cmd.City, cmd.Venue, cmd.StartsAt, cmd.Capacity, s.now())
	if err != nil {
		return EventDetail{}, err
	}
	return s.repo.CreateEvent(ctx, event)
}

func (s *Service) UpdateEvent(ctx context.Context, eventID, organizerID string, cmd UpdateEventCommand) (EventDetail, error) {
	if strings.TrimSpace(organizerID) == "" {
		return EventDetail{}, ErrUnauthorized
	}
	return s.repo.UpdateEvent(ctx, strings.TrimSpace(eventID), strings.TrimSpace(organizerID), cmd, s.now())
}

func (s *Service) CancelEvent(ctx context.Context, eventID, organizerID string) (EventDetail, error) {
	if strings.TrimSpace(organizerID) == "" {
		return EventDetail{}, ErrUnauthorized
	}
	return s.repo.CancelEvent(ctx, strings.TrimSpace(eventID), strings.TrimSpace(organizerID), s.now())
}

func (s *Service) ListPublishedEvents(ctx context.Context) ([]EventDetail, error) {
	return s.repo.ListPublishedEvents(ctx)
}

func (s *Service) GetEventDetail(ctx context.Context, eventID, viewerID string) (EventDetail, error) {
	return s.repo.GetEventDetail(ctx, strings.TrimSpace(eventID), strings.TrimSpace(viewerID))
}

func (s *Service) JoinEvent(ctx context.Context, eventID, userID, idempotencyKey string) (JoinResult, error) {
	if strings.TrimSpace(userID) == "" {
		return JoinResult{}, ErrUnauthorized
	}
	return s.repo.JoinEvent(ctx, strings.TrimSpace(eventID), strings.TrimSpace(userID), strings.TrimSpace(idempotencyKey), s.now())
}

func (s *Service) CancelJoin(ctx context.Context, eventID, userID string) (CancelJoinResult, error) {
	if strings.TrimSpace(userID) == "" {
		return CancelJoinResult{}, ErrUnauthorized
	}
	return s.repo.CancelJoin(ctx, strings.TrimSpace(eventID), strings.TrimSpace(userID), s.now())
}

func (s *Service) GetJoinStatus(ctx context.Context, eventID, userID string) (RegistrationStatus, error) {
	if strings.TrimSpace(userID) == "" {
		return "", ErrUnauthorized
	}
	return s.repo.GetJoinStatus(ctx, strings.TrimSpace(eventID), strings.TrimSpace(userID))
}
