package chromedp

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/joaojsr/shiori-server/internal/platform/browser"
)

func TestNewProvider(t *testing.T) {
	provider := New(t.TempDir())
	if provider == nil {
		t.Error("expected provider to not be nil")
	}
}

func TestProviderSnapshotReturnsRenderedDOM(t *testing.T) {
	if os.Getenv("SHIORI_BROWSER_INTEGRATION_TEST") != "1" {
		t.Skip("set SHIORI_BROWSER_INTEGRATION_TEST=1 to run with a local Chrome or Edge installation")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html>
<html>
<body>
	<main id="content">Initial</main>
	<img src="/cover.jpg">
	<script>
		document.querySelector("#content").textContent = "Rendered by JavaScript";
		document.querySelector("#content").setAttribute("data-ready", "true");
	</script>
</body>
</html>`)
	}))
	defer server.Close()

	provider := New(t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	navigation, err := provider.Navigate(ctx, browser.NavigateRequest{
		URL:     server.URL,
		WaitFor: `[data-ready="true"]`,
		Timeout: 10_000,
	})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "executable file not found") {
			t.Skipf("Chrome/Edge is unavailable: %v", err)
		}
		t.Fatalf("Navigate() error = %v", err)
	}
	defer provider.CloseSession(context.Background(), navigation.SessionID)

	snapshot, err := provider.Snapshot(ctx, navigation.SessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if !strings.Contains(snapshot.HTML, "Rendered by JavaScript") {
		t.Fatalf("Snapshot() did not return the rendered DOM: %s", snapshot.HTML)
	}
	if snapshot.FinalURL != server.URL+"/" {
		t.Errorf("Snapshot().FinalURL = %q, want %q", snapshot.FinalURL, server.URL+"/")
	}
	if len(snapshot.Assets) != 1 || snapshot.Assets[0] != server.URL+"/cover.jpg" {
		t.Errorf("Snapshot().Assets = %#v, want [%q]", snapshot.Assets, server.URL+"/cover.jpg")
	}
	if snapshot.UserAction {
		t.Error("Snapshot().UserAction = true for a normal page")
	}
}

func TestProviderSnapshotRejectsUnknownSession(t *testing.T) {
	provider := New(t.TempDir())
	_, err := provider.Snapshot(context.Background(), "missing")
	if err == nil {
		t.Fatal("Snapshot() error = nil, want session-not-found error")
	}
}

func TestRequiresUserAction(t *testing.T) {
	tests := []struct {
		name string
		html string
		want bool
	}{
		{name: "normal page", html: `<html><body>Manga</body></html>`, want: false},
		{name: "cloudflare challenge", html: `<form id="challenge-form"><div class="cf-turnstile"></div></form>`, want: true},
		{name: "challenge platform", html: `<script src="/cdn-cgi/challenge-platform/h/g/orchestrate"></script>`, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := requiresUserAction(tt.html); got != tt.want {
				t.Errorf("requiresUserAction() = %v, want %v", got, tt.want)
			}
		})
	}
}
