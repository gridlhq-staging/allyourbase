//go:build integration

package auth_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/descope/virtualwebauthn"
	"github.com/go-webauthn/webauthn/protocol"
)

func TestWebAuthnMFA_DisabledReturns404(t *testing.T) {
	ctx := t.Context()
	resetAndMigrate(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = testJWTSecret
	cfg.Auth.WebAuthnEnabled = false

	authSvc := newAuthService()
	srv := server.New(cfg, logger, ch, sharedPG.Pool, authSvc, nil)
	token := registerAndGetToken(t, srv, "webauthn-disabled@example.com")

	for _, ep := range []string{
		"/api/auth/mfa/webauthn/enroll",
		"/api/auth/mfa/webauthn/enroll/confirm",
		"/api/auth/mfa/webauthn/challenge",
		"/api/auth/mfa/webauthn/verify",
		"/api/auth/mfa/webauthn/",
		"/api/auth/webauthn/login/begin",
		"/api/auth/webauthn/login/finish",
		"/api/auth/webauthn/login/discover/begin",
		"/api/auth/webauthn/login/discover/finish",
	} {
		method := "POST"
		if ep == "/api/auth/mfa/webauthn/" {
			method = "DELETE"
		}
		w := doJSON(t, srv, method, ep, map[string]any{}, token)
		testutil.StatusCode(t, http.StatusNotFound, w.Code)
	}
}

func TestWebAuthnFirstFactorLoginBegin_Contract(t *testing.T) {
	srv, authSvc, _ := setupMFAServer(t)
	accessToken, userID := registerForMFA(t, srv, "webauthn-begin@example.com")
	registerAndGetToken(t, srv, "webauthn-begin-no-passkey@example.com")
	displayName := "Primary security key"

	rp := expectedRelyingPartyFromConfig(t)
	virtualAuthenticator := virtualwebauthn.NewAuthenticator()
	virtualCredential := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)

	enrollOptions := beginWebAuthnEnroll(t, srv, accessToken)
	attestationResponse := virtualwebauthn.CreateAttestationResponse(rp, virtualAuthenticator, virtualCredential, *enrollOptions)
	confirm := doJSON(t, srv, "POST", "/api/auth/mfa/webauthn/enroll/confirm", map[string]any{
		"display_name":         displayName,
		"attestation_response": mustJSONObject(t, attestationResponse),
	}, accessToken)
	testutil.StatusCode(t, http.StatusOK, confirm.Code)

	factors, err := authSvc.GetUserMFAFactors(t.Context(), userID)
	testutil.NoError(t, err)
	testutil.SliceLen(t, factors, 1)
	testutil.Equal(t, "webauthn", factors[0].Method)

	missingEmail := doJSON(t, srv, "POST", "/api/auth/webauthn/login/begin", map[string]any{}, "")
	testutil.StatusCode(t, http.StatusBadRequest, missingEmail.Code)
	testutil.Contains(t, missingEmail.Body.String(), "email is required")

	unknownChallenge := beginWebAuthnFirstFactorChallenge(t, srv, "unknown-webauthn-begin@example.com")
	unknownChallengeAgain := beginWebAuthnFirstFactorChallenge(t, srv, "unknown-webauthn-begin@example.com")
	noPasskeyChallenge := beginWebAuthnFirstFactorChallenge(t, srv, "webauthn-begin-no-passkey@example.com")
	enrolledChallenge := beginWebAuthnFirstFactorChallenge(t, srv, "  WEBAUTHN-BEGIN@EXAMPLE.COM  ")

	challenges := []*webauthnChallengeContract{unknownChallenge, noPasskeyChallenge, enrolledChallenge}
	for _, challenge := range challenges {
		testutil.Equal(t, rp.ID, challenge.Options.RelyingPartyID)
		testutil.True(t, len(challenge.Options.Challenge) > 0, "first-factor begin must return assertion challenge bytes")
		testutil.True(t, len(challenge.Options.AllowCredentials) > 0, "first-factor begin must always include credential descriptors")
	}
	testutil.Equal(t, 1, len(unknownChallenge.Options.AllowCredentials))
	testutil.Equal(t, 1, len(unknownChallengeAgain.Options.AllowCredentials))
	testutil.Equal(t, 1, len(noPasskeyChallenge.Options.AllowCredentials))
	testutil.Equal(
		t,
		unknownChallenge.Options.AllowCredentials[0],
		unknownChallengeAgain.Options.AllowCredentials[0],
	)
	testutil.True(
		t,
		unknownChallenge.Options.AllowCredentials[0] != noPasskeyChallenge.Options.AllowCredentials[0],
		"decoy begin responses must not share a global credential fingerprint across probed emails",
	)

	virtualAuthenticator.Options.UserHandle = []byte(userID)
	virtualAuthenticator.AddCredential(virtualCredential)

	virtualCredential.Counter = 1
	successAssertion := virtualwebauthn.CreateAssertionResponse(rp, virtualAuthenticator, virtualCredential, *enrolledChallenge.Options)
	finish := doJSON(t, srv, "POST", "/api/auth/webauthn/login/finish", map[string]any{
		"challenge_id":       enrolledChallenge.ChallengeID,
		"assertion_response": mustJSONObject(t, successAssertion),
	}, "")
	testutil.StatusCode(t, http.StatusOK, finish.Code)

	for _, decoyChallenge := range []*webauthnChallengeContract{unknownChallenge, noPasskeyChallenge} {
		decoyFinish := doJSON(t, srv, "POST", "/api/auth/webauthn/login/finish", map[string]any{
			"challenge_id":       decoyChallenge.ChallengeID,
			"assertion_response": mustJSONObject(t, successAssertion),
		}, "")
		testutil.StatusCode(t, http.StatusUnauthorized, decoyFinish.Code)
	}
}

