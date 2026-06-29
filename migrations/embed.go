// Package migrations embeds the SQL migration files so they ship inside the
// binary. This keeps deployments to a single artifact — there are no loose .sql
// files to copy alongside the executable.
package migrations

import "embed"

// FS holds every .sql migration. Files are applied in lexical order of their
// "*.up.sql" names, so the numeric prefix (0001_, 0002_, ...) defines order.
//
//go:embed *.sql
var FS embed.FS
