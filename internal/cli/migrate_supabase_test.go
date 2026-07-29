package cli

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/allyourbase/ayb/internal/sbmigrate"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/spf13/cobra"
)

type fakeSupabaseMigrator struct {
	analyzeFn func(context.Context) (*migrate.AnalysisReport, error)
	migrateFn func(context.Context) (*sbmigrate.MigrationStats, error)
	closeFn   func() error
}

func (f fakeSupabaseMigrator) Analyze(ctx context.Context) (*migrate.AnalysisReport, error) {
	if f.analyzeFn != nil {
		return f.analyzeFn(ctx)
	}
	return &migrate.AnalysisReport{SourceType: "Supabase"}, nil
}

func (f fakeSupabaseMigrator) Migrate(ctx context.Context) (*sbmigrate.MigrationStats, error) {
	if f.migrateFn != nil {
		return f.migrateFn(ctx)
	}
	return &sbmigrate.MigrationStats{}, nil
}

func (f fakeSupabaseMigrator) Close() error {
	if f.closeFn != nil {
		return f.closeFn()
	}
	return nil
}

func newSupabaseTestCommand(t *testing.T, values map[string]string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.Flags().String("source-url", "", "")
	cmd.Flags().String("database-url", "", "")
	cmd.Flags().String("storage-export", "", "")
	cmd.Flags().String("storage-path", "", "")
	cmd.Flags().String("storage-s3-endpoint", "", "")
	cmd.Flags().String("storage-s3-region", "", "")
	cmd.Flags().String("storage-s3-access-key", "", "")
	cmd.Flags().String("storage-s3-secret-key", "", "")
	cmd.Flags().Bool("storage-s3-use-ssl", true, "")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().Bool("force", false, "")
	cmd.Flags().Bool("verbose", false, "")
	cmd.Flags().Bool("skip-rls", false, "")
	cmd.Flags().Bool("skip-oauth", false, "")
	cmd.Flags().Bool("skip-data", false, "")
	cmd.Flags().Bool("skip-functions", false, "")
	cmd.Flags().Bool("skip-storage", false, "")
	cmd.Flags().Bool("include-anonymous", false, "")
	cmd.Flags().Bool("yes", false, "")
	cmd.Flags().Bool("json", false, "")
	for k, v := range values {
		testutil.NoError(t, cmd.Flags().Set(k, v))
	}
	return cmd
}

func supabaseS3TestFlags() map[string]string {
	return map[string]string{
		"storage-s3-endpoint":   "minio.internal:9000",
		"storage-s3-region":     "us-east-1",
		"storage-s3-access-key": "access",
		"storage-s3-secret-key": "secret",
		"storage-s3-use-ssl":    "false",
	}
}

func TestMigrateSupabaseForwardsStorageFlags(t *testing.T) {
	oldFactory := newSupabaseMigrator
	t.Cleanup(func() { newSupabaseMigrator = oldFactory })

	var got sbmigrate.MigrationOptions
	newSupabaseMigrator = func(opts sbmigrate.MigrationOptions) (supabaseMigrator, error) {
		got = opts
		return fakeSupabaseMigrator{}, nil
	}

	cmd := newSupabaseTestCommand(t, map[string]string{
		"source-url":     "postgres://source",
		"database-url":   "postgres://target",
		"storage-export": "./export",
		"storage-path":   "./storage",
		"skip-storage":   "true",
		"yes":            "true",
	})

	_ = captureStderr(t, func() {
		err := runMigrateSupabase(cmd, nil)
		testutil.NoError(t, err)
	})
	testutil.Equal(t, "./export", got.StorageExportPath)
	testutil.Equal(t, "./storage", got.StoragePath)
	testutil.True(t, got.SkipStorage, "expected skip-storage to be forwarded")
}

