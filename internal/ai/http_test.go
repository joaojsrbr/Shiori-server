package ai_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/joaojsr/shiori-server/internal/ai"
	"github.com/joaojsr/shiori-server/internal/platform/config"
)

func TestHandler_ListModels(t *testing.T) {
	cfg := config.AIConfig{
		LMStudioBaseURL: "http://invalid-url-for-test",
		ModelTiny:       "tiny",
		ModelDefault:    "default",
		ModelQuality:    "quality",
	}

	h := ai.NewHandler(cfg)
	r := chi.NewRouter()
	h.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/ai/models", nil)
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	// Will fail because LM studio is not mocked, but HTTP layer works
	if rr.Code != http.StatusBadGateway {
		t.Errorf("expected 502 Bad Gateway due to invalid LM Studio URL, got %d", rr.Code)
	}
}

func TestHandler_LoadModel(t *testing.T) {
	cfg := config.AIConfig{
		LMStudioBaseURL: "http://invalid-url-for-test",
		ModelTiny:       "tiny",
		ModelDefault:    "default",
		ModelQuality:    "quality",
	}

	h := ai.NewHandler(cfg)
	r := chi.NewRouter()
	h.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/ai/models/tiny/load", nil)
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Errorf("expected 502 Bad Gateway, got %d", rr.Code)
	}

	reqInvalid := httptest.NewRequest(http.MethodPost, "/ai/models/invalid/load", nil)
	rrInvalid := httptest.NewRecorder()
	r.ServeHTTP(rrInvalid, reqInvalid)

	if rrInvalid.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for invalid key, got %d", rrInvalid.Code)
	}
}
