package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"

	"github.com/joaojsr/shiori-server/internal/platform/database"
	_ "modernc.org/sqlite" // register sqlite driver
)

type provider struct {
	db *sql.DB
}

// New creates a new SQLite database provider using modernc.org/sqlite.
// It applies pragma settings suitable for WAL mode and connection pooling.
func New(dbPath string) (database.Provider, error) {
	// Ensure absolute path
	absPath, err := filepath.Abs(dbPath)
	if err != nil {
		return nil, fmt.Errorf("resolving database path: %w", err)
	}

	// PRAGMAs:
	// - _pragma=foreign_keys(1) : Enable foreign key constraints
	// - _pragma=journal_mode(WAL) : Better concurrency
	// - _pragma=synchronous(NORMAL) : Safe enough with WAL
	// - _pragma=busy_timeout(5000) : Wait 5s before returning "database is locked"
	dsn := fmt.Sprintf("%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)", absPath)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite database: %w", err)
	}

	// With WAL, concurrent reads are fine, but concurrent writes can lock.
	// Setting MaxOpenConns > 1 allows concurrent reads, but write contention might cause busy errors.
	// A safe default for personal local apps is a small pool.
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(5)

	return &provider{db: db}, nil
}

func (p *provider) DB() *sql.DB {
	return p.db
}

func (p *provider) Close() error {
	return p.db.Close()
}

func (p *provider) Ping(ctx context.Context) error {
	return p.db.PingContext(ctx)
}
