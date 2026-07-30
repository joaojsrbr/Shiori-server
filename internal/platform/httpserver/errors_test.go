package httpserver_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/joaojsr/shiori-server/internal/platform/httpserver"
)

func TestErrorHelpers(t *testing.T) {
	tests := []struct {
		name     string
		problem  httpserver.Problem
		wantStat int
		wantTit  string
		wantDet  string
	}{
		{
			name:     "ErrorNotFound",
			problem:  httpserver.ErrorNotFound("media not found"),
			wantStat: http.StatusNotFound,
			wantTit:  "Not Found",
			wantDet:  "media not found",
		},
		{
			name:     "ErrorBadRequest",
			problem:  httpserver.ErrorBadRequest("invalid id"),
			wantStat: http.StatusBadRequest,
			wantTit:  "Bad Request",
			wantDet:  "invalid id",
		},
		{
			name:     "ErrorValidation",
			problem:  httpserver.ErrorValidation("missing title"),
			wantStat: http.StatusUnprocessableEntity,
			wantTit:  "Validation Error",
			wantDet:  "missing title",
		},
		{
			name:     "ErrorConflict",
			problem:  httpserver.ErrorConflict("already exists"),
			wantStat: http.StatusConflict,
			wantTit:  "Conflict",
			wantDet:  "already exists",
		},
		{
			name:     "ErrorInternal",
			problem:  httpserver.ErrorInternal(errors.New("db error"), "failed to save"),
			wantStat: http.StatusInternalServerError,
			wantTit:  "Internal Server Error",
			wantDet:  "failed to save",
		},
		{
			name:     "ErrorInternal Default Detail",
			problem:  httpserver.ErrorInternal(nil, ""),
			wantStat: http.StatusInternalServerError,
			wantTit:  "Internal Server Error",
			wantDet:  "An unexpected error occurred.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.problem.Status != tt.wantStat {
				t.Errorf("Status = %d, want %d", tt.problem.Status, tt.wantStat)
			}
			if tt.problem.Title != tt.wantTit {
				t.Errorf("Title = %q, want %q", tt.problem.Title, tt.wantTit)
			}
			if tt.problem.Detail != tt.wantDet {
				t.Errorf("Detail = %q, want %q", tt.problem.Detail, tt.wantDet)
			}
		})
	}
}
