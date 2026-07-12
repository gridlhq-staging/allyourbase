package auth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestOAuthStartContractProbe(t *testing.T) {
	t.Parallel()

	t.Run("redirects to provider with generated server state", func(t *testing.T) {
		t.Parallel()

		h := newOAuthStartContractHandler(t)
		router := h.Routes()

		req := httptest.NewRequest(http.MethodGet, "/oauth/github?state=fixture-state", nil)
		req.Host = "api.example.com"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusTemporaryRedirect, w.Code)
		location := w.Header().Get("Location")
		testutil.True(t, strings.HasPrefix(location, "https://github.com/login/oauth/authorize?"), "OAuth start must redirect to GitHub authorize endpoint")

		providerURL, err := url.Parse(location)
		testutil.NoError(t, err)
		testutil.Equal(t, "https", providerURL.Scheme)
		testutil.Equal(t, "github.com", providerURL.Host)
		testutil.Equal(t, "/login/oauth/authorize", providerURL.Path)

		query := providerURL.Query()
		state := query.Get("state")
		testutil.True(t, state != "", "provider state must be generated")
		testutil.True(t, state != "fixture-state", "provider state must not reuse client supplied fixture state")
		testutil.Equal(t, "github-client-id", query.Get("client_id"))
		testutil.Equal(t, "http://api.example.com/api/auth/oauth/github/callback", query.Get("redirect_uri"))
		testutil.Equal(t, "code", query.Get("response_type"))
		testutil.Equal(t, "user:email", query.Get("scope"))

		h.oauthStateStore.mu.Lock()
		entry, ok := h.oauthStateStore.states[state]
		stateCount := len(h.oauthStateStore.states)
		h.oauthStateStore.mu.Unlock()
		testutil.True(t, ok, "generated provider state must be stored")
		testutil.Equal(t, 1, stateCount)
		testutil.Equal(t, "", entry.returnTo)
	})

	t.Run("rejects invalid redirect_to before state generation", func(t *testing.T) {
		t.Parallel()

		h := newOAuthStartContractHandler(t)
		router := h.Routes()

		req := httptest.NewRequest(
			http.MethodGet,
			"/oauth/github?state=fixture-state&redirect_to=https%3A%2F%2Fevil.example.com%2Fphish",
			nil,
		)
		req.Host = "api.example.com"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		testutil.Equal(t, http.StatusBadRequest, w.Code)
		testutil.Contains(t, w.Body.String(), "invalid redirect_to")
		testutil.Equal(t, "", w.Header().Get("Location"))

		h.oauthStateStore.mu.Lock()
		stateCount := len(h.oauthStateStore.states)
		h.oauthStateStore.mu.Unlock()
		testutil.Equal(t, 0, stateCount)
	})
}

func newOAuthStartContractHandler(t *testing.T) *Handler {
	t.Helper()

	h := NewHandler(newTestService(), testutil.DiscardLogger())
	h.SetOAuthProvider("github", OAuthClientConfig{
		ClientID:     "github-client-id",
		ClientSecret: "github-client-secret",
	})
	h.SetOAuthRedirectURL("https://app.example.com/auth/callback")
	return h
}
