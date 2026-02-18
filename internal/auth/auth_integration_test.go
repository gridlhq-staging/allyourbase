//go:build integration

package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/mailer"
	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/testutil"
)

var sharedPG *testutil.PGContainer

func TestMain(m *testing.M) {
	ctx := context.Background()
	pg, cleanup := testutil.StartPostgresForTestMain(ctx)
	sharedPG = pg
	code := m.Run()
	cleanup()
	os.Exit(code)
}

const testJWTSecret = "integration-test-secret-that-is-at-least-32-chars!!"

func resetAndMigrate(t *testing.T, ctx context.Context) {
	t.Helper()
	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	if err != nil {
		t.Fatalf("resetting schema: %v", err)
	}

	logger := testutil.DiscardLogger()
	runner := migrations.NewRunner(sharedPG.Pool, logger)
	if err := runner.Bootstrap(ctx); err != nil {
		t.Fatalf("bootstrapping migrations: %v", err)
	}
	if _, err := runner.Run(ctx); err != nil {
		t.Fatalf("running migrations: %v", err)
	}
}

func newAuthService() *auth.Service {
	return auth.NewService(sharedPG.Pool, testJWTSecret, time.Hour, 7*24*time.Hour, 8, testutil.DiscardLogger())
}

func setupAuthServer(t *testing.T, ctx context.Context) *server.Server {
	t.Helper()
	resetAndMigrate(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	if err := ch.Load(ctx); err != nil {
		t.Fatalf("loading schema cache: %v", err)
	}

	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = testJWTSecret

	authSvc := newAuthService()
	return server.New(cfg, logger, ch, sharedPG.Pool, authSvc, nil)
}

func doJSON(t *testing.T, srv *server.Server, method, path string, body any, token string) *httptest.ResponseRecorder {
	t.Helper()
	var reqBody *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("doJSON: marshal body: %v", err)
		}
		reqBody = bytes.NewReader(b)
	} else {
		reqBody = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reqBody)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	return w
}

type authResp struct {
	Token        string         `json:"token"`
	RefreshToken string         `json:"refreshToken"`
	User         map[string]any `json:"user"`
}

func parseAuthResp(t *testing.T, w *httptest.ResponseRecorder) authResp {
	t.Helper()
	var resp authResp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parsing auth response: %v (body: %s)", err, w.Body.String())
	}
	return resp
}

// --- Registration tests ---

func TestRegisterSuccess(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	w := doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "alice@example.com", "password": "password123",
	}, "")

	testutil.StatusCode(t, http.StatusCreated, w.Code)

	resp := parseAuthResp(t, w)
	testutil.True(t, resp.Token != "", "should return a token")
	testutil.True(t, resp.RefreshToken != "", "should return a refresh token")
	testutil.Equal(t, "alice@example.com", resp.User["email"].(string))
	testutil.True(t, resp.User["id"].(string) != "", "should have user id")
}

func TestRegisterDuplicateEmail(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	body := map[string]string{"email": "dup@example.com", "password": "password123"}
	w := doJSON(t, srv, "POST", "/api/auth/register", body, "")
	testutil.StatusCode(t, http.StatusCreated, w.Code)

	// Same email again.
	w = doJSON(t, srv, "POST", "/api/auth/register", body, "")
	testutil.StatusCode(t, http.StatusConflict, w.Code)
}

func TestRegisterDuplicateEmailCaseInsensitive(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	w := doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "User@Example.com", "password": "password123",
	}, "")
	testutil.StatusCode(t, http.StatusCreated, w.Code)

	// Same email, different case.
	w = doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "user@example.com", "password": "password123",
	}, "")
	testutil.StatusCode(t, http.StatusConflict, w.Code)
}

// --- Login tests ---

func TestLoginSuccess(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	// Register first.
	doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "login@example.com", "password": "password123",
	}, "")

	// Login.
	w := doJSON(t, srv, "POST", "/api/auth/login", map[string]string{
		"email": "login@example.com", "password": "password123",
	}, "")
	testutil.StatusCode(t, http.StatusOK, w.Code)

	resp := parseAuthResp(t, w)
	testutil.True(t, resp.Token != "", "should return a token")
	testutil.True(t, resp.RefreshToken != "", "should return a refresh token")
	testutil.Equal(t, "login@example.com", resp.User["email"].(string))
}

func TestLoginWrongPassword(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "wrong@example.com", "password": "password123",
	}, "")

	w := doJSON(t, srv, "POST", "/api/auth/login", map[string]string{
		"email": "wrong@example.com", "password": "wrongpassword",
	}, "")
	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)
}

func TestLoginNonexistentEmail(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	w := doJSON(t, srv, "POST", "/api/auth/login", map[string]string{
		"email": "noone@example.com", "password": "password123",
	}, "")
	// Same status as wrong password — no enumeration.
	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)
}

// --- /me endpoint tests ---

