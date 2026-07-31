package browser

import (
	"context"
	"testing"
	"time"
)

func TestChallengeManager_CreateAndGet(t *testing.T) {
	m := NewChallengeManager()
	sessionID := "session-123"

	token := m.Create(sessionID)
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	gotSession, err := m.GetSession(token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotSession != sessionID {
		t.Errorf("got session %q, want %q", gotSession, sessionID)
	}
}

func TestChallengeManager_WaitAndResolve(t *testing.T) {
	m := NewChallengeManager()
	token := m.Create("session-123")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	errCh := make(chan error)
	go func() {
		errCh <- m.Wait(ctx, token)
	}()

	// Simulate some work before resolving
	time.Sleep(50 * time.Millisecond)
	m.Resolve(token)

	err := <-errCh
	if err != nil {
		t.Errorf("expected no error after resolution, got %v", err)
	}

	// Token should be removed after resolve
	_, err = m.GetSession(token)
	if err != ErrChallengeNotFound {
		t.Errorf("expected ErrChallengeNotFound, got %v", err)
	}
}

func TestChallengeManager_WaitTimeout(t *testing.T) {
	m := NewChallengeManager()
	token := m.Create("session-123")

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := m.Wait(ctx, token)
	if err != context.DeadlineExceeded {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}

	// Token should be removed after timeout
	_, err = m.GetSession(token)
	if err != ErrChallengeNotFound {
		t.Errorf("expected ErrChallengeNotFound, got %v", err)
	}
}

func TestChallengeManager_Remove(t *testing.T) {
	m := NewChallengeManager()
	token := m.Create("session-123")

	m.Remove(token)

	_, err := m.GetSession(token)
	if err != ErrChallengeNotFound {
		t.Errorf("expected ErrChallengeNotFound, got %v", err)
	}
}
