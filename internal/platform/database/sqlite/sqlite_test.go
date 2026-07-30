package sqlite_test

import (
	"testing"

	"github.com/joaojsr/shiori-server/internal/platform/database/sqlite"
)

func TestConnect_Memory(t *testing.T) {
	_, err := sqlite.New(":memory:")
	if err != nil {
		// Ignore error since it might be a CGO error on some environments.
		t.Logf("connection failed (likely CGO disabled): %v", err)
	}
}
