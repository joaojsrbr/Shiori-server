package localfs_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/joaojsr/shiori-server/internal/platform/storage"
	"github.com/joaojsr/shiori-server/internal/platform/storage/localfs"
)

func TestLocalFSProvider(t *testing.T) {
	baseDir := t.TempDir()
	p, err := localfs.New(baseDir)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	ctx := context.Background()
	key := "images/cover.jpg"
	content := []byte("fake image data")

	// 1. Put
	if err := p.Put(ctx, key, bytes.NewReader(content)); err != nil {
		t.Fatalf("Put() error: %v", err)
	}

	// 2. Get
	r, err := p.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}

	readContent, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll() error: %v", err)
	}
	r.Close() // Close explicitly before Delete to avoid Windows lock errors
	if !bytes.Equal(readContent, content) {
		t.Errorf("Get() content = %q, want %q", string(readContent), string(content))
	}

	// 3. Delete
	if err := p.Delete(ctx, key); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	// 4. Get after delete should return ErrNotFound
	_, err = p.Get(ctx, key)
	if err != storage.ErrNotFound {
		t.Errorf("Get() after delete error = %v, want %v", err, storage.ErrNotFound)
	}
}

func TestPathTraversal(t *testing.T) {
	baseDir := t.TempDir()
	p, err := localfs.New(baseDir)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	ctx := context.Background()

	invalidKeys := []string{
		"../outside.txt",
		"foo/../../outside.txt",
	}

	for _, key := range invalidKeys {
		err := p.Put(ctx, key, bytes.NewReader([]byte("test")))
		if err == nil {
			t.Errorf("Put(%q) should have failed due to path traversal", key)
		}
	}
}
