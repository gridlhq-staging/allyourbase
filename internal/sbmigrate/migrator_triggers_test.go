package sbmigrate

import (
	"io"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestClassifyTriggerCatalogEntryReportsClosedBranches(t *testing.T) {
	t.Parallel()

	handler := FunctionIdentity{SchemaName: "public", Name: "apply_trigger_side_effect"}
	migratedHandlers := map[string]struct{}{handler.Key(): {}}

	for _, tt := range triggerCatalogClassificationCases() {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			entry := supportedTriggerCatalogEntry()
			if tt.mutate != nil {
				tt.mutate(&entry)
			}

			classification := classifyTriggerCatalogEntry(entry, migratedHandlers, tt.skipped)

			testutil.Equal(t, tt.action, classification.Action)
			testutil.Equal(t, tt.reason, classification.Reason)
			testutil.Equal(t, tt.counted, classifyTriggerCatalogEntry(entry, migratedHandlers, tt.skipped).Action != triggerCatalogIgnore)
		})
	}
}

type triggerCatalogClassificationCase struct {
	name    string
	mutate  func(*triggerCatalogEntry)
	skipped SkippedTableReasons
	action  triggerCatalogAction
	reason  string
	counted bool
}

func triggerCatalogClassificationCases() []triggerCatalogClassificationCase {
	cases := []triggerCatalogClassificationCase{
		{name: "ordinary table trigger", action: triggerCatalogMigrate, counted: true},
		{
			name: "partitioned table trigger",
			mutate: func(entry *triggerCatalogEntry) {
				entry.RelationKind = "p"
			},
			action:  triggerCatalogMigrate,
			counted: true,
		},
	}
	cases = append(cases, triggerTableSkipClassificationCases()...)
	cases = append(cases, triggerHandlerSkipClassificationCases()...)
	return append(cases, triggerMalformedAndUnsupportedClassificationCases()...)
}

func triggerTableSkipClassificationCases() []triggerCatalogClassificationCase {
	return []triggerCatalogClassificationCase{
		{
			name: "internal catalog table",
			mutate: func(entry *triggerCatalogEntry) {
				entry.Identity.TableSchemaName = "pg_catalog"
			},
			action: triggerCatalogIgnore,
		},
		{
			name: "excluded table schema",
			mutate: func(entry *triggerCatalogEntry) {
				entry.Identity.TableSchemaName = "auth"
				entry.Identity.TableName = "users"
			},
			action:  triggerCatalogSkip,
			reason:  "trigger table auth.users is not migrated",
			counted: true,
		},
		{
			name: "schema-skipped table",
			mutate: func(entry *triggerCatalogEntry) {
				entry.Identity.TableSchemaName = "billing"
				entry.Identity.TableName = "legacy_problem"
			},
			skipped: SkippedTableReasons{
				tableKey("billing", "legacy_problem"): "unsupported column type",
			},
			action:  triggerCatalogSkip,
			reason:  "trigger table billing.legacy_problem was skipped during schema migration",
			counted: true,
		},
	}
}

func triggerHandlerSkipClassificationCases() []triggerCatalogClassificationCase {
	return []triggerCatalogClassificationCase{
		{
			name: "excluded schema handler",
			mutate: func(entry *triggerCatalogEntry) {
				entry.Identity.HandlerSchemaName = "auth"
				entry.Identity.HandlerName = "uid"
			},
			action:  triggerCatalogSkip,
			reason:  "trigger handler belongs to excluded schema auth",
			counted: true,
		},
		{
			name: "excluded schema definition reference",
			mutate: func(entry *triggerCatalogEntry) {
				entry.Definition = `CREATE TRIGGER sync BEFORE INSERT ON public.items FOR EACH ROW WHEN (auth.uid() IS NOT NULL) EXECUTE FUNCTION public.apply_trigger_side_effect()`
			},
			action:  triggerCatalogSkip,
			reason:  "trigger definition references excluded schema auth",
			counted: true,
		},
		{
			name: "handler absent from migrated set",
			mutate: func(entry *triggerCatalogEntry) {
				entry.Identity.HandlerName = "missing_handler"
			},
			action:  triggerCatalogSkip,
			reason:  "trigger handler public.missing_handler() was not migrated",
			counted: true,
		},
	}
}

func triggerMalformedAndUnsupportedClassificationCases() []triggerCatalogClassificationCase {
	return []triggerCatalogClassificationCase{
		{
			name: "unsupported relation kind",
			mutate: func(entry *triggerCatalogEntry) {
				entry.RelationKind = "v"
			},
			action:  triggerCatalogSkip,
			reason:  "trigger relation kind v is not supported",
			counted: true,
		},
		{
			name: "constraint trigger",
			mutate: func(entry *triggerCatalogEntry) {
				entry.Constraint = true
			},
			action:  triggerCatalogSkip,
			reason:  "constraint triggers are not supported",
			counted: true,
		},
		{
			name: "event trigger",
			mutate: func(entry *triggerCatalogEntry) {
				entry.Identity.EventTrigger = true
				entry.Identity.TableSchemaName = ""
				entry.Identity.TableName = ""
				entry.Definition = ""
				entry.RelationKind = ""
			},
			action:  triggerCatalogSkip,
			reason:  "event triggers are not supported",
			counted: true,
		},
		{
			name: "malformed table trigger",
			mutate: func(entry *triggerCatalogEntry) {
				entry.Definition = ""
			},
			action:  triggerCatalogSkip,
			reason:  "trigger catalog row is malformed",
			counted: true,
		},
	}
}

