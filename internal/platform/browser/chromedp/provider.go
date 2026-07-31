package chromedp

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/google/uuid"
	"github.com/joaojsr/shiori-server/internal/platform/browser"
)

// Provider implements browser.Provider using local chromedp.
type Provider struct {
	baseDir  string
	mu       sync.Mutex
	sessions map[string]*session
}

type session struct {
	ctx        context.Context
	cancel     context.CancelFunc
	profileDir string
	status     int
}

type renderedPage struct {
	HTML                string   `json:"-"`
	FinalURL            string   `json:"-"`
	Assets              []string `json:"-"`
	Title               string   `json:"title"`
	BodyText            string   `json:"bodyText"`
	HasVisibleChallenge bool     `json:"hasVisibleChallenge"`
}

const automaticChallengeWait = 10 * time.Second

// New creates the provider.
// baseDir is used for isolated browser profiles.
func New(baseDir string) *Provider {
	return &Provider{
		baseDir:  baseDir,
		sessions: make(map[string]*session),
	}
}

func (p *Provider) IsAvailable() bool {
	// We'll trust chromedp's internal LookPath during Navigate.
	// Returning true to allow attempts.
	return true
}

func (p *Provider) Navigate(ctx context.Context, req browser.NavigateRequest) (*browser.NavigateResult, error) {
	if !p.IsAvailable() {
		return nil, errors.New("chromedp: browser not available")
	}

	// Create isolated profile dir for this session
	sessionID := uuid.New().String()
	profileDir := filepath.Join(p.baseDir, "session_"+sessionID)
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		return nil, fmt.Errorf("chromedp: create profile directory: %w", err)
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.UserDataDir(profileDir),
		// Run headless by default for now
		chromedp.Flag("headless", true),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
	// Keep the context alive so the browser stays open until CloseSession
	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	cancelSession := func() {
		browserCancel()
		allocCancel()
	}

	// The first chromedp.Run allocates the browser and its initial target.
	// It must use the long-lived session context: cancelling a timeout context
	// on the first Run also shuts down the browser, making Snapshot impossible.
	if err := chromedp.Run(browserCtx); err != nil {
		cancelSession()
		return nil, fmt.Errorf("chromedp: start browser: %w", err)
	}

	var timeoutCtx context.Context
	var cancelTimeout context.CancelFunc
	if req.Timeout > 0 {
		timeoutCtx, cancelTimeout = context.WithTimeout(browserCtx, time.Duration(req.Timeout)*time.Millisecond)
	} else {
		timeoutCtx, cancelTimeout = context.WithTimeout(browserCtx, 30*time.Second)
	}
	defer cancelTimeout()

	targetURL := req.URL
	if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
		targetURL = "https://" + targetURL
	}

	actions := []chromedp.Action{
		chromedp.Navigate(targetURL),
		chromedp.WaitReady("body", chromedp.ByQuery),
	}
	if req.WaitFor != "" {
		actions = append(actions, chromedp.WaitReady(req.WaitFor, chromedp.ByQuery))
	}

	if req.AutoScroll {
		scrollJS := `
			new Promise((resolve) => {
				let lastHeight = 0;
				let retries = 0;
				let maxRetries = 3;
				const scrollInterval = setInterval(() => {
					window.scrollTo(0, document.body.scrollHeight);
					let newHeight = document.body.scrollHeight;
					if (newHeight === lastHeight) {
						retries++;
						if (retries >= maxRetries) {
							clearInterval(scrollInterval);
							resolve();
						}
					} else {
						lastHeight = newHeight;
						retries = 0;
					}
				}, 1000);
			});
		`
		actions = append(actions, chromedp.Evaluate(scrollJS, nil, func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
			return p.WithAwaitPromise(true)
		}))
	}

	if req.ClickSelector != "" {
		clickJS := fmt.Sprintf(`
			new Promise((resolve) => {
				const sel = %q;
				let btn = document.querySelector(sel);
				if (!btn) return resolve();
				
				let clicks = 0;
				let maxClicks = 30; // Sanity limit
				
				const observer = new MutationObserver(() => {
					// Disconnect temporarily so our own click doesn't trigger it if it modifies DOM sync
					observer.disconnect();
					setTimeout(() => tryClick(), 800);
				});
				
				function tryClick() {
					btn = document.querySelector(sel);
					if (!btn || btn.disabled || clicks >= maxClicks || btn.offsetParent === null) {
						resolve();
						return;
					}
					clicks++;
					observer.observe(document.body, { childList: true, subtree: true });
					btn.click();
				}
				
				tryClick();
			});
		`, req.ClickSelector)
		actions = append(actions, chromedp.Evaluate(clickJS, nil, func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
			return p.WithAwaitPromise(true)
		}))
	}

	var finalURL string
	actions = append(actions, chromedp.Location(&finalURL))

	if err := chromedp.Run(timeoutCtx, actions...); err != nil {
		cancelSession()
		return nil, fmt.Errorf("chromedp: navigate failed: %w", err)
	}

	p.mu.Lock()
	p.sessions[sessionID] = &session{
		ctx:        browserCtx,
		cancel:     cancelSession,
		profileDir: profileDir,
		status:     200,
	}
	p.mu.Unlock()

	// Simplification for MVP: We don't have true HTTP status code natively in chromedp without network interception.
	// Return 200 until response interception is implemented.
	return &browser.NavigateResult{
		SessionID: sessionID,
		FinalURL:  finalURL,
		Status:    200,
	}, nil
}

