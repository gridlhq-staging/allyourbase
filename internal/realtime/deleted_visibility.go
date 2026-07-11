package realtime

import (
	"strconv"
	"strings"

	"github.com/allyourbase/ayb/internal/schema"
)

func deletedRecordPlaceholder(tbl *schema.Table, column string, index int) string {
	placeholder := "$" + strconv.Itoa(index)
	col := tbl.ColumnByName(column)
	if col == nil {
		return placeholder
	}
	typeName := strings.TrimSpace(col.TypeName)
	if typeName == "" || !isSafePostgresTypeName(typeName) {
		return placeholder
	}
	return placeholder + "::" + typeName
}

func isSafePostgresTypeName(typeName string) bool {
	for _, r := range typeName {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == ' ' || r == '[' || r == ']' || r == '(' || r == ')' || r == ',' || r == '.':
		default:
			return false
		}
	}
	return true
}
