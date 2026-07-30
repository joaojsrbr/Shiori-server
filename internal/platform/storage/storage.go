package storage

import (
	"context"
	"errors"
	"io"
)

var (
	// ErrNotFound is returned when the requested file does not exist.
	ErrNotFound = errors.New("storage: object not found")
)

// Provider abstracts file storage operations, allowing transparent switching
// between local filesystem (portable profile) and MinIO/S3 (docker profile).
type Provider interface {
	// Put writes data from the reader to the specified key.
	// The key should use forward slashes (/) as path separators regardless of OS.
	Put(ctx context.Context, key string, r io.Reader) error

	// Get returns a reader for the specified key.
	// The caller is responsible for closing the reader.
	Get(ctx context.Context, key string) (io.ReadCloser, error)

	// Delete removes the object at the specified key.
	// It should return nil if the object does not exist.
	Delete(ctx context.Context, key string) error
}
