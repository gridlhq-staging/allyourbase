package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestOAuthReturnToCallbackUsesStateBoundReturnTargetOnce(t *testing.T) {
	t.Parallel()

	h := newOAuthReturnToSuccessHandler(t)
	h.SetOAuthRedirectURL("https://app.example.com/auth/callback")
	router := h.Routes()

	startReq := httptest.NewRequest(http.MethodGet, "/oauth/google?redirect_to="+url.QueryEscape("/workspace?tab=members"), nil)
	startReq.Host = "localhost:8090"
	startW := httptest.NewRecorder()
	router.ServeHTTP(startW, startReq)
	testutil.Equal(t, http.StatusTemporaryRedirect, startW.Code)

	providerRedirect := startW.Header().Get("Location")
	redirectURL, err := url.Parse(providerRedirect)
	testutil.NoError(t, err)
	state := redirectURL.Query().Get("state")
	testutil.True(t, state != "", "OAuth start redirect must include state")

	callbackReq := httptest.NewRequest(http.MethodGet, "/oauth/google/callback?code=test-code&state="+url.QueryEscape(state), nil)
	callbackReq.Host = "localhost:8090"
	callbackW := httptest.NewRecorder()
	router.ServeHTTP(callbackW, callbackReq)

	testutil.Equal(t, http.StatusTemporaryRedirect, callbackW.Code)
	testutil.Equal(t, "https://app.example.com/workspace?tab=members#refreshToken=ayb-refresh&token=ayb-access", callbackW.Header().Get("Location"))

	// Baseline invariant: OAuth state remains one-time use in this stage.
	replayReq := httptest.NewRequest(http.MethodGet, "/oauth/google/callback?code=test-code&state="+url.QueryEscape(state), nil)
	replayReq.Host = "localhost:8090"
	replayW := httptest.NewRecorder()
	router.ServeHTTP(replayW, replayReq)
	testutil.Equal(t, http.StatusBadRequest, replayW.Code)
	testutil.Contains(t, replayW.Body.String(), "invalid or expired OAuth state")
}

func TestOAuthReturnToCallbackFallsBackToConfiguredRedirectWhenReturnTargetAbsent(t *testing.T) {
	t.Parallel()

	h := newOAuthReturnToSuccessHandler(t)
	h.SetOAuthRedirectURL("https://app.example.com/auth/callback")
	router := h.Routes()

	startReq := httptest.NewRequest(http.MethodGet, "/oauth/google", nil)
	startReq.Host = "localhost:8090"
	startW := httptest.NewRecorder()
	router.ServeHTTP(startW, startReq)
	testutil.Equal(t, http.StatusTemporaryRedirect, startW.Code)

	providerRedirect := startW.Header().Get("Location")
	redirectURL, err := url.Parse(providerRedirect)
	testutil.NoError(t, err)
	state := redirectURL.Query().Get("state")
	testutil.True(t, state != "", "OAuth start redirect must include state")

	callbackReq := httptest.NewRequest(http.MethodGet, "/oauth/google/callback?code=test-code&state="+url.QueryEscape(state), nil)
	callbackReq.Host = "localhost:8090"
	callbackW := httptest.NewRecorder()
	router.ServeHTTP(callbackW, callbackReq)

	// Baseline invariant for this stage: when no per-request target exists,
	// callback still uses configured OAuth redirect URL.
	testutil.Equal(t, http.StatusTemporaryRedirect, callbackW.Code)
	testutil.Equal(t, "https://app.example.com/auth/callback#refreshToken=ayb-refresh&token=ayb-access", callbackW.Header().Get("Location"))
}

func TestOAuthRedirect_OpenRedirectAttackerRejected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		redirectTo string
	}{
		{
			name:       "off allowlist host",
			redirectTo: "https://evil.example.com/phish",
		},
		{
			name:       "malformed URL",
			redirectTo: "https://app.example.com/%zz",
		},
		{
			name:       "unsafe scheme",
			redirectTo: "javascript:alert(1)",
		},
		{
			name:       "userinfo URL",
			redirectTo: "https://user@app.example.com/steal",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc := newTestService()
			h := NewHandler(svc, testutil.DiscardLogger())
			h.SetOAuthProvider("google", OAuthClientConfig{ClientID: "test-id", ClientSecret: "test-secret"})
			h.SetOAuthRedirectURL("https://app.example.com/auth/callback")
			router := h.Routes()

			req := httptest.NewRequest(http.MethodGet, "/oauth/google?redirect_to="+url.QueryEscape(tt.redirectTo), nil)
			req.Host = "localhost:8090"
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			testutil.Equal(t, http.StatusBadRequest, w.Code)
			testutil.Contains(t, w.Body.String(), "invalid redirect_to")
			location := w.Header().Get("Location")
			testutil.Equal(t, "", location)
			testutil.False(t, strings.Contains(strings.ToLower(location), "github"), "rejected request must not leak provider redirect")
			// Contract proof: no provider redirect side effect (state must not be generated).
			testutil.Equal(t, 0, len(h.oauthStateStore.states))
		})
	}
}

