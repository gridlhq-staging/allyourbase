//go:build integration

package auth_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func assertSDKContractFixture(t *testing.T, name string, got any) {
	t.Helper()

	fixture, ok := loadSDKContractFixture(t, name)
	if !ok {
		t.Fatalf("sdk contract fixture %s is missing; sanitized payload:\n%s", name, mustIndentedJSON(t, got))
	}
	if reflect.DeepEqual(fixture, got) {
		return
	}

	t.Fatalf("sdk contract fixture %s drifted\nwant:\n%s\ngot:\n%s",
		name,
		mustIndentedJSON(t, fixture),
		mustIndentedJSON(t, got),
	)
}

func loadSDKContractFixture(t *testing.T, name string) (any, bool) {
	t.Helper()

	path := sdkContractFixturePath(t, name)
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false
		}
		testutil.NoError(t, err)
	}
	var fixture any
	testutil.NoError(t, json.Unmarshal(body, &fixture))
	return fixture, true
}

func sdkContractFixturePath(t *testing.T, name string) string {
	t.Helper()
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsAny(name, `/\`) {
		t.Fatalf("sdk contract fixture name must be a single filename: %q", name)
	}

	dir, err := os.Getwd()
	testutil.NoError(t, err)
	for {
		candidate := filepath.Join(dir, "tests", "contract", "fixtures", "sdk_contract", name)
		if _, err := os.Stat(filepath.Dir(candidate)); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate sdk contract fixture directory from %s", dir)
		}
		dir = parent
	}
}

func mustIndentedJSON(t *testing.T, v any) string {
	t.Helper()
	encoded, err := json.MarshalIndent(v, "", "  ")
	testutil.NoError(t, err)
	return string(encoded)
}

func sanitizeWebAuthnEnrollBeginFixture(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()

	clone := cloneJSONMap(t, payload)
	setString(clone, "challenge", "webauthn_enroll_begin_challenge")
	if user, ok := clone["user"].(map[string]any); ok {
		user["id"] = "webauthn_enroll_user_id"
	}
	sanitizeCredentialDescriptorIDs(clone["excludeCredentials"], "webauthn_enroll_exclude_credential")
	return clone
}

func sanitizeWebAuthnEnrollConfirmRequestFixture(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()

	clone := cloneJSONMap(t, payload)
	if response, ok := clone["attestation_response"].(map[string]any); ok {
		sanitizeCredentialResponse(response, "webauthn_enroll_credential")
		normalizeClientDataJSON(t, response["response"], "webauthn_enroll_begin_challenge")
		sanitizeNestedString(response["response"], "attestationObject", "webauthn_enroll_attestation_object")
	}
	return clone
}

func sanitizeWebAuthnMFAChallengeFixture(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()

	clone := cloneJSONMap(t, payload)
	clone["challenge_id"] = "webauthn_mfa_challenge_fixture"
	if options, ok := clone["options"].(map[string]any); ok {
		options["challenge"] = "webauthn_mfa_challenge"
		sanitizeCredentialDescriptorIDs(options["allowCredentials"], "webauthn_mfa_credential")
	}
	return clone
}

func sanitizeWebAuthnDiscoverBeginFixture(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()

	clone := cloneJSONMap(t, payload)
	clone["challenge_id"] = "webauthn_discover_challenge_fixture"
	if options, ok := clone["options"].(map[string]any); ok {
		options["challenge"] = "webauthn_discover_challenge"
	}
	return clone
}

func sanitizeWebAuthnDiscoverFinishRequestFixture(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()

	clone := cloneJSONMap(t, payload)
	clone["challenge_id"] = "webauthn_discover_challenge_fixture"
	if response, ok := clone["assertion_response"].(map[string]any); ok {
		sanitizeCredentialResponse(response, "webauthn_discover_credential")
		normalizeClientDataJSON(t, response["response"], "webauthn_discover_challenge")
		sanitizeNestedString(response["response"], "signature", "webauthn_discover_signature")
		sanitizeNestedString(response["response"], "userHandle", "webauthn_discover_user_handle")
	}
	return clone
}

func sanitizeWebAuthnMFAVerifyRequestFixture(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()

	clone := cloneJSONMap(t, payload)
	clone["challenge_id"] = "webauthn_mfa_challenge_fixture"
	if response, ok := clone["assertion_response"].(map[string]any); ok {
		sanitizeCredentialResponse(response, "webauthn_mfa_credential")
		normalizeClientDataJSON(t, response["response"], "webauthn_mfa_challenge")
		sanitizeNestedString(response["response"], "signature", "webauthn_mfa_signature")
		sanitizeNestedString(response["response"], "userHandle", "webauthn_mfa_user_handle")
	}
	return clone
}

func sanitizeWebAuthnMFAVerifyResponseFixture(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()

	clone := cloneJSONMap(t, payload)
	clone["token"] = "jwt_webauthn_mfa"
	clone["refreshToken"] = "refresh_webauthn_mfa"
	if user, ok := clone["user"].(map[string]any); ok {
		user["id"] = "usr_webauthn_mfa"
		setString(user, "createdAt", "2026-07-11T00:00:00Z")
		setString(user, "updatedAt", "2026-07-11T00:00:00Z")
		setString(user, "created_at", "2026-07-11T00:00:00Z")
		setString(user, "updated_at", "2026-07-11T00:00:00Z")
	}
	return clone
}

func setString(payload map[string]any, key string, replacement string) {
	if _, found := payload[key]; found {
		payload[key] = replacement
	}
}

func sanitizeCredentialResponse(payload map[string]any, prefix string) {
	setString(payload, "id", prefix)
	setString(payload, "rawId", prefix)
	setString(payload, "raw_id", prefix)
}

func sanitizeCredentialDescriptorIDs(value any, prefix string) {
	credentials, ok := value.([]any)
	if !ok {
		return
	}
	for i, raw := range credentials {
		if credential, ok := raw.(map[string]any); ok {
			credential["id"] = prefix + "_" + string(rune('a'+i))
		}
	}
}

func sanitizeNestedString(value any, key string, replacement string) {
	if payload, ok := value.(map[string]any); ok {
		if _, found := payload[key]; found {
			payload[key] = replacement
		}
	}
}

func normalizeClientDataJSON(t *testing.T, value any, challenge string) {
	t.Helper()

	payload, ok := value.(map[string]any)
	if !ok {
		return
	}
	clientDataJSON, _ := payload["clientDataJSON"].(string)
	if clientDataJSON == "" {
		return
	}

	decoded, err := base64.RawURLEncoding.DecodeString(clientDataJSON)
	testutil.NoError(t, err)

	var fields map[string]json.RawMessage
	testutil.NoError(t, json.Unmarshal(decoded, &fields))

	originalChallenge, found := fields["challenge"]
	if !found {
		t.Fatalf("clientDataJSON missing challenge field: %s", string(decoded))
	}

	replacement, err := json.Marshal(challenge)
	testutil.NoError(t, err)
	updated, ok := replaceJSONObjectFieldValue(decoded, []byte(`"challenge"`), originalChallenge, replacement)
	if !ok {
		t.Fatalf("clientDataJSON challenge field bytes could not be replaced: %s", string(decoded))
	}
	payload["clientDataJSON"] = encodeBase64URL(updated)
}

func cloneJSONMap(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()

	encoded, err := json.Marshal(payload)
	testutil.NoError(t, err)
	var clone map[string]any
	testutil.NoError(t, json.Unmarshal(encoded, &clone))
	return clone
}

func decodeBase64URLJSON(value string, dest any) error {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(decoded, dest)
}

func encodeBase64URL(value []byte) string {
	return base64.RawURLEncoding.EncodeToString(value)
}

func replaceJSONObjectFieldValue(document []byte, key []byte, oldValue []byte, newValue []byte) ([]byte, bool) {
	keyIndex := bytes.Index(document, key)
	if keyIndex < 0 {
		return nil, false
	}
	valueStart := keyIndex + len(key)
	for valueStart < len(document) && isJSONSpace(document[valueStart]) {
		valueStart++
	}
	if valueStart >= len(document) || document[valueStart] != ':' {
		return nil, false
	}
	valueStart++
	for valueStart < len(document) && isJSONSpace(document[valueStart]) {
		valueStart++
	}
	valueEnd := valueStart + len(oldValue)
	if valueEnd > len(document) || !bytes.Equal(document[valueStart:valueEnd], oldValue) {
		return nil, false
	}

	updated := make([]byte, 0, len(document)-len(oldValue)+len(newValue))
	updated = append(updated, document[:valueStart]...)
	updated = append(updated, newValue...)
	updated = append(updated, document[valueEnd:]...)
	return updated, true
}

func isJSONSpace(value byte) bool {
	switch value {
	case ' ', '\n', '\r', '\t':
		return true
	default:
		return false
	}
}

func TestNormalizeClientDataJSONPreservesAdditionalFields(t *testing.T) {
	payload := map[string]any{
		"clientDataJSON": encodeBase64URL([]byte(`{"type":"webauthn.get","challenge":"live-challenge","origin":"http://127.0.0.1:8090","crossOrigin":false}`)),
	}

	normalizeClientDataJSON(t, payload, "webauthn_mfa_challenge")

	var normalized struct {
		Type        string `json:"type"`
		Challenge   string `json:"challenge"`
		Origin      string `json:"origin"`
		CrossOrigin *bool  `json:"crossOrigin"`
	}
	testutil.NoError(t, decodeBase64URLJSON(payload["clientDataJSON"].(string), &normalized))
	if normalized.CrossOrigin == nil {
		t.Fatal("expected clientDataJSON optional fields to be preserved")
	}
	if normalized.Challenge != "webauthn_mfa_challenge" {
		t.Fatalf("expected normalized challenge, got %q", normalized.Challenge)
	}
	if normalized.Type != "webauthn.get" {
		t.Fatalf("expected type to be preserved, got %q", normalized.Type)
	}
	if normalized.Origin != "http://127.0.0.1:8090" {
		t.Fatalf("expected origin to be preserved, got %q", normalized.Origin)
	}
	if *normalized.CrossOrigin {
		t.Fatalf("expected crossOrigin false, got true")
	}
}
