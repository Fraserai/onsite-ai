// Package store holds Postgres access and the SQL migrations.
package store

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"net/url"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // registers the pgx5:// driver
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Store is the server's handle on Postgres.
type Store struct {
	pool *pgxpool.Pool
}

// Open connects to Postgres and checks the connection works.
func Open(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect to database: %w", redact(err, databaseURL))
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", redact(err, databaseURL))
	}
	return &Store{pool: pool}, nil
}

// Close releases all database connections.
func (s *Store) Close() { s.pool.Close() }

// Ping reports whether the database is reachable.
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// Pool exposes the connection pool to the rest of the store package's callers
// that need transactions.
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// MigrateUp applies every migration that has not run yet.
func MigrateUp(databaseURL string) error {
	m, err := newMigrate(databaseURL)
	if err != nil {
		return err
	}
	defer closeMigrate(m)
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", redact(err, databaseURL))
	}
	return nil
}

// MigrateDown rolls every migration back. Used by tests.
func MigrateDown(databaseURL string) error {
	m, err := newMigrate(databaseURL)
	if err != nil {
		return err
	}
	defer closeMigrate(m)
	if err := m.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("roll back migrations: %w", redact(err, databaseURL))
	}
	return nil
}

func newMigrate(databaseURL string) (*migrate.Migrate, error) {
	src, err := iofs.New(migrationFiles, "migrations")
	if err != nil {
		return nil, fmt.Errorf("load migrations: %w", err)
	}
	u, err := url.Parse(databaseURL)
	if err != nil {
		// url.Parse errors echo the input, which holds the password.
		return nil, errors.New("database URL is not a valid URL")
	}
	u.Scheme = "pgx5"
	m, err := migrate.NewWithSourceInstance("iofs", src, u.String())
	if err != nil {
		return nil, fmt.Errorf("open database for migrations: %w", redact(err, databaseURL))
	}
	return m, nil
}

func closeMigrate(m *migrate.Migrate) {
	// Close errors here carry nothing actionable once the migration result
	// itself has been returned.
	_, _ = m.Close()
}
