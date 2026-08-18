// Package migrations embeds the ordered SQL schema migrations for Honryu.
// Files are named NNNN_name.sql and applied in lexical order, one statement per
// file (see mysql.Migrate: the app connection does not enable multiStatements).
//
// This set is a fresh baseline. It replaces the migrations inherited from
// Shibuya, which carried v2-compatible table names (plan, collection,
// collection_plan, ...) and a v3_ prefix on the newer tables so that v2 and v3
// could share one database during a strangler cutover. Honryu migrates from
// Shibuya by importing JMX assets only -- no database is carried across -- so
// there is no compatibility to preserve and no data to migrate.
package migrations

import "embed"

// FS holds the embedded *.sql migration files.
//
//go:embed *.sql
var FS embed.FS
