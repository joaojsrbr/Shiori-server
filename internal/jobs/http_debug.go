package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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
		var payload struct {
			URL           string                `json:"url"`
			Target        extraction.TargetType `json:"target"`
			AutoScroll    bool                  `json:"auto_scroll,omitempty"`
			ClickSelector string                `json:"click_selector,omitempty"`
		}
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

		var finalResultJSON map[string]interface{}
		currentURL := payload.URL
		pageCount := 1
		var lastRes *extraction.Result
		var loopErr error

		for {
			sendEvent("progress", map[string]string{"step": "navigating", "message": fmt.Sprintf("Loading page %d in browser...", pageCount)})
			navReq := browser.NavigateRequest{
				URL:           currentURL,
				AutoScroll:    payload.AutoScroll,
				ClickSelector: payload.ClickSelector,
			}

			navRes, err := b.Navigate(ctx, navReq)
			if err != nil {
				sendError("Browser Error", "Failed to navigate: "+err.Error())
				return
			}

			sendEvent("progress", map[string]string{"step": "snapshot", "message": "Taking DOM snapshot..."})
			snap, err := b.Snapshot(ctx, navRes.SessionID)
			if err != nil {
				b.CloseSession(context.Background(), navRes.SessionID)
				sendError("Browser Error", "Failed to get snapshot: "+err.Error())
				return
			}

			if snap.UserAction {
				b.CloseSession(context.Background(), navRes.SessionID)
				token := cm.Create(navRes.SessionID) // This is technically invalid since session is closed, but debug handler is sync and won't wait.
				sendEvent("challenge", map[string]string{
					"message":  "The page requires human interaction (captcha/cloudflare). Debug endpoint cannot wait.",
					"instance": fmt.Sprintf("http://localhost:9180/api/v1/challenges/%s", token),
				})
				return
			}

			b.CloseSession(context.Background(), navRes.SessionID)

			sendEvent("progress", map[string]string{"step": "distilling", "message": "Cleaning HTML..."})
			cleanHTML := distillHTML(snap.HTML)

			sendEvent("progress", map[string]string{"step": "extracting", "message": fmt.Sprintf("Running AI extraction for page %d...", pageCount)})
			extReq := extraction.Request{
				URL:     currentURL,
				Content: cleanHTML,
				Target:  payload.Target,
				OnProgress: func(msg string) {
					sendEvent("progress", map[string]string{"step": "extracting", "message": msg})
				},
			}

			lastRes, loopErr = ext.Extract(ctx, extReq)
			if loopErr != nil {
				sendError("Extraction Error", "AI extraction failed: "+loopErr.Error())
				return
			}

			var parsed map[string]interface{}
			if err := json.Unmarshal(lastRes.RawJSON, &parsed); err != nil {
				sendError("Extraction Error", "Invalid JSON from AI")
				return
			}

			if finalResultJSON == nil {
				finalResultJSON = parsed
			} else {
				for k, v := range parsed {
					if parsedList, ok := v.([]interface{}); ok {
						if baseList, ok := finalResultJSON[k].([]interface{}); ok {
							finalResultJSON[k] = append(baseList, parsedList...)
						} else if finalResultJSON[k] == nil {
							finalResultJSON[k] = parsedList
						}
					}
				}
			}

			nextURL, ok := parsed["next_page_url"].(string)
			if !ok || nextURL == "" || nextURL == currentURL {
				break
			}

			if !strings.HasPrefix(nextURL, "http") {
				if strings.HasPrefix(nextURL, "/") {
					parts := strings.Split(currentURL, "/")
					if len(parts) >= 3 {
						nextURL = parts[0] + "//" + parts[2] + nextURL
					}
				} else {
					break
				}
			}

			currentURL = nextURL
			pageCount++
		}

		finalBytes, _ := json.MarshalIndent(finalResultJSON, "", "  ")
		finalRaw := json.RawMessage(finalBytes)

		// 5. Try to persist (best effort)
		sendEvent("progress", map[string]string{"step": "saving", "message": "Saving to database..."})
		resp := DebugExtractResponse{
			URL:        payload.URL,
			Target:     payload.Target,
			AIOutput:   finalRaw,
			Confidence: lastRes.Confidence,
			Method:     lastRes.Method,
			Warnings:   lastRes.Warnings,
		}

		if payload.Target == extraction.TargetManga {
			var createReq library.MediaCreateRequest
			if err := json.Unmarshal(finalRaw, &createReq); err == nil {
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