func TestMigrateSupabaseForwardsS3StorageFlags(t *testing.T) {
	oldFactory := newSupabaseMigrator
	t.Cleanup(func() { newSupabaseMigrator = oldFactory })

	var got sbmigrate.MigrationOptions
	newSupabaseMigrator = func(opts sbmigrate.MigrationOptions) (supabaseMigrator, error) {
		got = opts
		return fakeSupabaseMigrator{}, nil
	}

	flags := supabaseS3TestFlags()
	flags["source-url"] = "postgres://source"
	flags["database-url"] = "postgres://target"
	flags["storage-path"] = "./storage"
	flags["yes"] = "true"

	_ = captureStderr(t, func() {
		err := runMigrateSupabase(newSupabaseTestCommand(t, flags), nil)
		testutil.NoError(t, err)
	})

	testutil.Equal(t, "minio.internal:9000", got.StorageS3Endpoint)
	testutil.Equal(t, "us-east-1", got.StorageS3Region)
	testutil.Equal(t, "access", got.StorageS3AccessKey)
	testutil.Equal(t, "secret", got.StorageS3SecretKey)
	testutil.False(t, got.StorageS3UseSSL, "expected explicit HTTP setting to be forwarded")
}

func TestMigrateSupabaseRejectsStorageExportWithAnyS3Flag(t *testing.T) {
	s3Flags := []string{
		"storage-s3-endpoint",
		"storage-s3-region",
		"storage-s3-access-key",
		"storage-s3-secret-key",
		"storage-s3-use-ssl",
	}
	for _, s3Flag := range s3Flags {
		t.Run(s3Flag, func(t *testing.T) {
			oldFactory := newSupabaseMigrator
			t.Cleanup(func() { newSupabaseMigrator = oldFactory })

			factoryCalled := false
			newSupabaseMigrator = func(opts sbmigrate.MigrationOptions) (supabaseMigrator, error) {
				factoryCalled = true
				return fakeSupabaseMigrator{}, nil
			}
			flags := map[string]string{
				"source-url":     "postgres://source",
				"database-url":   "postgres://target",
				"storage-export": "./export",
				s3Flag:           "set",
			}
			if s3Flag == "storage-s3-use-ssl" {
				flags[s3Flag] = "false"
			}

			err := runMigrateSupabase(newSupabaseTestCommand(t, flags), nil)

			testutil.ErrorContains(t, err, "--storage-export cannot be combined with --storage-s3-*")
			testutil.False(t, factoryCalled, "validation must run before constructing the migrator")
		})
	}
}

func TestMigrateSupabaseRejectsIncompleteS3Source(t *testing.T) {
	tests := []struct {
		name    string
		missing string
	}{
		{name: "missing endpoint", missing: "storage-s3-endpoint"},
		{name: "missing region", missing: "storage-s3-region"},
		{name: "missing access key", missing: "storage-s3-access-key"},
		{name: "missing secret key", missing: "storage-s3-secret-key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldFactory := newSupabaseMigrator
			t.Cleanup(func() { newSupabaseMigrator = oldFactory })

			factoryCalled := false
			newSupabaseMigrator = func(opts sbmigrate.MigrationOptions) (supabaseMigrator, error) {
				factoryCalled = true
				return fakeSupabaseMigrator{}, nil
			}
			flags := supabaseS3TestFlags()
			delete(flags, tt.missing)
			flags["source-url"] = "postgres://source"
			flags["database-url"] = "postgres://target"

			err := runMigrateSupabase(newSupabaseTestCommand(t, flags), nil)

			testutil.ErrorContains(t, err, "--"+tt.missing+" is required when using --storage-s3-*")
			testutil.False(t, factoryCalled, "validation must run before constructing the migrator")
		})
	}
}

func TestMigrateSupabaseForwardsSkipFunctionsFlag(t *testing.T) {
	oldFactory := newSupabaseMigrator
	t.Cleanup(func() { newSupabaseMigrator = oldFactory })

	var got sbmigrate.MigrationOptions
	newSupabaseMigrator = func(opts sbmigrate.MigrationOptions) (supabaseMigrator, error) {
		got = opts
		return fakeSupabaseMigrator{}, nil
	}

	cmd := newSupabaseTestCommand(t, map[string]string{
		"source-url":     "postgres://source",
		"database-url":   "postgres://target",
		"skip-functions": "true",
		"yes":            "true",
	})

	_ = captureStderr(t, func() {
		err := runMigrateSupabase(cmd, nil)
		testutil.NoError(t, err)
	})

	testutil.True(t, got.SkipFunctions, "expected skip-functions to be forwarded")
}

