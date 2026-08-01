package browser

import (
	"context"
	"math"
)

// NavigateRequest holds the parameters for a navigation action.
type NavigateRequest struct {
	URL           string
	ProfileURL    string // Optional URL whose hostname owns the isolated persistent profile.
	WaitFor       string // CSS selector to wait for (optional)
	Timeout       int    // Timeout in milliseconds (0 means default)
	AutoScroll    bool   // If true, attempts to scroll to bottom repeatedly until network is idle
	ClickSelector string // If not empty, clicks this selector and waits for DOM mutation
}

// NavigateResult contains the outcome of a navigation action.
type NavigateResult struct {
	SessionID string
	FinalURL  string
	Status    int
}

// PageSnapshot contains a sanitised DOM snapshot and metadata.
type PageSnapshot struct {
	HTML           string
	FinalURL       string
	Status         int
	Headers        map[string]string
	Assets         []string // List of observed asset URLs
	UserAction     bool     // True when navigation cannot proceed without a person.
	UserActionKind UserActionKind
}

type UserActionKind string

const (
	UserActionNone      UserActionKind = ""
	UserActionChallenge UserActionKind = "challenge"
	UserActionLogin     UserActionKind = "login"
	UserActionBlocked   UserActionKind = "blocked"
)

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

	// Screencast starts a screencast stream for the session.
	// Frames are sent to the frames channel as raw JPEG bytes.
	// Input events are received from the input channel.
	// This method blocks until the context is cancelled or the session closes.
	Screencast(ctx context.Context, sessionID string, frames chan<- []byte, input <-chan InputEvent) error
}

// InputEvent represents a user interaction on the screencast canvas.
type InputEvent struct {
	Type      string  `json:"type"`
	X         float64 `json:"x,omitempty"`
	Y         float64 `json:"y,omitempty"`
	Button    string  `json:"button,omitempty"`
	DeltaX    float64 `json:"delta_x,omitempty"`
	DeltaY    float64 `json:"delta_y,omitempty"`
	Key       string  `json:"key,omitempty"`
	Code      string  `json:"code,omitempty"`
	Text      string  `json:"text,omitempty"`
	Modifiers int64   `json:"modifiers,omitempty"`
	Width     int64   `json:"width,omitempty"`
	Height    int64   `json:"height,omitempty"`
}

// Valid rejects malformed or abusive remote input before it reaches CDP.
func (e InputEvent) Valid() bool {
	if e.Modifiers < 0 || e.Modifiers > 15 || len(e.Key) > 64 || len(e.Code) > 64 || len(e.Text) > 16 {
		return false
	}
	finite := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
	validPoint := finite(e.X) && finite(e.Y) && e.X >= 0 && e.Y >= 0 && e.X <= 20000 && e.Y <= 20000
	validButton := e.Button == "none" || e.Button == "left" || e.Button == "middle" || e.Button == "right"

	switch e.Type {
	case "mouseMoved", "mousePressed", "mouseReleased":
		return validPoint && validButton
	case "mouseWheel":
		return validPoint && finite(e.DeltaX) && finite(e.DeltaY) && math.Abs(e.DeltaX) <= 4000 && math.Abs(e.DeltaY) <= 4000
	case "keyDown", "keyUp":
		return e.Key != "" && e.Code != ""
	case "viewport":
		return e.Width >= 320 && e.Width <= 3840 && e.Height >= 240 && e.Height <= 2160
	default:
		return false
	}
}
