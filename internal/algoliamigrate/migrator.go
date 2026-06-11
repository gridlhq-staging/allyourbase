// Package algoliamigrate.
package algoliamigrate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/allyourbase/ayb/internal/searchsynonyms"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	defaultSchemaName     = "public"
	defaultTableName      = "algolia_records"
	maxPostgresParameters = 65535
)

// Migrator executes planned Algolia record imports inside one database transaction.
type Migrator struct {
	db       *sql.DB
	opts     ImportOptions
	progress migrate.ProgressReporter
}

// NewMigrator returns a transactional importer for already-browsed records.
func NewMigrator(db *sql.DB, opts ImportOptions, reporter migrate.ProgressReporter) *Migrator {
	if reporter == nil {
		reporter = migrate.NopReporter{}
	}
	return &Migrator{db: db, opts: opts, progress: reporter}
}

// PlanImport performs pure record analysis and deterministic PostgreSQL planning.
func PlanImport(records []Record, opts ImportOptions) (*ImportPlan, error) {
	schemaName := normalizeSchemaName(opts.TargetSchema)
	tableName, err := normalizeTargetTableName(opts.TargetTable)
	if err != nil {
		return nil, err
	}

	schema, err := AnalyzeRecords(records)
	if err != nil {
		return nil, err
	}
	synonymPlan := SynonymPlan{}
	if opts.Synonyms != nil {
		synonymPlan = MapAlgoliaSynonyms(*opts.Synonyms)
	}

	target := buildTargetPlan(schema, schemaName, tableName)
	return &ImportPlan{
		Source:   SourceFacts{RecordCount: schema.RecordCount},
		Schema:   schema,
		Target:   target,
		DryRun:   DryRunStats{TablesPlanned: 1, RecordsPlanned: schema.RecordCount},
		Synonyms: synonymPlan,
	}, nil
}

// ImportRecords creates the planned target table and inserts all browsed records.
func (m *Migrator) ImportRecords(ctx context.Context, records []Record) (*ImportStats, error) {
	if m.db == nil {
		return nil, errors.New("target database is required")
	}
	plan, err := PlanImport(records, m.opts)
	if err != nil {
		return nil, err
	}

	if err := ensureTargetTableAbsent(ctx, m.db, plan.Target); err != nil {
		return nil, err
	}
	if m.opts.SynonymClient != nil {
		input, err := m.opts.SynonymClient.SearchSynonyms(ctx)
		if err != nil {
			return nil, err
		}
		if input != nil {
			plan.Synonyms = MapAlgoliaSynonyms(*input)
		}
	}

	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning algolia import transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx, plan.Target.CreateTableSQL); err != nil {
		return nil, fmt.Errorf("creating target table %s: %w", plan.Target.TableName, err)
	}

	inserted, err := m.insertRecords(ctx, tx, plan.Target, records)
	if err != nil {
		return nil, err
	}

	if err := m.replaceSynonymGroups(ctx, tx, plan); err != nil {
		return nil, err
	}

	stats := &ImportStats{Tables: 1, Records: inserted, DryRun: m.opts.DryRun, Synonyms: plan.Synonyms.Stats}
	if m.opts.DryRun {
		return stats, nil
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing algolia import transaction: %w", err)
	}
	return stats, nil
}

func (m *Migrator) replaceSynonymGroups(ctx context.Context, tx *sql.Tx, plan *ImportPlan) error {
	if len(plan.Synonyms.Groups) == 0 {
		return nil
	}
	if err := searchsynonyms.ReplaceGroupsSQLTx(ctx, tx, plan.Target.SchemaName, plan.Target.TableName, plan.Synonyms.Groups); err != nil {
		return fmt.Errorf("replacing search synonyms for %s.%s: %w", plan.Target.SchemaName, plan.Target.TableName, err)
	}
	return nil
}

func ensureTargetTableAbsent(ctx context.Context, db *sql.DB, target TargetPlan) error {
	rows, err := db.QueryContext(ctx, target.PreflightSQL)
	if err == nil {
		rows.Close()
		return fmt.Errorf("target table %s already exists", target.TableName)
	}
	if isUndefinedTable(err) {
		return nil
	}
	return fmt.Errorf("checking target table %s: %w", target.TableName, err)
}

func isUndefinedTable(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "42P01"
}

func (m *Migrator) insertRecords(ctx context.Context, tx *sql.Tx, target TargetPlan, records []Record) (int, error) {
	phase := migrate.Phase{Name: "Records", Index: 1, Total: 1}
	m.progress.StartPhase(phase, len(records))
	start := time.Now()

	inserted := 0
	batchSize := effectiveBatchSize(normalizeBatchSize(m.opts.BatchSize), len(target.Columns))
	for startIndex := 0; startIndex < len(records); startIndex += batchSize {
		endIndex := startIndex + batchSize
		if endIndex > len(records) {
			endIndex = len(records)
		}

		batch, err := buildInsertBatch(target, records[startIndex:endIndex])
		if err != nil {
			return inserted, err
		}
		result, err := tx.ExecContext(ctx, batch.SQL, batch.Values...)
		if err != nil {
			return inserted, fmt.Errorf("inserting records %d-%d into %s: %w", startIndex, endIndex-1, target.TableName, err)
		}
		if rows, _ := result.RowsAffected(); rows > 0 {
			inserted += int(rows)
		}
		m.progress.Progress(phase, inserted, len(records))
	}
	m.progress.CompletePhase(phase, inserted, time.Since(start))
	return inserted, nil
}

