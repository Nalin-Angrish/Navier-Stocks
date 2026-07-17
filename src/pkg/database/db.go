// Package database provides PostgreSQL connectivity and store abstractions
// for the three core tables: positions (live portfolio), trade_log (execution
// audit trail), and sentiment_scores (LLM output used by the Risk Manager).
//
// Each store type wraps a *sql.DB and exposes focused CRUD methods.  A single
// database connection is shared across all stores via composition.
package database

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/lib/pq"
)

// Connect opens a PostgreSQL connection using environment variables:
//
//	PG_HOST, PG_PORT, PG_USER, PG_PASSWORD, PG_DATABASE, PG_SSLMODE.
//
// Only PG_PASSWORD has no default; the rest fall back to localhost / navier /
// navier_stocks / disable respectively.  The caller should verify connectivity
// with db.Ping() before use.
func Connect() (*sql.DB, error) {
	host := envOrDefault("PG_HOST", "localhost")
	port := envOrDefault("PG_PORT", "5432")
	user := envOrDefault("PG_USER", "navier")
	password := os.Getenv("PG_PASSWORD")
	dbname := envOrDefault("PG_DATABASE", "navier_stocks")
	sslmode := envOrDefault("PG_SSLMODE", "disable")

	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, password, dbname, sslmode,
	)
	return sql.Open("postgres", dsn)
}

// envOrDefault returns the environment variable value if set, otherwise the
// provided fallback string.
func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
