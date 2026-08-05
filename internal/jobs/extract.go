package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"unicode/utf8"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"golang.org/x/net/html"

	"github.com/joaojsr/shiori-server/internal/extraction"
	"github.com/joaojsr/shiori-server/internal/library"
	"github.com/joaojsr/shiori-server/internal/platform/browser"
	"github.com/joaojsr/shiori-server/internal/platform/events"
	"github.com/joaojsr/shiori-server/internal/platform/queue"
	"github.com/joaojsr/shiori-server/internal/platform/storage"
	"github.com/joaojsr/shiori-server/internal/worker"
)

// ExtractPayload defines the JSON payload expected in the job.
type ExtractPayload struct {
	URL           string                `json:"url"`
	Target        extraction.TargetType `json:"target"`
	RequiresLogin bool                  `json:"requires_login,omitempty"`
	LoginURL      string                `json:"login_url,omitempty"`
	AutoScroll    bool                  `json:"auto_scroll,omitempty"`
	ClickSelector string                `json:"click_selector,omitempty"`
}

// Validate rejects incomplete and inconsistent extraction instructions before
// they enter the durable queue. Credentials remain inside Chromium.
func (p ExtractPayload) Validate() error {
	if err := validateHTTPURL(p.URL); err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	if p.Target == "" {
		return errors.New("target is required")
	}
	if p.RequiresLogin {
		if err := validateHTTPURL(p.LoginURL); err != nil {
			return fmt.Errorf("requires_login needs a valid login_url: %w", err)
		}
	} else if strings.TrimSpace(p.LoginURL) != "" {
		return errors.New("login_url requires requires_login=true")
	}
	return nil
}

func validateHTTPURL(raw string) error {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(raw))
	if err != nil {
		return err
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return errors.New("must be an absolute http or https URL")
	}
	return nil
}

func sameLoginLocation(loginURL, finalURL string) bool {
	expected, expectedErr := url.Parse(strings.TrimSpace(loginURL))
	actual, actualErr := url.Parse(strings.TrimSpace(finalURL))
	if expectedErr != nil || actualErr != nil || actual.Hostname() == "" {
		return false
	}
	normalizePath := func(value string) string {
		value = strings.TrimSuffix(value, "/")
		if value == "" {
			return "/"
		}
		return value
	}
	return strings.EqualFold(expected.Scheme, actual.Scheme) &&
		strings.EqualFold(expected.Host, actual.Host) &&
		normalizePath(expected.Path) == normalizePath(actual.Path)
}

// stripNoisyTags performs a first-pass deep cleanup removing elements that
// carry zero informational value for LLM-based extraction (scripts, styles,
// ads, navigation chrome, etc).
func stripNoisyTags(htmlStr, baseURL string) (string, string) {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return htmlStr, ""
	}
	metadata := extractDocumentMetadata(doc)
	base, _ := url.Parse(baseURL)

	removeTags := map[string]bool{
		// "form": true, "input": true, "select": true, "option": true, "textarea": true,
		// "dialog": true, "canvas": true, "map": true, "area": true, "aside": true,
		// "script": true,
		"style": true, "noscript": true, "nav": true,
		// "svg": true, "path": true,
		// "iframe": true, "footer": true, "header": true,
		// "link": true, "meta": true, "button": true,
		// "picture": true, "source": true,
	}

	// Only keep attributes that carry real data for scraping.
	keepAttr := map[string]bool{
		"href": true, "src": true, "alt": true, "title": true,
	}

	var fNode func(*html.Node)
	fNode = func(n *html.Node) {
		for c := n.LastChild; c != nil; {
			next := c.PrevSibling
			switch c.Type {
			case html.ElementNode:
				if removeTags[c.Data] {
					n.RemoveChild(c)
				} else {
					var keep []html.Attribute
					for _, a := range c.Attr {
						if keepAttr[strings.ToLower(a.Key)] {
							if strings.EqualFold(a.Key, "src") && strings.HasPrefix(strings.ToLower(strings.TrimSpace(a.Val)), "data:") {
								// Embedded chapter images are harvested before AI extraction.
								// Never send their base64 payload to the model context.
								a.Val = "embedded-image"
							}
							if base != nil && (a.Key == "href" || a.Key == "src") {
								if ref, err := url.Parse(strings.TrimSpace(a.Val)); err == nil && !ref.IsAbs() && ref.Scheme == "" {
									a.Val = base.ResolveReference(ref).String()
								}
							}
							keep = append(keep, a)
						}
					}
					c.Attr = keep
					fNode(c)
				}
			case html.CommentNode:
				n.RemoveChild(c)
			default:
				fNode(c)
			}
			c = next
		}
	}
	fNode(doc)

	var buf strings.Builder
	html.Render(&buf, doc)
	return buf.String(), metadata
}

