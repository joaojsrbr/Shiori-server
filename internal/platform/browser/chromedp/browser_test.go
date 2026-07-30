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

func TestProviderWaitsForAutomaticChallenge(t *testing.T) {
	if os.Getenv("SHIORI_BROWSER_INTEGRATION_TEST") != "1" {
		t.Skip("set SHIORI_BROWSER_INTEGRATION_TEST=1 to run with a local Chrome or Edge installation")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html>
<html>
<head><title>Just a moment...</title></head>
<body>
	<form id="challenge-form">Performing security verification</form>
	<script>
		setTimeout(() => {
			document.title = "Manga";
			document.body.innerHTML = '<main data-ready="true">Challenge completed</main>';
		}, 750);
	</script>
</body>
</html>`)
	}))
	defer server.Close()

	provider := New(t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	navigation, err := provider.Navigate(ctx, browser.NavigateRequest{URL: server.URL})
	if err != nil {
		t.Fatalf("Navigate() error = %v", err)
	}
	defer provider.CloseSession(context.Background(), navigation.SessionID)

	snapshot, err := provider.Snapshot(ctx, navigation.SessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if snapshot.UserAction {
		t.Fatal("Snapshot().UserAction = true after the automatic challenge completed")
	}
	if !strings.Contains(snapshot.HTML, "Challenge completed") {
		t.Fatalf("Snapshot() did not refresh the DOM after the challenge: %s", snapshot.HTML)
	}
}

func TestRequiresUserAction(t *testing.T) {
	tests := []struct {
		name string
		page renderedPage
		want bool
	}{
		{
			name: "normal page with cloudflare script",
			page: renderedPage{
				Title:    "Manga",
				BodyText: "Chapter list",
				HTML:     `<script src="/cdn-cgi/challenge-platform/h/g/orchestrate"></script>`,
			},
			want: false,
		},
		{
			name: "visible challenge widget",
			page: renderedPage{Title: "Manga", HasVisibleChallenge: true},
			want: true,
		},
		{
			name: "cloudflare challenge title",
			page: renderedPage{Title: "Just a moment..."},
			want: true,
		},
		{
			name: "security verification text",
			page: renderedPage{BodyText: "Performing security verification"},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := requiresUserAction(tt.page); got != tt.want {
				t.Errorf("requiresUserAction() = %v, want %v", got, tt.want)
			}
		})
	}
}
