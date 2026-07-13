package allyourbase

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
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

func TestAuthBeginWebAuthnLoginPostsEmailWithoutMutatingTokens(t *testing.T) {
	response := mustLoadSDKContractResponse(t, "webauthn_login_begin_response.json")
	var requestBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/auth/webauthn/login/begin" {
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
	res, err := c.Auth.BeginWebAuthnLogin(context.Background(), "fixture@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if res.ChallengeID != "webauthn_challenge_fixture" {
		t.Fatalf("unexpected challenge id %q", res.ChallengeID)
	}
	if c.Token() != "existing_tok" || c.RefreshToken() != "existing_ref" {
		t.Fatalf("tokens mutated: token=%q refresh=%q", c.Token(), c.RefreshToken())
	}
	if len(requestBody) != 1 {
		t.Fatalf("unexpected request body %+v", requestBody)
	}
	if got := requestBody["email"]; got != "fixture@example.com" {
		t.Fatalf("unexpected email %#v", got)
	}
}

func TestAuthBeginDiscoverableLoginPostsBodylessRequestWithoutMutatingTokens(t *testing.T) {
	response := mustLoadSDKContractResponse(t, "webauthn_discover_begin_response.json")
	var requestBody []byte
	var contentType string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/auth/webauthn/login/discover/begin" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		var err error
		requestBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		contentType = r.Header.Get("Content-Type")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	c.SetTokens("existing_tok", "existing_ref")
	res, err := c.Auth.BeginDiscoverableLogin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.ChallengeID != "webauthn_discover_challenge_fixture" {
		t.Fatalf("unexpected challenge id %q", res.ChallengeID)
	}
	if len(res.Options.AllowCredentials) != 0 {
		t.Fatalf("expected absent or empty allowCredentials, got %+v", res.Options.AllowCredentials)
	}
	if c.Token() != "existing_tok" || c.RefreshToken() != "existing_ref" {
		t.Fatalf("tokens mutated: token=%q refresh=%q", c.Token(), c.RefreshToken())
	}
	if len(requestBody) != 0 {
		t.Fatalf("expected bodyless request, got %q", string(requestBody))
	}
	if contentType != "" {
		t.Fatalf("expected no content type for bodyless request, got %q", contentType)
	}
}

func TestAuthFinishWebAuthnLoginPostsRawAssertionAndStoresTokens(t *testing.T) {
	response := mustLoadSDKContractResponse(t, "auth_response.json")
	assertion := json.RawMessage(`{"id":"credential-a","response":{"clientDataJSON":"client","authenticatorData":"auth","signature":"sig"},"type":"public-key"}`)
	var requestBody struct {
		ChallengeID       string          `json:"challenge_id"`
		AssertionResponse json.RawMessage `json:"assertion_response"`
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/auth/webauthn/login/finish" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	res, err := c.Auth.FinishWebAuthnLogin(context.Background(), "webauthn_challenge_fixture", assertion)
	if err != nil {
		t.Fatal(err)
	}
	if res == nil {
		t.Fatalf("expected AuthResponse")
	}
	if res.Token != "jwt_stage3" || res.RefreshToken != "refresh_stage3" {
		t.Fatalf("unexpected auth response %+v", res)
	}
	if c.Token() != "jwt_stage3" || c.RefreshToken() != "refresh_stage3" {
		t.Fatalf("tokens not stored: token=%q refresh=%q", c.Token(), c.RefreshToken())
	}
	if requestBody.ChallengeID != "webauthn_challenge_fixture" {
		t.Fatalf("unexpected challenge_id %q", requestBody.ChallengeID)
	}
	if string(requestBody.AssertionResponse) != string(assertion) {
		t.Fatalf("assertion_response changed: got %s want %s", requestBody.AssertionResponse, assertion)
	}
}

func TestAuthFinishDiscoverableLoginPostsRawAssertionAndStoresTokens(t *testing.T) {
	response := mustLoadSDKContractResponse(t, "auth_response.json")
	requestData := mustLoadContractFixture(t, "webauthn_discover_finish_request.json")
	var expectedRequest WebAuthnLoginFinishRequest
	if err := json.Unmarshal(requestData, &expectedRequest); err != nil {
		t.Fatalf("decode webauthn_discover_finish_request: %v", err)
	}
	var requestBody WebAuthnLoginFinishRequest
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/auth/webauthn/login/discover/finish" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	res, err := c.Auth.FinishDiscoverableLogin(
		context.Background(),
		expectedRequest.ChallengeID,
		expectedRequest.AssertionResponse,
	)
	if err != nil {
		t.Fatal(err)
	}
	if res == nil {
		t.Fatalf("expected AuthResponse")
	}
	if res.Token != "jwt_stage3" || res.RefreshToken != "refresh_stage3" {
		t.Fatalf("unexpected auth response %+v", res)
	}
	if c.Token() != "jwt_stage3" || c.RefreshToken() != "refresh_stage3" {
		t.Fatalf("tokens not stored: token=%q refresh=%q", c.Token(), c.RefreshToken())
	}
	if requestBody.ChallengeID != expectedRequest.ChallengeID {
		t.Fatalf("unexpected challenge_id %q", requestBody.ChallengeID)
	}
	var actualAssertion map[string]any
	if err := json.Unmarshal(requestBody.AssertionResponse, &actualAssertion); err != nil {
		t.Fatalf("decode actual assertion_response: %v", err)
	}
	var expectedAssertion map[string]any
	if err := json.Unmarshal(expectedRequest.AssertionResponse, &expectedAssertion); err != nil {
		t.Fatalf("decode expected assertion_response: %v", err)
	}
	if !reflect.DeepEqual(actualAssertion, expectedAssertion) {
		t.Fatalf("assertion_response changed: got %#v want %#v", actualAssertion, expectedAssertion)
	}
}