const maxMetadataBytes = 8192

// extractDocumentMetadata retains compact, high-signal data that would
// otherwise disappear with <head>, <meta>, and <script> cleanup.
func extractDocumentMetadata(doc *html.Node) string {
	var entries []string
	allowedMeta := map[string]bool{
		"description": true, "og:title": true, "og:type": true,
		"og:url": true, "twitter:title": true,
	}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "title":
				if value := strings.TrimSpace(nodeText(n)); value != "" {
					entries = append(entries, "title: "+value)
				}
			case "meta":
				var key, value string
				for _, attr := range n.Attr {
					switch strings.ToLower(attr.Key) {
					case "name", "property":
						key = strings.ToLower(strings.TrimSpace(attr.Val))
					case "content":
						value = strings.TrimSpace(attr.Val)
					}
				}
				if allowedMeta[key] && value != "" {
					entries = append(entries, key+": "+value)
				}
			case "script":
				for _, attr := range n.Attr {
					if strings.EqualFold(attr.Key, "type") && strings.EqualFold(strings.TrimSpace(attr.Val), "application/ld+json") {
						if value := strings.TrimSpace(nodeText(n)); value != "" {
							entries = append(entries, "json-ld: "+value)
						}
					}
				}
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	result := strings.Join(entries, "\n")
	if len(result) > maxMetadataBytes {
		result = result[:maxMetadataBytes]
		for len(result) > 0 && !utf8.ValidString(result) {
			result = result[:len(result)-1]
		}
	}
	return result
}

func nodeText(n *html.Node) string {
	var result strings.Builder
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.TextNode {
			if text := strings.TrimSpace(current.Data); text != "" {
				result.WriteString(text)
				result.WriteByte(' ')
			}
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	return strings.Join(strings.Fields(result.String()), " ")
}

// htmlToMarkdown converts cleaned HTML to Markdown.
// Markdown is the native language of LLMs: significantly more compact than HTML
// and strips all remaining tag verbosity, achieving 65-90% token reduction.

// distillHTML is the two-stage pipeline:
// 1. Strip all noisy/decorative HTML elements (script, style, nav, etc.)
// 2. Convert the remaining semantic HTML to clean Markdown
// This consistently achieves 65-90% token reduction vs raw HTML while
// preserving every piece of scrapeable content (titles, links, chapter lists, etc).
func distillHTML(htmlStr, baseURL string) string {
	stripped, metadata := stripNoisyTags(htmlStr, baseURL)
	md, err := htmltomarkdown.ConvertString(stripped)
	if err != nil {
		// If conversion fails, fall back to the stripped HTML with collapsed whitespace
		return strings.Join(strings.Fields(stripped), " ")
	}
	// Collapse excessive blank lines produced by the converter
	lines := strings.Split(md, "\n")
	var result []string
	blankCount := 0
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			blankCount++
			if blankCount <= 1 {
				result = append(result, line)
			}
		} else {
			blankCount = 0
			result = append(result, line)
		}
	}
	body := strings.Join(result, "\n")
	if metadata != "" {
		return "# Document metadata\n\n" + metadata + "\n\n# Document content\n\n" + body
	}
	return body
}

// ExtractResponse is the response format for extraction progress and completion.
type ExtractResponse struct {
	URL        string                `json:"url"`
	Target     extraction.TargetType `json:"target"`
	AIOutput   json.RawMessage       `json:"ai_output"`
	Confidence float64               `json:"confidence"`
	Method     string                `json:"method"`
	Warnings   []string              `json:"warnings,omitempty"`
	Saved      bool                  `json:"saved"`
	MediaID    string                `json:"media_id,omitempty"`
}

