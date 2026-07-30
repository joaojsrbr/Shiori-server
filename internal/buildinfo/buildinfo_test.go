package buildinfo_test

import (
	"testing"

	"github.com/joaojsr/shiori-server/internal/buildinfo"
)

func TestBuildInfo(t *testing.T) {
	if buildinfo.Version == "" {
		t.Error("expected version to not be empty")
	}
}