func (p *Provider) Snapshot(ctx context.Context, sessionID string) (*browser.PageSnapshot, error) {
	p.mu.Lock()
	activeSession, exists := p.sessions[sessionID]
	p.mu.Unlock()

	if !exists {
		return nil, errors.New("chromedp: session not found")
	}

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("chromedp: snapshot cancelled: %w", ctx.Err())
	default:
	}

	snapshotCtx, cancelSnapshot := context.WithCancel(activeSession.ctx)
	stopCancellation := context.AfterFunc(ctx, cancelSnapshot)
	defer func() {
		stopCancellation()
		cancelSnapshot()
	}()

	page, err := captureRenderedPage(snapshotCtx)
	if err != nil {
		return nil, fmt.Errorf("chromedp: capture snapshot: %w", err)
	}
	if requiresUserAction(page) {
		page, err = waitForAutomaticChallenge(snapshotCtx, page)
		if err != nil {
			return nil, fmt.Errorf("chromedp: wait for automatic challenge: %w", err)
		}
	}
	if strings.TrimSpace(page.HTML) == "" {
		return nil, errors.New("chromedp: captured an empty document")
	}

	return &browser.PageSnapshot{
		HTML:       page.HTML,
		FinalURL:   page.FinalURL,
		Status:     activeSession.status,
		Headers:    make(map[string]string),
		Assets:     page.Assets,
		UserAction: requiresUserAction(page),
	}, nil
}

func captureRenderedPage(ctx context.Context) (renderedPage, error) {
	var page renderedPage
	err := chromedp.Run(ctx,
		chromedp.OuterHTML("html", &page.HTML, chromedp.ByQuery),
		chromedp.Location(&page.FinalURL),
		chromedp.Evaluate(`Array.from(new Set(
			Array.from(document.querySelectorAll("img[src],source[src],video[src],audio[src]"))
				.map((element) => element.currentSrc || element.src)
				.filter(Boolean)
		))`, &page.Assets),
		chromedp.Evaluate(`(() => {
			const isVisible = (element) => {
				const style = window.getComputedStyle(element);
				const rect = element.getBoundingClientRect();
				return style.display !== "none"
					&& style.visibility !== "hidden"
					&& Number(style.opacity || "1") > 0
					&& rect.width > 0
					&& rect.height > 0;
			};
			const selectors = [
				"#challenge-form",
				".cf-turnstile",
				"iframe[src*='challenges.cloudflare.com']",
				"form[action*='/cdn-cgi/challenge-platform/']"
			];
			return {
				title: document.title || "",
				bodyText: (document.body && document.body.innerText || "").slice(0, 8192),
				hasVisibleChallenge: selectors.some((selector) =>
					Array.from(document.querySelectorAll(selector)).some(isVisible)
				)
			};
		})()`, &page),
	)
	return page, err
}

