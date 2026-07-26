// Package sbmigrate migrates auth users, OAuth identities, RLS policies,
// and data tables from a Supabase PostgreSQL database to AYB.
package sbmigrate

import (
	"encoding/json"
	"time"

	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/allyourbase/ayb/internal/sqlutil"
)

// SupabaseUser represents a user from Supabase's auth.users table.
type SupabaseUser struct {
	ID                string
	Email             string
	EncryptedPassword string     // bcrypt hash
	EmailConfirmedAt  *time.Time // non-nil = verified
	RawUserMetaData   map[string]any
	RawAppMetaData    map[string]any
	CreatedAt         time.Time
	UpdatedAt         time.Time
	IsAnonymous       bool
}

// SupabaseIdentity represents an OAuth identity from Supabase's auth.identities table.
type SupabaseIdentity struct {
	UserID       string
	Provider     string
	IdentityData map[string]any // JSONB with sub, email, name, full_name
	CreatedAt    time.Time
}

// RLSPolicy represents an existing RLS policy read from pg_catalog.
type RLSPolicy struct {
	PolicyName string
	TableName  string
	SchemaName string
	Command    string // SELECT, INSERT, UPDATE, DELETE, ALL
	Permissive bool
	UsingExpr  string
	CheckExpr  string
}

// TableInfo represents a source table's schema.
type TableInfo struct {
	SchemaName  string
	Name        string
	Columns     []ColumnInfo
	PrimaryKey  string // column name of the PK (empty if composite/none)
	ForeignKeys []ForeignKeyInfo
	Sequences   []SequenceInfo
	RowCount    int64
}

func (t TableInfo) QualifiedName() string {
	return schemaQualifiedName(t.SchemaName, t.Name)
}

func (t TableInfo) TableKey() string {
	return tableKey(t.SchemaName, t.Name)
}

func (t TableInfo) quotedName() string {
	return sqlutil.QuoteOptionallyQualifiedName(t.SchemaName, t.Name)
}

// ColumnInfo describes a single column in a table.
type ColumnInfo struct {
	Name         string
	DataType     string // PostgreSQL type name (e.g., "integer", "text", "uuid")
	IsNullable   bool
	DefaultValue string // empty string = no default
	OrdinalPos   int
}

// ForeignKeyInfo describes a foreign key constraint.
type ForeignKeyInfo struct {
	ConstraintName string
	ColumnName     string
	RefSchemaName  string
	RefTable       string
	RefColumn      string
}

// ViewInfo represents a source view.
type ViewInfo struct {
	SchemaName string
	Name       string
	Definition string // CREATE OR REPLACE VIEW ... AS ...
}

func (v ViewInfo) QualifiedName() string {
	return schemaQualifiedName(v.SchemaName, v.Name)
}

func (v ViewInfo) quotedName() string {
	return sqlutil.QuoteOptionallyQualifiedName(v.SchemaName, v.Name)
}

// SequenceInfo represents a source sequence.
type SequenceInfo struct {
	SchemaName      string
	Name            string
	TableSchemaName string
	TableName       string // owning table
	ColumnName      string // owning column
}

// SkippedTableReasons tracks skipped tables by the internal collision-safe key.
type SkippedTableReasons map[string]string

// SkippedTableReport is the stable JSON shape for skipped-table reporting.
type SkippedTableReport struct {
	SchemaName string `json:"schema"`
	TableName  string `json:"table"`
	Reason     string `json:"reason"`
}

func (r SkippedTableReport) QualifiedName() string {
	return schemaQualifiedName(r.SchemaName, r.TableName)
}

func (r SkippedTableReasons) MarshalJSON() ([]byte, error) {
	return json.Marshal(sortedSkippedTableReports(r))
}

func (r *SkippedTableReasons) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*r = nil
		return nil
	}

	var reports []SkippedTableReport
	if err := json.Unmarshal(data, &reports); err == nil {
		decoded := make(SkippedTableReasons, len(reports))
		for _, report := range reports {
			decoded[tableKey(report.SchemaName, report.TableName)] = report.Reason
		}
		*r = decoded
		return nil
	}

	var legacy map[string]string
	if err := json.Unmarshal(data, &legacy); err != nil {
		return err
	}
	*r = SkippedTableReasons(legacy)
	return nil
}

// MigrationStats tracks migration progress.
type MigrationStats struct {
	Users         int                 `json:"users"`
	OAuthLinks    int                 `json:"oauthLinks"`
	Policies      int                 `json:"policies"`
	Tables        int                 `json:"tables"`
	Views         int                 `json:"views"`
	Records       int                 `json:"records"`
	Sequences     int                 `json:"sequences"`
	StorageFiles  int                 `json:"storageFiles"`
	StorageBytes  int64               `json:"storageBytes"`
	Skipped       int                 `json:"skipped"`
	SkippedTables SkippedTableReasons `json:"skippedTables,omitempty"`
	Errors        []string            `json:"errors,omitempty"`
}

func schemaQualifiedName(schema, name string) string {
	if schema == "" {
		return name
	}
	return schema + "." + name
}

func tableKey(schema, name string) string {
	data, err := json.Marshal([2]string{schema, name})
	if err != nil {
		return schemaQualifiedName(schema, name)
	}
	return string(data)
}

func skippedTableReport(key, reason string) SkippedTableReport {
	schema, table, ok := tableKeyParts(key)
	if !ok {
		return SkippedTableReport{TableName: key, Reason: reason}
	}
	return SkippedTableReport{
		SchemaName: schema,
		TableName:  table,
		Reason:     reason,
	}
}

func tableKeyParts(key string) (string, string, bool) {
	var parts [2]string
	if err := json.Unmarshal([]byte(key), &parts); err != nil {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// MigrationOptions configures the Supabase migration process.
type MigrationOptions struct {
	SourceURL         string // Supabase PostgreSQL connection URL
	TargetURL         string // AYB PostgreSQL connection URL
	DryRun            bool
	Force             bool // allow migration when _ayb_users is not empty
	Verbose           bool
	SkipRLS           bool   // skip RLS policy rewriting
	SkipOAuth         bool   // skip OAuth identity migration
	SkipData          bool   // skip data table migration
	SkipStorage       bool   // skip storage file migration
	IncludeAnonymous  bool   // include is_anonymous users (default: skip)
	StorageExportPath string // local directory containing exported Supabase storage files
	StoragePath       string // destination path for AYB storage (default: ./ayb_storage)
	Progress          migrate.ProgressReporter
}
