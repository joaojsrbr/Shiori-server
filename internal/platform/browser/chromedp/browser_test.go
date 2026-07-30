package chromedp_test

import (
	"testing"

	"github.com/joaojsr/shiori-server/internal/platform/browser/chromedp"
)

func TestNewProvider(t *testing.T) {
	provider := chromedp.New("dummy")
	if provider == nil {
		t.Error("expected provider to not be nil")
	}
}
