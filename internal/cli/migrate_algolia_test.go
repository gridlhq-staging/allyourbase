package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/algoliamigrate"
	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/spf13/cobra"
)

type fakeAlgoliaMigrator struct {
	analyzeFn func(context.Context) (*migrate.AnalysisReport, error)
	migrateFn func(context.Context) (*algoliamigrate.ImportStats, error)
	closeFn   func() error
}

func (f fakeAlgoliaMigrator) Analyze(ctx context.Context) (*migrate.AnalysisReport, error) {
	if f.analyzeFn != nil {
		return f.analyzeFn(ctx)
	}
	return &migrate.AnalysisReport{SourceType: "Algolia"}, nil
}

func (f fakeAlgoliaMigrator) Migrate(ctx context.Context) (*algoliamigrate.ImportStats, error) {
	if f.migrateFn != nil {
		return f.migrateFn(ctx)
	}
	return &algoliamigrate.ImportStats{}, nil
}

func (f fakeAlgoliaMigrator) Close() error {
	if f.closeFn != nil {
		return f.closeFn()
	}
	return nil
}

func newAlgoliaTestCommand(t *testing.T, values map[string]string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.Flags().String("app-id", "", "")
	cmd.Flags().String("api-key", "", "")
	cmd.Flags().String("index", "", "")
	cmd.Flags().String("database-url", "", "")
	cmd.Flags().String("table", "", "")
	cmd.Flags().Bool("include-synonyms", false, "")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().Bool("yes", false, "")
	cmd.Flags().Bool("json", false, "")
	for k, v := range values {
		testutil.NoError(t, cmd.Flags().Set(k, v))
	}
	return cmd
}

func TestRunMigrateAlgoliaForwardsRequiredFlagsAndSynonyms(t *testing.T) {
	oldFactory := newAlgoliaMigrator
	t.Cleanup(func() { newAlgoliaMigrator = oldFactory })

	var got algoliaMigrationOptions
	newAlgoliaMigrator = func(opts algoliaMigrationOptions) (algoliaMigrator, error) {
		got = opts
		return fakeAlgoliaMigrator{}, nil
	}

	cmd := newAlgoliaTestCommand(t, map[string]string{
		"app-id":           "APP123",
		"api-key":          "key",
		"index":            "products",
		"database-url":     "postgres://target",
		"table":            "search_products",
		"include-synonyms": "true",
		"yes":              "true",
	})

	_ = captureStderr(t, func() {
		err := runMigrateAlgolia(cmd, nil)
		testutil.NoError(t, err)
	})

	testutil.Equal(t, "APP123", got.AppID)
	testutil.Equal(t, "key", got.APIKey)
	testutil.Equal(t, "products", got.IndexName)
	testutil.Equal(t, "postgres://target", got.DatabaseURL)
	testutil.Equal(t, "search_products", got.TargetTable)
	testutil.True(t, got.IncludeSynonyms, "expected include-synonyms to be forwarded")
}

