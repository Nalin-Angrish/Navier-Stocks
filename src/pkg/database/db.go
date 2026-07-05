package database

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/lib/pq"
)

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

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
