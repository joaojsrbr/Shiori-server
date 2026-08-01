package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestRegisterAPIV1RoutesMountsCoreAndDebugTogether(t *testing.T) {
	router := chi.NewRouter()

	registerAPIV1Routes(router, func(r chi.Router) {
		r.Get("/capabilities", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	}, func(r chi.Router) {
		r.Post("/debug/extract", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
		})
	})

	tests := []struct {
		method string
		path   string
		status int
	}{
		{method: http.MethodGet, path: "/api/v1/capabilities", status: http.StatusOK},
		{method: http.MethodPost, path: "/api/v1/debug/extract", status: http.StatusBadRequest},
	}
	for _, tt := range tests {
		request := httptest.NewRequest(tt.method, tt.path, nil)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != tt.status {
			t.Errorf("%s %s status = %d, want %d", tt.method, tt.path, response.Code, tt.status)
		}
	}
}
