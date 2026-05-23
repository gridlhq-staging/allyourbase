package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGraphQLDefaults(t *testing.T) {
	t.Parallel()
	cfg := Default()
	if cfg.GraphQL.MaxDepth != 0 {
		t.Fatalf("expected GraphQL.MaxDepth default 0, got %d", cfg.GraphQL.MaxDepth)
	}
	if cfg.GraphQL.MaxComplexity != 0 {
		t.Fatalf("expected GraphQL.MaxComplexity default 0, got %d", cfg.GraphQL.MaxComplexity)
	}
	if cfg.GraphQL.Introspection != "" {
		t.Fatalf("expected GraphQL.Introspection default empty string, got %q", cfg.GraphQL.Introspection)
	}
}

func TestGraphQLIntrospectionValidation(t *testing.T) {
	t.Parallel()
	cfg := Default()
	cfg.GraphQL.Introspection = "invalid"
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected validation error for invalid graphql.introspection")
	}
}

func TestFlyAybTomlEnablesGraphQLFromCommittedConfig(t *testing.T) {
	t.Parallel()

	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(workingDirectory, "..", ".."))
	configPath := filepath.Join(repoRoot, "deploy", "fly", "ayb.toml")

	cfg, err := Load(configPath, nil)
	if err != nil {
		t.Fatalf("Load(%q) returned error: %v", configPath, err)
	}
	if !cfg.GraphQL.Enabled {
		t.Fatalf("expected graphql.enabled=true in %s", configPath)
	}
}
