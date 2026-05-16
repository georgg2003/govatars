package testdb

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"

	"github.com/golang-migrate/migrate/v4"
	pgxmigrate "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file" // register file:// migration source
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
)

// RunMigrations applies all UP migrations from migrationsDir using [github.com/golang-migrate/migrate]
// (same semantics as `migrate -path <dir> up`). migrationsDir should contain versioned *.sql files.
// pool is not closed; a short-lived [database/sql.DB] wrapper is closed before return.
func RunMigrations(pool *pgxpool.Pool, migrationsDir string) error {
	db := stdlib.OpenDBFromPool(pool)
	defer func() {
		//nolint:errcheck // wrapper around pool; closing releases the sql.DB facade only.
		_ = db.Close()
	}()

	driver, err := pgxmigrate.WithInstance(db, &pgxmigrate.Config{})
	if err != nil {
		return fmt.Errorf("migrate pgx driver: %w", err)
	}

	absDir, err := filepath.Abs(migrationsDir)
	if err != nil {
		return fmt.Errorf("migrate abs migrations dir: %w", err)
	}
	srcURL := (&url.URL{Scheme: "file", Path: filepath.ToSlash(absDir)}).String()

	m, err := migrate.NewWithDatabaseInstance(srcURL, "pgx5", driver)
	if err != nil {
		return fmt.Errorf("migrate new: %w", err)
	}
	defer func() {
		//nolint:errcheck // source/db drivers already applied migrations
		_, _ = m.Close()
	}()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}
