package sbmigrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/allyourbase/ayb/internal/sqlutil"
)

type triggerCatalogAction uint8

const (
	triggerCatalogIgnore triggerCatalogAction = iota
	triggerCatalogMigrate
	triggerCatalogSkip
)

type migratedFunctionSet map[string]struct{}
type migratedTriggerSet map[string]struct{}
type targetPartitionTriggerCloneSet map[string]struct{}

type triggerCatalogEntry struct {
	Identity     TriggerIdentity
	RootIdentity TriggerIdentity
	Definition   string
	RelationKind string
	Constraint   bool
	Enabled      string
}

type triggerCatalogClassification struct {
	Action triggerCatalogAction
	Reason string
}

func loadTriggerCatalog(ctx context.Context, db *sql.DB) ([]triggerCatalogEntry, error) {
	tableTriggers, err := loadTableTriggerCatalog(ctx, db)
	if err != nil {
		return nil, err
	}
	eventTriggers, err := loadEventTriggerCatalog(ctx, db)
	if err != nil {
		return nil, err
	}
	return append(tableTriggers, eventTriggers...), nil
}

func loadTableTriggerCatalog(ctx context.Context, db *sql.DB) ([]triggerCatalogEntry, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT
			tn.nspname,
			c.relname,
			t.tgname,
			hn.nspname,
			p.proname,
			pg_get_function_identity_arguments(p.oid),
			c.relkind::text,
			t.tgconstraint <> 0,
			t.tgenabled::text,
			pg_get_triggerdef(t.oid)
		FROM pg_trigger t
		JOIN pg_class c ON c.oid = t.tgrelid
		JOIN pg_namespace tn ON tn.oid = c.relnamespace
		JOIN pg_proc p ON p.oid = t.tgfoid
		JOIN pg_namespace hn ON hn.oid = p.pronamespace
		WHERE NOT t.tgisinternal
		  AND t.tgparentid = 0
		ORDER BY tn.nspname, c.relname, t.tgname
	`)
	if err != nil {
		return nil, fmt.Errorf("querying trigger catalog: %w", err)
	}
	defer rows.Close()

	var entries []triggerCatalogEntry
	for rows.Next() {
		var entry triggerCatalogEntry
		if err := rows.Scan(
			&entry.Identity.TableSchemaName,
			&entry.Identity.TableName,
			&entry.Identity.Name,
			&entry.Identity.HandlerSchemaName,
			&entry.Identity.HandlerName,
			&entry.Identity.HandlerIdentityArguments,
			&entry.RelationKind,
			&entry.Constraint,
			&entry.Enabled,
			&entry.Definition,
		); err != nil {
			return nil, fmt.Errorf("scanning trigger catalog: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading trigger catalog: %w", err)
	}
	return entries, nil
}

func loadPartitionTriggerCloneStates(ctx context.Context, db *sql.DB) ([]triggerCatalogEntry, error) {
	rows, err := db.QueryContext(ctx, `
		WITH RECURSIVE clone_roots AS (
			SELECT t.oid AS clone_oid, t.tgparentid AS ancestor_oid
			FROM pg_trigger t
			WHERE NOT t.tgisinternal
			  AND t.tgparentid <> 0
			UNION ALL
			SELECT clone_roots.clone_oid, parent.tgparentid
			FROM clone_roots
			JOIN pg_trigger parent ON parent.oid = clone_roots.ancestor_oid
			WHERE parent.tgparentid <> 0
		)
		SELECT
			child_namespace.nspname,
			child_relation.relname,
			child.tgname,
			child.tgenabled::text,
			root_namespace.nspname,
			root_relation.relname,
			root.tgname,
			handler_namespace.nspname,
			handler.proname,
			pg_get_function_identity_arguments(handler.oid)
		FROM clone_roots
		JOIN pg_trigger child ON child.oid = clone_roots.clone_oid
		JOIN pg_class child_relation ON child_relation.oid = child.tgrelid
		JOIN pg_namespace child_namespace ON child_namespace.oid = child_relation.relnamespace
		JOIN pg_trigger root ON root.oid = clone_roots.ancestor_oid AND root.tgparentid = 0
		JOIN pg_class root_relation ON root_relation.oid = root.tgrelid
		JOIN pg_namespace root_namespace ON root_namespace.oid = root_relation.relnamespace
		JOIN pg_proc handler ON handler.oid = root.tgfoid
		JOIN pg_namespace handler_namespace ON handler_namespace.oid = handler.pronamespace
		ORDER BY child_namespace.nspname, child_relation.relname, child.tgname
	`)
	if err != nil {
		return nil, fmt.Errorf("querying partition trigger clone states: %w", err)
	}
	defer rows.Close()

	var entries []triggerCatalogEntry
	for rows.Next() {
		var entry triggerCatalogEntry
		if err := rows.Scan(
			&entry.Identity.TableSchemaName,
			&entry.Identity.TableName,
			&entry.Identity.Name,
			&entry.Enabled,
			&entry.RootIdentity.TableSchemaName,
			&entry.RootIdentity.TableName,
			&entry.RootIdentity.Name,
			&entry.RootIdentity.HandlerSchemaName,
			&entry.RootIdentity.HandlerName,
			&entry.RootIdentity.HandlerIdentityArguments,
		); err != nil {
			return nil, fmt.Errorf("scanning partition trigger clone state: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading partition trigger clone states: %w", err)
	}
	return entries, nil
}

func loadTargetPartitionTriggerClones(ctx context.Context, tx *sql.Tx) (targetPartitionTriggerCloneSet, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT
			namespace.nspname,
			relation.relname,
			trigger.tgname
		FROM pg_trigger trigger
		JOIN pg_class relation ON relation.oid = trigger.tgrelid
		JOIN pg_namespace namespace ON namespace.oid = relation.relnamespace
		WHERE NOT trigger.tgisinternal
		  AND trigger.tgparentid <> 0
	`)
	if err != nil {
		return nil, fmt.Errorf("querying target partition trigger clones: %w", err)
	}
	defer rows.Close()

	clones := make(targetPartitionTriggerCloneSet)
	for rows.Next() {
		var identity TriggerIdentity
		if err := rows.Scan(&identity.TableSchemaName, &identity.TableName, &identity.Name); err != nil {
			return nil, fmt.Errorf("scanning target partition trigger clone: %w", err)
		}
		clones[identity.Key()] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading target partition trigger clones: %w", err)
	}
	return clones, nil
}