func TestWebAuthnFirstFactorLoginBegin_EnumerationResistance(t *testing.T) {
	srv, _, _ := setupMFAServer(t)
	registerAndGetToken(t, srv, "webauthn-begin-enumeration-no-passkey@example.com")

	unknownEmail := doJSON(t, srv, "POST", "/api/auth/webauthn/login/begin", map[string]any{
		"email": "unknown-webauthn-begin-enumeration@example.com",
	}, "")
	knownNoPasskey := doJSON(t, srv, "POST", "/api/auth/webauthn/login/begin", map[string]any{
		"email": "webauthn-begin-enumeration-no-passkey@example.com",
	}, "")

	testutil.Equal(t, unknownEmail.Code, knownNoPasskey.Code)
	if unknownEmail.Code == http.StatusOK {
		assertWebAuthnBeginSuccessEnvelope(t, unknownEmail)
		assertWebAuthnBeginSuccessEnvelope(t, knownNoPasskey)
		return
	}
	assertSameErrorEnvelopeClass(t, unknownEmail, knownNoPasskey)
}

func assertWebAuthnBeginSuccessEnvelope(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()

	body := mustJSONObjectFromBytes(t, w.Body.Bytes())
	challengeID, _ := body["challenge_id"].(string)
	testutil.True(t, challengeID != "", "first-factor begin success response must include challenge_id")
	_, hasOptions := body["options"]
	testutil.True(t, hasOptions, "first-factor begin success response must include options")
	for _, field := range []string{"code", "message", "doc_url", "data"} {
		_, found := body[field]
		testutil.True(t, !found, "first-factor begin success response must not include %s", field)
	}
}

func assertSameErrorEnvelopeClass(t *testing.T, a, b *httptest.ResponseRecorder) {
	t.Helper()

	var aBody httputil.ErrorResponse
	if err := json.Unmarshal(a.Body.Bytes(), &aBody); err != nil {
		t.Fatalf("parsing first WebAuthn begin error response: %v (body: %s)", err, a.Body.String())
	}
	var bBody httputil.ErrorResponse
	if err := json.Unmarshal(b.Body.Bytes(), &bBody); err != nil {
		t.Fatalf("parsing second WebAuthn begin error response: %v (body: %s)", err, b.Body.String())
	}
	testutil.Equal(t, aBody.Code, bBody.Code)
	testutil.Equal(t, aBody.Message, bBody.Message)
	testutil.Equal(t, aBody.DocURL, bBody.DocURL)
	testutil.True(t, len(aBody.Data) == 0, "first WebAuthn begin error response must not include data")
	testutil.True(t, len(bBody.Data) == 0, "second WebAuthn begin error response must not include data")
}

func TestWebAuthnFirstFactorLoginBegin_NullPasswordHashUser_Contract(t *testing.T) {
	srv, authSvc, _ := setupMFAServer(t)
	accessToken, userID := registerForMFA(t, srv, "webauthn-begin-nullhash-enrolled@example.com")

	rp := expectedRelyingPartyFromConfig(t)
	virtualAuthenticator := virtualwebauthn.NewAuthenticator()
	virtualCredential := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)

	enrollOptions := beginWebAuthnEnroll(t, srv, accessToken)
	attestationResponse := virtualwebauthn.CreateAttestationResponse(rp, virtualAuthenticator, virtualCredential, *enrollOptions)
	confirm := doJSON(t, srv, "POST", "/api/auth/mfa/webauthn/enroll/confirm", map[string]any{
		"display_name":         "Primary security key",
		"attestation_response": mustJSONObject(t, attestationResponse),
	}, accessToken)
	testutil.StatusCode(t, http.StatusOK, confirm.Code)

	anonymousUser, _, _, err := authSvc.CreateAnonymousUser(t.Context())
	testutil.NoError(t, err)
	_, _, _, err = authSvc.LinkOAuth(t.Context(), anonymousUser.ID, "github", &auth.OAuthUserInfo{
		ProviderUserID: "github-null-hash-webauthn-begin",
		Email:          "webauthn-begin-nullhash-nopasskey@example.com",
		Name:           "No Passkey",
	})
	testutil.NoError(t, err)

	unknownChallenge := beginWebAuthnFirstFactorChallenge(t, srv, "unknown-webauthn-nullhash-begin@example.com")
	unknownChallengeAgain := beginWebAuthnFirstFactorChallenge(t, srv, "unknown-webauthn-nullhash-begin@example.com")
	nullHashNoPasskeyChallenge := beginWebAuthnFirstFactorChallenge(t, srv, "  WEBAUTHN-BEGIN-NULLHASH-NOPASSKEY@EXAMPLE.COM  ")
	enrolledChallenge := beginWebAuthnFirstFactorChallenge(t, srv, "  WEBAUTHN-BEGIN-NULLHASH-ENROLLED@EXAMPLE.COM ")

	challenges := []*webauthnChallengeContract{unknownChallenge, nullHashNoPasskeyChallenge, enrolledChallenge}
	for _, challenge := range challenges {
		testutil.Equal(t, rp.ID, challenge.Options.RelyingPartyID)
		testutil.True(t, len(challenge.Options.Challenge) > 0, "first-factor begin must return assertion challenge bytes")
		testutil.True(t, len(challenge.Options.AllowCredentials) > 0, "first-factor begin must always include credential descriptors")
	}
	testutil.Equal(t, 1, len(unknownChallenge.Options.AllowCredentials))
	testutil.Equal(t, 1, len(unknownChallengeAgain.Options.AllowCredentials))
	testutil.Equal(t, 1, len(nullHashNoPasskeyChallenge.Options.AllowCredentials))
	testutil.Equal(
		t,
		unknownChallenge.Options.AllowCredentials[0],
		unknownChallengeAgain.Options.AllowCredentials[0],
	)
	testutil.True(
		t,
		unknownChallenge.Options.AllowCredentials[0] != nullHashNoPasskeyChallenge.Options.AllowCredentials[0],
		"decoy begin responses must stay email-specific even when the account exists without a password hash",
	)

	virtualAuthenticator.Options.UserHandle = []byte(userID)
	virtualAuthenticator.AddCredential(virtualCredential)
	virtualCredential.Counter = 1
	successAssertion := virtualwebauthn.CreateAssertionResponse(rp, virtualAuthenticator, virtualCredential, *enrolledChallenge.Options)
	for _, decoyChallenge := range []*webauthnChallengeContract{unknownChallenge, nullHashNoPasskeyChallenge} {
		decoyFinish := doJSON(t, srv, "POST", "/api/auth/webauthn/login/finish", map[string]any{
			"challenge_id":       decoyChallenge.ChallengeID,
			"assertion_response": mustJSONObject(t, successAssertion),
		}, "")
		testutil.StatusCode(t, http.StatusUnauthorized, decoyFinish.Code)
	}
}

