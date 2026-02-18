package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/testutil"
)

func newTestService() *Service {
	return &Service{
		jwtSecret:  []byte(testSecret),
		tokenDur:   time.Hour,
		refreshDur: 7 * 24 * time.Hour,
		minPwLen:   8,
	}
}

func generateTestToken(t *testing.T, svc *Service, userID, email string) string {
	t.Helper()
	user := &User{ID: userID, Email: email}
	token, err := svc.generateToken(user)
	if err != nil {
		t.Fatalf("generating test token: %v", err)
	}
	return token
}

func TestRequireAuthValidToken(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	token := generateTestToken(t, svc, "user-1", "test@example.com")

	var gotClaims *Claims
	handler := RequireAuth(svc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClaims = ClaimsFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.NotNil(t, gotClaims)
	testutil.Equal(t, "user-1", gotClaims.Subject)
	testutil.Equal(t, "test@example.com", gotClaims.Email)
}

func TestRequireAuthMissingHeader(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	called := false
	handler := RequireAuth(svc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusUnauthorized, w.Code)
	testutil.False(t, called, "next handler should not be called")
}

func TestRequireAuthMalformedHeader(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	handler := RequireAuth(svc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// No "Bearer " prefix.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Token abc123")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequireAuthExpiredToken(t *testing.T) {
	t.Parallel()
	svc := &Service{
		jwtSecret: []byte(testSecret),
		tokenDur:  -time.Hour,
	}
	token := generateTestToken(t, svc, "user-1", "test@example.com")

	validSvc := newTestService()
	handler := RequireAuth(validSvc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestOptionalAuthNoHeader(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	var gotClaims *Claims
	handler := OptionalAuth(svc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClaims = ClaimsFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Nil(t, gotClaims)
}

func TestOptionalAuthInvalidToken(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	var gotClaims *Claims
	handler := OptionalAuth(svc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClaims = ClaimsFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	// Invalid token should be silently ignored, not rejected.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer invalid-garbage-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Nil(t, gotClaims)
}

func TestOptionalAuthValidToken(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	token := generateTestToken(t, svc, "user-2", "other@example.com")

	var gotClaims *Claims
	handler := OptionalAuth(svc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClaims = ClaimsFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.NotNil(t, gotClaims)
	testutil.Equal(t, "user-2", gotClaims.Subject)
}

func TestClaimsFromContextNil(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	claims := ClaimsFromContext(req.Context())
	testutil.Nil(t, claims)
}

func TestRequireAuthMissingHeaderDocURL(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	handler := RequireAuth(svc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	var resp httputil.ErrorResponse
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	testutil.Equal(t, "https://allyourbase.io/guide/authentication", resp.DocURL)
}

func TestRequireAuthExpiredTokenDocURL(t *testing.T) {
	t.Parallel()
	svc := &Service{
		jwtSecret: []byte(testSecret),
		tokenDur:  -time.Hour,
	}
	token := generateTestToken(t, svc, "user-1", "test@example.com")

	validSvc := newTestService()
	handler := RequireAuth(validSvc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	var resp httputil.ErrorResponse
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	testutil.Equal(t, "https://allyourbase.io/guide/authentication", resp.DocURL)
}
