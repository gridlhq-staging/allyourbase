package server_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/testutil"
)

// authSettingsAdminToken creates a test server with admin password and returns it with a valid admin token.
func authSettingsAdminToken(t *testing.T) (*server.Server, string) {
	t.Helper()
	srv := newTestServerWithPassword(t, "admin-pass")
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/auth",
		strings.NewReader(`{"password":"admin-pass"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
	var body map[string]string
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return srv, body["token"]
}

// authSettingsServerWithAuth creates a test server with auth enabled and returns it with a valid admin token.
func authSettingsServerWithAuth(t *testing.T) (*server.Server, string) {
	t.Helper()
	cfg := config.Default()
	cfg.Admin.Password = "admin-pass"
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = "test-secret-that-is-at-least-32-chars-long"
	cfg.Auth.TOTPEnabled = false
	cfg.Auth.AnonymousAuthEnabled = false
	cfg.Auth.EmailMFAEnabled = false

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ch := schema.NewCacheHolder(nil, logger)
	authSvc := auth.NewService(nil, cfg.Auth.JWTSecret, 15*time.Minute, 7*24*time.Hour, 8, logger)
	srv := server.New(cfg, logger, ch, nil, authSvc, nil)

	// Get admin token.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/auth",
		strings.NewReader(`{"password":"admin-pass"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
	var loginResp map[string]string
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &loginResp))
	return srv, loginResp["token"]
}

func TestAuthSettingsGet_NoAuth(t *testing.T) {
	t.Parallel()
	// Server without auth service — should return 404.
	srv, token := authSettingsAdminToken(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/auth-settings", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestAuthSettingsGet_RequiresAdminToken(t *testing.T) {
	t.Parallel()
	srv, _ := authSettingsAdminToken(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/auth-settings", nil)
	// No auth header.
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuthSettingsGet_ReturnsDefaults(t *testing.T) {
	t.Parallel()
	// Server WITH auth enabled — WebAuthn follows the startup default while
	// the other optional auth toggles remain off until explicitly enabled.
	srv, token := authSettingsServerWithAuth(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/auth-settings", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	var settings auth.AuthSettings
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &settings))
	testutil.False(t, settings.MagicLinkEnabled)
	testutil.False(t, settings.SMSEnabled)
	testutil.False(t, settings.EmailMFAEnabled)
	testutil.False(t, settings.AnonymousAuthEnabled)
	testutil.False(t, settings.TOTPEnabled)
	testutil.True(t, settings.WebAuthnEnabled)
}

func TestAuthSettingsUpdate_RequiresAdminToken(t *testing.T) {
	t.Parallel()
	srv, _ := authSettingsAdminToken(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/auth-settings",
		strings.NewReader(`{"totp_enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuthSettingsGetAndUpdate_WithAuthService(t *testing.T) {
	t.Parallel()
	srv, token := authSettingsServerWithAuth(t)

	// GET auth settings — all should be false by default.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/auth-settings", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	var settings auth.AuthSettings
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &settings))
	testutil.False(t, settings.TOTPEnabled)
	testutil.False(t, settings.AnonymousAuthEnabled)
	testutil.False(t, settings.EmailMFAEnabled)
	testutil.True(t, settings.WebAuthnEnabled)

	// PUT to enable TOTP and anonymous auth while preserving the startup
	// WebAuthn default; omitted fields are treated as false by this endpoint.
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/admin/auth-settings",
		strings.NewReader(`{"totp_enabled":true,"anonymous_auth_enabled":true,"email_mfa_enabled":false,"sms_enabled":false,"magic_link_enabled":false,"webauthn_enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	var updated auth.AuthSettings
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &updated))
	testutil.True(t, updated.TOTPEnabled)
	testutil.True(t, updated.AnonymousAuthEnabled)
	testutil.False(t, updated.EmailMFAEnabled)
	testutil.True(t, updated.WebAuthnEnabled)

	// GET again to confirm persistence.
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/admin/auth-settings", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	var confirmed auth.AuthSettings
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &confirmed))
	testutil.True(t, confirmed.TOTPEnabled)
	testutil.True(t, confirmed.AnonymousAuthEnabled)
	testutil.True(t, confirmed.WebAuthnEnabled)
}

// TestAuthSettingsGet_ReturnsDefaults_IncludesWebAuthn (Stage 3 red test): GET
// /api/admin/auth-settings should include the webauthn_enabled toggle in the
// AuthSettings response payload and default it to true. This will fail to
// compile until the WebAuthnEnabled field is added to auth.AuthSettings in
// Stage 5.
func TestAuthSettingsGet_ReturnsDefaults_IncludesWebAuthn(t *testing.T) {
	t.Parallel()
	srv, token := authSettingsServerWithAuth(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/auth-settings", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	var settings auth.AuthSettings
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &settings))
	// WebAuthn must default to true; field must exist on the struct so the
	// GET response includes it (locks the public JSON contract).
	testutil.True(t, settings.WebAuthnEnabled)
}

// TestAuthSettingsUpdate_WebAuthnRoundTrip (Stage 3 red test): PUT a full
// AuthSettings body with webauthn_enabled=true; the PUT response and a
// follow-up GET must both report WebAuthnEnabled=true. This proves the
// full-struct replacement semantics propagate the new field through
// UpdateAuthSettings and GetAuthSettings. Fails to compile until
// WebAuthnEnabled exists on auth.AuthSettings.
func TestAuthSettingsUpdate_WebAuthnRoundTrip(t *testing.T) {
	t.Parallel()
	srv, token := authSettingsServerWithAuth(t)

	// PUT enables webauthn while explicitly setting every other toggle.
	w := httptest.NewRecorder()
	body := `{"totp_enabled":false,"anonymous_auth_enabled":false,"email_mfa_enabled":false,"sms_enabled":false,"magic_link_enabled":false,"webauthn_enabled":true}`
	req := httptest.NewRequest(http.MethodPut, "/api/admin/auth-settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	var updated auth.AuthSettings
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &updated))
	testutil.True(t, updated.WebAuthnEnabled)

	// GET again to confirm persistence across handler calls.
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/admin/auth-settings", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router().ServeHTTP(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)

	var confirmed auth.AuthSettings
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &confirmed))
	testutil.True(t, confirmed.WebAuthnEnabled)
}