func TestWebAuthnRuntimeToggleContract_UsesAuthHandlerSettingsSeam(t *testing.T) {
	h := auth.NewHandler(newAuthService(), testutil.DiscardLogger())
	h.UpdateAuthSettings(auth.AuthSettings{
		MagicLinkEnabled:     true,
		SMSEnabled:           true,
		EmailMFAEnabled:      true,
		AnonymousAuthEnabled: true,
		TOTPEnabled:          true,
	})

	settings := h.GetAuthSettings()
	payload := mustJSONMap(t, settings)

	_, hasWebAuthnToggle := payload["webauthn_enabled"]
	testutil.True(t, hasWebAuthnToggle, "auth settings must expose webauthn_enabled on UpdateAuthSettings/GetAuthSettings")
}

func TestWebAuthnRPAndAttestationContract(t *testing.T) {
	cases := []struct {
		name          string
		host          string
		port          int
		siteURL       string
		wantPublicURL string
		wantRPID      string
	}{
		{
			name:          "site url path retained but rp id strips path",
			host:          "0.0.0.0",
			port:          8090,
			siteURL:       "https://auth.example.com/tenant/portal/",
			wantPublicURL: "https://auth.example.com/tenant/portal",
			wantRPID:      "auth.example.com",
		},
		{
			name:          "bind all host normalizes to localhost",
			host:          "0.0.0.0",
			port:          8090,
			wantPublicURL: "http://localhost:8090",
			wantRPID:      "localhost",
		},
		{
			name:          "explicit host with port derives hostname-only rp id",
			host:          "127.0.0.1",
			port:          9090,
			wantPublicURL: "http://127.0.0.1:9090",
			wantRPID:      "127.0.0.1",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Default()
			cfg.Server.Host = tc.host
			cfg.Server.Port = tc.port
			cfg.Server.SiteURL = tc.siteURL

			base := cfg.PublicBaseURL()
			testutil.Equal(t, tc.wantPublicURL, base)
			testutil.Equal(t, tc.wantRPID, deriveWebAuthnRPID(t, base))
		})
	}

	testutil.Equal(t, "none", string(protocol.PreferNoAttestation))
}

func TestWebAuthnMFA_AllowsMultipleCredentialsForOneFactor(t *testing.T) {
	srv, authSvc, _ := setupMFAServer(t)
	accessToken, userID := registerForMFA(t, srv, "webauthn-multi-credential@example.com")
	rp := expectedRelyingPartyFromConfig(t)

	primary := enrollVirtualWebAuthnCredential(t, srv, accessToken, rp, "Primary security key",
		[]virtualwebauthn.Transport{virtualwebauthn.TransportUSB})
	aal2Token := webAuthnAAL2TokenForCredential(t, srv, rp, userID, "webauthn-multi-credential@example.com", primary)
	secondary := enrollVirtualWebAuthnCredential(t, srv, aal2Token, rp, "Backup security key",
		[]virtualwebauthn.Transport{virtualwebauthn.TransportInternal})

	factors, err := authSvc.GetUserMFAFactors(t.Context(), userID)
	testutil.NoError(t, err)
	testutil.SliceLen(t, factors, 1)
	testutil.Equal(t, "webauthn", factors[0].Method)

	rows := loadWebAuthnCredentialRows(t, userID)
	testutil.SliceLen(t, rows, 2)
	assertWebAuthnCredentialRow(t, rows, primary.Credential.ID, "Primary security key", []string{"usb"}, 1, true)
	assertWebAuthnCredentialRow(t, rows, secondary.Credential.ID, "Backup security key", []string{"internal"}, 0, false)
}

