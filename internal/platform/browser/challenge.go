package browser

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	ErrChallengeExpired   = errors.New("challenge expired")
	ErrChallengeCancelled = errors.New("challenge cancelled")
	ErrChallengeNotFound  = errors.New("challenge not found")
	ErrChallengeInUse     = errors.New("challenge already has an active controller")
	ErrChallengeState     = errors.New("invalid challenge state")
)

type ChallengeStatus string

const (
	ChallengePending   ChallengeStatus = "pending"
	ChallengeVerifying ChallengeStatus = "verifying"
	ChallengeCompleted ChallengeStatus = "completed"
	ChallengeCancelled ChallengeStatus = "cancelled"
	ChallengeExpired   ChallengeStatus = "expired"
)

const defaultChallengeTTL = 3 * time.Minute

// ChallengeView is the public, secret-free state returned to API clients.
type ChallengeView struct {
	Status    ChallengeStatus `json:"status"`
	Connected bool            `json:"connected"`
	CreatedAt time.Time       `json:"created_at"`
	ExpiresAt time.Time       `json:"expires_at"`
}

type challenge struct {
	sessionID string
	status    ChallengeStatus
	connected bool
	createdAt time.Time
	expiresAt time.Time
	done      chan struct{}
	doneOnce  sync.Once
}

func (c *challenge) finish(status ChallengeStatus) {
	c.status = status
	c.connected = false
	c.doneOnce.Do(func() { close(c.done) })
}

func (c *challenge) view() ChallengeView {
	return ChallengeView{
		Status:    c.status,
		Connected: c.connected,
		CreatedAt: c.createdAt.UTC(),
		ExpiresAt: c.expiresAt.UTC(),
	}
}

// ChallengeManager tracks short-lived browser handoffs. Browser session IDs
// never leave this manager; clients receive only opaque capability tokens.
type ChallengeManager struct {
	mu         sync.Mutex
	challenges map[string]*challenge
	ttl        time.Duration
	now        func() time.Time
}

func NewChallengeManager() *ChallengeManager {
	return NewChallengeManagerWithTTL(defaultChallengeTTL)
}

func NewChallengeManagerWithTTL(ttl time.Duration) *ChallengeManager {
	if ttl <= 0 {
		ttl = defaultChallengeTTL
	}
	return &ChallengeManager{
		challenges: make(map[string]*challenge),
		ttl:        ttl,
		now:        time.Now,
	}
}

// Create generates a single-purpose, short-lived capability token.
func (m *ChallengeManager) Create(sessionID string) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.now()
	m.removeStaleLocked(now)
	token := uuid.NewString()
	m.challenges[token] = &challenge{
		sessionID: sessionID,
		status:    ChallengePending,
		createdAt: now,
		expiresAt: now.Add(m.ttl),
		done:      make(chan struct{}),
	}
	return token
}

func (m *ChallengeManager) Get(token string) (ChallengeView, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, err := m.lookupLocked(token)
	if err != nil {
		if c != nil {
			return c.view(), err
		}
		return ChallengeView{}, err
	}
	return c.view(), nil
}

// GetSession resolves a capability token internally without exposing the
// browser session identifier to API clients.
func (m *ChallengeManager) GetSession(token string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, err := m.lookupLocked(token)
	if err != nil {
		return "", err
	}
	if c.status != ChallengePending && c.status != ChallengeVerifying {
		return "", stateError(c.status)
	}
	return c.sessionID, nil
}

// AcquireController permits one interactive WebSocket controller per token.
func (m *ChallengeManager) AcquireController(token string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, err := m.lookupLocked(token)
	if err != nil {
		return "", err
	}
	if c.status != ChallengePending {
		return "", stateError(c.status)
	}
	if c.connected {
		return "", ErrChallengeInUse
	}
	c.connected = true
	return c.sessionID, nil
}

