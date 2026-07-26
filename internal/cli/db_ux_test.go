package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/backup"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/spf13/cobra"
)

func TestRunDBRestoreLocalRequiresConfirmation(t *testing.T) {
	for _, extension := range []string{".dump", ".sql"} {
		t.Run(extension, func(t *testing.T) {
			inputPath, markerPath := installRestoreTool(t, extension)
			cmd := newDBRestoreTestCmd(t, false)
			cmd.SetIn(strings.NewReader("n\n"))

			err := runDBRestoreLocal(cmd, "postgresql://localhost/test", inputPath)
			testutil.NoError(t, err)
			testutil.Contains(t, cmd.ErrOrStderr().(*bytes.Buffer).String(), "Restore cancelled.")
			if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
				t.Fatalf("restore tool was invoked after declined confirmation")
			}
		})
	}
}

func TestRunDBRestoreLocalYesIsNonInteractive(t *testing.T) {
	for _, extension := range []string{".dump", ".sql"} {
		t.Run(extension, func(t *testing.T) {
			inputPath, markerPath := installRestoreTool(t, extension)
			cmd := newDBRestoreTestCmd(t, true)
			cmd.SetIn(errorReader{})

			err := runDBRestoreLocal(cmd, "postgresql://localhost/test", inputPath)
			testutil.NoError(t, err)
			if _, err := os.Stat(markerPath); err != nil {
				t.Fatalf("restore tool was not invoked with --yes: %v", err)
			}
		})
	}
}

func TestDBExactArgCommandsIncludeMissingInputHelp(t *testing.T) {
	tests := []struct {
		name    string
		command *cobra.Command
		usage   string
		example string
	}{
		{
			name:    "fdw create server",
			command: dbFDWCreateServerCmd,
			usage:   "ayb db fdw create-server <name>",
			example: "ayb db fdw create-server analytics --type postgres_fdw",
		},
		{
			name:    "fdw import tables",
			command: dbFDWImportTablesCmd,
			usage:   "ayb db fdw import-tables <server-name>",
			example: "ayb db fdw import-tables analytics",
		},
		{
			name:    "fdw drop server",
			command: dbFDWDropServerCmd,
			usage:   "ayb db fdw drop-server <name>",
			example: "ayb db fdw drop-server analytics",
		},
		{
			name:    "replica remove",
			command: dbReplicasRemoveCmd,
			usage:   "ayb db replicas remove <name>",
			example: "ayb db replicas remove replica-east",
		},
		{
			name:    "replica promote",
			command: dbReplicasPromoteCmd,
			usage:   "ayb db replicas promote <name>",
			example: "ayb db replicas promote replica-east",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.command.Args(test.command, nil)
			testutil.NotNil(t, err)
			assertMissingInputHelp(t, err, "Usage: "+test.usage, "Example: "+test.example)
		})
	}
}

func TestExactArgsWithHelpDerivesUsageFromCommandMetadata(t *testing.T) {
	root := &cobra.Command{Use: "ayb"}
	command := &cobra.Command{
		Use:  "archive <path>",
		Args: exactArgsWithHelp(1, "ayb archive backup.dump"),
	}
	root.AddCommand(command)

	err := command.Args(command, nil)
	testutil.NotNil(t, err)
	assertMissingInputHelp(
		t,
		err,
		"Usage: ayb archive <path>",
		"Example: ayb archive backup.dump",
	)
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) {
	return 0, errors.New("stdin must not be read")
}

func newDBRestoreTestCmd(t *testing.T, yes bool) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.Flags().Bool("yes", false, "")
	testutil.NoError(t, cmd.Flags().Set("yes", strconv.FormatBool(yes)))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	return cmd
}

func installRestoreTool(t *testing.T, extension string) (string, string) {
	t.Helper()
	directory := t.TempDir()
	inputPath := filepath.Join(directory, "backup"+extension)
	testutil.NoError(t, os.WriteFile(inputPath, []byte("SELECT 1;\n"), 0o600))

	markerPath := filepath.Join(directory, "invoked")
	toolName := "psql"
	if extension == ".dump" {
		toolName = "pg_restore"
	}
	toolPath := filepath.Join(directory, toolName)
	script := "#!/bin/sh\nwhile IFS= read -r line; do :; done\n: > \"$AYB_RESTORE_TEST_MARKER\"\n"
	testutil.NoError(t, os.WriteFile(toolPath, []byte(script), 0o700))
	t.Setenv("AYB_RESTORE_TEST_MARKER", markerPath)
	t.Setenv("PATH", directory)
	return inputPath, markerPath
}

func TestRunDBBackupS3JSONKeepsStdoutDataOnly(t *testing.T) {
	cmd := newDBBackupTestCmd(t, "json")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	executor := func(context.Context, dbBackupRequest) (backup.RunResult, func(), error) {
		return backup.RunResult{
			BackupID:  "backup-123",
			ObjectKey: "backups/test/backup-123.sql.gz",
			SizeBytes: 42,
			Checksum:  "abc123",
			Status:    backup.StatusCompleted,
		}, func() {}, nil
	}

	err := runDBBackupS3WithExecutor(cmd, nil, executor)
	testutil.NoError(t, err)
	testutil.Contains(t, stderr.String(), `Starting backup of database "test"`)
	testutil.Equal(t, 0, strings.Count(stdout.String(), "Starting backup"))

	var result backup.RunResult
	decoder := json.NewDecoder(&stdout)
	testutil.NoError(t, decoder.Decode(&result))
	testutil.Equal(t, "backup-123", result.BackupID)
	testutil.Equal(t, backup.StatusCompleted, result.Status)
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		t.Fatalf("expected exactly one JSON value, got trailing decode error %v and value %#v", err, extra)
	}
}

func TestRunDBBackupS3WritesProgressBeforePreflight(t *testing.T) {
	cmd := newDBBackupTestCmd(t, "table")
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)
	preflightError := errors.New("preflight stopped")

	executor := func(context.Context, dbBackupRequest) (backup.RunResult, func(), error) {
		testutil.Equal(t, "Starting backup of database \"test\"...\n", stderr.String())
		return backup.RunResult{}, func() {}, preflightError
	}

	err := runDBBackupS3WithExecutor(cmd, nil, executor)
	if !errors.Is(err, preflightError) {
		t.Fatalf("expected preflight error, got %v", err)
	}
}

func newDBBackupTestCmd(t *testing.T, format string) *cobra.Command {
	t.Helper()
	directory := t.TempDir()
	configPath := filepath.Join(directory, "ayb.toml")
	configContents := `[backup]
enabled = true
bucket = "test-bucket"
region = "us-east-1"
access_key = "test-access"
secret_key = "test-secret"
[database]
url = "postgresql://localhost/test"
`
	testutil.NoError(t, os.WriteFile(configPath, []byte(configContents), 0o600))

	cmd := &cobra.Command{}
	cmd.Flags().String("config", "", "")
	cmd.Flags().String("database-url", "", "")
	cmd.Flags().String("output", "table", "")
	testutil.NoError(t, cmd.Flags().Set("config", configPath))
	testutil.NoError(t, cmd.Flags().Set("output", format))
	return cmd
}
