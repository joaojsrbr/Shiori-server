package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
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

		var mu sync.Mutex
		sendEvent := func(evt string, data any) {
			b, _ := json.Marshal(data)
			mu.Lock()
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt, string(b))
			flusher.Flush()
			mu.Unlock()
		}

		sendError := func(title, detail string) {
			sendEvent("error", map[string]string{"title": title, "detail": detail})
		}

		ctx := r.Context()

		// Keep alive heartbeat to prevent idle connection drop
		go func() {
			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					mu.Lock()
					fmt.Fprintf(w, ": heartbeat\n\n")
					flusher.Flush()
					mu.Unlock()
				}
			}
		}()

		// 2. Navigate and get HTML
		sendEvent("progress", map[string]string{"step": "navigating", "message": "Loading page in browser..."})
		navReq := browser.NavigateRequest{URL: payload.URL}
		navRes, err := b.Navigate(ctx, navReq)
		if err != nil {
			sendError("Browser Error", "Failed to navigate: "+err.Error())
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

		sendEvent("progress", map[string]string{"step": "snapshot", "message": "Taking DOM snapshot..."})
		snap, err := b.Snapshot(ctx, navRes.SessionID)
		if err != nil {
			sendError("Browser Error", "Failed to get snapshot: "+err.Error())
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

			sendEvent("challenge", map[string]string{
				"message":  "The page requires human interaction (captcha/cloudflare).",
				"instance": fmt.Sprintf("http://localhost:9180/api/v1/challenges/%s", token),
			})
			return
		}

		// 3. Sanitize HTML
		sendEvent("progress", map[string]string{"step": "distilling", "message": "Cleaning HTML..."})
		cleanHTML := distillHTML(snap.HTML)
		slog.Debug("html distilled to markdown", "original_bytes", len(snap.HTML), "distilled_bytes", len(cleanHTML))

		// 4. Extract via AI
		sendEvent("progress", map[string]string{"step": "extracting", "message": "Running AI extraction..."})
		extReq := extraction.Request{
			URL:     payload.URL,
			Content: cleanHTML,
			Target:  payload.Target,
			OnProgress: func(msg string) {
				sendEvent("progress", map[string]string{"step": "extracting", "message": msg})
			},
		}

		res, err := ext.Extract(ctx, extReq)
		if err != nil {
			sendError("Extraction Error", "AI extraction failed: "+err.Error())
			return
		}

		// 5. Try to persist (best effort)
		sendEvent("progress", map[string]string{"step": "saving", "message": "Saving to database..."})
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

		sendEvent("done", resp)
	}
}
