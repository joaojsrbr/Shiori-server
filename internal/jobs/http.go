package jobs

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/joaojsr/shiori-server/internal/extraction"
	"github.com/joaojsr/shiori-server/internal/library"
	"github.com/joaojsr/shiori-server/internal/platform/browser"
	"github.com/joaojsr/shiori-server/internal/platform/events"
	"github.com/joaojsr/shiori-server/internal/platform/httpserver"
	"github.com/joaojsr/shiori-server/internal/platform/queue"
)

// Handler manages the HTTP endpoints for job queuing.
type Handler struct {
	q   queue.Provider
	hub *events.Hub
}

// RegisterDebugExtractRoute attaches the synchronous extraction endpoint only
// when debug mode is explicitly enabled. Keeping the gate beside the route
// prevents accidental exposure from a caller that reuses the handler.
func RegisterDebugExtractRoute(
	r chi.Router,
	enabled bool,
	b browser.Provider,
	ext extraction.Provider,
	repo library.MediaRepository,
	cm *browser.ChallengeManager,
) {
	if !enabled || b == nil || ext == nil {
		return
	}
	r.Post("/debug/extract", HandleDebugExtract(b, ext, repo, cm))
}

// HandleDebugExtract runs the same worker pipeline synchronously and relays its
// progress as SSE. The extraction implementation remains single-sourced in
// NewExtractHandler instead of drifting into a second debug-only copy.
func HandleDebugExtract(
	b browser.Provider,
	ext extraction.Provider,
	repo library.MediaRepository,
	cm *browser.ChallengeManager,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload ExtractPayload
		if err := httpserver.DecodeJSON(r, &payload); err != nil {
			httpserver.RespondError(w, httpserver.Problem{Status: http.StatusBadRequest, Title: "Invalid Payload", Detail: err.Error()})
			return
		}
		if payload.URL == "" || payload.Target == "" {
			httpserver.RespondError(w, httpserver.Problem{Status: http.StatusBadRequest, Title: "Invalid Request", Detail: "URL and Target are required"})
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			httpserver.RespondError(w, httpserver.Problem{Status: http.StatusInternalServerError, Title: "SSE Not Supported", Detail: "Streaming unsupported by client"})
			return
		}

		rawPayload, _ := json.Marshal(payload)
		jobID := library.NewULID()
		job := &queue.Job{ID: jobID, Type: "extract_media", Payload: rawPayload}
		hub := events.NewHub()
		topic := "job:" + jobID
		stream := hub.Subscribe(topic)
		defer hub.Unsubscribe(topic, stream)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		done := make(chan error, 1)
		go func() {
			done <- NewExtractHandler(b, ext, repo, cm, hub)(r.Context(), job)
		}()

		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		forward := func(eventData any) bool {
			message, ok := eventData.(map[string]any)
			if !ok {
				return false
			}
			event, _ := message["event"].(string)
			data, _ := json.Marshal(message["data"])
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
			flusher.Flush()
			return event == "done" || event == "error"
		}
		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				fmt.Fprint(w, ": heartbeat\n\n")
				flusher.Flush()
			case err := <-done:
				// The worker publishes its terminal event before returning. Drain
				// queued events first so select scheduling cannot hide that detail.
				for draining := true; draining; {
					select {
					case eventData := <-stream:
						if forward(eventData) {
							return
						}
					default:
						draining = false
					}
				}
				if err != nil {
					// NewExtractHandler normally emits the detailed error first; this
					// fallback covers failures before event publication.
					data, _ := json.Marshal(map[string]string{"title": "Extraction Error", "detail": err.Error()})
					fmt.Fprintf(w, "event: error\ndata: %s\n\n", data)
					flusher.Flush()
				}
				return
			case eventData := <-stream:
				if forward(eventData) {
					return
				}
			}
		}
	}
}

// NewHandler creates a new HTTP handler for jobs.
func NewHandler(q queue.Provider, hub *events.Hub) *Handler {
	return &Handler{q: q, hub: hub}
}

// RegisterRoutes registers the jobs routes to the router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Post("/jobs/extract", h.EnqueueExtract)
	r.Get("/jobs/{jobID}/events", h.JobEvents)
}

func (h *Handler) EnqueueExtract(w http.ResponseWriter, r *http.Request) {
	var payload ExtractPayload
	if err := httpserver.DecodeJSON(r, &payload); err != nil {
		httpserver.RespondError(w, httpserver.Problem{
			Status: http.StatusBadRequest,
			Title:  "Invalid Request",
			Detail: err.Error(),
		})
		return
	}

	if err := payload.Validate(); err != nil {
		httpserver.RespondError(w, httpserver.Problem{
			Status: http.StatusBadRequest,
			Title:  "Invalid Request",
			Detail: err.Error(),
		})
		return
	}

	rawPayload, _ := json.Marshal(payload)

	job := &queue.Job{
		ID:             library.NewULID(),
		IdempotencyKey: library.NewULID(), // Allow re-extracting same URL
		Type:           "extract_media",
		Payload:        rawPayload,
	}

	if err := h.q.Enqueue(r.Context(), job); err != nil {
		if errors.Is(err, queue.ErrConflict) {
			httpserver.RespondError(w, httpserver.Problem{
				Status: http.StatusConflict,
				Title:  "Conflict",
				Detail: "A job for this URL is already enqueued.",
			})
			return
		}

		httpserver.RespondError(w, httpserver.Problem{
			Status: http.StatusInternalServerError,
			Title:  "Queue Error",
			Detail: "Failed to enqueue job: " + err.Error(),
		})
		return
	}

	resp := map[string]string{
		"job_id": job.ID,
	}

	httpserver.RespondJSON(w, http.StatusAccepted, resp)
}

func (h *Handler) JobEvents(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")
	if jobID == "" {
		httpserver.RespondError(w, httpserver.Problem{
			Status: http.StatusBadRequest,
			Title:  "Invalid Request",
			Detail: "jobID is required",
		})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		httpserver.RespondError(w, httpserver.Problem{
			Status: http.StatusInternalServerError,
			Title:  "SSE Not Supported",
			Detail: "Streaming unsupported by client",
		})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	sendEvent := func(evt string, data any) {
		b, _ := json.Marshal(data)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt, string(b))
		flusher.Flush()
	}

	// We only have the hub. If the job is already done, it won't emit new events.
	// In a complete system, we would check the DB for the job status first.
	// For now, we subscribe to live events.
	topic := "job:" + jobID
	ch := h.hub.Subscribe(topic)
	defer h.hub.Unsubscribe(topic, ch)

	// Keep alive loop or wait for events
	ctx := r.Context()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Send a heartbeat comment to keep the connection alive
			// Most proxies/browsers drop idle connections after 60-120s
			fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
		case eventData := <-ch:
			// eventData is map[string]any{"event": evt, "data": data}
			if m, ok := eventData.(map[string]any); ok {
				evt, _ := m["event"].(string)
				data := m["data"]
				sendEvent(evt, data)

				if evt == "done" || evt == "error" {
					return
				}
			}
		}
	}
}
