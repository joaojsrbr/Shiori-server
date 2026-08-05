package library

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/joaojsr/shiori-server/internal/platform/httpserver"
	"github.com/joaojsr/shiori-server/internal/platform/storage"
)

// Handler handles HTTP requests for library domain.
type Handler struct {
	repo     MediaRepository
	chapters ChapterRepository
	storage  storage.Provider
	features FeatureRepository
}

// NewHandler creates a new Library Handler.
func NewHandler(repo MediaRepository, dependencies ...any) *Handler {
	h := &Handler{repo: repo}
	for _, dependency := range dependencies {
		if chapters, ok := dependency.(ChapterRepository); ok {
			h.chapters = chapters
		}
		if files, ok := dependency.(storage.Provider); ok {
			h.storage = files
		}
		if features, ok := dependency.(FeatureRepository); ok {
			h.features = features
		}
	}
	return h
}

// RegisterRoutes attaches the routes to the given chi.Router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/media", h.ListMedia)
	r.Post("/media", h.CreateMedia)
	r.Get("/media/{mediaId}", h.GetMedia)
	r.Delete("/media/{mediaId}", h.DeleteMedia)
	if h.chapters != nil {
		r.Get("/media/{mediaId}/chapters", h.ListChapters)
		r.Get("/chapters/{chapterId}", h.GetChapter)
	}
	if h.storage != nil {
		r.Get("/reader/assets/*", h.GetReaderAsset)
	}
	if h.features != nil {
		r.Get("/collections", h.ListCollections)
		r.Post("/collections", h.CreateCollection)
		r.Get("/collections/{collectionId}", h.GetCollection)
		r.Patch("/collections/{collectionId}", h.UpdateCollection)
		r.Delete("/collections/{collectionId}", h.DeleteCollection)
		r.Get("/collections/{collectionId}/media", h.ListCollectionMedia)
		r.Put("/collections/{collectionId}/media/{mediaId}", h.AddCollectionMedia)
		r.Delete("/collections/{collectionId}/media/{mediaId}", h.RemoveCollectionMedia)
		r.Get("/history", h.ListHistory)
		r.Put("/history/{chapterId}", h.UpsertHistory)
		r.Delete("/history/{chapterId}", h.DeleteHistory)
		r.Get("/downloads", h.ListDownloads)
		r.Delete("/downloads/{chapterId}", h.DeleteDownload)
		r.Get("/profiles", h.ListProfiles)
		r.Get("/settings", h.GetSettings)
		r.Put("/settings", h.UpdateSettings)
		r.Get("/browser/history", h.ListBrowserHistory)
		r.Get("/filters/presets", h.ListFilterPresets)
		r.Post("/filters/presets", h.CreateFilterPreset)
	}
}

