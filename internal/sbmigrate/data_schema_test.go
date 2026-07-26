package sbmigrate

import (
	"errors"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestSchemaQualifiedTableIdentityAndSQL(t *testing.T) {
	t.Parallel()

	publicInvoices := TableInfo{SchemaName: "public", Name: "invoices"}
	billingInvoices := TableInfo{
		SchemaName: "billing",
		Name:       "invoices",
		Columns: []ColumnInfo{
			{Name: "id", DataType: "integer", IsNullable: false, OrdinalPos: 1},
			{Name: "account_id", DataType: "integer", IsNullable: false, OrdinalPos: 2},
		},
		PrimaryKey: "id",
		ForeignKeys: []ForeignKeyInfo{
			{
				ConstraintName: "invoices_account_id_fkey",
				ColumnName:     "account_id",
				RefSchemaName:  "crm",
				RefTable:       "accounts",
				RefColumn:      "id",
			},
		},
	}

	testutil.Equal(t, "public.invoices", publicInvoices.QualifiedName())
	testutil.Equal(t, "billing.invoices", billingInvoices.QualifiedName())
	testutil.False(t, publicInvoices.QualifiedName() == billingInvoices.QualifiedName(), "qualified table keys must not collide")

	ddl := createTableSQL(billingInvoices)
	testutil.Contains(t, ddl, `CREATE TABLE IF NOT EXISTS "billing"."invoices"`)
	testutil.Contains(t, ddl, `CONSTRAINT "invoices_account_id_fkey" FOREIGN KEY ("account_id") REFERENCES "crm"."accounts"("id")`)

	testutil.Equal(t, `SELECT "id", "account_id" FROM "billing"."invoices" ORDER BY 1`, copyTableSelectSQL(billingInvoices))
	testutil.Equal(t, `INSERT INTO "billing"."invoices" ("id", "account_id") VALUES ($1, $2) ON CONFLICT DO NOTHING`, copyTableInsertSQL(billingInvoices))
	testutil.Equal(t,
		`SELECT setval(pg_get_serial_sequence('"billing"."invoices"', 'id'), COALESCE(MAX("id"), 1), MAX("id") IS NOT NULL) FROM "billing"."invoices"`,
		resetSequenceSQL(billingInvoices, "id"),
	)
}

func TestTableIdentityIsUnambiguousForValidIdentifiers(t *testing.T) {
	t.Parallel()

	left := TableInfo{SchemaName: "a.b", Name: "c"}
	right := TableInfo{SchemaName: "a", Name: "b.c"}

	testutil.Equal(t, "a.b.c", left.QualifiedName())
	testutil.Equal(t, "a.b.c", right.QualifiedName())
	testutil.False(t, left.TableKey() == right.TableKey(), "internal table keys must not collide")
	testutil.Equal(t, `"a.b"."c"`, left.quotedName())
	testutil.Equal(t, `"a"."b.c"`, right.quotedName())
}

func TestCreateTableSQLCreatesOwnedSequencesBeforeDefaults(t *testing.T) {
	t.Parallel()

	table := TableInfo{
		SchemaName: "Billing.Schema",
		Name:       "Invoice.Table",
		Columns: []ColumnInfo{
			{
				Name:         "ID",
				DataType:     "integer",
				IsNullable:   false,
				DefaultValue: `nextval('"Billing.Schema"."Invoice.Table_ID_seq"'::regclass)`,
				OrdinalPos:   1,
			},
			{Name: "description", DataType: "text", IsNullable: false, OrdinalPos: 2},
		},
		PrimaryKey: "ID",
		Sequences: []SequenceInfo{
			{
				SchemaName:      "Billing.Schema",
				Name:            "Invoice.Table_ID_seq",
				TableSchemaName: "Billing.Schema",
				TableName:       "Invoice.Table",
				ColumnName:      "ID",
			},
		},
	}

	createSequences := createSequenceSQLs(table)
	testutil.Equal(t, 1, len(createSequences))
	testutil.Equal(t, `CREATE SEQUENCE IF NOT EXISTS "Billing.Schema"."Invoice.Table_ID_seq"`, createSequences[0])
	ownSequences := ownSequenceSQLs(table)
	testutil.Equal(t, 1, len(ownSequences))
	testutil.Equal(t, `ALTER SEQUENCE "Billing.Schema"."Invoice.Table_ID_seq" OWNED BY "Billing.Schema"."Invoice.Table"."ID"`, ownSequences[0])

	ddl := createTableSQL(table)
	testutil.Contains(t, ddl, `DEFAULT nextval('"Billing.Schema"."Invoice.Table_ID_seq"'::regclass)`)
}

func TestCreateViewSQLQualifiesViewName(t *testing.T) {
	t.Parallel()

	view := ViewInfo{
		SchemaName: "billing",
		Name:       "open_invoices",
		Definition: "SELECT id, account_id FROM billing.invoices WHERE paid_at IS NULL",
	}

	got := createViewSQL(view)
	testutil.Equal(t, `CREATE OR REPLACE VIEW "billing"."open_invoices" AS SELECT id, account_id FROM billing.invoices WHERE paid_at IS NULL`, got)
}

func TestIsAdmittedUserSchema(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		schema   string
		admitted bool
	}{
		{"public", "public", true},
		{"billing", "billing", true},
		{"auth", "auth", false},
		{"storage", "storage", false},
		{"realtime", "realtime", false},
		{"extensions", "extensions", false},
		{"graphql", "graphql", false},
		{"graphql_public", "graphql_public", false},
		{"vault", "vault", false},
		{"pgsodium", "pgsodium", false},
		{"cron", "cron", false},
		{"net", "net", false},
		{"pgbouncer", "pgbouncer", false},
		{"pgsodium masks", "pgsodium_masks", false},
		{"pgtle", "pgtle", false},
		{"analytics internal", "_analytics", false},
		{"realtime internal", "_realtime", false},
		{"supabase private", "_supabase", false},
		{"supabase prefix", "supabase_functions", false},
		{"pg catalog", "pg_catalog", false},
		{"pg toast", "pg_toast", false},
		{"information schema", "information_schema", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// The explicit Supabase list prevents product-owned internals from being treated as user data.
			testutil.Equal(t, tt.admitted, isAdmittedUserSchema(tt.schema))
		})
	}
}