func normalizeBatchSize(batchSize int) int {
	if batchSize <= 0 {
		return 500
	}
	return batchSize
}

func effectiveBatchSize(batchSize, columnCount int) int {
	if columnCount <= 0 {
		return 1
	}
	maxRows := maxPostgresParameters / columnCount
	if maxRows < 1 {
		return 1
	}
	if batchSize > maxRows {
		return maxRows
	}
	return batchSize
}

func normalizeSchemaName(schema string) string {
	schema = strings.TrimSpace(schema)
	if schema == "" {
		return defaultSchemaName
	}
	return schema
}

func normalizeTargetTableName(table string) (string, error) {
	sanitized := migrate.SanitizeIdentifier(strings.TrimSpace(table))
	sanitized = collapseIdentifierUnderscores(sanitized)
	if strings.HasPrefix(sanitized, "_ayb_") {
		return "", fmt.Errorf("target table %q uses reserved _ayb_ prefix", sanitized)
	}
	sanitized = strings.Trim(sanitized, "_")
	if sanitized == "" || sanitized == "id" {
		sanitized = defaultTableName
	}
	return sanitized, nil
}

func collapseIdentifierUnderscores(identifier string) string {
	var b strings.Builder
	lastUnderscore := false
	for _, r := range identifier {
		if r == '_' {
			if !lastUnderscore {
				b.WriteRune(r)
			}
			lastUnderscore = true
			continue
		}
		b.WriteRune(r)
		lastUnderscore = false
	}
	return b.String()
}

func buildTargetPlan(schema Schema, schemaName, tableName string) TargetPlan {
	return TargetPlan{
		SchemaName:     schemaName,
		TableName:      tableName,
		Columns:        append([]Column(nil), schema.Columns...),
		CreateTableSQL: buildCreateTableSQL(schema, schemaName, tableName),
		PreflightSQL:   buildPreflightSQL(schemaName, tableName),
		InsertSQL:      buildInsertSQL(schema.Columns, schemaName, tableName),
	}
}

func buildPreflightSQL(schemaName, tableName string) string {
	return "SELECT 1 FROM " + migrate.QuoteQualifiedTable(schemaName, tableName) + " LIMIT 0"
}

func buildInsertSQL(columns []Column, schemaName, tableName string) string {
	return buildBatchInsertSQL(columns, schemaName, tableName, 1)
}

func buildBatchInsertSQL(columns []Column, schemaName, tableName string, recordCount int) string {
	quotedColumns := make([]string, len(columns))
	for i, column := range columns {
		quotedColumns[i] = migrate.QuoteIdent(column.Name)
	}

	placeholderRows := make([]string, recordCount)
	placeholder := 1
	for row := range placeholderRows {
		placeholders := make([]string, len(columns))
		for col := range columns {
			placeholders[col] = fmt.Sprintf("$%d", placeholder)
			placeholder++
		}
		placeholderRows[row] = "(" + strings.Join(placeholders, ", ") + ")"
	}

	return "INSERT INTO " + migrate.QuoteQualifiedTable(schemaName, tableName) +
		" (" + strings.Join(quotedColumns, ", ") + ") VALUES " + strings.Join(placeholderRows, ", ")
}

type insertBatch struct {
	SQL    string
	Values []any
}

func buildInsertBatch(target TargetPlan, records []Record) (insertBatch, error) {
	if len(records) == 0 {
		return insertBatch{}, errors.New("cannot build insert batch without records")
	}

	values := make([]any, 0, len(records)*len(target.Columns))
	for i, record := range records {
		rowValues, err := recordValues(record, target.Columns)
		if err != nil {
			return insertBatch{}, fmt.Errorf("building insert values for batch record %d: %w", i, err)
		}
		values = append(values, rowValues...)
	}

	return insertBatch{
		SQL:    buildBatchInsertSQL(target.Columns, target.SchemaName, target.TableName, len(records)),
		Values: values,
	}, nil
}

func recordValues(record Record, columns []Column) ([]any, error) {
	values := make([]any, len(columns))
	for i, column := range columns {
		value, ok := record[column.Name]
		if !ok || value == nil {
			values[i] = nil
			continue
		}
		converted, err := convertValue(value, column)
		if err != nil {
			return nil, err
		}
		values[i] = converted
	}
	return values, nil
}

