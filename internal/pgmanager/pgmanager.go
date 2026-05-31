package pgmanager

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Config holds settings for the managed Postgres manager.
type Config struct {
	BaseDir                string   // root directory for managed Postgres state (default ~/.ayb)
	Port                   uint32   // default 15432
	DataDir                string   // persistent data directory (default ~/.ayb/data)
	RuntimeDir             string   // ephemeral runtime directory (default ~/.ayb/run)
	PIDFile                string   // manager PID file path (default ~/.ayb/pg.pid)
	BinCacheDir            string   // binary cache directory (default ~/.ayb/pg)
	BinDir                 string   // extracted binaries directory (default ~/.ayb/pgbin)
	BinaryURL              string   // custom download URL template (empty = GitHub default)
	PGVersion              string   // PG major version (default "16")
	Extensions             []string // extensions to ensure on every start (CREATE EXTENSION IF NOT EXISTS)
	SharedPreloadLibraries []string // shared_preload_libraries for postgresql.conf
	Logger                 *slog.Logger
}

// Manager manages the lifecycle of a managed PostgreSQL child process.
type Manager struct {
	cfg        Config
	connURL    string
	running    bool
	logger     *slog.Logger
	pidFile    string
	binDir     string
	dataDir    string
	runtimeDir string
	cacheDir   string
}

const (
	dbName = "ayb"
	dbUser = "ayb"
	dbPass = "ayb"
)

// New creates a new Manager. Does not start anything.
func New(cfg Config) *Manager {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Manager{
		cfg:    cfg,
		logger: cfg.Logger,
	}
}

func (m *Manager) Start(ctx context.Context) (string, error) {
	if m.running {
		return m.connURL, nil
	}

	// Resolve paths, defaulting to ~/.ayb/ subdirectories.
	home, err := resolveAYBHome(m.cfg.BaseDir)
	if err != nil {
		return "", fmt.Errorf("resolving ayb home: %w", err)
	}

	port, pgVersion, err := m.prepareStartLayout(home)
	if err != nil {
		return "", err
	}

	// Check for orphaned process.
	m.pidFile = resolvePIDFile(m.cfg, home)
	cleanupOrphan(m.pidFile, m.logger)

	// Platform detection.
	platform, err := platformKey()
	if err != nil {
		return "", fmt.Errorf("detecting platform: %w", err)
	}

	// Download binary (with cache check).
	usedLegacyFallback, err := ensureBinary(ctx, ensureBinaryOpts{
		version:   pgVersion,
		platform:  platform,
		cacheDir:  m.cacheDir,
		binDir:    m.binDir,
		baseURL:   m.cfg.BinaryURL,
		sha256URL: sha256SumsURL(m.cfg.BinaryURL, pgVersion),
	})
	if err != nil {
		return "", fmt.Errorf("ensuring PG binary: %w", err)
	}
	if usedLegacyFallback {
		m.logger.Warn("managed postgres is using the legacy fallback binary source; advanced extensions may be unavailable",
			"pg_version", pgVersion,
			"platform", platform,
		)
	}
	preloadLibraries := sharedPreloadLibrariesForBinarySource(m.cfg.SharedPreloadLibraries, usedLegacyFallback)
	if usedLegacyFallback && len(preloadLibraries) != len(m.cfg.SharedPreloadLibraries) {
		m.logger.Warn("dropping unsupported shared_preload_libraries for the legacy fallback binary source",
			"requested", m.cfg.SharedPreloadLibraries,
			"effective", preloadLibraries,
		)
	}

	// Initialize data directory (skips if already initialized).
	if err := runInitDB(ctx, m.binDir, m.dataDir, m.logger); err != nil {
		return "", fmt.Errorf("initializing data directory: %w", err)
	}

	// Write postgresql.conf.
	if err := writePostgresConf(m.dataDir, port, m.runtimeDir, preloadLibraries); err != nil {
		return "", fmt.Errorf("writing postgresql.conf: %w", err)
	}

	// Start postgres.
	if err := startPostgres(ctx, m.binDir, m.dataDir, port, m.logger); err != nil {
		return "", fmt.Errorf("starting managed postgres: %w", err)
	}
	if err := ensureManagedDatabase(ctx, port, m.logger); err != nil {
		_ = stopPostgres(m.binDir, m.dataDir, m.logger)
		return "", fmt.Errorf("ensuring managed database: %w", err)
	}

	// Write our PID file by reading the Postgres postmaster.pid.
	pgPidFile := filepath.Join(m.dataDir, "postmaster.pid")
	if pid, err := readPostmasterPID(pgPidFile); err == nil && pid > 0 {
		_ = writePID(m.pidFile, pid)
	}

	m.connURL = managedConnURL(port, dbName)
	m.running = true

	// Ensure configured extensions are created. CREATE EXTENSION IF NOT EXISTS
	// is idempotent, so running on every start is safe and allows extensions
	// added to config after initial setup to take effect.
	if len(m.cfg.Extensions) > 0 {
		if err := initExtensions(ctx, m.connURL, m.cfg.Extensions, m.logger); err != nil {
			m.logger.Warn("extension initialization failed (non-fatal)", "error", err)
		}
	}

	m.logger.Info("managed postgres started",
		"port", port,
		"data", m.dataDir,
	)
	return m.connURL, nil
}

