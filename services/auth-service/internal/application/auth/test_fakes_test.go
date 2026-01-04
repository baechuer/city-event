package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/baechuer/real-time-ressys/services/auth-service/internal/domain"
)

/*
Shared audit capture
*/

type auditEntry struct {
	action string
	fields map[string]string
}

/*
Fakes for ports
*/

type fakeUserRepo struct {
	mu sync.Mutex

	byID    map[string]domain.User
	byEmail map[string]domain.User

	// injected errors (if set, method returns error)
	getByIDErr     error
	getByEmailErr  error
	createErr      error
	lockErr        error
	unlockErr      error
	setRoleErr     error
	updatePwdErr   error
	setVerifiedErr error
	countByRoleErr error

	// record calls
	lockedIDs   []string
	unlockedIDs []string
	setRoles    []struct{ id, role string }
	updatedPwd  []struct{ id, hash string }
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{
		byID:    map[string]domain.User{},
		byEmail: map[string]domain.User{},
	}
}

func (f *fakeUserRepo) GetByEmail(ctx context.Context, email string) (domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.getByEmailErr != nil {
		return domain.User{}, f.getByEmailErr
	}
	u, ok := f.byEmail[email]
	if !ok {
		return domain.User{}, domain.ErrUserNotFound()
	}
	return u, nil
}

func (f *fakeUserRepo) GetByID(ctx context.Context, id string) (domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.getByIDErr != nil {
		return domain.User{}, f.getByIDErr
	}
	u, ok := f.byID[id]
	if !ok {
		return domain.User{}, domain.ErrUserNotFound()
	}
	return u, nil
}

func (f *fakeUserRepo) Create(ctx context.Context, u domain.User) (domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.createErr != nil {
		return domain.User{}, f.createErr
	}
	f.byID[u.ID] = u
	f.byEmail[u.Email] = u
	return u, nil
}

func (f *fakeUserRepo) UpdatePasswordHash(ctx context.Context, userID string, newHash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.updatePwdErr != nil {
		return f.updatePwdErr
	}
	u, ok := f.byID[userID]
	if !ok {
		return errors.New("not found")
	}
	u.PasswordHash = newHash
	f.byID[userID] = u
	f.byEmail[u.Email] = u
	f.updatedPwd = append(f.updatedPwd, struct{ id, hash string }{userID, newHash})
	return nil
}

func (f *fakeUserRepo) SetEmailVerified(ctx context.Context, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.setVerifiedErr != nil {
		return f.setVerifiedErr
	}
	u, ok := f.byID[userID]
	if !ok {
		return errors.New("not found")
	}
	u.EmailVerified = true
	f.byID[userID] = u
	f.byEmail[u.Email] = u
	return nil
}

func (f *fakeUserRepo) LockUser(ctx context.Context, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.lockErr != nil {
		return f.lockErr
	}
	u, ok := f.byID[userID]
	if !ok {
		return errors.New("not found")
	}
	u.Locked = true
	f.byID[userID] = u
	f.byEmail[u.Email] = u
	f.lockedIDs = append(f.lockedIDs, userID)
	return nil
}

func (f *fakeUserRepo) UnlockUser(ctx context.Context, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.unlockErr != nil {
		return f.unlockErr
	}
	u, ok := f.byID[userID]
	if !ok {
		return errors.New("not found")
	}
	u.Locked = false
	f.byID[userID] = u
	f.byEmail[u.Email] = u
	f.unlockedIDs = append(f.unlockedIDs, userID)
	return nil
}

func (f *fakeUserRepo) SetRole(ctx context.Context, userID string, role string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.setRoleErr != nil {
		return f.setRoleErr
	}
	u, ok := f.byID[userID]
	if !ok {
		return errors.New("not found")
	}
	u.Role = role
	f.byID[userID] = u
	f.byEmail[u.Email] = u
	f.setRoles = append(f.setRoles, struct{ id, role string }{userID, role})
	return nil
}

func (f *fakeUserRepo) CountByRole(ctx context.Context, role string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.countByRoleErr != nil {
		return 0, f.countByRoleErr
	}
	cnt := 0
	for _, u := range f.byID {
		if u.Role == role {
			cnt++
		}
	}
	return cnt, nil
}

