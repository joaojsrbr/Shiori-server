package jobs_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/joaojsr/shiori-server/internal/extraction"
	"github.com/joaojsr/shiori-server/internal/jobs"
	"github.com/joaojsr/shiori-server/internal/library"
	"github.com/joaojsr/shiori-server/internal/platform/browser"
	"github.com/joaojsr/shiori-server/internal/platform/events"
	"github.com/joaojsr/shiori-server/internal/platform/queue"
)

type mockBrowserProvider struct {
	FailNavigate bool
	FailSnapshot bool
	UserAction   bool
	HTMLReturn   string
}

type handoffBrowser struct {
	navigations int
	closes      int
	snapshots   []*browser.PageSnapshot
}

func (b *handoffBrowser) IsAvailable() bool { return true }
func (b *handoffBrowser) Navigate(context.Context, browser.NavigateRequest) (*browser.NavigateResult, error) {
	b.navigations++
	return &browser.NavigateResult{SessionID: fmt.Sprintf("session-%d", b.navigations)}, nil
}
func (b *handoffBrowser) Snapshot(context.Context, string) (*browser.PageSnapshot, error) {
	if len(b.snapshots) == 0 {
		return nil, errors.New("no snapshot configured")
	}
	snapshot := b.snapshots[0]
	b.snapshots = b.snapshots[1:]
	return snapshot, nil
}
func (b *handoffBrowser) CloseSession(context.Context, string) error {
	b.closes++
	return nil
}
func (b *handoffBrowser) Screencast(context.Context, string, chan<- []byte, <-chan browser.InputEvent) error {
	return nil
}

func (m *mockBrowserProvider) IsAvailable() bool { return true }
func (m *mockBrowserProvider) Navigate(ctx context.Context, req browser.NavigateRequest) (*browser.NavigateResult, error) {
	if m.FailNavigate {
		return nil, errors.New("navigate error")
	}
	return &browser.NavigateResult{SessionID: "s1"}, nil
}
func (m *mockBrowserProvider) Snapshot(ctx context.Context, sessionID string) (*browser.PageSnapshot, error) {
	if m.FailSnapshot {
		return nil, errors.New("snapshot error")
	}
	return &browser.PageSnapshot{HTML: m.HTMLReturn, UserAction: m.UserAction}, nil
}
func (m *mockBrowserProvider) CloseSession(ctx context.Context, sessionID string) error {
	return nil
}

func (m *mockBrowserProvider) Screencast(ctx context.Context, sessionID string, frames chan<- []byte, input <-chan browser.InputEvent) error {
	return nil
}

type mockExtractionProvider struct {
	FailExtract bool
	JSONReturn  json.RawMessage
}

func (m *mockExtractionProvider) Extract(ctx context.Context, req extraction.Request) (*extraction.Result, error) {
	if m.FailExtract {
		return nil, errors.New("extract error")
	}
	return &extraction.Result{RawJSON: m.JSONReturn}, nil
}

type mockMediaRepoExtract struct {
	FailCreate bool
	SavedTitle string
}

func (m *mockMediaRepoExtract) Create(ctx context.Context, req library.MediaCreateRequest) (*library.Media, error) {
	if m.FailCreate {
		return nil, errors.New("create error")
	}
	m.SavedTitle = req.Title
	return &library.Media{}, nil
}
func (m *mockMediaRepoExtract) GetByID(ctx context.Context, id string) (*library.Media, error) {
	return nil, nil
}
func (m *mockMediaRepoExtract) List(ctx context.Context) ([]*library.Media, error) { return nil, nil }
func (m *mockMediaRepoExtract) Delete(ctx context.Context, id string) error        { return nil }

