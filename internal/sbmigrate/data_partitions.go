package sbmigrate

import (
	"context"
	"database/sql"
	"fmt"
	"sort"

	"github.com/allyourbase/ayb/internal/sqlutil"
)

func introspectTablePartitioning(ctx context.Context, db *sql.DB, table *TableInfo) error {
	err := db.QueryRowContext(ctx, `
		SELECT
			COALESCE(pg_get_partkeydef(c.oid), ''),
			COALESCE(CASE WHEN c.relispartition THEN parent_ns.nspname END, ''),
			COALESCE(CASE WHEN c.relispartition THEN parent.relname END, ''),
			COALESCE(CASE WHEN c.relispartition THEN pg_get_expr(c.relpartbound, c.oid) END, '')
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		LEFT JOIN pg_inherits i ON i.inhrelid = c.oid
		LEFT JOIN pg_class parent ON parent.oid = i.inhparent
		LEFT JOIN pg_namespace parent_ns ON parent_ns.oid = parent.relnamespace
		WHERE n.nspname = $1
		  AND c.relname = $2
		  AND c.relkind IN ('r', 'p')
	`, table.SchemaName, table.Name).Scan(
		&table.PartitionKey,
		&table.PartitionParentSchema,
		&table.PartitionParentName,
		&table.PartitionBound,
	)
	if err != nil {
		return fmt.Errorf("querying partition metadata: %w", err)
	}
	return nil
}

func sortTablesForSchemaCreation(tables []TableInfo) {
	sort.SliceStable(tables, func(i, j int) bool {
		left := tables[i]
		right := tables[j]
		if left.TableKey() == partitionParentKey(right) {
			return true
		}
		if right.TableKey() == partitionParentKey(left) {
			return false
		}
		return left.QualifiedName() < right.QualifiedName()
	})
}

func partitionParentKey(table TableInfo) string {
	if table.PartitionParentSchema == "" || table.PartitionParentName == "" {
		return ""
	}
	return tableKey(table.PartitionParentSchema, table.PartitionParentName)
}

func createPartitionSQL(table TableInfo) string {
	parent := sqlutil.QuoteOptionallyQualifiedName(table.PartitionParentSchema, table.PartitionParentName)
	return fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s PARTITION OF %s %s;",
		table.quotedName(),
		parent,
		table.PartitionBound,
	)
}