func TestMeWithRegisterToken(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	w := doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "me@example.com", "password": "password123",
	}, "")
	resp := parseAuthResp(t, w)

	w = doJSON(t, srv, "GET", "/api/auth/me", nil, resp.Token)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	var user map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &user); err != nil {
		t.Fatalf("parsing /me response: %v (body: %s)", err, w.Body.String())
	}
	testutil.Equal(t, "me@example.com", user["email"].(string))
}

func TestMeWithLoginToken(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "melogin@example.com", "password": "password123",
	}, "")

	w := doJSON(t, srv, "POST", "/api/auth/login", map[string]string{
		"email": "melogin@example.com", "password": "password123",
	}, "")
	resp := parseAuthResp(t, w)

	w = doJSON(t, srv, "GET", "/api/auth/me", nil, resp.Token)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	var user map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &user); err != nil {
		t.Fatalf("parsing /me response: %v (body: %s)", err, w.Body.String())
	}
	testutil.Equal(t, "melogin@example.com", user["email"].(string))
}

func TestMeWithoutToken(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	w := doJSON(t, srv, "GET", "/api/auth/me", nil, "")
	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)
}

// --- Protected collection endpoints ---

func TestCollectionEndpointRequiresAuth(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	// Create a test table.
	_, err := sharedPG.Pool.Exec(ctx, `
		CREATE TABLE posts (
			id SERIAL PRIMARY KEY,
			title TEXT NOT NULL
		)
	`)
	testutil.NoError(t, err)

	// Reload schema.
	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = testJWTSecret
	authSvc := newAuthService()
	srv = server.New(cfg, logger, ch, sharedPG.Pool, authSvc, nil)

	// Without token → 401.
	w := doJSON(t, srv, "GET", "/api/collections/posts/", nil, "")
	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)

	// Register and get token.
	w = doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "authed@example.com", "password": "password123",
	}, "")
	resp := parseAuthResp(t, w)

	// With token → 200.
	w = doJSON(t, srv, "GET", "/api/collections/posts/", nil, resp.Token)
	testutil.StatusCode(t, http.StatusOK, w.Code)
}

// --- RLS enforcement ---

func TestRLSEnforcement(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	// Create a table with RLS.
	_, err := sharedPG.Pool.Exec(ctx, `
		CREATE TABLE notes (
			id SERIAL PRIMARY KEY,
			owner_id TEXT NOT NULL,
			content TEXT NOT NULL
		);
		ALTER TABLE notes ENABLE ROW LEVEL SECURITY;
		ALTER TABLE notes FORCE ROW LEVEL SECURITY;
		CREATE POLICY notes_owner ON notes
			USING (owner_id = current_setting('ayb.user_id', true));
	`)
	testutil.NoError(t, err)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = testJWTSecret
	authSvc := newAuthService()
	srv := server.New(cfg, logger, ch, sharedPG.Pool, authSvc, nil)

	// Register two users.
	w := doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "user1@example.com", "password": "password123",
	}, "")
	user1 := parseAuthResp(t, w)

	w = doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "user2@example.com", "password": "password123",
	}, "")
	user2 := parseAuthResp(t, w)

	user1ID := user1.User["id"].(string)
	user2ID := user2.User["id"].(string)

	// Insert notes owned by each user (bypass RLS with superuser pool).
	_, err = sharedPG.Pool.Exec(ctx,
		"INSERT INTO notes (owner_id, content) VALUES ($1, 'user1 note'), ($2, 'user2 note')",
		user1ID, user2ID)
	testutil.NoError(t, err)

	// User 1 should only see their note.
	w = doJSON(t, srv, "GET", "/api/collections/notes/", nil, user1.Token)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	var list1 struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &list1); err != nil {
		t.Fatalf("parsing user1 notes response: %v (body: %s)", err, w.Body.String())
	}
	testutil.Equal(t, 1, len(list1.Items))
	testutil.Equal(t, "user1 note", list1.Items[0]["content"])

	// User 2 should only see their note.
	w = doJSON(t, srv, "GET", "/api/collections/notes/", nil, user2.Token)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	var list2 struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &list2); err != nil {
		t.Fatalf("parsing user2 notes response: %v (body: %s)", err, w.Body.String())
	}
	testutil.Equal(t, 1, len(list2.Items))
	testutil.Equal(t, "user2 note", list2.Items[0]["content"])
}

// --- Refresh token tests ---

