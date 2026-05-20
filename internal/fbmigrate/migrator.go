// Package fbmigrate orchestrates migration from Firebase exports into AYB databases, handling Auth, Firestore, RTDB, and Storage datasets.
package fbmigrate

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"

	"github.com/allyourbase/ayb/internal/migrate"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// Migrator orchestrates Firebase → AYB migration.
type Migrator struct {
	db       *sql.DB
	opts     MigrationOptions
	stats    MigrationStats
	output   io.Writer
	verbose  bool
	progress migrate.ProgressReporter
}

// NewMigrator creates a new Firebase migrator, validating options and connecting to the target DB.
func NewMigrator(opts MigrationOptions) (*Migrator, error) {
	if opts.AuthExportPath == "" && opts.FirestoreExportPath == "" && opts.RTDBExportPath == "" && opts.StorageExportPath == "" {
		return nil, fmt.Errorf("at least one export path is required (auth, Firestore, RTDB, or storage)")
	}
	if opts.DatabaseURL == "" {
		return nil, fmt.Errorf("database URL is required")
	}

	// Validate auth export file exists if specified.
	if opts.AuthExportPath != "" {
		if _, err := os.Stat(opts.AuthExportPath); err != nil {
			return nil, fmt.Errorf("auth export file: %w", err)
		}
	}

	// Validate Firestore export directory exists if specified.
	if opts.FirestoreExportPath != "" {
		info, err := os.Stat(opts.FirestoreExportPath)
		if err != nil {
			return nil, fmt.Errorf("firestore export path: %w", err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("firestore export path must be a directory")
		}
	}

	// Validate RTDB export file exists if specified.
	if opts.RTDBExportPath != "" {
		if _, err := os.Stat(opts.RTDBExportPath); err != nil {
			return nil, fmt.Errorf("RTDB export file: %w", err)
		}
	}

	// Validate storage export directory exists if specified.
	if opts.StorageExportPath != "" {
		info, err := os.Stat(opts.StorageExportPath)
		if err != nil {
			return nil, fmt.Errorf("storage export path: %w", err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("storage export path must be a directory")
		}
	}

	db, err := sql.Open("pgx", opts.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("connecting to database: %w", err)
	}
	if err := db.PingContext(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	output := io.Writer(os.Stdout)
	if opts.DryRun && !opts.Verbose {
		output = io.Discard
	}

	progress := opts.Progress
	if progress == nil {
		progress = migrate.NopReporter{}
	}

	return &Migrator{
		db:       db,
		opts:     opts,
		output:   output,
		verbose:  opts.Verbose,
		progress: progress,
	}, nil
}

// Close releases the database connection.
func (m *Migrator) Close() error {
	if m.db != nil {
		return m.db.Close()
	}
	return nil
}

// phaseCount returns the number of migration phases.
func (m *Migrator) phaseCount() int {
	n := 0
	if m.opts.AuthExportPath != "" {
		n += 2 // auth users + OAuth links
	}
	if m.opts.FirestoreExportPath != "" {
		n++ // Firestore data
	}
	if m.opts.RTDBExportPath != "" {
		n++ // Realtime Database
	}
	if m.opts.StorageExportPath != "" {
		n++ // Storage files
	}
	return n
}

// Migrate runs the full Firebase → AYB migration.
func (m *Migrator) Migrate(ctx context.Context) (*MigrationStats, error) {
	fmt.Fprintln(m.output, "Starting Firebase migration...")

	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	totalPhases := m.phaseCount()
	phaseIdx := 0

	// Phase: Auth users.
	if m.opts.AuthExportPath != "" {
		users, hashConfig, err := ParseAuthExport(m.opts.AuthExportPath)
		if err != nil {
			return nil, err
		}

		// Use provided hash config or the one from the export.
		if m.opts.HashConfig != nil {
			hashConfig = m.opts.HashConfig
		}

		phaseIdx++
		if err := m.migrateAuthUsers(ctx, tx, users, hashConfig, phaseIdx, totalPhases); err != nil {
			return nil, fmt.Errorf("auth migration: %w", err)
		}

		phaseIdx++
		if err := m.migrateOAuthLinks(ctx, tx, users, phaseIdx, totalPhases); err != nil {
			return nil, fmt.Errorf("OAuth migration: %w", err)
		}
	}

	// Phase: Firestore data.
	if m.opts.FirestoreExportPath != "" {
		phaseIdx++
		if err := m.migrateFirestoreData(ctx, tx, phaseIdx, totalPhases); err != nil {
			return nil, fmt.Errorf("firestore migration: %w", err)
		}
	}

	// Phase: Realtime Database.
	if m.opts.RTDBExportPath != "" {
		phaseIdx++
		if err := m.migrateRTDB(ctx, tx, phaseIdx, totalPhases); err != nil {
			return nil, fmt.Errorf("RTDB migration: %w", err)
		}
	}

	// Commit DB transaction before filesystem operations.
	if m.opts.DryRun {
		fmt.Fprintln(m.output, "\n[DRY RUN] Rolling back (no changes made)")
	} else {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("committing transaction: %w", err)
		}
	}

	// Phase: Storage files (outside transaction — filesystem operations).
	if m.opts.StorageExportPath != "" && !m.opts.DryRun {
		phaseIdx++
		if err := m.migrateStorage(phaseIdx, totalPhases); err != nil {
			return nil, fmt.Errorf("storage migration: %w", err)
		}
	}

	fmt.Fprintln(m.output, "\nMigration complete!")
	m.printStats()

	return &m.stats, nil
}

// Analyze performs pre-flight analysis of the Firebase export.
func (m *Migrator) Analyze(ctx context.Context) (*migrate.AnalysisReport, error) {
	report := &migrate.AnalysisReport{
		SourceType: "Firebase",
	}

	if m.opts.AuthExportPath != "" {
		users, _, err := ParseAuthExport(m.opts.AuthExportPath)
		if err != nil {
			return nil, fmt.Errorf("parsing auth export: %w", err)
		}
		report.SourceInfo = m.opts.AuthExportPath

		for _, u := range users {
			if u.Disabled {
				continue
			}
			if IsAnonymousUser(u) || IsPhoneOnlyUser(u) {
				continue
			}
			if !IsEmailUser(u) {
				continue
			}
			report.AuthUsers++
			// Count OAuth links only for email users, matching migrateOAuthLinks() behavior.
			for range OAuthProviders(u) {
				report.OAuthLinks++
			}
		}
	}

	if m.opts.FirestoreExportPath != "" {
		collections, err := ParseFirestoreExport(m.opts.FirestoreExportPath)
		if err != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("could not read Firestore export: %v", err))
		} else {
			report.Tables = len(collections)
			for _, c := range collections {
				report.Records += len(c.Documents)
			}
		}
	}

	// Analyze RTDB export.
	if m.opts.RTDBExportPath != "" {
		nodes, err := ParseRTDBExport(m.opts.RTDBExportPath)
		if err != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("could not read RTDB export: %v", err))
		} else {
			report.Tables += len(nodes)
			for _, n := range nodes {
				report.Records += len(n.Children)
			}
		}
	}

	// Analyze storage export.
	if m.opts.StorageExportPath != "" {
		buckets, err := scanStorageExport(m.opts.StorageExportPath)
		if err != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("could not scan storage export: %v", err))
		} else {
			for _, files := range buckets {
				report.Files += len(files)
				for _, f := range files {
					report.FileSizeBytes += f.Size
				}
			}
		}
	}

	return report, nil
}

