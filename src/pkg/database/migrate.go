package database

import (
	"database/sql"
	"embed"
	"fmt"
	"log"
	"sort"
	"strings"
)

//go:embed migrations/*.up.sql
var migrationsFS embed.FS

// Migrate runs all embedded .up.sql files in order against the given DB.
// Each migration is idempotent (IF NOT EXISTS / ADD COLUMN IF NOT EXISTS)
// so it's safe to run on every boot.
func Migrate(db *sql.DB) error {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	// Sort by filename to ensure 001, 002, 003… order.
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".up.sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, name := range files {
		data, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		if _, err := db.Exec(string(data)); err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		log.Printf("[Migrate] %s applied", name)
	}

	log.Printf("[Migrate] %d migration(s) applied", len(files))
	return nil
}
