package postgres

import (
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

const defaultURL = "postgres://postgres:postgres@localhost:5432/migrate"

func url() string {
	u := os.Getenv("POSTGRES_URL")
	if u == "" {
		return defaultURL
	}
	return u
}

// NewTest opens a Postgres connection pool for use in tests. If the connection
// cannot be established the test is skipped. The pool is closed
// automatically when the test finishes.
func NewTest(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), url())
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}
