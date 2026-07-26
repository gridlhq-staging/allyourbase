package sbmigrate

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/allyourbase/ayb/internal/sqlutil"
)

// migrateRLSPolicies migrates row-level security policies from the source database to the target, rewriting them for AYB compatibility and skipping policies on tables that failed schema migration.
func (m *Migrator) migrateRLSPolicies(ctx context.Context, tx *sql.Tx, phaseIdx, totalPhases int) error {
	phase := migrate.Phase{Name: "RLS policies", Index: phaseIdx, Total: totalPhases}
	m.progress.StartPhase(phase, 0)
	start := time.Now()

	fmt.Fprintln(m.output, "Migrating RLS policies...")

	policies, err := ReadRLSPolicies(ctx, m.source)
	if err != nil {
		return err
	}

	if len(policies) == 0 {
		fmt.Fprintln(m.output, "  no RLS policies found in user schemas")
		m.progress.CompletePhase(phase, 0, time.Since(start))
		return nil
	}

	for _, p := range policies {
		policyTable := TableInfo{SchemaName: p.SchemaName, Name: p.TableName}
		if !isAdmittedUserTable(p.SchemaName, p.TableName) {
			m.progress.Warn(fmt.Sprintf("skipping policy %s on %s: table is not admitted for migration", p.PolicyName, policyTable.QualifiedName()))
			continue
		}
		if m.isSkippedTable(policyTable) {
			m.progress.Warn(fmt.Sprintf("skipping policy %s on %s: table was skipped during schema migration", p.PolicyName, policyTable.QualifiedName()))
			continue
		}

		if m.verbose {
			fmt.Fprintf(m.output, "  %s.%s: %s\n", policyTable.QualifiedName(), p.PolicyName, p.Command)
		}

		// Drop existing policy on target (idempotent).
		dropSQL := fmt.Sprintf("DROP POLICY IF EXISTS %s ON %s",
			sqlutil.QuoteIdent(p.PolicyName), sqlutil.QuoteQualifiedName(p.SchemaName, p.TableName))
		if _, err := tx.ExecContext(ctx, dropSQL); err != nil {
			return fmt.Errorf("dropping policy %s on %s: %w", p.PolicyName, policyTable.QualifiedName(), err)
		}

		// Enable RLS on the table.
		enableSQL := fmt.Sprintf("ALTER TABLE %s ENABLE ROW LEVEL SECURITY",
			sqlutil.QuoteQualifiedName(p.SchemaName, p.TableName))
		if _, err := tx.ExecContext(ctx, enableSQL); err != nil {
			return fmt.Errorf("enabling RLS on %s: %w", policyTable.QualifiedName(), err)
		}

		// Create rewritten policy.
		rewrittenSQL := GenerateRewrittenPolicy(p)
		if _, err := tx.ExecContext(ctx, rewrittenSQL); err != nil {
			return fmt.Errorf("creating policy %s on %s: %w", p.PolicyName, policyTable.QualifiedName(), err)
		}
		m.stats.Policies++
		m.progress.Progress(phase, m.stats.Policies, len(policies))
	}

	m.progress.CompletePhase(phase, m.stats.Policies, time.Since(start))
	fmt.Fprintf(m.output, "  ✓ %d RLS policies rewritten\n", m.stats.Policies)
	return nil
}
