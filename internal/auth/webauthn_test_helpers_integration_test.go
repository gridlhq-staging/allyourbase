//go:build integration

package auth_test

import (
	"encoding/json"
	"net"
	"net/url"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/descope/virtualwebauthn"
)

type webauthnChallengeContract struct {
	ChallengeID string
	Options     *virtualwebauthn.AssertionOptions
	Body        map[string]any
}

type webauthnEnrollBeginContract struct {
	Options *virtualwebauthn.AttestationOptions
	Body    map[string]any
}

func beginWebAuthnEnroll(t *testing.T, srv *server.Server, token string) *virtualwebauthn.AttestationOptions {
	t.Helper()
	return beginWebAuthnEnrollResponse(t, srv, token).Options
}

func beginWebAuthnEnrollResponse(t *testing.T, srv *server.Server, token string) *webauthnEnrollBeginContract {
	t.Helper()

	w := doJSON(t, srv, "POST", "/api/auth/mfa/webauthn/enroll", nil, token)
	testutil.StatusCode(t, 200, w.Code)

	body := mustJSONObjectFromBytes(t, w.Body.Bytes())
	attestation := findNestedString(body, "attestation")
	testutil.Equal(t, "none", attestation)

	optionsJSON := extractOptionsJSON(t, body, w.Body.String())
	options, err := virtualwebauthn.ParseAttestationOptions(optionsJSON)
	testutil.NoError(t, err)
	return &webauthnEnrollBeginContract{Options: options, Body: body}
}

func beginWebAuthnChallenge(t *testing.T, srv *server.Server, pendingToken string) *webauthnChallengeContract {
	t.Helper()

	w := doJSON(t, srv, "POST", "/api/auth/mfa/webauthn/challenge", nil, pendingToken)
	testutil.StatusCode(t, 200, w.Code)

	body := mustJSONObjectFromBytes(t, w.Body.Bytes())
	challengeID, _ := body["challenge_id"].(string)
	testutil.True(t, challengeID != "", "challenge endpoint must return challenge_id")

	optionsJSON := extractOptionsJSON(t, body, w.Body.String())
	options, err := virtualwebauthn.ParseAssertionOptions(optionsJSON)
	testutil.NoError(t, err)

	return &webauthnChallengeContract{ChallengeID: challengeID, Options: options, Body: body}
}

func beginWebAuthnFirstFactorChallenge(t *testing.T, srv *server.Server, email string) *webauthnChallengeContract {
	t.Helper()

	w := doJSON(t, srv, "POST", "/api/auth/webauthn/login/begin", map[string]any{
		"email": email,
	}, "")
	testutil.StatusCode(t, 200, w.Code)

	body := mustJSONObjectFromBytes(t, w.Body.Bytes())
	challengeID, _ := body["challenge_id"].(string)
	testutil.True(t, challengeID != "", "first-factor begin endpoint must return challenge_id")

	optionsJSON := extractOptionsJSON(t, body, w.Body.String())
	options, err := virtualwebauthn.ParseAssertionOptions(optionsJSON)
	testutil.NoError(t, err)

	return &webauthnChallengeContract{ChallengeID: challengeID, Options: options, Body: body}
}

func beginWebAuthnDiscoverableChallenge(t *testing.T, srv *server.Server) *webauthnChallengeContract {
	t.Helper()

	w := doJSON(t, srv, "POST", "/api/auth/webauthn/login/discover/begin", map[string]any{}, "")
	testutil.StatusCode(t, 200, w.Code)

	body := mustJSONObjectFromBytes(t, w.Body.Bytes())
	challengeID, _ := body["challenge_id"].(string)
	testutil.True(t, challengeID != "", "discoverable begin endpoint must return challenge_id")

	optionsJSON := extractOptionsJSON(t, body, w.Body.String())
	options, err := virtualwebauthn.ParseAssertionOptions(optionsJSON)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, len(options.AllowCredentials))

	return &webauthnChallengeContract{ChallengeID: challengeID, Options: options, Body: body}
}

type webAuthnMFAVerification struct {
	RP                   virtualwebauthn.RelyingParty
	UserID               string
	Email                string
	Passkey              virtualWebAuthnCredential
	Counter              uint32
	AllowedCredentialIDs [][]byte
}

func verifyWebAuthnMFACredential(
	t *testing.T,
	srv *server.Server,
	verification webAuthnMFAVerification,
) {
	t.Helper()

	pendingToken := loginAndGetPendingToken(t, srv, verification.Email)
	challenge := beginWebAuthnChallenge(t, srv, pendingToken)
	testutil.Equal(t, len(verification.AllowedCredentialIDs), len(challenge.Options.AllowCredentials))
	for _, credentialID := range verification.AllowedCredentialIDs {
		assertCredentialAllowedForAssertion(t, challenge.Options, credentialID)
	}

	authenticator := verification.Passkey.Authenticator
	authenticator.Options.UserHandle = []byte(verification.UserID)
	credential := verification.Passkey.Credential
	credential.Counter = verification.Counter
	assertionResponse := virtualwebauthn.CreateAssertionResponse(
		verification.RP,
		authenticator,
		credential,
		*challenge.Options,
	)
	verify := doJSON(t, srv, "POST", "/api/auth/mfa/webauthn/verify", map[string]any{
		"challenge_id":       challenge.ChallengeID,
		"assertion_response": mustJSONObject(t, assertionResponse),
	}, pendingToken)
	testutil.StatusCode(t, 200, verify.Code)
}

func assertWebAuthnAAL2Claims(t *testing.T, authSvc *auth.Service, token string) {
	t.Helper()

	claims, err := authSvc.ValidateToken(token)
	testutil.NoError(t, err)
	testutil.Equal(t, "aal2", claims.AAL)
	testutil.Equal(t, 2, len(claims.AMR))
	testutil.Equal(t, "password", claims.AMR[0])
	testutil.Equal(t, "webauthn", claims.AMR[1])
}

func deleteWebAuthnAndAssertNoFactors(
	t *testing.T,
	srv *server.Server,
	authSvc *auth.Service,
	userID string,
	accessToken string,
) {
	t.Helper()

	deleteResp := doJSON(t, srv, "DELETE", "/api/auth/mfa/webauthn/", nil, accessToken)
	testutil.StatusCode(t, 204, deleteResp.Code)

	factors, err := authSvc.GetUserMFAFactors(t.Context(), userID)
	testutil.NoError(t, err)
	testutil.SliceLen(t, factors, 0)
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
