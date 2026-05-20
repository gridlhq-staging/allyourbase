package pbmigrate

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/allyourbase/ayb/internal/sqlutil"
)

// migrateData imports non-system, non-auth, non-view collection records in batches.
func (m *Migrator) migrateData(ctx context.Context, tx *sql.Tx, collections []PBCollection, phase migrate.Phase) error {
	totalCompleted := 0

	for _, coll := range collections {
		// Skip system, auth, and view collections.
		if coll.System || coll.Type == "auth" || coll.Type == "view" {
			continue
		}

		count, err := m.reader.CountRecords(coll.Name)
		if err != nil {
			return fmt.Errorf("failed to count records in %s: %w", coll.Name, err)
		}

		if count == 0 {
			if m.verbose {
				fmt.Fprintf(m.output, "  %s: 0 records (skipping)\n", coll.Name)
			}
			continue
		}

		records, err := m.reader.ReadRecords(coll.Name, coll.Schema)
		if err != nil {
			return fmt.Errorf("failed to read records from %s: %w", coll.Name, err)
		}

		const batchSize = 1000
		for i := 0; i < len(records); i += batchSize {
			end := i + batchSize
			if end > len(records) {
				end = len(records)
			}

			batch := records[i:end]
			if !m.opts.DryRun {
				if err := m.insertBatch(ctx, tx, coll.Name, coll.Schema, batch); err != nil {
					return fmt.Errorf("failed to insert batch into %s: %w", coll.Name, err)
				}
			}

			m.stats.Records += len(batch)
			totalCompleted += len(batch)
			m.progress.Progress(phase, totalCompleted, totalCompleted)
		}
	}

	return nil
}

// insertBatch inserts a slice of PBRecord rows into a PostgreSQL table, coercing
// SQLite booleans and multi-value fields to PostgreSQL-compatible values.
func (m *Migrator) insertBatch(ctx context.Context, tx *sql.Tx, tableName string, schema []PBField, records []PBRecord) error {
	if len(records) == 0 {
		return nil
	}

	columns := buildInsertColumns(records[0], schema)
	fieldTypes, fieldsByName := buildFieldTypeLookups(schema)

	for _, record := range records {
		placeholders := make([]string, len(columns))
		values := make([]interface{}, len(columns))

		for i, col := range columns {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
			values[i] = buildInsertValue(record, col, fieldTypes, fieldsByName)
		}

		query := fmt.Sprintf(
			"INSERT INTO %s (%s) VALUES (%s)",
			SanitizeIdentifier(tableName),
			sqlutil.QuoteIdentList(columns),
			strings.Join(placeholders, ", "),
		)

		if _, err := tx.ExecContext(ctx, query, values...); err != nil {
			return fmt.Errorf("failed to insert record %s: %w", record.ID, err)
		}
	}

	return nil
}

func buildInsertColumns(record PBRecord, schema []PBField) []string {
	columns := []string{"id"}
	if _, ok := record.Data["created"]; ok {
		columns = append(columns, "created")
	}
	if _, ok := record.Data["updated"]; ok {
		columns = append(columns, "updated")
	}

	for _, field := range schema {
		if field.System || containsString(columns, field.Name) {
			continue
		}
		columns = append(columns, field.Name)
	}

	return columns
}

func buildFieldTypeLookups(schema []PBField) (map[string]string, map[string]PBField) {
	fieldTypes := make(map[string]string, len(schema))
	fieldsByName := make(map[string]PBField, len(schema))
	for _, field := range schema {
		fieldTypes[field.Name] = field.Type
		fieldsByName[field.Name] = field
	}
	return fieldTypes, fieldsByName
}

func buildInsertValue(record PBRecord, column string, fieldTypes map[string]string, fieldsByName map[string]PBField) interface{} {
	switch column {
	case "id":
		return record.ID
	case "created", "updated":
		return record.Data[column]
	default:
		val := record.Data[column]
		if fieldTypes[column] == "bool" {
			val = coerceToBool(val)
		}
		if field, ok := fieldsByName[column]; ok && fieldExpectsTextArray(field) {
			val = coerceToTextArray(val)
		}
		return val
	}
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

// coerceToBool converts SQLite INTEGER values (1/0) to Go bool for PostgreSQL BOOLEAN columns.
// SQLite stores booleans as INTEGER; pgx doesn't auto-convert int64 -> bool.
func coerceToBool(val interface{}) interface{} {
	switch v := val.(type) {
	case bool:
		return v
	case int64:
		return v != 0
	case int:
		return v != 0
	case float64:
		return v != 0
	default:
		return val
	}
}

func fieldExpectsTextArray(field PBField) bool {
	switch field.Type {
	case "select", "file", "relation":
		return fieldMaxSelect(field) > 1
	default:
		return false
	}
}

// coerceToTextArray converts a value to a PostgreSQL text array ([]string) for
// columns that expect array types.
func coerceToTextArray(val interface{}) interface{} {
	switch v := val.(type) {
	case nil:
		return []string{}
	case string:
		if v == "" {
			return []string{}
		}
		var arr []string
		if err := json.Unmarshal([]byte(v), &arr); err == nil {
			return arr
		}
		return []string{v}
	case []byte:
		s := string(v)
		if s == "" {
			return []string{}
		}
		var arr []string
		if err := json.Unmarshal(v, &arr); err == nil {
			return arr
		}
		return []string{s}
	case []string:
		return v
	case []interface{}:
		arr := make([]string, 0, len(v))
		for _, item := range v {
			arr = append(arr, fmt.Sprint(item))
		}
		return arr
	default:
		return val
	}
}
