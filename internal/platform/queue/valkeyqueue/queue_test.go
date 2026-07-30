package valkeyqueue_test

import (
	"testing"

	"github.com/joaojsr/shiori-server/internal/platform/queue/valkeyqueue"
)

func TestNewProvider(t *testing.T) {
	provider := valkeyqueue.New(nil, "group", "consumer")
	if provider == nil {
		t.Error("expected provider to not be nil")
	}
}
