package sqlite

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/joaojsr/shiori-server/migrations"
)

func (p *provider) Migrate(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	sourceDriver, err := iofs.New(migrations.SQLiteFS, "sqlite")
	if err != nil {
		return fmt.Errorf("creating iofs source driver: %w", err)
	}

	dbDriver, err := sqlite.WithInstance(p.db, &sqlite.Config{})
	if err != nil {
		return fmt.Errorf("creating sqlite database driver: %w", err)
	}

	m, err := migrate.NewWithInstance(
		"iofs",
		sourceDriver,
		"sqlite",
		dbDriver,
	)
	if err != nil {
		return fmt.Errorf("creating migrate instance: %w", err)
	}
	defer m.Close()

	slog.Info("running sqlite migrations")
	err = m.Up()
	if err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			slog.Info("no new migrations to apply")
			return nil
		}
		return fmt.Errorf("running up migrations: %w", err)
	}

	slog.Info("sqlite migrations applied successfully")
	return nil
}
