package codehealth

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const expectedSecuritySurfaceCount = 12

var expectedSecuritySurfaceTupleFingerprints = map[string]string{
	"JWT entropy":               "owners=internal/config/config_validate_auth.go tests=internal/config/config_jwt_secret_strength_test.go[TestValidateRejectsLowEntropyJWTSecrets]",
	"SAML signature":            "owners=internal/auth/saml_support.go tests=internal/auth/saml_signature_test.go[TestSAMLAssertionSignatureRejectsUnsignedOrCorruptSignature]",
	"auth RLS":                  "owners=internal/auth/rls.go tests=internal/auth/rls_integration_test.go[TestRLSTenantIsolation];internal/auth/rls_test.go[TestRLSStatements]",
	"storage RLS":               "owners=internal/storage/rls.go tests=internal/storage/storage_integration_test.go[TestStorageRLSUserIsolationAdminBypassAndPolicyUpdate]",
	"realtime fail-closed":      "owners=internal/realtime/handler.go tests=internal/realtime/visibility_test.go[TestCanSeeRecordNonPublicMissingMetadataFailsClosed]",
	"signed-URL tenant binding": "owners=internal/storage/storage.go tests=internal/storage/storage_integration_test.go[TestStorageSignedURLTenantIsolation];internal/storage/storage_test.go[TestSignAndValidateURL]",
	"traversal rejection":       "owners=internal/storage/local.go,internal/storage/storage.go tests=internal/storage/local_test.go[TestLocalBackendRejectsTraversalName];internal/storage/storage_test.go[TestValidateName]",
	"config masking":            "owners=internal/config/config_mask.go tests=internal/config/config_mask_completeness_test.go[TestMaskedCopyHasNoUnmaskedSecrets]",
	"log redaction":             "owners=internal/urlutil/redact.go tests=internal/urlutil/redact_test.go[TestRedactURL]",
	"metrics loopback":          "owners=internal/server/helpers.go tests=internal/server/metrics_test.go[TestMetricsEndpointTokenlessScrapeRejectsNonLoopbackAndUntrustedSources,TestMetricsEndpointTokenlessScrapeRequiresLoopbackRemoteAddr]",
	"OAuth return_to":           "owners=internal/auth/handler_oauth.go tests=internal/auth/oauth_return_to_test.go[TestOAuthRedirect_OpenRedirectAttackerRejected]",
	"SQL identifier quoting":    "owners=internal/sqlutil/sqlutil.go tests=internal/sqlutil/sqlutil_test.go[TestQuoteIdent]",
}

type securitySurfaceMapping struct {
	surface          string
	productionOwners []string
	tests            []securitySurfaceTest
}

type securitySurfaceTest struct {
	file      string
	functions []string
}

func TestSecuritySurfaceCoverage(t *testing.T) {
	t.Parallel()

	mappings := securitySurfaceMappings()
	repoRoot := findRepoRoot(t)
	coveredSurfaces := 0
	missing := missingSecuritySurfaceMappingConfig(mappings)

	for _, mapping := range mappings {
		surfaceMissing := missingSecuritySurfaceEvidence(t, repoRoot, mapping)
		if len(surfaceMissing) == 0 {
			coveredSurfaces++
			continue
		}
		missing = append(missing, fmt.Sprintf("%s: %s", mapping.surface, strings.Join(surfaceMissing, "; ")))
	}

	t.Logf("%d/%d security surfaces have named tests", coveredSurfaces, expectedSecuritySurfaceCount)
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf("security surface coverage gaps:\n- %s", strings.Join(missing, "\n- "))
	}
}

