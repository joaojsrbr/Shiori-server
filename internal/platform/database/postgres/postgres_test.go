package postgres_test

import (
	"testing"

	"github.com/joaojsr/shiori-server/internal/platform/database/postgres"
)

func TestConnect_Invalid(t *testing.T) {
	_, err := postgres.New("invalid-dsn")
	if err == nil {
		t.Error("expected error for invalid dsn")
	}
}
