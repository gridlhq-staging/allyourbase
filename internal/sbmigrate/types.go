// Package sbmigrate migrates auth users, OAuth identities, RLS policies,
// and data tables from a Supabase PostgreSQL database to AYB.
package sbmigrate

import (
	"encoding/json"
	"fmt"
	"strings"
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
	SchemaName            string
	Name                  string
	Columns               []ColumnInfo
	PrimaryKey            string // column name of the PK (empty if composite/none)
	ForeignKeys           []ForeignKeyInfo
	Sequences             []SequenceInfo
	RowCount              int64
	PartitionKey          string
	PartitionParentSchema string
	PartitionParentName   string
	PartitionBound        string
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

// FunctionIdentity identifies a PostgreSQL function by the same fields pg_get_function_identity_arguments uses.
type FunctionIdentity struct {
	SchemaName        string
	Name              string
	IdentityArguments string
}

func (i FunctionIdentity) Key() string {
	return functionKey(i.SchemaName, i.Name, i.IdentityArguments)
}

func (i FunctionIdentity) QualifiedName() string {
	if i.IdentityArguments == "" {
		return schemaQualifiedName(i.SchemaName, i.Name) + "()"
	}
	return schemaQualifiedName(i.SchemaName, i.Name) + "(" + i.IdentityArguments + ")"
}

// SkippedFunctionReasons tracks skipped functions by the internal collision-safe key.
type SkippedFunctionReasons map[string]string

// SkippedFunctionReport is the stable JSON shape for skipped-function reporting.
type SkippedFunctionReport struct {
	SchemaName        string `json:"schema"`
	Name              string `json:"name"`
	IdentityArguments string `json:"identityArguments"`
	Reason            string `json:"reason"`
}

func (r SkippedFunctionReport) Identity() FunctionIdentity {
	return FunctionIdentity{
		SchemaName:        r.SchemaName,
		Name:              r.Name,
		IdentityArguments: r.IdentityArguments,
	}
}

func (r SkippedFunctionReport) QualifiedName() string {
	return r.Identity().QualifiedName()
}

func (r SkippedFunctionReasons) MarshalJSON() ([]byte, error) {
	return json.Marshal(sortedSkippedFunctionReports(r))
}

func (r *SkippedFunctionReasons) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*r = nil
		return nil
	}

	var reports []SkippedFunctionReport
	if err := json.Unmarshal(data, &reports); err == nil {
		decoded := make(SkippedFunctionReasons, len(reports))
		for _, report := range reports {
			decoded[report.Identity().Key()] = report.Reason
		}
		*r = decoded
		return nil
	}

	var legacy map[string]string
	if err := json.Unmarshal(data, &legacy); err != nil {
		return err
	}
	*r = SkippedFunctionReasons(legacy)
	return nil
}

// TriggerIdentity identifies a PostgreSQL trigger and the handler function it invokes when known.
type TriggerIdentity struct {
	EventTrigger             bool
	TableSchemaName          string
	TableName                string
	Name                     string
	HandlerSchemaName        string
	HandlerName              string
	HandlerIdentityArguments string
}

func (i TriggerIdentity) Key() string {
	return triggerKey(
		i.EventTrigger,
		i.TableSchemaName,
		i.TableName,
		i.Name,
		i.HandlerSchemaName,
		i.HandlerName,
		i.HandlerIdentityArguments,
	)
}

func (i TriggerIdentity) QualifiedName() string {
	if i.EventTrigger {
		return "event trigger " + i.Name
	}
	return schemaQualifiedName(i.TableSchemaName, i.TableName) + "." + i.Name
}

func (i TriggerIdentity) HandlerQualifiedName() string {
	if i.HandlerName == "" {
		return ""
	}
	return FunctionIdentity{
		SchemaName:        i.HandlerSchemaName,
		Name:              i.HandlerName,
		IdentityArguments: i.HandlerIdentityArguments,
	}.QualifiedName()
}

func (i TriggerIdentity) DisplayName() string {
	if handler := i.HandlerQualifiedName(); handler != "" {
		return i.QualifiedName() + " using " + handler
	}
	return i.QualifiedName()
}

// SkippedTriggerReasons tracks skipped triggers by the internal collision-safe key.
type SkippedTriggerReasons map[string]string

// SkippedTriggerReport is the stable JSON shape for skipped-trigger reporting.
type SkippedTriggerReport struct {
	EventTrigger             bool   `json:"eventTrigger,omitempty"`
	TableSchemaName          string `json:"tableSchema"`
	TableName                string `json:"table"`
	Name                     string `json:"name"`
	HandlerSchemaName        string `json:"handlerSchema,omitempty"`
	HandlerName              string `json:"handlerName,omitempty"`
	HandlerIdentityArguments string `json:"handlerIdentityArguments,omitempty"`
	Reason                   string `json:"reason"`
}

func (r SkippedTriggerReport) Identity() TriggerIdentity {
	return TriggerIdentity{
		EventTrigger:             r.EventTrigger,
		TableSchemaName:          r.TableSchemaName,
		TableName:                r.TableName,
		Name:                     r.Name,
		HandlerSchemaName:        r.HandlerSchemaName,
		HandlerName:              r.HandlerName,
		HandlerIdentityArguments: r.HandlerIdentityArguments,
	}
}

