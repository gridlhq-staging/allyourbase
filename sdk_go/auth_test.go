package allyourbase

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestAuthRegisterLoginMeRefreshLifecycle(t *testing.T) {
	step := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch step {
		case 0, 1, 3:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"token":        "tok1",
				"refreshToken": "ref1",
				"user": map[string]any{
					"id":    "usr_1",
					"email": "alice@example.com",
				},
			})
		case 2:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "usr_1", "email": "alice@example.com"})
		}
		step++
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	if _, err := c.Auth.Register(context.Background(), "alice@example.com", "secret"); err != nil {
		t.Fatal(err)
	}
	res, err := c.Auth.Login(context.Background(), "alice@example.com", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if res.Token != "tok1" || c.Token() != "tok1" || c.RefreshToken() != "ref1" {
		t.Fatalf("tokens not set")
	}
	me, err := c.Auth.Me(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if me.Email != "alice@example.com" {
		t.Fatalf("unexpected me")
	}
	if _, err := c.Auth.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestAuthLogoutDeleteAndUtilityEndpoints(t *testing.T) {
	step := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if step == 0 || step == 1 || step == 2 || step == 3 || step == 4 || step == 5 {
			w.WriteHeader(http.StatusNoContent)
		}
		step++
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	c.SetTokens("tok", "ref")
	if err := c.Auth.RequestPasswordReset(context.Background(), "alice@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := c.Auth.ConfirmPasswordReset(context.Background(), "token", "password"); err != nil {
		t.Fatal(err)
	}
	if err := c.Auth.VerifyEmail(context.Background(), "token"); err != nil {
		t.Fatal(err)
	}
	if err := c.Auth.ResendVerification(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := c.Auth.Logout(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.Token() != "" {
		t.Fatalf("expected token cleared")
	}
	c.SetTokens("tok", "ref")
	if err := c.Auth.DeleteAccount(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.Token() != "" {
		t.Fatalf("expected token cleared")
	}
}

func TestAuthSignInAnonymouslyStoresTokens(t *testing.T) {
	fixture := mustLoadSDKParityFixture(t, "anonymous.json")
	var requestBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/auth/anonymous" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(fixture.Response)
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	res, err := c.Auth.SignInAnonymously(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.User.IsAnonymous == nil || !*res.User.IsAnonymous {
		t.Fatalf("expected anonymous user, got %+v", res.User)
	}
	if c.Token() != res.Token || c.RefreshToken() != res.RefreshToken {
		t.Fatalf("tokens not stored")
	}
	if len(requestBody) != 0 {
		t.Fatalf("expected empty request body, got %+v", requestBody)
	}
}

func TestAuthRequestMagicLinkPostsEmailWithoutMutatingTokens(t *testing.T) {
	response := mustLoadSDKContractResponse(t, "magic_link_request_response.json")
	var requestBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/auth/magic-link" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	res, err := c.Auth.RequestMagicLink(context.Background(), "fixture@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if res.Message != "If an account exists, a magic link has been sent." {
		t.Fatalf("unexpected message %q", res.Message)
	}
	if c.Token() != "" || c.RefreshToken() != "" {
		t.Fatalf("requestMagicLink should not mutate tokens")
	}
	if got := requestBody["email"]; got != "fixture@example.com" {
		t.Fatalf("unexpected email %#v", got)
	}
}

func TestAuthConfirmMagicLinkStoresTokensForAuthenticatedResponse(t *testing.T) {
	response := mustLoadSDKContractResponse(t, "magic_link_confirm_success_response.json")
	var requestBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/auth/magic-link/confirm" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	res, err := c.Auth.ConfirmMagicLink(context.Background(), "sdk-parity-magic-token")
	if err != nil {
		t.Fatal(err)
	}
	if res.Auth == nil {
		t.Fatalf("expected authenticated response, got %+v", res)
	}
	if res.Auth.User.Email != "magic@allyourbase.io" {
		t.Fatalf("unexpected email %s", res.Auth.User.Email)
	}
	if res.Auth.Token != "jwt_magic_link" {
		t.Fatalf("unexpected token %q", res.Auth.Token)
	}
	if res.Auth.RefreshToken != "refresh_magic_link" {
		t.Fatalf("unexpected refresh token %q", res.Auth.RefreshToken)
	}
	if c.Token() != res.Auth.Token || c.RefreshToken() != res.Auth.RefreshToken {
		t.Fatalf("tokens not stored")
	}
	if got := requestBody["token"]; got != "sdk-parity-magic-token" {
		t.Fatalf("unexpected token %#v", got)
	}
}

func TestAuthLinkEmailUsesAuthenticatedRequestAndReturnsLinkedUser(t *testing.T) {
	fixture := mustLoadSDKParityFixture(t, "link_email.json")
	var requestBody map[string]any
	var authHeader string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/auth/link/email" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		authHeader = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(fixture.Response)
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	c.SetTokens("anon_token", "anon_refresh")
	res, err := c.Auth.LinkEmail(context.Background(), "upgraded@example.com", "LinkedPass123!")
	if err != nil {
		t.Fatal(err)
	}
	if res.User.Email != "upgraded@example.com" {
		t.Fatalf("unexpected email %s", res.User.Email)
	}
	if res.User.IsAnonymous != nil && *res.User.IsAnonymous {
		t.Fatalf("expected non-anonymous linked user, got %+v", res.User)
	}
	if res.User.LinkedAt == nil || *res.User.LinkedAt == "" {
		t.Fatalf("expected linked_at in response, got %+v", res.User)
	}
	if c.Token() != res.Token || c.RefreshToken() != res.RefreshToken {
		t.Fatalf("tokens not updated")
	}
	if authHeader != "Bearer anon_token" {
		t.Fatalf("unexpected authorization header %q", authHeader)
	}
	if got := requestBody["email"]; got != "upgraded@example.com" {
		t.Fatalf("unexpected email %#v", got)
	}
	if got := requestBody["password"]; got != "LinkedPass123!" {
		t.Fatalf("unexpected password %#v", got)
	}
}

func TestAuthConfirmMagicLinkReturnsPendingMFAWithoutMutatingTokens(t *testing.T) {
	response := mustLoadSDKContractResponse(t, "magic_link_confirm_pending_mfa_response.json")
	var requestBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/auth/magic-link/confirm" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	c.SetTokens("existing_tok", "existing_ref")
	res, err := c.Auth.ConfirmMagicLink(context.Background(), "sdk-parity-magic-token-pending")
	if err != nil {
		t.Fatal(err)
	}
	if !res.MFAPending {
		t.Fatalf("expected MFAPending=true, got %+v", res)
	}
	if res.MFAToken != "mfa_pending_token_stage1" {
		t.Fatalf("unexpected MFA token %q", res.MFAToken)
	}
	if res.Auth != nil {
		t.Fatalf("expected nil Auth on pending MFA, got %+v", res.Auth)
	}
	if c.Token() != "existing_tok" || c.RefreshToken() != "existing_ref" {
		t.Fatalf("tokens mutated on pending MFA: token=%q refresh=%q", c.Token(), c.RefreshToken())
	}
	if got := requestBody["token"]; got != "sdk-parity-magic-token-pending" {
		t.Fatalf("unexpected token %#v", got)
	}
}

func TestAuthConfirmMagicLinkPropagatesNon2xxError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    "auth/invalid-token",
			"message": "Token expired",
		})
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	c.SetTokens("pre_tok", "pre_ref")
	res, err := c.Auth.ConfirmMagicLink(context.Background(), "expired-token")
	if err == nil {
		t.Fatalf("expected error, got nil (res=%+v)", res)
	}
	apiErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T: %v", err, err)
	}
	if apiErr.Status != 401 {
		t.Fatalf("unexpected status %d", apiErr.Status)
	}
	if apiErr.Code != "auth/invalid-token" {
		t.Fatalf("unexpected code %q", apiErr.Code)
	}
	if apiErr.Message != "Token expired" {
		t.Fatalf("unexpected message %q", apiErr.Message)
	}
	if c.Token() != "pre_tok" || c.RefreshToken() != "pre_ref" {
		t.Fatalf("tokens mutated on error: token=%q refresh=%q", c.Token(), c.RefreshToken())
	}
}

type sdkParityFixture struct {
	Request  map[string]any `json:"request"`
	Response map[string]any `json:"response"`
}

func mustLoadSDKParityFixture(t *testing.T, name string) sdkParityFixture {
	t.Helper()

	path := filepath.Join("..", "tests", "contract", "fixtures", "sdk_parity", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	var fixture sdkParityFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode fixture %s: %v", name, err)
	}
	return fixture
}

// mustLoadSDKContractResponse reads a canonical sdk_contract fixture as a bare
// response body (no request/response envelope). The sdk_contract tree is the
// single source of truth for magic-link wire shapes shared across SDKs.
func mustLoadSDKContractResponse(t *testing.T, name string) map[string]any {
	t.Helper()

	path := filepath.Join("..", "tests", "contract", "fixtures", "sdk_contract", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	var response map[string]any
	if err := json.Unmarshal(data, &response); err != nil {
		t.Fatalf("decode fixture %s: %v", name, err)
	}
	return response
}
