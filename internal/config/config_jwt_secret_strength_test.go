package config

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"
)

func validateJWTSecretForTest(authEnabled bool, secret string) error {
	cfg := Default()
	cfg.Auth.Enabled = authEnabled
	cfg.Auth.JWTSecret = secret
	return cfg.Validate()
}

func TestValidateRejectsKnownPlaceholderJWTSecrets(t *testing.T) {
	tests := []struct {
		name   string
		secret string
		source string
	}{
		{
			name:   "docker compose and postgis guide placeholder",
			secret: "change-me-to-a-secure-random-string-at-least-32-chars",
			source: "docker-compose.yml:9 and docs-site/guide/postgis.md:59",
		},
		{
			name:   "authentication and oauth provider guide placeholder",
			secret: "your-secret-key-at-least-32-characters-long",
			source: "docs-site/guide/authentication.md:12,19 and docs-site/guide/oauth-provider.md:22",
		},
		{
			name:   "kanban tutorial placeholder",
			secret: "replace-with-a-random-secret-at-least-32-chars-long",
			source: "docs-site/guide/tutorial-kanban.md:55",
		},
		{
			name:   "deployment guide placeholder",
			secret: "replace-with-a-long-random-secret",
			source: "docs-site/guide/deployment.md:49",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateJWTSecretForTest(true, tt.secret)
			if err == nil {
				t.Fatalf("Validate accepted placeholder JWT secret from %s", tt.source)
			}

			message := err.Error()
			if !strings.Contains(message, "auth.jwt_secret") {
				t.Fatalf("Validate error = %q, want auth.jwt_secret field name", message)
			}
			if !strings.Contains(message, "openssl rand -hex 32") {
				t.Fatalf("Validate error = %q, want openssl rand -hex 32 remedy", message)
			}
			if strings.Contains(message, "must be at least 32 characters") {
				t.Fatalf("Validate error = %q, want strength error instead of length error", message)
			}
		})
	}
}

func TestValidateRejectsLowEntropyJWTSecrets(t *testing.T) {
	tests := []struct {
		name   string
		secret string
	}{
		{
			name:   "one distinct rune",
			secret: strings.Repeat("a", 32), // One distinct rune gives no meaningful signing-key entropy.
		},
		{
			name:   "two distinct runes",
			secret: strings.Repeat("ab", 20), // Two distinct runes are still trivially guessable despite sufficient length.
		},
		{
			name:   "repeated ten rune cycle",
			secret: strings.Repeat("0123456789", 4), // Ten distinct runes can still be a repeated cycle a distinct-count-only rule would miss.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateJWTSecretForTest(true, tt.secret)
			if err == nil {
				t.Fatalf("Validate accepted low-entropy JWT secret %q", tt.secret)
			}

			message := err.Error()
			if !strings.Contains(message, "auth.jwt_secret") {
				t.Fatalf("Validate error = %q, want auth.jwt_secret field name", message)
			}
			if !strings.Contains(message, "openssl rand -hex 32") {
				t.Fatalf("Validate error = %q, want openssl rand -hex 32 remedy", message)
			}
			if strings.Contains(message, "must be at least 32 characters") {
				t.Fatalf("Validate error = %q, want strength error instead of length error", message)
			}
		})
	}
}

// TestValidateJWTSecretRejectionBranchesAreDistinct proves the three rejection
// rules — normalized known-default, placeholder-marker, and low-entropy — each
// produce their own actionable error and cannot satisfy one another's contract.
// This is what keeps the ordered rule set honest: a normalized known default
// must report the known-default branch even though it also contains a marker
// substring, and a varied marker secret must report the marker branch rather
// than the entropy branch.
func TestValidateJWTSecretRejectionBranchesAreDistinct(t *testing.T) {
	const (
		knownDefaultPhrase = "well-known published default"
		markerPhrase       = "placeholder marker"
		entropyPhrase      = "insufficient entropy"
	)

	tests := []struct {
		name   string
		secret string
		want   string
		forbid []string
	}{
		{
			// Trim + case normalization must still map onto the published default.
			name:   "trim and case normalized known default",
			secret: "  CHANGE-ME-TO-A-SECURE-RANDOM-STRING-AT-LEAST-32-CHARS  ",
			want:   knownDefaultPhrase,
			forbid: []string{markerPhrase, entropyPhrase},
		},
		{
			// Contains the "change-me" marker but is varied, non-repeating, and
			// not equal to any published default, so it must hit the marker branch
			// (which is ordered before the entropy branch).
			name:   "varied marker secret that is not a known default",
			secret: "my-team-please-change-me-before-you-deploy-9x7q",
			want:   markerPhrase,
			forbid: []string{knownDefaultPhrase, entropyPhrase},
		},
		{
			// Ten distinct runes defeats a distinct-count-only rule; the repeated
			// cycle must still be rejected by the repetition rule.
			name:   "repeated ten rune cycle",
			secret: strings.Repeat("0123456789", 4),
			want:   entropyPhrase,
			forbid: []string{knownDefaultPhrase, markerPhrase},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateJWTSecretForTest(true, tt.secret)
			if err == nil {
				t.Fatalf("Validate accepted rejectable JWT secret for %q case", tt.name)
			}

			message := err.Error()
			if !strings.Contains(message, "auth.jwt_secret") {
				t.Fatalf("Validate error = %q, want auth.jwt_secret field name", message)
			}
			if !strings.Contains(message, "openssl rand -hex 32") {
				t.Fatalf("Validate error = %q, want openssl rand -hex 32 remedy", message)
			}
			if !strings.Contains(message, tt.want) {
				t.Fatalf("Validate error = %q, want branch phrase %q", message, tt.want)
			}
			for _, forbidden := range tt.forbid {
				if strings.Contains(message, forbidden) {
					t.Fatalf("Validate error = %q must not contain other branch phrase %q", message, forbidden)
				}
			}
			if strings.Contains(message, strings.TrimSpace(tt.secret)) {
				t.Fatalf("Validate error must not echo the rejected secret; got %q", message)
			}
		})
	}
}