func (r SkippedTriggerReport) DisplayName() string {
	return r.Identity().DisplayName()
}

func (r SkippedTriggerReasons) MarshalJSON() ([]byte, error) {
	return json.Marshal(sortedSkippedTriggerReports(r))
}

func (r *SkippedTriggerReasons) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*r = nil
		return nil
	}

	var reports []SkippedTriggerReport
	if err := json.Unmarshal(data, &reports); err == nil {
		decoded := make(SkippedTriggerReasons, len(reports))
		for _, report := range reports {
			decoded[report.Identity().Key()] = report.Reason
		}
		*r = decoded
		return nil
	}

	var legacy map[string]string
	if err := json.Unmarshal(data, &legacy); err != nil {
		return err
	}
	decoded := make(SkippedTriggerReasons, len(legacy))
	for key, reason := range legacy {
		if identity, ok := triggerKeyParts(key); ok {
			decoded[identity.Key()] = reason
			continue
		}
		decoded[key] = reason
	}
	*r = decoded
	return nil
}

// MigrationStats tracks migration progress.
type MigrationStats struct {
	Users            int                    `json:"users"`
	OAuthLinks       int                    `json:"oauthLinks"`
	Policies         int                    `json:"policies"`
	Tables           int                    `json:"tables"`
	Views            int                    `json:"views"`
	Functions        int                    `json:"functions"`
	Triggers         int                    `json:"triggers"`
	Records          int                    `json:"records"`
	Sequences        int                    `json:"sequences"`
	StorageFiles     int                    `json:"storageFiles"`
	StorageBytes     int64                  `json:"storageBytes"`
	Skipped          int                    `json:"skipped"`
	SkippedTables    SkippedTableReasons    `json:"skippedTables,omitempty"`
	SkippedFunctions SkippedFunctionReasons `json:"skippedFunctions,omitempty"`
	SkippedTriggers  SkippedTriggerReasons  `json:"skippedTriggers,omitempty"`
	Errors           []string               `json:"errors,omitempty"`
}

func schemaQualifiedName(schema, name string) string {
	if schema == "" {
		return name
	}
	return schema + "." + name
}

// MigrationOptions configures the Supabase migration process.
type MigrationOptions struct {
	SourceURL          string // Supabase PostgreSQL connection URL
	TargetURL          string // AYB PostgreSQL connection URL
	DryRun             bool
	Force              bool // allow migration when _ayb_users is not empty
	Verbose            bool
	SkipRLS            bool   // skip RLS policy rewriting
	SkipOAuth          bool   // skip OAuth identity migration
	SkipData           bool   // skip data table migration
	SkipFunctions      bool   // skip database function and trigger migration
	SkipStorage        bool   // skip storage file migration
	IncludeAnonymous   bool   // include is_anonymous users (default: skip)
	StorageExportPath  string // local directory containing exported Supabase storage files
	StoragePath        string // destination path for AYB storage (default: ./ayb_storage)
	StorageS3Endpoint  string // S3-compatible source endpoint for storage object bytes
	StorageS3Region    string // S3-compatible source region
	StorageS3AccessKey string // S3-compatible source access key
	StorageS3SecretKey string // S3-compatible source secret key
	StorageS3UseSSL    bool   // use HTTPS for S3 source endpoints without an explicit scheme
	Progress           migrate.ProgressReporter
}

func (opts MigrationOptions) storageSourceConfigured() bool {
	return strings.TrimSpace(opts.StorageExportPath) != "" || opts.storageS3SourceConfigured()
}

func (opts MigrationOptions) storageS3SourceConfigured() bool {
	return strings.TrimSpace(opts.StorageS3Endpoint) != "" &&
		strings.TrimSpace(opts.StorageS3Region) != "" &&
		strings.TrimSpace(opts.StorageS3AccessKey) != "" &&
		strings.TrimSpace(opts.StorageS3SecretKey) != ""
}

func (opts MigrationOptions) validateStorageSourceOptions() error {
	hasS3Values := strings.TrimSpace(opts.StorageS3Endpoint) != "" ||
		strings.TrimSpace(opts.StorageS3Region) != "" ||
		strings.TrimSpace(opts.StorageS3AccessKey) != "" ||
		strings.TrimSpace(opts.StorageS3SecretKey) != ""
	if strings.TrimSpace(opts.StorageExportPath) != "" && hasS3Values {
		return fmt.Errorf("storage export path cannot be combined with S3 storage source options")
	}
	if !hasS3Values {
		return nil
	}
	for _, required := range []struct {
		name  string
		value string
	}{
		{name: "endpoint", value: opts.StorageS3Endpoint},
		{name: "region", value: opts.StorageS3Region},
		{name: "access key", value: opts.StorageS3AccessKey},
		{name: "secret key", value: opts.StorageS3SecretKey},
	} {
		if strings.TrimSpace(required.value) == "" {
			return fmt.Errorf("S3 storage source %s is required", required.name)
		}
	}
	return nil
}