func setupAuthServerWithRefreshDur(t *testing.T, ctx context.Context, refreshDur time.Duration) *server.Server {
	t.Helper()
	resetAndMigrate(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	if err := ch.Load(ctx); err != nil {
		t.Fatalf("loading schema cache: %v", err)
	}

	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = testJWTSecret

	authSvc := auth.NewService(sharedPG.Pool, testJWTSecret, time.Hour, refreshDur, 8, logger)
	return server.New(cfg, logger, ch, sharedPG.Pool, authSvc, nil)
}

func TestRefreshTokenFlow(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	// Register.
	w := doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "refresh@example.com", "password": "password123",
	}, "")
	testutil.StatusCode(t, http.StatusCreated, w.Code)
	resp := parseAuthResp(t, w)
	testutil.True(t, resp.RefreshToken != "", "should return refresh token")

	// Use refresh token to get new tokens.
	w = doJSON(t, srv, "POST", "/api/auth/refresh", map[string]string{
		"refreshToken": resp.RefreshToken,
	}, "")
	testutil.StatusCode(t, http.StatusOK, w.Code)
	refreshResp := parseAuthResp(t, w)
	testutil.True(t, refreshResp.Token != "", "should return new access token")
	testutil.True(t, refreshResp.RefreshToken != "", "should return new refresh token")

	// Verify the new access token works on /me.
	w = doJSON(t, srv, "GET", "/api/auth/me", nil, refreshResp.Token)
	testutil.StatusCode(t, http.StatusOK, w.Code)
}

func TestRefreshTokenExpired(t *testing.T) {
	ctx := context.Background()
	// Use a 1ms refresh duration so it expires immediately.
	srv := setupAuthServerWithRefreshDur(t, ctx, time.Millisecond)

	w := doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "expired@example.com", "password": "password123",
	}, "")
	resp := parseAuthResp(t, w)

	// Wait for the refresh token to expire.
	time.Sleep(50 * time.Millisecond)

	w = doJSON(t, srv, "POST", "/api/auth/refresh", map[string]string{
		"refreshToken": resp.RefreshToken,
	}, "")
	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)
}

func TestLogout(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	w := doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "logout@example.com", "password": "password123",
	}, "")
	resp := parseAuthResp(t, w)

	// Logout.
	w = doJSON(t, srv, "POST", "/api/auth/logout", map[string]string{
		"refreshToken": resp.RefreshToken,
	}, "")
	testutil.StatusCode(t, http.StatusNoContent, w.Code)

	// Refresh with the logged-out token should fail.
	w = doJSON(t, srv, "POST", "/api/auth/refresh", map[string]string{
		"refreshToken": resp.RefreshToken,
	}, "")
	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)
}

// --- OAuth integration tests ---

func TestOAuthLoginNewUser(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	svc := newAuthService()
	info := &auth.OAuthUserInfo{
		ProviderUserID: "google-123",
		Email:          "oauth@example.com",
		Name:           "OAuth User",
	}

	user, token, refreshToken, err := svc.OAuthLogin(ctx, "google", info)
	testutil.NoError(t, err)
	testutil.True(t, user.ID != "", "should create user")
	testutil.Equal(t, "oauth@example.com", user.Email)
	testutil.True(t, token != "", "should return access token")
	testutil.True(t, refreshToken != "", "should return refresh token")

	// Verify the access token works.
	claims, err := svc.ValidateToken(token)
	testutil.NoError(t, err)
	testutil.Equal(t, user.ID, claims.Subject)
}

func TestOAuthLoginExistingIdentity(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	svc := newAuthService()
	info := &auth.OAuthUserInfo{
		ProviderUserID: "google-456",
		Email:          "repeat@example.com",
		Name:           "Repeat User",
	}

	// First login creates user.
	user1, _, _, err := svc.OAuthLogin(ctx, "google", info)
	testutil.NoError(t, err)

	// Second login with same provider identity returns same user.
	user2, _, _, err := svc.OAuthLogin(ctx, "google", info)
	testutil.NoError(t, err)
	testutil.Equal(t, user1.ID, user2.ID)
}

func TestOAuthLoginLinksToExistingEmailUser(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	svc := newAuthService()

	// Register a user with email/password first.
	emailUser, _, _, err := svc.Register(ctx, "linked@example.com", "password123")
	testutil.NoError(t, err)

	// Login via OAuth with the same email.
	info := &auth.OAuthUserInfo{
		ProviderUserID: "github-789",
		Email:          "linked@example.com",
		Name:           "Linked User",
	}
	oauthUser, _, _, err := svc.OAuthLogin(ctx, "github", info)
	testutil.NoError(t, err)

	// Should be the same user (linked, not a new account).
	testutil.Equal(t, emailUser.ID, oauthUser.ID)
}

func TestOAuthLoginMultipleProvidersSameUser(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	svc := newAuthService()

	// Login via Google.
	googleInfo := &auth.OAuthUserInfo{
		ProviderUserID: "google-multi",
		Email:          "multi@example.com",
		Name:           "Multi User",
	}
	user1, _, _, err := svc.OAuthLogin(ctx, "google", googleInfo)
	testutil.NoError(t, err)

	// Login via GitHub with same email.
	githubInfo := &auth.OAuthUserInfo{
		ProviderUserID: "github-multi",
		Email:          "multi@example.com",
		Name:           "Multi User",
	}
	user2, _, _, err := svc.OAuthLogin(ctx, "github", githubInfo)
	testutil.NoError(t, err)

	// Should be the same user.
	testutil.Equal(t, user1.ID, user2.ID)
}