func TestWebAuthnMFA_DuplicateCredentialReturnsCleanConfirmError(t *testing.T) {
	srv, _, _ := setupMFAServer(t)
	accessToken, userID := registerForMFA(t, srv, "webauthn-duplicate-credential@example.com")
	rp := expectedRelyingPartyFromConfig(t)

	primary := enrollVirtualWebAuthnCredential(t, srv, accessToken, rp, "Primary security key", nil)
	aal2Token := webAuthnAAL2TokenForCredential(t, srv, rp, userID, "webauthn-duplicate-credential@example.com", primary)

	enrollOptions := beginWebAuthnEnroll(t, srv, aal2Token)
	attestationResponse := virtualwebauthn.CreateAttestationResponse(
		rp,
		primary.Authenticator,
		primary.Credential,
		*enrollOptions,
	)
	confirm := doJSON(t, srv, "POST", "/api/auth/mfa/webauthn/enroll/confirm", map[string]any{
		"display_name":         "Duplicate security key",
		"attestation_response": mustJSONObject(t, attestationResponse),
	}, aal2Token)
	testutil.StatusCode(t, http.StatusBadRequest, confirm.Code)
	testutil.Contains(t, confirm.Body.String(), "WebAuthn enrollment verification failed")
}

func TestWebAuthnMFA_EnrollResidentKey_ServedContract(t *testing.T) {
	srv, _, _ := setupMFAServer(t)
	token := registerAndGetToken(t, srv, "webauthn-resident-key@example.com")

	enroll := doJSON(t, srv, "POST", "/api/auth/mfa/webauthn/enroll", nil, token)
	testutil.StatusCode(t, http.StatusOK, enroll.Code)

	var response struct {
		AuthenticatorSelection struct {
			ResidentKey        string `json:"residentKey"`
			RequireResidentKey *bool  `json:"requireResidentKey"`
		} `json:"authenticatorSelection"`
	}
	testutil.NoError(t, json.Unmarshal(enroll.Body.Bytes(), &response))
	testutil.Equal(t, "preferred", response.AuthenticatorSelection.ResidentKey)
	testutil.NotNil(t, response.AuthenticatorSelection.RequireResidentKey)
	testutil.False(t, *response.AuthenticatorSelection.RequireResidentKey)
}

func TestWebAuthnMFA_EnrollConfirmChallengeVerify_Contract(t *testing.T) {
	srv, authSvc, _ := setupMFAServer(t)
	accessToken, userID := registerForMFA(t, srv, "webauthn-e2e@example.com")
	displayName := "Primary security key"

	rp := expectedRelyingPartyFromConfig(t)
	virtualAuthenticator := virtualwebauthn.NewAuthenticator()
	virtualCredential := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)

	enrollBegin := beginWebAuthnEnrollResponse(t, srv, accessToken)
	enrollOptions := enrollBegin.Options
	testutil.Equal(t, rp.ID, enrollOptions.RelyingPartyID)
	assertSDKContractFixture(
		t,
		"webauthn_enroll_begin_response.json",
		sanitizeWebAuthnEnrollBeginFixture(t, enrollBegin.Body),
	)

	attestationResponse := virtualwebauthn.CreateAttestationResponse(rp, virtualAuthenticator, virtualCredential, *enrollOptions)
	confirmRequest := map[string]any{
		"display_name":         displayName,
		"attestation_response": mustJSONObject(t, attestationResponse),
	}
	assertSDKContractFixture(
		t,
		"webauthn_enroll_confirm_request.json",
		sanitizeWebAuthnEnrollConfirmRequestFixture(t, confirmRequest),
	)
	confirm := doJSON(t, srv, "POST", "/api/auth/mfa/webauthn/enroll/confirm", confirmRequest, accessToken)
	testutil.StatusCode(t, http.StatusOK, confirm.Code)
	assertSDKContractFixture(
		t,
		"webauthn_enroll_confirm_response.json",
		mustJSONObjectFromBytes(t, confirm.Body.Bytes()),
	)

	factors, err := authSvc.GetUserMFAFactors(t.Context(), userID)
	testutil.NoError(t, err)
	testutil.SliceLen(t, factors, 1)
	testutil.Equal(t, "webauthn", factors[0].Method)
	testutil.Equal(t, displayName, factors[0].Label)
	testutil.Equal(t, displayName, factors[0].DisplayName)

	virtualAuthenticator.Options.UserHandle = []byte(userID)
	virtualAuthenticator.AddCredential(virtualCredential)

	pendingToken := loginAndGetPendingToken(t, srv, "webauthn-e2e@example.com")
	challenge := beginWebAuthnChallenge(t, srv, pendingToken)
	assertCredentialAllowedForAssertion(t, challenge.Options, virtualCredential.ID)
	assertSDKContractFixture(
		t,
		"webauthn_mfa_challenge_response.json",
		sanitizeWebAuthnMFAChallengeFixture(t, challenge.Body),
	)

	virtualCredential.Counter = 1
	assertionResponse := virtualwebauthn.CreateAssertionResponse(rp, virtualAuthenticator, virtualCredential, *challenge.Options)
	verifyRequest := map[string]any{
		"challenge_id":       challenge.ChallengeID,
		"assertion_response": mustJSONObject(t, assertionResponse),
	}
	assertSDKContractFixture(
		t,
		"webauthn_mfa_verify_request.json",
		sanitizeWebAuthnMFAVerifyRequestFixture(t, verifyRequest),
	)
	verify := doJSON(t, srv, "POST", "/api/auth/mfa/webauthn/verify", verifyRequest, pendingToken)
	testutil.StatusCode(t, http.StatusOK, verify.Code)
	assertSDKContractFixture(
		t,
		"webauthn_mfa_verify_response.json",
		sanitizeWebAuthnMFAVerifyResponseFixture(t, mustJSONObjectFromBytes(t, verify.Body.Bytes())),
	)

	aal2 := parseAuthResp(t, verify)
	assertWebAuthnAAL2Claims(t, authSvc, aal2.Token)

	assertWebAuthnCounterPersisted(t, virtualCredential.ID, 1, true)

	secondary := enrollVirtualWebAuthnCredential(t, srv, aal2.Token, rp, "Backup security key",
		[]virtualwebauthn.Transport{virtualwebauthn.TransportInternal})
	assertWebAuthnCounterPersisted(t, secondary.Credential.ID, 0, false)

	verifyWebAuthnMFACredential(t, srv, webAuthnMFAVerification{
		RP:                   rp,
		UserID:               userID,
		Email:                "webauthn-e2e@example.com",
		Passkey:              secondary,
		Counter:              5,
		AllowedCredentialIDs: [][]byte{virtualCredential.ID, secondary.Credential.ID},
	})
	assertWebAuthnCounterPersisted(t, virtualCredential.ID, 1, true)
	assertWebAuthnCounterPersisted(t, secondary.Credential.ID, 5, true)

	deleteWebAuthnAndAssertNoFactors(t, srv, authSvc, userID, accessToken)
}