// BuildValidationSummary compares source analysis with migration stats.
func BuildValidationSummary(report *migrate.AnalysisReport, stats *MigrationStats) *migrate.ValidationSummary {
	summary := &migrate.ValidationSummary{
		SourceLabel: "Firebase (source)",
		TargetLabel: "AYB (target)",
	}

	if report.AuthUsers > 0 || stats.Users > 0 {
		summary.Rows = append(summary.Rows, migrate.ValidationRow{
			Label: "Auth users", SourceCount: report.AuthUsers, TargetCount: stats.Users,
		})
	}
	if report.OAuthLinks > 0 || stats.OAuthLinks > 0 {
		summary.Rows = append(summary.Rows, migrate.ValidationRow{
			Label: "OAuth links", SourceCount: report.OAuthLinks, TargetCount: stats.OAuthLinks,
		})
	}
	if report.Tables > 0 || stats.Collections > 0 || stats.RTDBNodes > 0 {
		summary.Rows = append(summary.Rows, migrate.ValidationRow{
			Label: "Collections", SourceCount: report.Tables, TargetCount: stats.Collections + stats.RTDBNodes,
		})
	}
	if report.Records > 0 || stats.Documents > 0 || stats.RTDBRecords > 0 {
		summary.Rows = append(summary.Rows, migrate.ValidationRow{
			Label: "Documents", SourceCount: report.Records, TargetCount: stats.Documents + stats.RTDBRecords,
		})
	}
	if stats.RTDBNodes > 0 {
		summary.Rows = append(summary.Rows, migrate.ValidationRow{
			Label: "RTDB nodes", SourceCount: stats.RTDBNodes, TargetCount: stats.RTDBNodes,
		})
	}
	if report.Files > 0 || stats.StorageFiles > 0 {
		summary.Rows = append(summary.Rows, migrate.ValidationRow{
			Label: "Storage files", SourceCount: report.Files, TargetCount: stats.StorageFiles,
		})
	}

	for _, row := range summary.Rows {
		if row.SourceCount != row.TargetCount {
			summary.Warnings = append(summary.Warnings,
				fmt.Sprintf("%s count mismatch: source=%d target=%d", row.Label, row.SourceCount, row.TargetCount))
		}
	}

	if stats.Skipped > 0 {
		summary.Warnings = append(summary.Warnings,
			fmt.Sprintf("%d items skipped during migration", stats.Skipped))
	}
	if len(stats.Errors) > 0 {
		summary.Warnings = append(summary.Warnings,
			fmt.Sprintf("%d errors occurred during migration", len(stats.Errors)))
	}

	return summary
}

