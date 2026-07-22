package pgconf

import "strings"

// EscapeStringLiteral escapes a value for use inside a PostgreSQL configuration
// single-quoted string literal. Callers own writing the surrounding quotes.
func EscapeStringLiteral(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}