func TestMigrateSupabaseSkipFunctionsDefaultsFalse(t *testing.T) {
	oldFactory := newSupabaseMigrator
	t.Cleanup(func() { newSupabaseMigrator = oldFactory })

	var got sbmigrate.MigrationOptions
	newSupabaseMigrator = func(opts sbmigrate.MigrationOptions) (supabaseMigrator, error) {
		got = opts
		return fakeSupabaseMigrator{}, nil
	}

	cmd := newSupabaseTestCommand(t, map[string]string{
		"source-url":   "postgres://source",
		"database-url": "postgres://target",
		"yes":          "true",
	})

	_ = captureStderr(t, func() {
		err := runMigrateSupabase(cmd, nil)
		testutil.NoError(t, err)
	})

	testutil.False(t, got.SkipFunctions, "skip-functions should default false")
}

func TestMigrateSupabaseSkipDataForcesSkipFunctions(t *testing.T) {
	oldFactory := newSupabaseMigrator
	t.Cleanup(func() { newSupabaseMigrator = oldFactory })

	var got sbmigrate.MigrationOptions
	newSupabaseMigrator = func(opts sbmigrate.MigrationOptions) (supabaseMigrator, error) {
		got = opts
		return fakeSupabaseMigrator{}, nil
	}

	cmd := newSupabaseTestCommand(t, map[string]string{
		"source-url":   "postgres://source",
		"database-url": "postgres://target",
		"skip-data":    "true",
		"yes":          "true",
	})

	_ = captureStderr(t, func() {
		err := runMigrateSupabase(cmd, nil)
		testutil.NoError(t, err)
	})

	testutil.True(t, got.SkipData, "expected skip-data to be forwarded")
	testutil.True(t, got.SkipFunctions, "skip-data must imply skip-functions")
}

func TestMigrateSupabaseHelpDescribesStorageAtomicity(t *testing.T) {
	testutil.False(t,
		strings.Contains(migrateSupabaseCmd.Long, "either everything succeeds or\nnothing is changed"),
		"help must not promise transactionality for storage files and metadata",
	)
	testutil.Contains(t, migrateSupabaseCmd.Long,
		"Database migration runs in a transaction. Storage files and metadata are migrated afterward")
	testutil.Contains(t, migrateSupabaseCmd.Long, "--storage-s3-endpoint")
	testutil.Contains(t, migrateSupabaseCmd.Long, "source bucket names come from the Supabase PostgreSQL inventory")
	testutil.Contains(t, migrateSupabaseCmd.Long, "--storage-export and --storage-s3-* cannot be combined")
}

func TestRunMigrateSupabasePreflightPromptAndSummary(t *testing.T) {
	oldFactory := newSupabaseMigrator
	oldSummary := buildSupabaseValidationSummary
	t.Cleanup(func() {
		newSupabaseMigrator = oldFactory
		buildSupabaseValidationSummary = oldSummary
	})

	callOrder := make([]string, 0, 2)
	newSupabaseMigrator = func(opts sbmigrate.MigrationOptions) (supabaseMigrator, error) {
		return fakeSupabaseMigrator{
			analyzeFn: func(context.Context) (*migrate.AnalysisReport, error) {
				callOrder = append(callOrder, "analyze")
				return &migrate.AnalysisReport{SourceType: "Supabase", AuthUsers: 2}, nil
			},
			migrateFn: func(context.Context) (*sbmigrate.MigrationStats, error) {
				callOrder = append(callOrder, "migrate")
				return &sbmigrate.MigrationStats{Users: 2}, nil
			},
		}, nil
	}
	buildSupabaseValidationSummary = func(report *migrate.AnalysisReport, stats *sbmigrate.MigrationStats) *migrate.ValidationSummary {
		return &migrate.ValidationSummary{
			SourceLabel: "Supabase (source)",
			TargetLabel: "AYB (target)",
			Rows: []migrate.ValidationRow{{
				Label:       "Auth users",
				SourceCount: report.AuthUsers,
				TargetCount: stats.Users,
			}},
		}
	}

	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	testutil.NoError(t, err)
	_, err = w.WriteString("y\n")
	testutil.NoError(t, err)
	testutil.NoError(t, w.Close())
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = oldStdin
		_ = r.Close()
	})

	cmd := newSupabaseTestCommand(t, map[string]string{
		"source-url":   "postgres://source",
		"database-url": "postgres://target",
	})

	output := captureStderr(t, func() {
		err := runMigrateSupabase(cmd, nil)
		testutil.NoError(t, err)
	})

	if !reflect.DeepEqual(callOrder, []string{"analyze", "migrate"}) {
		t.Fatalf("unexpected call order: %v", callOrder)
	}
	testutil.Contains(t, output, "AYB Migration Report")
	testutil.Contains(t, output, "Proceed? [Y/n]")
	testutil.Contains(t, output, "Validation Summary")
}

