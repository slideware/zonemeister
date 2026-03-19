package sqlite

import (
	"database/sql"
	"io/fs"

	"zonemeister/internal/dbmigrate"
)

// RunMigrations executes all pending SQLite migrations against the database.
func RunMigrations(db *sql.DB, migrationsFS fs.FS) error {
	return dbmigrate.RunMigrations(db, migrationsFS, dbmigrate.SQLitePlaceholder)
}
