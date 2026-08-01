package chromedp

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/google/uuid"
	"github.com/joaojsr/shiori-server/internal/platform/browser"
)

// Provider implements browser.Provider using local chromedp.
type Provider struct {
	baseDir       string
	mu            sync.Mutex
	sessions      map[string]*session
	profileLeases map[string]chan struct{}
}

type session struct {
	ctx            context.Context
	cancel         context.CancelFunc
	releaseProfile func()
	requestedURL   string
	status         int
}

type renderedPage struct {
	HTML                string   `json:"-"`
	FinalURL            string   `json:"-"`
	Assets              []string `json:"-"`
	Title               string   `json:"title"`
	BodyText            string   `json:"bodyText"`
	HasVisibleChallenge bool     `json:"hasVisibleChallenge"`
	HasVisibleLogin     bool     `json:"hasVisibleLogin"`
}

const automaticChallengeWait = 10 * time.Second

// New creates the provider.
// baseDir is used for isolated browser profiles.
func New(baseDir string) *Provider {
	return &Provider{
		baseDir:       baseDir,
		sessions:      make(map[string]*session),
		profileLeases: make(map[string]chan struct{}),
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

	targetURL := req.URL
	if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
		targetURL = "https://" + targetURL
	}
	profileURL := req.ProfileURL
	if profileURL == "" {
		profileURL = targetURL
	}
	profileKey, err := domainProfileKey(profileURL)
	if err != nil {
		return nil, fmt.Errorf("chromedp: invalid target URL: %w", err)
	}
	releaseProfile, err := p.acquireProfile(ctx, profileKey)
	if err != nil {
		return nil, fmt.Errorf("chromedp: waiting for domain profile: %w", err)
	}
	releaseOnError := true
	defer func() {
		if releaseOnError {
			releaseProfile()
		}
	}()

	// Chromium owns cookie and credential encryption inside this persistent,
	// domain-isolated profile. The backend never exports their raw values.
	sessionID := uuid.New().String()
	profileDir := filepath.Join(p.baseDir, "domains", profileKey)
	if err := os.MkdirAll(profileDir, 0700); err != nil {
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
		ctx:            browserCtx,
		cancel:         cancelSession,
		releaseProfile: releaseProfile,
		requestedURL:   targetURL,
		status:         200,
	}
	p.mu.Unlock()
	releaseOnError = false

	// Simplification for MVP: We don't have true HTTP status code natively in chromedp without network interception.
	// Return 200 until response interception is implemented.
	return &browser.NavigateResult{
		SessionID: sessionID,
		FinalURL:  finalURL,
		Status:    200,
	}, nil
}

func domainProfileKey(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	hostname := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if hostname == "" {
		return "", errors.New("URL has no hostname")
	}
	hash := sha256.Sum256([]byte(hostname))
	return fmt.Sprintf("domain_%x", hash[:12]), nil
}

