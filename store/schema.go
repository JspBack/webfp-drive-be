package store

import "github.com/jackc/pgx/v5"

// schemaTables resolves table names against the configured schema.
//
// Queries are schema-qualified explicitly instead of relying on search_path:
// pooled Postgres endpoints (PgBouncer) silently ignore the search_path
// startup parameter, which leaves the session on "$user", public and makes
// every unqualified table reference fail with SQLSTATE 42P01.
type schemaTables string

func (s schemaTables) table(name string) string {
	return pgx.Identifier{string(s), name}.Sanitize()
}
