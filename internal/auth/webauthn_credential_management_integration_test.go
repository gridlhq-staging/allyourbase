//go:build integration

package auth_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/mailer"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/sms"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/descope/virtualwebauthn"
)

const passkeyNotificationAppName = "Passkey Test App"

type webAuthnManagementFixture struct {
	UserID    string
	Email     string
	Token     string
	AAL2Token string
	RP        virtualwebauthn.RelyingParty
	Primary   virtualWebAuthnCredential
	Backup    virtualWebAuthnCredential
}

func TestWebAuthnMFACredentialManagement_ListRenameDelete(t *testing.T) {
	srv, authSvc, fixture := setupWebAuthnCredentialManagement(t)

	list := doJSON(t, srv, "GET", "/api/auth/mfa/webauthn/credentials", nil, fixture.AAL2Token)
	testutil.StatusCode(t, http.StatusOK, list.Code)
	credentials := parseWebAuthnCredentialList(t, list)
	testutil.Equal(t, 2, len(credentials))

	primaryID := webAuthnCredentialAPIID(fixture.Primary.Credential.ID)
	backupID := webAuthnCredentialAPIID(fixture.Backup.Credential.ID)
	assertWebAuthnCredentialMetadata(t, credentials, primaryID, "Primary security key", []string{"usb"}, true)
	assertWebAuthnCredentialMetadata(t, credentials, backupID, "Backup security key", []string{"internal"}, false)

	rename := doJSON(t, srv, "PATCH", "/api/auth/mfa/webauthn/credentials/"+primaryID, map[string]any{
		"display_name": "  Renamed primary key  ",
	}, fixture.AAL2Token)
	testutil.StatusCode(t, http.StatusOK, rename.Code)

	renamed := parseWebAuthnCredentialList(t,
		doJSON(t, srv, "GET", "/api/auth/mfa/webauthn/credentials", nil, fixture.AAL2Token))
	assertWebAuthnCredentialMetadata(t, renamed, primaryID, "Renamed primary key", []string{"usb"}, true)
	assertWebAuthnCredentialMetadata(t, renamed, backupID, "Backup security key", []string{"internal"}, false)

	factors, err := authSvc.GetUserMFAFactors(t.Context(), fixture.UserID)
	testutil.NoError(t, err)
	testutil.SliceLen(t, factors, 1)
	testutil.Equal(t, "Renamed primary key", factors[0].Label)
	testutil.Equal(t, "Renamed primary key", factors[0].DisplayName)

	deletePrimary := doJSON(t, srv, "DELETE", "/api/auth/mfa/webauthn/credentials/"+primaryID, nil, fixture.AAL2Token)
	testutil.StatusCode(t, http.StatusNoContent, deletePrimary.Code)

	rows := loadWebAuthnCredentialRows(t, fixture.UserID)
	testutil.SliceLen(t, rows, 1)
	testutil.True(t, bytes.Equal(fixture.Backup.Credential.ID, rows[0].CredentialID), "delete must keep the unaddressed credential")

	factors, err = authSvc.GetUserMFAFactors(t.Context(), fixture.UserID)
	testutil.NoError(t, err)
	testutil.SliceLen(t, factors, 1)
	testutil.Equal(t, "webauthn", factors[0].Method)

	verifyWebAuthnMFACredential(t, srv, webAuthnMFAVerification{
		RP:                   fixture.RP,
		UserID:               fixture.UserID,
		Email:                fixture.Email,
		Passkey:              fixture.Backup,
		Counter:              5,
		AllowedCredentialIDs: [][]byte{fixture.Backup.Credential.ID},
	})
}

func TestWebAuthnCredentialManagement_PasskeyChangeNotifications(t *testing.T) {
	srv, _, capture := setupWebAuthnCredentialManagementServerWithMailer(t, &captureEmailMailer{})
	email := "webauthn-credential-notifications@example.com"
	accessToken, userID := registerForMFA(t, srv, email)
	clearCapturedEmails(capture)
	rp := expectedRelyingPartyFromConfig(t)

	primary := enrollVirtualWebAuthnCredential(t, srv, accessToken, rp, "   ", nil)
	assertSinglePasskeyChangeEmail(t, capture, email, "added", "Unnamed passkey")
	clearCapturedEmails(capture)

	aal2Token := webAuthnAAL2TokenForCredential(t, srv, rp, userID, email, primary)
	assertNoCapturedEmails(t, capture)

	backup := enrollVirtualWebAuthnCredential(t, srv, aal2Token, rp, "Backup security key", nil)
	assertSinglePasskeyChangeEmail(t, capture, email, "added", "Backup security key")
	clearCapturedEmails(capture)

	primaryID := webAuthnCredentialAPIID(primary.Credential.ID)
	rename := doJSON(t, srv, "PATCH", "/api/auth/mfa/webauthn/credentials/"+primaryID, map[string]any{
		"display_name": "  Trimmed primary key  ",
	}, aal2Token)
	testutil.StatusCode(t, http.StatusOK, rename.Code)
	assertSinglePasskeyChangeEmail(t, capture, email, "renamed", "Trimmed primary key")
	clearCapturedEmails(capture)

	deletePrimary := doJSON(t, srv, "DELETE", "/api/auth/mfa/webauthn/credentials/"+primaryID, nil, aal2Token)
	testutil.StatusCode(t, http.StatusNoContent, deletePrimary.Code)
	assertSinglePasskeyChangeEmail(t, capture, email, "deleted", "Trimmed primary key")

	rows := loadWebAuthnCredentialRows(t, userID)
	testutil.SliceLen(t, rows, 1)
	testutil.True(t, bytes.Equal(backup.Credential.ID, rows[0].CredentialID), "delete must keep the unaddressed credential")
}

