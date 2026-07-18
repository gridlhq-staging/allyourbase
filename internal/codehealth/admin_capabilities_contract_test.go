package codehealth

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"testing"
)

const expectedAdminCapabilityNameCount = 15

var goAdminCapabilityNamePattern = regexp.MustCompile(`adminCapability\w+\s*=\s*"([a-z_]+)"`)

func TestAdminCapabilityNamesGoMatchesTS(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	goSource := readAdminCapabilityContractFile(t, repoRoot, "internal/server/admin_capabilities.go")
	tsSource := readAdminCapabilityContractFile(t, repoRoot, "ui/src/api_capabilities.ts")

	goNames := parseGoAdminCapabilityNames(goSource)
	tsNames := parseStringLiteralArrayMatch(tsSource, "ADMIN_CAPABILITY_NAMES")

	// Empty or partial parses must not pass vacuously just because both sides
	// miss the same capability names.
	assertAdminCapabilityNameCount(t, "Go admin capability constants", goNames)
	assertAdminCapabilityNameCount(t, "TypeScript ADMIN_CAPABILITY_NAMES", tsNames)
	assertAdminCapabilityNameDivergence(t, goNames, tsNames, nil, nil)
}

func TestAdminCapabilityNamesParserKnownAnswer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		goSource   string
		tsSource   string
		wantGo     []string
		wantTS     []string
		wantGoOnly []string
		wantTSOnly []string
	}{
		{
			name: "agreeing pair",
			goSource: `const (
	adminCapabilityAuth = "auth"
	adminCapabilityJobs = "jobs"
)`,
			tsSource: `export const ADMIN_CAPABILITY_NAMES = [
  "auth",
  "jobs",
] as const;`,
			wantGo: []string{"auth", "jobs"},
			wantTS: []string{"auth", "jobs"},
		},
		{
			name: "go only extra capability",
			goSource: `const (
	adminCapabilityAuth = "auth"
	adminCapabilityJobs = "jobs"
	adminCapabilitySupport = "support"
)`,
			tsSource: `export const ADMIN_CAPABILITY_NAMES = [
  "auth",
  "jobs",
] as const;`,
			wantGo:     []string{"auth", "jobs", "support"},
			wantTS:     []string{"auth", "jobs"},
			wantGoOnly: []string{"support"},
		},
		{
			name: "typescript only extra capability",
			goSource: `const (
	adminCapabilityAuth = "auth"
	adminCapabilityJobs = "jobs"
)`,
			tsSource: `export const ADMIN_CAPABILITY_NAMES = [
  "auth",
  "jobs",
  "storage",
] as const;`,
			wantGo:     []string{"auth", "jobs"},
			wantTS:     []string{"auth", "jobs", "storage"},
			wantTSOnly: []string{"storage"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			goNames := parseGoAdminCapabilityNames(tt.goSource)
			tsNames := parseStringLiteralArrayMatch(tt.tsSource, "ADMIN_CAPABILITY_NAMES")

			assertAdminCapabilityNames(t, "Go parser", goNames, tt.wantGo)
			assertAdminCapabilityNames(t, "TypeScript parser", tsNames, tt.wantTS)
			assertAdminCapabilityNameDivergence(t, goNames, tsNames, tt.wantGoOnly, tt.wantTSOnly)
		})
	}
}

func readAdminCapabilityContractFile(t *testing.T, repoRoot, relativePath string) string {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(repoRoot, relativePath))
	if err != nil {
		t.Fatalf("read %s: %v", relativePath, err)
	}
	return string(content)
}

func parseGoAdminCapabilityNames(source string) []string {
	matches := goAdminCapabilityNamePattern.FindAllStringSubmatch(source, -1)
	if len(matches) == 0 {
		return nil
	}

	names := make([]string, 0, len(matches))
	for _, match := range matches {
		names = append(names, match[1])
	}
	return names
}

func assertAdminCapabilityNameCount(t *testing.T, sourceDescription string, names []string) {
	t.Helper()

	if len(names) != expectedAdminCapabilityNameCount {
		t.Fatalf("%s parsed %d names, want %d: %v", sourceDescription, len(names), expectedAdminCapabilityNameCount, names)
	}
}

func assertAdminCapabilityNames(t *testing.T, sourceDescription string, got, want []string) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s parsed names mismatch\nwant: %v\ngot:  %v", sourceDescription, want, got)
	}
}

func assertAdminCapabilityNameDivergence(t *testing.T, goNames, tsNames, wantGoOnly, wantTSOnly []string) {
	t.Helper()

	goOnly, tsOnly := adminCapabilityNameDivergence(goNames, tsNames)
	if !reflect.DeepEqual(goOnly, wantGoOnly) || !reflect.DeepEqual(tsOnly, wantTSOnly) {
		t.Fatalf("admin capability name divergence mismatch\nwant Go-only: %v\n got Go-only: %v\nwant TS-only: %v\n got TS-only: %v", wantGoOnly, goOnly, wantTSOnly, tsOnly)
	}
}

func adminCapabilityNameDivergence(goNames, tsNames []string) ([]string, []string) {
	goSet := adminCapabilityNameSet(goNames)
	tsSet := adminCapabilityNameSet(tsNames)

	return adminCapabilityNamesOnlyIn(goSet, tsSet), adminCapabilityNamesOnlyIn(tsSet, goSet)
}

func adminCapabilityNameSet(names []string) map[string]struct{} {
	set := make(map[string]struct{}, len(names))
	for _, name := range names {
		set[name] = struct{}{}
	}
	return set
}

func adminCapabilityNamesOnlyIn(source, target map[string]struct{}) []string {
	var only []string
	for name := range source {
		if _, ok := target[name]; !ok {
			only = append(only, name)
		}
	}
	sort.Strings(only)
	return only
}
