package feed

import (
	"context"
	"strings"
)

type Service struct {
	repo  Repository
	cache Cache
}

func NewService(repo Repository, cache Cache) *Service {
	return &Service{repo: repo, cache: cache}
}

func (s *Service) ListEvents(ctx context.Context, query Query) ([]Event, error) {
	query = NormalizeQuery(query)
	key := listCacheKey(query)
	var cached []Event
	if s.cache != nil {
		if ok, err := s.cache.Get(ctx, key, &cached); err == nil && ok {
			return cached, nil
		}
	}
	events, err := s.repo.ListEvents(ctx, query)
	if err != nil {
		return nil, err
	}
	if s.cache != nil {
		_ = s.cache.Set(ctx, key, events, cacheTTL)
	}
	return events, nil
}

func (s *Service) GetEvent(ctx context.Context, eventID string) (Event, error) {
	eventID = strings.TrimSpace(eventID)
	key := detailCacheKey(eventID)
	var cached Event
	if s.cache != nil {
		if ok, err := s.cache.Get(ctx, key, &cached); err == nil && ok {
			return cached, nil
		}
	}
	event, err := s.repo.GetEvent(ctx, eventID)
	if err != nil {
		return Event{}, err
	}
	if s.cache != nil {
		_ = s.cache.Set(ctx, key, event, cacheTTL)
	}
	return event, nil
}