func TestTriggerCatalogDenominatorCountsEveryNonIgnoredEntry(t *testing.T) {
	t.Parallel()

	handler := FunctionIdentity{SchemaName: "public", Name: "apply_trigger_side_effect"}
	entries := []triggerCatalogEntry{
		supportedTriggerCatalogEntry(),
		func() triggerCatalogEntry {
			entry := supportedTriggerCatalogEntry()
			entry.Identity.TableSchemaName = "pg_catalog"
			return entry
		}(),
		func() triggerCatalogEntry {
			entry := supportedTriggerCatalogEntry()
			entry.Identity.EventTrigger = true
			entry.Identity.TableSchemaName = ""
			entry.Identity.TableName = ""
			entry.RelationKind = ""
			entry.Definition = ""
			return entry
		}(),
	}

	got := triggerCatalogDenominator(entries, map[string]struct{}{handler.Key(): {}}, nil)

	testutil.Equal(t, 2, got)
}

func TestClassifyTriggerCatalogEntryDoesNotTreatDataSkipAsSchemaSkip(t *testing.T) {
	t.Parallel()

	handler := FunctionIdentity{SchemaName: "public", Name: "apply_trigger_side_effect"}
	entry := supportedTriggerCatalogEntry()
	key := tableKey(entry.Identity.TableSchemaName, entry.Identity.TableName)
	schemaTables := tableIdentitySet{key: {}}
	dataSkippedTables := SkippedTableReasons{
		key: "violates foreign key constraint after data copy retries",
	}

	classification := classifyTriggerCatalogEntryWithSchemaTables(
		entry,
		map[string]struct{}{handler.Key(): {}},
		dataSkippedTables,
		schemaTables,
	)

	testutil.Equal(t, triggerCatalogMigrate, classification.Action)
	testutil.Equal(t, "", classification.Reason)
}

func TestSupabaseMigrationPhaseOrderPlacesTriggersAfterDatabasePrerequisites(t *testing.T) {
	t.Parallel()

	recorder := &phaseRecorder{}
	m := &Migrator{
		opts:     MigrationOptions{},
		output:   io.Discard,
		progress: recorder,
	}

	for _, phase := range supabaseTransactionPhaseNames(m.opts) {
		recorder.StartPhase(migrate.Phase{Name: phase}, 0)
	}

	recorder.AssertAfter(t, "Triggers", "Data", "Auth users", "OAuth", "RLS policies")
}

func supportedTriggerCatalogEntry() triggerCatalogEntry {
	return triggerCatalogEntry{
		Identity: TriggerIdentity{
			TableSchemaName:   "public",
			TableName:         "items",
			Name:              "sync_item",
			HandlerSchemaName: "public",
			HandlerName:       "apply_trigger_side_effect",
		},
		RelationKind: "r",
		Enabled:      "O",
		Definition:   `CREATE TRIGGER sync_item BEFORE INSERT ON public.items FOR EACH ROW EXECUTE FUNCTION public.apply_trigger_side_effect()`,
	}
}

type phaseRecorder struct {
	names []string
}

func (r *phaseRecorder) StartPhase(phase migrate.Phase, _ int) {
	r.names = append(r.names, phase.Name)
}

func (r *phaseRecorder) Progress(migrate.Phase, int, int) {}

func (r *phaseRecorder) CompletePhase(migrate.Phase, int, time.Duration) {}

func (r *phaseRecorder) Warn(string) {}

func (r *phaseRecorder) AssertAfter(t *testing.T, phase string, prerequisites ...string) {
	t.Helper()
	phaseIndex := r.index(phase)
	if phaseIndex < 0 {
		t.Fatalf("phase %q not recorded in %v", phase, r.names)
	}
	for _, prerequisite := range prerequisites {
		prerequisiteIndex := r.index(prerequisite)
		if prerequisiteIndex < 0 {
			t.Fatalf("prerequisite phase %q not recorded in %v", prerequisite, r.names)
		}
		if phaseIndex <= prerequisiteIndex {
			t.Fatalf("phase order %v places %q before %q", r.names, phase, prerequisite)
		}
	}
}

func (r *phaseRecorder) index(name string) int {
	for idx, candidate := range r.names {
		if candidate == name {
			return idx
		}
	}
	return -1
}

var _ migrate.ProgressReporter = (*phaseRecorder)(nil)