func (p *Provider) acquireProfile(ctx context.Context, key string) (func(), error) {
	p.mu.Lock()
	lease, ok := p.profileLeases[key]
	if !ok {
		lease = make(chan struct{}, 1)
		p.profileLeases[key] = lease
	}
	p.mu.Unlock()

	select {
	case lease <- struct{}{}:
		var once sync.Once
		return func() {
			once.Do(func() { <-lease })
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
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
	if classifyUserAction(page, activeSession.requestedURL) == browser.UserActionChallenge {
		page, err = waitForAutomaticChallenge(snapshotCtx, page)
		if err != nil {
			return nil, fmt.Errorf("chromedp: wait for automatic challenge: %w", err)
		}
	}
	if strings.TrimSpace(page.HTML) == "" {
		return nil, errors.New("chromedp: captured an empty document")
	}

	actionKind := classifyUserAction(page, activeSession.requestedURL)
	return &browser.PageSnapshot{
		HTML:           page.HTML,
		FinalURL:       page.FinalURL,
		Status:         activeSession.status,
		Headers:        make(map[string]string),
		Assets:         page.Assets,
		UserAction:     actionKind != browser.UserActionNone,
		UserActionKind: actionKind,
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
			const loginWords = /sign\s*in|log\s*in|login|entrar|acessar|iniciar\s+sesi[oó]n|connexion|anmelden|ログイン|登录|登入|로그인/i;
			const visiblePassword = Array.from(document.querySelectorAll("input[type='password']"))
				.some((element) => isVisible(element) && element.autocomplete !== "new-password");
			const visibleIdentity = Array.from(document.querySelectorAll(
				"input[type='email'], input[autocomplete='username'], input[name*='email' i], input[name*='user' i], input[name*='login' i]"
			)).some(isVisible);
			const visibleLoginForm = Array.from(document.querySelectorAll("form")).some((form) => {
				if (!isVisible(form)) return false;
				const action = form.getAttribute("action") || "";
				const submitCopy = Array.from(form.querySelectorAll("button, input[type='submit']"))
					.filter(isVisible)
					.map((element) => element.innerText || element.value || element.getAttribute("aria-label") || "")
					.join(" ");
				return loginWords.test(action) || loginWords.test(submitCopy);
			});
			const bodyCopy = document.body && document.body.innerText || "";
			return {
				title: document.title || "",
				bodyText: bodyCopy.slice(0, 8192),
				hasVisibleChallenge: selectors.some((selector) =>
					Array.from(document.querySelectorAll(selector)).some(isVisible)
				),
				hasVisibleLogin: visiblePassword || visibleLoginForm || (visibleIdentity && loginWords.test(bodyCopy))
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

	for isCloudflareChallenge(current) {
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

func isCloudflareChallenge(page renderedPage) bool {
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

func classifyUserAction(page renderedPage, requestedURL string) browser.UserActionKind {
	bodyText := strings.ToLower(page.BodyText)
	blockedMarkers := []string{
		"why have i been blocked?",
		"sorry, you have been blocked",
		"cloudflare ray id",
	}
	for _, marker := range blockedMarkers {
		if strings.Contains(bodyText, marker) {
			return browser.UserActionBlocked
		}
	}
	if isCloudflareChallenge(page) {
		return browser.UserActionChallenge
	}
	if page.HasVisibleLogin || redirectedToLogin(requestedURL, page.FinalURL) {
		return browser.UserActionLogin
	}
	return browser.UserActionNone
}

func requiresUserAction(page renderedPage) bool {
	return classifyUserAction(page, "") != browser.UserActionNone
}

func redirectedToLogin(requestedURL, finalURL string) bool {
	if requestedURL == "" || finalURL == "" || requestedURL == finalURL {
		return false
	}
	parsed, err := url.Parse(finalURL)
	if err != nil {
		return false
	}
	path := strings.ToLower(strings.Trim(parsed.Path, "/"))
	segments := strings.Split(path, "/")
	for _, segment := range segments {
		switch segment {
		case "login", "signin", "sign-in", "sign_in", "authenticate", "authentication":
			return true
		}
	}
	// Common framework routes use /sessions/new for a login form.
	if strings.HasSuffix(path, "sessions/new") || strings.HasSuffix(path, "session/new") {
		return true
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

	closeCtx, cancelClose := context.WithTimeout(activeSession.ctx, 5*time.Second)
	closeErr := chromedp.Cancel(closeCtx)
	cancelClose()
	activeSession.cancel()
	if activeSession.releaseProfile != nil {
		activeSession.releaseProfile()
	}
	if closeErr != nil && !errors.Is(closeErr, context.Canceled) {
		return fmt.Errorf("chromedp: close browser gracefully: %w", closeErr)
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

	streamCtx, cancelStream := context.WithCancel(activeSession.ctx)
	stopCallerCancellation := context.AfterFunc(ctx, cancelStream)
	defer func() {
		stopCallerCancellation()
		_ = chromedp.Run(activeSession.ctx, page.StopScreencast())
		cancelStream()
	}()

	// The listener is scoped to this WebSocket stream so reconnecting does not
	// accumulate frame handlers on the long-lived browser session.
	chromedp.ListenTarget(streamCtx, func(ev interface{}) {
		if ev, ok := ev.(*page.EventScreencastFrame); ok {
			go func() {
				_ = chromedp.Run(streamCtx, page.ScreencastFrameAck(ev.SessionID))
			}()

			data, err := base64.StdEncoding.DecodeString(ev.Data)
			if err == nil {
				select {
				case <-streamCtx.Done():
				case frames <- data:
				default:
					// Prefer a fresh frame over blocking Chrome's event loop.
				}
			}
		}
	})

	err := chromedp.Run(streamCtx,
		page.StartScreencast().WithFormat(page.ScreencastFormatJpeg).WithQuality(80).WithEveryNthFrame(1),
	)
	if err != nil {
		return fmt.Errorf("chromedp: start screencast failed: %w", err)
	}

	for {
		select {
		case <-streamCtx.Done():
			return streamCtx.Err()
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
				act = input.DispatchMouseEvent(input.MouseReleased, ev.X, ev.Y).WithButton(input.MouseButton(ev.Button)).WithClickCount(1)
			case "mouseWheel":
				act = input.DispatchMouseEvent(input.MouseWheel, ev.X, ev.Y).WithDeltaX(ev.DeltaX).WithDeltaY(ev.DeltaY)
			case "keyDown":
				params := input.DispatchKeyEvent(input.KeyDown).
					WithKey(ev.Key).
					WithCode(ev.Code).
					WithModifiers(input.Modifier(ev.Modifiers))
				if ev.Text != "" {
					params = params.WithText(ev.Text)
				}
				act = params
			case "keyUp":
				act = input.DispatchKeyEvent(input.KeyUp).
					WithKey(ev.Key).
					WithCode(ev.Code).
					WithModifiers(input.Modifier(ev.Modifiers))
			case "viewport":
				act = emulation.SetDeviceMetricsOverride(ev.Width, ev.Height, 1, false).
					WithScreenWidth(ev.Width).
					WithScreenHeight(ev.Height)
			}
			if act != nil {
				if err := chromedp.Run(streamCtx, act); err != nil && streamCtx.Err() == nil {
					return fmt.Errorf("chromedp: dispatch input: %w", err)
				}
			}
		}
	}
}
