// Package allyourbase provides Go SDK auth helpers for credential, magic-link, anonymous, OAuth, and WebAuthn flows.
package allyourbase

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type AuthClient struct {
	client *Client
}

func (a *AuthClient) Register(ctx context.Context, email, password string) (*AuthResponse, error) {
	return a.authWithCredentials(ctx, "/api/auth/register", email, password)
}

func (a *AuthClient) Login(ctx context.Context, email, password string) (*AuthResponse, error) {
	return a.authWithCredentials(ctx, "/api/auth/login", email, password)
}

func (a *AuthClient) OAuthStartURL(provider, state string, scopes []string, redirectTo *string) string {
	query := []string{"state=" + oauthQueryEscape(state)}
	if len(scopes) > 0 {
		query = append(query, "scopes="+oauthQueryEscape(strings.Join(scopes, ",")))
	}
	if redirectTo != nil {
		query = append(query, "redirect_to="+oauthQueryEscape(*redirectTo))
	}
	return a.client.baseURL + "/api/auth/oauth/" + url.PathEscape(provider) + "?" + strings.Join(query, "&")
}

func oauthQueryEscape(value string) string {
	escaped := url.QueryEscape(value)
	escaped = strings.ReplaceAll(escaped, "+", "%20")
	escaped = strings.ReplaceAll(escaped, "%21", "!")
	escaped = strings.ReplaceAll(escaped, "%27", "'")
	escaped = strings.ReplaceAll(escaped, "%28", "(")
	escaped = strings.ReplaceAll(escaped, "%29", ")")
	return strings.ReplaceAll(escaped, "%2A", "*")
}

