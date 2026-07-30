package fake_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/joaojsr/shiori-server/internal/extraction"
	"github.com/joaojsr/shiori-server/internal/extraction/fake"
)

func TestFakeProvider_Extract(t *testing.T) {
	ctx := context.Background()
	provider := fake.New()

	successURL := "https://example.com/manga/1"
	errorURL := "https://example.com/error"
	unknownURL := "https://example.com/unknown"

	expectedJSON := json.RawMessage(`{"title": "Test Manga"}`)

	provider.Responses[successURL] = &extraction.Result{
		RawJSON:    expectedJSON,
		Confidence: 0.99,
		Method:     "fake",
	}

	provider.Errors[errorURL] = extraction.ErrModelUnavailable

	t.Run("Returns configured response", func(t *testing.T) {
		req := extraction.Request{URL: successURL, Target: extraction.TargetMedia}
		res, err := provider.Extract(ctx, req)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if string(res.RawJSON) != string(expectedJSON) {
			t.Errorf("expected %s, got %s", expectedJSON, res.RawJSON)
		}
	})

	t.Run("Returns configured error", func(t *testing.T) {
		req := extraction.Request{URL: errorURL, Target: extraction.TargetMedia}
		res, err := provider.Extract(ctx, req)
		if err != extraction.ErrModelUnavailable {
			t.Fatalf("expected ErrModelUnavailable, got %v", err)
		}
		if res != nil {
			t.Errorf("expected nil result, got %v", res)
		}
	})

	t.Run("Returns default error for unknown URL without default result", func(t *testing.T) {
		req := extraction.Request{URL: unknownURL, Target: extraction.TargetMedia}
		_, err := provider.Extract(ctx, req)
		if err != extraction.ErrExtractionFailed {
			t.Fatalf("expected ErrExtractionFailed, got %v", err)
		}
	})

	t.Run("Returns default result for unknown URL if set", func(t *testing.T) {
		defaultJSON := json.RawMessage(`{"title": "Default"}`)
		provider.DefaultResult = &extraction.Result{RawJSON: defaultJSON}

		req := extraction.Request{URL: unknownURL, Target: extraction.TargetMedia}
		res, err := provider.Extract(ctx, req)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if string(res.RawJSON) != string(defaultJSON) {
			t.Errorf("expected %s, got %s", defaultJSON, res.RawJSON)
		}
	})
}