// TestValidateRejectsReplaceThisMarkerJWTSecret keeps the explicit
// "replace-this" marker branch covered even though the docs no longer publish a
// copy-paste JWT literal. If an operator pastes an instructional stub instead
// of a generated value, validation must still refuse startup with a remedy.
func TestValidateRejectsReplaceThisMarkerJWTSecret(t *testing.T) {
	const markerPlaceholder = "replace-this-with-openssl-rand-hex-32"

	err := validateJWTSecretForTest(true, markerPlaceholder)
	if err == nil {
		t.Fatalf("Validate accepted replace-this marker placeholder %q; a pasted instructional stub must never boot", markerPlaceholder)
	}
	if !strings.Contains(err.Error(), "openssl rand -hex 32") {
		t.Fatalf("Validate error = %q, want openssl rand -hex 32 remedy", err.Error())
	}
}

func TestValidateAcceptsStrongJWTSecrets(t *testing.T) {
	for i := 0; i < 64; i++ {
		if err := validateJWTSecretForTest(true, randomHexJWTSecret(t)); err != nil {
			t.Fatalf("Validate rejected random hex JWT secret at iteration %d: %v", i, err)
		}
	}

	for i := 0; i < 64; i++ {
		if err := validateJWTSecretForTest(true, randomBase64JWTSecret(t)); err != nil {
			t.Fatalf("Validate rejected random base64 JWT secret at iteration %d: %v", i, err)
		}
	}

	// These in-tree secrets protect packages this lane may not edit.
	tests := []string{
		"this-is-a-secret-that-is-at-least-32-characters-long",
		"integration-test-secret-that-is-at-least-32-chars!!",
		"demo-integration-test-secret-that-is-at-least-32-chars!!",
		"e2e-integration-test-secret-that-is-at-least-32-chars!!",
		"stage4-load-jwt-secret-0123456789",
		"sdk-parity-jwt-secret-that-is-at-least-32-bytes",
		"test-secret-that-is-at-least-32-chars-long",
	}

	for _, secret := range tests {
		t.Run(secret, func(t *testing.T) {
			if err := validateJWTSecretForTest(true, secret); err != nil {
				t.Fatalf("Validate rejected in-tree JWT secret %q: %v", secret, err)
			}
		})
	}
}

func TestValidateJWTSecretStrengthOnlyWhenAuthEnabled(t *testing.T) {
	if err := validateJWTSecretForTest(false, strings.Repeat("a", 32)); err != nil {
		t.Fatalf("Validate rejected weak JWT secret while auth disabled: %v", err)
	}

	err := validateJWTSecretForTest(true, "")
	if err == nil {
		t.Fatal("Validate accepted empty JWT secret while auth enabled")
	}
	if got, want := err.Error(), "auth.jwt_secret is required when auth is enabled"; got != want {
		t.Fatalf("Validate error = %q, want %q", got, want)
	}

	err = validateJWTSecretForTest(true, "0123456789abcdef0123456789abcde")
	if err == nil {
		t.Fatal("Validate accepted 31-character JWT secret")
	}
	if got, want := err.Error(), "auth.jwt_secret must be at least 32 characters, got 31"; got != want {
		t.Fatalf("Validate error = %q, want %q", got, want)
	}
}

func randomHexJWTSecret(t *testing.T) string {
	t.Helper()

	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		t.Fatalf("generate random hex JWT secret: %v", err)
	}
	return hex.EncodeToString(secret)
}

func randomBase64JWTSecret(t *testing.T) string {
	t.Helper()

	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		t.Fatalf("generate random base64 JWT secret: %v", err)
	}
	return base64.StdEncoding.EncodeToString(secret)
}
