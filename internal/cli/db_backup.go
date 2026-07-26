// Package cli Provides CLI command handlers for database backup operations, including executing S3 backups and listing backup records.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"text/tabwriter"

	"github.com/allyourbase/ayb/internal/backup"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/spf13/cobra"
)

type dbBackupRequest struct {
	cfg    *config.Config
	dbName string
	dbURL  string
}

type dbBackupExecutor func(context.Context, dbBackupRequest) (backup.RunResult, func(), error)

// runDBBackupS3 is a Cobra command handler that executes a manual database backup to S3, validating backup enablement in the configuration and outputting the result in JSON or table format.
func runDBBackupS3(cmd *cobra.Command, args []string) error {
	return runDBBackupS3WithExecutor(cmd, args, executeDBBackup)
}

func runDBBackupS3WithExecutor(cmd *cobra.Command, _ []string, execute dbBackupExecutor) error {
	ctx := cmd.Context()
	cfg, err := loadDBConfig(cmd)
	if err != nil {
		return err
	}
	if !cfg.Backup.Enabled {
		return fmt.Errorf("backups are not enabled — set [backup] enabled = true in ayb.toml")
	}

	dbURL, err := resolveDBURL(cmd)
	if err != nil {
		return err
	}
	dbName := extractDBName(dbURL)

	fmt.Fprintf(cmd.ErrOrStderr(), "Starting backup of database %q...\n", dbName)
	result, cleanup, err := execute(ctx, dbBackupRequest{
		cfg:    cfg,
		dbName: dbName,
		dbURL:  dbURL,
	})
	if err != nil {
		return err
	}
	defer cleanup()

	if result.Status == "failed" {
		return fmt.Errorf("backup failed: %v", result.Err)
	}
	if result.Status == "skipped" {
		return fmt.Errorf("backup skipped: %v", result.Err)
	}

	return writeDBBackupResult(cmd, result)
}

func executeDBBackup(ctx context.Context, request dbBackupRequest) (backup.RunResult, func(), error) {
	store, err := s3StoreFromConfig(ctx, request.cfg)
	if err != nil {
		return backup.RunResult{}, nil, fmt.Errorf("initialising S3 client: %w", err)
	}

	pool, err := openPool(ctx, request.dbURL)
	if err != nil {
		return backup.RunResult{}, nil, err
	}
	logger := slog.Default()
	repo := backup.NewRepository(pool)
	dumper := &backup.DumpRunner{}
	notifier := backup.NewLogNotifier(logger)

	engine := backup.NewEngine(
		backup.Config{
			Prefix:         request.cfg.Backup.Prefix,
			RetentionCount: request.cfg.Backup.RetentionCount,
			RetentionDays:  request.cfg.Backup.RetentionDays,
		},
		store, repo, dumper, notifier, logger, request.dbName, request.dbURL,
	)
	return engine.Run(ctx, "manual"), pool.Close, nil
}

func writeDBBackupResult(cmd *cobra.Command, result backup.RunResult) error {
	if outputFormat(cmd) == "json" {
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	}

	writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintf(writer, "ID\t%s\n", result.BackupID)
	fmt.Fprintf(writer, "Status\t%s\n", result.Status)
	fmt.Fprintf(writer, "Object Key\t%s\n", result.ObjectKey)
	fmt.Fprintf(writer, "Size\t%d bytes\n", result.SizeBytes)
	fmt.Fprintf(writer, "Checksum\t%s\n", result.Checksum)
	return writer.Flush()
}

// runDBBackupList is a Cobra command handler that retrieves and displays database backup records with optional filtering by status and limit, supporting JSON and table output formats.
func runDBBackupList(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	dbURL, err := resolveDBURL(cmd)
	if err != nil {
		return err
	}

	pool, err := openPool(ctx, dbURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	statusFilter, _ := cmd.Flags().GetString("status")
	limit, _ := cmd.Flags().GetInt("limit")

	repo := backup.NewRepository(pool)
	records, total, err := repo.List(ctx, backup.ListFilter{
		Status: statusFilter,
		Limit:  limit,
	})
	if err != nil {
		return fmt.Errorf("listing backups: %w", err)
	}

	if outputFormat(cmd) == "json" {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
			"backups": records,
			"total":   total,
		})
	}

	if len(records) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No backup records found.")
		return nil
	}

	writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "ID\tDB\tSTATUS\tSIZE\tSTARTED")
	for _, record := range records {
		size := fmt.Sprintf("%d B", record.SizeBytes)
		if record.SizeBytes > 1<<20 {
			size = fmt.Sprintf("%.1f MB", float64(record.SizeBytes)/(1<<20))
		} else if record.SizeBytes > 1<<10 {
			size = fmt.Sprintf("%.1f KB", float64(record.SizeBytes)/(1<<10))
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n",
			record.ID, record.DBName, record.Status, size,
			record.StartedAt.Format("2006-01-02 15:04:05"),
		)
	}
	writer.Flush()
	fmt.Fprintf(cmd.OutOrStdout(), "\nShowing %d of %d backups.\n", len(records), total)
	return nil
}
