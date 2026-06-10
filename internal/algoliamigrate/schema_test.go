package algoliamigrate

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestAnalyzeRecordsInfersFixtureSchema(t *testing.T) {
	t.Parallel()

	records := loadBrowseFixtureRecords(t)
	schema, err := AnalyzeRecords(records)
	if err != nil {
		t.Fatalf("AnalyzeRecords: %v", err)
	}

	if schema.RecordCount != 3 {
		t.Fatalf("RecordCount = %d, want 3", schema.RecordCount)
	}
	assertColumn(t, schema, "objectID", ColumnTypeText, false, true)
	assertColumn(t, schema, "title", ColumnTypeText, false, false)
	assertColumn(t, schema, "subtitle", ColumnTypeText, true, false)
	assertColumn(t, schema, "inventory_count", ColumnTypeInteger, false, false)
	assertColumn(t, schema, "price", ColumnTypeDouble, false, false)
	assertColumn(t, schema, "published", ColumnTypeBoolean, false, false)
	assertColumn(t, schema, "tags", ColumnTypeJSONB, false, false)
	assertColumn(t, schema, "dimensions", ColumnTypeJSONB, false, false)

	gotOrder := columnNames(schema.Columns)
	wantOrder := []string{"objectID", "dimensions", "inventory_count", "price", "published", "subtitle", "tags", "title"}
	if !reflect.DeepEqual(gotOrder, wantOrder) {
		t.Fatalf("column order = %#v, want %#v", gotOrder, wantOrder)
	}
}

func TestAnalyzeRecordsRejectsMissingOrBlankObjectID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		records []Record
	}{
		{
			name: "missing",
			records: []Record{
				{"title": "missing id"},
			},
		},
		{
			name: "blank",
			records: []Record{
				{"objectID": "   ", "title": "blank id"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := AnalyzeRecords(tt.records)
			if err == nil {
				t.Fatal("AnalyzeRecords unexpectedly succeeded")
			}
			if !strings.Contains(err.Error(), "objectID") {
				t.Fatalf("error = %q, want objectID context", err)
			}
		})
	}
}

func TestPlanImportBuildsDeterministicDDL(t *testing.T) {
	t.Parallel()

	plan, err := PlanImport(loadBrowseFixtureRecords(t), ImportOptions{
		TargetSchema: "public",
		TargetTable:  "Products Index!",
	})
	if err != nil {
		t.Fatalf("PlanImport: %v", err)
	}

	if plan.Target.TableName != "products_index" {
		t.Fatalf("table name = %q, want products_index", plan.Target.TableName)
	}
	wantDDL := strings.Join([]string{
		`CREATE TABLE "public"."products_index" (`,
		`  "objectID" text PRIMARY KEY,`,
		`  "dimensions" jsonb NOT NULL,`,
		`  "inventory_count" bigint NOT NULL,`,
		`  "price" double precision NOT NULL,`,
		`  "published" boolean NOT NULL,`,
		`  "subtitle" text,`,
		`  "tags" jsonb NOT NULL,`,
		`  "title" text NOT NULL`,
		`);`,
	}, "\n")
	if plan.Target.CreateTableSQL != wantDDL {
		t.Fatalf("CreateTableSQL:\n%s\nwant:\n%s", plan.Target.CreateTableSQL, wantDDL)
	}
	if plan.DryRun.TablesPlanned != 1 || plan.DryRun.RecordsPlanned != 3 {
		t.Fatalf("dry-run stats = %#v, want one table and three records", plan.DryRun)
	}
}

func loadBrowseFixtureRecords(t *testing.T) []Record {
	t.Helper()

	raw, err := os.ReadFile("testdata/algolia_browse_sample.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	envelope, err := DecodeBrowseResponse(raw)
	if err != nil {
		t.Fatalf("DecodeBrowseResponse: %v", err)
	}
	return envelope.Hits
}

func assertColumn(t *testing.T, schema Schema, name string, typ ColumnType, nullable, primaryKey bool) {
	t.Helper()

	for _, col := range schema.Columns {
		if col.Name == name {
			if col.Type != typ || col.Nullable != nullable || col.PrimaryKey != primaryKey {
				t.Fatalf("%s = %#v, want type=%s nullable=%v primaryKey=%v", name, col, typ, nullable, primaryKey)
			}
			return
		}
	}
	t.Fatalf("missing column %q in %#v", name, schema.Columns)
}

func columnNames(columns []Column) []string {
	names := make([]string, len(columns))
	for i, col := range columns {
		names[i] = col.Name
	}
	return names
}