func TestWebAuthnMFACredentialManagement_MutationsRequireAAL2(t *testing.T) {
	srv, _, fixture := setupWebAuthnCredentialManagement(t)
	backupID := base64.RawURLEncoding.EncodeToString(fixture.Backup.Credential.ID)

	passwordRename := doJSON(t, srv, "PATCH", "/api/auth/mfa/webauthn/credentials/"+backupID, map[string]any{
		"display_name": "Password-only rename",
	}, fixture.Token)
	testutil.StatusCode(t, http.StatusForbidden, passwordRename.Code)
	assertErrorMessage(t, passwordRename, "MFA verification is required for this action")

	passwordDelete := doJSON(t, srv, "DELETE", "/api/auth/mfa/webauthn/credentials/"+backupID, nil, fixture.Token)
	testutil.StatusCode(t, http.StatusForbidden, passwordDelete.Code)
	assertErrorMessage(t, passwordDelete, "MFA verification is required for this action")

	renamed := doJSON(t, srv, "PATCH", "/api/auth/mfa/webauthn/credentials/"+backupID, map[string]any{
		"display_name": "Renamed backup security key",
	}, fixture.AAL2Token)
	testutil.StatusCode(t, http.StatusOK, renamed.Code)
	renamedPayload := mustJSONObjectFromBytes(t, renamed.Body.Bytes())
	renamedDisplayName, _ := renamedPayload["display_name"].(string)
	testutil.Equal(t, "Renamed backup security key", renamedDisplayName)

	deleteCredential := doJSON(t, srv, "DELETE", "/api/auth/mfa/webauthn/credentials/"+backupID, nil, fixture.AAL2Token)
	testutil.StatusCode(t, http.StatusNoContent, deleteCredential.Code)

	rows := loadWebAuthnCredentialRows(t, fixture.UserID)
	testutil.SliceLen(t, rows, 1)
	testutil.True(t, bytes.Equal(fixture.Primary.Credential.ID, rows[0].CredentialID), "AAL2 delete must keep the unaddressed credential")
}