func TestWebAuthnCredentialManagement_LoginUsageDoesNotSendPasskeyChangeEmail(t *testing.T) {
	srv, _, capture := setupWebAuthnCredentialManagementServerWithMailer(t, &captureEmailMailer{})
	email := "webauthn-credential-login-notification@example.com"
	accessToken, userID := registerForMFA(t, srv, email)
	clearCapturedEmails(capture)
	rp := expectedRelyingPartyFromConfig(t)
	passkey := enrollVirtualWebAuthnCredential(t, srv, accessToken, rp, "Login security key", nil)
	clearCapturedEmails(capture)

	verifyWebAuthnMFACredential(t, srv, webAuthnMFAVerification{
		RP:                   rp,
		UserID:               userID,
		Email:                email,
		Passkey:              passkey,
		Counter:              3,
		AllowedCredentialIDs: [][]byte{passkey.Credential.ID},
	})
	assertNoCapturedEmails(t, capture)

	passkey.Authenticator.Options.UserHandle = []byte(userID)
	challenge := beginWebAuthnDiscoverableChallenge(t, srv)
	passkey.Credential.Counter = 4
	assertionResponse := virtualwebauthn.CreateAssertionResponse(rp, passkey.Authenticator, passkey.Credential, *challenge.Options)
	finish := doJSON(t, srv, "POST", "/api/auth/webauthn/login/discover/finish", map[string]any{
		"challenge_id":       challenge.ChallengeID,
		"assertion_response": mustJSONObject(t, assertionResponse),
	}, "")
	testutil.StatusCode(t, http.StatusOK, finish.Code)
	assertNoCapturedEmails(t, capture)
}

func TestWebAuthnCredentialManagement_NotificationFailuresDoNotRollbackMutations(t *testing.T) {
	t.Run("template render failure", func(t *testing.T) {
		srv, authSvc, _ := setupWebAuthnCredentialManagementServerWithMailer(t, &captureEmailMailer{})
		authSvc.SetEmailTemplateService(failingTemplateRenderer{})
		email := "webauthn-credential-template-failure@example.com"
		accessToken, userID := registerForMFA(t, srv, email)
		rp := expectedRelyingPartyFromConfig(t)

		enrollVirtualWebAuthnCredential(t, srv, accessToken, rp, "Render failure key", nil)

		rows := loadWebAuthnCredentialRows(t, userID)
		testutil.SliceLen(t, rows, 1)
		testutil.Equal(t, "Render failure key", rows[0].DisplayName)
	})

	t.Run("mailer send failure", func(t *testing.T) {
		srv, _, _ := setupWebAuthnCredentialManagementServerWithMailer(t, failingEmailMailer{})
		email := "webauthn-credential-mailer-failure@example.com"
		accessToken, userID := registerForMFA(t, srv, email)
		rp := expectedRelyingPartyFromConfig(t)
		primary := enrollVirtualWebAuthnCredential(t, srv, accessToken, rp, "Primary key", nil)
		aal2Token := webAuthnAAL2TokenForCredential(t, srv, rp, userID, email, primary)
		backup := enrollVirtualWebAuthnCredential(t, srv, aal2Token, rp, "Backup key", nil)

		backupID := webAuthnCredentialAPIID(backup.Credential.ID)
		deleteBackup := doJSON(t, srv, "DELETE", "/api/auth/mfa/webauthn/credentials/"+backupID, nil, aal2Token)
		testutil.StatusCode(t, http.StatusNoContent, deleteBackup.Code)

		rows := loadWebAuthnCredentialRows(t, userID)
		testutil.SliceLen(t, rows, 1)
		testutil.True(t, bytes.Equal(primary.Credential.ID, rows[0].CredentialID), "send failure must not roll back delete")
	})
}

