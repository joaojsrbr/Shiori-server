package openapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/joaojsr/shiori-server/api/openapi"
)

func TestHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil)
	rr := httptest.NewRecorder()

	handler := openapi.Handler()
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	contentType := rr.Header().Get("Content-Type")
	if contentType != "application/yaml" {
		t.Errorf("handler returned unexpected content type: got %v want %v", contentType, "application/yaml")
	}

	if len(rr.Body.String()) == 0 {
		t.Errorf("expected non-empty body")
	}
}
