package sqlite_test

import (
	"testing"

	"github.com/joaojsr/shiori-server/internal/library/sqlite"
	_ "github.com/mattn/go-sqlite3"
)

func TestNewRepository(t *testing.T) {
	repo := sqlite.NewRepository(nil)
	if repo == nil {
		t.Error("expected repository to not be nil")
	}
}