func TestOAuthLoginNoEmail(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	svc := newAuthService()
	info := &auth.OAuthUserInfo{
		ProviderUserID: "github-noemail",
		Email:          "",
		Name:           "No Email User",
	}

	user, _, _, err := svc.OAuthLogin(ctx, "github", info)
	testutil.NoError(t, err)
	testutil.True(t, user.ID != "", "should create user even without email")
	// Should have a placeholder email.
	testutil.True(t, user.Email != "", "should have placeholder email")
}

func TestOAuthHandlerFullFlowMocked(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	// Set up fake OAuth provider endpoints.
	fakeProvider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			if err := json.NewEncoder(w).Encode(map[string]string{
				"access_token": "fake-access-token",
			}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		case "/userinfo":
			if err := json.NewEncoder(w).Encode(map[string]any{
				"id":    "12345",
				"email": "fakeuser@example.com",
				"name":  "Fake User",
			}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer fakeProvider.Close()

	// Override Google's endpoints to point to our fake server.
	auth.SetProviderURLs("google", auth.OAuthProviderConfig{
		AuthURL:     "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL:    fakeProvider.URL + "/token",
		UserInfoURL: fakeProvider.URL + "/userinfo",
		Scopes:      []string{"openid", "email", "profile"},
	})
	defer auth.ResetProviderURLs("google")

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = testJWTSecret
	cfg.Auth.OAuth = map[string]config.OAuthProvider{
		"google": {Enabled: true, ClientID: "test-id", ClientSecret: "test-secret"},
	}
	cfg.Auth.OAuthRedirectURL = "http://localhost:5173/callback"

	svc := newAuthService()
	srv := server.New(cfg, logger, ch, sharedPG.Pool, svc, nil)

	// Step 1: Initiate OAuth → should redirect to Google.
	req := httptest.NewRequest(http.MethodGet, "/api/auth/oauth/google", nil)
	req.Host = "localhost:8090"
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	testutil.StatusCode(t, http.StatusTemporaryRedirect, w.Code)
	loc := w.Header().Get("Location")
	testutil.True(t, loc != "", "should redirect")

	// Extract state from the redirect URL.
	var state string
	if idx := len("state="); true {
		for _, part := range splitQuery(loc) {
			if len(part) > idx && part[:idx] == "state=" {
				state = part[idx:]
				break
			}
		}
	}
	testutil.True(t, state != "", "redirect should include state")

	// Step 2: Simulate callback from provider.
	callbackURL := fmt.Sprintf("/api/auth/oauth/google/callback?code=test-code&state=%s", state)
	req = httptest.NewRequest(http.MethodGet, callbackURL, nil)
	req.Host = "localhost:8090"
	w = httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	// Should redirect to the configured redirect URL with tokens.
	testutil.StatusCode(t, http.StatusTemporaryRedirect, w.Code)
	redirectLoc := w.Header().Get("Location")
	testutil.True(t, redirectLoc != "", "should redirect with tokens")
	testutil.True(t, len(redirectLoc) > len("http://localhost:5173/callback#"), "redirect should have fragment")

	// Verify the user was created.
	var count int
	err := sharedPG.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM _ayb_users WHERE email = 'fakeuser@example.com'",
	).Scan(&count)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, count)

	// Verify the OAuth account was linked.
	err = sharedPG.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM _ayb_oauth_accounts WHERE provider = 'google' AND provider_user_id = '12345'",
	).Scan(&count)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, count)
}

// splitQuery splits a URL's query string into key=value pairs.
func splitQuery(rawURL string) []string {
	idx := 0
	for i, c := range rawURL {
		if c == '?' {
			idx = i + 1
			break
		}
	}
	if idx == 0 {
		return nil
	}
	query := rawURL[idx:]
	var parts []string
	for _, p := range splitOn(query, '&') {
		parts = append(parts, p)
	}
	return parts
}

func splitOn(s string, sep byte) []string {
	var result []string
	start := 0
	for i := range len(s) {
		if s[i] == sep {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}

// --- Refresh token rotation tests ---

func TestRefreshTokenRotation(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	// Register and get initial tokens.
	w := doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "refresh@example.com", "password": "password123",
	}, "")
	testutil.StatusCode(t, http.StatusCreated, w.Code)
	resp1 := parseAuthResp(t, w)
	oldRefreshToken := resp1.RefreshToken

	// Use refresh token to get new tokens.
	w = doJSON(t, srv, "POST", "/api/auth/refresh", map[string]string{
		"refreshToken": oldRefreshToken,
	}, "")
	testutil.StatusCode(t, http.StatusOK, w.Code)
	resp2 := parseAuthResp(t, w)

	// Verify new tokens are different.
	testutil.NotEqual(t, resp1.Token, resp2.Token)
	testutil.NotEqual(t, resp1.RefreshToken, resp2.RefreshToken)

	// Old refresh token should no longer work (rotation invalidates it).
	w = doJSON(t, srv, "POST", "/api/auth/refresh", map[string]string{
		"refreshToken": oldRefreshToken,
	}, "")
	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)

	// New refresh token should work.
	w = doJSON(t, srv, "POST", "/api/auth/refresh", map[string]string{
		"refreshToken": resp2.RefreshToken,
	}, "")
	testutil.StatusCode(t, http.StatusOK, w.Code)
}