func TestWebAuthnFirstFactorLoginBeginFinish_Contract(t *testing.T) {
	srv, authSvc, _ := setupMFAServer(t)
	accessToken, userID := registerForMFA(t, srv, "webauthn-first-factor@example.com")
	displayName := "Primary security key"

	rp := expectedRelyingPartyFromConfig(t)
	virtualAuthenticator := virtualwebauthn.NewAuthenticator()
	virtualCredential := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)

	enrollOptions := beginWebAuthnEnroll(t, srv, accessToken)
	attestationResponse := virtualwebauthn.CreateAttestationResponse(rp, virtualAuthenticator, virtualCredential, *enrollOptions)
	confirm := doJSON(t, srv, "POST", "/api/auth/mfa/webauthn/enroll/confirm", map[string]any{
		"display_name":         displayName,
		"attestation_response": mustJSONObject(t, attestationResponse),
	}, accessToken)
	testutil.StatusCode(t, http.StatusOK, confirm.Code)

	virtualAuthenticator.Options.UserHandle = []byte(userID)
	virtualAuthenticator.AddCredential(virtualCredential)

	aal2Token := webAuthnAAL2TokenForCredential(
		t,
		srv,
		rp,
		userID,
		"webauthn-first-factor@example.com",
		virtualWebAuthnCredential{Authenticator: virtualAuthenticator, Credential: virtualCredential},
	)
	secondary := enrollVirtualWebAuthnCredential(t, srv, aal2Token, rp, "Backup security key",
		[]virtualwebauthn.Transport{virtualwebauthn.TransportInternal})
	secondary.Authenticator.Options.UserHandle = []byte(userID)

	challenge := beginWebAuthnFirstFactorChallenge(t, srv, "  WEBAUTHN-FIRST-FACTOR@EXAMPLE.COM ")
	testutil.Equal(t, 2, len(challenge.Options.AllowCredentials))
	assertCredentialAllowedForAssertion(t, challenge.Options, virtualCredential.ID)
	assertCredentialAllowedForAssertion(t, challenge.Options, secondary.Credential.ID)

	secondary.Credential.Counter = 7
	assertionResponse := virtualwebauthn.CreateAssertionResponse(rp, secondary.Authenticator, secondary.Credential, *challenge.Options)
	finish := doJSON(t, srv, "POST", "/api/auth/webauthn/login/finish", map[string]any{
		"challenge_id":       challenge.ChallengeID,
		"assertion_response": mustJSONObject(t, assertionResponse),
	}, "")
	testutil.StatusCode(t, http.StatusOK, finish.Code)

	login := parseAuthResp(t, finish)
	testutil.True(t, login.Token != "", "finish endpoint must return token")
	testutil.True(t, login.RefreshToken != "", "finish endpoint must return refresh token")

	claims, err := authSvc.ValidateToken(login.Token)
	testutil.NoError(t, err)
	testutil.Equal(t, "aal1", claims.AAL)
	testutil.Equal(t, 1, len(claims.AMR))
	testutil.Equal(t, "webauthn", claims.AMR[0])

	me := doJSON(t, srv, "GET", "/api/auth/me", nil, login.Token)
	testutil.StatusCode(t, http.StatusOK, me.Code)
	var mePayload map[string]any
	testutil.NoError(t, json.Unmarshal(me.Body.Bytes(), &mePayload))
	meID, _ := mePayload["id"].(string)
	testutil.Equal(t, userID, meID)

	assertWebAuthnCounterPersisted(t, virtualCredential.ID, 1, true)
	assertWebAuthnCounterPersisted(t, secondary.Credential.ID, 7, true)
}

func TestWebAuthnDiscoverableLoginBeginFinish_Contract(t *testing.T) {
	srv, authSvc, _ := setupMFAServer(t)
	accessToken, userID := registerForMFA(t, srv, "webauthn-discoverable@example.com")
	rp := expectedRelyingPartyFromConfig(t)

	passkey := enrollVirtualWebAuthnCredential(t, srv, accessToken, rp, "Discoverable security key",
		[]virtualwebauthn.Transport{virtualwebauthn.TransportInternal})
	passkey.Authenticator.Options.UserHandle = []byte(userID)

	challenge := beginWebAuthnDiscoverableChallenge(t, srv)
	assertSDKContractFixture(
		t,
		"webauthn_discover_begin_response.json",
		sanitizeWebAuthnDiscoverBeginFixture(t, challenge.Body),
	)
	testutil.Equal(t, rp.ID, challenge.Options.RelyingPartyID)
	testutil.True(t, len(challenge.Options.Challenge) > 0, "discoverable begin must return assertion challenge bytes")
	testutil.Equal(t, 0, len(challenge.Options.AllowCredentials))

	passkey.Credential.Counter = 3
	assertionResponse := virtualwebauthn.CreateAssertionResponse(rp, passkey.Authenticator, passkey.Credential, *challenge.Options)
	finishRequest := map[string]any{
		"challenge_id":       challenge.ChallengeID,
		"assertion_response": mustJSONObject(t, assertionResponse),
	}
	assertSDKContractFixture(
		t,
		"webauthn_discover_finish_request.json",
		sanitizeWebAuthnDiscoverFinishRequestFixture(t, finishRequest),
	)
	finish := doJSON(t, srv, "POST", "/api/auth/webauthn/login/discover/finish", finishRequest, "")
	testutil.StatusCode(t, http.StatusOK, finish.Code)

	login := parseAuthResp(t, finish)
	testutil.True(t, login.Token != "", "discoverable finish endpoint must return token")
	testutil.True(t, login.RefreshToken != "", "discoverable finish endpoint must return refresh token")

	claims, err := authSvc.ValidateToken(login.Token)
	testutil.NoError(t, err)
	testutil.Equal(t, "aal1", claims.AAL)
	testutil.Equal(t, 1, len(claims.AMR))
	testutil.Equal(t, "webauthn", claims.AMR[0])

	me := doJSON(t, srv, "GET", "/api/auth/me", nil, login.Token)
	testutil.StatusCode(t, http.StatusOK, me.Code)
	var mePayload map[string]any
	testutil.NoError(t, json.Unmarshal(me.Body.Bytes(), &mePayload))
	meID, _ := mePayload["id"].(string)
	testutil.Equal(t, userID, meID)

	assertWebAuthnCounterPersisted(t, passkey.Credential.ID, 3, true)

	replayFinish := doJSON(t, srv, "POST", "/api/auth/webauthn/login/discover/finish", map[string]any{
		"challenge_id":       challenge.ChallengeID,
		"assertion_response": mustJSONObject(t, assertionResponse),
	}, "")
	testutil.StatusCode(t, http.StatusConflict, replayFinish.Code)
	assertWebAuthnCounterPersisted(t, passkey.Credential.ID, 3, true)

	unknownChallenge := beginWebAuthnDiscoverableChallenge(t, srv)
	unknownAuthenticator := virtualwebauthn.NewAuthenticatorWithOptions(virtualwebauthn.AuthenticatorOptions{
		UserHandle: []byte(userID),
	})
	unknownCredential := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)
	unknownAuthenticator.AddCredential(unknownCredential)
	unknownCredential.Counter = 1
	unknownAssertion := virtualwebauthn.CreateAssertionResponse(rp, unknownAuthenticator, unknownCredential, *unknownChallenge.Options)
	unknownFinish := doJSON(t, srv, "POST", "/api/auth/webauthn/login/discover/finish", map[string]any{
		"challenge_id":       unknownChallenge.ChallengeID,
		"assertion_response": mustJSONObject(t, unknownAssertion),
	}, "")
	testutil.StatusCode(t, http.StatusUnauthorized, unknownFinish.Code)
	var unknownErr httputil.ErrorResponse
	testutil.NoError(t, json.Unmarshal(unknownFinish.Body.Bytes(), &unknownErr))
	testutil.Equal(t, "WebAuthn assertion failed", unknownErr.Message)
	assertWebAuthnCounterPersisted(t, passkey.Credential.ID, 3, true)
}

