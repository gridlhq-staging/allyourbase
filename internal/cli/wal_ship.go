// Package cli Wal_ship implements the wal-ship CLI command for uploading PostgreSQL WAL segments to point-in-time recovery archive storage. It manages configuration loading, S3 connectivity, and segment shipping.
package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/allyourbase/ayb/internal/backup"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
)

type walShipPool interface {
	Close()
}

type walSegmentShipper interface {
	Ship(ctx context.Context, walFilePath string, walFileName string) error
}

var (
	loadWALShipConfig = config.Load
	newWALShipStore   = func(ctx context.Context, cfg backup.S3Config) (backup.Store, error) {
		return backup.NewS3Store(ctx, cfg)
	}
	newWALShipPool = func(ctx context.Context, databaseURL string) (walShipPool, error) {
		return pgxpool.New(ctx, databaseURL)
	}
	newWALShipRepo = func(pool walShipPool) (backup.WALSegmentRepo, error) {
		pgxPool, ok := pool.(*pgxpool.Pool)
		if !ok {
			return nil, fmt.Errorf("unsupported WAL ship pool type %T", pool)
		}
		return backup.NewPgWALSegmentRepo(pgxPool), nil
	}
	newWALShipShipper = func(
		store backup.Store,
		repo backup.WALSegmentRepo,
		cfg backup.PITRConfig,
		projectID string,
		databaseID string,
		notify backup.Notifier,
	) walSegmentShipper {
		return backup.NewWALShipper(store, repo, cfg, projectID, databaseID, notify)
	}
)

var walShipCmd = &cobra.Command{
	Use:   "wal-ship <wal-file-path> <wal-file-name>",
	Short: "Ship a single PostgreSQL WAL segment to PITR archive storage",
	Args:  cobra.ExactArgs(2),
	RunE:  runWALShip,
}

func init() {
	walShipCmd.Flags().String("config", "", "Path to ayb.toml config file")
}

// runWALShip executes the wal-ship command to ship a single PostgreSQL WAL segment to PITR archive storage. It loads configuration from ayb.toml, validates PITR is enabled and database URL is set, establishes an S3 connection for the archive, and invokes the shipper to upload the segment.
func runWALShip(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	configPath, _ := cmd.Flags().GetString("config")
	if configPath == "" {
		configPath = "ayb.toml"
	}

	cfg, err := loadWALShipConfig(configPath, nil)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if !cfg.Backup.PITR.Enabled {
		return fmt.Errorf("WAL shipping is not enabled (set [backup.pitr] enabled = true)")
	}
	if cfg.Database.URL == "" {
		return fmt.Errorf("no database URL configured (set database.url in ayb.toml)")
	}

	endpoint := cfg.Backup.Endpoint
	if endpoint == "" {
		endpoint = fmt.Sprintf("s3.%s.amazonaws.com", cfg.Backup.Region)
	}

	store, err := newWALShipStore(ctx, backup.S3Config{
		Endpoint:   endpoint,
		Bucket:     cfg.Backup.PITR.ArchiveBucket,
		Region:     cfg.Backup.Region,
		AccessKey:  cfg.Backup.AccessKey,
		SecretKey:  cfg.Backup.SecretKey,
		Encryption: cfg.Backup.Encryption,
		KMSKeyID:   cfg.Backup.PITR.KMSKeyID,
		UseSSL:     cfg.Backup.UseSSL,
	})
	if err != nil {
		return fmt.Errorf("initialising PITR S3 store: %w", err)
	}

	pool, err := newWALShipPool(ctx, cfg.Database.URL)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()
	walRepo, err := newWALShipRepo(pool)
	if err != nil {
		return fmt.Errorf("creating WAL metadata repository: %w", err)
	}

	projectID := os.Getenv("AYB_PROJECT_ID")
	if projectID == "" {
		projectID = "default"
	}
	databaseID := extractDBName(cfg.Database.URL)

	shipper := newWALShipShipper(
		store,
		walRepo,
		backup.PITRConfig{ArchivePrefix: cfg.Backup.PITR.ArchivePrefix},
		projectID,
		databaseID,
		backup.NewLogNotifier(slog.Default()),
	)

	if err := shipper.Ship(ctx, args[0], args[1]); err != nil {
		return fmt.Errorf("shipping WAL segment: %w", err)
	}
	return nil
}