func TestRefreshTokenCanOnlyBeUsedOnce(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	// Register.
	w := doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "once@example.com", "password": "password123",
	}, "")
	resp := parseAuthResp(t, w)
	refreshToken := resp.RefreshToken

	// First refresh succeeds.
	w = doJSON(t, srv, "POST", "/api/auth/refresh", map[string]string{
		"refreshToken": refreshToken,
	}, "")
	testutil.StatusCode(t, http.StatusOK, w.Code)

	// Second use of same token fails.
	w = doJSON(t, srv, "POST", "/api/auth/refresh", map[string]string{
		"refreshToken": refreshToken,
	}, "")
	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)
}

func TestRefreshTokenRejectedAfterExpiry(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	// Create auth service with very short refresh token expiry (1 second).
	authSvc := auth.NewService(sharedPG.Pool, testJWTSecret, time.Hour, 1*time.Second, 8, testutil.DiscardLogger())

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = testJWTSecret
	srv := server.New(cfg, logger, ch, sharedPG.Pool, authSvc, nil)

	// Register.
	w := doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "expiry@example.com", "password": "password123",
	}, "")
	resp := parseAuthResp(t, w)

	// Wait for refresh token to expire.
	time.Sleep(1200 * time.Millisecond)

	// Refresh should fail.
	w = doJSON(t, srv, "POST", "/api/auth/refresh", map[string]string{
		"refreshToken": resp.RefreshToken,
	}, "")
	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)
}

// --- Verification token tests ---

func TestVerificationTokenReuse(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	authSvc := newAuthService()

	// Create a user.
	user, err := auth.CreateUser(ctx, sharedPG.Pool, "verify@example.com", "password123", 8)
	testutil.NoError(t, err)

	// Manually insert a verification token (simulating SendVerificationEmail).
	token := "test-verification-token-12345"
	hash := auth.HashTokenForTest(token)
	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_email_verifications (user_id, token_hash, expires_at)
		 VALUES ($1, $2, $3)`,
		user.ID, hash, time.Now().Add(time.Hour),
	)
	testutil.NoError(t, err)

	// Verify email.
	err = authSvc.ConfirmEmail(ctx, token)
	testutil.NoError(t, err)

	// Check user is verified.
	var verified bool
	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT email_verified FROM _ayb_users WHERE id = $1`, user.ID,
	).Scan(&verified)
	testutil.NoError(t, err)
	testutil.True(t, verified, "email should be verified")

	// Try to use same token again — should fail (token deleted after use).
	err = authSvc.ConfirmEmail(ctx, token)
	testutil.ErrorContains(t, err, "invalid or expired verification token")
}

func TestVerificationTokenExpiry(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	authSvc := newAuthService()

	// Create a user.
	user, err := auth.CreateUser(ctx, sharedPG.Pool, "expired@example.com", "password123", 8)
	testutil.NoError(t, err)

	// Insert an expired verification token.
	token := "expired-token-12345"
	hash := auth.HashTokenForTest(token)
	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_email_verifications (user_id, token_hash, expires_at)
		 VALUES ($1, $2, $3)`,
		user.ID, hash, time.Now().Add(-time.Hour), // expired
	)
	testutil.NoError(t, err)

	// Try to verify with expired token.
	err = authSvc.ConfirmEmail(ctx, token)
	testutil.ErrorContains(t, err, "invalid or expired verification token")
}

func TestVerificationTokenInvalidFormat(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	authSvc := newAuthService()

	// Try to verify with invalid token.
	err := authSvc.ConfirmEmail(ctx, "not-a-real-token")
	testutil.ErrorContains(t, err, "invalid or expired verification token")
}

// --- API key management integration tests ---

func registerAndGetToken(t *testing.T, srv *server.Server, email string) string {
	t.Helper()
	w := doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": email, "password": "password123",
	}, "")
	testutil.StatusCode(t, http.StatusCreated, w.Code)
	resp := parseAuthResp(t, w)
	return resp.Token
}

func TestAPIKeyCreateSuccess(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)
	token := registerAndGetToken(t, srv, "apikey-create@example.com")

	w := doJSON(t, srv, "POST", "/api/auth/api-keys/", map[string]string{
		"name": "my-key",
	}, token)
	testutil.StatusCode(t, http.StatusCreated, w.Code)

	var resp struct {
		Key    string `json:"key"`
		APIKey struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"apiKey"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	// API key should have realistic length (prefix + hash).
	testutil.True(t, len(resp.Key) >= 32, "apiKey should be at least 32 chars")
	testutil.Contains(t, resp.Key, "ayb_")
	testutil.Equal(t, "my-key", resp.APIKey.Name)
	// UUID should be exactly 36 chars (8-4-4-4-12 with hyphens).
	testutil.Equal(t, 36, len(resp.APIKey.ID))
}

func TestAPIKeyCreateWithScope(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)
	token := registerAndGetToken(t, srv, "apikey-scope@example.com")

	w := doJSON(t, srv, "POST", "/api/auth/api-keys/", map[string]any{
		"name":  "readonly-key",
		"scope": "readonly",
	}, token)
	testutil.StatusCode(t, http.StatusCreated, w.Code)

	var resp struct {
		Key    string `json:"key"`
		APIKey struct {
			Scope string `json:"scope"`
			Name  string `json:"name"`
		} `json:"apiKey"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	testutil.Equal(t, "readonly", resp.APIKey.Scope)
	testutil.Equal(t, "readonly-key", resp.APIKey.Name)
}

