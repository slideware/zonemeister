package migrations

import (
	"embed"
	"io/fs"
)

//go:embed sqlite/*.sql
var sqliteFS embed.FS

//go:embed postgres/*.sql
var postgresFS embed.FS

// SQLiteFS returns the SQLite migration files.
func SQLiteFS() fs.FS {
	sub, _ := fs.Sub(sqliteFS, "sqlite")
	return sub
}

// PostgresFS returns the PostgreSQL migration files.
func PostgresFS() fs.FS {
	sub, _ := fs.Sub(postgresFS, "postgres")
	return sub
}
