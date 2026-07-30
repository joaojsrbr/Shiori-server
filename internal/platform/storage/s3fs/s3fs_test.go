package s3fs_test

import (
	"testing"

	"github.com/joaojsr/shiori-server/internal/platform/storage/s3fs"
)

func TestNewProvider(t *testing.T) {
	// S3 uses AWS config under the hood, we just test object creation.
	provider, err := s3fs.New("endpoint", "key", "secret", "dummy-bucket", false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if provider == nil {
		t.Error("expected provider to not be nil")
	}
}
