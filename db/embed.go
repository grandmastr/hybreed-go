// Package db embeds the SQL migrations so the binary can self-migrate at startup
// (no external files needed in the container image).
package db

import "embed"

// MigrationsFS holds the golang-migrate up/down files under migrations/.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS
