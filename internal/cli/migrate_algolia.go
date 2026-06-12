package cli

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/allyourbase/ayb/internal/algoliamigrate"
	"github.com/allyourbase/ayb/internal/migrate"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/spf13/cobra"
)

type algoliaMigrator interface {
	Analyze(context.Context) (*migrate.AnalysisReport, error)
	Migrate(context.Context) (*algoliamigrate.ImportStats, error)
	Close() error
}

type algoliaMigrationOptions struct {
	AppID           string
	APIKey          string
	IndexName       string
	DatabaseURL     string
	TargetTable     string
	IncludeSynonyms bool
	IncludeSettings bool
	DryRun          bool
	Progress        migrate.ProgressReporter
}

const algoliaAPIKeyEnv = "ALGOLIA_API_KEY"

type algoliaCLIAdapter struct {
	opts     algoliaMigrationOptions
	db       *sql.DB
	browse   *algoliamigrate.BrowseResult
	synonyms *algoliamigrate.SynonymInput
	settings *algoliamigrate.AlgoliaSettings
	plan     *algoliamigrate.ImportPlan
	report   *migrate.AnalysisReport
}

var newAlgoliaMigrator = func(opts algoliaMigrationOptions) (algoliaMigrator, error) {
	return &algoliaCLIAdapter{opts: opts}, nil
}

var buildAlgoliaValidationSummary = algoliamigrate.BuildValidationSummary

var migrateAlgoliaCmd = newMigrateAlgoliaCommand()

func newMigrateAlgoliaCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "algolia",
		Short: "Migrate records from an Algolia index",
		RunE:  runMigrateAlgolia,
	}

	cmd.Flags().String("app-id", "", "Algolia application ID")
	cmd.Flags().String("api-key", "", "Algolia API key with browse access (or set ALGOLIA_API_KEY)")
	cmd.Flags().String("index", "", "Algolia index name to browse")
	cmd.Flags().String("database-url", "", "AYB PostgreSQL connection URL (target)")
	cmd.Flags().String("table", "", "Target PostgreSQL table name")
	cmd.Flags().Bool("include-synonyms", false, "Import supported Algolia synonym groups")
	cmd.Flags().Bool("include-settings", false, "Import Algolia index search settings")
	cmd.Flags().Bool("dry-run", false, "Preview what would be migrated without making changes")
	cmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")
	cmd.Flags().Bool("json", false, "Output import stats as JSON")

	cmd.MarkFlagRequired("app-id")
	cmd.MarkFlagRequired("index")
	cmd.MarkFlagRequired("database-url")
	cmd.MarkFlagRequired("table")
	return cmd
}

func runMigrateAlgolia(cmd *cobra.Command, args []string) error {
	opts, yes, jsonOut := algoliaOptionsFromCommand(cmd)
	if strings.TrimSpace(opts.APIKey) == "" {
		return fmt.Errorf("algolia API key is required; pass --api-key or set %s", algoliaAPIKeyEnv)
	}
	migrator, err := newAlgoliaMigrator(opts)
	if err != nil {
		return fmt.Errorf("failed to create migrator: %w", err)
	}
	defer migrator.Close()

	ctx := context.Background()
	report, err := migrator.Analyze(ctx)
	if err != nil {
		return fmt.Errorf("analysis failed: %w", err)
	}

	if !jsonOut {
		report.PrintReport(os.Stderr)
		proceed, err := confirmAlgoliaMigration(yes, opts.DryRun)
		if err != nil {
			return err
		}
		if !proceed {
			return nil
		}
		fmt.Fprintln(os.Stderr)
	}

	stats, err := migrator.Migrate(ctx)
	if err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	if !jsonOut && !opts.DryRun {
		summary := buildAlgoliaValidationSummary(report, stats)
		summary.PrintSummary(os.Stderr)
	}

	if jsonOut {
		return json.NewEncoder(os.Stdout).Encode(stats)
	}
	return nil
}

func algoliaOptionsFromCommand(cmd *cobra.Command) (algoliaMigrationOptions, bool, bool) {
	appID, _ := cmd.Flags().GetString("app-id")
	apiKey, _ := cmd.Flags().GetString("api-key")
	indexName, _ := cmd.Flags().GetString("index")
	databaseURL, _ := cmd.Flags().GetString("database-url")
	targetTable, _ := cmd.Flags().GetString("table")
	includeSynonyms, _ := cmd.Flags().GetBool("include-synonyms")
	includeSettings, _ := cmd.Flags().GetBool("include-settings")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	yes, _ := cmd.Flags().GetBool("yes")
	jsonOut, _ := cmd.Flags().GetBool("json")
	if strings.TrimSpace(apiKey) == "" {
		apiKey = os.Getenv(algoliaAPIKeyEnv)
	}

	var progress migrate.ProgressReporter
	if jsonOut {
		progress = migrate.NopReporter{}
	} else {
		progress = migrate.NewCLIReporter(os.Stderr)
	}

	return algoliaMigrationOptions{
		AppID:           appID,
		APIKey:          apiKey,
		IndexName:       indexName,
		DatabaseURL:     databaseURL,
		TargetTable:     targetTable,
		IncludeSynonyms: includeSynonyms,
		IncludeSettings: includeSettings,
		DryRun:          dryRun,
		Progress:        progress,
	}, yes, jsonOut
}

