package browser

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	ErrChallengeExpired  = errors.New("challenge expired or cancelled")
	ErrChallengeNotFound = errors.New("challenge not found")
)

// Challenge represents a pending user action.
type Challenge struct {
	Token     string
	SessionID string
	CreatedAt time.Time
	ch        chan struct{}
}

// ChallengeManager tracks pending challenges and allows waiting for their resolution.
type ChallengeManager struct {
	mu         sync.Mutex
	challenges map[string]*Challenge
}

// NewChallengeManager creates a new ChallengeManager.
func NewChallengeManager() *ChallengeManager {
	return &ChallengeManager{
		challenges: make(map[string]*Challenge),
	}
}

// Create generates a new challenge token for a given session.
func (m *ChallengeManager) Create(sessionID string) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	token := uuid.New().String()
	m.challenges[token] = &Challenge{
		Token:     token,
		SessionID: sessionID,
		CreatedAt: time.Now(),
		ch:        make(chan struct{}),
	}
	return token
}

// GetSession returns the underlying browser session ID for a valid token.
func (m *ChallengeManager) GetSession(token string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	c, ok := m.challenges[token]
	if !ok {
		return "", ErrChallengeNotFound
	}
	return c.SessionID, nil
}

// Wait blocks until the challenge is resolved or the context is cancelled.
func (m *ChallengeManager) Wait(ctx context.Context, token string) error {
	m.mu.Lock()
	c, ok := m.challenges[token]
	m.mu.Unlock()

	if !ok {
		return ErrChallengeNotFound
	}

	select {
	case <-ctx.Done():
		m.Remove(token)
		return ctx.Err()
	case <-c.ch:
		// Challenge resolved
		return nil
	}
}

// Resolve marks a challenge as solved and unblocks waiters.
func (m *ChallengeManager) Resolve(token string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if c, ok := m.challenges[token]; ok {
		close(c.ch)
		delete(m.challenges, token)
	}
}

// Remove forcibly cancels and deletes a challenge.
func (m *ChallengeManager) Remove(token string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.challenges[token]; ok {
		delete(m.challenges, token)
	}
}
