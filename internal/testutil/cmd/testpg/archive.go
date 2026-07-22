package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/cli"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/testutil"
)

const (
	testPGArchiveProjectID       = "project-roundtrip"
	testPGArchiveExecutableEnv   = "TESTPG_ARCHIVE_EXECUTABLE"
	testPGArchiveContainerPrefix = "ayb-testpg-"
)

type archiveIdentity struct {
	containerName string
	bucket        string
	archivePrefix string
}

type archiveRuntime struct {
	configPath string
	command    string
	harness    *testutil.MinIOHarness
}

func prepareArchiveRuntime(ctx context.Context, tempRoot string, port int) (*archiveRuntime, error) {
	binaryPath, err := buildCurrentAYBBinary(ctx, tempRoot)
	if err != nil {
		return nil, err
	}

	identity := newArchiveIdentity(tempRoot)
	harness, err := testutil.StartMinIOHarness(ctx, testutil.MinIOHarnessOptions{
		ContainerName: identity.containerName,
		Bucket:        identity.bucket,
	})
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*archiveRuntime, error) {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = harness.Close(cleanupCtx)
		return nil, err
	}

	cfg := newArchiveRuntimeConfig(testPGDatabaseURL(port), harness, identity.archivePrefix)
	configPath := filepath.Join(tempRoot, "effective config", "ayb.toml")
	if err := writeArchiveRuntimeConfig(configPath, cfg); err != nil {
		return fail(err)
	}
	archiveCommand, err := archiveCommandForRuntime(cfg, binaryPath, configPath)
	if err != nil {
		return fail(err)
	}
	return &archiveRuntime{configPath: configPath, command: archiveCommand, harness: harness}, nil
}

func (r *archiveRuntime) close(ctx context.Context) error {
	return r.harness.Close(ctx)
}

func newArchiveIdentity(tempRoot string) archiveIdentity {
	runID := archiveResourceToken(filepath.Base(tempRoot))
	name := testPGArchiveContainerPrefix + strings.TrimPrefix(runID, testPGArchiveContainerPrefix)
	return archiveIdentity{
		containerName: name,
		bucket:        name,
		archivePrefix: "roundtrip/" + name,
	}
}

func archiveResourceToken(value string) string {
	var token strings.Builder
	for _, char := range strings.ToLower(value) {
		switch {
		case char >= 'a' && char <= 'z', char >= '0' && char <= '9', char == '-':
			token.WriteRune(char)
		default:
			token.WriteByte('-')
		}
	}
	return strings.Trim(token.String(), "-")
}

func buildCurrentAYBBinary(ctx context.Context, tempRoot string) (string, error) {
	binaryPath := filepath.Join(tempRoot, "ayb binary", "ayb")
	if err := os.MkdirAll(filepath.Dir(binaryPath), 0o700); err != nil {
		return "", fmt.Errorf("create AYB binary directory: %w", err)
	}
	cmd := exec.CommandContext(ctx, "go", "build", "-o", binaryPath, "./cmd/ayb")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("build current AYB executable: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return binaryPath, nil
}

func newArchiveRuntimeConfig(databaseURL string, harness *testutil.MinIOHarness, archivePrefix string) *config.Config {
	cfg := config.Default()
	cfg.Database.URL = databaseURL
	cfg.Backup.Enabled = true
	cfg.Backup.Bucket = harness.Bucket
	cfg.Backup.Endpoint = "http://" + harness.Endpoint
	cfg.Backup.AccessKey = harness.AccessKey
	cfg.Backup.SecretKey = harness.SecretKey
	cfg.Backup.UseSSL = harness.UseSSL
	cfg.Backup.Encryption = ""
	cfg.Backup.PITR.Enabled = true
	cfg.Backup.PITR.ArchiveBucket = harness.Bucket
	cfg.Backup.PITR.ArchivePrefix = archivePrefix
	cfg.Backup.PITR.EnvironmentClass = "test"
	cfg.Backup.PITR.ShadowMode = false
	return cfg
}

func writeArchiveRuntimeConfig(path string, cfg *config.Config) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validate test-local archive config: %w", err)
	}
	toml, err := cfg.ToTOML()
	if err != nil {
		return fmt.Errorf("encode test-local archive config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create test-local archive config directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(toml), 0o600); err != nil {
		return fmt.Errorf("write test-local archive config: %w", err)
	}
	return nil
}

func archiveCommandForRuntime(cfg *config.Config, binaryPath, configPath string) (string, error) {
	if err := cfg.Validate(); err != nil {
		return "", fmt.Errorf("validate archive command config: %w", err)
	}
	executablePath := binaryPath
	if override := strings.TrimSpace(os.Getenv(testPGArchiveExecutableEnv)); override != "" {
		executablePath = override
	}
	return cli.BuildManagedPostgresArchiveCommand(executablePath, configPath)
}

func testPGDatabaseURL(port int) string {
	return fmt.Sprintf("postgresql://ayb:ayb@127.0.0.1:%d/postgres?sslmode=disable", port)
}

func setArchiveProjectEnvironment() (func(), error) {
	previous, existed := os.LookupEnv(testutil.ArchiveProjectIDEnv)
	if err := os.Setenv(testutil.ArchiveProjectIDEnv, testPGArchiveProjectID); err != nil {
		return nil, err
	}
	return func() {
		if existed {
			_ = os.Setenv(testutil.ArchiveProjectIDEnv, previous)
		} else {
			_ = os.Unsetenv(testutil.ArchiveProjectIDEnv)
		}
	}, nil
}
