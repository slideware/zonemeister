package dbmigrate

import (
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strconv"
	"strings"
)

// RunMigrations executes all pending migrations against the database.
// The provided fs.FS should contain SQL files named NNN_description.sql.
// The placeholder function returns the appropriate parameter placeholder
// for the given 1-based index (e.g., "?" for SQLite, "$1" for PostgreSQL).
func RunMigrations(db *sql.DB, migrationsFS fs.FS, placeholder func(int) string) error {
	// Create schema_migrations table if it doesn't exist.
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY)`); err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	// Get current version.
	var currentVersion int
	row := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`)
	if err := row.Scan(&currentVersion); err != nil {
		return fmt.Errorf("get current migration version: %w", err)
	}

	// Read migration files.
	entries, err := fs.ReadDir(migrationsFS, ".")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	type migration struct {
		version int
		name    string
	}

	var migrations []migration
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		parts := strings.SplitN(entry.Name(), "_", 2)
		if len(parts) < 2 {
			continue
		}
		version, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		if version > currentVersion {
			migrations = append(migrations, migration{version: version, name: entry.Name()})
		}
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].version < migrations[j].version
	})

	for _, m := range migrations {
		slog.Info("running migration", "version", m.version, "file", m.name)

		content, err := fs.ReadFile(migrationsFS, m.name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", m.name, err)
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin transaction for migration %s: %w", m.name, err)
		}

		if _, err := tx.Exec(string(content)); err != nil {
			tx.Rollback()
			return fmt.Errorf("execute migration %s: %w", m.name, err)
		}

		if _, err := tx.Exec(
			fmt.Sprintf(`INSERT INTO schema_migrations (version) VALUES (%s)`, placeholder(1)),
			m.version,
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration version %d: %w", m.version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", m.name, err)
		}
	}

	return nil
}

// SQLitePlaceholder returns "?" for any index (SQLite style).
func SQLitePlaceholder(_ int) string {
	return "?"
}

// PostgresPlaceholder returns "$1", "$2", etc. (PostgreSQL style).
func PostgresPlaceholder(index int) string {
	return fmt.Sprintf("$%d", index)
}