func TestExtractHandler_Success(t *testing.T) {
	b := &mockBrowserProvider{HTMLReturn: "<html><body>Content</body></html>"}
	e := &mockExtractionProvider{JSONReturn: json.RawMessage(`{"title": "Test Title"}`)}
	repo := &mockMediaRepoExtract{}
	cm := browser.NewChallengeManager()

	handler := jobs.NewExtractHandler(b, e, repo, cm, nil)

	payload := jobs.ExtractPayload{URL: "http://test.com", Target: extraction.TargetManga}
	rawPayload, _ := json.Marshal(payload)

	job := &queue.Job{Payload: rawPayload}
	err := handler(context.Background(), job)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if repo.SavedTitle != "Test Title" {
		t.Errorf("expected repo to save 'Test Title', got '%s'", repo.SavedTitle)
	}
}

func TestExtractHandler_Failures(t *testing.T) {
	b := &mockBrowserProvider{FailNavigate: true}
	e := &mockExtractionProvider{}
	repo := &mockMediaRepoExtract{}
	cm := browser.NewChallengeManager()

	handler := jobs.NewExtractHandler(b, e, repo, cm, nil)
	payload := jobs.ExtractPayload{URL: "http://test.com", Target: extraction.TargetManga}
	rawPayload, _ := json.Marshal(payload)
	job := &queue.Job{Payload: rawPayload}

	err := handler(context.Background(), job)
	if err == nil {
		t.Error("expected error due to navigation failure")
	}

	// Test user action required timeout
	b.FailNavigate = false
	b.UserAction = true
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err = handler(ctx, job)
	if err == nil {
		t.Error("expected error due to user action timeout")
	}

	// Test extraction failure
	b.UserAction = false
	e.FailExtract = true
	err = handler(context.Background(), job)
	if err == nil {
		t.Error("expected error due to extraction failure")
	}
}

func TestExtractHandlerResumesOriginalURLAfterLogin(t *testing.T) {
	b := &handoffBrowser{snapshots: []*browser.PageSnapshot{
		{HTML: "<form><input type=password></form>", FinalURL: "https://example.test/login", UserAction: true, UserActionKind: browser.UserActionLogin},
		{HTML: "<main>Requested manga page</main>", FinalURL: "https://example.test/manga/1"},
	}}
	extractor := &mockExtractionProvider{JSONReturn: json.RawMessage(`{"title":"Authenticated Manga"}`)}
	repo := &mockMediaRepoExtract{}
	manager := browser.NewChallengeManager()
	hub := events.NewHub()
	job := &queue.Job{ID: "login-job"}
	payload := jobs.ExtractPayload{URL: "https://example.test/manga/1", Target: extraction.TargetManga}
	job.Payload, _ = json.Marshal(payload)

	stream := hub.Subscribe("job:" + job.ID)
	defer hub.Unsubscribe("job:"+job.ID, stream)
	done := make(chan error, 1)
	go func() {
		done <- jobs.NewExtractHandler(b, extractor, repo, manager, hub)(context.Background(), job)
	}()

	deadline := time.After(2 * time.Second)
	for {
		select {
		case raw := <-stream:
			event, _ := raw.(map[string]any)
			if event["event"] != "challenge" {
				continue
			}
			data, _ := event["data"].(map[string]string)
			if data["kind"] != string(browser.UserActionLogin) {
				t.Fatalf("challenge kind = %q, want login", data["kind"])
			}
			token := strings.TrimPrefix(data["challenge_url"], "/api/v1/challenges/")
			if _, err := manager.BeginVerification(token); err != nil {
				t.Fatalf("BeginVerification() error = %v", err)
			}
			if _, err := manager.Resolve(token); err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			goto waitForResult
		case <-deadline:
			t.Fatal("timed out waiting for login handoff event")
		}
	}

waitForResult:
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("handler error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for resumed extraction")
	}
	if b.navigations != 2 {
		t.Fatalf("navigations = %d, want initial + resumed URL", b.navigations)
	}
	if b.closes != 2 {
		t.Fatalf("closes = %d, want both browser sessions closed", b.closes)
	}
	if repo.SavedTitle != "Authenticated Manga" {
		t.Fatalf("saved title = %q", repo.SavedTitle)
	}
}