func TestAPIKeyCreateInvalidScope(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)
	token := registerAndGetToken(t, srv, "apikey-badscope@example.com")

	w := doJSON(t, srv, "POST", "/api/auth/api-keys/", map[string]string{
		"name":  "bad-scope-key",
		"scope": "admin",
	}, token)
	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid scope")
}

func TestAPIKeyListSuccess(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)
	token := registerAndGetToken(t, srv, "apikey-list@example.com")

	// Create two keys.
	for _, name := range []string{"key-1", "key-2"} {
		w := doJSON(t, srv, "POST", "/api/auth/api-keys/", map[string]string{
			"name": name,
		}, token)
		testutil.StatusCode(t, http.StatusCreated, w.Code)
	}

	// List keys.
	w := doJSON(t, srv, "GET", "/api/auth/api-keys/", nil, token)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	var keys []json.RawMessage
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &keys))
	testutil.Equal(t, 2, len(keys))
}

func TestAPIKeyListEmpty(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)
	token := registerAndGetToken(t, srv, "apikey-empty@example.com")

	w := doJSON(t, srv, "GET", "/api/auth/api-keys/", nil, token)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	var keys []json.RawMessage
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &keys))
	testutil.Equal(t, 0, len(keys))
}

func TestAPIKeyRevokeSuccess(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)
	token := registerAndGetToken(t, srv, "apikey-revoke@example.com")

	// Create a key.
	w := doJSON(t, srv, "POST", "/api/auth/api-keys/", map[string]string{
		"name": "to-revoke",
	}, token)
	testutil.StatusCode(t, http.StatusCreated, w.Code)

	var createResp struct {
		APIKey struct {
			ID string `json:"id"`
		} `json:"apiKey"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &createResp))
	testutil.True(t, createResp.APIKey.ID != "", "should return key ID")

	// Revoke it.
	w = doJSON(t, srv, "DELETE", "/api/auth/api-keys/"+createResp.APIKey.ID, nil, token)
	testutil.StatusCode(t, http.StatusNoContent, w.Code)

	// List should show the key with revokedAt set (key still exists, just revoked).
	w = doJSON(t, srv, "GET", "/api/auth/api-keys/", nil, token)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	var keys []struct {
		ID        string  `json:"id"`
		RevokedAt *string `json:"revokedAt"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &keys))
	testutil.Equal(t, 1, len(keys))
	testutil.NotNil(t, keys[0].RevokedAt)
}

func TestAPIKeyRevokeNotFound(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)
	token := registerAndGetToken(t, srv, "apikey-notfound@example.com")

	w := doJSON(t, srv, "DELETE", "/api/auth/api-keys/00000000-0000-0000-0000-000000000000", nil, token)
	testutil.StatusCode(t, http.StatusNotFound, w.Code)
	testutil.Contains(t, w.Body.String(), "api key not found")
}

func TestAPIKeyRevokeInvalidUUID(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)
	token := registerAndGetToken(t, srv, "apikey-baduuid@example.com")

	w := doJSON(t, srv, "DELETE", "/api/auth/api-keys/not-a-uuid", nil, token)
	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid api key id format")
}

func TestAPIKeyRevokeAlreadyRevoked(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)
	token := registerAndGetToken(t, srv, "apikey-double-revoke@example.com")

	// Create and revoke a key.
	w := doJSON(t, srv, "POST", "/api/auth/api-keys/", map[string]string{
		"name": "double-revoke",
	}, token)
	testutil.StatusCode(t, http.StatusCreated, w.Code)

	var createResp struct {
		APIKey struct {
			ID string `json:"id"`
		} `json:"apiKey"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &createResp))

	// First revoke succeeds.
	w = doJSON(t, srv, "DELETE", "/api/auth/api-keys/"+createResp.APIKey.ID, nil, token)
	testutil.StatusCode(t, http.StatusNoContent, w.Code)

	// Second revoke returns 404 (revoked_at IS NULL clause fails).
	w = doJSON(t, srv, "DELETE", "/api/auth/api-keys/"+createResp.APIKey.ID, nil, token)
	testutil.StatusCode(t, http.StatusNotFound, w.Code)
	testutil.Contains(t, w.Body.String(), "api key not found")
}

func TestAPIKeyIsolationBetweenUsers(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)
	token1 := registerAndGetToken(t, srv, "apikey-user1@example.com")
	token2 := registerAndGetToken(t, srv, "apikey-user2@example.com")

	// User 1 creates a key.
	w := doJSON(t, srv, "POST", "/api/auth/api-keys/", map[string]string{
		"name": "user1-key",
	}, token1)
	testutil.StatusCode(t, http.StatusCreated, w.Code)

	var createResp struct {
		APIKey struct {
			ID string `json:"id"`
		} `json:"apiKey"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &createResp))

	// User 2 cannot see user 1's keys.
	w = doJSON(t, srv, "GET", "/api/auth/api-keys/", nil, token2)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	var keys []json.RawMessage
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &keys))
	testutil.Equal(t, 0, len(keys))

	// User 2 cannot revoke user 1's key.
	w = doJSON(t, srv, "DELETE", "/api/auth/api-keys/"+createResp.APIKey.ID, nil, token2)
	testutil.StatusCode(t, http.StatusNotFound, w.Code)
}

