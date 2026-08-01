package chromedp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/joaojsr/shiori-server/internal/platform/browser"
)

func TestDomainProfileKeyIsStableAndHostScoped(t *testing.T) {
	first, err := domainProfileKey("https://Example.COM/manga/1")
	if err != nil {
		t.Fatalf("domainProfileKey() error = %v", err)
	}
	second, err := domainProfileKey("https://example.com/other")
	if err != nil {
		t.Fatalf("domainProfileKey() error = %v", err)
	}
	subdomain, err := domainProfileKey("https://reader.example.com/")
	if err != nil {
		t.Fatalf("domainProfileKey() error = %v", err)
	}
	if first != second {
		t.Fatalf("same hostname produced different keys: %q and %q", first, second)
	}
	if first == subdomain {
		t.Fatalf("different hostnames shared profile key %q", first)
	}
	if _, err := domainProfileKey("https:///missing-host"); err == nil {
		t.Fatal("URL without hostname should be rejected")
	}
}

func TestPersistentProfileRetainsCookiesForSameDomain(t *testing.T) {
	if os.Getenv("SHIORI_BROWSER_INTEGRATION_TEST") != "1" {
		t.Skip("set SHIORI_BROWSER_INTEGRATION_TEST=1 to run with a local Chrome or Edge installation")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cookie, err := r.Cookie("shiori_session"); err == nil && cookie.Value == "verified" {
			fmt.Fprint(w, "session restored")
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     "shiori_session",
			Value:    "verified",
			Path:     "/",
			HttpOnly: true,
			MaxAge:   3600,
			Expires:  time.Now().Add(time.Hour),
		})
		fmt.Fprint(w, "session created")
	}))
	defer server.Close()

	provider := New(t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	first, err := provider.Navigate(ctx, browser.NavigateRequest{URL: server.URL})
	if err != nil {
		t.Fatalf("first Navigate() error = %v", err)
	}
	if err := provider.CloseSession(context.Background(), first.SessionID); err != nil {
		t.Fatalf("first CloseSession() error = %v", err)
	}

	second, err := provider.Navigate(ctx, browser.NavigateRequest{URL: server.URL})
	if err != nil {
		t.Fatalf("second Navigate() error = %v", err)
	}
	defer provider.CloseSession(context.Background(), second.SessionID)
	snapshot, err := provider.Snapshot(ctx, second.SessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if !strings.Contains(snapshot.HTML, "session restored") {
		t.Fatalf("persistent profile did not restore cookie: %s", snapshot.HTML)
	}
}

func TestAcquireProfileSerializesSameDomain(t *testing.T) {
	provider := New(t.TempDir())
	release, err := provider.acquireProfile(context.Background(), "domain-a")
	if err != nil {
		t.Fatalf("first acquireProfile() error = %v", err)
	}

	waitCtx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	if _, err := provider.acquireProfile(waitCtx, "domain-a"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("concurrent acquireProfile() error = %v, want deadline exceeded", err)
	}

	release()
	releaseAgain, err := provider.acquireProfile(context.Background(), "domain-a")
	if err != nil {
		t.Fatalf("acquireProfile() after release error = %v", err)
	}
	releaseAgain()
}
