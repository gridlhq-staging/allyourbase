// Package cli start.go implements the server startup command, routing to
// foreground or detached mode.
package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/jobs"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/spf13/cobra"
)

const adminLogBufferCapacity = 1000

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the AYB server",
	Long: `Start the Allyourbase server. If no database URL is configured,
AYB starts a managed PostgreSQL instance automatically.`,
	Example: `ayb start
ayb start --database-url postgresql://user:pass@localhost:5432/mydb  # External database
ayb start --from ./pb_data  # Migrate and start from PocketBase
ayb start --from postgres://db.xxx.supabase.co:5432/postgres  # Migrate and start from Supabase`,
	RunE: runStart,
}

var registerBillingUsageSyncHandler = jobs.RegisterBillingUsageSyncHandler
var registerBillingUsageSyncSchedule = jobs.RegisterBillingUsageSyncSchedule
var registerProviderTokenRefreshHandler = jobs.RegisterProviderTokenRefreshHandler
var registerProviderTokenRefreshSchedule = jobs.RegisterProviderTokenRefreshSchedule
var registerAnonymousUserCleanupHandler = jobs.RegisterAnonymousUserCleanupHandler
var registerAnonymousUserCleanupSchedule = jobs.RegisterAnonymousUserCleanupSchedule

type startForegroundInput struct {
	flags              map[string]string
	configPath         string
	configPathExplicit bool
	fromValue          string
	branchName         string
}

func init() {
	startCmd.Flags().String("database-url", "", "PostgreSQL connection URL")
	startCmd.Flags().Int("port", 0, "Server port (default 8090)")
	startCmd.Flags().String("host", "", "Server host (default 127.0.0.1)")
	startCmd.Flags().String("config", "", "Path to ayb.toml config file")
	startCmd.Flags().String("from", "", "Migrate from another platform and start (path to pb_data, or postgres:// URL)")
	startCmd.Flags().String("domain", "", "Domain for automatic HTTPS via Let's Encrypt (e.g. api.myapp.com)")
	startCmd.Flags().String("branch", "", "Start using a database branch (created via ayb branch create)")
	startCmd.Flags().Bool("foreground", false, "Run in foreground (blocks terminal)")
	startCmd.Flags().MarkHidden("foreground") //nolint:errcheck
}

// runStart is the entry point for the start command that determines whether
// to run the server in foreground or detached mode.
func runStart(cmd *cobra.Command, args []string) error {
	fg, _ := cmd.Flags().GetBool("foreground")
	fromValue, _ := cmd.Flags().GetString("from")

	// --from requires interactive output, force foreground.
	if fromValue != "" {
		fg = true
	}

	// Windows doesn't support background mode.
	if !fg && !detachSupported() {
		fmt.Fprintln(os.Stderr, "Background mode not supported on this platform, running in foreground.")
		fg = true
	}

	if fg {
		return runStartForeground(cmd, args)
	}
	return runStartDetached(cmd, args)
}

func ensureConfiguredAdminPassword(cfg *config.Config) (string, error) {
	if !cfg.Admin.Enabled || cfg.Admin.Password != "" {
		return "", nil
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating admin password: %w", err)
	}
	generatedPassword := hex.EncodeToString(b)
	cfg.Admin.Password = generatedPassword
	return generatedPassword, nil
}

func newAdminLogger(logger *slog.Logger) (*slog.Logger, *server.LogBuffer) {
	logBuffer := server.NewLogBuffer(logger.Handler(), adminLogBufferCapacity)
	return slog.New(logBuffer), logBuffer
}

