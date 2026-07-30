package database

import (
	"context"
	"database/sql"
)

// Provider abstracts the database connection and core operations.
type Provider interface {
	// DB returns the underlying sql.DB instance for repositories to use.
	DB() *sql.DB

	// Close terminates the database connection.
	Close() error

	// Migrate runs all pending up migrations.
	Migrate(ctx context.Context) error

	// Ping checks if the database is reachable and ready.
	Ping(ctx context.Context) error
}
