package postgres

import "embed"

// FS contains all PostgreSQL migration SQL files.
//
//go:embed *.sql
var FS embed.FS
