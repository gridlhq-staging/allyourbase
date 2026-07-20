package config

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

// isSecretFieldName applies a default-deny contract: a tag that looks secret
// is secret unless it is in an explicit, individually justified allowlist.
func isSecretFieldName(tomlTag string) bool {
	tag := strings.ToLower(tomlTag)
	allowlist := map[string]bool{
		"key_id":                  true, // Public identifier selecting a key, not key material.
		"kms_key_id":              true, // Public AWS KMS key identifier, not key material.
		"key_file":                true, // Filesystem path to externally protected key material.
		"sp_key_file":             true, // Filesystem path to the SAML service-provider key.
		"credentials_file":        true, // Filesystem path to externally protected credentials.
		"custom_access_token":     true, // Auth-hook function name, not an access token value.
		"before_password_reset":   true, // Auth-hook function name, not a password value.
		"send_rate_limit_per_key": true, // Numeric per-caller rate limit, not a credential.
		"token_duration":          true, // Numeric token lifetime, not a token value.
		"refresh_token_duration":  true, // Numeric refresh-token lifetime, not a token value.
		"access_token_duration":   true, // Numeric access-token lifetime, not a token value.
		"auth_code_duration":      true, // Numeric authorization-code lifetime, not a credential.
	}
	if allowlist[tag] {
		return false
	}
	for _, marker := range []string{
		"secret", "token", "password", "apikey", "api_key", "private_key", "credential",
	} {
		if strings.Contains(tag, marker) {
			return true
		}
	}
	return tag == "key" ||
		strings.HasPrefix(tag, "key_") ||
		strings.HasSuffix(tag, "_key") ||
		strings.Contains(tag, "_key_")
}

func TestMaskCompletenessClassifierKnownAnswer(t *testing.T) {
	tests := []struct {
		tag  string
		want bool
	}{
		{tag: "secret", want: true},
		{tag: "TOKEN", want: true},
		{tag: "password", want: true},
		{tag: "apikey", want: true},
		{tag: "api_key", want: true},
		{tag: "private_key", want: true},
		{tag: "credential", want: true},
		{tag: "key", want: true},
		{tag: "jwt_secret", want: true},
		{tag: "encryption_key", want: true},
		{tag: "twilio_token", want: true},
		{tag: "plivo_auth_token", want: true},
		{tag: "telnyx_api_key", want: true},
		{tag: "msg91_auth_key", want: true},
		{tag: "vonage_api_key", want: true},
		{tag: "vonage_api_secret", want: true},
		{tag: "sms_webhook_secret", want: true},
		{tag: "master_key", want: true},
		{tag: "client_secret", want: true},
		{tag: "webhook_secret", want: true},
		{tag: "s3_access_key", want: true},
		{tag: "s3_secret_key", want: true},
		{tag: "stripe_secret_key", want: true},
		{tag: "stripe_webhook_secret", want: true},
		{tag: "api_token", want: true},
		{tag: "signing_secret", want: true},
		{tag: "auth_token", want: true},
		{tag: "access_key", want: true},
		{tag: "key_id", want: false},
		{tag: "kms_key_id", want: false},
		{tag: "key_file", want: false},
		{tag: "sp_key_file", want: false},
		{tag: "credentials_file", want: false},
		{tag: "custom_access_token", want: false},
		{tag: "before_password_reset", want: false},
		{tag: "send_rate_limit_per_key", want: false},
		{tag: "token_duration", want: false},
		{tag: "refresh_token_duration", want: false},
		{tag: "access_token_duration", want: false},
		{tag: "auth_code_duration", want: false},
		{tag: "url", want: false},
		{tag: "twilio_sid", want: false},
		{tag: "provider", want: false},
		{tag: "enabled", want: false},
		{tag: "from_name", want: false},
		{tag: "s3_bucket", want: false},
		{tag: "team_id", want: false},
		{tag: "zone_id", want: false},
		{tag: "distribution_id", want: false},
		{tag: "stripe_starter_price_id", want: false},
		{tag: "plivo_auth_id", want: false},
		{tag: "msg91_template_id", want: false},
		{tag: "monkey", want: false},
		{tag: "monkey_id", want: false},
		{tag: "foo_keynote", want: false},
		{tag: "keynote", want: false},
		{tag: "turnkey", want: false},
		{tag: "turnkey_value", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			testutil.Equal(t, tt.want, isSecretFieldName(tt.tag))
		})
	}
}

type secretSentinel struct {
	path     string
	sentinel string
	injected string
}

func TestMaskedCopyHasNoUnmaskedSecrets(t *testing.T) {
	cfg := Default()
	populateSecretBearingContainers(cfg)
	records := injectSecretSentinels(cfg)
	records = append(records, injectLogDrainHeaderSentinels(cfg)...)

	// 20 paths MaskedCopy currently masks (including database.url), plus the
	// nine verified Stage 2 leaks and one secret-bearing log drain header.
	testutil.Equal(t, 30, len(records))

	masked := cfg.MaskedCopy()
	tomlOutput, err := masked.ToTOML()
	testutil.NoError(t, err)
	jsonOutput, err := json.Marshal(masked)
	testutil.NoError(t, err)

	testutil.Contains(t, tomlOutput, "h:5432/d")
	testutil.Contains(t, string(jsonOutput), "h:5432/d")
	assertNoSentinelLeaks(t, records, tomlOutput, string(jsonOutput))
	assertOriginalConfigUnmutated(t, cfg, records)
}