func (m *ChallengeManager) ReleaseController(token string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.challenges[token]; ok {
		c.connected = false
	}
}

func (m *ChallengeManager) BeginVerification(token string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, err := m.lookupLocked(token)
	if err != nil {
		return "", err
	}
	if c.status != ChallengePending {
		return "", stateError(c.status)
	}
	c.status = ChallengeVerifying
	return c.sessionID, nil
}

func (m *ChallengeManager) RejectVerification(token string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, err := m.lookupLocked(token)
	if err != nil {
		return err
	}
	if c.status != ChallengeVerifying {
		return stateError(c.status)
	}
	c.status = ChallengePending
	return nil
}

// Resolve marks a backend-verified challenge as complete and wakes waiters.
func (m *ChallengeManager) Resolve(token string) (ChallengeView, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, err := m.lookupLocked(token)
	if err != nil {
		return ChallengeView{}, err
	}
	if c.status != ChallengeVerifying {
		return ChallengeView{}, stateError(c.status)
	}
	c.finish(ChallengeCompleted)
	return c.view(), nil
}

func (m *ChallengeManager) Cancel(token string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, err := m.lookupLocked(token)
	if err != nil {
		return err
	}
	if c.status == ChallengeCompleted {
		return stateError(c.status)
	}
	c.finish(ChallengeCancelled)
	return nil
}

// Wait blocks until verification succeeds, the user cancels, or the token
// expires. Expiration is independent of the caller's deadline.
func (m *ChallengeManager) Wait(ctx context.Context, token string) error {
	m.mu.Lock()
	c, err := m.lookupLocked(token)
	if err != nil {
		m.mu.Unlock()
		return err
	}
	done := c.done
	expiresAt := c.expiresAt
	m.mu.Unlock()

	timer := time.NewTimer(time.Until(expiresAt))
	defer timer.Stop()

	select {
	case <-ctx.Done():
		_ = m.Cancel(token)
		return ctx.Err()
	case <-timer.C:
		m.expire(token)
		return ErrChallengeExpired
	case <-done:
		view, getErr := m.Get(token)
		if getErr != nil && !errors.Is(getErr, ErrChallengeExpired) {
			return getErr
		}
		switch view.Status {
		case ChallengeCompleted:
			return nil
		case ChallengeCancelled:
			return ErrChallengeCancelled
		default:
			return ErrChallengeExpired
		}
	}
}

// Remove forgets a terminal challenge. Removing an active challenge first
// cancels it so every waiter is released.
func (m *ChallengeManager) Remove(token string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.challenges[token]; ok {
		if c.status == ChallengePending || c.status == ChallengeVerifying {
			c.finish(ChallengeCancelled)
		}
		delete(m.challenges, token)
	}
}

func (m *ChallengeManager) lookupLocked(token string) (*challenge, error) {
	c, ok := m.challenges[token]
	if !ok {
		return nil, ErrChallengeNotFound
	}
	if (c.status == ChallengePending || c.status == ChallengeVerifying) && !m.now().Before(c.expiresAt) {
		c.finish(ChallengeExpired)
	}
	if c.status == ChallengeExpired {
		return c, ErrChallengeExpired
	}
	return c, nil
}

func (m *ChallengeManager) expire(token string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.challenges[token]; ok && (c.status == ChallengePending || c.status == ChallengeVerifying) {
		c.finish(ChallengeExpired)
	}
}

func (m *ChallengeManager) removeStaleLocked(now time.Time) {
	for token, c := range m.challenges {
		if c.status != ChallengePending && c.status != ChallengeVerifying && now.Sub(c.expiresAt) > m.ttl {
			delete(m.challenges, token)
		}
	}
}

func stateError(status ChallengeStatus) error {
	switch status {
	case ChallengeExpired:
		return ErrChallengeExpired
	case ChallengeCancelled:
		return ErrChallengeCancelled
	default:
		return ErrChallengeState
	}
}
