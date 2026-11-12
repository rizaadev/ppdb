package sqlfiles

import "embed"

// MigrationsFS holds all ordered PostgreSQL schema migrations under sql/migrations.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

