package library

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/joaojsr/shiori-server/internal/platform/httpserver"
)

// Handler handles HTTP requests for library domain.
type Handler struct {
	repo MediaRepository
}

// NewHandler creates a new Library Handler.
func NewHandler(repo MediaRepository) *Handler {
	return &Handler{repo: repo}
}

// RegisterRoutes attaches the routes to the given chi.Router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/media", h.ListMedia)
	r.Post("/media", h.CreateMedia)
	r.Get("/media/{mediaId}", h.GetMedia)
}

func (h *Handler) ListMedia(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	items, err := h.repo.List(ctx)
	if err != nil {
		httpserver.RespondError(w, httpserver.Problem{
			Status: http.StatusInternalServerError,
			Title:  "Database Error",
			Detail: "Failed to list media from the library.",
		})
		return
	}

	// Always return an array, not null
	if items == nil {
		items = []*Media{}
	}

	httpserver.RespondJSON(w, http.StatusOK, items)
}

func (h *Handler) CreateMedia(w http.ResponseWriter, r *http.Request) {
	var req MediaCreateRequest
	if err := httpserver.DecodeJSON(r, &req); err != nil {
		httpserver.RespondError(w, httpserver.Problem{
			Status: http.StatusBadRequest,
			Title:  "Invalid Request",
			Detail: err.Error(),
		})
		return
	}

	// Simplistic validation
	if req.Title == "" || req.Type == "" || req.Status == "" {
		httpserver.RespondError(w, httpserver.Problem{
			Status: http.StatusBadRequest,
			Title:  "Validation Error",
			Detail: "title, type, and status are required fields.",
		})
		return
	}

	if req.AlternativeTitles == nil {
		req.AlternativeTitles = []string{}
	}
	if req.Authors == nil {
		req.Authors = []string{}
	}
	if req.Artists == nil {
		req.Artists = []string{}
	}
	if req.Genres == nil {
		req.Genres = []string{}
	}

	m, err := h.repo.Create(r.Context(), req)
	if err != nil {
		httpserver.RespondError(w, httpserver.Problem{
			Status: http.StatusInternalServerError,
			Title:  "Database Error",
			Detail: "Failed to create media in the library.",
		})
		return
	}

	httpserver.RespondJSON(w, http.StatusCreated, m)
}

func (h *Handler) GetMedia(w http.ResponseWriter, r *http.Request) {
	mediaID := httpserver.PathParam(r, "mediaId")

	m, err := h.repo.GetByID(r.Context(), mediaID)
	if err != nil {
		httpserver.RespondError(w, httpserver.Problem{
			Status: http.StatusInternalServerError,
			Title:  "Database Error",
			Detail: "Failed to get media from the library.",
		})
		return
	}

	if m == nil {
		httpserver.RespondError(w, httpserver.Problem{
			Status: http.StatusNotFound,
			Title:  "Not Found",
			Detail: "The specified media was not found.",
		})
		return
	}

	httpserver.RespondJSON(w, http.StatusOK, m)
}