func TestRunMigrateSupabaseWarnsAboutMissingStorageSource(t *testing.T) {
	oldFactory := newSupabaseMigrator
	t.Cleanup(func() { newSupabaseMigrator = oldFactory })

	newSupabaseMigrator = func(opts sbmigrate.MigrationOptions) (supabaseMigrator, error) {
		return fakeSupabaseMigrator{
			analyzeFn: func(context.Context) (*migrate.AnalysisReport, error) {
				return &migrate.AnalysisReport{SourceType: "Supabase", Files: 7}, nil
			},
		}, nil
	}

	const warning = "7 analyzed storage files will not migrate; supply --storage-export or --storage-s3-* credentials, or intentionally choose --skip-storage."
	tests := []struct {
		name         string
		flags        map[string]string
		warningCount int
	}{
		{
			name:         "missing export",
			flags:        map[string]string{},
			warningCount: 1,
		},
		{
			name:         "storage explicitly skipped",
			flags:        map[string]string{"skip-storage": "true"},
			warningCount: 0,
		},
		{
			name:         "storage export supplied",
			flags:        map[string]string{"storage-export": "./supabase-storage"},
			warningCount: 0,
		},
		{
			name:         "S3 source supplied",
			flags:        supabaseS3TestFlags(),
			warningCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags := map[string]string{
				"source-url":   "postgres://source",
				"database-url": "postgres://target",
				"yes":          "true",
			}
			for name, value := range tt.flags {
				flags[name] = value
			}

			output := captureStderr(t, func() {
				err := runMigrateSupabase(newSupabaseTestCommand(t, flags), nil)
				testutil.NoError(t, err)
			})

			testutil.Equal(t, tt.warningCount, strings.Count(output, warning))
		})
	}
}

func TestRunMigrateSupabaseJSONOutputsStats(t *testing.T) {
	oldFactory := newSupabaseMigrator
	t.Cleanup(func() { newSupabaseMigrator = oldFactory })

	newSupabaseMigrator = func(opts sbmigrate.MigrationOptions) (supabaseMigrator, error) {
		return fakeSupabaseMigrator{
			analyzeFn: func(context.Context) (*migrate.AnalysisReport, error) {
				return &migrate.AnalysisReport{SourceType: "Supabase", AuthUsers: 1}, nil
			},
			migrateFn: func(context.Context) (*sbmigrate.MigrationStats, error) {
				return &sbmigrate.MigrationStats{Users: 1}, nil
			},
		}, nil
	}

	cmd := newSupabaseTestCommand(t, map[string]string{
		"source-url":   "postgres://source",
		"database-url": "postgres://target",
		"json":         "true",
	})

	var stdout string
	stderr := captureStderr(t, func() {
		stdout = captureStdout(t, func() {
			err := runMigrateSupabase(cmd, nil)
			testutil.NoError(t, err)
		})
	})

	testutil.False(t, strings.Contains(stderr, "Proceed? [Y/n]"), "json mode must skip prompt")

	var stats sbmigrate.MigrationStats
	testutil.NoError(t, json.Unmarshal([]byte(stdout), &stats))
	testutil.Equal(t, 1, stats.Users)
}