// --- Magic link integration tests ---

func setupMagicLinkServer(t *testing.T, ctx context.Context) *server.Server {
	t.Helper()
	resetAndMigrate(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	if err := ch.Load(ctx); err != nil {
		t.Fatalf("loading schema cache: %v", err)
	}

	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = testJWTSecret
	cfg.Auth.MagicLinkEnabled = true

	authSvc := newAuthService()
	authSvc.SetMagicLinkDuration(10 * time.Minute)
	return server.New(cfg, logger, ch, sharedPG.Pool, authSvc, nil)
}

func TestMagicLinkRequestReturns200(t *testing.T) {
	ctx := context.Background()
	srv := setupMagicLinkServer(t, ctx)

	// Request for nonexistent email should still return 200 (no enumeration).
	w := doJSON(t, srv, "POST", "/api/auth/magic-link", map[string]string{
		"email": "nobody@example.com",
	}, "")
	testutil.StatusCode(t, http.StatusOK, w.Code)
	testutil.Contains(t, w.Body.String(), "if valid, a login link has been sent")
}

func TestMagicLinkFullFlowNewUser(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	authSvc := newAuthService()
	authSvc.SetMagicLinkDuration(10 * time.Minute)

	email := "newmagic@example.com"

	// Verify user doesn't exist yet.
	var count int
	err := sharedPG.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM _ayb_users WHERE LOWER(email) = $1", email,
	).Scan(&count)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, count)

	// Insert a magic link token directly (simulating what RequestMagicLink does).
	token := "test-magic-token-new-user"
	hash := auth.HashTokenForTest(token)
	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_magic_links (email, token_hash, expires_at)
		 VALUES ($1, $2, $3)`,
		email, hash, time.Now().Add(10*time.Minute),
	)
	testutil.NoError(t, err)

	// Confirm the magic link.
	user, accessToken, refreshToken, err := authSvc.ConfirmMagicLink(ctx, token)
	testutil.NoError(t, err)
	testutil.True(t, user.ID != "", "should create user")
	testutil.Equal(t, email, user.Email)
	testutil.True(t, accessToken != "", "should return access token")
	testutil.True(t, refreshToken != "", "should return refresh token")

	// Verify the access token works.
	claims, err := authSvc.ValidateToken(accessToken)
	testutil.NoError(t, err)
	testutil.Equal(t, user.ID, claims.Subject)

	// Verify user was created in DB with email_verified = true.
	var verified bool
	err = sharedPG.Pool.QueryRow(ctx,
		"SELECT email_verified FROM _ayb_users WHERE id = $1", user.ID,
	).Scan(&verified)
	testutil.NoError(t, err)
	testutil.True(t, verified, "email should be verified after magic link login")
}

func TestMagicLinkFullFlowExistingUser(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	authSvc := newAuthService()
	authSvc.SetMagicLinkDuration(10 * time.Minute)

	// Register a user first.
	existingUser, _, _, err := authSvc.Register(ctx, "existing@example.com", "password123")
	testutil.NoError(t, err)

	// Insert a magic link token for the existing user's email.
	token := "test-magic-token-existing"
	hash := auth.HashTokenForTest(token)
	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_magic_links (email, token_hash, expires_at)
		 VALUES ($1, $2, $3)`,
		existingUser.Email, hash, time.Now().Add(10*time.Minute),
	)
	testutil.NoError(t, err)

	// Confirm the magic link.
	user, accessToken, _, err := authSvc.ConfirmMagicLink(ctx, token)
	testutil.NoError(t, err)
	testutil.Equal(t, existingUser.ID, user.ID) // same user, not a new one
	testutil.True(t, accessToken != "", "should return access token")

	// Email should now be verified.
	var verified bool
	err = sharedPG.Pool.QueryRow(ctx,
		"SELECT email_verified FROM _ayb_users WHERE id = $1", user.ID,
	).Scan(&verified)
	testutil.NoError(t, err)
	testutil.True(t, verified, "email should be verified after magic link login")
}

