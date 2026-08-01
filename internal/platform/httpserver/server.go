// Package httpserver provides the HTTP server setup, middleware, and shared
// response helpers for the Shiori API.
package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Server wraps an http.Server with Shiori-specific configuration.
type Server struct {
	http    *http.Server
	router  chi.Router
	logger  *slog.Logger
	readyCh chan struct{}
	ready   bool
}

// New creates a new Server with the given address and logger.
func New(addr string, logger *slog.Logger) *Server {
	r := chi.NewRouter()

	s := &Server{
		router:  r,
		logger:  logger,
		readyCh: make(chan struct{}),
	}

	// Custom JSON error handlers for unmatched routes
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		RespondError(w, ErrorNotFound("Endpoint not found."))
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		RespondError(w, ErrorMethodNotAllowed("HTTP method not allowed for this endpoint."))
	})

	// Global middleware stack
	r.Use(middleware.RealIP)
	r.Use(middleware.RequestID)
	r.Use(requestLogger(logger))
	r.Use(middleware.Recoverer)

	// Health endpoints outside /api/v1
	r.Get("/health/live", s.handleLive)
	r.Get("/health/ready", s.handleReady)

	s.http = &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
		BaseContext: func(_ net.Listener) context.Context {
			return context.Background()
		},
	}

	return s
}

// Router returns the chi router for registering additional routes.
func (s *Server) Router() chi.Router {
	return s.router
}

// SetTimeouts configures the HTTP server timeouts.
func (s *Server) SetTimeouts(read, write, idle time.Duration) {
	s.http.ReadTimeout = read
	s.http.WriteTimeout = write
	s.http.IdleTimeout = idle
}

// MarkReady signals that the server is ready to accept traffic.
// Call this after all initialization (migrations, etc.) is complete.
func (s *Server) MarkReady() {
	if !s.ready {
		s.ready = true
		close(s.readyCh)
	}
}

// ListenAndServe starts the HTTP server. It blocks until the server stops.
func (s *Server) ListenAndServe() error {
	s.logger.Info("starting HTTP server", "addr", s.http.Addr)
	err := s.http.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Shutdown gracefully stops the server with the given timeout.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("shutting down HTTP server")
	return s.http.Shutdown(ctx)
}

// --- Health Handlers ---

func (s *Server) handleLive(w http.ResponseWriter, _ *http.Request) {
	RespondJSON(w, http.StatusOK, map[string]string{"status": "alive"})
}

func (s *Server) handleReady(w http.ResponseWriter, _ *http.Request) {
	if !s.ready {
		RespondError(w, Problem{
			Status: http.StatusServiceUnavailable,
			Title:  "Service Unavailable",
			Detail: "Server is not yet ready to accept requests.",
		})
		return
	}
	RespondJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

// --- Response Helpers ---

// RespondJSON writes a JSON response with the given status code.
func RespondJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to encode JSON response", "error", err)
	}
}

// DecodeJSON reads and decodes a JSON request body into v.
// It limits the body to 1MB.
func DecodeJSON(r *http.Request, v any) error {
	const maxBodyBytes = 1 << 20 // 1 MB
	r.Body = http.MaxBytesReader(nil, r.Body, maxBodyBytes)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("decoding JSON: %w", err)
	}
	return nil
}

// PathParam extracts a URL path parameter by name from the chi context.
func PathParam(r *http.Request, key string) string {
	return chi.URLParam(r, key)
}

// --- Middleware ---

func requestLogger(logger *slog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			logger.LogAttrs(r.Context(), slog.LevelInfo, "http request",
				slog.String("method", r.Method),
				slog.String("path", redactSensitivePath(r.URL.Path)),
				slog.Int("status", ww.Status()),
				slog.Int("bytes", ww.BytesWritten()),
				slog.Duration("duration", time.Since(start)),
				slog.String("request_id", middleware.GetReqID(r.Context())),
			)
		})
	}
}

func redactSensitivePath(path string) string {
	const prefix = "/api/v1/challenges/"
	if !strings.HasPrefix(path, prefix) {
		return path
	}
	remainder := strings.TrimPrefix(path, prefix)
	if remainder == "assets/client.js" {
		return path
	}
	parts := strings.SplitN(remainder, "/", 2)
	redacted := prefix + "{redacted}"
	if len(parts) == 2 {
		redacted += "/" + parts[1]
	}
	return redacted
}