func TestAuthWebAuthnEnrollmentUsesSessionBearerWithoutMutatingTokens(t *testing.T) {
	enrollResponse := mustLoadSDKContractResponse(t, "webauthn_enroll_begin_response.json")
	confirmResponse := mustLoadSDKContractResponse(t, "webauthn_enroll_confirm_response.json")
	confirmRequestData := mustLoadContractFixture(t, "webauthn_enroll_confirm_request.json")
	var confirmRequest WebAuthnEnrollConfirmRequest
	if err := json.Unmarshal(confirmRequestData, &confirmRequest); err != nil {
		t.Fatalf("decode confirm request fixture: %v", err)
	}

	var confirmBody map[string]any
	paths := make([]string, 0, 3)
	methods := make([]string, 0, 3)
	authHeaders := make([]string, 0, 3)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		methods = append(methods, r.Method)
		authHeaders = append(authHeaders, r.Header.Get("Authorization"))
		switch r.URL.Path {
		case "/api/auth/mfa/webauthn/enroll":
			if r.Method != http.MethodPost {
				t.Fatalf("expected enroll POST, got %s", r.Method)
			}
			_ = json.NewEncoder(w).Encode(enrollResponse)
		case "/api/auth/mfa/webauthn/enroll/confirm":
			if r.Method != http.MethodPost {
				t.Fatalf("expected confirm POST, got %s", r.Method)
			}
			if err := json.NewDecoder(r.Body).Decode(&confirmBody); err != nil {
				t.Fatalf("decode confirm body: %v", err)
			}
			_ = json.NewEncoder(w).Encode(confirmResponse)
		case "/api/auth/mfa/webauthn":
			if r.Method != http.MethodDelete {
				t.Fatalf("expected delete DELETE, got %s", r.Method)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	c.SetTokens("session_token", "session_refresh")
	begin, err := c.Auth.EnrollWebAuthn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	confirm, err := c.Auth.ConfirmWebAuthnEnrollment(
		context.Background(),
		confirmRequest.DisplayName,
		confirmRequest.AttestationResponse,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Auth.DeleteWebAuthn(context.Background()); err != nil {
		t.Fatal(err)
	}

	if begin.Challenge != "webauthn_enroll_begin_challenge" {
		t.Fatalf("unexpected enroll challenge %q", begin.Challenge)
	}
	if confirm.Message != "WebAuthn MFA enrollment confirmed" {
		t.Fatalf("unexpected confirm message %q", confirm.Message)
	}
	if c.Token() != "session_token" || c.RefreshToken() != "session_refresh" {
		t.Fatalf("tokens mutated: token=%q refresh=%q", c.Token(), c.RefreshToken())
	}
	expectedPaths := []string{"/api/auth/mfa/webauthn/enroll", "/api/auth/mfa/webauthn/enroll/confirm", "/api/auth/mfa/webauthn"}
	if !reflect.DeepEqual(paths, expectedPaths) {
		t.Fatalf("paths mismatch: got %#v want %#v", paths, expectedPaths)
	}
	expectedMethods := []string{http.MethodPost, http.MethodPost, http.MethodDelete}
	if !reflect.DeepEqual(methods, expectedMethods) {
		t.Fatalf("methods mismatch: got %#v want %#v", methods, expectedMethods)
	}
	for _, got := range authHeaders {
		if got != "Bearer session_token" {
			t.Fatalf("unexpected authorization headers %#v", authHeaders)
		}
	}
	var expectedConfirmBody map[string]any
	if err := json.Unmarshal(confirmRequestData, &expectedConfirmBody); err != nil {
		t.Fatalf("decode expected confirm body: %v", err)
	}
	if !reflect.DeepEqual(confirmBody, expectedConfirmBody) {
		t.Fatalf("confirm body mismatch\n got: %#v\nwant: %#v", confirmBody, expectedConfirmBody)
	}
}

func TestAuthWebAuthnChallengeAndVerifyUseMFABearerAndVerifyStoresTokens(t *testing.T) {
	challengeResponse := mustLoadSDKContractResponse(t, "webauthn_mfa_challenge_response.json")
	verifyResponse := mustLoadSDKContractResponse(t, "webauthn_mfa_verify_response.json")
	verifyRequestData := mustLoadContractFixture(t, "webauthn_mfa_verify_request.json")
	var verifyRequest WebAuthnMFAVerifyRequest
	if err := json.Unmarshal(verifyRequestData, &verifyRequest); err != nil {
		t.Fatalf("decode verify request fixture: %v", err)
	}

	var verifyBody map[string]any
	paths := make([]string, 0, 2)
	authHeaders := make([]string, 0, 2)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		authHeaders = append(authHeaders, r.Header.Get("Authorization"))
		switch r.URL.Path {
		case "/api/auth/mfa/webauthn/challenge":
			if r.Method != http.MethodPost {
				t.Fatalf("expected challenge POST, got %s", r.Method)
			}
			_ = json.NewEncoder(w).Encode(challengeResponse)
		case "/api/auth/mfa/webauthn/verify":
			if r.Method != http.MethodPost {
				t.Fatalf("expected verify POST, got %s", r.Method)
			}
			if err := json.NewDecoder(r.Body).Decode(&verifyBody); err != nil {
				t.Fatalf("decode verify body: %v", err)
			}
			_ = json.NewEncoder(w).Encode(verifyResponse)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	c.SetTokens("session_token", "session_refresh")
	challenge, err := c.Auth.WebAuthnChallenge(context.Background(), "mfa_pending_token")
	if err != nil {
		t.Fatal(err)
	}
	if c.Token() != "session_token" || c.RefreshToken() != "session_refresh" {
		t.Fatalf("challenge mutated tokens: token=%q refresh=%q", c.Token(), c.RefreshToken())
	}
	auth, err := c.Auth.WebAuthnVerify(
		context.Background(),
		"mfa_pending_token",
		verifyRequest.ChallengeID,
		verifyRequest.AssertionResponse,
	)
	if err != nil {
		t.Fatal(err)
	}

	if challenge.ChallengeID != "webauthn_mfa_challenge_fixture" {
		t.Fatalf("unexpected challenge id %q", challenge.ChallengeID)
	}
	if auth.Token != "jwt_webauthn_mfa" || auth.RefreshToken != "refresh_webauthn_mfa" {
		t.Fatalf("unexpected auth response %+v", auth)
	}
	if c.Token() != "jwt_webauthn_mfa" || c.RefreshToken() != "refresh_webauthn_mfa" {
		t.Fatalf("verify did not store tokens: token=%q refresh=%q", c.Token(), c.RefreshToken())
	}
	expectedPaths := []string{"/api/auth/mfa/webauthn/challenge", "/api/auth/mfa/webauthn/verify"}
	if !reflect.DeepEqual(paths, expectedPaths) {
		t.Fatalf("paths mismatch: got %#v want %#v", paths, expectedPaths)
	}
	for _, got := range authHeaders {
		if got != "Bearer mfa_pending_token" {
			t.Fatalf("unexpected authorization headers %#v", authHeaders)
		}
	}
	var expectedVerifyBody map[string]any
	if err := json.Unmarshal(verifyRequestData, &expectedVerifyBody); err != nil {
		t.Fatalf("decode expected verify body: %v", err)
	}
	if !reflect.DeepEqual(verifyBody, expectedVerifyBody) {
		t.Fatalf("verify body mismatch\n got: %#v\nwant: %#v", verifyBody, expectedVerifyBody)
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

// TestAuthOAuthStartURLTableFromFixture pins the OAuth start-URL builder against
// the canonical cross-SDK encoding matrix in oauth_start_url_cases.json. The
// encoding contract mirrors sdk_python's get_oauth_start_url: state, comma-joined
// scopes, and redirect_to are each encodeURIComponent-escaped (comma -> %2C,
// space -> %20, etc.), and the provider segment is path-escaped.
func TestAuthOAuthStartURLTableFromFixture(t *testing.T) {
	data := mustLoadContractFixture(t, "oauth_start_url_cases.json")
	var cases []struct {
		BaseURL           string   `json:"base_url"`
		Provider          string   `json:"provider"`
		State             string   `json:"state"`
		Scopes            []string `json:"scopes"`
		RedirectTo        *string  `json:"redirect_to"`
		ExpectedPathQuery string   `json:"expected_path_query"`
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("decode oauth_start_url_cases: %v", err)
	}
	if len(cases) != 6 {
		t.Fatalf("expected 6 oauth start-url cases, got %d", len(cases))
	}
	for _, tc := range cases {
		t.Run(tc.State, func(t *testing.T) {
			c := NewClient(tc.BaseURL)
			got := c.Auth.OAuthStartURL(tc.Provider, tc.State, tc.Scopes, tc.RedirectTo)
			want := tc.BaseURL + tc.ExpectedPathQuery
			if got != want {
				t.Fatalf("OAuthStartURL mismatch\n got: %q\nwant: %q", got, want)
			}
		})
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
