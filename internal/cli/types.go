package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/openapi"
	"github.com/allyourbase/ayb/internal/postgres"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/typegen"
	"github.com/spf13/cobra"
)

var typesCmd = &cobra.Command{
	Use:   "types",
	Short: "Generate typed interfaces from database schema",
	Long: `Generate typed interfaces by introspecting a running PostgreSQL database.

Supported formats:
  typescript    Generate TypeScript interfaces (.d.ts)`,
	Example: `ayb types typescript --database-url postgresql://user:pass@localhost:5432/mydb
ayb types typescript --database-url postgresql://localhost/mydb -o src/types/ayb.d.ts`,
}

var typesTypeScriptCmd = &cobra.Command{
	Use:   "typescript",
	Short: "Generate TypeScript interfaces from database schema",
	Long: `Connect to PostgreSQL, introspect the schema, and emit TypeScript
interfaces for every user table. System tables (_ayb_*) are excluded.

Output includes:
  - An interface for each table (e.g., export interface Posts { ... })
  - A Create type that omits auto-generated columns (PK, defaults)
  - An Update type (Partial<Create>)
  - Enum union types for PostgreSQL enums`,
	RunE: runTypesTypeScript,
}

var typesOpenAPICmd = &cobra.Command{
	Use:   "openapi",
	Short: "Generate OpenAPI 3.1 spec from database schema",
	Long: `Connect to PostgreSQL, introspect the schema, and emit an OpenAPI 3.1 JSON
specification documenting all CRUD endpoints, query parameters, and RPC functions.

If the AYB server is running, you can also fetch the spec from /api/openapi.json.

Examples:
  ayb types openapi --database-url postgresql://user:pass@localhost:5432/mydb
  ayb types openapi -o openapi.json`,
	RunE: runTypesOpenAPI,
}

func init() {
	typesCmd.AddCommand(typesTypeScriptCmd)
	typesCmd.AddCommand(typesOpenAPICmd)
	typesTypeScriptCmd.Flags().String("database-url", "", "PostgreSQL connection URL (required)")
	typesTypeScriptCmd.Flags().StringP("output", "o", "", "Output file path (default: stdout)")
	typesOpenAPICmd.Flags().String("database-url", "", "PostgreSQL connection URL (required)")
	typesOpenAPICmd.Flags().StringP("output", "o", "", "Output file path (default: stdout)")
}

func runTypesTypeScript(cmd *cobra.Command, args []string) error {
	dbURL, err := resolveTypesDatabaseURL(cmd)
	if err != nil {
		return err
	}

	output, _ := cmd.Flags().GetString("output")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger, _, _, closeLog := newLogger("error", "json")
	defer closeLog()

	pool, err := postgres.New(ctx, postgres.Config{
		URL:      dbURL,
		MaxConns: 2,
		MinConns: 1,
	}, logger)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	sc, err := schema.BuildCache(ctx, pool.DB())
	if err != nil {
		return fmt.Errorf("introspecting schema: %w", err)
	}

	result := typegen.TypeScript(sc)

	if output == "" {
		fmt.Print(result)
		return nil
	}

	if err := os.WriteFile(output, []byte(result), 0644); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Wrote %d bytes to %s\n", len(result), output)
	return nil
}

func runTypesOpenAPI(cmd *cobra.Command, args []string) error {
	dbURL, err := resolveTypesDatabaseURL(cmd)
	if err != nil {
		return err
	}

	output, _ := cmd.Flags().GetString("output")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger, _, _, closeLog := newLogger("error", "json")
	defer closeLog()

	pool, err := postgres.New(ctx, postgres.Config{
		URL:      dbURL,
		MaxConns: 2,
		MinConns: 1,
	}, logger)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	sc, err := schema.BuildCache(ctx, pool.DB())
	if err != nil {
		return fmt.Errorf("introspecting schema: %w", err)
	}

	data, err := openapi.Generate(sc, openapi.Options{BasePath: "/api"})
	if err != nil {
		return fmt.Errorf("generating OpenAPI spec: %w", err)
	}

	if output == "" {
		fmt.Print(string(data))
		return nil
	}

	if err := os.WriteFile(output, data, 0644); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Wrote %d bytes to %s\n", len(data), output)
	return nil
}

// resolveTypesDatabaseURL owns database discovery for both types output formats.
// Types directly prioritize DATABASE_URL, then config.Load applies ayb.toml plus
// normal AYB_* overrides such as AYB_DATABASE_URL, before the managed-Postgres
// fallback. resolveDBURL is not reused because it has no PID fallback.
func resolveTypesDatabaseURL(cmd *cobra.Command) (string, error) {
	if dbURL, _ := cmd.Flags().GetString("database-url"); dbURL != "" {
		return dbURL, nil
	}
	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		return dbURL, nil
	}

	cfg, err := config.Load("", nil)
	if err != nil {
		return "", fmt.Errorf("loading config: %w", err)
	}
	if cfg.Database.URL != "" {
		return cfg.Database.URL, nil
	}
	if _, _, err := readAYBPID(); err == nil {
		return fmt.Sprintf(
			"postgresql://ayb:ayb@127.0.0.1:%d/ayb?sslmode=disable",
			cfg.Database.EmbeddedPort,
		), nil
	}
	return "", fmt.Errorf("--database-url is required (or set DATABASE_URL, AYB_DATABASE_URL, or database.url in ayb.toml)")
}
