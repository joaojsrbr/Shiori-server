package httpserver

import "testing"

func TestRedactSensitivePath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{path: "/health/live", want: "/health/live"},
		{path: "/api/v1/challenges/assets/client.js", want: "/api/v1/challenges/assets/client.js"},
		{path: "/api/v1/challenges/secret-token", want: "/api/v1/challenges/{redacted}"},
		{path: "/api/v1/challenges/secret-token/ws", want: "/api/v1/challenges/{redacted}/ws"},
		{path: "/api/v1/challenges/secret-token/complete", want: "/api/v1/challenges/{redacted}/complete"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := redactSensitivePath(tt.path); got != tt.want {
				t.Fatalf("redactSensitivePath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