func TestWebAuthnDiscoverableLoginFinish_MalformedBodyErrors(t *testing.T) {
	srv, _, _ := setupMFAServer(t)

	cases := []struct {
		name string
		body map[string]any
		want string
	}{
		{
			name: "missing challenge",
			body: map[string]any{},
			want: "challenge_id is required",
		},
		{
			name: "missing assertion",
			body: map[string]any{"challenge_id": "00000000-0000-0000-0000-000000000000"},
			want: "assertion_response is required",
		},
		{
			name: "invalid assertion",
			body: map[string]any{
				"challenge_id":       "00000000-0000-0000-0000-000000000000",
				"assertion_response": map[string]any{"bad": true},
			},
			want: "invalid assertion response",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := doJSON(t, srv, "POST", "/api/auth/webauthn/login/discover/finish", tc.body, "")
			testutil.StatusCode(t, http.StatusBadRequest, w.Code)
			testutil.Contains(t, w.Body.String(), tc.want)
		})
	}
}

func TestWebAuthnDiscoverableLoginBegin_UsesLoginRateLimit(t *testing.T) {
	resetAndMigrate(t, t.Context())

	cfg := config.Default()
	h := auth.NewHandler(newAuthService(), testutil.DiscardLogger())
	h.SetWebAuthnEnabled(true)
	h.SetWebAuthnPublicBaseURL(cfg.PublicBaseURL())
	h.SetLoginRateLimit(2, time.Minute)
	t.Cleanup(h.StopRateLimiters)

	routes := h.Routes()
	loginLimit := 2
	for i := 0; i < loginLimit; i++ {
		w := doAuthHandlerJSON(t, routes, "POST", "/webauthn/login/discover/begin", map[string]any{})
		testutil.StatusCode(t, http.StatusOK, w.Code)
		testutil.True(t, w.Header().Get("X-RateLimit-Limit") != "", "discoverable begin must expose rate-limit headers")
		testutil.Equal(t, "2", w.Header().Get("X-RateLimit-Limit"))
	}

	overLimit := doAuthHandlerJSON(t, routes, "POST", "/webauthn/login/discover/begin", map[string]any{})
	testutil.StatusCode(t, http.StatusTooManyRequests, overLimit.Code)
	testutil.True(t, overLimit.Header().Get("X-RateLimit-Limit") != "", "rate-limited discoverable begin must expose rate-limit headers")
	testutil.Equal(t, "2", overLimit.Header().Get("X-RateLimit-Limit"))
	testutil.Equal(t, "0", overLimit.Header().Get("X-RateLimit-Remaining"))
}

func doAuthHandlerJSON(
	t *testing.T,
	handler http.Handler,
	method string,
	path string,
	body any,
) *httptest.ResponseRecorder {
	t.Helper()

	payload, err := json.Marshal(body)
	testutil.NoError(t, err)
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w
}

func TestWebAuthnFirstFactorLoginFinish_RejectsConsumedChallenge(t *testing.T) {
	srv, _, _ := setupMFAServer(t)
	accessToken, userID := registerForMFA(t, srv, "webauthn-first-factor-consumed@example.com")

	rp := expectedRelyingPartyFromConfig(t)
	virtualAuthenticator := virtualwebauthn.NewAuthenticator()
	virtualCredential := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)

	enrollOptions := beginWebAuthnEnroll(t, srv, accessToken)
	attestationResponse := virtualwebauthn.CreateAttestationResponse(rp, virtualAuthenticator, virtualCredential, *enrollOptions)
	confirm := doJSON(t, srv, "POST", "/api/auth/mfa/webauthn/enroll/confirm", map[string]any{
		"display_name":         "Primary security key",
		"attestation_response": mustJSONObject(t, attestationResponse),
	}, accessToken)
	testutil.StatusCode(t, http.StatusOK, confirm.Code)

	virtualAuthenticator.Options.UserHandle = []byte(userID)
	virtualAuthenticator.AddCredential(virtualCredential)

	challenge := beginWebAuthnFirstFactorChallenge(t, srv, "webauthn-first-factor-consumed@example.com")
	virtualCredential.Counter = 1
	assertionResponse := virtualwebauthn.CreateAssertionResponse(rp, virtualAuthenticator, virtualCredential, *challenge.Options)

	firstFinish := doJSON(t, srv, "POST", "/api/auth/webauthn/login/finish", map[string]any{
		"challenge_id":       challenge.ChallengeID,
		"assertion_response": mustJSONObject(t, assertionResponse),
	}, "")
	testutil.StatusCode(t, http.StatusOK, firstFinish.Code)
	assertWebAuthnCounterPersisted(t, virtualCredential.ID, 1, true)

	replayFinish := doJSON(t, srv, "POST", "/api/auth/webauthn/login/finish", map[string]any{
		"challenge_id":       challenge.ChallengeID,
		"assertion_response": mustJSONObject(t, assertionResponse),
	}, "")
	testutil.StatusCode(t, http.StatusConflict, replayFinish.Code)
	assertWebAuthnCounterPersisted(t, virtualCredential.ID, 1, true)
}

