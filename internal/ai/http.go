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
	r.Post("/ai/models/{modelKey}/unload", h.UnloadModel)
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
		loadedMap[m.Key] = len(m.LoadedInstances) > 0
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

func (h *Handler) modelName(modelKey string) (string, bool) {
	switch modelKey {
	case "tiny":
		return h.cfg.ModelTiny, true
	case "default":
		return h.cfg.ModelDefault, true
	case "quality":
		return h.cfg.ModelQuality, true
	default:
		return "", false
	}
}

func (h *Handler) LoadModel(w http.ResponseWriter, r *http.Request) {
	modelKey := httpserver.PathParam(r, "modelKey")

	modelName, ok := h.modelName(modelKey)
	if !ok {
		httpserver.RespondError(w, httpserver.Problem{
			Status: http.StatusBadRequest,
			Title:  "Invalid Model Key",
			Detail: "Available keys are: tiny, default, quality.",
		})
		return
	}

	err := h.client.LoadModel(r.Context(), modelName, h.cfg.MaxContextLength)
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

func (h *Handler) UnloadModel(w http.ResponseWriter, r *http.Request) {
	modelName, ok := h.modelName(httpserver.PathParam(r, "modelKey"))
	if !ok {
		httpserver.RespondError(w, httpserver.Problem{Status: 400, Title: "Invalid Model Key", Detail: "Available keys are: tiny, default, quality."})
		return
	}
	models, err := h.client.ListModels(r.Context())
	if err != nil {
		httpserver.RespondError(w, httpserver.Problem{Status: 502, Title: "LM Studio Unavailable", Detail: err.Error()})
		return
	}
	instanceID := ""
	for _, model := range models {
		if model.Key == modelName && len(model.LoadedInstances) > 0 {
			instanceID = model.LoadedInstances[0].ID
			break
		}
	}
	if instanceID == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := h.client.UnloadModel(r.Context(), instanceID); err != nil {
		httpserver.RespondError(w, httpserver.Problem{Status: 502, Title: "LM Studio Failed to Unload", Detail: err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
