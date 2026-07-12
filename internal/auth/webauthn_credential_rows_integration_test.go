//go:build integration

package auth_test

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"sort"
	"testing"

	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/descope/virtualwebauthn"
)

type virtualWebAuthnCredential struct {
	Authenticator virtualwebauthn.Authenticator
	Credential    virtualwebauthn.Credential
}

type webAuthnCredentialRow struct {
	FactorID     string
	CredentialID []byte
	PublicKey    []byte
	Transports   []string
	SignCount    int64
	DisplayName  string
	LastUsedAt   sql.NullTime
}

func enrollVirtualWebAuthnCredential(
	t *testing.T,
	srv *server.Server,
	accessToken string,
	rp virtualwebauthn.RelyingParty,
	displayName string,
	transports []virtualwebauthn.Transport,
) virtualWebAuthnCredential {
	t.Helper()

	authenticator := virtualwebauthn.NewAuthenticatorWithOptions(virtualwebauthn.AuthenticatorOptions{
		Transports: transports,
	})
	credential := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)
	enrollOptions := beginWebAuthnEnroll(t, srv, accessToken)
	attestationResponse := virtualwebauthn.CreateAttestationResponse(rp, authenticator, credential, *enrollOptions)
	confirm := doJSON(t, srv, "POST", "/api/auth/mfa/webauthn/enroll/confirm", map[string]any{
		"display_name":         displayName,
		"attestation_response": mustJSONObject(t, attestationResponse),
	}, accessToken)
	testutil.StatusCode(t, 200, confirm.Code)

	authenticator.AddCredential(credential)
	return virtualWebAuthnCredential{Authenticator: authenticator, Credential: credential}
}

func webAuthnAAL2TokenForCredential(
	t *testing.T,
	srv *server.Server,
	rp virtualwebauthn.RelyingParty,
	userID string,
	email string,
	passkey virtualWebAuthnCredential,
) string {
	t.Helper()

	authenticator := passkey.Authenticator
	authenticator.Options.UserHandle = []byte(userID)
	credential := passkey.Credential
	credential.Counter = 1

	pendingToken := loginAndGetPendingToken(t, srv, email)
	challenge := beginWebAuthnChallenge(t, srv, pendingToken)
	assertCredentialAllowedForAssertion(t, challenge.Options, credential.ID)
	assertionResponse := virtualwebauthn.CreateAssertionResponse(rp, authenticator, credential, *challenge.Options)
	verify := doJSON(t, srv, "POST", "/api/auth/mfa/webauthn/verify", map[string]any{
		"challenge_id":       challenge.ChallengeID,
		"assertion_response": mustJSONObject(t, assertionResponse),
	}, pendingToken)
	testutil.StatusCode(t, 200, verify.Code)
	return parseAuthResp(t, verify).Token
}

func loadWebAuthnCredentialRows(t *testing.T, userID string) []webAuthnCredentialRow {
	t.Helper()

	rows, err := sharedPG.Pool.Query(t.Context(),
		`SELECT c.factor_id::text, c.credential_id, c.public_key, c.transports,
		        c.sign_count, c.display_name, c.last_used_at
		   FROM _ayb_webauthn_credentials c
		   JOIN _ayb_user_mfa f ON f.id = c.factor_id
		  WHERE f.user_id = $1 AND f.method = 'webauthn' AND f.enabled = true
		  ORDER BY c.display_name`,
		userID,
	)
	testutil.NoError(t, err)
	defer rows.Close()

	var result []webAuthnCredentialRow
	for rows.Next() {
		var row webAuthnCredentialRow
		err := rows.Scan(
			&row.FactorID,
			&row.CredentialID,
			&row.PublicKey,
			&row.Transports,
			&row.SignCount,
			&row.DisplayName,
			&row.LastUsedAt,
		)
		testutil.NoError(t, err)
		result = append(result, row)
	}
	testutil.NoError(t, rows.Err())
	return result
}

func assertWebAuthnCredentialRow(
	t *testing.T,
	rows []webAuthnCredentialRow,
	credentialID []byte,
	displayName string,
	transports []string,
	signCount int64,
	wantLastUsed bool,
) {
	t.Helper()

	for _, row := range rows {
		if !bytes.Equal(row.CredentialID, credentialID) {
			continue
		}
		testutil.True(t, row.FactorID != "", "credential row must keep the MFA factor id")
		testutil.True(t, len(row.PublicKey) > 0, "credential row must persist public key bytes")
		testutil.Equal(t, displayName, row.DisplayName)
		testutil.Equal(t, signCount, row.SignCount)
		assertStringSet(t, transports, row.Transports)
		testutil.Equal(t, wantLastUsed, row.LastUsedAt.Valid)
		return
	}
	t.Fatalf("credential row %q not found", base64.RawURLEncoding.EncodeToString(credentialID))
}

func assertWebAuthnCounterPersisted(t *testing.T, credentialID []byte, want int64, wantLastUsed bool) {
	t.Helper()

	var row webAuthnCredentialRow
	err := sharedPG.Pool.QueryRow(t.Context(),
		`SELECT factor_id::text, credential_id, public_key, transports,
		        sign_count, display_name, last_used_at
		   FROM _ayb_webauthn_credentials
		  WHERE credential_id = $1`,
		credentialID,
	).Scan(
		&row.FactorID,
		&row.CredentialID,
		&row.PublicKey,
		&row.Transports,
		&row.SignCount,
		&row.DisplayName,
		&row.LastUsedAt,
	)
	testutil.NoError(t, err)
	testutil.Equal(t, want, row.SignCount)
	testutil.Equal(t, wantLastUsed, row.LastUsedAt.Valid)
}

func assertCredentialAllowedForAssertion(
	t *testing.T,
	options *virtualwebauthn.AssertionOptions,
	credentialID []byte,
) {
	t.Helper()

	encodedID := base64.RawURLEncoding.EncodeToString(credentialID)
	for _, allowedID := range options.AllowCredentials {
		if allowedID == encodedID {
			return
		}
	}
	t.Fatalf("credential %q was not present in allowCredentials", encodedID)
}

func assertStringSet(t *testing.T, want, got []string) {
	t.Helper()

	wantCopy := append([]string(nil), want...)
	gotCopy := append([]string(nil), got...)
	sort.Strings(wantCopy)
	sort.Strings(gotCopy)
	testutil.Equal(t, len(wantCopy), len(gotCopy))
	for i := range wantCopy {
		testutil.Equal(t, wantCopy[i], gotCopy[i])
	}
}
