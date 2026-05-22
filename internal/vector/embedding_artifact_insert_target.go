package vector

import (
	"errors"
	"fmt"
	"strings"
)

// extractInsertStatementForTable returns the full INSERT statement for the
// requested table name, terminated by the first top-level semicolon.
func extractInsertStatementForTable(sql, tableName string) (string, error) {
	searchFrom := 0
	for {
		insertIdx := indexOfKeywordAfter(sql, "INSERT", searchFrom)
		if insertIdx < 0 {
			return "", fmt.Errorf("no INSERT INTO %s statement found in seed SQL", tableName)
		}
		intoIdx := indexOfKeywordAfter(sql, "INTO", insertIdx+len("INSERT"))
		if intoIdx < 0 {
			return "", errors.New("INSERT clause missing INTO keyword")
		}

		i := intoIdx + len("INTO")
		for i < len(sql) && isSQLSpace(sql[i]) {
			i++
		}
		parts, next := scanQualifiedIdentifierParts(sql, i)
		if next <= i || len(parts) == 0 {
			searchFrom = i + 1
			continue
		}
		if matchesTargetTable(parts, tableName) {
			end := endOfSQLStatement(sql, insertIdx)
			return strings.TrimSpace(sql[insertIdx:end]), nil
		}
		searchFrom = next
	}
}

func scanQualifiedIdentifierParts(sql string, from int) ([]string, int) {
	parts := make([]string, 0, 2)
	i := from

	for {
		part, next := scanIdentifierPart(sql, i)
		if next <= i {
			break
		}
		parts = append(parts, part)
		i = next

		spaces := i
		for spaces < len(sql) && isSQLSpace(sql[spaces]) {
			spaces++
		}
		if spaces >= len(sql) || sql[spaces] != '.' {
			i = spaces
			break
		}

		spaces++
		for spaces < len(sql) && isSQLSpace(sql[spaces]) {
			spaces++
		}
		i = spaces
	}
	return parts, i
}

func scanIdentifierPart(sql string, from int) (string, int) {
	if from >= len(sql) {
		return "", from
	}
	if sql[from] == '"' {
		i := from + 1
		for i < len(sql) {
			if sql[i] == '"' {
				if i+1 < len(sql) && sql[i+1] == '"' {
					i += 2
					continue
				}
				return sql[from : i+1], i + 1
			}
			i++
		}
		return "", from
	}
	i := from
	for i < len(sql) && isIdentByte(sql[i]) {
		i++
	}
	if i == from {
		return "", from
	}
	return sql[from:i], i
}

func endOfSQLStatement(s string, from int) int {
	i := from
	for i < len(s) {
		switch s[i] {
		case '\'':
			i = skipSQLString(s, i)
			continue
		case ';':
			return i + 1
		}
		i++
	}
	return len(s)
}

func isSQLSpace(b byte) bool {
	return b == ' ' || b == '\n' || b == '\t' || b == '\r'
}

func normalizeSQLIdentifier(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	parts := splitQualifiedIdentifier(trimmed)
	for i := range parts {
		parts[i] = normalizeIdentifierPart(parts[i])
	}
	return strings.Join(parts, ".")
}

func splitQualifiedIdentifier(identifier string) []string {
	parts := make([]string, 0, 2)
	start := 0
	inQuotes := false
	i := 0
	for i < len(identifier) {
		switch identifier[i] {
		case '"':
			if inQuotes && i+1 < len(identifier) && identifier[i+1] == '"' {
				i += 2
				continue
			}
			inQuotes = !inQuotes
		case '.':
			if !inQuotes {
				parts = append(parts, identifier[start:i])
				start = i + 1
			}
		}
		i++
	}
	parts = append(parts, identifier[start:])
	return parts
}

func normalizeIdentifierPart(part string) string {
	trimmed := strings.TrimSpace(part)
	if len(trimmed) >= 2 && trimmed[0] == '"' && trimmed[len(trimmed)-1] == '"' {
		trimmed = trimmed[1 : len(trimmed)-1]
		trimmed = strings.ReplaceAll(trimmed, `""`, `"`)
		return trimmed
	}
	return strings.ToLower(strings.TrimSpace(trimmed))
}

func matchesTargetTable(parts []string, target string) bool {
	if len(parts) == 0 {
		return false
	}
	normalizedTarget := strings.ToLower(strings.TrimSpace(target))
	lastPart := normalizeIdentifierPart(parts[len(parts)-1])
	return lastPart == normalizedTarget
}