func TestOAuthCallback_RevalidatesStateBoundReturnToAndFallsBackWhenTampered(t *testing.T) {
	t.Parallel()

	h := newOAuthReturnToSuccessHandler(t)
	h.SetOAuthRedirectURL("https://app.example.com/auth/callback")
	router := h.Routes()

	startReq := httptest.NewRequest(http.MethodGet, "/oauth/google?redirect_to="+url.QueryEscape("/workspace?tab=members"), nil)
	startReq.Host = "localhost:8090"
	startW := httptest.NewRecorder()
	router.ServeHTTP(startW, startReq)
	testutil.Equal(t, http.StatusTemporaryRedirect, startW.Code)

	redirectURL, err := url.Parse(startW.Header().Get("Location"))
	testutil.NoError(t, err)
	state := redirectURL.Query().Get("state")
	testutil.True(t, state != "", "OAuth start redirect must include state")

	// Simulate state-store tampering between OAuth start and callback.
	h.oauthStateStore.mu.Lock()
	h.oauthStateStore.states[state] = oauthStateEntry{
		expiresAt: time.Now().Add(1 * time.Minute),
		returnTo:  "https://evil.example.com/phish",
	}
	h.oauthStateStore.mu.Unlock()

	callbackReq := httptest.NewRequest(http.MethodGet, "/oauth/google/callback?code=test-code&state="+url.QueryEscape(state)+"&redirect_to="+url.QueryEscape("https://evil.example.com/query"), nil)
	callbackReq.Host = "localhost:8090"
	callbackW := httptest.NewRecorder()
	router.ServeHTTP(callbackW, callbackReq)

	testutil.Equal(t, http.StatusTemporaryRedirect, callbackW.Code)
	testutil.Equal(t, "https://app.example.com/auth/callback#refreshToken=ayb-refresh&token=ayb-access", callbackW.Header().Get("Location"))
	testutil.False(t, strings.Contains(callbackW.Header().Get("Location"), "evil.example.com"), "callback must not redirect to attacker origin")

	// One-time state remains load-bearing for callback security.
	replayReq := httptest.NewRequest(http.MethodGet, "/oauth/google/callback?code=test-code&state="+url.QueryEscape(state), nil)
	replayReq.Host = "localhost:8090"
	replayW := httptest.NewRecorder()
	router.ServeHTTP(replayW, replayReq)
	testutil.Equal(t, http.StatusBadRequest, replayW.Code)
	testutil.Contains(t, replayW.Body.String(), "invalid or expired OAuth state")
}

func TestOAuthCallback_RevalidatesStoredReturnToWhenRedirectBaseChanges(t *testing.T) {
	t.Parallel()

	h := newOAuthReturnToSuccessHandler(t)
	h.SetOAuthRedirectURL("https://app.example.com/auth/callback")
	router := h.Routes()

	startReq := httptest.NewRequest(http.MethodGet, "/oauth/google?redirect_to="+url.QueryEscape("/workspace?tab=members"), nil)
	startReq.Host = "localhost:8090"
	startW := httptest.NewRecorder()
	router.ServeHTTP(startW, startReq)
	testutil.Equal(t, http.StatusTemporaryRedirect, startW.Code)

	redirectURL, err := url.Parse(startW.Header().Get("Location"))
	testutil.NoError(t, err)
	state := redirectURL.Query().Get("state")
	testutil.True(t, state != "", "OAuth start redirect must include state")

	// Rebind allowlist/base between start and callback: stored returnTo must be
	// re-validated against the current callback policy.
	h.SetOAuthRedirectURL("https://new.example.com/auth/callback")

	callbackReq := httptest.NewRequest(http.MethodGet, "/oauth/google/callback?code=test-code&state="+url.QueryEscape(state), nil)
	callbackReq.Host = "localhost:8090"
	callbackW := httptest.NewRecorder()
	router.ServeHTTP(callbackW, callbackReq)

	testutil.Equal(t, http.StatusTemporaryRedirect, callbackW.Code)
	testutil.Equal(t, "https://new.example.com/auth/callback#refreshToken=ayb-refresh&token=ayb-access", callbackW.Header().Get("Location"))
}

func TestOAuthReturnToBaselineSSEFlowRemainsPopupClosePage(t *testing.T) {
	t.Parallel()

	h := newOAuthReturnToSuccessHandler(t)
	h.SetOAuthRedirectURL("https://app.example.com/auth/callback")

	pub := newFakeOAuthPublisher()
	pub.clients["sse-return-to"] = true
	h.SetOAuthPublisher(pub)
	h.oauthStateStore.RegisterExternalState("sse-return-to")

	req := httptest.NewRequest(http.MethodGet, "/oauth/google/callback?code=test-code&state=sse-return-to", nil)
	req.Host = "localhost:8090"
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	// Baseline invariant: SSE callback flow remains popup-close HTML, not HTTP redirect.
	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Contains(t, w.Body.String(), "Authentication complete")
	testutil.Contains(t, w.Body.String(), "window.close()")
	testutil.Equal(t, "", w.Header().Get("Location"))
	testutil.SliceLen(t, pub.published, 1)
	testutil.Equal(t, "sse-return-to", pub.lastTarget)
}

func newOAuthReturnToSuccessHandler(t *testing.T) *Handler {
	t.Helper()

	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetOAuthProvider("google", OAuthClientConfig{ClientID: "id", ClientSecret: "secret"})
	h.SetProviderURLs("google", OAuthProviderConfig{
		TokenURL:    "https://provider.test/token",
		UserInfoURL: "https://provider.test/userinfo",
	})
	h.oauthLoginFn = func(_ context.Context, _ string, info *OAuthUserInfo) (*User, string, string, error) {
		return &User{ID: "user-1", Email: info.Email}, "ayb-access", "ayb-refresh", nil
	}
	h.oauthHTTPClient = &http.Client{
		Transport: oauthRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.Path {
			case "/token":
				return oauthJSONResponse(http.StatusOK, `{
					"access_token":"provider-access",
					"refresh_token":"provider-refresh",
					"token_type":"Bearer"
				}`), nil
			case "/userinfo":
				return oauthJSONResponse(http.StatusOK, `{
					"id":"provider-user-1",
					"email":"returnto@example.com",
					"name":"Return To User"
				}`), nil
			default:
				return oauthJSONResponse(http.StatusNotFound, `{"error":"not found"}`), nil
			}
		}),
	}
	return h
}
