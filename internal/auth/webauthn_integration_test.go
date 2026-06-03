//go:build integration

package auth_test

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/descope/virtualwebauthn"
	"github.com/go-webauthn/webauthn/protocol"
)

type webauthnChallengeContract struct {
	ChallengeID string
	Options     *virtualwebauthn.AssertionOptions
}

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

func TestWebAuthnMFA_EnrollConfirmChallengeVerify_Contract(t *testing.T) {
	srv, authSvc, _ := setupMFAServer(t)
	accessToken, userID := registerForMFA(t, srv, "webauthn-e2e@example.com")
	displayName := "Primary security key"

	rp := expectedRelyingPartyFromConfig(t)
	virtualAuthenticator := virtualwebauthn.NewAuthenticator()
	virtualCredential := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)

	enrollOptions := beginWebAuthnEnroll(t, srv, accessToken)
	testutil.Equal(t, rp.ID, enrollOptions.RelyingPartyID)

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
	testutil.Equal(t, displayName, factors[0].Label)
	testutil.Equal(t, displayName, factors[0].DisplayName)

	virtualAuthenticator.Options.UserHandle = []byte(userID)
	virtualAuthenticator.AddCredential(virtualCredential)

	pendingToken := loginAndGetPendingToken(t, srv, "webauthn-e2e@example.com")
	challenge := beginWebAuthnChallenge(t, srv, pendingToken)

	virtualCredential.Counter = 1
	assertionResponse := virtualwebauthn.CreateAssertionResponse(rp, virtualAuthenticator, virtualCredential, *challenge.Options)
	verify := doJSON(t, srv, "POST", "/api/auth/mfa/webauthn/verify", map[string]any{
		"challenge_id":       challenge.ChallengeID,
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

	assertWebAuthnCounterPersisted(t, userID, 1)

	pendingToken2 := loginAndGetPendingToken(t, srv, "webauthn-e2e@example.com")
	challenge2 := beginWebAuthnChallenge(t, srv, pendingToken2)
	_ = challenge2

	replay := doJSON(t, srv, "POST", "/api/auth/mfa/webauthn/verify", map[string]any{
		"challenge_id":       challenge2.ChallengeID,
		"assertion_response": mustJSONObject(t, assertionResponse),
	}, pendingToken2)
	testutil.StatusCode(t, http.StatusUnauthorized, replay.Code)
	assertWebAuthnCounterPersisted(t, userID, 1)

	deleteResp := doJSON(t, srv, "DELETE", "/api/auth/mfa/webauthn/", nil, accessToken)
	testutil.StatusCode(t, http.StatusNoContent, deleteResp.Code)

	factors, err = authSvc.GetUserMFAFactors(t.Context(), userID)
	testutil.NoError(t, err)
	testutil.SliceLen(t, factors, 0)
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

	challenge := beginWebAuthnFirstFactorChallenge(t, srv, "  WEBAUTHN-FIRST-FACTOR@EXAMPLE.COM ")

	virtualCredential.Counter = 1
	assertionResponse := virtualwebauthn.CreateAssertionResponse(rp, virtualAuthenticator, virtualCredential, *challenge.Options)
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
	assertWebAuthnCounterPersisted(t, userID, 1)

	replayFinish := doJSON(t, srv, "POST", "/api/auth/webauthn/login/finish", map[string]any{
		"challenge_id":       challenge.ChallengeID,
		"assertion_response": mustJSONObject(t, assertionResponse),
	}, "")
	testutil.StatusCode(t, http.StatusConflict, replayFinish.Code)
	assertWebAuthnCounterPersisted(t, userID, 1)
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
	assertWebAuthnCounterPersisted(t, userID, 0)
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

func beginWebAuthnEnroll(t *testing.T, srv *server.Server, token string) *virtualwebauthn.AttestationOptions {
	t.Helper()

	w := doJSON(t, srv, "POST", "/api/auth/mfa/webauthn/enroll", nil, token)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := mustJSONObjectFromBytes(t, w.Body.Bytes())
	attestation := findNestedString(body, "attestation")
	testutil.Equal(t, "none", attestation)

	optionsJSON := extractOptionsJSON(t, body, w.Body.String())
	options, err := virtualwebauthn.ParseAttestationOptions(optionsJSON)
	testutil.NoError(t, err)
	return options
}

func beginWebAuthnChallenge(t *testing.T, srv *server.Server, pendingToken string) *webauthnChallengeContract {
	t.Helper()

	w := doJSON(t, srv, "POST", "/api/auth/mfa/webauthn/challenge", nil, pendingToken)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := mustJSONObjectFromBytes(t, w.Body.Bytes())
	challengeID, _ := body["challenge_id"].(string)
	testutil.True(t, challengeID != "", "challenge endpoint must return challenge_id")

	optionsJSON := extractOptionsJSON(t, body, w.Body.String())
	options, err := virtualwebauthn.ParseAssertionOptions(optionsJSON)
	testutil.NoError(t, err)

	return &webauthnChallengeContract{ChallengeID: challengeID, Options: options}
}

func beginWebAuthnFirstFactorChallenge(t *testing.T, srv *server.Server, email string) *webauthnChallengeContract {
	t.Helper()

	w := doJSON(t, srv, "POST", "/api/auth/webauthn/login/begin", map[string]any{
		"email": email,
	}, "")
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := mustJSONObjectFromBytes(t, w.Body.Bytes())
	challengeID, _ := body["challenge_id"].(string)
	testutil.True(t, challengeID != "", "first-factor begin endpoint must return challenge_id")

	optionsJSON := extractOptionsJSON(t, body, w.Body.String())
	options, err := virtualwebauthn.ParseAssertionOptions(optionsJSON)
	testutil.NoError(t, err)

	return &webauthnChallengeContract{ChallengeID: challengeID, Options: options}
}

func expectedRelyingPartyFromConfig(t *testing.T) virtualwebauthn.RelyingParty {
	t.Helper()

	cfg := config.Default()
	origin := cfg.PublicBaseURL()

	return virtualwebauthn.RelyingParty{
		ID:     deriveWebAuthnRPID(t, origin),
		Name:   "Allyourbase",
		Origin: origin,
	}
}

func deriveWebAuthnRPID(t *testing.T, publicBaseURL string) string {
	t.Helper()

	u, err := url.Parse(publicBaseURL)
	testutil.NoError(t, err)
	host := strings.TrimSpace(u.Hostname())
	testutil.True(t, host != "", "public base URL must include a hostname")

	if strings.EqualFold(host, "localhost") {
		return "localhost"
	}
	if ip := net.ParseIP(host); ip != nil {
		return host
	}

	return strings.ToLower(host)
}

func extractOptionsJSON(t *testing.T, payload map[string]any, fallback string) string {
	t.Helper()

	if raw, ok := payload["options"]; ok {
		switch v := raw.(type) {
		case string:
			return v
		default:
			encoded, err := json.Marshal(v)
			testutil.NoError(t, err)
			return string(encoded)
		}
	}

	return fallback
}

func findNestedString(payload map[string]any, key string) string {
	if value, ok := payload[key].(string); ok {
		return value
	}
	for _, value := range payload {
		if nested, ok := value.(map[string]any); ok {
			if found := findNestedString(nested, key); found != "" {
				return found
			}
		}
	}
	return ""
}

func mustJSONObjectFromBytes(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var payload map[string]any
	testutil.NoError(t, json.Unmarshal(body, &payload))
	return payload
}

func mustJSONObject(t *testing.T, raw string) map[string]any {
	t.Helper()
	var payload map[string]any
	testutil.NoError(t, json.Unmarshal([]byte(raw), &payload))
	return payload
}

func mustJSONMap(t *testing.T, v any) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(v)
	testutil.NoError(t, err)
	var payload map[string]any
	testutil.NoError(t, json.Unmarshal(encoded, &payload))
	return payload
}

func assertWebAuthnCounterPersisted(t *testing.T, userID string, want int64) {
	t.Helper()

	var counter int64
	err := sharedPG.Pool.QueryRow(t.Context(), `
		SELECT webauthn_sign_count
		FROM _ayb_user_mfa
		WHERE user_id = $1 AND method = 'webauthn' AND enabled = true
	`, userID).Scan(&counter)
	testutil.NoError(t, err)
	testutil.Equal(t, want, counter)
}
