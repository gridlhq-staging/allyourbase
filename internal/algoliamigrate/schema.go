// Package algoliamigrate.
package algoliamigrate

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/allyourbase/ayb/internal/migrate"
)

const objectIDField = "objectID"

// AnalyzeRecords infers one deterministic PostgreSQL table schema from records.
func AnalyzeRecords(records []Record) (Schema, error) {
	if len(records) == 0 {
		return Schema{}, errors.New("cannot infer schema from empty Algolia record set")
	}

	fields := map[string]*fieldInference{}
	for i, record := range records {
		if err := validateObjectID(record, i); err != nil {
			return Schema{}, err
		}
		for name, value := range record {
			field := fields[name]
			if field == nil {
				field = &fieldInference{name: name}
				fields[name] = field
			}
			if err := field.observe(value); err != nil {
				return Schema{}, fmt.Errorf("field %q in record %d: %w", name, i, err)
			}
		}
	}

	columns, err := buildColumns(fields, len(records))
	if err != nil {
		return Schema{}, err
	}
	return Schema{Columns: columns, RecordCount: len(records)}, nil
}

func validateObjectID(record Record, index int) error {
	raw, ok := record[objectIDField]
	if !ok {
		return fmt.Errorf("record %d is missing required objectID", index)
	}
	id, ok := raw.(string)
	if !ok || strings.TrimSpace(id) == "" {
		return fmt.Errorf("record %d has blank or non-string objectID", index)
	}
	return nil
}

func buildColumns(fields map[string]*fieldInference, recordCount int) ([]Column, error) {
	names := make([]string, 0, len(fields))
	for name := range fields {
		if name != objectIDField {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	names = append([]string{objectIDField}, names...)

	columns := make([]Column, 0, len(names))
	for _, name := range names {
		field := fields[name]
		if field == nil {
			return nil, fmt.Errorf("required field %q was not observed", name)
		}
		columnType, err := field.columnType()
		if err != nil {
			return nil, err
		}
		columns = append(columns, Column{
			Name:       name,
			Type:       columnType,
			Nullable:   name != objectIDField && field.observed < recordCount,
			PrimaryKey: name == objectIDField,
		})
	}
	return columns, nil
}

type fieldInference struct {
	name     string
	kind     inferredKind
	observed int
}

type inferredKind int

const (
	inferredUnknown inferredKind = iota
	inferredText
	inferredInteger
	inferredDouble
	inferredBoolean
	inferredJSONB
)

func (f *fieldInference) observe(value any) error {
	if value == nil {
		return nil
	}
	f.observed++
	return f.merge(kindForValue(value))
}

func (f *fieldInference) merge(next inferredKind) error {
	if f.kind == inferredUnknown {
		f.kind = next
		return nil
	}
	if f.kind == next {
		return nil
	}
	if (f.kind == inferredInteger && next == inferredDouble) || (f.kind == inferredDouble && next == inferredInteger) {
		f.kind = inferredDouble
		return nil
	}
	f.kind = inferredJSONB
	return nil
}

func (f *fieldInference) columnType() (ColumnType, error) {
	switch f.kind {
	case inferredText:
		return ColumnTypeText, nil
	case inferredInteger:
		return ColumnTypeInteger, nil
	case inferredDouble:
		return ColumnTypeDouble, nil
	case inferredBoolean:
		return ColumnTypeBoolean, nil
	case inferredJSONB:
		return ColumnTypeJSONB, nil
	default:
		return "", fmt.Errorf("field %q has no non-null values", f.name)
	}
}

func kindForValue(value any) inferredKind {
	switch v := value.(type) {
	case string:
		return inferredText
	case bool:
		return inferredBoolean
	case json.Number:
		if _, err := strconv.ParseInt(v.String(), 10, 64); err == nil {
			return inferredInteger
		}
		return inferredDouble
	case float64, float32:
		return inferredDouble
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32:
		return inferredInteger
	case []any, map[string]any:
		return inferredJSONB
	default:
		return inferredJSONB
	}
}

func buildCreateTableSQL(schema Schema, schemaName, tableName string) string {
	lines := []string{"CREATE TABLE " + migrate.QuoteQualifiedTable(schemaName, tableName) + " ("}
	for i, column := range schema.Columns {
		line := "  " + migrate.QuoteIdent(column.Name) + " " + string(column.Type)
		if column.PrimaryKey {
			line += " PRIMARY KEY"
		} else if !column.Nullable {
			line += " NOT NULL"
		}
		if i < len(schema.Columns)-1 {
			line += ","
		}
		lines = append(lines, line)
	}
	lines = append(lines, ");")
	return strings.Join(lines, "\n")
}