func (f *fakeUserRepo) GetTokenVersion(ctx context.Context, userID string) (int64, error) {
	return 1, nil
}
func (f *fakeUserRepo) BumpTokenVersion(ctx context.Context, userID string) (int64, error) {
	return 2, nil
}

func (f *fakeUserRepo) UpdateAvatarImageID(ctx context.Context, userID string, avatarImageID *string) (*string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return nil, nil
}

type fakeHasher struct {
	hashFn    func(pw string) (string, error)
	compareFn func(hash, pw string) error
}

func (h *fakeHasher) Hash(password string) (string, error) {
	if h.hashFn != nil {
		return h.hashFn(password)
	}
	return "hash:" + password, nil
}

func (h *fakeHasher) Compare(hash string, password string) error {
	if h.compareFn != nil {
		return h.compareFn(hash, password)
	}
	if hash == "hash:"+password {
		return nil
	}
	return errors.New("mismatch")
}

type fakeSigner struct {
	signFn func(userID, role string, ttl time.Duration) (string, error)
}

func (s *fakeSigner) SignAccessToken(userID string, role string, ttl time.Duration) (string, error) {
	if s.signFn != nil {
		return s.signFn(userID, role, ttl)
	}
	return fmt.Sprintf("jwt(%s,%s)", userID, role), nil
}

func (s *fakeSigner) VerifyAccessToken(token string) (TokenClaims, error) {
	return TokenClaims{}, nil
}

type fakeSessions struct {
	mu sync.Mutex

	byToken map[string]string // refreshToken -> userID

	createErr    error
	rotateErr    error
	revokeErr    error
	revokeAllErr error
	getUserErr   error

	revoked    []string
	revokedAll []string
}

func newFakeSessions() *fakeSessions {
	return &fakeSessions{byToken: map[string]string{}}
}

func (s *fakeSessions) CreateRefreshToken(ctx context.Context, userID string, ttl time.Duration) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.createErr != nil {
		return "", s.createErr
	}
	tok := "rft:" + userID
	s.byToken[tok] = userID
	return tok, nil
}

func (s *fakeSessions) RotateRefreshToken(ctx context.Context, oldToken string, ttl time.Duration) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.rotateErr != nil {
		return "", s.rotateErr
	}
	uid, ok := s.byToken[oldToken]
	if !ok {
		return "", errors.New("invalid refresh")
	}
	delete(s.byToken, oldToken)
	newTok := "rft2:" + uid
	s.byToken[newTok] = uid
	return newTok, nil
}

func (s *fakeSessions) RevokeRefreshToken(ctx context.Context, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.revokeErr != nil {
		return s.revokeErr
	}
	delete(s.byToken, token)
	s.revoked = append(s.revoked, token)
	return nil
}

func (s *fakeSessions) RevokeAll(ctx context.Context, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.revokeAllErr != nil {
		return s.revokeAllErr
	}
	for tok, uid := range s.byToken {
		if uid == userID {
			delete(s.byToken, tok)
		}
	}
	s.revokedAll = append(s.revokedAll, userID)
	return nil
}

func (s *fakeSessions) GetUserIDByRefreshToken(ctx context.Context, token string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.getUserErr != nil {
		return "", s.getUserErr
	}
	uid, ok := s.byToken[token]
	if !ok {
		return "", errors.New("invalid refresh")
	}
	return uid, nil
}

type fakeOTT struct {
	mu sync.Mutex

	data map[OneTimeTokenKind]map[string]string // kind -> token -> userID

	saveErr    error
	peekErr    error
	consumeErr error
}

func newFakeOTT() *fakeOTT {
	return &fakeOTT{data: map[OneTimeTokenKind]map[string]string{}}
}

func (o *fakeOTT) Save(ctx context.Context, kind OneTimeTokenKind, token string, userID string, ttl time.Duration) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.saveErr != nil {
		return o.saveErr
	}
	if o.data[kind] == nil {
		o.data[kind] = map[string]string{}
	}
	o.data[kind][token] = userID
	return nil
}

func (o *fakeOTT) Consume(ctx context.Context, kind OneTimeTokenKind, token string) (string, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.consumeErr != nil {
		return "", o.consumeErr
	}
	m := o.data[kind]
	uid, ok := m[token]
	if !ok {
		switch kind {
		case TokenPasswordReset:
			return "", domain.ErrResetTokenNotFound()
		case TokenVerifyEmail:
			return "", domain.ErrVerifyTokenNotFound()
		default:
			return "", domain.ErrTokenInvalid()
		}
	}
	delete(m, token)
	return uid, nil
}

