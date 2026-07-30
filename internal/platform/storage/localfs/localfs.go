package localfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/joaojsr/shiori-server/internal/platform/storage"
)

// Provider implements storage.Provider using the local filesystem.
type Provider struct {
	baseDir string
}

// New creates a new local filesystem storage provider.
// It ensures that the base directory exists.
func New(baseDir string) (*Provider, error) {
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, fmt.Errorf("localfs: resolving absolute path: %w", err)
	}

	if err := os.MkdirAll(absBase, 0755); err != nil {
		return nil, fmt.Errorf("localfs: creating base directory: %w", err)
	}

	return &Provider{baseDir: absBase}, nil
}

func (p *Provider) resolvePath(key string) (string, error) {
	// Prevent path traversal
	if strings.Contains(key, "..") {
		return "", errors.New("localfs: invalid key containing '..'")
	}

	// Normalize key
	cleanKey := filepath.FromSlash(key)
	if strings.HasPrefix(cleanKey, string(filepath.Separator)) {
		cleanKey = cleanKey[1:]
	}

	fullPath := filepath.Join(p.baseDir, cleanKey)

	// Ensure the resolved path is actually under baseDir (extra safety)
	if !strings.HasPrefix(fullPath, p.baseDir) {
		return "", errors.New("localfs: key escapes base directory")
	}

	return fullPath, nil
}

// Put writes data to the local filesystem.
func (p *Provider) Put(ctx context.Context, key string, r io.Reader) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	fullPath, err := p.resolvePath(key)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return fmt.Errorf("localfs: creating parent directories: %w", err)
	}

	f, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("localfs: creating file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, r); err != nil {
		return fmt.Errorf("localfs: writing file: %w", err)
	}
	return nil
}

// Get reads data from the local filesystem.
func (p *Provider) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	fullPath, err := p.resolvePath(key)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, storage.ErrNotFound
		}
		return nil, fmt.Errorf("localfs: opening file: %w", err)
	}
	return f, nil
}

// Delete removes the file from the local filesystem.
func (p *Provider) Delete(ctx context.Context, key string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	fullPath, err := p.resolvePath(key)
	if err != nil {
		return err
	}

	err = os.Remove(fullPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("localfs: deleting file: %w", err)
	}
	return nil
}
