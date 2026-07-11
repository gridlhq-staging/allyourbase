package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestOAuthCallbackPublishesViaSSEWhenHasClientReturnsFalse(t *testing.T) {
	t.Parallel()

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
					"email":"crossnode@example.com",
					"name":"Cross Node User"
				}`), nil
			default:
				return oauthJSONResponse(http.StatusNotFound, `{"error":"not found"}`), nil
			}
		}),
	}

	pub := newFakeOAuthPublisher()
	// Deliberately do NOT register the client in pub.clients — HasClient returns false.
	h.SetOAuthPublisher(pub)

	state, err := h.oauthStateStore.GenerateWithReturnToAndSSEClient("", "remote-sse-client")
	testutil.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/oauth/google/callback?code=test-code&state="+url.QueryEscape(state), nil)
	req.Host = "localhost:8090"
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Contains(t, w.Body.String(), "Authentication complete")

	testutil.SliceLen(t, pub.published, 1)
	testutil.Equal(t, "remote-sse-client", pub.lastTarget)
	testutil.Equal(t, "ayb-access", pub.published[0].Token)
	testutil.Equal(t, "ayb-refresh", pub.published[0].RefreshToken)
}

func TestOAuthCallbackNonPopupFlowIgnoresSSE(t *testing.T) {
	t.Parallel()

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
					"token_type":"Bearer"
				}`), nil
			case "/userinfo":
				return oauthJSONResponse(http.StatusOK, `{
					"id":"provider-user-1",
					"email":"regular@example.com",
					"name":"Regular User"
				}`), nil
			default:
				return oauthJSONResponse(http.StatusNotFound, `{"error":"not found"}`), nil
			}
		}),
	}

	pub := newFakeOAuthPublisher()
	h.SetOAuthPublisher(pub)

	state, err := h.oauthStateStore.GenerateWithReturnToAndSSEClient("", "")
	testutil.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/oauth/google/callback?code=test-code&state="+url.QueryEscape(state), nil)
	req.Host = "localhost:8090"
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	// Without sseClientID, callback should return JSON, not the close page.
	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Contains(t, w.Body.String(), "ayb-access")
	testutil.SliceLen(t, pub.published, 0)
}
