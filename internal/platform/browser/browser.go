package browser

import (
	"context"
)

// NavigateRequest holds the parameters for a navigation action.
type NavigateRequest struct {
	URL     string
	WaitFor string // CSS selector to wait for (optional)
	Timeout int    // Timeout in milliseconds (0 means default)
}

// NavigateResult contains the outcome of a navigation action.
type NavigateResult struct {
	SessionID string
	FinalURL  string
	Status    int
}

// PageSnapshot contains a sanitised DOM snapshot and metadata.
type PageSnapshot struct {
	HTML       string
	FinalURL   string
	Status     int
	Headers    map[string]string
	Assets     []string // List of observed asset URLs
	UserAction bool     // True if a challenge/captcha requires user action
}

// Provider abstracts a browser automation tool.
type Provider interface {
	// IsAvailable checks if the underlying browser engine (e.g., Chrome executable) is available.
	IsAvailable() bool

	// Navigate opens a new session and navigates to the URL.
	// It may return a state requiring user action.
	Navigate(ctx context.Context, req NavigateRequest) (*NavigateResult, error)

	// Snapshot retrieves the DOM snapshot of an active session.
	Snapshot(ctx context.Context, sessionID string) (*PageSnapshot, error)

	// CloseSession closes a given session.
	CloseSession(ctx context.Context, sessionID string) error
}