func TestRunMigrateSupabaseSummaryIgnoresSkippedScopes(t *testing.T) {
	oldFactory := newSupabaseMigrator
	oldSummary := buildSupabaseValidationSummary
	t.Cleanup(func() {
		newSupabaseMigrator = oldFactory
		buildSupabaseValidationSummary = oldSummary
	})

	var gotReport *migrate.AnalysisReport
	newSupabaseMigrator = func(opts sbmigrate.MigrationOptions) (supabaseMigrator, error) {
		return fakeSupabaseMigrator{
			analyzeFn: func(context.Context) (*migrate.AnalysisReport, error) {
				return &migrate.AnalysisReport{
					SourceType:  "Supabase",
					Tables:      4,
					Views:       2,
					Functions:   6,
					Triggers:    7,
					Records:     40,
					AuthUsers:   5,
					OAuthLinks:  3,
					RLSPolicies: 2,
					Files:       7,
				}, nil
			},
			migrateFn: func(context.Context) (*sbmigrate.MigrationStats, error) {
				return &sbmigrate.MigrationStats{Users: 5}, nil
			},
		}, nil
	}
	buildSupabaseValidationSummary = func(report *migrate.AnalysisReport, stats *sbmigrate.MigrationStats) *migrate.ValidationSummary {
		got := *report
		gotReport = &got
		return &migrate.ValidationSummary{
			SourceLabel: "Supabase (source)",
			TargetLabel: "AYB (target)",
		}
	}

	cmd := newSupabaseTestCommand(t, map[string]string{
		"source-url":     "postgres://source",
		"database-url":   "postgres://target",
		"skip-data":      "true",
		"skip-functions": "true",
		"skip-oauth":     "true",
		"skip-rls":       "true",
		"yes":            "true",
	})

	_ = captureStderr(t, func() {
		err := runMigrateSupabase(cmd, nil)
		testutil.NoError(t, err)
	})

	if gotReport == nil {
		t.Fatal("expected validation summary to receive a report")
	}
	testutil.Equal(t, 0, gotReport.Tables)
	testutil.Equal(t, 0, gotReport.Views)
	testutil.Equal(t, 0, gotReport.Functions)
	testutil.Equal(t, 0, gotReport.Triggers)
	testutil.Equal(t, 0, gotReport.Records)
	testutil.Equal(t, 0, gotReport.OAuthLinks)
	testutil.Equal(t, 0, gotReport.RLSPolicies)
	testutil.Equal(t, 0, gotReport.Files)
	testutil.Equal(t, 5, gotReport.AuthUsers)
}

func TestRunMigrateSupabaseSummaryIncludesS3Storage(t *testing.T) {
	oldFactory := newSupabaseMigrator
	oldSummary := buildSupabaseValidationSummary
	t.Cleanup(func() {
		newSupabaseMigrator = oldFactory
		buildSupabaseValidationSummary = oldSummary
	})

	var gotReport *migrate.AnalysisReport
	newSupabaseMigrator = func(opts sbmigrate.MigrationOptions) (supabaseMigrator, error) {
		return fakeSupabaseMigrator{
			analyzeFn: func(context.Context) (*migrate.AnalysisReport, error) {
				return &migrate.AnalysisReport{
					SourceType:    "Supabase",
					Files:         7,
					FileSizeBytes: 1024,
				}, nil
			},
		}, nil
	}
	buildSupabaseValidationSummary = func(report *migrate.AnalysisReport, stats *sbmigrate.MigrationStats) *migrate.ValidationSummary {
		got := *report
		gotReport = &got
		return &migrate.ValidationSummary{}
	}

	flags := supabaseS3TestFlags()
	flags["source-url"] = "postgres://source"
	flags["database-url"] = "postgres://target"
	flags["yes"] = "true"
	_ = captureStderr(t, func() {
		err := runMigrateSupabase(newSupabaseTestCommand(t, flags), nil)
		testutil.NoError(t, err)
	})

	if gotReport == nil {
		t.Fatal("expected validation summary to receive a report")
	}
	testutil.Equal(t, 7, gotReport.Files)
	testutil.Equal(t, int64(1024), gotReport.FileSizeBytes)
}
