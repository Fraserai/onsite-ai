// Package storetest gives tests a fresh, migrated Postgres database.
package storetest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"onsite-ai/internal/store"
)

// EnvVar names the environment variable that points tests at Postgres.
// `make test` sets it; without it database tests are skipped.
const EnvVar = "TEST_DATABASE_URL"

// NewURL creates an empty database for one test and returns its URL.
// The database is dropped when the test finishes.
func NewURL(t *testing.T) string {
	t.Helper()
	base := os.Getenv(EnvVar)
	if base == "" {
		t.Skipf("%s is not set; run tests with `make test`", EnvVar)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatalf("connect to test database server: %v", err)
	}
	defer func() { _ = admin.Close(ctx) }()

	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatalf("random database name: %v", err)
	}
	name := "test_" + hex.EncodeToString(suffix)
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("create test database: %v", err)
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		conn, err := pgx.Connect(ctx, base)
		if err != nil {
			t.Logf("connect to drop test database: %v", err)
			return
		}
		defer func() { _ = conn.Close(ctx) }()
		if _, err := conn.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)"); err != nil {
			t.Logf("drop test database: %v", err)
		}
	})

	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("%s is not a valid URL", EnvVar)
	}
	u.Path = "/" + name
	return u.String()
}

// New returns a store connected to a fresh database with all migrations applied.
func New(t *testing.T) *store.Store {
	t.Helper()
	dbURL := NewURL(t)
	if err := store.MigrateUp(dbURL); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	s, err := store.Open(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(s.Close)
	return s
}
