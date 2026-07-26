package sbmigrate

import (
	"context"
	"fmt"
	"sort"

	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/allyourbase/ayb/internal/urlutil"
)

// Analyze performs a pre-flight analysis of the source database.
func (m *Migrator) Analyze(ctx context.Context) (*migrate.AnalysisReport, error) {
	report := &migrate.AnalysisReport{
		SourceType: "Supabase",
		SourceInfo: urlutil.RedactURL(m.opts.SourceURL),
	}

	// Count auth users (excluding anonymous, matching Migrate behavior).
	hasIsAnonymous, err := m.sourceColumnExists(ctx, "auth", "users", "is_anonymous")
	if err != nil {
		return nil, err
	}
	hasDeletedAt, err := m.sourceColumnExists(ctx, "auth", "users", "deleted_at")
	if err != nil {
		return nil, err
	}
	authQuery := buildAuthUsersCountQuery(m.opts.IncludeAnonymous, hasIsAnonymous, hasDeletedAt)
	err = m.source.QueryRowContext(ctx, authQuery).Scan(&report.AuthUsers)
	if err != nil {
		return nil, fmt.Errorf("counting auth users: %w", err)
	}

	// Count OAuth identities (excluding 'email' provider).
	err = m.source.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM auth.identities
		WHERE provider != 'email'
	`).Scan(&report.OAuthLinks)
	if err != nil {
		report.Warnings = append(report.Warnings, fmt.Sprintf("could not count OAuth identities: %v", err))
	}

	// Count RLS policies using the same admitted schema predicate as migration.
	policies, err := ReadRLSPolicies(ctx, m.source)
	if err != nil {
		report.Warnings = append(report.Warnings, fmt.Sprintf("could not count RLS policies: %v", err))
	} else {
		report.RLSPolicies = countAdmittedRLSPolicies(policies)
	}

	// Count public tables and total rows.
	if !m.opts.SkipData {
		tables, err := introspectTables(ctx, m.source)
		if err != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("could not introspect tables: %v", err))
		} else {
			report.Tables = len(tables)
			for _, t := range tables {
				report.Records += int(t.RowCount)
			}
		}

		views, err := introspectViews(ctx, m.source)
		if err != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("could not introspect views: %v", err))
		} else {
			report.Views = len(views)
		}
	}

	// Count storage objects.
	if !m.opts.SkipStorage {
		buckets, err := m.listStorageBuckets(ctx)
		if err != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("could not list storage buckets: %v", err))
		} else {
			for _, b := range buckets {
				objects, err := m.listStorageObjects(ctx, b.ID)
				if err != nil {
					report.Warnings = append(report.Warnings,
						fmt.Sprintf("could not list objects in bucket %s: %v", b.Name, err))
					continue
				}
				report.Files += len(objects)
				for _, o := range objects {
					report.FileSizeBytes += o.Size
				}
			}
		}
	}

	return report, nil
}

// BuildValidationSummary compares source analysis with migration stats.
func BuildValidationSummary(report *migrate.AnalysisReport, stats *MigrationStats) *migrate.ValidationSummary {
	summary := &migrate.ValidationSummary{
		SourceLabel: "Supabase (source)",
		TargetLabel: "AYB (target)",
	}

	if report.Tables > 0 || stats.Tables > 0 {
		summary.Rows = append(summary.Rows, migrate.ValidationRow{
			Label: "Tables", SourceCount: report.Tables, TargetCount: stats.Tables,
		})
	}
	if report.Views > 0 || stats.Views > 0 {
		summary.Rows = append(summary.Rows, migrate.ValidationRow{
			Label: "Views", SourceCount: report.Views, TargetCount: stats.Views,
		})
	}
	if report.Records > 0 || stats.Records > 0 {
		summary.Rows = append(summary.Rows, migrate.ValidationRow{
			Label: "Records", SourceCount: report.Records, TargetCount: stats.Records,
		})
	}
	summary.Rows = append(summary.Rows, migrate.ValidationRow{
		Label: "Auth users", SourceCount: report.AuthUsers, TargetCount: stats.Users,
	})
	if report.OAuthLinks > 0 || stats.OAuthLinks > 0 {
		summary.Rows = append(summary.Rows, migrate.ValidationRow{
			Label: "OAuth links", SourceCount: report.OAuthLinks, TargetCount: stats.OAuthLinks,
		})
	}
	if report.RLSPolicies > 0 || stats.Policies > 0 {
		summary.Rows = append(summary.Rows, migrate.ValidationRow{
			Label: "RLS policies", SourceCount: report.RLSPolicies, TargetCount: stats.Policies,
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
	for _, table := range sortedSkippedTableReports(stats.SkippedTables) {
		summary.Warnings = append(summary.Warnings,
			fmt.Sprintf("table %s skipped during migration: %s", table.QualifiedName(), table.Reason))
	}
	if len(stats.Errors) > 0 {
		summary.Warnings = append(summary.Warnings,
			fmt.Sprintf("%d errors occurred during migration", len(stats.Errors)))
	}

	return summary
}

func countAdmittedRLSPolicies(policies []RLSPolicy) int {
	count := 0
	for _, policy := range policies {
		if isAdmittedUserTable(policy.SchemaName, policy.TableName) {
			count++
		}
	}
	return count
}

func sortedSkippedTableReports(skipped SkippedTableReasons) []SkippedTableReport {
	reports := make([]SkippedTableReport, 0, len(skipped))
	for key, reason := range skipped {
		reports = append(reports, skippedTableReport(key, reason))
	}
	sort.Slice(reports, func(i, j int) bool {
		if reports[i].SchemaName != reports[j].SchemaName {
			return reports[i].SchemaName < reports[j].SchemaName
		}
		if reports[i].TableName != reports[j].TableName {
			return reports[i].TableName < reports[j].TableName
		}
		return reports[i].Reason < reports[j].Reason
	})
	return reports
}
