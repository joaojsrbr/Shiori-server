package jobs

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/joaojsr/shiori-server/internal/library"
	"github.com/joaojsr/shiori-server/internal/platform/events"
	"github.com/joaojsr/shiori-server/internal/platform/httpserver"
	"github.com/joaojsr/shiori-server/internal/platform/queue"
)

// Handler manages the HTTP endpoints for job queuing.
type Handler struct {
	q   queue.Provider
	hub *events.Hub
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

	if payload.URL == "" || payload.Target == "" {
		httpserver.RespondError(w, httpserver.Problem{
			Status: http.StatusBadRequest,
			Title:  "Invalid Request",
			Detail: "url and target are required",
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
