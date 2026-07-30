package httpserver

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// Problem represents an RFC 9457 problem detail.
type Problem struct {
	Type     string `json:"type,omitempty"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail,omitempty"`
	Instance string `json:"instance,omitempty"`
}

// RespondError writes an RFC 9457 problem+json error response.
func RespondError(w http.ResponseWriter, p Problem) {
	if p.Type == "" {
		p.Type = "about:blank"
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(p.Status)
	if err := json.NewEncoder(w).Encode(p); err != nil {
		slog.Error("failed to encode error response", "error", err)
	}
}

// ErrorNotFound returns a Problem representing a 404 Not Found error.
func ErrorNotFound(detail string) Problem {
	return Problem{
		Status: http.StatusNotFound,
		Title:  "Not Found",
		Detail: detail,
	}
}

// ErrorBadRequest returns a Problem representing a 400 Bad Request error.
func ErrorBadRequest(detail string) Problem {
	return Problem{
		Status: http.StatusBadRequest,
		Title:  "Bad Request",
		Detail: detail,
	}
}

// ErrorValidation returns a Problem representing a 422 Unprocessable Entity error.
func ErrorValidation(detail string) Problem {
	return Problem{
		Status: http.StatusUnprocessableEntity,
		Title:  "Validation Error",
		Detail: detail,
	}
}

// ErrorInternal returns a Problem representing a 500 Internal Server Error.
// It logs the internal error if provided.
func ErrorInternal(err error, detail string) Problem {
	if err != nil {
		slog.Error("internal server error", "error", err)
	}
	if detail == "" {
		detail = "An unexpected error occurred."
	}
	return Problem{
		Status: http.StatusInternalServerError,
		Title:  "Internal Server Error",
		Detail: detail,
	}
}

// ErrorConflict returns a Problem representing a 409 Conflict error.
func ErrorConflict(detail string) Problem {
	return Problem{
		Status: http.StatusConflict,
		Title:  "Conflict",
		Detail: detail,
	}
}

// ErrorMethodNotAllowed returns a Problem representing a 405 Method Not Allowed error.
func ErrorMethodNotAllowed(detail string) Problem {
	return Problem{
		Status: http.StatusMethodNotAllowed,
		Title:  "Method Not Allowed",
		Detail: detail,
	}
}
