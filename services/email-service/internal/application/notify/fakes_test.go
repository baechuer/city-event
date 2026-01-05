package notify

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ---- Fake Sender ----

type fakeSender struct {
	mu sync.Mutex

	verifyCalls int
	resetCalls  int

	lastVerifyTo   string
	lastVerifyLink string

	lastResetTo   string
	lastResetLink string

	// Optional: allow scripted failures
	verifyErr error
	resetErr  error
}

func (s *fakeSender) SendVerifyEmail(ctx context.Context, toEmail, url string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.verifyCalls++
	s.lastVerifyTo = toEmail
	s.lastVerifyLink = url
	return s.verifyErr
}

func (s *fakeSender) SendPasswordReset(ctx context.Context, toEmail, url string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.resetCalls++
	s.lastResetTo = toEmail
	s.lastResetLink = url
	return s.resetErr
}

func (s *fakeSender) SendEventCanceled(ctx context.Context, toEmail, eventID, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return nil
}

func (s *fakeSender) SendEventUnpublished(ctx context.Context, toEmail, eventID, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return nil
}

func (s *fakeSender) VerifyCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.verifyCalls
}

func (s *fakeSender) ResetCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.resetCalls
}

func (s *fakeSender) SetVerifyErr(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.verifyErr = err
}

func (s *fakeSender) SetResetErr(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resetErr = err
}

// ---- Fake Idempotency Store (B semantics) ----

type fakeIdem struct {
	mu sync.Mutex

	seen map[string]bool

	seenErr error
	markErr error

	seenCalls int
	markCalls int

	lastSeenKey string
	lastMarkKey string
	lastMarkTTL time.Duration
}

func newFakeIdem() *fakeIdem {
	return &fakeIdem{seen: map[string]bool{}}
}

func (s *fakeIdem) Seen(ctx context.Context, key string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.seenCalls++
	s.lastSeenKey = key

	if s.seenErr != nil {
		return false, s.seenErr
	}
	return s.seen[key], nil
}

func (s *fakeIdem) MarkSent(ctx context.Context, key string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.markCalls++
	s.lastMarkKey = key
	s.lastMarkTTL = ttl

	if s.markErr != nil {
		return s.markErr
	}
	s.seen[key] = true
	return nil
}

func (s *fakeIdem) SetSeen(key string, v bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seen[key] = v
}

func (s *fakeIdem) SetSeenErr(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seenErr = err
}

func (s *fakeIdem) SetMarkErr(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.markErr = err
}

func (s *fakeIdem) SeenCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.seenCalls
}

func (s *fakeIdem) MarkCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.markCalls
}

// helper error for tests
var errBoom = errors.New("boom")