func TestRunMigrateAlgoliaPreflightPromptAndSummary(t *testing.T) {
	oldFactory := newAlgoliaMigrator
	oldSummary := buildAlgoliaValidationSummary
	t.Cleanup(func() {
		newAlgoliaMigrator = oldFactory
		buildAlgoliaValidationSummary = oldSummary
	})

	callOrder := make([]string, 0, 2)
	newAlgoliaMigrator = func(opts algoliaMigrationOptions) (algoliaMigrator, error) {
		return fakeAlgoliaMigrator{
			analyzeFn: func(context.Context) (*migrate.AnalysisReport, error) {
				callOrder = append(callOrder, "analyze")
				return &migrate.AnalysisReport{SourceType: "Algolia", Tables: 1, Records: 2}, nil
			},
			migrateFn: func(context.Context) (*algoliamigrate.ImportStats, error) {
				callOrder = append(callOrder, "migrate")
				return &algoliamigrate.ImportStats{Tables: 1, Records: 2}, nil
			},
		}, nil
	}
	buildAlgoliaValidationSummary = func(report *migrate.AnalysisReport, stats *algoliamigrate.ImportStats) *migrate.ValidationSummary {
		return &migrate.ValidationSummary{
			SourceLabel: "Algolia (source)",
			TargetLabel: "AYB (target)",
			Rows: []migrate.ValidationRow{{
				Label:       "Records",
				SourceCount: report.Records,
				TargetCount: stats.Records,
			}},
		}
	}

	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	testutil.NoError(t, err)
	_, err = w.WriteString("yes\n")
	testutil.NoError(t, err)
	testutil.NoError(t, w.Close())
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = oldStdin
		_ = r.Close()
	})

	cmd := newAlgoliaTestCommand(t, requiredAlgoliaFlagValues())
	output := captureStderr(t, func() {
		err := runMigrateAlgolia(cmd, nil)
		testutil.NoError(t, err)
	})

	if !reflect.DeepEqual(callOrder, []string{"analyze", "migrate"}) {
		t.Fatalf("unexpected call order: %v", callOrder)
	}
	testutil.Contains(t, output, "AYB Migration Report")
	testutil.Contains(t, output, "Proceed? [Y/n]")
	testutil.Contains(t, output, "Validation Summary")
}

func TestRunMigrateAlgoliaYesSkipsPrompt(t *testing.T) {
	oldFactory := newAlgoliaMigrator
	t.Cleanup(func() { newAlgoliaMigrator = oldFactory })

	newAlgoliaMigrator = func(opts algoliaMigrationOptions) (algoliaMigrator, error) {
		return fakeAlgoliaMigrator{}, nil
	}

	values := requiredAlgoliaFlagValues()
	values["yes"] = "true"
	cmd := newAlgoliaTestCommand(t, values)
	output := captureStderr(t, func() {
		err := runMigrateAlgolia(cmd, nil)
		testutil.NoError(t, err)
	})
	testutil.False(t, strings.Contains(output, "Proceed? [Y/n]"), "--yes should skip prompt")
}

func TestRunMigrateAlgoliaDryRunSkipsPromptAndSummary(t *testing.T) {
	oldFactory := newAlgoliaMigrator
	oldSummary := buildAlgoliaValidationSummary
	t.Cleanup(func() {
		newAlgoliaMigrator = oldFactory
		buildAlgoliaValidationSummary = oldSummary
	})

	summaryCalled := false
	newAlgoliaMigrator = func(opts algoliaMigrationOptions) (algoliaMigrator, error) {
		testutil.True(t, opts.DryRun, "expected dry-run to be forwarded")
		return fakeAlgoliaMigrator{
			analyzeFn: func(context.Context) (*migrate.AnalysisReport, error) {
				return &migrate.AnalysisReport{SourceType: "Algolia", Tables: 1, Records: 2}, nil
			},
			migrateFn: func(context.Context) (*algoliamigrate.ImportStats, error) {
				return &algoliamigrate.ImportStats{Tables: 1, Records: 2, DryRun: true}, nil
			},
		}, nil
	}
	buildAlgoliaValidationSummary = func(report *migrate.AnalysisReport, stats *algoliamigrate.ImportStats) *migrate.ValidationSummary {
		summaryCalled = true
		return &migrate.ValidationSummary{}
	}

	values := requiredAlgoliaFlagValues()
	values["dry-run"] = "true"
	cmd := newAlgoliaTestCommand(t, values)
	output := captureStderr(t, func() {
		err := runMigrateAlgolia(cmd, nil)
		testutil.NoError(t, err)
	})

	testutil.Contains(t, output, "AYB Migration Report")
	testutil.False(t, strings.Contains(output, "Proceed? [Y/n]"), "dry-run should skip prompt")
	testutil.False(t, summaryCalled, "dry-run should not print post-run validation summary")
}

