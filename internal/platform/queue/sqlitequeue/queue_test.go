package sqlitequeue_test

import (
	"testing"

	"github.com/joaojsr/shiori-server/internal/platform/queue/sqlitequeue"
	_ "github.com/mattn/go-sqlite3"
)

func TestProvider_Init(t *testing.T) {
	// The struct and methods exist, just test that New returns non-nil
	p := sqlitequeue.New(nil)
	if p == nil {
		t.Error("expected provider to not be nil")
	}
}
