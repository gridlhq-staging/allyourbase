package sbmigrate

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestMigrationStatsSkippedFunctionsAndTriggersJSONUsesStructuredRows(t *testing.T) {
	t.Parallel()

	m := &Migrator{}
	leftFunction := FunctionIdentity{SchemaName: "a.b", Name: "c", IdentityArguments: "integer"}
	rightFunction := FunctionIdentity{SchemaName: "a", Name: "b.c", IdentityArguments: "text"}
	leftTrigger := TriggerIdentity{
		TableSchemaName:          "a.b",
		TableName:                "c",
		Name:                     "audit",
		HandlerSchemaName:        "fn",
		HandlerName:              "record_change",
		HandlerIdentityArguments: "jsonb",
	}
	rightTrigger := TriggerIdentity{
		TableSchemaName:          "a",
		TableName:                "b.c",
		Name:                     "audit",
		HandlerSchemaName:        "fn",
		HandlerName:              "record_change",
		HandlerIdentityArguments: "text",
	}
	eventTrigger := TriggerIdentity{
		EventTrigger:             true,
		Name:                     "ddl_audit",
		HandlerSchemaName:        "public",
		HandlerName:              "record_ddl",
		HandlerIdentityArguments: "",
	}

	m.markSkippedFunction(leftFunction, errPublicReason{})
	m.markSkippedFunction(rightFunction, errBillingReason{})
	m.markSkippedTrigger(leftTrigger, errPublicReason{})
	m.markSkippedTrigger(rightTrigger, errBillingReason{})
	m.markSkippedTrigger(eventTrigger, errPublicReason{})

	data, err := json.Marshal(m.stats)
	testutil.NoError(t, err)
	testutil.Equal(t, `{"users":0,"oauthLinks":0,"policies":0,"tables":0,"views":0,"functions":0,"triggers":0,"records":0,"sequences":0,"storageFiles":0,"storageBytes":0,"skipped":0,"skippedFunctions":[{"schema":"a","name":"b.c","identityArguments":"text","reason":"billing reason"},{"schema":"a.b","name":"c","identityArguments":"integer","reason":"public reason"}],"skippedTriggers":[{"tableSchema":"a","table":"b.c","name":"audit","handlerSchema":"fn","handlerName":"record_change","handlerIdentityArguments":"text","reason":"billing reason"},{"tableSchema":"a.b","table":"c","name":"audit","handlerSchema":"fn","handlerName":"record_change","handlerIdentityArguments":"jsonb","reason":"public reason"},{"eventTrigger":true,"tableSchema":"","table":"","name":"ddl_audit","handlerSchema":"public","handlerName":"record_ddl","reason":"public reason"}]}`, string(data))
	testutil.False(t, strings.Contains(string(data), functionKey("a", "b.c", "text")), "internal function key must not leak into JSON output")
	testutil.False(t, strings.Contains(string(data), triggerKey(false, "a", "b.c", "audit", "fn", "record_change", "text")), "internal trigger key must not leak into JSON output")

	var decoded MigrationStats
	testutil.NoError(t, json.Unmarshal(data, &decoded))
	testutil.Equal(t, "billing reason", decoded.SkippedFunctions[rightFunction.Key()])
	testutil.Equal(t, "public reason", decoded.SkippedFunctions[leftFunction.Key()])
	testutil.Equal(t, "billing reason", decoded.SkippedTriggers[rightTrigger.Key()])
	testutil.Equal(t, "public reason", decoded.SkippedTriggers[leftTrigger.Key()])
	testutil.Equal(t, "public reason", decoded.SkippedTriggers[eventTrigger.Key()])
}

func TestSkippedTriggerReasonsUnmarshalLegacySixFieldKeysWithoutShiftingFields(t *testing.T) {
	t.Parallel()

	legacyIdentity := TriggerIdentity{
		TableSchemaName:          "public",
		TableName:                "orders",
		Name:                     "audit_order",
		HandlerSchemaName:        "audit",
		HandlerName:              "record_order",
		HandlerIdentityArguments: "integer",
	}
	legacyKey, err := json.Marshal([6]string{
		legacyIdentity.TableSchemaName,
		legacyIdentity.TableName,
		legacyIdentity.Name,
		legacyIdentity.HandlerSchemaName,
		legacyIdentity.HandlerName,
		legacyIdentity.HandlerIdentityArguments,
	})
	testutil.NoError(t, err)
	legacyMapKey, err := json.Marshal(string(legacyKey))
	testutil.NoError(t, err)

	var decoded MigrationStats
	data := []byte(`{"skippedTriggers":{` + string(legacyMapKey) + `:"legacy reason"}}`)
	testutil.NoError(t, json.Unmarshal(data, &decoded))

	testutil.Equal(t, "legacy reason", decoded.SkippedTriggers[legacyIdentity.Key()])
}

func TestBuildValidationSummaryIncludesFunctionAndTriggerRowsAndSkippedWarnings(t *testing.T) {
	t.Parallel()

	m := &Migrator{}
	m.markSkippedFunction(FunctionIdentity{
		SchemaName:        "public",
		Name:              "sync_profile",
		IdentityArguments: "uuid, jsonb",
	}, errPublicReason{})
	m.markSkippedTrigger(TriggerIdentity{
		TableSchemaName:          "billing",
		TableName:                "invoices",
		Name:                     "audit_invoice",
		HandlerSchemaName:        "audit",
		HandlerName:              "write_invoice_audit",
		HandlerIdentityArguments: "jsonb",
	}, errBillingReason{})

	report := &migrate.AnalysisReport{
		Functions: 2,
		Triggers:  3,
		AuthUsers: 1,
	}
	stats := &MigrationStats{
		Functions:        1,
		Triggers:         2,
		Users:            1,
		Skipped:          2,
		SkippedFunctions: m.stats.SkippedFunctions,
		SkippedTriggers:  m.stats.SkippedTriggers,
	}
	summary := BuildValidationSummary(report, stats)

	testutil.Equal(t, "Functions", summary.Rows[0].Label)
	testutil.Equal(t, 2, summary.Rows[0].SourceCount)
	testutil.Equal(t, 1, summary.Rows[0].TargetCount)
	testutil.Equal(t, "Triggers", summary.Rows[1].Label)
	testutil.Equal(t, 3, summary.Rows[1].SourceCount)
	testutil.Equal(t, 2, summary.Rows[1].TargetCount)
	testutil.Contains(t, strings.Join(summary.Warnings, "\n"), "Functions count mismatch: source=2 target=1")
	testutil.Contains(t, strings.Join(summary.Warnings, "\n"), "Triggers count mismatch: source=3 target=2")
	testutil.Contains(t, strings.Join(summary.Warnings, "\n"), "function public.sync_profile(uuid, jsonb) skipped during migration: public reason")
	testutil.Contains(t, strings.Join(summary.Warnings, "\n"), "trigger billing.invoices.audit_invoice using audit.write_invoice_audit(jsonb) skipped during migration: billing reason")
}