// NewExtractHandler returns a worker.Handler that executes the extraction pipeline.
func NewExtractHandler(
	b browser.Provider,
	ext extraction.Provider,
	repo library.MediaRepository,
	cm *browser.ChallengeManager,
	hub *events.Hub,
	dependencies ...any,
) worker.Handler {
	var files storage.Provider
	for _, dependency := range dependencies {
		if provider, ok := dependency.(storage.Provider); ok {
			files = provider
		}
	}
	chapterRepo, _ := repo.(library.ChapterRepository)
	return func(ctx context.Context, job *queue.Job) error {
		topic := "job:" + job.ID

		sendEvent := func(evt string, data any) {
			if hub != nil {
				hub.Publish(topic, map[string]any{"event": evt, "data": data})
			}
		}

		sendError := func(title, detail string, err error) error {
			if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
				sendEvent("cancelled", map[string]string{"job_id": job.ID, "status": string(queue.StatusCancelled)})
				return context.Canceled
			}
			sendEvent("error", map[string]string{"title": title, "detail": detail})
			return fmt.Errorf("%s: %w", title, err)
		}

		// 1. Decode Payload
		var payload ExtractPayload
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			return sendError("Invalid Payload", "Failed to parse job payload", err)
		}

		if err := payload.Validate(); err != nil {
			return sendError("Invalid Request", err.Error(), err)
		}

		var finalResultJSON map[string]interface{}
		currentURL := payload.URL
		loginPhase := false
		loginConfirmed := false
		if payload.RequiresLogin {
			currentURL = payload.LoginURL
			loginPhase = true
		}
		pageCount := 1
		visitedURLs := make(map[string]bool)
		visitedURLs[currentURL] = true
		var lastRes *extraction.Result
		var loopErr error

		for {
			// 2. Navigate and get HTML
			loadMessage := fmt.Sprintf("Fetching page %d with FlareSolverr...", pageCount)
			if payload.RequiresLogin {
				loadMessage = fmt.Sprintf("Loading page %d in the login browser...", pageCount)
			}
			sendEvent("progress", map[string]string{"step": "navigating", "message": loadMessage})
			navReq := browser.NavigateRequest{
				URL:           currentURL,
				ProfileURL:    payload.URL,
				RequiresLogin: payload.RequiresLogin,
				AutoScroll:    payload.AutoScroll,
				ClickSelector: payload.ClickSelector,
			}

			navRes, err := b.Navigate(ctx, navReq)
			if err != nil {
				return sendError("Browser Error", "Failed to navigate: "+err.Error(), err)
			}

			snapshotMessage := "Using the HTML returned by FlareSolverr..."
			if payload.RequiresLogin {
				snapshotMessage = "Taking DOM snapshot from the login browser..."
			}
			sendEvent("progress", map[string]string{"step": "snapshot", "message": snapshotMessage})
			snap, err := b.Snapshot(ctx, navRes.SessionID)
			if err != nil {
				b.CloseSession(context.Background(), navRes.SessionID)
				return sendError("Browser Error", "Failed to get snapshot: "+err.Error(), err)
			}

			// An explicit login URL covers custom authentication pages that do
			// not expose conventional input names or copy. Redirecting away from
			// it without a visible login means the saved session is already valid.
			if loginPhase && !loginConfirmed && !snap.UserAction && sameLoginLocation(payload.LoginURL, snap.FinalURL) {
				snap.UserAction = true
				snap.UserActionKind = browser.UserActionLogin
			}

			if snap.UserAction {
				if snap.UserActionKind == browser.UserActionBlocked {
					b.CloseSession(context.Background(), navRes.SessionID)
					return sendError(
						"Source Blocked",
						"The source returned a definitive Cloudflare block instead of an interactive challenge.",
						fmt.Errorf("source blocked by Cloudflare"),
					)
				}
				actionKind := snap.UserActionKind
				if actionKind == browser.UserActionNone {
					actionKind = browser.UserActionChallenge
				}
				token := cm.Create(navRes.SessionID, actionKind)
				challengeURL := "/api/v1/challenges/" + token
				slog.Info("user action required, waiting for browser handoff", "job_id", job.ID, "kind", actionKind)

				message := "The page requires human verification (captcha/cloudflare)."
				if actionKind == browser.UserActionLogin {
					message = "The source requires login. Authenticate in the remote browser to continue."
				}
				sendEvent("challenge", map[string]string{
					"kind":          string(actionKind),
					"message":       message,
					"challenge_url": challengeURL,
					"instance":      challengeURL,
				})

				waitErr := cm.Wait(ctx, token)
				cm.Remove(token)
				if waitErr != nil {
					b.CloseSession(context.Background(), navRes.SessionID)
					return sendError("Challenge Failed", "Challenge resolution failed or timed out", waitErr)
				}
				if actionKind == browser.UserActionLogin {
					loginConfirmed = true
				}

				// Close gracefully so Chromium persists cookies, then reopen the
				// exact requested URL with the same domain profile. This prevents
				// extracting an account/landing page after login.
				if err := b.CloseSession(context.Background(), navRes.SessionID); err != nil {
					return sendError("Browser Error", "Failed to persist browser session: "+err.Error(), err)
				}
				sendEvent("progress", map[string]string{"step": "resuming", "message": "Reopening the requested page with the verified session..."})
				navRes, err = b.Navigate(ctx, navReq)
				if err != nil {
					return sendError("Browser Error", "Failed to resume navigation: "+err.Error(), err)
				}
				snap, err = b.Snapshot(ctx, navRes.SessionID)
				if err != nil {
					b.CloseSession(context.Background(), navRes.SessionID)
					return sendError("Browser Error", "Failed to snapshot the resumed page: "+err.Error(), err)
				}
				if snap.UserAction {
					b.CloseSession(context.Background(), navRes.SessionID)
					return sendError("Human Action Failed", "The requested page still requires human action after the session was resumed.", fmt.Errorf("user action still required: %s", snap.UserActionKind))
				}
			}

			// Close session after snapshot
			b.CloseSession(context.Background(), navRes.SessionID)
			if loginPhase {
				loginPhase = false
				currentURL = payload.URL
				visitedURLs[currentURL] = true
				sendEvent("progress", map[string]string{"step": "authenticated", "message": "Login session saved. Opening the requested page..."})
				continue
			}

			// 3. Sanitize HTML
			sendEvent("progress", map[string]string{"step": "distilling", "message": "Cleaning HTML..."})
			baseURL := snap.FinalURL
			if baseURL == "" {
				baseURL = currentURL
			}
			cleanHTML := distillHTML(snap.HTML, baseURL)
			verifiedUnits := discoverUnitURLs(snap.HTML, baseURL)
			cleanHTML = appendDiscoveredUnitURLs(cleanHTML, verifiedUnits, payload.Target)
			if len(verifiedUnits) > 0 {
				sendEvent("progress", map[string]any{
					"step": "resolving_urls", "message": fmt.Sprintf("Recovered %d verified chapter/episode URLs from the page.", len(verifiedUnits)),
					"resolved_urls": len(verifiedUnits),
				})
			} else {
				sendEvent("progress", map[string]any{
					"step": "resolving_urls", "message": "No verified chapter/episode URL pattern was found in this page.",
					"resolved_urls": 0,
				})
			}
			slog.Debug("html distilled to markdown", "original_bytes", len(snap.HTML), "distilled_bytes", len(cleanHTML))

			// 4. Extract structured data via AI
			sendEvent("progress", map[string]string{"step": "extracting", "message": fmt.Sprintf("Running AI extraction for page %d...", pageCount)})
			extReq := extraction.Request{
				URL:     currentURL,
				Content: cleanHTML,
				Target:  payload.Target,
				OnProgress: func(msg string) {
					sendEvent("progress", map[string]string{"step": "extracting", "message": msg})
				},
				OnInferenceProgress: func(progress extraction.InferenceProgress) {
					message := fmt.Sprintf("LM Studio: waiting for the first token (chunk %d of %d)...", progress.Chunk, progress.Chunks)
					if progress.Phase == "generating" || progress.Phase == "completed" {
						qualifier := "tokens"
						if progress.Estimated {
							qualifier = "estimated tokens"
						}
						message = fmt.Sprintf("LM Studio: %d %s generated at %.1f tok/s", progress.GeneratedTokens, qualifier, progress.TokensPerSecond)
						if progress.Phase == "completed" {
							message = "LM Studio generation completed: " + strings.TrimPrefix(message, "LM Studio: ")
						}
					}
					sendEvent("progress", map[string]any{
						"step": "ai_inference", "phase": progress.Phase, "message": message,
						"chunk": progress.Chunk, "chunks": progress.Chunks,
						"generated_tokens": progress.GeneratedTokens, "token_limit": progress.TokenLimit,
						"tokens_per_second": progress.TokensPerSecond, "elapsed_seconds": progress.Elapsed.Seconds(),
						"estimated": progress.Estimated,
					})
				},
			}

			lastRes, loopErr = ext.Extract(ctx, extReq)
			if loopErr != nil {
				return sendError("Extraction Error", "AI extraction failed: "+loopErr.Error(), loopErr)
			}

			var parsed map[string]interface{}
			if err := json.Unmarshal(lastRes.RawJSON, &parsed); err != nil {
				return sendError("Extraction Error", "Invalid JSON from AI", err)
			}
			enrichExtractedUnitURLs(parsed, snap.HTML, baseURL, payload.Target)

			if finalResultJSON == nil {
				finalResultJSON = parsed
			} else {
				// Merge arrays
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

			// Check pagination
			nextURL, ok := parsed["next_page_url"].(string)
			if !ok || nextURL == "" || nextURL == currentURL {
				break
			}
			if extractedUnitContainsURL(parsed, payload.Target, nextURL, currentURL) {
				break
			}

			// Resolve relative URLs if needed
			if !strings.HasPrefix(nextURL, "http") {
				if strings.HasPrefix(nextURL, "/") {
					// Extremely basic relative resolution for simplicity
					parts := strings.Split(currentURL, "/")
					if len(parts) >= 3 {
						nextURL = parts[0] + "//" + parts[2] + nextURL
					}
				} else {
					break // Unhandled relative format, stop to avoid infinite loops
				}
			}

			if visitedURLs[nextURL] {
				break
			}
			visitedURLs[nextURL] = true

			currentURL = nextURL
			pageCount++
		}

		// Deduplicate arrays in finalResultJSON
		for k, v := range finalResultJSON {
			if list, ok := v.([]interface{}); ok {
				var deduped []interface{}
				seen := make(map[string]bool)
				for _, item := range list {
					key := ""
					if m, isMap := item.(map[string]interface{}); isMap {
						if u, hasURL := m["url"].(string); hasURL && u != "" {
							key = u
						} else {
							b, _ := json.Marshal(item)
							key = string(b)
						}
					} else {
						b, _ := json.Marshal(item)
						key = string(b)
					}

					if !seen[key] {
						seen[key] = true
						deduped = append(deduped, item)
					}
				}
				finalResultJSON[k] = deduped
			}
		}

		finalBytes, _ := json.MarshalIndent(finalResultJSON, "", "  ")
		finalRaw := json.RawMessage(finalBytes)

		// 5. Persist to DB
		sendEvent("progress", map[string]string{"step": "saving", "message": "Saving to database..."})
		resp := ExtractResponse{
			URL:        payload.URL,
			Target:     payload.Target,
			AIOutput:   finalRaw,
			Confidence: lastRes.Confidence,
			Method:     lastRes.Method,
			Warnings:   lastRes.Warnings,
		}

		if payload.Target == extraction.TargetManga || payload.Target == extraction.TargetAnime {
			var createReq library.MediaCreateRequest
			if err := json.Unmarshal(finalRaw, &createReq); err != nil {
				resp.Warnings = append(resp.Warnings, "failed to unmarshal extracted JSON: "+err.Error())
				loopErr = fmt.Errorf("unmarshal error: %w", err) // Keep the error for logging but allow done event
			} else {
				createReq.SourceURL = payload.URL
				if createReq.Type == "" {
					if payload.Target == extraction.TargetAnime {
						createReq.Type = library.MediaTypeAnime
					} else {
						createReq.Type = library.MediaTypeManga
					}
				}
				media, dbErr := repo.Create(ctx, createReq)
				if dbErr != nil {
					resp.Warnings = append(resp.Warnings, "failed to save to database: "+dbErr.Error())
					loopErr = fmt.Errorf("db error: %w", dbErr)
				} else {
					resp.Saved = true
					resp.MediaID = media.ID
					if chapterRepo != nil && files != nil {
						resp.Warnings = append(resp.Warnings, importExtractedUnits(ctx, b, files, chapterRepo, media, payload, finalResultJSON, sendEvent)...)
					}
					slog.Info("successfully extracted and saved media", "title", createReq.Title)
					loopErr = nil
				}
			}
		} else {
			resp.Warnings = append(resp.Warnings, "unsupported target for persistence: "+string(payload.Target))
			loopErr = fmt.Errorf("unsupported target")
		}

		sendEvent("done", resp)
		return loopErr
	}
}