func securitySurfaceMappings() []securitySurfaceMapping {
	return []securitySurfaceMapping{
		{
			surface:          "JWT entropy",
			productionOwners: []string{"internal/config/config_validate_auth.go"},
			tests: []securitySurfaceTest{{
				file:      "internal/config/config_jwt_secret_strength_test.go",
				functions: []string{"TestValidateRejectsLowEntropyJWTSecrets"},
			}},
		},
		{
			surface:          "SAML signature",
			productionOwners: []string{"internal/auth/saml_support.go"},
			tests: []securitySurfaceTest{{
				file:      "internal/auth/saml_signature_test.go",
				functions: []string{"TestSAMLAssertionSignatureRejectsUnsignedOrCorruptSignature"},
			}},
		},
		{
			surface:          "auth RLS",
			productionOwners: []string{"internal/auth/rls.go"},
			tests: []securitySurfaceTest{
				{
					file:      "internal/auth/rls_test.go",
					functions: []string{"TestRLSStatements"},
				},
				{
					file:      "internal/auth/rls_integration_test.go",
					functions: []string{"TestRLSTenantIsolation"},
				},
			},
		},
		{
			surface:          "storage RLS",
			productionOwners: []string{"internal/storage/rls.go"},
			tests: []securitySurfaceTest{{
				file:      "internal/storage/storage_integration_test.go",
				functions: []string{"TestStorageRLSUserIsolationAdminBypassAndPolicyUpdate"},
			}},
		},
		{
			surface:          "realtime fail-closed",
			productionOwners: []string{"internal/realtime/handler.go"},
			tests: []securitySurfaceTest{{
				file:      "internal/realtime/visibility_test.go",
				functions: []string{"TestCanSeeRecordNonPublicMissingMetadataFailsClosed"},
			}},
		},
		{
			surface:          "signed-URL tenant binding",
			productionOwners: []string{"internal/storage/storage.go"},
			tests: []securitySurfaceTest{
				{
					file:      "internal/storage/storage_test.go",
					functions: []string{"TestSignAndValidateURL"},
				},
				{
					file:      "internal/storage/storage_integration_test.go",
					functions: []string{"TestStorageSignedURLTenantIsolation"},
				},
			},
		},
		{
			surface:          "traversal rejection",
			productionOwners: []string{"internal/storage/storage.go", "internal/storage/local.go"},
			tests: []securitySurfaceTest{
				{
					file:      "internal/storage/storage_test.go",
					functions: []string{"TestValidateName"},
				},
				{
					file:      "internal/storage/local_test.go",
					functions: []string{"TestLocalBackendRejectsTraversalName"},
				},
			},
		},
		{
			surface:          "config masking",
			productionOwners: []string{"internal/config/config_mask.go"},
			tests: []securitySurfaceTest{{
				file:      "internal/config/config_mask_completeness_test.go",
				functions: []string{"TestMaskedCopyHasNoUnmaskedSecrets"},
			}},
		},
		{
			surface:          "log redaction",
			productionOwners: []string{"internal/urlutil/redact.go"},
			tests: []securitySurfaceTest{{
				file:      "internal/urlutil/redact_test.go",
				functions: []string{"TestRedactURL"},
			}},
		},
		{
			surface:          "metrics loopback",
			productionOwners: []string{"internal/server/helpers.go"},
			tests: []securitySurfaceTest{{
				file: "internal/server/metrics_test.go",
				functions: []string{
					"TestMetricsEndpointTokenlessScrapeRequiresLoopbackRemoteAddr",
					"TestMetricsEndpointTokenlessScrapeRejectsNonLoopbackAndUntrustedSources",
				},
			}},
		},
		{
			surface:          "OAuth return_to",
			productionOwners: []string{"internal/auth/handler_oauth.go"},
			tests: []securitySurfaceTest{{
				file:      "internal/auth/oauth_return_to_test.go",
				functions: []string{"TestOAuthRedirect_OpenRedirectAttackerRejected"},
			}},
		},
		{
			surface:          "SQL identifier quoting",
			productionOwners: []string{"internal/sqlutil/sqlutil.go"},
			tests: []securitySurfaceTest{{
				file:      "internal/sqlutil/sqlutil_test.go",
				functions: []string{"TestQuoteIdent"},
			}},
		},
	}
}

