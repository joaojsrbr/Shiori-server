package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // Register pgx driver
)

type Provider struct {
	db *sql.DB
}

func New(dsn string) (*Provider, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening postgres: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("pinging postgres: %w", err)
	}

	return &Provider{db: db}, nil
}

func (p *Provider) DB() *sql.DB {
	return p.db
}

func (p *Provider) Close() error {
	return p.db.Close()
}

func (p *Provider) Migrate(ctx context.Context) error {
	// For now we don't automatically migrate PG from code, as it usually runs in a separate container/job.
	// We can implement it if needed using golang-migrate, similar to what we did in migrations.go.
	// To comply with the interface:
	return Migrate(p.db, "file://migrations/postgres")
}

func (p *Provider) Ping(ctx context.Context) error {
	return p.db.PingContext(ctx)
}