func TestWebAuthnMFACredentialManagement_Failures(t *testing.T) {
	srv, _, fixture := setupWebAuthnCredentialManagement(t)
	primaryID := webAuthnCredentialAPIID(fixture.Primary.Credential.ID)
	unknownID := webAuthnCredentialAPIID([]byte("unknown credential id"))

	cases := []struct {
		name    string
		method  string
		id      string
		token   string
		body    any
		status  int
		message string
	}{
		{"missing auth", "GET", "", "", nil, http.StatusUnauthorized, "missing or invalid authorization header"},
		{"malformed id", "PATCH", "not*base64url", fixture.AAL2Token, map[string]any{"display_name": "Bad"}, http.StatusBadRequest, "invalid credential_id"},
		{"padded id", "DELETE", primaryID + "=", fixture.AAL2Token, nil, http.StatusBadRequest, "invalid credential_id"},
		{"unknown id", "DELETE", unknownID, fixture.AAL2Token, nil, http.StatusNotFound, "WebAuthn credential not found"},
		{"empty rename", "PATCH", primaryID, fixture.AAL2Token, map[string]any{"display_name": "   "}, http.StatusBadRequest, "display_name is required"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := "/api/auth/mfa/webauthn/credentials"
			if tc.id != "" {
				path += "/" + tc.id
			}
			w := doJSON(t, srv, tc.method, path, tc.body, tc.token)
			testutil.StatusCode(t, tc.status, w.Code)
			assertErrorMessage(t, w, tc.message)
		})
	}

	otherToken, _ := registerForMFA(t, srv, "webauthn-credential-other@example.com")
	otherDelete := doJSON(t, srv, "DELETE", "/api/auth/mfa/webauthn/credentials/"+primaryID, nil, otherToken)
	testutil.StatusCode(t, http.StatusNotFound, otherDelete.Code)
	assertErrorMessage(t, otherDelete, "WebAuthn credential not found")
}

func TestWebAuthnMFACredentialManagement_LastPasskeyGuardPreservesLegacyDelete(t *testing.T) {
	srv, authSvc, _ := setupMFAServer(t)
	email := "webauthn-credential-last-passkey@example.com"
	accessToken, userID := registerForMFA(t, srv, email)
	rp := expectedRelyingPartyFromConfig(t)
	primary := enrollVirtualWebAuthnCredential(t, srv, accessToken, rp, "Primary security key", nil)
	aal2Token := webAuthnAAL2TokenForCredential(t, srv, rp, userID, email, primary)

	credentialID := webAuthnCredentialAPIID(primary.Credential.ID)
	guarded := doJSON(t, srv, "DELETE", "/api/auth/mfa/webauthn/credentials/"+credentialID, nil, aal2Token)
	testutil.StatusCode(t, http.StatusForbidden, guarded.Code)
	assertErrorMessage(t, guarded, "cannot delete final WebAuthn credential")
	testutil.SliceLen(t, loadWebAuthnCredentialRows(t, userID), 1)

	deleteAll := doJSON(t, srv, "DELETE", "/api/auth/mfa/webauthn/", nil, accessToken)
	testutil.StatusCode(t, http.StatusNoContent, deleteAll.Code)
	factors, err := authSvc.GetUserMFAFactors(t.Context(), userID)
	testutil.NoError(t, err)
	testutil.SliceLen(t, factors, 0)
}

func setupWebAuthnCredentialManagement(t *testing.T) (*server.Server, *auth.Service, webAuthnManagementFixture) {
	t.Helper()

	srv, authSvc := setupWebAuthnCredentialManagementServer(t)
	email := "webauthn-credential-management@example.com"
	accessToken, userID := registerForMFA(t, srv, email)
	rp := expectedRelyingPartyFromConfig(t)
	primary := enrollVirtualWebAuthnCredential(t, srv, accessToken, rp, "Primary security key",
		[]virtualwebauthn.Transport{virtualwebauthn.TransportUSB})
	aal2Token := webAuthnAAL2TokenForCredential(t, srv, rp, userID, email, primary)
	backup := enrollVirtualWebAuthnCredential(t, srv, aal2Token, rp, "Backup security key",
		[]virtualwebauthn.Transport{virtualwebauthn.TransportInternal})

	return srv, authSvc, webAuthnManagementFixture{
		UserID:    userID,
		Email:     email,
		Token:     accessToken,
		AAL2Token: aal2Token,
		RP:        rp,
		Primary:   primary,
		Backup:    backup,
	}
}

func setupWebAuthnCredentialManagementServer(t *testing.T) (*server.Server, *auth.Service) {
	srv, authSvc, _ := setupWebAuthnCredentialManagementServerWithMailer(t, nil)
	return srv, authSvc
}