func waitForAutomaticChallenge(ctx context.Context, current renderedPage) (renderedPage, error) {
	timer := time.NewTimer(automaticChallengeWait)
	defer timer.Stop()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for requiresUserAction(current) {
		select {
		case <-ctx.Done():
			return renderedPage{}, ctx.Err()
		case <-timer.C:
			return current, nil
		case <-ticker.C:
			next, err := captureRenderedPage(ctx)
			if err != nil {
				return renderedPage{}, err
			}
			current = next
		}
	}
	return current, nil
}

func requiresUserAction(page renderedPage) bool {
	if page.HasVisibleChallenge {
		return true
	}

	title := strings.ToLower(strings.TrimSpace(page.Title))
	bodyText := strings.ToLower(page.BodyText)
	titleMarkers := []string{
		"just a moment...",
		"attention required! | cloudflare",
	}
	for _, marker := range titleMarkers {
		if strings.Contains(title, marker) {
			return true
		}
	}

	bodyMarkers := []string{
		"performing security verification",
		"verify you are human",
		"checking your browser",
		"enable javascript and cookies to continue",
	}
	for _, marker := range bodyMarkers {
		if strings.Contains(bodyText, marker) {
			return true
		}
	}

	return false
}

func (p *Provider) CloseSession(ctx context.Context, sessionID string) error {
	p.mu.Lock()
	activeSession, exists := p.sessions[sessionID]
	if exists {
		delete(p.sessions, sessionID)
	}
	p.mu.Unlock()

	if !exists {
		return nil
	}

	activeSession.cancel()
	if err := os.RemoveAll(activeSession.profileDir); err != nil {
		return fmt.Errorf("chromedp: remove profile directory: %w", err)
	}
	return nil
}

func (p *Provider) Screencast(ctx context.Context, sessionID string, frames chan<- []byte, in <-chan browser.InputEvent) error {
	p.mu.Lock()
	activeSession, exists := p.sessions[sessionID]
	p.mu.Unlock()

	if !exists {
		return errors.New("chromedp: session not found")
	}

	// Wait for context cancellation or session end
	defer func() {
		// Stop screencast if we are leaving
		_ = chromedp.Run(activeSession.ctx, page.StopScreencast())
	}()

	// Register event listener for screencast frames
	chromedp.ListenTarget(activeSession.ctx, func(ev interface{}) {
		if ev, ok := ev.(*page.EventScreencastFrame); ok {
			// Acknowledge frame to receive the next one
			go func() {
				_ = chromedp.Run(activeSession.ctx, page.ScreencastFrameAck(ev.SessionID))
			}()

			// Decode base64 frame
			data, err := base64.StdEncoding.DecodeString(ev.Data)
			if err == nil {
				select {
				case <-ctx.Done():
				case frames <- data:
				}
			}
		}
	})

	// Start screencast
	err := chromedp.Run(activeSession.ctx,
		page.StartScreencast().WithFormat(page.ScreencastFormatJpeg).WithQuality(80).WithEveryNthFrame(1),
	)
	if err != nil {
		return fmt.Errorf("chromedp: start screencast failed: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-activeSession.ctx.Done():
			return activeSession.ctx.Err()
		case ev, ok := <-in:
			if !ok {
				return nil
			}
			var act chromedp.Action
			switch ev.Type {
			case "mouseMoved":
				act = input.DispatchMouseEvent(input.MouseMoved, float64(ev.X), float64(ev.Y))
			case "mousePressed":
				act = input.DispatchMouseEvent(input.MousePressed, float64(ev.X), float64(ev.Y)).WithButton(input.MouseButton(ev.Button)).WithClickCount(1)
			case "mouseReleased":
				act = input.DispatchMouseEvent(input.MouseReleased, float64(ev.X), float64(ev.Y)).WithButton(input.MouseButton(ev.Button)).WithClickCount(1)
			}
			if act != nil {
				_ = chromedp.Run(activeSession.ctx, act)
			}
		}
	}
}
