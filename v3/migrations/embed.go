// Package migrations embeds the ordered SQL schema migrations for Setagaya v3.
// Files are named NNNN_name.sql and applied in lexical order.
package migrations

import "embed"

// FS holds the embedded *.sql migration files.
//
//go:embed *.sql
var FS embed.FS