func (a *AuthClient) BeginWebAuthnLogin(ctx context.Context, email string) (*WebAuthnLoginBeginResponse, error) {
	body, err := a.client.doJSON(ctx, http.MethodPost, "/api/auth/webauthn/login/begin", nil, map[string]string{"email": email})
	if err != nil {
		return nil, err
	}
	var out WebAuthnLoginBeginResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (a *AuthClient) BeginDiscoverableLogin(ctx context.Context) (*WebAuthnLoginBeginResponse, error) {
	body, err := a.client.doJSON(ctx, http.MethodPost, "/api/auth/webauthn/login/discover/begin", nil, nil)
	if err != nil {
		return nil, err
	}
	var out WebAuthnLoginBeginResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// FinishWebAuthnLogin completes a WebAuthn login ceremony and stores returned session tokens on the client.
func (a *AuthClient) FinishWebAuthnLogin(ctx context.Context, challengeID string, assertionResponse json.RawMessage) (*AuthResponse, error) {
	req := WebAuthnLoginFinishRequest{
		ChallengeID:       challengeID,
		AssertionResponse: assertionResponse,
	}
	body, err := a.client.doJSON(ctx, http.MethodPost, "/api/auth/webauthn/login/finish", nil, req)
	if err != nil {
		return nil, err
	}
	var out AuthResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	a.client.SetTokens(out.Token, out.RefreshToken)
	return &out, nil
}

// FinishDiscoverableLogin completes a discoverable WebAuthn login and stores returned session tokens on the client.
func (a *AuthClient) FinishDiscoverableLogin(ctx context.Context, challengeID string, assertionResponse json.RawMessage) (*AuthResponse, error) {
	req := WebAuthnLoginFinishRequest{
		ChallengeID:       challengeID,
		AssertionResponse: assertionResponse,
	}
	body, err := a.client.doJSON(ctx, http.MethodPost, "/api/auth/webauthn/login/discover/finish", nil, req)
	if err != nil {
		return nil, err
	}
	var out AuthResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	a.client.SetTokens(out.Token, out.RefreshToken)
	return &out, nil
}

func (a *AuthClient) EnrollWebAuthn(ctx context.Context) (*WebAuthnEnrollBeginResponse, error) {
	body, err := a.client.doJSON(ctx, http.MethodPost, "/api/auth/mfa/webauthn/enroll", nil, nil)
	if err != nil {
		return nil, err
	}
	var out WebAuthnEnrollBeginResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (a *AuthClient) ConfirmWebAuthnEnrollment(ctx context.Context, displayName string, attestationResponse json.RawMessage) (*WebAuthnEnrollConfirmResponse, error) {
	req := WebAuthnEnrollConfirmRequest{
		DisplayName:         displayName,
		AttestationResponse: attestationResponse,
	}
	body, err := a.client.doJSON(ctx, http.MethodPost, "/api/auth/mfa/webauthn/enroll/confirm", nil, req)
	if err != nil {
		return nil, err
	}
	var out WebAuthnEnrollConfirmResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (a *AuthClient) WebAuthnChallenge(ctx context.Context, mfaToken string) (*WebAuthnMFAChallengeResponse, error) {
	body, err := a.doJSONWithBearer(ctx, http.MethodPost, "/api/auth/mfa/webauthn/challenge", mfaToken, nil)
	if err != nil {
		return nil, err
	}
	var out WebAuthnMFAChallengeResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// WebAuthnVerify verifies a WebAuthn MFA assertion with the provided MFA bearer token and stores returned session tokens.
func (a *AuthClient) WebAuthnVerify(ctx context.Context, mfaToken, challengeID string, assertionResponse json.RawMessage) (*AuthResponse, error) {
	req := WebAuthnMFAVerifyRequest{
		ChallengeID:       challengeID,
		AssertionResponse: assertionResponse,
	}
	body, err := a.doJSONWithBearer(ctx, http.MethodPost, "/api/auth/mfa/webauthn/verify", mfaToken, req)
	if err != nil {
		return nil, err
	}
	var out AuthResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	a.client.SetTokens(out.Token, out.RefreshToken)
	return &out, nil
}

func (a *AuthClient) DeleteWebAuthn(ctx context.Context) error {
	_, err := a.client.doJSON(ctx, http.MethodDelete, "/api/auth/mfa/webauthn", nil, nil)
	return err
}

// doJSONWithBearer encodes an optional JSON body and sends an authenticated request using the provided bearer token.
func (a *AuthClient) doJSONWithBearer(ctx context.Context, method, path, bearer string, body any) ([]byte, error) {
	var rdr io.Reader
	headers := map[string]string{"Authorization": "Bearer " + bearer}
	if body != nil {
		buf := bytes.NewBuffer(nil)
		if err := json.NewEncoder(buf).Encode(body); err != nil {
			return nil, err
		}
		rdr = buf
		headers["Content-Type"] = "application/json"
	}
	_, respBody, _, err := a.client.do(ctx, method, path, nil, rdr, headers, false)
	if err != nil {
		return nil, err
	}
	return respBody, nil
}

func (a *AuthClient) SignInAnonymously(ctx context.Context) (*AuthResponse, error) {
	body, err := a.client.doJSON(ctx, http.MethodPost, "/api/auth/anonymous", nil, map[string]any{})
	if err != nil {
		return nil, err
	}
	var out AuthResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	a.client.SetTokens(out.Token, out.RefreshToken)
	return &out, nil
}

func (a *AuthClient) RequestMagicLink(ctx context.Context, email string) (*MagicLinkRequestResponse, error) {
	body, err := a.client.doJSON(ctx, http.MethodPost, "/api/auth/magic-link", nil, map[string]string{"email": email})
	if err != nil {
		return nil, err
	}
	var out MagicLinkRequestResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (a *AuthClient) ConfirmMagicLink(ctx context.Context, token string) (*MagicLinkConfirmResponse, error) {
	body, err := a.client.doJSON(ctx, http.MethodPost, "/api/auth/magic-link/confirm", nil, map[string]string{"token": token})
	if err != nil {
		return nil, err
	}
	var out MagicLinkConfirmResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	if out.Auth != nil {
		a.client.SetTokens(out.Auth.Token, out.Auth.RefreshToken)
	}
	return &out, nil
}

func (a *AuthClient) LinkEmail(ctx context.Context, email, password string) (*AuthResponse, error) {
	return a.authWithCredentials(ctx, "/api/auth/link/email", email, password)
}

func (a *AuthClient) authWithCredentials(ctx context.Context, path, email, password string) (*AuthResponse, error) {
	body, err := a.client.doJSON(ctx, http.MethodPost, path, nil, map[string]string{"email": email, "password": password})
	if err != nil {
		return nil, err
	}
	var out AuthResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	a.client.SetTokens(out.Token, out.RefreshToken)
	return &out, nil
}

func (a *AuthClient) Me(ctx context.Context) (*User, error) {
	body, err := a.client.doJSON(ctx, http.MethodGet, "/api/auth/me", nil, nil)
	if err != nil {
		return nil, err
	}
	var out User
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (a *AuthClient) Refresh(ctx context.Context) (*AuthResponse, error) {
	body, err := a.client.doJSON(ctx, http.MethodPost, "/api/auth/refresh", nil, map[string]string{"refreshToken": a.client.RefreshToken()})
	if err != nil {
		return nil, err
	}
	var out AuthResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	a.client.SetTokens(out.Token, out.RefreshToken)
	return &out, nil
}

func (a *AuthClient) Logout(ctx context.Context) error {
	_, err := a.client.doJSON(ctx, http.MethodPost, "/api/auth/logout", nil, map[string]string{"refreshToken": a.client.RefreshToken()})
	if err != nil {
		return err
	}
	a.client.ClearTokens()
	return nil
}

func (a *AuthClient) DeleteAccount(ctx context.Context) error {
	_, err := a.client.doJSON(ctx, http.MethodDelete, "/api/auth/me", nil, nil)
	if err != nil {
		return err
	}
	a.client.ClearTokens()
	return nil
}

func (a *AuthClient) RequestPasswordReset(ctx context.Context, email string) error {
	_, err := a.client.doJSON(ctx, http.MethodPost, "/api/auth/password-reset", nil, map[string]string{"email": email})
	return err
}

func (a *AuthClient) ConfirmPasswordReset(ctx context.Context, token, password string) error {
	_, err := a.client.doJSON(ctx, http.MethodPost, "/api/auth/password-reset/confirm", nil, map[string]string{"token": token, "password": password})
	return err
}

func (a *AuthClient) VerifyEmail(ctx context.Context, token string) error {
	_, err := a.client.doJSON(ctx, http.MethodPost, "/api/auth/verify", nil, map[string]string{"token": token})
	return err
}

func (a *AuthClient) ResendVerification(ctx context.Context) error {
	_, err := a.client.doJSON(ctx, http.MethodPost, "/api/auth/verify/resend", nil, nil)
	return err
}
