package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/spf13/cobra"
)

func TestTypesOpenAPICmd_isRegistered(t *testing.T) {
	// Not parallel: cobra.Command.Commands() sorts in-place.
	found := false
	for _, sub := range typesCmd.Commands() {
		if sub.Use == "openapi" {
			found = true
			break
		}
	}
	if !found {
		t.Error("ayb types openapi subcommand is not registered")
	}
}

func TestTypesOpenAPICmd_hasOutputFlag(t *testing.T) {
	t.Parallel()
	f := typesOpenAPICmd.Flags().Lookup("output")
	if f == nil {
		t.Error("types openapi command missing --output flag")
	}
}

func TestTypesOpenAPICmd_hasDatabaseURLFlag(t *testing.T) {
	t.Parallel()
	f := typesOpenAPICmd.Flags().Lookup("database-url")
	if f == nil {
		t.Error("types openapi command missing --database-url flag")
	}
}

func TestRunTypesOpenAPI_missingDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())

	cmd := &cobra.Command{}
	cmd.Flags().String("database-url", "", "")
	cmd.Flags().StringP("output", "o", "", "")
	testutil.NoError(t, cmd.Flags().Set("database-url", ""))

	err := runTypesOpenAPI(cmd, nil)
	testutil.ErrorContains(t, err, "database-url")
}

func TestResolveTypesDatabaseURLNoSourceErrorListsSupportedSources(t *testing.T) {
	prepareTypesResolverTest(t, "", "", false)
	t.Setenv("DATABASE_URL", "")

	cmd := &cobra.Command{}
	cmd.Flags().String("database-url", "", "")

	_, err := resolveTypesDatabaseURL(cmd)
	testutil.Equal(
		t,
		"--database-url is required (or set DATABASE_URL, AYB_DATABASE_URL, or database.url in ayb.toml)",
		err.Error(),
	)
}

func TestResolveTypesDatabaseURLPrecedence(t *testing.T) {
	const (
		flagURL    = "postgresql://flag_user:flag_pass@flag.example:5432/flag_db"
		envURL     = "postgresql://env_user:env_pass@env.example:5432/env_db"
		aybEnvURL  = "postgresql://ayb_env_user:ayb_env_pass@ayb-env.example:5432/ayb_env_db"
		configURL  = "postgresql://config_user:config_pass@config.example:5432/config_db"
		managedURL = "postgresql://ayb:ayb@127.0.0.1:25432/ayb?sslmode=disable"
	)

	tests := []struct {
		name           string
		explicitURL    string
		environmentURL string
		aybDatabaseURL string
		configURL      string
		running        bool
		expected       string
	}{
		{
			name:           "explicit flag wins over environment",
			explicitURL:    flagURL,
			environmentURL: envURL,
			configURL:      configURL,
			running:        true,
			expected:       flagURL,
		},
		{
			name:           "DATABASE_URL wins over config",
			environmentURL: envURL,
			configURL:      configURL,
			running:        true,
			expected:       envURL,
		},
		{
			name:           "DATABASE_URL wins over AYB_DATABASE_URL config override",
			environmentURL: envURL,
			aybDatabaseURL: aybEnvURL,
			configURL:      configURL,
			running:        true,
			expected:       envURL,
		},
		{
			name:           "AYB_DATABASE_URL is accepted through config loading",
			aybDatabaseURL: aybEnvURL,
			configURL:      configURL,
			running:        true,
			expected:       aybEnvURL,
		},
		{
			name:      "config file wins without flag or environment override",
			configURL: configURL,
			running:   true,
			expected:  configURL,
		},
		{
			name:     "managed Postgres is last fallback",
			running:  true,
			expected: managedURL,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			prepareTypesResolverTest(t, test.configURL, test.aybDatabaseURL, test.running)
			t.Setenv("DATABASE_URL", test.environmentURL)

			cmd := &cobra.Command{}
			cmd.Flags().String("database-url", "", "")
			testutil.NoError(t, cmd.Flags().Set("database-url", test.explicitURL))

			dbURL, err := resolveTypesDatabaseURL(cmd)
			testutil.NoError(t, err)
			testutil.Equal(t, test.expected, dbURL)
		})
	}
}

func prepareTypesResolverTest(t *testing.T, databaseURL string, aybDatabaseURL string, running bool) {
	t.Helper()

	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AYB_DATABASE_URL", aybDatabaseURL)
	t.Chdir(project)

	configBody := fmt.Sprintf("[database]\nurl = %q\nembedded_port = 25432\n", databaseURL)
	testutil.NoError(t, os.WriteFile(filepath.Join(project, "ayb.toml"), []byte(configBody), 0o600))

	if !running {
		return
	}
	aybDir := filepath.Join(home, ".ayb")
	testutil.NoError(t, os.MkdirAll(aybDir, 0o700))
	pidBody := fmt.Sprintf("%d\n8090\n", os.Getpid())
	testutil.NoError(t, os.WriteFile(filepath.Join(aybDir, "ayb.pid"), []byte(pidBody), 0o600))
}