func TestSecuritySurfaceMappingConfigGuards(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		build        func() []securitySurfaceMapping
		expectedGaps []string
	}{
		{
			name: "missing one mapping",
			build: func() []securitySurfaceMapping {
				mappings := append([]securitySurfaceMapping(nil), securitySurfaceMappings()...)
				return mappings[:len(mappings)-1]
			},
			expectedGaps: []string{"expected 12 security surface mappings, got 11"},
		},
		{
			name: "no mappings configured",
			build: func() []securitySurfaceMapping {
				return nil
			},
			expectedGaps: []string{
				"VACUOUS: no security surface mappings configured",
				"expected 12 security surface mappings, got 0",
			},
		},
		{
			name: "missing test entries",
			build: func() []securitySurfaceMapping {
				mappings := append([]securitySurfaceMapping(nil), securitySurfaceMappings()...)
				mappings[0].tests = nil
				return mappings
			},
			expectedGaps: []string{"JWT entropy: no test files configured"},
		},
		{
			name: "missing test functions",
			build: func() []securitySurfaceMapping {
				mappings := append([]securitySurfaceMapping(nil), securitySurfaceMappings()...)
				mappings[0].tests[0].functions = nil
				return mappings
			},
			expectedGaps: []string{"JWT entropy: internal/config/config_jwt_secret_strength_test.go has no required test functions"},
		},
		{
			name: "empty test functions",
			build: func() []securitySurfaceMapping {
				mappings := append([]securitySurfaceMapping(nil), securitySurfaceMappings()...)
				mappings[0].tests[0].functions = []string{}
				return mappings
			},
			expectedGaps: []string{"JWT entropy: internal/config/config_jwt_secret_strength_test.go has no required test functions"},
		},
		{
			name: "duplicate substitution",
			build: func() []securitySurfaceMapping {
				mappings := append([]securitySurfaceMapping(nil), securitySurfaceMappings()...)
				mappings[1].surface = mappings[0].surface
				return mappings
			},
			expectedGaps: []string{
				"duplicate security surface mapping: JWT entropy",
				"missing security surface mapping: SAML signature",
			},
		},
		{
			name: "unexpected surface label",
			build: func() []securitySurfaceMapping {
				mappings := append([]securitySurfaceMapping(nil), securitySurfaceMappings()...)
				mappings[0].surface = "JWT entropy drift"
				return mappings
			},
			expectedGaps: []string{
				"unexpected security surface mapping: JWT entropy drift",
				"missing security surface mapping: JWT entropy",
			},
		},
		{
			name: "nil production owners",
			build: func() []securitySurfaceMapping {
				mappings := append([]securitySurfaceMapping(nil), securitySurfaceMappings()...)
				mappings[0].productionOwners = nil
				return mappings
			},
			expectedGaps: []string{"JWT entropy: no production owners configured"},
		},
		{
			name: "empty production owners",
			build: func() []securitySurfaceMapping {
				mappings := append([]securitySurfaceMapping(nil), securitySurfaceMappings()...)
				mappings[0].productionOwners = []string{}
				return mappings
			},
			expectedGaps: []string{"JWT entropy: no production owners configured"},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			missing := missingSecuritySurfaceMappingConfig(test.build())
			for _, expectedGap := range test.expectedGaps {
				assertSecuritySurfaceGap(t, missing, expectedGap)
			}
		})
	}
}

func TestSecuritySurfaceMappingsRejectDuplicateSubstitution(t *testing.T) {
	t.Parallel()

	mappings := append([]securitySurfaceMapping(nil), securitySurfaceMappings()...)
	mappings[1].surface = mappings[0].surface

	missing := missingSecuritySurfaceMappingConfig(mappings)
	assertSecuritySurfaceGap(t, missing, "duplicate security surface mapping: JWT entropy")
	assertSecuritySurfaceGap(t, missing, "missing security surface mapping: SAML signature")
}

func TestSecuritySurfaceMappingsRejectTupleSubstitution(t *testing.T) {
	t.Parallel()

	mappings := append([]securitySurfaceMapping(nil), securitySurfaceMappings()...)
	mappings[1].productionOwners = append([]string(nil), mappings[0].productionOwners...)
	mappings[1].tests = append([]securitySurfaceTest(nil), mappings[0].tests...)

	missing := missingSecuritySurfaceMappingConfig(mappings)
	assertSecuritySurfaceGap(t, missing, "SAML signature: mapping tuple mismatch")
}

func missingSecuritySurfaceMappingConfig(mappings []securitySurfaceMapping) []string {
	missing := make([]string, 0)
	if len(mappings) == 0 {
		missing = append(missing, "VACUOUS: no security surface mappings configured")
	}
	if len(mappings) != expectedSecuritySurfaceCount {
		missing = append(missing, fmt.Sprintf("expected %d security surface mappings, got %d", expectedSecuritySurfaceCount, len(mappings)))
	}
	missing = append(missing, missingDistinctSurfaceLabels(mappings)...)
	missing = append(missing, missingChangedSecuritySurfaceTuples(mappings)...)

	for _, mapping := range mappings {
		if len(mapping.productionOwners) == 0 {
			missing = append(missing, fmt.Sprintf("%s: no production owners configured", mapping.surface))
		}
		if len(mapping.tests) == 0 {
			missing = append(missing, fmt.Sprintf("%s: no test files configured", mapping.surface))
			continue
		}
		for _, test := range mapping.tests {
			if len(test.functions) == 0 {
				missing = append(
					missing,
					fmt.Sprintf("%s: %s has no required test functions", mapping.surface, test.file),
				)
			}
		}
	}
	return missing
}