func TestWebAuthnFirstFactorLoginFinish_RejectsExpiredChallenge(t *testing.T) {
	srv, _, _ := setupMFAServer(t)
	accessToken, userID := registerForMFA(t, srv, "webauthn-first-factor-expired@example.com")

	rp := expectedRelyingPartyFromConfig(t)
	virtualAuthenticator := virtualwebauthn.NewAuthenticator()
	virtualCredential := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)

	enrollOptions := beginWebAuthnEnroll(t, srv, accessToken)
	attestationResponse := virtualwebauthn.CreateAttestationResponse(rp, virtualAuthenticator, virtualCredential, *enrollOptions)
	confirm := doJSON(t, srv, "POST", "/api/auth/mfa/webauthn/enroll/confirm", map[string]any{
		"display_name":         "Primary security key",
		"attestation_response": mustJSONObject(t, attestationResponse),
	}, accessToken)
	testutil.StatusCode(t, http.StatusOK, confirm.Code)

	virtualAuthenticator.Options.UserHandle = []byte(userID)
	virtualAuthenticator.AddCredential(virtualCredential)

	challenge := beginWebAuthnFirstFactorChallenge(t, srv, "webauthn-first-factor-expired@example.com")
	_, err := sharedPG.Pool.Exec(t.Context(),
		`UPDATE _ayb_mfa_challenges
		 SET expires_at = NOW() - INTERVAL '1 second'
		 WHERE id = $1 AND challenge_scope = 'webauthn_first_factor'`,
		challenge.ChallengeID,
	)
	testutil.NoError(t, err)

	virtualCredential.Counter = 1
	assertionResponse := virtualwebauthn.CreateAssertionResponse(rp, virtualAuthenticator, virtualCredential, *challenge.Options)

	expiredFinish := doJSON(t, srv, "POST", "/api/auth/webauthn/login/finish", map[string]any{
		"challenge_id":       challenge.ChallengeID,
		"assertion_response": mustJSONObject(t, assertionResponse),
	}, "")
	testutil.StatusCode(t, http.StatusUnauthorized, expiredFinish.Code)
	assertWebAuthnCounterPersisted(t, virtualCredential.ID, 0, false)
}

func TestWebAuthnFirstFactorLoginFinish_RejectsMFAChallenge_Contract(t *testing.T) {
	srv, authSvc, _ := setupMFAServer(t)
	accessToken, userID := registerForMFA(t, srv, "webauthn-mfa-boundary@example.com")
	displayName := "Primary security key"

	rp := expectedRelyingPartyFromConfig(t)
	virtualAuthenticator := virtualwebauthn.NewAuthenticator()
	virtualCredential := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)

	enrollOptions := beginWebAuthnEnroll(t, srv, accessToken)
	attestationResponse := virtualwebauthn.CreateAttestationResponse(rp, virtualAuthenticator, virtualCredential, *enrollOptions)
	confirm := doJSON(t, srv, "POST", "/api/auth/mfa/webauthn/enroll/confirm", map[string]any{
		"display_name":         displayName,
		"attestation_response": mustJSONObject(t, attestationResponse),
	}, accessToken)
	testutil.StatusCode(t, http.StatusOK, confirm.Code)

	virtualAuthenticator.Options.UserHandle = []byte(userID)
	virtualAuthenticator.AddCredential(virtualCredential)

	pendingToken := loginAndGetPendingToken(t, srv, "webauthn-mfa-boundary@example.com")
	mfaChallenge := beginWebAuthnChallenge(t, srv, pendingToken)

	virtualCredential.Counter = 1
	assertionResponse := virtualwebauthn.CreateAssertionResponse(rp, virtualAuthenticator, virtualCredential, *mfaChallenge.Options)

	invalidFinish := doJSON(t, srv, "POST", "/api/auth/webauthn/login/finish", map[string]any{
		"challenge_id":       mfaChallenge.ChallengeID,
		"assertion_response": mustJSONObject(t, assertionResponse),
	}, "")
	testutil.StatusCode(t, http.StatusUnauthorized, invalidFinish.Code)

	verify := doJSON(t, srv, "POST", "/api/auth/mfa/webauthn/verify", map[string]any{
		"challenge_id":       mfaChallenge.ChallengeID,
		"assertion_response": mustJSONObject(t, assertionResponse),
	}, pendingToken)
	testutil.StatusCode(t, http.StatusOK, verify.Code)

	aal2 := parseAuthResp(t, verify)
	claims, err := authSvc.ValidateToken(aal2.Token)
	testutil.NoError(t, err)
	testutil.Equal(t, "aal2", claims.AAL)
	testutil.Equal(t, 2, len(claims.AMR))
	testutil.Equal(t, "password", claims.AMR[0])
	testutil.Equal(t, "webauthn", claims.AMR[1])
}
