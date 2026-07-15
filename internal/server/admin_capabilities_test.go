package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestAdminCapabilitiesUnauthorized(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Admin.Password = "secret123"
	srv := newTestServerWithConfig(t, cfg)

	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/admin/capabilities", nil))
	testutil.Equal(t, http.StatusUnauthorized, w.Code)

	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/capabilities", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAdminCapabilitiesConfiguredToken(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Admin.Password = "secret123"
	cfg.Auth.Enabled = true
	cfg.Auth.MagicLinkEnabled = true
	cfg.Auth.EmailMFAEnabled = true
	cfg.Auth.AnonymousAuthEnabled = true
	cfg.Auth.TOTPEnabled = true
	cfg.Auth.WebAuthnEnabled = true
	cfg.Auth.SMSEnabled = true
	cfg.Auth.OAuthProviderMode.Enabled = true
	cfg.Storage.Enabled = true
	cfg.Jobs.Enabled = true
	cfg.Status.Enabled = true
	cfg.Push.Enabled = true
	cfg.Support.Enabled = true
	cfg.Billing.Provider = "stripe"
	srv := newTestServerWithConfig(t, cfg)

	body := getAdminCapabilities(t, srv, loginAdmin(t, srv, "secret123"))

	expected := map[string]bool{
		"auth":                true,
		"auth_anonymous":      true,
		"auth_email_mfa":      true,
		"auth_magic_link":     true,
		"auth_oauth_provider": true,
		"auth_sms":            true,
		"auth_totp":           true,
		"auth_webauthn":       true,
		"billing":             true,
		"edge_functions":      false,
		"jobs":                true,
		"push":                true,
		"status":              true,
		"storage":             true,
		"support":             true,
	}
	assertCapabilities(t, expected, body)
}

func TestAdminCapabilitiesDisabledSubsystem(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Admin.Password = "secret123"
	cfg.Auth.Enabled = true
	cfg.Auth.MagicLinkEnabled = true
	cfg.Auth.SMSEnabled = false
	cfg.Storage.Enabled = false
	cfg.Jobs.Enabled = true
	cfg.Support.Enabled = true
	srv := newTestServerWithConfig(t, cfg)

	body := getAdminCapabilities(t, srv, loginAdmin(t, srv, "secret123"))

	expected := map[string]bool{
		"auth":                true,
		"auth_anonymous":      false,
		"auth_email_mfa":      false,
		"auth_magic_link":     true,
		"auth_oauth_provider": false,
		"auth_sms":            false,
		"auth_totp":           false,
		"auth_webauthn":       true,
		"billing":             false,
		"edge_functions":      false,
		"jobs":                true,
		"push":                false,
		"status":              false,
		"storage":             false,
		"support":             true,
	}
	assertCapabilities(t, expected, body)
}

func TestAdminCapabilitiesEdgeFunctions(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Admin.Password = "secret123"
	srv := newTestServerWithConfig(t, cfg)

	body := getAdminCapabilities(t, srv, loginAdmin(t, srv, "secret123"))
	testutil.False(t, body["edge_functions"])
}

func TestAdminCapabilitiesDefaultPasswordless(t *testing.T) {
	t.Parallel()

	srv := newTestServerWithConfig(t, config.Default())

	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/admin/capabilities", nil))

	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAdminCapabilitiesDoNotLeakIntoAnonymousStatus(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Admin.Password = "secret123"
	cfg.Auth.Enabled = true
	cfg.Storage.Enabled = true
	cfg.Billing.Provider = "stripe"
	srv := newTestServerWithConfig(t, cfg)

	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/admin/status", nil))

	testutil.Equal(t, http.StatusOK, w.Code)
	var body map[string]bool
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assertCapabilities(t, map[string]bool{"auth": true}, body)
}

func getAdminCapabilities(t *testing.T, srv *server.Server, token string) map[string]bool {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/admin/capabilities", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	var body map[string]bool
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body
}

func assertCapabilities(t *testing.T, expected, actual map[string]bool) {
	t.Helper()
	if !reflect.DeepEqual(expected, actual) {
		t.Fatalf("capabilities mismatch\nexpected: %#v\nactual:   %#v", expected, actual)
	}
}
