package browser

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestChallengeManagerLifecycle(t *testing.T) {
	m := NewChallengeManagerWithTTL(time.Minute)
	token := m.Create("session-123")

	view, err := m.Get(token)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if view.Status != ChallengePending || view.Connected {
		t.Fatalf("Get() = %#v, want pending and disconnected", view)
	}

	sessionID, err := m.AcquireController(token)
	if err != nil || sessionID != "session-123" {
		t.Fatalf("AcquireController() = %q, %v", sessionID, err)
	}
	if _, err := m.AcquireController(token); !errors.Is(err, ErrChallengeInUse) {
		t.Fatalf("second AcquireController() error = %v, want ErrChallengeInUse", err)
	}
	m.ReleaseController(token)

	sessionID, err = m.BeginVerification(token)
	if err != nil || sessionID != "session-123" {
		t.Fatalf("BeginVerification() = %q, %v", sessionID, err)
	}
	if _, err := m.Resolve(token); err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if err := m.Wait(context.Background(), token); err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
	view, err = m.Get(token)
	if err != nil || view.Status != ChallengeCompleted {
		t.Fatalf("completed Get() = %#v, %v", view, err)
	}
}

func TestChallengeManagerRejectsFailedVerification(t *testing.T) {
	m := NewChallengeManager()
	token := m.Create("session-123")
	if _, err := m.BeginVerification(token); err != nil {
		t.Fatal(err)
	}
	if err := m.RejectVerification(token); err != nil {
		t.Fatal(err)
	}
	view, err := m.Get(token)
	if err != nil || view.Status != ChallengePending {
		t.Fatalf("Get() = %#v, %v; want pending", view, err)
	}
}

func TestChallengeManagerExpiration(t *testing.T) {
	m := NewChallengeManagerWithTTL(25 * time.Millisecond)
	token := m.Create("session-123")

	err := m.Wait(context.Background(), token)
	if !errors.Is(err, ErrChallengeExpired) {
		t.Fatalf("Wait() error = %v, want ErrChallengeExpired", err)
	}
	view, err := m.Get(token)
	if !errors.Is(err, ErrChallengeExpired) || view.Status != ChallengeExpired {
		t.Fatalf("expired Get() = %#v, %v", view, err)
	}
}

func TestChallengeManagerCancelWakesWaiter(t *testing.T) {
	m := NewChallengeManager()
	token := m.Create("session-123")
	errCh := make(chan error, 1)
	go func() { errCh <- m.Wait(context.Background(), token) }()

	if err := m.Cancel(token); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	select {
	case err := <-errCh:
		if !errors.Is(err, ErrChallengeCancelled) {
			t.Fatalf("Wait() error = %v, want ErrChallengeCancelled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Wait() was not released by Cancel()")
	}
}

func TestChallengeManagerRemoveForgetsToken(t *testing.T) {
	m := NewChallengeManager()
	token := m.Create("session-123")
	m.Remove(token)
	if _, err := m.Get(token); !errors.Is(err, ErrChallengeNotFound) {
		t.Fatalf("Get() error = %v, want ErrChallengeNotFound", err)
	}
}

func TestInputEventValid(t *testing.T) {
	tests := []struct {
		name  string
		event InputEvent
		want  bool
	}{
		{name: "mouse", event: InputEvent{Type: "mousePressed", X: 10, Y: 20, Button: "left"}, want: true},
		{name: "wheel", event: InputEvent{Type: "mouseWheel", X: 10, Y: 20, DeltaY: 120}, want: true},
		{name: "keyboard", event: InputEvent{Type: "keyDown", Key: "a", Code: "KeyA", Text: "a"}, want: true},
		{name: "viewport", event: InputEvent{Type: "viewport", Width: 1280, Height: 720}, want: true},
		{name: "viewport too small", event: InputEvent{Type: "viewport", Width: 200, Height: 100}, want: false},
		{name: "viewport too large", event: InputEvent{Type: "viewport", Width: 5000, Height: 3000}, want: false},
		{name: "unknown", event: InputEvent{Type: "executeScript"}, want: false},
		{name: "negative coordinate", event: InputEvent{Type: "mouseMoved", X: -1, Y: 20, Button: "none"}, want: false},
		{name: "oversized wheel", event: InputEvent{Type: "mouseWheel", X: 1, Y: 1, DeltaY: 5000}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.event.Valid(); got != tt.want {
				t.Fatalf("Valid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestChallengeViewIncludesActionKind(t *testing.T) {
	manager := NewChallengeManager()
	token := manager.Create("session-login", UserActionLogin)
	view, err := manager.Get(token)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if view.Kind != UserActionLogin {
		t.Fatalf("Kind = %q, want %q", view.Kind, UserActionLogin)
	}
}
