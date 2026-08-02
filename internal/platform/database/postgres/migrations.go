package postgres

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/joaojsr/shiori-server/migrations"
)

// Migrate runs the PostgreSQL migrations embedded in the server binary.
func Migrate(db *sql.DB, _ string) error {
	sourceDriver, err := iofs.New(migrations.PostgresFS, "postgres")
	if err != nil {
		return fmt.Errorf("creating embedded postgres migration source: %w", err)
	}
	defer sourceDriver.Close()

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("creating postgres driver for migrate: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", sourceDriver, "postgres", driver)
	if err != nil {
		return fmt.Errorf("creating migrate instance: %w", err)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrating database up: %w", err)
	}

	return nil
}