func missingDistinctSurfaceLabels(mappings []securitySurfaceMapping) []string {
	missing := make([]string, 0)

	expectedLabels := make([]string, 0, len(expectedSecuritySurfaceTupleFingerprints))
	expectedSet := make(map[string]struct{}, len(expectedLabels))
	for label := range expectedSecuritySurfaceTupleFingerprints {
		expectedLabels = append(expectedLabels, label)
		expectedSet[label] = struct{}{}
	}
	sort.Strings(expectedLabels)

	seen := make(map[string]struct{}, len(mappings))
	for _, mapping := range mappings {
		if _, dup := seen[mapping.surface]; dup {
			missing = append(missing, fmt.Sprintf("duplicate security surface mapping: %s", mapping.surface))
		}
		seen[mapping.surface] = struct{}{}
		if _, ok := expectedSet[mapping.surface]; !ok {
			missing = append(missing, fmt.Sprintf("unexpected security surface mapping: %s", mapping.surface))
		}
	}
	for _, label := range expectedLabels {
		if _, ok := seen[label]; !ok {
			missing = append(missing, fmt.Sprintf("missing security surface mapping: %s", label))
		}
	}
	return missing
}

func missingChangedSecuritySurfaceTuples(mappings []securitySurfaceMapping) []string {
	missing := make([]string, 0)
	for _, mapping := range mappings {
		if want, ok := expectedSecuritySurfaceTupleFingerprints[mapping.surface]; ok && securitySurfaceTupleFingerprint(mapping) != want {
			missing = append(missing, fmt.Sprintf("%s: mapping tuple mismatch", mapping.surface))
		}
	}
	return missing
}

func securitySurfaceTupleFingerprint(mapping securitySurfaceMapping) string {
	owners := append([]string(nil), mapping.productionOwners...)
	sort.Strings(owners)
	tests := make([]string, 0, len(mapping.tests))
	for _, test := range mapping.tests {
		functions := append([]string(nil), test.functions...)
		sort.Strings(functions)
		tests = append(tests, fmt.Sprintf("%s[%s]", test.file, strings.Join(functions, ",")))
	}
	sort.Strings(tests)
	return fmt.Sprintf("owners=%s tests=%s", strings.Join(owners, ","), strings.Join(tests, ";"))
}

func assertSecuritySurfaceGap(t *testing.T, missing []string, expectedGap string) {
	t.Helper()

	for _, gap := range missing {
		if gap == expectedGap {
			return
		}
	}
	t.Fatalf("expected gap %q, got %v", expectedGap, missing)
}

func missingSecuritySurfaceEvidence(t *testing.T, repoRoot string, mapping securitySurfaceMapping) []string {
	t.Helper()

	missing := make([]string, 0)
	for _, owner := range mapping.productionOwners {
		if err := requireRelativeFile(repoRoot, owner); err != nil {
			missing = append(missing, err.Error())
		}
	}
	for _, test := range mapping.tests {
		if !strings.HasSuffix(test.file, "_test.go") {
			missing = append(missing, fmt.Sprintf("%s is not a _test.go file", test.file))
			continue
		}
		if err := requireRelativeFile(repoRoot, test.file); err != nil {
			missing = append(missing, err.Error())
			continue
		}
		functions, err := topLevelFunctions(repoRoot, test.file)
		if err != nil {
			missing = append(missing, fmt.Sprintf("%s: %v", test.file, err))
			continue
		}
		for _, function := range test.functions {
			if _, exists := functions[function]; !exists {
				missing = append(missing, fmt.Sprintf("%s missing top-level test function %s", test.file, function))
			}
		}
	}
	return missing
}

func requireRelativeFile(repoRoot, relativePath string) error {
	path := filepath.Join(repoRoot, filepath.FromSlash(relativePath))
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s missing: %w", relativePath, err)
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory, want file", relativePath)
	}
	return nil
}

func topLevelFunctions(repoRoot, relativePath string) (map[string]struct{}, error) {
	source, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(relativePath)))
	if err != nil {
		return nil, err
	}

	fileSet := token.NewFileSet()
	parsedFile, err := parser.ParseFile(fileSet, relativePath, source, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}

	functions := make(map[string]struct{})
	for _, declaration := range parsedFile.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Recv != nil {
			continue
		}
		functions[function.Name.Name] = struct{}{}
	}
	return functions, nil
}
