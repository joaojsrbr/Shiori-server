package jobs_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/joaojsr/shiori-server/internal/jobs"
	"github.com/joaojsr/shiori-server/internal/platform/browser"
	"github.com/joaojsr/shiori-server/internal/platform/queue"
)

// mockQueueProvider provides a dummy implementation of queue.Provider
type mockQueueProvider struct {
	Enqueued bool
}

func TestDebugExtractRouteRegistration(t *testing.T) {
	browserProvider := &mockBrowserProvider{}
	extractor := &mockExtractionProvider{}
	repo := &mockMediaRepoExtract{}
	manager := browser.NewChallengeManager()

	tests := []struct {
		name     string
		enabled  bool
		wantCode int
	}{
		{name: "disabled outside debug", enabled: false, wantCode: http.StatusNotFound},
		{name: "registered in debug", enabled: true, wantCode: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := chi.NewRouter()
			jobs.RegisterDebugExtractRoute(router, tt.enabled, browserProvider, extractor, repo, manager)
			request := httptest.NewRequest(http.MethodPost, "/debug/extract", bytes.NewBufferString(`{"target":"manga"}`))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d", response.Code, tt.wantCode)
			}
		})
	}
}

func (m *mockQueueProvider) Enqueue(ctx context.Context, job *queue.Job) error {
	m.Enqueued = true
	return nil
}
func (m *mockQueueProvider) Dequeue(ctx context.Context, types []string) (*queue.Job, error) {
	return nil, nil
}
func (m *mockQueueProvider) Ack(ctx context.Context, jobID string) error                 { return nil }
func (m *mockQueueProvider) Nack(ctx context.Context, jobID string, reason string) error { return nil }
func (m *mockQueueProvider) Heartbeat(ctx context.Context, jobID string) error           { return nil }
func (m *mockQueueProvider) Cancel(ctx context.Context, jobID string) error              { return nil }
func (m *mockQueueProvider) Status(ctx context.Context, jobID string) (*queue.Job, error) {
	return nil, nil
}

func TestEnqueueExtract(t *testing.T) {
	q := &mockQueueProvider{}
	h := jobs.NewHandler(q, nil)

	r := chi.NewRouter()
	h.RegisterRoutes(r)

	body := []byte(`{"url": "https://test.com", "target": "media"}`)
	req := httptest.NewRequest(http.MethodPost, "/jobs/extract", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Errorf("expected 202 Accepted, got %d", rr.Code)
	}

	if !q.Enqueued {
		t.Error("expected job to be enqueued")
	}
}

func TestEnqueueExtract_Invalid(t *testing.T) {
	q := &mockQueueProvider{}
	h := jobs.NewHandler(q, nil)

	r := chi.NewRouter()
	h.RegisterRoutes(r)

	// Missing target
	body := []byte(`{"url": "https://test.com"}`)
	req := httptest.NewRequest(http.MethodPost, "/jobs/extract", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request, got %d", rr.Code)
	}
}