func populateSecretBearingContainers(cfg *Config) {
	cfg.Auth.OAuth = map[string]OAuthProvider{"apple": {}}
	cfg.Auth.OIDC = map[string]OIDCProvider{"okta": {}}
	cfg.AI.Providers["openai"] = ProviderConfig{}
	cfg.Auth.SAMLProviders = []SAMLProvider{{}}
	cfg.Logging.Drains = []LogDrainConfig{{Headers: map[string]string{"Authorization": ""}}}
}

func injectSecretSentinels(cfg *Config) []secretSentinel {
	var records []secretSentinel
	walkConfigStrings(reflect.ValueOf(cfg), "", func(path, tag string, field reflect.Value) {
		if !isSecretFieldName(tag) && path != "database.url" && path != "auth.twilio_sid" {
			return
		}
		sentinel := fmt.Sprintf("AYBSENTINEL_%d_%s", len(records)+1, path)
		injected := sentinel
		if path == "database.url" {
			injected = fmt.Sprintf("postgres://u:%s@h:5432/d", sentinel)
		}
		field.SetString(injected)
		records = append(records, secretSentinel{path: path, sentinel: sentinel, injected: injected})
	})
	return records
}

func injectLogDrainHeaderSentinels(cfg *Config) []secretSentinel {
	sentinel := "AYBSENTINEL_30_logging.drains[0].headers.Authorization"
	cfg.Logging.Drains[0].Headers["Authorization"] = sentinel
	return []secretSentinel{{
		path:     "logging.drains[0].headers.Authorization",
		sentinel: sentinel,
		injected: sentinel,
	}}
}

func assertNoSentinelLeaks(t *testing.T, records []secretSentinel, tomlOutput, jsonOutput string) {
	t.Helper()
	for _, record := range records {
		var formats []string
		if strings.Contains(tomlOutput, record.sentinel) {
			formats = append(formats, "TOML")
		}
		if strings.Contains(jsonOutput, record.sentinel) {
			formats = append(formats, "JSON")
		}
		if len(formats) > 0 {
			t.Errorf("secret path %s leaked in %s", record.path, strings.Join(formats, " and "))
		}
	}
}

func assertOriginalConfigUnmutated(t *testing.T, cfg *Config, records []secretSentinel) {
	t.Helper()
	expected := make(map[string]string, len(records))
	for _, record := range records {
		expected[record.path] = record.injected
	}
	seen := make(map[string]string, len(records))
	walkConfigStrings(reflect.ValueOf(cfg), "", func(path, _ string, field reflect.Value) {
		if _, ok := expected[path]; ok {
			seen[path] = field.String()
		}
	})
	testutil.Equal(t, len(records), len(seen))
	for _, record := range records {
		testutil.Equal(t, record.injected, seen[record.path])
	}
}

func walkConfigStrings(value reflect.Value, path string, visit func(string, string, reflect.Value)) {
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			if value.Type().Elem().Kind() != reflect.Struct || !value.CanSet() {
				return
			}
			value.Set(reflect.New(value.Type().Elem()))
		}
		walkConfigStrings(value.Elem(), path, visit)
		return
	}
	switch value.Kind() {
	case reflect.String:
		visit(path, "", value)
	case reflect.Struct:
		walkConfigStructStrings(value, path, visit)
	case reflect.Map:
		walkConfigMapStrings(value, path, visit)
	case reflect.Slice:
		for i := 0; i < value.Len(); i++ {
			walkConfigStrings(value.Index(i), fmt.Sprintf("%s[%d]", path, i), visit)
		}
	}
}

func walkConfigStructStrings(value reflect.Value, path string, visit func(string, string, reflect.Value)) {
	typeOfValue := value.Type()
	for i := 0; i < value.NumField(); i++ {
		structField := typeOfValue.Field(i)
		if structField.PkgPath != "" {
			continue
		}
		tag := strings.Split(structField.Tag.Get("toml"), ",")[0]
		if tag == "-" {
			continue
		}
		if tag == "" {
			tag = strings.ToLower(structField.Name)
		}
		fieldPath := joinConfigPath(path, tag)
		field := value.Field(i)
		if field.Kind() == reflect.String {
			visit(fieldPath, tag, field)
			continue
		}
		walkConfigStrings(field, fieldPath, visit)
	}
}

func walkConfigMapStrings(value reflect.Value, path string, visit func(string, string, reflect.Value)) {
	if value.Type().Key().Kind() != reflect.String {
		return
	}
	keys := value.MapKeys()
	sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
	for _, key := range keys {
		mapValue := value.MapIndex(key)
		addressable := reflect.New(mapValue.Type()).Elem()
		addressable.Set(mapValue)
		walkConfigStrings(addressable, joinConfigPath(path, key.String()), visit)
		value.SetMapIndex(key, addressable)
	}
}

func joinConfigPath(parent, child string) string {
	if parent == "" {
		return child
	}
	return parent + "." + child
}
