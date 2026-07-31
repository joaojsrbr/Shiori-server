package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"golang.org/x/net/html"

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

// sanitizeHTML performs a deep cleanup to save LLM tokens by stripping useless tags and attributes.
func sanitizeHTML(htmlStr string) string {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return htmlStr // fallback to original if parsing fails
	}

	removeTags := map[string]bool{
		"script": true, "style": true, "noscript": true, "svg": true, "path": true,
		"iframe": true, "nav": true, "footer": true, "header": true,
		"link": true, "meta": true, "button": true,
		"form": true, "input": true, "select": true, "option": true, "textarea": true,
		"dialog": true, "canvas": true, "map": true, "area": true,
	}

	var fNode func(*html.Node)
	fNode = func(n *html.Node) {
		for c := n.LastChild; c != nil; {
			next := c.PrevSibling
			if c.Type == html.ElementNode {
				if removeTags[c.Data] {
					n.RemoveChild(c)
				} else {
					var keep []html.Attribute
					for _, a := range c.Attr {
						key := strings.ToLower(a.Key)
						if key == "href" || key == "src" || key == "alt" || key == "title" || key == "id" || key == "class" || strings.HasPrefix(key, "data-") {
							keep = append(keep, a)
						}
					}
					c.Attr = keep
					fNode(c)
				}
			} else if c.Type == html.CommentNode {
				n.RemoveChild(c)
			} else {
				fNode(c)
			}
			c = next
		}
	}
	fNode(doc)

	var buf strings.Builder
	html.Render(&buf, doc)
	clean := buf.String()

	// collapse whitespace
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

		// 5. Persist to DB (assuming TargetManga for now)
		if payload.Target == extraction.TargetManga {
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
