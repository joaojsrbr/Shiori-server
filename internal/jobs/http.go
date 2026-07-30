package jobs

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/joaojsr/shiori-server/internal/library"
	"github.com/joaojsr/shiori-server/internal/platform/httpserver"
	"github.com/joaojsr/shiori-server/internal/platform/queue"
)

// Handler manages the HTTP endpoints for job queuing.
type Handler struct {
	q queue.Provider
}

// NewHandler creates a new HTTP handler for jobs.
func NewHandler(q queue.Provider) *Handler {
	return &Handler{q: q}
}

// RegisterRoutes registers the jobs routes to the router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Post("/jobs/extract", h.EnqueueExtract)
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
		IdempotencyKey: payload.URL, // Basic idempotency by URL
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
