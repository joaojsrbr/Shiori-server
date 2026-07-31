package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/joaojsr/shiori-server/internal/extraction"
	"github.com/joaojsr/shiori-server/internal/library"
	"github.com/joaojsr/shiori-server/internal/platform/browser"
	"github.com/joaojsr/shiori-server/internal/platform/httpserver"
)

// DebugExtractResponse is the response for the debug extract endpoint.
type DebugExtractResponse struct {
	URL        string                `json:"url"`
	Target     extraction.TargetType `json:"target"`
	AIOutput   json.RawMessage       `json:"ai_output"`
	Confidence float64               `json:"confidence"`
	Method     string                `json:"method"`
	Warnings   []string              `json:"warnings,omitempty"`
	Saved      bool                  `json:"saved"`
	MediaID    string                `json:"media_id,omitempty"`
}

// HandleDebugExtract runs the full extraction pipeline synchronously and returns
// the raw AI output. This endpoint is expensive and should only be registered in
// debug builds/configuration.
func HandleDebugExtract(
	b browser.Provider,
	ext extraction.Provider,
	repo library.MediaRepository,
	cm *browser.ChallengeManager,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload ExtractPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			httpserver.RespondError(w, httpserver.Problem{
				Status: http.StatusBadRequest,
				Title:  "Invalid Payload",
				Detail: "Failed to parse JSON body",
			})
			return
		}

		if payload.URL == "" || payload.Target == "" {
			httpserver.RespondError(w, httpserver.Problem{
				Status: http.StatusBadRequest,
				Title:  "Invalid Request",
				Detail: "URL and Target are required",
			})
			return
		}

		ctx := r.Context()

		// 2. Navigate and get HTML
		navReq := browser.NavigateRequest{URL: payload.URL}
		navRes, err := b.Navigate(ctx, navReq)
		if err != nil {
			httpserver.RespondError(w, httpserver.Problem{
				Status: http.StatusBadGateway,
				Title:  "Browser Error",
				Detail: "Failed to navigate: " + err.Error(),
			})
			return
		}

		sessionClosed := false
		closeSession := func() {
			if !sessionClosed {
				b.CloseSession(context.Background(), navRes.SessionID)
				sessionClosed = true
			}
		}
		defer closeSession()

		snap, err := b.Snapshot(ctx, navRes.SessionID)
		if err != nil {
			httpserver.RespondError(w, httpserver.Problem{
				Status: http.StatusBadGateway,
				Title:  "Browser Error",
				Detail: "Failed to get snapshot: " + err.Error(),
			})
			return
		}

		if snap.UserAction {
			token := cm.Create(navRes.SessionID)

			// We take ownership of closing the session so it stays alive for the challenge
			sessionClosed = true
			go func() {
				defer b.CloseSession(context.Background(), navRes.SessionID)

				// Keep it alive until resolved or timed out (e.g. 5 minutes)
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				defer cancel()
				_ = cm.Wait(ctx, token)
			}()

			httpserver.RespondError(w, httpserver.Problem{
				Status:   http.StatusConflict,
				Title:    "User Action Required",
				Detail:   "The page requires human interaction (captcha/cloudflare).",
				Instance: fmt.Sprintf("http://localhost:8080/api/v1/challenges/%s", token),
			})
			return
		}

		// 3. Sanitize HTML
		cleanHTML := distillHTML(snap.HTML)
		slog.Debug("html distilled to markdown", "original_bytes", len(snap.HTML), "distilled_bytes", len(cleanHTML))

		// 4. Extract via AI
		extReq := extraction.Request{
			URL:     payload.URL,
			Content: cleanHTML,
			Target:  payload.Target,
		}

		res, err := ext.Extract(ctx, extReq)
		if err != nil {
			httpserver.RespondError(w, httpserver.Problem{
				Status: http.StatusInternalServerError,
				Title:  "Extraction Error",
				Detail: "AI extraction failed: " + err.Error(),
			})
			return
		}

		// 5. Try to persist (best effort)
		resp := DebugExtractResponse{
			URL:        payload.URL,
			Target:     payload.Target,
			AIOutput:   res.RawJSON,
			Confidence: res.Confidence,
			Method:     res.Method,
			Warnings:   res.Warnings,
		}

		if payload.Target == extraction.TargetManga {
			var createReq library.MediaCreateRequest
			if err := json.Unmarshal(res.RawJSON, &createReq); err == nil {
				createReq.SourceURL = payload.URL
				media, err := repo.Create(ctx, createReq)
				if err == nil {
					resp.Saved = true
					resp.MediaID = media.ID
				} else {
					resp.Warnings = append(resp.Warnings, "failed to save to database: "+err.Error())
				}
			} else {
				resp.Warnings = append(resp.Warnings, "failed to unmarshal AI output to MediaCreateRequest: "+err.Error())
			}
		}

		httpserver.RespondJSON(w, http.StatusOK, resp)
	}
}
