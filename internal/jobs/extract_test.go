package jobs_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/joaojsr/shiori-server/internal/extraction"
	"github.com/joaojsr/shiori-server/internal/jobs"
	"github.com/joaojsr/shiori-server/internal/library"
	"github.com/joaojsr/shiori-server/internal/platform/browser"
	"github.com/joaojsr/shiori-server/internal/platform/queue"
)

type mockBrowserProvider struct {
	FailNavigate bool
	FailSnapshot bool
	UserAction   bool
	HTMLReturn   string
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
func (m *mockBrowserProvider) CloseSession(ctx context.Context, sessionID string) error { return nil }

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
func (m *mockMediaRepoExtract) GetByID(ctx context.Context, id string) (*library.Media, error) { return nil, nil }
func (m *mockMediaRepoExtract) List(ctx context.Context) ([]*library.Media, error) { return nil, nil }
func (m *mockMediaRepoExtract) Delete(ctx context.Context, id string) error { return nil }

func TestExtractHandler_Success(t *testing.T) {
	b := &mockBrowserProvider{HTMLReturn: "<html><body>Content</body></html>"}
	e := &mockExtractionProvider{JSONReturn: json.RawMessage(`{"title": "Test Title"}`)}
	repo := &mockMediaRepoExtract{}

	handler := jobs.NewExtractHandler(b, e, repo)

	payload := jobs.ExtractPayload{URL: "http://test.com", Target: extraction.TargetMedia}
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

	handler := jobs.NewExtractHandler(b, e, repo)
	payload := jobs.ExtractPayload{URL: "http://test.com", Target: extraction.TargetMedia}
	rawPayload, _ := json.Marshal(payload)
	job := &queue.Job{Payload: rawPayload}

	err := handler(context.Background(), job)
	if err == nil {
		t.Error("expected error due to navigation failure")
	}

	// Test user action required
	b.FailNavigate = false
	b.UserAction = true
	err = handler(context.Background(), job)
	if err == nil {
		t.Error("expected error due to user action required")
	}

	// Test extraction failure
	b.UserAction = false
	e.FailExtract = true
	err = handler(context.Background(), job)
	if err == nil {
		t.Error("expected error due to extraction failure")
	}
}