func (o *fakeOTT) Peek(ctx context.Context, kind OneTimeTokenKind, token string) (string, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.peekErr != nil {
		return "", o.peekErr
	}
	m := o.data[kind]
	uid, ok := m[token]
	if !ok {
		switch kind {
		case TokenPasswordReset:
			return "", domain.ErrResetTokenNotFound()
		case TokenVerifyEmail:
			return "", domain.ErrVerifyTokenNotFound()
		default:
			return "", domain.ErrTokenInvalid()
		}
	}
	return uid, nil
}

type fakePublisher struct {
	verifyErr error
	resetErr  error

	verifyEvts []VerifyEmailEvent
	resetEvts  []PasswordResetEvent
	avatarEvts []AvatarUpdatedEvent
}

func (p *fakePublisher) PublishVerifyEmail(ctx context.Context, evt VerifyEmailEvent) error {
	if p.verifyErr != nil {
		return p.verifyErr
	}
	p.verifyEvts = append(p.verifyEvts, evt)
	return nil
}

func (p *fakePublisher) PublishPasswordReset(ctx context.Context, evt PasswordResetEvent) error {
	if p.resetErr != nil {
		return p.resetErr
	}
	p.resetEvts = append(p.resetEvts, evt)
	return nil
}

func (p *fakePublisher) PublishAvatarUpdated(ctx context.Context, evt AvatarUpdatedEvent) error {
	p.avatarEvts = append(p.avatarEvts, evt)
	return nil
}

/*
Service factory for tests
*/

func newSvcForTest(t *testing.T) (*Service, *fakeUserRepo, *fakeHasher, *fakeSigner, *fakeSessions, *fakeOTT, *fakePublisher, *[]auditEntry) {
	t.Helper()

	users := newFakeUserRepo()
	hasher := &fakeHasher{}
	signer := &fakeSigner{}
	sessions := newFakeSessions()
	ott := newFakeOTT()
	pub := &fakePublisher{}

	audits := &[]auditEntry{}
	cfg := Config{
		AccessTTL:             15 * time.Minute,
		RefreshTTL:            7 * 24 * time.Hour,
		VerifyEmailBaseURL:    "https://fe/verify?token=",
		PasswordResetBaseURL:  "https://fe/reset?token=",
		VerifyEmailTokenTTL:   24 * time.Hour,
		PasswordResetTokenTTL: 30 * time.Minute,
	}

	svc := NewService(users, hasher, signer, sessions, ott, pub, cfg).
		WithAudit(func(action string, fields map[string]string) {
			cp := map[string]string{}
			for k, v := range fields {
				cp[k] = v
			}
			*audits = append(*audits, auditEntry{action: action, fields: cp})
		})

	// sanity check: no nil ports
	if svc == nil {
		t.Fatalf("svc is nil")
	}

	return svc, users, hasher, signer, sessions, ott, pub, audits
}

/*
Small assertions
*/

func requireDomainCode(t *testing.T, err error, wantCode string) {
	t.Helper()
	got := domainCode(err)
	if got != wantCode {
		t.Fatalf("expected domain code %q, got %q (err=%v)", wantCode, got, err)
	}
}

func lastAudit(audits *[]auditEntry) (auditEntry, bool) {
	if audits == nil || len(*audits) == 0 {
		return auditEntry{}, false
	}
	return (*audits)[len(*audits)-1], true
}

func requireAuditAction(t *testing.T, audits *[]auditEntry, wantAction string) auditEntry {
	t.Helper()
	e, ok := lastAudit(audits)
	if !ok {
		t.Fatalf("expected audit entry, got none")
	}
	if e.action != wantAction {
		t.Fatalf("expected audit action %q, got %q", wantAction, e.action)
	}
	return e
}

func requireAuditField(t *testing.T, e auditEntry, k, want string) {
	t.Helper()
	got := strings.TrimSpace(e.fields[k])
	if got != want {
		t.Fatalf("expected audit field %q=%q, got %q (all=%v)", k, want, got, e.fields)
	}
}
