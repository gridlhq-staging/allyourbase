package cli

import (
	"bytes"
	"errors"
	"log/slog"
	"testing"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/postgres"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/spf13/cobra"
)

func TestMigrateCommandsWriteProgressBeforeConnect(t *testing.T) {
	tests := []struct {
		name     string
		progress string
		run      func(*cobra.Command, []string, migrateConnector) error
	}{
		{name: "up", progress: "Applying database migrations...\n", run: runMigrateUpWithConnector},
		{name: "status", progress: "Checking migration status...\n", run: runMigrateStatusWithConnector},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := newMigrateProgressTestCmd(t)
			var stderr bytes.Buffer
			cmd.SetErr(&stderr)
			connectError := errors.New("connect stopped")
			connector := func(*cobra.Command, *config.Config, *slog.Logger) (*postgres.Pool, func(), error) {
				testutil.Equal(t, test.progress, stderr.String())
				return nil, nil, connectError
			}

			err := test.run(cmd, nil, connector)
			if !errors.Is(err, connectError) {
				t.Fatalf("expected connector error, got %v", err)
			}
		})
	}
}

func newMigrateProgressTestCmd(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.Flags().String("config", "", "")
	cmd.Flags().String("database-url", "", "")
	cmd.Flags().String("migrations-dir", "", "")
	testutil.NoError(t, cmd.Flags().Set("database-url", "postgresql://localhost/test"))
	return cmd
}
