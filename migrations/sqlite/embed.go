package sqlite

import "embed"

// FS contains all SQLite migration SQL files.
//
//go:embed *.sql
var FS embed.FS