func loadEventTriggerCatalog(ctx context.Context, db *sql.DB) ([]triggerCatalogEntry, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT
			e.evtname,
			hn.nspname,
			p.proname,
			pg_get_function_identity_arguments(p.oid),
			e.evtenabled::text
		FROM pg_event_trigger e
		JOIN pg_proc p ON p.oid = e.evtfoid
		JOIN pg_namespace hn ON hn.oid = p.pronamespace
		ORDER BY e.evtname
	`)
	if err != nil {
		return nil, fmt.Errorf("querying event trigger catalog: %w", err)
	}
	defer rows.Close()

	var entries []triggerCatalogEntry
	for rows.Next() {
		entry := triggerCatalogEntry{Identity: TriggerIdentity{EventTrigger: true}}
		if err := rows.Scan(
			&entry.Identity.Name,
			&entry.Identity.HandlerSchemaName,
			&entry.Identity.HandlerName,
			&entry.Identity.HandlerIdentityArguments,
			&entry.Enabled,
		); err != nil {
			return nil, fmt.Errorf("scanning event trigger catalog: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading event trigger catalog: %w", err)
	}
	return entries, nil
}

func classifyTriggerCatalogEntry(
	entry triggerCatalogEntry,
	migratedFunctions migratedFunctionSet,
	skippedTables SkippedTableReasons,
) triggerCatalogClassification {
	return classifyTriggerCatalogEntryWithSchemaTables(entry, migratedFunctions, skippedTables, nil)
}

func classifyTriggerCatalogEntryWithSchemaTables(
	entry triggerCatalogEntry,
	migratedFunctions migratedFunctionSet,
	skippedTables SkippedTableReasons,
	schemaTables tableIdentitySet,
) triggerCatalogClassification {
	if entry.Identity.EventTrigger {
		return classifyEventTriggerCatalogEntry(entry)
	}
	if tableTriggerCatalogEntryMalformed(entry) {
		return skippedTriggerClassification("trigger catalog row is malformed")
	}
	if isPostgresCatalogSchema(entry.Identity.TableSchemaName) {
		return triggerCatalogClassification{Action: triggerCatalogIgnore}
	}
	if !isAdmittedUserTable(entry.Identity.TableSchemaName, entry.Identity.TableName) {
		return skippedTriggerClassification(
			"trigger table " + triggerTableQualifiedName(entry) + " is not migrated",
		)
	}
	if schemaTableSkippedDuringSchemaMigration(entry, skippedTables, schemaTables) {
		return skippedTriggerClassification(
			"trigger table " + triggerTableQualifiedName(entry) + " was skipped during schema migration",
		)
	}
	if entry.RelationKind != "r" && entry.RelationKind != "p" {
		return skippedTriggerClassification("trigger relation kind " + entry.RelationKind + " is not supported")
	}
	if entry.Constraint {
		return skippedTriggerClassification("constraint triggers are not supported")
	}
	if !isAdmittedUserSchema(entry.Identity.HandlerSchemaName) {
		return skippedTriggerClassification(
			"trigger handler belongs to excluded schema " + entry.Identity.HandlerSchemaName,
		)
	}
	if schema, found := excludedSchemaReference(entry.Definition); found {
		return skippedTriggerClassification("trigger definition references excluded schema " + schema)
	}
	if _, ok := migratedFunctions[entry.handlerIdentity().Key()]; !ok {
		return skippedTriggerClassification(
			"trigger handler " + entry.handlerIdentity().QualifiedName() + " was not migrated",
		)
	}
	return triggerCatalogClassification{Action: triggerCatalogMigrate}
}

func schemaTableSkippedDuringSchemaMigration(
	entry triggerCatalogEntry,
	skippedTables SkippedTableReasons,
	schemaTables tableIdentitySet,
) bool {
	if schemaTables != nil {
		_, ok := schemaTables[tableKey(entry.Identity.TableSchemaName, entry.Identity.TableName)]
		return !ok
	}
	_, ok := skippedTables[tableKey(entry.Identity.TableSchemaName, entry.Identity.TableName)]
	return ok
}

func classifyEventTriggerCatalogEntry(entry triggerCatalogEntry) triggerCatalogClassification {
	if entry.Identity.Name == "" ||
		entry.Identity.HandlerSchemaName == "" ||
		entry.Identity.HandlerName == "" ||
		entry.Enabled == "" {
		return skippedTriggerClassification("trigger catalog row is malformed")
	}
	return skippedTriggerClassification("event triggers are not supported")
}

func tableTriggerCatalogEntryMalformed(entry triggerCatalogEntry) bool {
	return entry.Identity.TableSchemaName == "" ||
		entry.Identity.TableName == "" ||
		entry.Identity.Name == "" ||
		entry.Identity.HandlerSchemaName == "" ||
		entry.Identity.HandlerName == "" ||
		entry.RelationKind == "" ||
		entry.Enabled == "" ||
		entry.Definition == ""
}

func skippedTriggerClassification(reason string) triggerCatalogClassification {
	return triggerCatalogClassification{Action: triggerCatalogSkip, Reason: reason}
}

func triggerCatalogDenominator(
	entries []triggerCatalogEntry,
	migratedFunctions migratedFunctionSet,
	skippedTables SkippedTableReasons,
) int {
	return triggerCatalogDenominatorWithSchemaTables(entries, migratedFunctions, skippedTables, nil)
}

func triggerCatalogDenominatorWithSchemaTables(
	entries []triggerCatalogEntry,
	migratedFunctions migratedFunctionSet,
	skippedTables SkippedTableReasons,
	schemaTables tableIdentitySet,
) int {
	total := 0
	for _, entry := range entries {
		if classifyTriggerCatalogEntryWithSchemaTables(entry, migratedFunctions, skippedTables, schemaTables).Action != triggerCatalogIgnore {
			total++
		}
	}
	return total
}

func (entry triggerCatalogEntry) handlerIdentity() FunctionIdentity {
	return FunctionIdentity{
		SchemaName:        entry.Identity.HandlerSchemaName,
		Name:              entry.Identity.HandlerName,
		IdentityArguments: entry.Identity.HandlerIdentityArguments,
	}
}

func triggerTableQualifiedName(entry triggerCatalogEntry) string {
	return schemaQualifiedName(entry.Identity.TableSchemaName, entry.Identity.TableName)
}

func (m *Migrator) migrateTriggers(
	ctx context.Context,
	tx *sql.Tx,
	entries []triggerCatalogEntry,
	cloneStates []triggerCatalogEntry,
	migratedFunctions migratedFunctionSet,
	phase migrate.Phase,
) error {
	totalItems := triggerCatalogDenominatorWithSchemaTables(entries, migratedFunctions, m.stats.SkippedTables, m.schemaTables)
	m.progress.StartPhase(phase, totalItems)
	fmt.Fprintln(m.output, "Migrating triggers...")
	start := time.Now()

	processed := 0
	migrated := 0
	skipped := 0
	migratedTriggers := make(migratedTriggerSet)
	for _, entry := range entries {
		classification := classifyTriggerCatalogEntryWithSchemaTables(entry, migratedFunctions, m.stats.SkippedTables, m.schemaTables)
		switch classification.Action {
		case triggerCatalogIgnore:
			continue
		case triggerCatalogSkip:
			m.skipTrigger(phase, entry.Identity, classification.Reason)
			skipped++
		case triggerCatalogMigrate:
			if err := createTrigger(ctx, tx, entry); err != nil {
				return err
			}
			m.stats.Triggers++
			migratedTriggers[entry.Identity.Key()] = struct{}{}
			migrated++
			if m.verbose {
				fmt.Fprintf(m.output, "  CREATE TRIGGER %s\n", entry.Identity.DisplayName())
			}
		}
		processed++
		m.progress.Progress(phase, processed, totalItems)
	}
	targetClones, err := loadTargetPartitionTriggerClones(ctx, tx)
	if err != nil {
		return err
	}
	if err := restorePartitionTriggerCloneStates(ctx, tx, cloneStates, migratedTriggers, targetClones); err != nil {
		return err
	}

	m.progress.CompletePhase(phase, processed, time.Since(start))
	fmt.Fprintf(m.output, "  ✓ %d triggers migrated, %d skipped\n", migrated, skipped)
	return nil
}

func restorePartitionTriggerCloneStates(
	ctx context.Context,
	tx *sql.Tx,
	entries []triggerCatalogEntry,
	migratedTriggers migratedTriggerSet,
	targetClones targetPartitionTriggerCloneSet,
) error {
	for _, entry := range entries {
		if _, ok := migratedTriggers[entry.RootIdentity.Key()]; !ok {
			continue
		}
		if _, ok := targetClones[entry.Identity.Key()]; !ok {
			continue
		}
		if err := restoreTriggerEnabledState(ctx, tx, entry); err != nil {
			return fmt.Errorf("restoring partition trigger clone state %s: %w", entry.Identity.DisplayName(), err)
		}
	}
	return nil
}

func createTrigger(ctx context.Context, tx *sql.Tx, entry triggerCatalogEntry) error {
	if _, err := tx.ExecContext(ctx, entry.Definition); err != nil {
		return fmt.Errorf("creating trigger %s: %w", entry.Identity.DisplayName(), err)
	}
	if err := restoreTriggerEnabledState(ctx, tx, entry); err != nil {
		return fmt.Errorf("restoring trigger state %s: %w", entry.Identity.DisplayName(), err)
	}
	return nil
}

func restoreTriggerEnabledState(ctx context.Context, tx *sql.Tx, entry triggerCatalogEntry) error {
	stmt, ok := triggerEnabledStateSQL(entry)
	if !ok {
		return nil
	}
	_, err := tx.ExecContext(ctx, stmt)
	return err
}

func triggerEnabledStateSQL(entry triggerCatalogEntry) (string, bool) {
	tableName := sqlutil.QuoteQualifiedName(entry.Identity.TableSchemaName, entry.Identity.TableName)
	triggerName := sqlutil.QuoteIdent(entry.Identity.Name)
	switch entry.Enabled {
	case "O":
		return "", false
	case "A":
		return "ALTER TABLE " + tableName + " ENABLE ALWAYS TRIGGER " + triggerName, true
	case "R":
		return "ALTER TABLE " + tableName + " ENABLE REPLICA TRIGGER " + triggerName, true
	case "D":
		return "ALTER TABLE " + tableName + " DISABLE TRIGGER " + triggerName, true
	default:
		return "", false
	}
}

func (m *Migrator) skipTrigger(phase migrate.Phase, identity TriggerIdentity, reason string) {
	m.markSkippedTrigger(identity, errors.New(reason))
	m.stats.Skipped++
	m.progress.Warn(fmt.Sprintf("skipping trigger %s: %s", identity.DisplayName(), reason))
	if m.verbose {
		fmt.Fprintf(m.output, "  SKIP TRIGGER %s (%s)\n", identity.DisplayName(), reason)
	}
}
