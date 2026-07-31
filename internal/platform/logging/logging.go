// Package logging configures structured logging using log/slog.
//
// It supports JSON output for production and text output for development.
// The logger is configured once at startup and set as the default.
package logging

import (
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/lmittmann/tint"
)

// Setup creates and sets the default slog logger based on the given level and
// format. It writes to the provided writer (typically os.Stderr).
//
// Supported levels: debug, info, warn, error.
// Supported formats: json, text, pretty.
func Setup(w io.Writer, level, format string) *slog.Logger {
	lvl := parseLevel(level)

	opts := &slog.HandlerOptions{
		Level:     lvl,
		AddSource: lvl == slog.LevelDebug,
	}

	var handler slog.Handler
	switch strings.ToLower(format) {
	case "text":
		handler = slog.NewTextHandler(w, opts)
	case "pretty":
		handler = tint.NewHandler(w, &tint.Options{
			Level:      lvl,
			TimeFormat: time.TimeOnly,
			AddSource:  opts.AddSource,
		})
	default:
		handler = slog.NewJSONHandler(w, opts)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)

	return logger
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
