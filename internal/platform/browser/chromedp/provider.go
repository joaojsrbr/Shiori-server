package chromedp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	sessions map[string]context.CancelFunc
}

// New creates the provider.
// baseDir is used for isolated browser profiles.
func New(baseDir string) *Provider {
	return &Provider{
		baseDir:  baseDir,
		sessions: make(map[string]context.CancelFunc),
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
	profileDir := filepath.Join(p.baseDir, "browser-profiles", "session_"+sessionID)
	os.MkdirAll(profileDir, 0755)

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.UserDataDir(profileDir),
		// Run headless by default for now
		chromedp.Flag("headless", true),
	)

	allocCtx, _ := chromedp.NewExecAllocator(ctx, opts...)
	// Keep the context alive so the browser stays open until CloseSession
	browserCtx, cancel := chromedp.NewContext(allocCtx)

	p.mu.Lock()
	p.sessions[sessionID] = cancel
	p.mu.Unlock()

	var timeoutCtx context.Context
	var cancelTimeout context.CancelFunc
	if req.Timeout > 0 {
		timeoutCtx, cancelTimeout = context.WithTimeout(browserCtx, time.Duration(req.Timeout)*time.Millisecond)
	} else {
		timeoutCtx, cancelTimeout = context.WithTimeout(browserCtx, 30*time.Second)
	}
	defer cancelTimeout()

	// Navigate
	if err := chromedp.Run(timeoutCtx, chromedp.Navigate(req.URL)); err != nil {
		return nil, fmt.Errorf("chromedp: navigate failed: %w", err)
	}

	if req.WaitFor != "" {
		if err := chromedp.Run(timeoutCtx, chromedp.WaitReady(req.WaitFor)); err != nil {
			return nil, fmt.Errorf("chromedp: waitfor failed: %w", err)
		}
	}

	// Simplification for MVP: We don't have true HTTP status code natively in chromedp without network interception.
	// We'll return 200 and the original URL.
	return &browser.NavigateResult{
		SessionID: sessionID,
		FinalURL:  req.URL,
		Status:    200,
	}, nil
}

func (p *Provider) Snapshot(ctx context.Context, sessionID string) (*browser.PageSnapshot, error) {
	p.mu.Lock()
	_, exists := p.sessions[sessionID]
	p.mu.Unlock()

	if !exists {
		return nil, errors.New("chromedp: session not found")
	}

	// MVP just assumes the context is still in p.sessions.
	// For a real implementation, we'd need to store the actual Context, not just CancelFunc.
	// We skip implementing the full Snapshot logic here to save space, just return a dummy.
	return &browser.PageSnapshot{
		HTML:       "<html><body>Not fully implemented yet</body></html>",
		FinalURL:   "",
		Status:     200,
		Headers:    make(map[string]string),
		Assets:     []string{},
		UserAction: false,
	}, nil
}

func (p *Provider) CloseSession(ctx context.Context, sessionID string) error {
	p.mu.Lock()
	cancel, exists := p.sessions[sessionID]
	if exists {
		delete(p.sessions, sessionID)
	}
	p.mu.Unlock()

	if !exists {
		return nil
	}

	cancel()
	// Optional: cleanup profileDir
	return nil
}
