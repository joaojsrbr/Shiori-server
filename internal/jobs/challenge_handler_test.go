package jobs

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/joaojsr/shiori-server/internal/platform/browser"
)

type challengeBrowser struct {
	userAction bool
}

func (b *challengeBrowser) IsAvailable() bool { return true }
func (b *challengeBrowser) Navigate(context.Context, browser.NavigateRequest) (*browser.NavigateResult, error) {
	return &browser.NavigateResult{SessionID: "session-1"}, nil
}
func (b *challengeBrowser) Snapshot(context.Context, string) (*browser.PageSnapshot, error) {
	return &browser.PageSnapshot{HTML: "<html></html>", UserAction: b.userAction}, nil
}
func (b *challengeBrowser) CloseSession(context.Context, string) error { return nil }
func (b *challengeBrowser) Screencast(context.Context, string, chan<- []byte, <-chan browser.InputEvent) error {
	return nil
}

func challengeRouter(bp browser.Provider, manager *browser.ChallengeManager) http.Handler {
	router := chi.NewRouter()
	NewChallengeHandler(bp, manager).RegisterRoutes(router)
	return router
}

func TestChallengeHandlerServesProtectedHandoff(t *testing.T) {
	manager := browser.NewChallengeManager()
	token := manager.Create("session-1")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/challenges/"+token, nil)

	challengeRouter(&challengeBrowser{}, manager).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if !strings.Contains(recorder.Header().Get("Content-Security-Policy"), "default-src 'none'") {
		t.Fatalf("missing restrictive CSP: %q", recorder.Header().Get("Content-Security-Policy"))
	}
	if recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", recorder.Header().Get("Cache-Control"))
	}
	if !strings.Contains(recorder.Body.String(), "width: 100%; height: 100%") {
		t.Fatal("handoff canvas is not configured to fill the available viewport")
	}

	clientRecorder := httptest.NewRecorder()
	clientRequest := httptest.NewRequest(http.MethodGet, "/challenges/assets/client.js", nil)
	challengeRouter(&challengeBrowser{}, manager).ServeHTTP(clientRecorder, clientRequest)
	if !strings.Contains(clientRecorder.Body.String(), "new ResizeObserver(measureViewport)") ||
		!strings.Contains(clientRecorder.Body.String(), "type: 'viewport'") {
		t.Fatal("handoff client does not synchronize its viewport with the remote browser")
	}
}

func TestChallengeHandlerVerifiesBeforeCompleting(t *testing.T) {
	provider := &challengeBrowser{userAction: true}
	manager := browser.NewChallengeManager()
	token := manager.Create("session-1")
	router := challengeRouter(provider, manager)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/challenges/"+token+"/complete", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("challenge-visible status = %d, want 409", recorder.Code)
	}
	view, err := manager.Get(token)
	if err != nil || view.Status != browser.ChallengePending {
		t.Fatalf("challenge state = %#v, %v; want pending", view, err)
	}

	provider.userAction = false
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/challenges/"+token+"/complete", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("verified status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	if err := manager.Wait(context.Background(), token); err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
}

func TestChallengeHandlerCancelWakesWorkflow(t *testing.T) {
	manager := browser.NewChallengeManager()
	token := manager.Create("session-1")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, "/challenges/"+token, nil)

	challengeRouter(&challengeBrowser{}, manager).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", recorder.Code)
	}
	if err := manager.Wait(context.Background(), token); !errors.Is(err, browser.ErrChallengeCancelled) {
		t.Fatalf("Wait() error = %v, want ErrChallengeCancelled", err)
	}
}

func TestChallengeWebSocketRejectsCrossOrigin(t *testing.T) {
	manager := browser.NewChallengeManager()
	token := manager.Create("session-1")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/challenges/"+token+"/ws", nil)
	request.Host = "127.0.0.1:9180"
	request.Header.Set("Origin", "https://attacker.example")
	request.Header.Set("Connection", "Upgrade")
	request.Header.Set("Upgrade", "websocket")
	request.Header.Set("Sec-WebSocket-Version", "13")
	request.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

	challengeRouter(&challengeBrowser{}, manager).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", recorder.Code)
	}
}