// runStartForeground runs the AYB server in the foreground, initializing the database, wiring all services, and blocking until a shutdown signal is received.
func runStartForeground(cmd *cobra.Command, args []string) error {
	input, err := readStartForegroundInput(cmd)
	if err != nil {
		return err
	}

	cfg, err := config.Load(input.configPath, input.flags)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	generatedPassword, err := ensureConfiguredAdminPassword(cfg)
	if err != nil {
		return err
	}

	ctx, cancel, sigCh := newForegroundSignalContext()
	defer cancel()
	defer signal.Stop(sigCh)

	isTTY := colorEnabled()
	sp := newStartupProgress(os.Stderr, isTTY, isTTY)
	logger, logLevel, logPath, closeLog := newLogger(cfg.Logging.Level, cfg.Logging.Format)
	defer closeLog()
	logger, logBuffer := newAdminLogger(logger)
	if isTTY {
		logLevel.Set(slog.LevelWarn)
	}
	sp.header(bannerVersion(buildVersion))

	if err := runForegroundPreflight(cfg, input.configPathExplicit, input.fromValue, logger); err != nil {
		return err
	}

	pool, pgMgr, schemaCache, watcherCancel, err := initDatabase(ctx, cfg, input.configPath, input.fromValue, input.branchName, sigCh, logger, sp)
	if err != nil {
		return err
	}
	if pool == nil {
		return nil // early signal exit
	}
	defer pool.Close()
	defer watcherCancel()

	// --- Core services ---
	core, err := initCoreServices(ctx, cfg, pool, logger)
	if err != nil {
		stopManagedPostgres(pgMgr, logger)
		return err
	}

	sp.step("Starting server...")
	state, err := wireServices(ctx, cfg, pool, core, schemaCache, logger)
	if err != nil {
		stopManagedPostgres(pgMgr, logger)
		return err
	}
	state.srv.SetLogBuffer(logBuffer)
	defer state.cleanup(pool.DB(), logger)

	usrCh := notifyUSR1()
	ready := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		if cfg.Server.TLSEnabled {
			ln, redir, tlsErr := buildTLSListener(ctx, cfg, state.certmagicConfig, logger)
			if tlsErr != nil {
				errCh <- tlsErr
				return
			}
			state.tlsRedirectSrv = redir
			errCh <- state.srv.StartTLSWithReady(ln, ready)
		} else {
			errCh <- state.srv.StartWithReady(ready)
		}
	}()

	readyCleanup, err := waitForServerReady(ready, errCh, usrCh, cfg, pgMgr, state.srv, logger, sp, readyState{
		isTTY:             isTTY,
		generatedPassword: generatedPassword,
		logPath:           logPath,
		logLevel:          logLevel,
	})
	if err != nil {
		return err
	}
	defer readyCleanup()

	return runGracefulShutdown(ctx, errCh, sigCh, watcherCancel, state, pgMgr, logger)
}

func readStartForegroundInput(cmd *cobra.Command) (startForegroundInput, error) {
	input := startForegroundInput{flags: make(map[string]string)}
	if v, err := cmd.Flags().GetString("database-url"); err != nil {
		return input, err
	} else if v != "" {
		input.flags["database-url"] = v
	}
	if v, err := cmd.Flags().GetInt("port"); err != nil {
		return input, err
	} else if v != 0 {
		input.flags["port"] = fmt.Sprintf("%d", v)
	}
	if v, err := cmd.Flags().GetString("host"); err != nil {
		return input, err
	} else if v != "" {
		input.flags["host"] = v
	}
	if v, err := cmd.Flags().GetString("domain"); err != nil {
		return input, err
	} else if v != "" {
		input.flags["tls-domain"] = v
	}
	var err error
	input.configPath, err = cmd.Flags().GetString("config")
	if err != nil {
		return input, err
	}
	input.configPathExplicit = input.configPath != ""
	if input.configPath == "" {
		input.configPath = "ayb.toml"
	}
	input.configPath, err = filepath.Abs(input.configPath)
	if err != nil {
		return input, err
	}
	input.fromValue, err = cmd.Flags().GetString("from")
	if err != nil {
		return input, err
	}
	input.branchName, err = cmd.Flags().GetString("branch")
	return input, err
}

func newForegroundSignalContext() (context.Context, context.CancelFunc, chan os.Signal) {
	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	return ctx, cancel, sigCh
}
