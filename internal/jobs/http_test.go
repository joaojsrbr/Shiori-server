package jobs_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/joaojsr/shiori-server/internal/jobs"
	"github.com/joaojsr/shiori-server/internal/platform/queue"
)

// mockQueueProvider provides a dummy implementation of queue.Provider
type mockQueueProvider struct {
	Enqueued bool
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
