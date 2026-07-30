package postgres_test

import (
	"testing"

	"github.com/joaojsr/shiori-server/internal/library/postgres"
)

func TestNewRepository(t *testing.T) {
	// We can't easily test the Postgres connection in a pure unit test without a mock DB.
	// But we can ensure NewRepository returns a non-nil struct.
	repo := postgres.NewRepository(nil)
	if repo == nil {
		t.Error("expected repository to not be nil")
	}
}
