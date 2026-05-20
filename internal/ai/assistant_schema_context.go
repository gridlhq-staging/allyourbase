package ai

import (
	"fmt"
	"sort"
	"strings"

	"github.com/allyourbase/ayb/internal/schema"
)

// CompactSchemaContext renders a deterministic bounded summary of schema metadata.
func CompactSchemaContext(cache *schema.SchemaCache, maxChars int) string {
	if cache == nil {
		return "schema: unavailable"
	}
	if maxChars <= 0 {
		maxChars = assistantSchemaContextMaxChars
	}

	var builder strings.Builder
	capabilities := schemaCapabilities(cache)
	builder.WriteString("capabilities: ")
	builder.WriteString(strings.Join(capabilities, ", "))
	builder.WriteString("\n")

	if appendSchemaTables(&builder, cache, maxChars) {
		return trimWithEllipsis(builder.String(), maxChars)
	}

	if appendSchemaFunctions(&builder, cache, maxChars) {
		return trimWithEllipsis(builder.String(), maxChars)
	}

	return trimWithEllipsis(builder.String(), maxChars)
}

func schemaCapabilities(cache *schema.SchemaCache) []string {
	capabilities := []string{"postgres"}
	if cache.HasPgVector {
		capabilities = append(capabilities, "pgvector")
	}
	if cache.HasPostGIS {
		capabilities = append(capabilities, "postgis")
	}
	return capabilities
}

func appendSchemaTables(builder *strings.Builder, cache *schema.SchemaCache, maxChars int) bool {
	tableKeys := make([]string, 0, len(cache.Tables))
	for key := range cache.Tables {
		tableKeys = append(tableKeys, key)
	}
	sort.Strings(tableKeys)
	for _, key := range tableKeys {
		table := cache.Tables[key]
		if table == nil {
			continue
		}
		for _, chunk := range schemaTableChunks(table) {
			if appendWithBudget(builder, chunk, maxChars) {
				return true
			}
		}
	}
	return false
}

func appendSchemaFunctions(builder *strings.Builder, cache *schema.SchemaCache, maxChars int) bool {
	functionKeys := make([]string, 0, len(cache.Functions))
	for key := range cache.Functions {
		functionKeys = append(functionKeys, key)
	}
	sort.Strings(functionKeys)
	for _, key := range functionKeys {
		fn := cache.Functions[key]
		if fn == nil {
			continue
		}
		if appendWithBudget(builder, fmt.Sprintf("function %s.%s returns %s\n", fn.Schema, fn.Name, fn.ReturnType), maxChars) {
			return true
		}
	}
	return false
}

func schemaTableChunks(table *schema.Table) []string {
	chunks := []string{
		fmt.Sprintf("table %s.%s (%s) pk=[%s] RLS=%t\n", table.Schema, table.Name, table.Kind, strings.Join(table.PrimaryKey, ","), table.RLSEnabled),
	}
	if columnChunk := schemaColumnsChunk(table.Columns); columnChunk != "" {
		chunks = append(chunks, columnChunk)
	}
	if fkChunk := schemaForeignKeysChunk(table.ForeignKeys); fkChunk != "" {
		chunks = append(chunks, fkChunk)
	}
	if indexChunk := schemaIndexesChunk(table.Indexes); indexChunk != "" {
		chunks = append(chunks, indexChunk)
	}
	if policyChunk := schemaPoliciesChunk(table.RLSPolicies); policyChunk != "" {
		chunks = append(chunks, policyChunk)
	}
	return chunks
}

func schemaColumnsChunk(columns []*schema.Column) string {
	columnParts := make([]string, 0, len(columns))
	for _, col := range columns {
		if col == nil {
			continue
		}
		columnParts = append(columnParts, fmt.Sprintf("%s:%s", col.Name, col.TypeName))
	}
	if len(columnParts) == 0 {
		return ""
	}
	return "  columns " + strings.Join(columnParts, ", ") + "\n"
}

func schemaForeignKeysChunk(foreignKeys []*schema.ForeignKey) string {
	fkParts := make([]string, 0, len(foreignKeys))
	for _, fk := range foreignKeys {
		if fk == nil {
			continue
		}
		fkParts = append(fkParts, fmt.Sprintf("%s->%s.%s(%s)", strings.Join(fk.Columns, ","), fk.ReferencedSchema, fk.ReferencedTable, strings.Join(fk.ReferencedColumns, ",")))
	}
	sort.Strings(fkParts)
	if len(fkParts) == 0 {
		return ""
	}
	return "  fks " + strings.Join(fkParts, "; ") + "\n"
}

func schemaIndexesChunk(indexes []*schema.Index) string {
	idxParts := make([]string, 0, len(indexes))
	for _, idx := range indexes {
		if idx == nil {
			continue
		}
		idxParts = append(idxParts, idx.Name+"("+idx.Method+")")
	}
	sort.Strings(idxParts)
	if len(idxParts) == 0 {
		return ""
	}
	return "  indexes " + strings.Join(idxParts, ", ") + "\n"
}

func schemaPoliciesChunk(policies []*schema.RLSPolicy) string {
	policyParts := make([]string, 0, len(policies))
	for _, policy := range policies {
		if policy == nil {
			continue
		}
		policyParts = append(policyParts, fmt.Sprintf("%s:%s", policy.Name, policy.Command))
	}
	sort.Strings(policyParts)
	if len(policyParts) == 0 {
		return ""
	}
	return "  RLS " + strings.Join(policyParts, ", ") + "\n"
}

func appendWithBudget(builder *strings.Builder, chunk string, maxChars int) bool {
	if builder.Len()+len(chunk) > maxChars {
		return true
	}
	builder.WriteString(chunk)
	return false
}

func trimWithEllipsis(input string, maxChars int) string {
	if maxChars <= 0 || len(input) <= maxChars {
		return strings.TrimSpace(input)
	}
	if maxChars <= 3 {
		return input[:maxChars]
	}
	return strings.TrimSpace(input[:maxChars-3]) + "..."
}