func TestTableAdmissionScopesInternalNamesToOwningSchemas(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		schema   string
		table    string
		admitted bool
	}{
		{"public schema migrations remains hosted internal", "public", "schema_migrations", false},
		{"billing schema migrations is user data", "billing", "schema_migrations", true},
		{"public vector indexes remains hosted internal", "public", "vector_indexes", false},
		{"billing vector indexes is user data", "billing", "vector_indexes", true},
		{"public ayb event is internal", "public", "_ayb_events", false},
		{"billing ayb prefix is user data", "billing", "_ayb_events", true},
		{"storage vector indexes remains storage internal", "storage", "vector_indexes", false},
		{"supabase schema table excluded", "supabase_functions", "jobs", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			testutil.Equal(t, tt.admitted, isAdmittedUserTable(tt.schema, tt.table))
		})
	}
}

func TestSkippedTablesUseQualifiedIdentity(t *testing.T) {
	t.Parallel()

	m := &Migrator{}
	publicInvoices := TableInfo{SchemaName: "a.b", Name: "c"}
	billingInvoices := TableInfo{SchemaName: "a", Name: "b.c"}

	m.markSkippedTable(billingInvoices, errors.New("missing referenced table"))

	testutil.True(t, m.isSkippedTable(billingInvoices), "a.b.c should be skipped for the exact table")
	testutil.False(t, m.isSkippedTable(publicInvoices), "same display text must not collide")

	filtered := m.filterSkippedTables([]TableInfo{publicInvoices, billingInvoices})
	testutil.Equal(t, 1, len(filtered))
	testutil.Equal(t, "a.b.c", filtered[0].QualifiedName())
	testutil.Equal(t, "missing referenced table", m.stats.SkippedTables[billingInvoices.TableKey()])
}
