package fake

import (
	"context"

	"github.com/joaojsr/shiori-server/internal/extraction"
)

// Provider implements extraction.Provider returning pre-configured responses for tests.
type Provider struct {
	// Responses maps a URL to a pre-defined Result
	Responses map[string]*extraction.Result

	// Errors maps a URL to a pre-defined Error
	Errors map[string]error

	// DefaultResult is returned if no specific Response or Error is found
	DefaultResult *extraction.Result
}

// New Creates a new fake extraction provider.
func New() *Provider {
	return &Provider{
		Responses: make(map[string]*extraction.Result),
		Errors:    make(map[string]error),
	}
}

// Extract returns the pre-configured result or error based on the URL.
func (p *Provider) Extract(ctx context.Context, req extraction.Request) (*extraction.Result, error) {
	if err, ok := p.Errors[req.URL]; ok {
		return nil, err
	}

	if res, ok := p.Responses[req.URL]; ok {
		return res, nil
	}

	if p.DefaultResult != nil {
		return p.DefaultResult, nil
	}

	return nil, extraction.ErrExtractionFailed
}