func convertValue(value any, column Column) (any, error) {
	switch column.Type {
	case ColumnTypeText:
		return value, nil
	case ColumnTypeInteger:
		return integerValue(value, column.Name)
	case ColumnTypeDouble:
		return doubleValue(value, column.Name)
	case ColumnTypeBoolean:
		return value, nil
	case ColumnTypeJSONB:
		raw, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("marshaling %s as jsonb: %w", column.Name, err)
		}
		return string(raw), nil
	default:
		return nil, fmt.Errorf("unsupported column type %s for %s", column.Type, column.Name)
	}
}

func integerValue(value any, columnName string) (any, error) {
	switch v := value.(type) {
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return nil, fmt.Errorf("converting %s to bigint: %w", columnName, err)
		}
		return n, nil
	case int, int8, int16, int32, int64:
		return v, nil
	default:
		return nil, fmt.Errorf("field %s is not an integer value", columnName)
	}
}

func doubleValue(value any, columnName string) (any, error) {
	switch v := value.(type) {
	case json.Number:
		n, err := v.Float64()
		if err != nil {
			return nil, fmt.Errorf("converting %s to double precision: %w", columnName, err)
		}
		return n, nil
	case int:
		return float64(v), nil
	case int8:
		return float64(v), nil
	case int16:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case float32:
		return float64(v), nil
	case float64:
		return v, nil
	default:
		return nil, fmt.Errorf("field %s is not a numeric value", columnName)
	}
}

// BuildAnalysisReport adapts the package plan into the shared migration report.
func BuildAnalysisReport(plan *ImportPlan) *migrate.AnalysisReport {
	if plan == nil {
		return &migrate.AnalysisReport{SourceType: "Algolia"}
	}
	return &migrate.AnalysisReport{
		SourceType:    "Algolia",
		Tables:        plan.DryRun.TablesPlanned,
		Records:       plan.Source.RecordCount,
		SynonymGroups: plan.Synonyms.Stats.SupportedGroups,
		Warnings:      synonymWarningMessages(plan.Synonyms.Stats),
	}
}

// BuildValidationSummary compares source analysis with import stats.
func BuildValidationSummary(report *migrate.AnalysisReport, stats *ImportStats) *migrate.ValidationSummary {
	summary := &migrate.ValidationSummary{
		SourceLabel: "Algolia (source)",
		TargetLabel: "AYB (target)",
	}
	if report == nil {
		report = &migrate.AnalysisReport{}
	}
	if stats == nil {
		stats = &ImportStats{}
	}
	summary.Rows = append(summary.Rows,
		migrate.ValidationRow{Label: "Tables", SourceCount: report.Tables, TargetCount: stats.Tables},
		migrate.ValidationRow{Label: "Records", SourceCount: report.Records, TargetCount: stats.Records},
	)
	if report.SynonymGroups > 0 || stats.Synonyms.SupportedGroups > 0 || hasSynonymSkips(stats.Synonyms) {
		summary.Rows = append(summary.Rows,
			migrate.ValidationRow{Label: "Synonym groups", SourceCount: report.SynonymGroups, TargetCount: stats.Synonyms.SupportedGroups})
	}
	for _, row := range summary.Rows {
		if row.SourceCount != row.TargetCount {
			summary.Warnings = append(summary.Warnings,
				fmt.Sprintf("%s count mismatch: source=%d target=%d", row.Label, row.SourceCount, row.TargetCount))
		}
	}
	if stats.Skipped > 0 {
		summary.Warnings = append(summary.Warnings,
			fmt.Sprintf("%d records skipped during import", stats.Skipped))
	}
	if len(stats.Errors) > 0 {
		summary.Warnings = append(summary.Warnings,
			fmt.Sprintf("%d errors occurred during import", len(stats.Errors)))
	}
	summary.Warnings = append(summary.Warnings, synonymWarningMessages(stats.Synonyms)...)
	return summary
}

func hasSynonymSkips(stats SynonymStats) bool {
	return len(stats.SkippedUnsupportedTypes) > 0 ||
		stats.SkippedInvalidGroups > 0 ||
		stats.SkippedSettingsACL > 0 ||
		stats.SkippedMalformedHits > 0
}

// CheckRecordParity uses live browse credentials when present and otherwise
// compares fixture-backed source counts so normal tests never need secrets.
func CheckRecordParity(ctx context.Context, cfg BrowseConfig, fixture []Record, targetRecords int) (ParityResult, error) {
	source := "fixture"
	sourceRecords := len(fixture)
	if hasLiveBrowseCredentials(cfg) {
		client, err := NewBrowseClient(cfg)
		if err != nil {
			return ParityResult{}, err
		}
		result, err := client.Browse(ctx)
		if err != nil {
			return ParityResult{}, err
		}
		source = "live"
		sourceRecords = len(result.Records)
	}
	return ParityResult{
		Source:        source,
		SourceRecords: sourceRecords,
		TargetRecords: targetRecords,
		Match:         sourceRecords == targetRecords,
	}, nil
}

func hasLiveBrowseCredentials(cfg BrowseConfig) bool {
	return strings.TrimSpace(cfg.AppID) != "" &&
		strings.TrimSpace(cfg.APIKey) != "" &&
		strings.TrimSpace(cfg.IndexName) != ""
}
