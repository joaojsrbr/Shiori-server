package ai

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/joaojsr/shiori-server/internal/platform/ai/lmstudio"
	"github.com/joaojsr/shiori-server/internal/platform/config"
	"github.com/joaojsr/shiori-server/internal/platform/httpserver"
)

type Handler struct {
	cfg    config.AIConfig
	client *lmstudio.Client
}

func NewHandler(cfg config.AIConfig) *Handler {
	client := lmstudio.NewClient(cfg.LMStudioBaseURL, cfg.Token)
	return &Handler{
		cfg:    cfg,
		client: client,
	}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/ai/models", h.ListModels)
	r.Post("/ai/models/{modelKey}/load", h.LoadModel)
}

type AIModel struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	IsLoaded bool   `json:"is_loaded"`
}

func (h *Handler) ListModels(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	loaded, err := h.client.ListModels(ctx)
	if err != nil {
		httpserver.RespondError(w, httpserver.Problem{
			Status: http.StatusBadGateway,
			Title:  "LM Studio Unavailable",
			Detail: err.Error(),
		})
		return
	}

	loadedMap := make(map[string]bool)
	for _, m := range loaded {
		loadedMap[m.ID] = true
	}

	response := []AIModel{
		{
			Key:      "tiny",
			Name:     h.cfg.ModelTiny,
			IsLoaded: loadedMap[h.cfg.ModelTiny],
		},
		{
			Key:      "default",
			Name:     h.cfg.ModelDefault,
			IsLoaded: loadedMap[h.cfg.ModelDefault],
		},
		{
			Key:      "quality",
			Name:     h.cfg.ModelQuality,
			IsLoaded: loadedMap[h.cfg.ModelQuality],
		},
	}

	httpserver.RespondJSON(w, http.StatusOK, response)
}

func (h *Handler) LoadModel(w http.ResponseWriter, r *http.Request) {
	modelKey := httpserver.PathParam(r, "modelKey")

	var modelName string
	switch modelKey {
	case "tiny":
		modelName = h.cfg.ModelTiny
	case "default":
		modelName = h.cfg.ModelDefault
	case "quality":
		modelName = h.cfg.ModelQuality
	default:
		httpserver.RespondError(w, httpserver.Problem{
			Status: http.StatusBadRequest,
			Title:  "Invalid Model Key",
			Detail: "Available keys are: tiny, default, quality.",
		})
		return
	}

	err := h.client.LoadModel(r.Context(), modelName)
	if err != nil {
		httpserver.RespondError(w, httpserver.Problem{
			Status: http.StatusBadGateway,
			Title:  "LM Studio Failed to Load",
			Detail: err.Error(),
		})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