func (m *Manager) prepareStartLayout(home string) (uint32, string, error) {
	m.dataDir = defaultManagedPath(m.cfg.DataDir, home, "data")
	m.runtimeDir = defaultManagedPath(m.cfg.RuntimeDir, home, "run")
	m.cacheDir = defaultManagedPath(m.cfg.BinCacheDir, home, "pg")
	m.binDir = defaultManagedPath(m.cfg.BinDir, home, "pgbin")
	for _, dir := range []string{m.dataDir, m.runtimeDir, m.cacheDir, m.binDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return 0, "", fmt.Errorf("creating directory %s: %w", dir, err)
		}
	}
	return defaultManagedPort(m.cfg.Port), defaultManagedPGVersion(m.cfg.PGVersion), nil
}

func defaultManagedPath(configured, home, suffix string) string {
	if configured != "" {
		return configured
	}
	return filepath.Join(home, suffix)
}

func defaultManagedPort(port uint32) uint32 {
	if port == 0 {
		return 15432
	}
	return port
}

func defaultManagedPGVersion(version string) string {
	if version == "" {
		return "16"
	}
	return version
}

func sharedPreloadLibrariesForBinarySource(configured []string, usedLegacyFallback bool) []string {
	effective := append([]string(nil), configured...)
	if !usedLegacyFallback {
		return effective
	}

	filtered := effective[:0]
	for _, lib := range effective {
		if managedExtensionName(lib) == "pg_cron" {
			continue
		}
		filtered = append(filtered, lib)
	}
	return filtered
}

// Stop gracefully shuts down the managed PostgreSQL child process.
func (m *Manager) Stop() error {
	if !m.running {
		return nil
	}

	m.logger.Info("stopping managed postgres")
	err := stopPostgres(m.binDir, m.dataDir, m.logger)
	m.running = false

	// Clean up PID file.
	_ = removePID(m.pidFile)

	if err != nil {
		return fmt.Errorf("stopping managed postgres: %w", err)
	}
	m.logger.Info("managed postgres stopped")
	return nil
}

// ConnURL returns the connection URL. Only valid after Start() succeeds.
func (m *Manager) ConnURL() string {
	return m.connURL
}

// IsRunning returns true if the managed Postgres is currently running.
func (m *Manager) IsRunning() bool {
	return m.running
}

// --- AYB home directory ---

// aybHome returns ~/.ayb, creating it if necessary.
func aybHome() (string, error) {
	return resolveAYBHome("")
}

// resolveAYBHome returns the configured AYB state directory, or ~/.ayb by default.
func resolveAYBHome(baseDir string) (string, error) {
	if baseDir != "" {
		if err := os.MkdirAll(baseDir, 0o755); err != nil {
			return "", fmt.Errorf("creating %s: %w", baseDir, err)
		}
		return baseDir, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting user home directory: %w", err)
	}
	aybDir := filepath.Join(home, ".ayb")
	if err := os.MkdirAll(aybDir, 0o755); err != nil {
		return "", fmt.Errorf("creating ~/.ayb: %w", err)
	}
	return aybDir, nil
}

// --- PID file management ---

// writePID writes the given PID to a file.
func writePID(path string, pid int) error {
	return os.WriteFile(path, []byte(strconv.Itoa(pid)), 0o644)
}

// readPID reads a PID from a file. Returns 0 if the file doesn't exist.
func readPID(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("parsing pid file: %w", err)
	}
	return pid, nil
}

// removePID removes a PID file. No error if it doesn't exist.
func removePID(path string) error {
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func resolvePIDFile(cfg Config, home string) string {
	if cfg.PIDFile != "" {
		return cfg.PIDFile
	}
	return filepath.Join(home, "pg.pid")
}

// readPostmasterPID reads the PID from Postgres's postmaster.pid file.
// The PID is on the first line.
func readPostmasterPID(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	lines := strings.SplitN(string(data), "\n", 2)
	if len(lines) == 0 {
		return 0, fmt.Errorf("empty postmaster.pid")
	}
	return strconv.Atoi(strings.TrimSpace(lines[0]))
}

// cleanupOrphan checks for a stale PID file and kills the orphaned process.
func cleanupOrphan(pidPath string, logger *slog.Logger) {
	pid, err := readPID(pidPath)
	if err != nil || pid == 0 {
		return
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		_ = removePID(pidPath)
		return
	}

	// Check if process is alive (signal 0 tests existence).
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		// Process is dead — remove stale PID file.
		logger.Info("removed stale PID file", "pid", pid)
		_ = removePID(pidPath)
		return
	}

	// Process is alive — kill it.
	logger.Warn("found orphaned postgres process, terminating", "pid", pid)
	_ = proc.Signal(syscall.SIGTERM)

	// Wait up to 5 seconds for graceful shutdown.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
		if err := proc.Signal(syscall.Signal(0)); err != nil {
			// Process exited.
			_ = removePID(pidPath)
			logger.Info("orphaned postgres process terminated", "pid", pid)
			return
		}
	}

	// Force kill.
	logger.Warn("force-killing orphaned postgres", "pid", pid)
	_ = proc.Signal(syscall.SIGKILL)
	_ = removePID(pidPath)
}

// --- Log writer adapter ---

// logWriter adapts *slog.Logger to io.Writer for postgres output.
type logWriter struct {
	logger *slog.Logger
}

func newLogWriter(logger *slog.Logger) *logWriter {
	return &logWriter{logger: logger}
}

func (w *logWriter) Write(p []byte) (int, error) {
	msg := strings.TrimRight(string(p), "\n\r")
	if msg != "" {
		w.logger.Debug("postgres", "output", msg)
	}
	return len(p), nil
}
