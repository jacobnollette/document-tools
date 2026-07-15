// Package testdb provides Postgres-backed test databases. Tests that need a
// database are skipped unless TEST_DATABASE_URL is set (CI sets it via a
// postgres service; locally, run one with:
//
//	docker run -d -e POSTGRES_USER=test -e POSTGRES_PASSWORD=test -p 15432:5432 postgres:16-alpine
//	export TEST_DATABASE_URL=postgres://test:test@localhost:15432/test?sslmode=disable
package testdb

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"testing"

	"document-tools/internal/db"
)

// Open returns a migrated connection to a dedicated database for the given
// suite name, wiping its tables first. Each test package uses its own suite
// name so packages can run in parallel.
func Open(t *testing.T, suite string) *sql.DB {
	t.Helper()
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database tests")
	}
	ctx := context.Background()

	admin, err := db.Open(ctx, base)
	if err != nil {
		t.Fatalf("connect to test postgres: %v", err)
	}
	defer admin.Close()

	name := "dt_test_" + suite
	var exists bool
	if err := admin.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)`, name).Scan(&exists); err != nil {
		t.Fatalf("check test database: %v", err)
	}
	if !exists {
		if _, err := admin.ExecContext(ctx, `CREATE DATABASE `+name); err != nil {
			t.Fatalf("create test database: %v", err)
		}
	}

	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	u.Path = "/" + name

	conn, err := db.Open(ctx, u.String())
	if err != nil {
		t.Fatalf("connect to %s: %v", name, err)
	}
	t.Cleanup(func() { conn.Close() })

	if err := db.Migrate(ctx, conn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	for _, table := range []string{"documents", "users"} {
		if _, err := conn.ExecContext(ctx, fmt.Sprintf(`TRUNCATE %s RESTART IDENTITY CASCADE`, table)); err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}
	return conn
}
