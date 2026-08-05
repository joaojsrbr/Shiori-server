package lmstudio_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/joaojsr/shiori-server/internal/platform/ai/lmstudio"
)

func TestClient_ListModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/models" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		response := map[string]interface{}{
			"models": []map[string]string{
				{"id": "nuextract3@q4_k_m", "object": "model"},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer srv.Close()

	client := lmstudio.NewClient(srv.URL, "fake-token")
	models, err := client.ListModels(context.Background())

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}

	if models[0].ID != "nuextract3@q4_k_m" {
		t.Errorf("expected nuextract3@q4_k_m, got %s", models[0].ID)
	}
}

func TestClient_LoadModel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/models/load" || r.Method != http.MethodPost {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := lmstudio.NewClient(srv.URL, "")
	err := client.LoadModel(context.Background(), "nuextract-1.5-tiny", 8192)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}