func setupWebAuthnCredentialManagementServerWithMailer(
	t *testing.T,
	emailMailer mailer.Mailer,
) (*server.Server, *auth.Service, *captureEmailMailer) {
	t.Helper()
	ctx := t.Context()
	resetAndMigrate(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = testJWTSecret
	cfg.Auth.SMSEnabled = true
	cfg.Auth.WebAuthnEnabled = true
	cfg.Auth.RateLimit = 100
	cfg.Auth.RateLimitAuth = "100/min"

	authSvc := newAuthService()
	authSvc.SetSMSProvider(&sms.CaptureProvider{})
	authSvc.SetSMSConfig(sms.Config{
		CodeLength:       6,
		Expiry:           5 * time.Minute,
		MaxAttempts:      3,
		DailyLimit:       0,
		AllowedCountries: []string{"US", "CA"},
	})
	capture, _ := emailMailer.(*captureEmailMailer)
	if emailMailer != nil {
		authSvc.SetMailer(emailMailer, passkeyNotificationAppName, "http://localhost:8090/api")
	}

	return server.New(cfg, logger, ch, sharedPG.Pool, authSvc, nil), authSvc, capture
}

func parseWebAuthnCredentialList(t *testing.T, w *httptest.ResponseRecorder) []map[string]any {
	t.Helper()

	var payload struct {
		Credentials []map[string]any `json:"credentials"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	return payload.Credentials
}

func assertWebAuthnCredentialMetadata(
	t *testing.T,
	credentials []map[string]any,
	credentialID string,
	displayName string,
	transports []string,
	wantLastUsed bool,
) {
	t.Helper()

	for _, credential := range credentials {
		if credential["credential_id"] != credentialID {
			continue
		}
		gotDisplayName, ok := credential["display_name"].(string)
		testutil.True(t, ok, "credential metadata must include string display_name")
		testutil.Equal(t, displayName, gotDisplayName)
		assertInterfaceStringSet(t, transports, credential["transports"])
		testutil.True(t, credential["created_at"] != "", "credential metadata must include created_at")
		_, hasLastUsed := credential["last_used_at"]
		testutil.Equal(t, wantLastUsed, hasLastUsed)
		for _, forbidden := range []string{"id", "factor_id", "public_key", "sign_count"} {
			_, found := credential[forbidden]
			testutil.True(t, !found, "credential metadata must not expose %s", forbidden)
		}
		return
	}
	t.Fatalf("credential %q not found in list response", credentialID)
}

func assertInterfaceStringSet(t *testing.T, want []string, raw any) {
	t.Helper()

	values, ok := raw.([]any)
	testutil.True(t, ok, "transports must be a JSON array")
	got := make([]string, 0, len(values))
	for _, value := range values {
		text, ok := value.(string)
		testutil.True(t, ok, "transport entries must be strings")
		got = append(got, text)
	}
	assertStringSet(t, want, got)
}

func assertErrorMessage(t *testing.T, w *httptest.ResponseRecorder, want string) {
	t.Helper()

	var errResp httputil.ErrorResponse
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	testutil.Equal(t, want, errResp.Message)
}

type failingTemplateRenderer struct{}

func (failingTemplateRenderer) RenderWithFallback(context.Context, string, map[string]string) (string, string, string, error) {
	return "", "", "", errors.New("template render failed")
}

type failingEmailMailer struct{}

func (failingEmailMailer) Send(context.Context, *mailer.Message) error {
	return errors.New("mailer send failed")
}

func clearCapturedEmails(capture *captureEmailMailer) {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	capture.calls = nil
}

func capturedEmails(capture *captureEmailMailer) []mailer.Message {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return append([]mailer.Message(nil), capture.calls...)
}

func assertNoCapturedEmails(t *testing.T, capture *captureEmailMailer) {
	t.Helper()
	testutil.Equal(t, 0, len(capturedEmails(capture)))
}

func assertSinglePasskeyChangeEmail(t *testing.T, capture *captureEmailMailer, to, action, credentialName string) {
	t.Helper()
	calls := capturedEmails(capture)
	testutil.Equal(t, 1, len(calls))
	msg := calls[0]
	testutil.Equal(t, to, msg.To)
	assertMessageContains(t, msg.Subject, action, "subject")
	assertMessageContains(t, msg.Subject, credentialName, "subject")
	body := msg.HTML + "\n" + msg.Text
	assertMessageContains(t, body, action, "body")
	assertMessageContains(t, body, credentialName, "body")
	assertMessageContains(t, body, passkeyNotificationAppName, "body")
}

func assertMessageContains(t *testing.T, value, want, field string) {
	t.Helper()
	testutil.True(t, strings.Contains(strings.ToLower(value), strings.ToLower(want)),
		"%s should contain %q, got %q", field, want, value)
}

func webAuthnCredentialAPIID(raw []byte) string {
	return base64.RawURLEncoding.EncodeToString(raw)
}
