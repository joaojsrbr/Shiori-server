package chromedp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

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

	actions := []chromedp.Action{
		chromedp.Navigate(req.URL),
		chromedp.WaitReady("body", chromedp.ByQuery),
	}
	if req.WaitFor != "" {
		actions = append(actions, chromedp.WaitReady(req.WaitFor, chromedp.ByQuery))
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

	var (
		html     string
		finalURL string
		assets   []string
	)

	err := chromedp.Run(snapshotCtx,
		chromedp.OuterHTML("html", &html, chromedp.ByQuery),
		chromedp.Location(&finalURL),
		chromedp.Evaluate(`Array.from(new Set(
			Array.from(document.querySelectorAll("img[src],source[src],video[src],audio[src]"))
				.map((element) => element.currentSrc || element.src)
				.filter(Boolean)
		))`, &assets),
	)
	if err != nil {
		return nil, fmt.Errorf("chromedp: capture snapshot: %w", err)
	}
	if strings.TrimSpace(html) == "" {
		return nil, errors.New("chromedp: captured an empty document")
	}

	return &browser.PageSnapshot{
		HTML:       html,
		FinalURL:   finalURL,
		Status:     activeSession.status,
		Headers:    make(map[string]string),
		Assets:     assets,
		UserAction: requiresUserAction(html),
	}, nil
}

func requiresUserAction(html string) bool {
	content := strings.ToLower(html)
	challengeMarkers := []string{
		"cf-chl-",
		"cf-turnstile",
		"/cdn-cgi/challenge-platform/",
		"challenge-form",
		"just a moment...",
		"attention required! | cloudflare",
	}
	for _, marker := range challengeMarkers {
		if strings.Contains(content, marker) {
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
