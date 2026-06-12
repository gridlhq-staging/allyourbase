package algoliamigrate

import (
	"context"
	"fmt"
	"strings"

	"github.com/allyourbase/ayb/internal/migrate"
)

// BuildAnalysisReport adapts the package plan into the shared migration report.
func BuildAnalysisReport(plan *ImportPlan) *migrate.AnalysisReport {
	if plan == nil {
		return &migrate.AnalysisReport{SourceType: "Algolia"}
	}
	warnings := settingsWarningMessages(plan.Settings.Stats)
	warnings = append(warnings, synonymWarningMessages(plan.Synonyms.Stats)...)
	return &migrate.AnalysisReport{
		SourceType:         "Algolia",
		Tables:             plan.DryRun.TablesPlanned,
		Records:            plan.Source.RecordCount,
		SettingsAttributes: plan.Settings.Stats.SupportedAttributes,
		SettingsRanking:    plan.Settings.Stats.SupportedCustomRanking,
		SynonymGroups:      plan.Synonyms.Stats.SupportedGroups,
		Warnings:           warnings,
	}
}

// BuildValidationSummary compares source analysis with import stats.
func BuildValidationSummary(report *migrate.AnalysisReport, stats *ImportStats) *migrate.ValidationSummary {
	summary := &migrate.ValidationSummary{
		SourceLabel: "Algolia (source)",
		TargetLabel: "AYB (target)",
	}
	if report == nil {
		report = &migrate.AnalysisReport{}
	}
	if stats == nil {
		stats = &ImportStats{}
	}
	summary.Rows = append(summary.Rows,
		migrate.ValidationRow{Label: "Tables", SourceCount: report.Tables, TargetCount: stats.Tables},
		migrate.ValidationRow{Label: "Records", SourceCount: report.Records, TargetCount: stats.Records},
	)
	if report.SettingsAttributes > 0 || stats.Settings.SupportedAttributes > 0 {
		summary.Rows = append(summary.Rows,
			migrate.ValidationRow{Label: "Settings attributes", SourceCount: report.SettingsAttributes, TargetCount: stats.Settings.SupportedAttributes})
	}
	if report.SettingsRanking > 0 || stats.Settings.SupportedCustomRanking > 0 {
		summary.Rows = append(summary.Rows,
			migrate.ValidationRow{Label: "Settings ranking", SourceCount: report.SettingsRanking, TargetCount: stats.Settings.SupportedCustomRanking})
	}
	if report.SynonymGroups > 0 || stats.Synonyms.SupportedGroups > 0 || hasSynonymSkips(stats.Synonyms) {
		summary.Rows = append(summary.Rows,
			migrate.ValidationRow{Label: "Synonym groups", SourceCount: report.SynonymGroups, TargetCount: stats.Synonyms.SupportedGroups})
	}
	for _, row := range summary.Rows {
		if row.SourceCount != row.TargetCount {
			summary.Warnings = append(summary.Warnings,
				fmt.Sprintf("%s count mismatch: source=%d target=%d", row.Label, row.SourceCount, row.TargetCount))
		}
	}
	if stats.Skipped > 0 {
		summary.Warnings = append(summary.Warnings,
			fmt.Sprintf("%d records skipped during import", stats.Skipped))
	}
	if len(stats.Errors) > 0 {
		summary.Warnings = append(summary.Warnings,
			fmt.Sprintf("%d errors occurred during import", len(stats.Errors)))
	}
	summary.Warnings = append(summary.Warnings, settingsWarningMessages(stats.Settings)...)
	summary.Warnings = append(summary.Warnings, synonymWarningMessages(stats.Synonyms)...)
	return summary
}

func hasSynonymSkips(stats SynonymStats) bool {
	return len(stats.SkippedUnsupportedTypes) > 0 ||
		stats.SkippedInvalidGroups > 0 ||
		stats.SkippedSettingsACL > 0 ||
		stats.SkippedMalformedHits > 0
}

// CheckRecordParity uses live browse credentials when present and otherwise
// compares fixture-backed source counts so normal tests never need secrets.
func CheckRecordParity(ctx context.Context, cfg BrowseConfig, fixture []Record, targetRecords int) (ParityResult, error) {
	source := "fixture"
	sourceRecords := len(fixture)
	if hasLiveBrowseCredentials(cfg) {
		client, err := NewBrowseClient(cfg)
		if err != nil {
			return ParityResult{}, err
		}
		result, err := client.Browse(ctx)
		if err != nil {
			return ParityResult{}, err
		}
		source = "live"
		sourceRecords = len(result.Records)
	}
	return ParityResult{
		Source:        source,
		SourceRecords: sourceRecords,
		TargetRecords: targetRecords,
		Match:         sourceRecords == targetRecords,
	}, nil
}

func hasLiveBrowseCredentials(cfg BrowseConfig) bool {
	return strings.TrimSpace(cfg.AppID) != "" &&
		strings.TrimSpace(cfg.APIKey) != "" &&
		strings.TrimSpace(cfg.IndexName) != ""
}