// printStats outputs a formatted summary of migration results including user counts, OAuth links, collections, documents, storage files, skipped items, and any errors that occurred.
func (m *Migrator) printStats() {
	fmt.Fprintf(m.output, "\nSummary:\n")
	if m.stats.Users > 0 {
		fmt.Fprintf(m.output, "  Users:       %d\n", m.stats.Users)
	}
	if m.stats.OAuthLinks > 0 {
		fmt.Fprintf(m.output, "  OAuth:       %d\n", m.stats.OAuthLinks)
	}
	if m.stats.Collections > 0 {
		fmt.Fprintf(m.output, "  Collections: %d\n", m.stats.Collections)
	}
	if m.stats.Documents > 0 {
		fmt.Fprintf(m.output, "  Documents:   %d\n", m.stats.Documents)
	}
	if m.stats.RTDBNodes > 0 {
		fmt.Fprintf(m.output, "  RTDB nodes:  %d\n", m.stats.RTDBNodes)
	}
	if m.stats.RTDBRecords > 0 {
		fmt.Fprintf(m.output, "  RTDB records: %d\n", m.stats.RTDBRecords)
	}
	if m.stats.StorageFiles > 0 {
		fmt.Fprintf(m.output, "  Files:       %d (%s)\n", m.stats.StorageFiles, migrate.FormatBytes(m.stats.StorageBytes))
	}
	if m.stats.Skipped > 0 {
		fmt.Fprintf(m.output, "  Skipped:     %d\n", m.stats.Skipped)
	}
	if len(m.stats.Errors) > 0 {
		fmt.Fprintf(m.output, "  Errors:      %d\n", len(m.stats.Errors))
		for _, e := range m.stats.Errors {
			fmt.Fprintf(m.output, "    - %s\n", e)
		}
	}
}