// DeleteMedia removes a library item and all database records that depend on
// it. The repository returns the image keys before the cascading delete so the
// handler can also remove the corresponding storage objects.
func (h *Handler) DeleteMedia(w http.ResponseWriter, r *http.Request) {
	keys, deleted, err := h.repo.Delete(r.Context(), httpserver.PathParam(r, "mediaId"))
	if err != nil {
		httpserver.RespondError(w, httpserver.Problem{Status: http.StatusInternalServerError, Title: "Database Error", Detail: "Failed to delete media from the library."})
		return
	}
	if !deleted {
		httpserver.RespondError(w, httpserver.Problem{Status: http.StatusNotFound, Title: "Not Found", Detail: "The specified media was not found."})
		return
	}
	if h.storage != nil {
		for _, key := range keys {
			if err := h.storage.Delete(r.Context(), key); err != nil {
				httpserver.RespondError(w, httpserver.Problem{Status: http.StatusInternalServerError, Title: "Storage Error", Detail: "The media was removed, but one or more image files could not be deleted."})
				return
			}
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListChapters(w http.ResponseWriter, r *http.Request) {
	items, err := h.chapters.ListChapters(r.Context(), httpserver.PathParam(r, "mediaId"))
	if err != nil {
		httpserver.RespondError(w, httpserver.Problem{Status: 500, Title: "Database Error", Detail: "Failed to list chapters."})
		return
	}
	for _, item := range items {
		exposeImageURLs(item)
	}
	httpserver.RespondJSON(w, http.StatusOK, items)
}

func (h *Handler) GetChapter(w http.ResponseWriter, r *http.Request) {
	item, err := h.chapters.GetChapter(r.Context(), httpserver.PathParam(r, "chapterId"))
	if err != nil {
		httpserver.RespondError(w, httpserver.Problem{Status: 500, Title: "Database Error", Detail: "Failed to get chapter."})
		return
	}
	if item == nil {
		httpserver.RespondError(w, httpserver.Problem{Status: 404, Title: "Not Found", Detail: "The specified chapter was not found."})
		return
	}
	exposeImageURLs(item)
	httpserver.RespondJSON(w, http.StatusOK, item)
}

func exposeImageURLs(chapter *Chapter) {
	for i := range chapter.Images {
		chapter.Images[i].URL = "/api/v1/reader/assets/" + chapter.Images[i].StorageKey
	}
}

func (h *Handler) GetReaderAsset(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(chi.URLParam(r, "*"), "/")
	if key == "" {
		http.NotFound(w, r)
		return
	}
	reader, err := h.storage.Get(r.Context(), key)
	if errors.Is(err, storage.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		httpserver.RespondError(w, httpserver.Problem{Status: 500, Title: "Storage Error", Detail: "Failed to read image."})
		return
	}
	defer reader.Close()
	contentType := mime.TypeByExtension(filepath.Ext(key))
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.Copy(w, reader)
}

func (h *Handler) ListMedia(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var items []*Media
	var next string
	var err error
	if pages, ok := h.repo.(MediaPageRepository); ok {
		items, next, err = pages.ListMediaPage(ctx, parseLimit(r), r.URL.Query().Get("cursor"))
	} else {
		items, err = h.repo.List(ctx)
	}
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
	setNextCursor(w, next)

	httpserver.RespondJSON(w, http.StatusOK, items)
}

func parseLimit(r *http.Request) int {
	n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if n < 1 {
		return 50
	}
	if n > 100 {
		return 100
	}
	return n
}
func setNextCursor(w http.ResponseWriter, cursor string) {
	if cursor != "" {
		w.Header().Set("X-Next-Cursor", cursor)
	}
}

type collectionPayload struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}
type historyPayload struct {
	Position  int     `json:"position"`
	Progress  float64 `json:"progress"`
	Completed bool    `json:"completed"`
}

func (h *Handler) ListCollections(w http.ResponseWriter, r *http.Request) {
	items, err := h.features.ListCollections(r.Context())
	if err != nil {
		h.featureError(w, err)
		return
	}
	httpserver.RespondJSON(w, http.StatusOK, items)
}
func (h *Handler) CreateCollection(w http.ResponseWriter, r *http.Request) {
	var p collectionPayload
	if err := httpserver.DecodeJSON(r, &p); err != nil || strings.TrimSpace(p.Name) == "" {
		httpserver.RespondError(w, httpserver.Problem{Status: 400, Title: "Invalid Collection", Detail: "name is required"})
		return
	}
	item, err := h.features.CreateCollection(r.Context(), strings.TrimSpace(p.Name), p.Description)
	if err != nil {
		h.featureError(w, err)
		return
	}
	httpserver.RespondJSON(w, http.StatusCreated, item)
}
func (h *Handler) GetCollection(w http.ResponseWriter, r *http.Request) {
	item, err := h.features.GetCollection(r.Context(), httpserver.PathParam(r, "collectionId"))
	if err != nil {
		h.featureError(w, err)
		return
	}
	if item == nil {
		httpserver.RespondError(w, httpserver.Problem{Status: 404, Title: "Not Found", Detail: "Collection not found."})
		return
	}
	httpserver.RespondJSON(w, http.StatusOK, item)
}
func (h *Handler) UpdateCollection(w http.ResponseWriter, r *http.Request) {
	var p collectionPayload
	if err := httpserver.DecodeJSON(r, &p); err != nil || strings.TrimSpace(p.Name) == "" {
		httpserver.RespondError(w, httpserver.Problem{Status: 400, Title: "Invalid Collection", Detail: "name is required"})
		return
	}
	item, err := h.features.UpdateCollection(r.Context(), httpserver.PathParam(r, "collectionId"), strings.TrimSpace(p.Name), p.Description)
	if err != nil {
		h.featureError(w, err)
		return
	}
	if item == nil {
		http.NotFound(w, r)
		return
	}
	httpserver.RespondJSON(w, http.StatusOK, item)
}
func (h *Handler) DeleteCollection(w http.ResponseWriter, r *http.Request) {
	if err := h.features.DeleteCollection(r.Context(), httpserver.PathParam(r, "collectionId")); err != nil {
		h.featureError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (h *Handler) ListCollectionMedia(w http.ResponseWriter, r *http.Request) {
	items, next, err := h.features.ListCollectionMedia(r.Context(), httpserver.PathParam(r, "collectionId"), parseLimit(r), r.URL.Query().Get("cursor"))
	if err != nil {
		h.featureError(w, err)
		return
	}
	setNextCursor(w, next)
	httpserver.RespondJSON(w, http.StatusOK, items)
}
func (h *Handler) AddCollectionMedia(w http.ResponseWriter, r *http.Request) {
	if err := h.features.AddCollectionMedia(r.Context(), httpserver.PathParam(r, "collectionId"), httpserver.PathParam(r, "mediaId")); err != nil {
		h.featureError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (h *Handler) RemoveCollectionMedia(w http.ResponseWriter, r *http.Request) {
	if err := h.features.RemoveCollectionMedia(r.Context(), httpserver.PathParam(r, "collectionId"), httpserver.PathParam(r, "mediaId")); err != nil {
		h.featureError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (h *Handler) ListHistory(w http.ResponseWriter, r *http.Request) {
	items, next, err := h.features.ListHistory(r.Context(), parseLimit(r), r.URL.Query().Get("cursor"))
	if err != nil {
		h.featureError(w, err)
		return
	}
	setNextCursor(w, next)
	httpserver.RespondJSON(w, http.StatusOK, items)
}
func (h *Handler) UpsertHistory(w http.ResponseWriter, r *http.Request) {
	var p historyPayload
	if err := httpserver.DecodeJSON(r, &p); err != nil || p.Progress < 0 || p.Progress > 1 || p.Position < 0 {
		httpserver.RespondError(w, httpserver.Problem{Status: 400, Title: "Invalid Progress", Detail: "position must be non-negative and progress must be between 0 and 1"})
		return
	}
	item, err := h.features.UpsertHistory(r.Context(), httpserver.PathParam(r, "chapterId"), p.Position, p.Progress, p.Completed)
	if err != nil {
		h.featureError(w, err)
		return
	}
	httpserver.RespondJSON(w, http.StatusOK, item)
}
func (h *Handler) DeleteHistory(w http.ResponseWriter, r *http.Request) {
	if err := h.features.DeleteHistory(r.Context(), httpserver.PathParam(r, "chapterId")); err != nil {
		h.featureError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (h *Handler) ListDownloads(w http.ResponseWriter, r *http.Request) {
	items, next, err := h.features.ListDownloads(r.Context(), parseLimit(r), r.URL.Query().Get("cursor"))
	if err != nil {
		h.featureError(w, err)
		return
	}
	setNextCursor(w, next)
	httpserver.RespondJSON(w, http.StatusOK, items)
}
func (h *Handler) DeleteDownload(w http.ResponseWriter, r *http.Request) {
	keys, err := h.features.DeleteDownload(r.Context(), httpserver.PathParam(r, "chapterId"))
	if err != nil {
		h.featureError(w, err)
		return
	}
	for _, key := range keys {
		if err := h.storage.Delete(r.Context(), key); err != nil {
			httpserver.RespondError(w, httpserver.Problem{Status: 500, Title: "Storage Error", Detail: "Database records were removed, but one or more files could not be deleted: " + err.Error()})
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
func (h *Handler) featureError(w http.ResponseWriter, err error) {
	httpserver.RespondError(w, httpserver.Problem{Status: 500, Title: "Database Error", Detail: err.Error()})
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

func (h *Handler) ListProfiles(w http.ResponseWriter, r *http.Request) {
	items, err := h.features.ListProfiles(r.Context())
	if err != nil {
		h.featureError(w, err)
		return
	}
	httpserver.RespondJSON(w, http.StatusOK, items)
}

func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	item, err := h.features.GetSettings(r.Context())
	if err != nil {
		h.featureError(w, err)
		return
	}
	httpserver.RespondJSON(w, http.StatusOK, item)
}

func (h *Handler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var s Settings
	if err := httpserver.DecodeJSON(r, &s); err != nil {
		httpserver.RespondError(w, httpserver.Problem{Status: http.StatusBadRequest, Title: "Invalid Settings", Detail: err.Error()})
		return
	}
	item, err := h.features.UpdateSettings(r.Context(), s)
	if err != nil {
		h.featureError(w, err)
		return
	}
	httpserver.RespondJSON(w, http.StatusOK, item)
}

func (h *Handler) ListBrowserHistory(w http.ResponseWriter, r *http.Request) {
	items, next, err := h.features.ListBrowserHistory(r.Context(), parseLimit(r), r.URL.Query().Get("cursor"))
	if err != nil {
		h.featureError(w, err)
		return
	}
	setNextCursor(w, next)
	httpserver.RespondJSON(w, http.StatusOK, items)
}

func (h *Handler) ListFilterPresets(w http.ResponseWriter, r *http.Request) {
	items, err := h.features.ListFilterPresets(r.Context())
	if err != nil {
		h.featureError(w, err)
		return
	}
	httpserver.RespondJSON(w, http.StatusOK, items)
}

func (h *Handler) CreateFilterPreset(w http.ResponseWriter, r *http.Request) {
	var input FilterPresetInput
	if err := httpserver.DecodeJSON(r, &input); err != nil || strings.TrimSpace(input.Name) == "" {
		httpserver.RespondError(w, httpserver.Problem{Status: http.StatusBadRequest, Title: "Invalid Filter Preset", Detail: "name is required"})
		return
	}
	item, err := h.features.CreateFilterPreset(r.Context(), strings.TrimSpace(input.Name), input.Filters)
	if err != nil {
		h.featureError(w, err)
		return
	}
	httpserver.RespondJSON(w, http.StatusCreated, item)
}
