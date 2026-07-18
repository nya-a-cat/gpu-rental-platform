package migrations

import "embed"

// Files contains the ordered SQL migrations used by the standalone migrate
// command and PostgreSQL integration tests.
//
//go:embed *.up.sql
var Files embed.FS
