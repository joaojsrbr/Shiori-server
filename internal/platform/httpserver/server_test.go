package httpserver_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/joaojsr/shiori-server/internal/platform/httpserver"
)

func newTestServer() *httpserver.Server {
	logger := slog.Default()
	return httpserver.New("127.0.0.1:0", logger)
}

func TestLiveEndpoint(t *testing.T) {
	srv := newTestServer()

	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	rec := httptest.NewRecorder()

	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /health/live status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if body["status"] != "alive" {
		t.Errorf("status = %q, want %q", body["status"], "alive")
	}
}

func TestReadyEndpointNotReady(t *testing.T) {
	srv := newTestServer()

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rec := httptest.NewRecorder()

	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("GET /health/ready (not ready) status = %d, want %d",
			rec.Code, http.StatusServiceUnavailable)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/problem+json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/problem+json")
	}
}

func TestReadyEndpointAfterMarkReady(t *testing.T) {
	srv := newTestServer()
	srv.MarkReady()

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rec := httptest.NewRecorder()

	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /health/ready (ready) status = %d, want %d",
			rec.Code, http.StatusOK)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if body["status"] != "ready" {
		t.Errorf("status = %q, want %q", body["status"], "ready")
	}
}

func TestRespondJSON(t *testing.T) {
	rec := httptest.NewRecorder()

	httpserver.RespondJSON(rec, http.StatusCreated, map[string]int{"count": 42})

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}

	var body map[string]int
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if body["count"] != 42 {
		t.Errorf("count = %d, want 42", body["count"])
	}
}

func TestRespondError(t *testing.T) {
	rec := httptest.NewRecorder()

	httpserver.RespondError(rec, httpserver.Problem{
		Status: http.StatusNotFound,
		Title:  "Not Found",
		Detail: "Media not found",
	})

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/problem+json")
	}

	var p httpserver.Problem
	if err := json.NewDecoder(rec.Body).Decode(&p); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if p.Type != "about:blank" {
		t.Errorf("type = %q, want %q", p.Type, "about:blank")
	}
	if p.Title != "Not Found" {
		t.Errorf("title = %q, want %q", p.Title, "Not Found")
	}
}

func TestMarkReadyIdempotent(t *testing.T) {
	srv := newTestServer()

	// Should not panic when called multiple times.
	srv.MarkReady()
	srv.MarkReady()

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rec := httptest.NewRecorder()

	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d after double MarkReady, want %d", rec.Code, http.StatusOK)
	}
}