func TestAlgoliaAdapterDryRunStillValidatesTargetDB(t *testing.T) {
	adapter := &algoliaCLIAdapter{
		opts: algoliaMigrationOptions{
			DatabaseURL: "not a postgres url",
			TargetTable: "search_products",
			DryRun:      true,
		},
		browse: &algoliamigrate.BrowseResult{},
		plan:   &algoliamigrate.ImportPlan{},
		report: &migrate.AnalysisReport{SourceType: "Algolia"},
	}

	_, err := adapter.Migrate(context.Background())
	if err == nil {
		t.Fatal("expected dry-run to validate the target database connection")
	}
	testutil.Contains(t, err.Error(), "database")
}

func TestRunMigrateAlgoliaPromptReadErrorFailsClosed(t *testing.T) {
	oldFactory := newAlgoliaMigrator
	t.Cleanup(func() { newAlgoliaMigrator = oldFactory })

	migrateCalled := false
	newAlgoliaMigrator = func(opts algoliaMigrationOptions) (algoliaMigrator, error) {
		return fakeAlgoliaMigrator{
			analyzeFn: func(context.Context) (*migrate.AnalysisReport, error) {
				return &migrate.AnalysisReport{SourceType: "Algolia", Tables: 1, Records: 2}, nil
			},
			migrateFn: func(context.Context) (*algoliamigrate.ImportStats, error) {
				migrateCalled = true
				return &algoliamigrate.ImportStats{Tables: 1, Records: 2}, nil
			},
		}, nil
	}

	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	testutil.NoError(t, err)
	testutil.NoError(t, w.Close())
	testutil.NoError(t, r.Close())
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = oldStdin })

	cmd := newAlgoliaTestCommand(t, requiredAlgoliaFlagValues())
	_ = captureStderr(t, func() {
		err := runMigrateAlgolia(cmd, nil)
		if err == nil {
			t.Fatal("expected prompt read failure to abort migration")
		}
		if !errors.Is(err, os.ErrClosed) {
			t.Fatalf("expected closed stdin error, got %v", err)
		}
	})

	testutil.False(t, migrateCalled, "stdin read failure must not proceed with migration")
}

func TestRunMigrateAlgoliaJSONOutputsOnlyStats(t *testing.T) {
	oldFactory := newAlgoliaMigrator
	t.Cleanup(func() { newAlgoliaMigrator = oldFactory })

	newAlgoliaMigrator = func(opts algoliaMigrationOptions) (algoliaMigrator, error) {
		if _, ok := opts.Progress.(migrate.NopReporter); !ok {
			t.Fatalf("json mode should use migrate.NopReporter, got %T", opts.Progress)
		}
		return fakeAlgoliaMigrator{
			analyzeFn: func(context.Context) (*migrate.AnalysisReport, error) {
				return &migrate.AnalysisReport{SourceType: "Algolia", Tables: 1, Records: 2}, nil
			},
			migrateFn: func(context.Context) (*algoliamigrate.ImportStats, error) {
				return &algoliamigrate.ImportStats{Tables: 1, Records: 2}, nil
			},
		}, nil
	}

	values := requiredAlgoliaFlagValues()
	values["json"] = "true"
	cmd := newAlgoliaTestCommand(t, values)
	var stdout string
	stderr := captureStderr(t, func() {
		stdout = captureStdout(t, func() {
			err := runMigrateAlgolia(cmd, nil)
			testutil.NoError(t, err)
		})
	})

	testutil.Equal(t, "", strings.TrimSpace(stderr))
	var stats algoliamigrate.ImportStats
	testutil.NoError(t, json.Unmarshal([]byte(stdout), &stats))
	testutil.Equal(t, 1, stats.Tables)
	testutil.Equal(t, 2, stats.Records)
}

func requiredAlgoliaFlagValues() map[string]string {
	return map[string]string{
		"app-id":       "APP123",
		"api-key":      "key",
		"index":        "products",
		"database-url": "postgres://target",
		"table":        "search_products",
	}
}