func TestMagicLinkTokenConsumedAfterUse(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	authSvc := newAuthService()
	authSvc.SetMagicLinkDuration(10 * time.Minute)

	email := "consumed@example.com"
	token := "test-magic-token-consumed"
	hash := auth.HashTokenForTest(token)
	_, err := sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_magic_links (email, token_hash, expires_at)
		 VALUES ($1, $2, $3)`,
		email, hash, time.Now().Add(10*time.Minute),
	)
	testutil.NoError(t, err)

	// First use succeeds.
	_, _, _, err = authSvc.ConfirmMagicLink(ctx, token)
	testutil.NoError(t, err)

	// Second use fails (token consumed).
	_, _, _, err = authSvc.ConfirmMagicLink(ctx, token)
	testutil.ErrorContains(t, err, "invalid or expired magic link token")
}

func TestMagicLinkTokenExpired(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	authSvc := newAuthService()

	email := "expired-magic@example.com"
	token := "test-magic-token-expired"
	hash := auth.HashTokenForTest(token)
	_, err := sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_magic_links (email, token_hash, expires_at)
		 VALUES ($1, $2, $3)`,
		email, hash, time.Now().Add(-time.Hour), // already expired
	)
	testutil.NoError(t, err)

	_, _, _, err = authSvc.ConfirmMagicLink(ctx, token)
	testutil.ErrorContains(t, err, "invalid or expired magic link token")
}

func TestMagicLinkTokenInvalid(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	authSvc := newAuthService()

	_, _, _, err := authSvc.ConfirmMagicLink(ctx, "not-a-real-token")
	testutil.ErrorContains(t, err, "invalid or expired magic link token")
}

func TestMagicLinkHandlerConfirmFullFlow(t *testing.T) {
	ctx := context.Background()
	srv := setupMagicLinkServer(t, ctx)

	// Insert a token directly.
	email := "handler-flow@example.com"
	token := "test-handler-magic-token"
	hash := auth.HashTokenForTest(token)
	_, err := sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_magic_links (email, token_hash, expires_at)
		 VALUES ($1, $2, $3)`,
		email, hash, time.Now().Add(10*time.Minute),
	)
	testutil.NoError(t, err)

	// Confirm via HTTP.
	w := doJSON(t, srv, "POST", "/api/auth/magic-link/confirm", map[string]string{
		"token": token,
	}, "")
	testutil.StatusCode(t, http.StatusOK, w.Code)

	resp := parseAuthResp(t, w)
	testutil.True(t, resp.Token != "", "should return access token")
	testutil.True(t, resp.RefreshToken != "", "should return refresh token")
	testutil.Equal(t, email, resp.User["email"].(string))
}

func TestMagicLinkHandlerConfirmInvalidToken(t *testing.T) {
	ctx := context.Background()
	srv := setupMagicLinkServer(t, ctx)

	w := doJSON(t, srv, "POST", "/api/auth/magic-link/confirm", map[string]string{
		"token": "bogus-token",
	}, "")
	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid or expired magic link token")
}

func TestMagicLinkDisabledReturns404(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = testJWTSecret
	// MagicLinkEnabled defaults to false.

	authSvc := newAuthService()
	srv := server.New(cfg, logger, ch, sharedPG.Pool, authSvc, nil)

	w := doJSON(t, srv, "POST", "/api/auth/magic-link", map[string]string{
		"email": "test@example.com",
	}, "")
	testutil.StatusCode(t, http.StatusNotFound, w.Code)
	testutil.Contains(t, w.Body.String(), "not enabled")
}

func TestMagicLinkRequestMagicLinkDeletesPreviousTokens(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	authSvc := newAuthService()
	authSvc.SetMagicLinkDuration(10 * time.Minute)
	// Wire up a log mailer so RequestMagicLink actually runs (it's a no-op without a mailer).
	authSvc.SetMailer(mailer.NewLogMailer(testutil.DiscardLogger()), "TestApp", "http://localhost:8090/api")

	email := "cleanup@example.com"

	// Insert two tokens for the same email.
	for _, tok := range []string{"old-token-1", "old-token-2"} {
		hash := auth.HashTokenForTest(tok)
		_, err := sharedPG.Pool.Exec(ctx,
			`INSERT INTO _ayb_magic_links (email, token_hash, expires_at)
			 VALUES ($1, $2, $3)`,
			email, hash, time.Now().Add(10*time.Minute),
		)
		testutil.NoError(t, err)
	}

	// Verify 2 tokens exist.
	var count int
	err := sharedPG.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM _ayb_magic_links WHERE email = $1", email,
	).Scan(&count)
	testutil.NoError(t, err)
	testutil.Equal(t, 2, count)

	// Call the actual RequestMagicLink method — this should delete old tokens and insert a new one.
	err = authSvc.RequestMagicLink(ctx, email)
	testutil.NoError(t, err)

	// After cleanup + insert, should be exactly 1 (the new token).
	err = sharedPG.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM _ayb_magic_links WHERE email = $1", email,
	).Scan(&count)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, count)
}

