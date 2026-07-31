package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/joaojsr/shiori-server/internal/extraction"
	"github.com/joaojsr/shiori-server/internal/library"
	"github.com/joaojsr/shiori-server/internal/platform/browser"
	"github.com/joaojsr/shiori-server/internal/platform/queue"
	"github.com/joaojsr/shiori-server/internal/worker"
)

// ExtractPayload defines the JSON payload expected in the job.
type ExtractPayload struct {
	URL    string                `json:"url"`
	Target extraction.TargetType `json:"target"`
}

// scriptStyleRegex matches tags that contain no useful text for extraction (scripts, styles, svgs, etc) and HTML comments.
var scriptStyleRegex = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>|<style[^>]*>.*?</style>|<noscript[^>]*>.*?</noscript>|<svg[^>]*>.*?</svg>|<!--.*?-->`)

// sanitizeHTML performs a lightweight cleanup to save LLM tokens.
func sanitizeHTML(html string) string {
	clean := scriptStyleRegex.ReplaceAllString(html, "")
	// Optionally we could remove comments or collapse spaces here
	clean = strings.Join(strings.Fields(clean), " ")
	return clean
}

// NewExtractHandler returns a worker.Handler that executes the extraction pipeline.
func NewExtractHandler(
	b browser.Provider,
	ext extraction.Provider,
	repo library.MediaRepository,
	cm *browser.ChallengeManager,
) worker.Handler {
	return func(ctx context.Context, job *queue.Job) error {
		// 1. Decode Payload
		var payload ExtractPayload
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			return fmt.Errorf("invalid job payload: %w", err)
		}

		if payload.URL == "" || payload.Target == "" {
			return fmt.Errorf("missing url or target in payload")
		}

		// 2. Navigate and get HTML
		navReq := browser.NavigateRequest{
			URL: payload.URL,
		}

		navRes, err := b.Navigate(ctx, navReq)
		if err != nil {
			return fmt.Errorf("browser failed to navigate: %w", err)
		}

		defer b.CloseSession(context.Background(), navRes.SessionID)

		snap, err := b.Snapshot(ctx, navRes.SessionID)
		if err != nil {
			return fmt.Errorf("browser failed to get snapshot: %w", err)
		}

		if snap.UserAction {
			token := cm.Create(navRes.SessionID)
			slog.Info("user action required, waiting for challenge resolution", "token", token, "url", fmt.Sprintf("/api/v1/challenges/%s", token))
			
			// We block the worker here. In a real system we would update the job status in the DB
			// to "requires_user_action" and include the URL, but our queue/job interface doesn't
			// support runtime status updates yet.
			if err := cm.Wait(ctx, token); err != nil {
				return fmt.Errorf("challenge failed or timed out: %w", err)
			}
			
			// Take snapshot again after challenge is solved
			snap, err = b.Snapshot(ctx, navRes.SessionID)
			if err != nil {
				return fmt.Errorf("browser failed to get snapshot after challenge: %w", err)
			}
			if snap.UserAction {
				return fmt.Errorf("user action still required after challenge resolution")
			}
		}

		// 3. Sanitize HTML
		cleanHTML := sanitizeHTML(snap.HTML)

		// 4. Extract structured data via AI
		req := extraction.Request{
			URL:     payload.URL,
			Content: cleanHTML,
			Target:  payload.Target,
		}

		res, err := ext.Extract(ctx, req)
		if err != nil {
			return fmt.Errorf("extraction failed: %w", err)
		}

		// 5. Persist to DB (assuming TargetMedia for now)
		if payload.Target == extraction.TargetMedia {
			var createReq library.MediaCreateRequest
			if err := json.Unmarshal(res.RawJSON, &createReq); err != nil {
				return fmt.Errorf("failed to unmarshal extracted JSON to MediaCreateRequest: %w", err)
			}
			createReq.SourceURL = payload.URL

			// Save to repo
			_, err := repo.Create(ctx, createReq)
			if err != nil {
				return fmt.Errorf("failed to save media to database: %w", err)
			}
			slog.Info("successfully extracted and saved media", "title", createReq.Title, "raw_json", string(res.RawJSON))
		} else {
			return fmt.Errorf("unsupported target for persistence: %s", payload.Target)
		}

		return nil
	}
}