func confirmAlgoliaMigration(yes bool, dryRun bool) (bool, error) {
	if yes || dryRun {
		return true, nil
	}
	fmt.Fprint(os.Stderr, "  Proceed? [Y/n] ")
	reader := bufio.NewReader(os.Stdin)
	answer, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("reading migration confirmation: %w", err)
	}
	answer = strings.TrimSpace(strings.ToLower(answer))
	if answer != "" && answer != "y" && answer != "yes" {
		fmt.Fprintln(os.Stderr, "  Migration cancelled.")
		return false, nil
	}
	return true, nil
}

func (a *algoliaCLIAdapter) Analyze(ctx context.Context) (*migrate.AnalysisReport, error) {
	if a.report != nil {
		return a.report, nil
	}
	browse, synonyms, settings, err := a.readAlgoliaSource(ctx)
	if err != nil {
		return nil, err
	}
	a.browse = browse
	a.synonyms = synonyms
	a.settings = settings
	plan, err := algoliamigrate.PlanImport(browse.Records, a.importOptions(synonyms, settings))
	if err != nil {
		return nil, err
	}
	a.plan = plan
	a.report = algoliamigrate.BuildAnalysisReport(plan)
	return a.report, nil
}

func (a *algoliaCLIAdapter) Migrate(ctx context.Context) (*algoliamigrate.ImportStats, error) {
	if _, err := a.Analyze(ctx); err != nil {
		return nil, err
	}
	if a.opts.DryRun {
		db, err := a.openTargetDB(ctx)
		if err != nil {
			return nil, err
		}
		if err := algoliamigrate.ValidateTargetAbsent(ctx, db, a.plan.Target); err != nil {
			return nil, err
		}
		return a.dryRunStats(), nil
	}
	db, err := a.openTargetDB(ctx)
	if err != nil {
		return nil, err
	}
	migrator := algoliamigrate.NewMigrator(db, a.importOptions(a.synonyms, a.settings), a.opts.Progress)
	return migrator.ImportRecords(ctx, a.browse.Records)
}

func (a *algoliaCLIAdapter) Close() error {
	if a.db != nil {
		return a.db.Close()
	}
	return nil
}

func (a *algoliaCLIAdapter) readAlgoliaSource(ctx context.Context) (*algoliamigrate.BrowseResult, *algoliamigrate.SynonymInput, *algoliamigrate.AlgoliaSettings, error) {
	cfg := algoliamigrate.BrowseConfig{
		Endpoint:  os.Getenv("ALGOLIA_ENDPOINT"),
		AppID:     a.opts.AppID,
		APIKey:    a.opts.APIKey,
		IndexName: a.opts.IndexName,
	}
	browseClient, err := algoliamigrate.NewBrowseClient(cfg)
	if err != nil {
		return nil, nil, nil, err
	}
	browse, err := browseClient.Browse(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	var synonyms *algoliamigrate.SynonymInput
	if a.opts.IncludeSynonyms {
		synonymClient, err := algoliamigrate.NewSynonymSearchClient(cfg)
		if err != nil {
			return nil, nil, nil, err
		}
		synonyms, err = synonymClient.SearchSynonyms(ctx)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	var settings *algoliamigrate.AlgoliaSettings
	if a.opts.IncludeSettings {
		settingsClient, err := algoliamigrate.NewSettingsClient(cfg)
		if err != nil {
			return nil, nil, nil, err
		}
		s, err := settingsClient.GetSettings(ctx)
		if err != nil {
			return nil, nil, nil, err
		}
		settings = &s
	}
	return browse, synonyms, settings, nil
}

func (a *algoliaCLIAdapter) importOptions(synonyms *algoliamigrate.SynonymInput, settings *algoliamigrate.AlgoliaSettings) algoliamigrate.ImportOptions {
	return algoliamigrate.ImportOptions{
		TargetTable: a.opts.TargetTable,
		DryRun:      a.opts.DryRun,
		Synonyms:    synonyms,
		Settings:    settings,
	}
}

func (a *algoliaCLIAdapter) dryRunStats() *algoliamigrate.ImportStats {
	if a.plan == nil {
		return &algoliamigrate.ImportStats{DryRun: true}
	}
	return &algoliamigrate.ImportStats{
		Tables:   a.plan.DryRun.TablesPlanned,
		Records:  a.plan.DryRun.RecordsPlanned,
		DryRun:   true,
		Settings: a.plan.Settings.Stats,
		Synonyms: a.plan.Synonyms.Stats,
	}
}

func (a *algoliaCLIAdapter) openTargetDB(ctx context.Context) (*sql.DB, error) {
	if a.db != nil {
		return a.db, nil
	}
	db, err := sql.Open("pgx", a.opts.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("connecting to database: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}
	a.db = db
	return db, nil
}
