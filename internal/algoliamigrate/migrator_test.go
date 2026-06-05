package algoliamigrate

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/migrate"
)

func TestPlanImportRejectsReservedAYBTableNamesBeforeSQL(t *testing.T) {
	t.Parallel()

	_, err := PlanImport([]Record{{"objectID": "one", "title": "Desk Lamp"}}, ImportOptions{
		TargetTable: "_ayb_users",
	})
	if err == nil {
		t.Fatal("PlanImport unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), "_ayb_") {
		t.Fatalf("error = %q, want reserved _ayb_ context", err)
	}
}

func TestPlanImportRejectsEmptyRecordSet(t *testing.T) {
	t.Parallel()

	_, err := PlanImport(nil, ImportOptions{TargetTable: "products"})
	if err == nil {
		t.Fatal("PlanImport unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), "empty Algolia record set") {
		t.Fatalf("error = %q, want empty record set context", err)
	}
}

func TestPlanImportBuildsQuotedPreflightAndParameterizedInsertSQL(t *testing.T) {
	t.Parallel()

	plan, err := PlanImport([]Record{{"objectID": "one", `bad"name`: "quoted"}}, ImportOptions{
		TargetSchema: `tenant"schema`,
		TargetTable:  `Sales "Index"`,
	})
	if err != nil {
		t.Fatalf("PlanImport: %v", err)
	}

	wantPreflight := `SELECT 1 FROM "tenant""schema"."sales_index" LIMIT 0`
	if plan.Target.PreflightSQL != wantPreflight {
		t.Fatalf("PreflightSQL = %q, want %q", plan.Target.PreflightSQL, wantPreflight)
	}

	wantInsert := `INSERT INTO "tenant""schema"."sales_index" ("objectID", "bad""name") VALUES ($1, $2)`
	if plan.Target.InsertSQL != wantInsert {
		t.Fatalf("InsertSQL = %q, want %q", plan.Target.InsertSQL, wantInsert)
	}
}

func TestBuildInsertBatchUsesOneParameterizedStatementForMultipleRecords(t *testing.T) {
	t.Parallel()

	plan, err := PlanImport([]Record{
		{"objectID": "one", "inventory_count": json.Number("4"), "tags": []any{"lighting", "office"}},
		{"objectID": "two", "inventory_count": json.Number("7"), "tags": []any{"kitchen"}},
	}, ImportOptions{
		TargetSchema: `tenant"schema`,
		TargetTable:  "Products",
	})
	if err != nil {
		t.Fatalf("PlanImport: %v", err)
	}

	batch, err := buildInsertBatch(plan.Target, []Record{
		{"objectID": "one", "inventory_count": json.Number("4"), "tags": []any{"lighting", "office"}},
		{"objectID": "two", "inventory_count": json.Number("7"), "tags": []any{"kitchen"}},
	})
	if err != nil {
		t.Fatalf("buildInsertBatch: %v", err)
	}

	wantSQL := `INSERT INTO "tenant""schema"."products" ("objectID", "inventory_count", "tags") VALUES ($1, $2, $3), ($4, $5, $6)`
	if batch.SQL != wantSQL {
		t.Fatalf("batch SQL = %q, want %q", batch.SQL, wantSQL)
	}
	wantValues := []any{"one", int64(4), `["lighting","office"]`, "two", int64(7), `["kitchen"]`}
	if !reflect.DeepEqual(batch.Values, wantValues) {
		t.Fatalf("batch values = %#v, want %#v", batch.Values, wantValues)
	}
}

func TestImportStatsJSONOmitsEmptySynonymStats(t *testing.T) {
	t.Parallel()

	raw, err := json.Marshal(ImportStats{Tables: 1, Records: 3})
	if err != nil {
		t.Fatalf("Marshal ImportStats: %v", err)
	}
	if strings.Contains(string(raw), "synonyms") {
		t.Fatalf("default stats JSON included synonyms: %s", raw)
	}

	withSynonyms, err := json.Marshal(ImportStats{
		Tables:   1,
		Records:  3,
		Synonyms: SynonymStats{SupportedGroups: 1},
	})
	if err != nil {
		t.Fatalf("Marshal ImportStats with synonyms: %v", err)
	}
	if !strings.Contains(string(withSynonyms), `"synonyms":{"supportedGroups":1}`) {
		t.Fatalf("synonym stats JSON = %s", withSynonyms)
	}
}

func TestBuildValidationSummaryReportsMismatchesSkippedAndErrors(t *testing.T) {
	t.Parallel()

	report := &migrate.AnalysisReport{Tables: 1, Records: 3}
	stats := &ImportStats{Tables: 1, Records: 2, Skipped: 1, Errors: []string{"insert failed"}}
	summary := BuildValidationSummary(report, stats)

	if summary.SourceLabel != "Algolia (source)" || summary.TargetLabel != "AYB (target)" {
		t.Fatalf("labels = %q/%q", summary.SourceLabel, summary.TargetLabel)
	}
	if len(summary.Rows) != 2 {
		t.Fatalf("rows = %#v, want Tables and Records", summary.Rows)
	}
	if summary.Rows[0].Label != "Tables" || summary.Rows[0].SourceCount != 1 || summary.Rows[0].TargetCount != 1 {
		t.Fatalf("tables row = %#v", summary.Rows[0])
	}
	if summary.Rows[1].Label != "Records" || summary.Rows[1].SourceCount != 3 || summary.Rows[1].TargetCount != 2 {
		t.Fatalf("records row = %#v", summary.Rows[1])
	}
	wantWarnings := []string{
		"Records count mismatch: source=3 target=2",
		"1 records skipped during import",
		"1 errors occurred during import",
	}
	if strings.Join(summary.Warnings, "\n") != strings.Join(wantWarnings, "\n") {
		t.Fatalf("warnings = %#v, want %#v", summary.Warnings, wantWarnings)
	}
}

func TestBuildReportsExposeSynonymStatsWithoutRecordSkippedInflation(t *testing.T) {
	t.Parallel()

	plan, err := PlanImport([]Record{{"objectID": "one", "title": "Desk Lamp"}}, ImportOptions{
		TargetTable: "products",
		Synonyms: &SynonymInput{
			Hits: []AlgoliaSynonymHit{
				{Type: "synonym", Synonyms: []string{"Desk Lamp", "Task Light"}},
				{Type: "oneWaySynonym", Synonyms: []string{"lamp", "light"}},
				{Type: "placeholder"},
				{Type: "synonym", Synonyms: []string{"single"}},
			},
			Stats: SynonymStats{SkippedSettingsACL: 1},
		},
	})
	if err != nil {
		t.Fatalf("PlanImport: %v", err)
	}

	report := BuildAnalysisReport(plan)
	if report.Records != 1 {
		t.Fatalf("report records = %d, want 1", report.Records)
	}
	wantWarnings := []string{
		"1 unsupported oneWaySynonym synonyms skipped: unsupported directional synonym",
		"1 unsupported placeholder synonyms skipped: unsupported placeholder synonym",
		"1 invalid synonym groups skipped",
		"1 synonym enumeration skipped due to missing settings ACL",
	}
	if strings.Join(report.Warnings, "\n") != strings.Join(wantWarnings, "\n") {
		t.Fatalf("report warnings = %#v, want %#v", report.Warnings, wantWarnings)
	}

	stats := &ImportStats{
		Tables:  1,
		Records: 1,
		Synonyms: SynonymStats{
			SupportedGroups:           plan.Synonyms.Stats.SupportedGroups,
			SkippedUnsupportedTypes:   plan.Synonyms.Stats.SkippedUnsupportedTypes,
			SkippedUnsupportedReasons: plan.Synonyms.Stats.SkippedUnsupportedReasons,
			SkippedInvalidGroups:      plan.Synonyms.Stats.SkippedInvalidGroups,
			SkippedSettingsACL:        plan.Synonyms.Stats.SkippedSettingsACL,
		},
	}
	summary := BuildValidationSummary(report, stats)
	if len(summary.Rows) != 3 {
		t.Fatalf("summary rows = %#v, want Tables, Records, Synonym groups", summary.Rows)
	}
	if summary.Rows[2].Label != "Synonym groups" || summary.Rows[2].SourceCount != 1 || summary.Rows[2].TargetCount != 1 {
		t.Fatalf("synonym row = %#v", summary.Rows[2])
	}
	for _, warning := range summary.Warnings {
		if strings.Contains(warning, "records skipped") {
			t.Fatalf("synonym skip was folded into record skipped warning: %#v", summary.Warnings)
		}
	}
	if strings.Join(summary.Warnings, "\n") != strings.Join(wantWarnings, "\n") {
		t.Fatalf("summary warnings = %#v, want %#v", summary.Warnings, wantWarnings)
	}
}

func TestBuildValidationSummaryReportsImportedSynonymCountMismatch(t *testing.T) {
	t.Parallel()

	report := &migrate.AnalysisReport{Tables: 1, Records: 1}
	stats := &ImportStats{Tables: 1, Records: 1, Synonyms: SynonymStats{SupportedGroups: 2}}
	summary := BuildValidationSummary(report, stats)

	got := summary.Rows[len(summary.Rows)-1]
	if got.Label != "Synonym groups" || got.SourceCount != 0 || got.TargetCount != 2 {
		t.Fatalf("synonym row = %#v, want source 0 target 2", got)
	}
	if !strings.Contains(strings.Join(summary.Warnings, "\n"), "Synonym groups count mismatch: source=0 target=2") {
		t.Fatalf("warnings = %#v, want synonym group mismatch", summary.Warnings)
	}
}

func TestCheckRecordParityDefaultsToFixtureAndUsesLiveWhenConfigured(t *testing.T) {
	t.Parallel()

	fixture, err := CheckRecordParity(context.Background(), BrowseConfig{}, []Record{
		{"objectID": "one"},
		{"objectID": "two"},
	}, 2)
	if err != nil {
		t.Fatalf("fixture CheckRecordParity: %v", err)
	}
	if !fixture.Match || fixture.Source != "fixture" || fixture.SourceRecords != 2 || fixture.TargetRecords != 2 {
		t.Fatalf("fixture parity = %#v", fixture)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"hits":[{"objectID":"one"},{"objectID":"two"},{"objectID":"three"}]}`))
	}))
	defer server.Close()

	live, err := CheckRecordParity(context.Background(), BrowseConfig{
		Endpoint:  server.URL,
		AppID:     "APPID123",
		APIKey:    "secret-key",
		IndexName: "products",
	}, nil, 2)
	if err != nil {
		t.Fatalf("live CheckRecordParity: %v", err)
	}
	if live.Match || live.Source != "live" || live.SourceRecords != 3 || live.TargetRecords != 2 {
		t.Fatalf("live parity = %#v", live)
	}
}
