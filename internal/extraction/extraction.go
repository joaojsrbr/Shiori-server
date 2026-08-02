package extraction

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// TargetType defines the semantic entity expected from the extraction.
type TargetType string

const (
	TargetManga         TargetType = "manga"
	TargetMangaChapters TargetType = "manga_chapters"
	TargetAnime         TargetType = "anime"
	TargetAnimeEpisodes TargetType = "anime_episodes"
)

var (
	// ErrExtractionFailed is returned when the extraction pipeline fails to extract data.
	ErrExtractionFailed = errors.New("extraction failed")
	// ErrModelUnavailable is returned when the requested AI model or method is offline.
	ErrModelUnavailable = errors.New("extraction model unavailable")
)

// Request contains the inputs required for extraction.
type Request struct {
	// Original URL of the page being extracted
	URL string

	// Sanitized HTML chunk or block to be extracted
	Content string

	// The semantic entity we are looking for
	Target TargetType

	// Callback to report progress (e.g. processing chunks)
	OnProgress func(string)

	// OnInferenceProgress reports live model generation telemetry. TokenLimit
	// is a ceiling, not a completion total, so callers must not present it as a
	// definitive percentage.
	OnInferenceProgress func(InferenceProgress)
}

type InferenceProgress struct {
	Phase           string
	Chunk           int
	Chunks          int
	GeneratedTokens int
	TokenLimit      int
	TokensPerSecond float64
	Elapsed         time.Duration
	Estimated       bool
}

// Result represents the outcome of the extraction process.
// It returns a raw JSON that needs to be consolidated and validated later.
type Result struct {
	// RawJSON is the raw, unvalidated JSON output from the extractor.
	RawJSON json.RawMessage

	// Confidence score of the extraction (0.0 to 1.0)
	Confidence float64

	// Method indicates which extractor generated the result (e.g., "nuextract3@q4_k_m", "fake")
	Method string

	// Warnings contains any non-fatal issues encountered during extraction.
	Warnings []string
}

// Provider abstracts the capability to extract structured data from unstructured content.
// Implementations can be AI-based (NuExtract), Heuristic-based, or Fakes for testing.
type Provider interface {
	Extract(ctx context.Context, req Request) (*Result, error)
}
